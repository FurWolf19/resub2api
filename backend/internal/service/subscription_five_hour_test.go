package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFiveHourWindowExpiryAndReset(t *testing.T) {
	start := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	sub := &UserSubscription{FiveHourWindowStart: &start, FiveHourUsageUSD: 3}

	require.False(t, sub.FiveHourWindowExpiredAt(start.Add(4*time.Hour+59*time.Minute)))
	require.True(t, sub.FiveHourWindowExpiredAt(start.Add(5*time.Hour)))
	require.NotNil(t, sub.FiveHourResetTime())
	require.Equal(t, start.Add(5*time.Hour), *sub.FiveHourResetTime())

	subs := []UserSubscription{*sub}
	normalizeExpiredWindowsAt(subs, start.Add(5*time.Hour))
	require.Nil(t, subs[0].FiveHourWindowStart, "过期窗口展示时应置空")
	require.Zero(t, subs[0].FiveHourUsageUSD)
}

func TestFiveHourLimitUsesEffectiveQuota(t *testing.T) {
	limit := 5.0
	group := &Group{FiveHourLimitUSD: &limit}
	sub := &UserSubscription{QuotaMultiplier: 0.5}

	require.True(t, sub.CheckFiveHourLimit(group, 0))
	require.True(t, sub.CheckFiveHourLimit(group, 2.5))
	require.False(t, sub.CheckFiveHourLimit(group, 2.6))

	sub.FiveHourUsageUSD = 2.5
	require.False(t, sub.CheckFiveHourLimit(group, 0.1))
}

func TestEffectiveLimitForAllWindows(t *testing.T) {
	daily, weekly, monthly, fiveHour := 10.0, 20.0, 30.0, 40.0
	group := &Group{
		DailyLimitUSD:    &daily,
		WeeklyLimitUSD:   &weekly,
		MonthlyLimitUSD:  &monthly,
		FiveHourLimitUSD: &fiveHour,
	}
	sub := &UserSubscription{QuotaMultiplier: 0.5}

	require.Equal(t, 5.0, sub.EffectiveLimit(group.DailyLimitUSD))
	require.Equal(t, 10.0, sub.EffectiveLimit(group.WeeklyLimitUSD))
	require.Equal(t, 15.0, sub.EffectiveLimit(group.MonthlyLimitUSD))
	require.Equal(t, 20.0, sub.EffectiveLimit(group.FiveHourLimitUSD))

	sub.QuotaMultiplier = 0
	require.True(t, sub.CheckDailyLimit(group, 0), "零倍率下 0 成本仍应放行")
	require.False(t, sub.CheckDailyLimit(group, 0.01), "零倍率下任何正消费都应超限")
	require.False(t, sub.CheckFiveHourLimit(group, 0.01))

	sub.QuotaMultiplier = 1
	sub.DailyUsageUSD = 9.5
	require.True(t, sub.CheckDailyLimit(group, 0.5))
	require.False(t, sub.CheckDailyLimit(group, 0.6))
}

func TestUnlimitedGroupWindowStaysUnlimitedWithMultiplier(t *testing.T) {
	sub := &UserSubscription{QuotaMultiplier: 0, DailyUsageUSD: 100, FiveHourUsageUSD: 100}
	group := &Group{}
	require.True(t, sub.CheckDailyLimit(group, 100))
	require.True(t, sub.CheckWeeklyLimit(group, 100))
	require.True(t, sub.CheckMonthlyLimit(group, 100))
	require.True(t, sub.CheckFiveHourLimit(group, 100))
}

func TestCalculateProgressIncludesFiveHourAndMultiplier(t *testing.T) {
	now := time.Now()
	start := now.Add(-time.Hour)
	fiveHour, daily := 10.0, 20.0
	sub := &UserSubscription{
		ID:                  1,
		ExpiresAt:           now.Add(10 * 24 * time.Hour),
		QuotaMultiplier:     0.5,
		FiveHourWindowStart: &start,
		FiveHourUsageUSD:    2,
		DailyWindowStart:    &start,
		DailyUsageUSD:       5,
	}
	group := &Group{Name: "g", FiveHourLimitUSD: &fiveHour, DailyLimitUSD: &daily}

	progress := (&SubscriptionService{}).calculateProgress(sub, group)
	require.Equal(t, 0.5, progress.QuotaMultiplier)
	require.NotNil(t, progress.FiveHour)
	require.Equal(t, 5.0, progress.FiveHour.LimitUSD)
	require.Equal(t, 2.0, progress.FiveHour.UsedUSD)
	require.Equal(t, 3.0, progress.FiveHour.RemainingUSD)
	require.Equal(t, 40.0, progress.FiveHour.Percentage)
	require.Equal(t, 10.0, progress.Daily.LimitUSD)
}

func TestRenewedTermResetsFiveHourButKeepsQuotaMultiplier(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	existing := &UserSubscription{
		ID:                  1,
		FiveHourWindowStart: &start,
		FiveHourUsageUSD:    4,
		QuotaMultiplier:     0.25,
	}
	renewed := renewedSubscriptionTerm(existing, "n", time.Now(), time.Now().Add(24*time.Hour))
	require.Nil(t, renewed.FiveHourWindowStart)
	require.Zero(t, renewed.FiveHourUsageUSD)
	require.Equal(t, 0.25, renewed.QuotaMultiplier)
}
