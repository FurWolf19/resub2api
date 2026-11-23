//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeFiveHourLimitUSD(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	tests := []struct {
		name string
		in   *float64
		want *float64
	}{
		{name: "nil stays nil", in: nil, want: nil},
		{name: "negative becomes nil", in: ptr(-1), want: nil},
		{name: "zero becomes nil", in: ptr(0), want: nil},
		{name: "tiny positive kept", in: ptr(0.0001), want: ptr(0.0001)},
		{name: "positive kept", in: ptr(2.5), want: ptr(2.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFiveHourLimitUSD(tt.in)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestCreateGroupZeroFiveHourLimitStoredAsNil(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}
	zero := 0.0

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "zero-five-hour",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeStandard,
		FiveHourLimitUSD: &zero,
	})
	require.NoError(t, err)
	require.Nil(t, group.FiveHourLimitUSD, "5h 限额 0 写为 nil")
	require.False(t, group.HasFiveHourLimit())
}

func TestUpdateGroupZeroFiveHourLimitStoredAsNil(t *testing.T) {
	five := 5.0
	repo := &groupRepoStubForAdmin{getByID: &Group{
		ID: 1, Name: "zero-five-hour-update", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1, FiveHourLimitUSD: &five,
	}}
	svc := &adminServiceImpl{groupRepo: repo}
	zero := 0.0

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{FiveHourLimitUSD: &zero})
	require.NoError(t, err)
	require.Nil(t, group.FiveHourLimitUSD)
	require.False(t, group.HasFiveHourLimit())
}

func TestUpdateGroupPositiveFiveHourLimitPreserved(t *testing.T) {
	repo := &groupRepoStubForAdmin{getByID: &Group{
		ID: 1, Name: "positive-five-hour", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1,
	}}
	svc := &adminServiceImpl{groupRepo: repo}
	limit := 2.5

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{FiveHourLimitUSD: &limit})
	require.NoError(t, err)
	require.NotNil(t, group.FiveHourLimitUSD)
	require.Equal(t, 2.5, *group.FiveHourLimitUSD)
	require.True(t, group.HasFiveHourLimit())
	require.Equal(t, 2.5, (&UserSubscription{QuotaMultiplier: 1}).EffectiveLimit(group.FiveHourLimitUSD))
}

func TestHasFiveHourLimitTreatsNilAndZeroAsUnlimited(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	tests := []struct {
		name  string
		limit *float64
		want  bool
	}{
		{name: "nil", limit: nil, want: false},
		{name: "zero", limit: ptr(0), want: false},
		{name: "negative", limit: ptr(-1), want: false},
		{name: "positive", limit: ptr(0.5), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Group{FiveHourLimitUSD: tt.limit}
			require.Equal(t, tt.want, g.HasFiveHourLimit())
		})
	}
}

func TestCheckFiveHourLimitTreatsHistoricalZeroAsUnlimited(t *testing.T) {
	zero := 0.0
	g := &Group{FiveHourLimitUSD: &zero}
	sub := &UserSubscription{FiveHourUsageUSD: 100}
	require.True(t, sub.CheckFiveHourLimit(g, 1000), "历史 0 不得阻断请求")
}

func TestDuplicateGroupNormalizesFiveHourLimit(t *testing.T) {
	zero := 0.0
	duplicate := cloneGroupForDuplicate(&Group{
		Name:             "source",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier:   1,
		FiveHourLimitUSD: &zero,
	}, "op")
	require.Nil(t, duplicate.FiveHourLimitUSD)
}
