package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIKeyAuthSnapshotV30MergedGroupFields 校验 v30 快照同时保留官方
// force_openai_ultrafast 与 fork 的 five_hour_limit_usd / peak_rate_windows。
func TestAPIKeyAuthSnapshotV30MergedGroupFields(t *testing.T) {
	groupID := int64(52)
	fiveHour := 12.5
	apiKey := &APIKey{
		ID: 84, UserID: 40, GroupID: &groupID, Key: "sk-v30-merged-roundtrip", Status: StatusActive,
		User: &User{ID: 40, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "v30-merged", Platform: PlatformOpenAI, Status: StatusActive,
			Hydrated:             true,
			ForceOpenAIUltrafast: true,
			FiveHourLimitUSD:     &fiveHour,
			PeakRateWindows: []PeakRateWindow{
				{Start: "09:00", End: "12:00", Multiplier: 1.5, Days: []int{1, 2, 3, 4, 5}},
			},
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	require.Contains(t, string(payload), `"force_openai_ultrafast":true`)
	require.Contains(t, string(payload), `"five_hour_limit_usd":12.5`)
	require.Contains(t, string(payload), `"peak_rate_windows"`)

	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))
	require.Equal(t, 30, cached.Snapshot.Version)

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.ForceOpenAIUltrafast)
	require.NotNil(t, materialized.Group.FiveHourLimitUSD)
	require.Equal(t, fiveHour, *materialized.Group.FiveHourLimitUSD)
	require.Equal(t, apiKey.Group.PeakRateWindows, materialized.Group.PeakRateWindows)
}
