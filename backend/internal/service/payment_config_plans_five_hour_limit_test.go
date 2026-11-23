//go:build unit

package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestGetGroupInfoMapNormalizesHistoricalZeroFiveHourLimit(t *testing.T) {
	ctx := context.Background()
	svc := newPaymentConfigPlansTestService(t)

	zeroGroup, err := svc.entClient.Group.Create().
		SetName("zero-five-hour-payment-plan-group").
		SetPlatform(PlatformOpenAI).
		SetFiveHourLimitUsd(0).
		Save(ctx)
	require.NoError(t, err)

	positiveGroup, err := svc.entClient.Group.Create().
		SetName("positive-five-hour-payment-plan-group").
		SetPlatform(PlatformOpenAI).
		SetFiveHourLimitUsd(2.5).
		Save(ctx)
	require.NoError(t, err)

	info := svc.GetGroupInfoMap(ctx, []*dbent.SubscriptionPlan{
		{GroupID: zeroGroup.ID},
		{GroupID: positiveGroup.ID},
	})
	require.Contains(t, info, zeroGroup.ID)
	require.Nil(t, info[zeroGroup.ID].FiveHourLimitUSD, "历史 0 读取时归一化为无限制")
	require.Contains(t, info, positiveGroup.ID)
	require.NotNil(t, info[positiveGroup.ID].FiveHourLimitUSD)
	require.InDelta(t, 2.5, *info[positiveGroup.ID].FiveHourLimitUSD, 1e-9)
}
