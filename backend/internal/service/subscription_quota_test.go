package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type quotaTestRepo struct {
	userSubRepoNoop

	sub       *UserSubscription
	active    map[int64]*UserSubscription
	created   *UserSubscription
	updated   *UserSubscription
	quotaByID map[int64]float64
	defaults  map[int64]float64

	resetFiveHourCalled   bool
	resetFiveHourExpected *time.Time
	resetByID             int64
}

func (r *quotaTestRepo) Create(_ context.Context, sub *UserSubscription) error {
	cp := *sub
	if cp.ID == 0 {
		cp.ID = 100
	}
	sub.ID = cp.ID
	r.created = &cp
	r.sub = &cp
	return nil
}

func (r *quotaTestRepo) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *quotaTestRepo) GetByIDForUpdate(_ context.Context, id int64) (*UserSubscription, error) {
	return r.GetByID(context.Background(), id)
}

func (r *quotaTestRepo) GetByUserIDAndGroupID(_ context.Context, _, _ int64) (*UserSubscription, error) {
	if r.sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *quotaTestRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, _ int64) (*UserSubscription, error) {
	if sub, ok := r.active[userID]; ok && sub != nil {
		cp := *sub
		return &cp, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (r *quotaTestRepo) ExistsByUserIDAndGroupID(_ context.Context, _, _ int64) (bool, error) {
	return r.sub != nil, nil
}

func (r *quotaTestRepo) Update(_ context.Context, sub *UserSubscription) error {
	cp := *sub
	r.updated = &cp
	r.sub = &cp
	return nil
}

func (r *quotaTestRepo) ExtendExpiry(_ context.Context, _ int64, expiresAt time.Time) error {
	if r.sub != nil {
		r.sub.ExpiresAt = expiresAt
	}
	return nil
}

func (r *quotaTestRepo) UpdateStatus(_ context.Context, id int64, status string) error {
	if r.sub != nil && r.sub.ID == id {
		r.sub.Status = status
	}
	return nil
}

func (r *quotaTestRepo) UpdateNotes(_ context.Context, id int64, notes string) error {
	if r.sub != nil && r.sub.ID == id {
		r.sub.Notes = notes
	}
	return nil
}

func (r *quotaTestRepo) ResetUsageWindows(_ context.Context, _ int64, daily, weekly, monthly bool, dailyStart, periodicStart time.Time) error {
	if r.sub == nil {
		return ErrSubscriptionNotFound
	}
	if daily {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &dailyStart
	}
	if weekly {
		r.sub.WeeklyUsageUSD = 0
		r.sub.WeeklyWindowStart = &periodicStart
	}
	if monthly {
		r.sub.MonthlyUsageUSD = 0
		r.sub.MonthlyWindowStart = &periodicStart
	}
	return nil
}

func (r *quotaTestRepo) ResetFiveHourUsage(_ context.Context, id int64, expected *time.Time) error {
	r.resetFiveHourCalled = true
	r.resetFiveHourExpected = expected
	r.resetByID = id
	if r.sub != nil && r.sub.ID == id {
		r.sub.FiveHourUsageUSD = 0
		r.sub.FiveHourWindowStart = nil
	}
	return nil
}

func (r *quotaTestRepo) SetSubscriptionQuotaMultiplier(_ context.Context, id int64, multiplier float64) error {
	if r.quotaByID == nil {
		r.quotaByID = map[int64]float64{}
	}
	r.quotaByID[id] = multiplier
	if r.sub != nil && r.sub.ID == id {
		r.sub.QuotaMultiplier = multiplier
	}
	if sub, ok := r.active[id]; ok && sub != nil {
		sub.QuotaMultiplier = multiplier
	}
	return nil
}

func (r *quotaTestRepo) ListGroupQuotaMultipliers(context.Context, int64) ([]GroupQuotaMultiplierEntry, error) {
	return nil, nil
}

func (r *quotaTestRepo) UpsertUserGroupQuotaDefault(_ context.Context, userID, _ int64, multiplier float64) error {
	if r.defaults == nil {
		r.defaults = map[int64]float64{}
	}
	r.defaults[userID] = multiplier
	return nil
}

func (r *quotaTestRepo) DeleteUserGroupQuotaDefault(_ context.Context, userID, _ int64) error {
	delete(r.defaults, userID)
	return nil
}

func newQuotaTestService(repo *quotaTestRepo) *SubscriptionService {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeSubscription, Name: "g"}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: group}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	return svc
}

func TestAssignSubscriptionQuotaMultiplierRules(t *testing.T) {
	t.Run("new subscription defaults to 1", func(t *testing.T) {
		repo := &quotaTestRepo{}
		svc := newQuotaTestService(repo)
		_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1, GroupID: 7})
		require.NoError(t, err)
		require.NotNil(t, repo.created)
		require.Equal(t, 1.0, repo.created.QuotaMultiplier)
	})

	t.Run("existing active subscription preserves when omitted", func(t *testing.T) {
		repo := &quotaTestRepo{sub: &UserSubscription{
			ID: 9, UserID: 1, GroupID: 7, Status: SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(24 * time.Hour), QuotaMultiplier: 0.3,
		}}
		svc := newQuotaTestService(repo)
		sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1, GroupID: 7})
		require.NoError(t, err)
		require.Equal(t, 0.3, sub.QuotaMultiplier)
		require.Empty(t, repo.quotaByID)
	})

	t.Run("existing active subscription explicit value overrides", func(t *testing.T) {
		repo := &quotaTestRepo{sub: &UserSubscription{
			ID: 9, UserID: 1, GroupID: 7, Status: SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(24 * time.Hour), QuotaMultiplier: 0.3,
		}}
		svc := newQuotaTestService(repo)
		value := 0.0
		sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
			UserID: 1, GroupID: 7, QuotaMultiplier: &value,
		})
		require.NoError(t, err)
		require.Zero(t, sub.QuotaMultiplier)
		require.Equal(t, 0.0, repo.quotaByID[9])
	})

	t.Run("expired renewal keeps existing multiplier", func(t *testing.T) {
		repo := &quotaTestRepo{sub: &UserSubscription{
			ID: 9, UserID: 1, GroupID: 7, Status: SubscriptionStatusExpired,
			StartsAt: time.Now().Add(-48 * time.Hour), ExpiresAt: time.Now().Add(-24 * time.Hour),
			QuotaMultiplier: 0.4,
		}}
		svc := newQuotaTestService(repo)
		sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1, GroupID: 7})
		require.NoError(t, err)
		require.Equal(t, 0.4, sub.QuotaMultiplier)
		require.NotNil(t, repo.updated)
		require.Equal(t, 0.4, repo.updated.QuotaMultiplier)
	})

	t.Run("rejects out of range multiplier", func(t *testing.T) {
		repo := &quotaTestRepo{}
		svc := newQuotaTestService(repo)
		value := 1.5
		_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
			UserID: 1, GroupID: 7, QuotaMultiplier: &value,
		})
		require.ErrorIs(t, err, ErrInvalidQuotaMultiplier)
	})
}

func TestAdminResetQuotaFiveHourClearsWindow(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	repo := &quotaTestRepo{sub: &UserSubscription{
		ID: 9, UserID: 1, GroupID: 7, FiveHourWindowStart: &start, FiveHourUsageUSD: 3,
	}}
	svc := newQuotaTestService(repo)
	sub, err := svc.AdminResetQuotaWithFiveHour(context.Background(), 9, false, false, false, true)
	require.NoError(t, err)
	require.True(t, repo.resetFiveHourCalled)
	require.Equal(t, int64(9), repo.resetByID)
	require.Nil(t, sub.FiveHourWindowStart)
	require.Zero(t, sub.FiveHourUsageUSD)
}

func TestCheckAndResetWindowsFiveHourDoesNotOpenNewWindow(t *testing.T) {
	repo := &quotaTestRepo{sub: &UserSubscription{ID: 9, UserID: 1, GroupID: 7}}
	svc := newQuotaTestService(repo)
	start := svc.now().Add(-6 * time.Hour)
	sub := &UserSubscription{ID: 9, UserID: 1, GroupID: 7, FiveHourWindowStart: &start, FiveHourUsageUSD: 2}

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))
	require.True(t, repo.resetFiveHourCalled)
	require.Nil(t, sub.FiveHourWindowStart)
	require.Zero(t, sub.FiveHourUsageUSD)
}

func TestBatchSetGroupQuotaMultipliers(t *testing.T) {
	repo := &quotaTestRepo{
		active: map[int64]*UserSubscription{
			1: {ID: 11, UserID: 1, GroupID: 7, Status: SubscriptionStatusActive, QuotaMultiplier: 1},
		},
		defaults: map[int64]float64{},
	}
	svc := newQuotaTestService(repo)
	half := 0.5
	one := 1.0

	err := svc.BatchSetGroupQuotaMultipliers(context.Background(), 7, []GroupQuotaMultiplierInput{
		{UserID: 1, QuotaMultiplier: &half},
		{UserID: 2, QuotaMultiplier: &half},
		{UserID: 3, QuotaMultiplier: &one},
		{UserID: 4, QuotaMultiplier: nil},
	})
	require.NoError(t, err)
	require.Equal(t, 0.5, repo.quotaByID[11], "有有效订阅的用户应更新订阅")
	require.Equal(t, 0.5, repo.defaults[2], "无有效订阅的用户应写入默认值")
	require.Equal(t, 1.0, repo.defaults[3])
	_, exists := repo.defaults[4]
	require.False(t, exists, "nil 应删除无订阅用户的默认值")
}

func TestBatchSetGroupQuotaMultiplierValidation(t *testing.T) {
	svc := newQuotaTestService(&quotaTestRepo{})
	bad := 1.01
	err := svc.BatchSetGroupQuotaMultipliers(context.Background(), 7, []GroupQuotaMultiplierInput{{UserID: 1, QuotaMultiplier: &bad}})
	require.ErrorIs(t, err, ErrInvalidQuotaMultiplier)
}
