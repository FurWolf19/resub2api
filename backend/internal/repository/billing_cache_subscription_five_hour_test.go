package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newBillingCacheForTest(t *testing.T) *billingCache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewBillingCache(client).(*billingCache)
}

func TestUpdateSubscriptionUsageFiveHourWindow(t *testing.T) {
	cache := newBillingCacheForTest(t)
	ctx := context.Background()
	key := billingSubKey(1, 2)

	require.NoError(t, cache.SetSubscriptionCache(ctx, 1, 2, &service.SubscriptionCacheData{
		Status:          service.SubscriptionStatusActive,
		ExpiresAt:       time.Now().Add(time.Hour),
		QuotaMultiplier: 1,
	}))

	// cost=0 不开启 5h 窗口。
	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, 0))
	data, err := cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Nil(t, data.FiveHourWindowStart, "cost=0 不得开启 5h 窗口")
	require.Zero(t, data.FiveHourUsage)

	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, 2))
	data, err = cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 2.0, data.FiveHourUsage)
	require.NotNil(t, data.FiveHourWindowStart, "cost>0 应开启 5h 窗口")

	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, 3))
	data, err = cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 5.0, data.FiveHourUsage, "窗口内应累加")

	// cost=0 不推进窗口、不清零用量。
	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, 0))
	data, err = cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 5.0, data.FiveHourUsage)
	require.NotNil(t, data.FiveHourWindowStart)

	// 模拟 5h 窗口过期：下一笔 cost>0 重新开窗，用量从本笔重新开始。
	oldStart := time.Now().Add(-6 * time.Hour)
	require.NoError(t, cache.SetSubscriptionCache(ctx, 1, 2, &service.SubscriptionCacheData{
		Status:              service.SubscriptionStatusActive,
		ExpiresAt:           time.Now().Add(time.Hour),
		FiveHourUsage:       5,
		FiveHourWindowStart: &oldStart,
		QuotaMultiplier:     1,
	}))
	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, 1.5))
	data, err = cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 1.5, data.FiveHourUsage, "过期后首笔应重置为新窗口用量")
	require.True(t, data.FiveHourWindowStart.After(oldStart))

	// 旧缓存缺少 quota_multiplier 时按 1.0 兼容。
	require.NoError(t, cache.rdb.HSet(ctx, key, "quota_multiplier", "").Err())
	data, err = cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 1.0, data.QuotaMultiplier)
}

func TestUpdateSubscriptionUsageNegativeReversesWithoutOpeningFiveHourWindow(t *testing.T) {
	cache := newBillingCacheForTest(t)
	ctx := context.Background()

	require.NoError(t, cache.SetSubscriptionCache(ctx, 1, 2, &service.SubscriptionCacheData{
		Status:          service.SubscriptionStatusActive,
		ExpiresAt:       time.Now().Add(time.Hour),
		QuotaMultiplier: 1,
	}))
	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, 5))
	before, err := cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.NotNil(t, before.FiveHourWindowStart)

	// 负额只冲正日/周/月与当前 5h 用量，不开启/推进窗口。
	require.NoError(t, cache.UpdateSubscriptionUsage(ctx, 1, 2, -10))
	after, err := cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.InDelta(t, -5, after.DailyUsage, 0.000001)
	require.InDelta(t, -5, after.WeeklyUsage, 0.000001)
	require.InDelta(t, -5, after.MonthlyUsage, 0.000001)
	require.Zero(t, after.FiveHourUsage, "5h 冲正后不得为负")
	require.Equal(t, before.FiveHourWindowStart.Unix(), after.FiveHourWindowStart.Unix(), "冲正不得改变窗口起点")
}

func TestParseSubscriptionCacheQuotaMultiplierFallback(t *testing.T) {
	cache := newBillingCacheForTest(t)
	ctx := context.Background()
	key := billingSubKey(1, 2)

	require.NoError(t, cache.SetSubscriptionCache(ctx, 1, 2, &service.SubscriptionCacheData{
		Status:          service.SubscriptionStatusActive,
		ExpiresAt:       time.Now().Add(time.Hour),
		QuotaMultiplier: 1,
	}))

	// 非法字符串必须回退到 1.0，不能变成 0 导致全量拦截。
	require.NoError(t, cache.rdb.HSet(ctx, key, "quota_multiplier", "not-a-number").Err())
	data, err := cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 1.0, data.QuotaMultiplier)

	// 合法的 0 必须保持 0。
	require.NoError(t, cache.rdb.HSet(ctx, key, "quota_multiplier", "0").Err())
	data, err = cache.GetSubscriptionCache(ctx, 1, 2)
	require.NoError(t, err)
	require.Zero(t, data.QuotaMultiplier)
}
