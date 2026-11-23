//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type quotaDefaultsPrefillRepo struct {
	*quotaTestRepo
	listCalls int
}

// ListGroupQuotaMultipliers 模拟分配弹窗预填数据；createSubscription 若误读
// 默认表，会拿到这里的 0.4 并反映到新订阅或触发 listCalls。
func (r *quotaDefaultsPrefillRepo) ListGroupQuotaMultipliers(_ context.Context, _ int64) ([]GroupQuotaMultiplierEntry, error) {
	r.listCalls++
	defaultMultiplier := 0.4
	return []GroupQuotaMultiplierEntry{{
		UserID: 1, DefaultQuotaMultiplier: &defaultMultiplier,
	}}, nil
}

func newQuotaDefaultsTestService(repo *quotaDefaultsPrefillRepo) *SubscriptionService {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeSubscription, Name: "g"}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: group}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	return svc
}

func TestAssignSubscriptionIgnoresQuotaDefault(t *testing.T) {
	repo := &quotaDefaultsPrefillRepo{quotaTestRepo: &quotaTestRepo{}}
	svc := newQuotaDefaultsTestService(repo)

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1, GroupID: 7})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, 1.0, repo.created.QuotaMultiplier, "默认倍率表只用于弹窗预填，不得自动应用")
	require.Equal(t, 1.0, sub.QuotaMultiplier)
	require.Zero(t, repo.listCalls, "分配路径不得读取默认倍率表")
}

func TestBatchClearQuotaMultiplierWritesOne(t *testing.T) {
	repo := &quotaTestRepo{
		active: map[int64]*UserSubscription{
			1: {ID: 11, UserID: 1, GroupID: 7, Status: SubscriptionStatusActive, QuotaMultiplier: 0.4},
		},
		defaults: map[int64]float64{1: 0.4},
	}
	svc := newQuotaTestService(repo)

	err := svc.BatchSetGroupQuotaMultipliers(context.Background(), 7, []GroupQuotaMultiplierInput{{UserID: 1, QuotaMultiplier: nil}})
	require.NoError(t, err)
	require.Equal(t, 1.0, repo.quotaByID[11], "清空必须写 1.0，不能写 NULL 或回退默认表")
	require.Equal(t, 0.4, repo.defaults[1], "默认表与订阅清空互不影响")
	require.Equal(t, 10.0, (&UserSubscription{QuotaMultiplier: repo.quotaByID[11]}).EffectiveLimit(float64PtrForTest(10)))
}

func TestRenewExpiredSubscriptionClearsFiveHourWindowThroughRepoUpdate(t *testing.T) {
	windowStart := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	repo := &quotaTestRepo{sub: &UserSubscription{
		ID: 9, UserID: 1, GroupID: 7, Status: SubscriptionStatusExpired,
		StartsAt:            time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		ExpiresAt:           time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
		FiveHourWindowStart: &windowStart,
		FiveHourUsageUSD:    3,
		QuotaMultiplier:     1,
	}}
	svc := newQuotaTestService(repo)

	sub, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1, GroupID: 7})
	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, repo.updated, "续费必须走仓储 Update 落库")
	require.Nil(t, repo.updated.FiveHourWindowStart, "续费交给仓储的窗口必须为 nil")
	require.Zero(t, repo.updated.FiveHourUsageUSD)
	require.Equal(t, 1.0, repo.updated.QuotaMultiplier)
	require.NotNil(t, sub)
	require.Nil(t, sub.FiveHourWindowStart)
}

func float64PtrForTest(v float64) *float64 { return &v }
