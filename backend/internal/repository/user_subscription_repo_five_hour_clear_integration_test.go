//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 本用例只验证仓储层：Update 在 nil 时真实写 NULL，随后 IncrementUsage 在真实
// PostgreSQL 上重新起窗。service 续费把窗口置 nil 的交接由 unit 测试覆盖。
func TestRepoFiveHourWindowClearPersistsNullThenPaidUsageReopens(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)

	user, err := client.User.Create().
		SetEmail("five-hour-e2e@example.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("five-hour-e2e-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	windowStart := now.Add(-2 * time.Hour)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(now.Add(-48 * time.Hour)).
		SetExpiresAt(now.Add(-24 * time.Hour)).
		SetStatus(service.SubscriptionStatusExpired).
		SetAssignedAt(now).
		SetNotes("").
		SetFiveHourWindowStart(windowStart).
		SetFiveHourUsageUsd(3).
		SetQuotaMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	loaded, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	loaded.FiveHourWindowStart = nil
	loaded.FiveHourUsageUSD = 0
	require.NoError(t, repo.Update(ctx, loaded))

	renewed, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.Nil(t, renewed.FiveHourWindowStart, "续费后数据库窗口起点必须为 NULL")
	require.Zero(t, renewed.FiveHourUsageUsd)

	require.NoError(t, repo.IncrementUsage(ctx, sub.ID, 1.25))
	afterUsage, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.NotNil(t, afterUsage.FiveHourWindowStart, "首笔付费消费必须重新起窗")
	require.WithinDuration(t, time.Now(), *afterUsage.FiveHourWindowStart, time.Minute)
	require.InDelta(t, 1.25, afterUsage.FiveHourUsageUsd, 1e-9)
}
