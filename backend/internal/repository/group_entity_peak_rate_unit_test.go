package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToService_NormalizesStoredLegacyPeakWindowTimes(t *testing.T) {
	group := &dbent.Group{
		ID:               1,
		Name:             "stored-legacy-peak",
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeSubscription,
		RateMultiplier:   1,
		PeakRateEnabled:  false,
		PeakStart:        "9:00",
		PeakEnd:          "18:00",
		PeakRateWindows: []service.PeakRateWindow{
			{Start: "9:00", End: "18:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
			{Start: "25:00", End: "09:60", Multiplier: 3},
		},
	}

	got := groupEntityToService(group)
	require.NotNil(t, got)
	require.Equal(t, "09:00", got.PeakRateWindows[0].Start)
	require.Equal(t, "18:00", got.PeakRateWindows[0].End)
	require.Equal(t, []int{1, 2, 3, 4, 5}, got.PeakRateWindows[0].Days)
	// 非法时间保持原样，仍会被严格校验拒绝。
	require.Equal(t, "25:00", got.PeakRateWindows[1].Start)
	require.Equal(t, "09:60", got.PeakRateWindows[1].End)
	// 旧字段本身不做改写，运行期由 peakRateWindowsFromLegacyConfig 归一化。
	require.Equal(t, "9:00", got.PeakStart)
}
