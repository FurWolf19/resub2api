//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newPeakWindowsGroup(windows []PeakRateWindow, enabled bool) *Group {
	return &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  enabled,
		PeakRateWindows:  windows,
	}
}

func TestValidatePeakRateWindows(t *testing.T) {
	tests := []struct {
		name        string
		windows     []PeakRateWindow
		wantErr     bool
		errContains string
	}{
		{
			name:    "single window",
			windows: []PeakRateWindow{{Start: "12:00", End: "14:00", Multiplier: 2}},
		},
		{
			name: "multiple non-overlapping windows",
			windows: []PeakRateWindow{
				{Start: "08:00", End: "10:00", Multiplier: 1.5},
				{Start: "18:00", End: "20:00", Multiplier: 3},
			},
		},
		{
			name: "adjacent boundary allowed",
			windows: []PeakRateWindow{
				{Start: "12:00", End: "14:00", Multiplier: 2},
				{Start: "14:00", End: "16:00", Multiplier: 3},
			},
		},
		{
			name: "same time different days allowed",
			windows: []PeakRateWindow{
				{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
				{Start: "12:00", End: "14:00", Multiplier: 3, Days: []int{0, 6}},
			},
		},
		{
			name: "workday and weekend custom ranges allowed",
			windows: []PeakRateWindow{
				{Start: "09:00", End: "18:00", Multiplier: 1.2, Days: []int{1, 2, 3, 4, 5}},
				{Start: "10:00", End: "12:00", Multiplier: 1.5, Days: []int{6}},
			},
		},
		{
			name: "same days overlap rejected",
			windows: []PeakRateWindow{
				{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1}},
				{Start: "13:00", End: "15:00", Multiplier: 3, Days: []int{1}},
			},
			wantErr:     true,
			errContains: "重叠",
		},
		{
			name: "overlap only on shared weekday rejected",
			windows: []PeakRateWindow{
				{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
				{Start: "13:00", End: "15:00", Multiplier: 3, Days: []int{5}},
			},
			wantErr:     true,
			errContains: "周五",
		},
		{
			name: "empty days means everyday so overlaps",
			windows: []PeakRateWindow{
				{Start: "12:00", End: "14:00", Multiplier: 2},
				{Start: "13:00", End: "15:00", Multiplier: 3, Days: []int{3}},
			},
			wantErr:     true,
			errContains: "重叠",
		},
		{
			name:        "cross day rejected",
			windows:     []PeakRateWindow{{Start: "22:00", End: "02:00", Multiplier: 2}},
			wantErr:     true,
			errContains: "不支持跨天",
		},
		{
			name:        "equal start end rejected",
			windows:     []PeakRateWindow{{Start: "12:00", End: "12:00", Multiplier: 2}},
			wantErr:     true,
			errContains: "晚于开始时间",
		},
		{
			name:        "malformed start rejected",
			windows:     []PeakRateWindow{{Start: "9", End: "10:00", Multiplier: 2}},
			wantErr:     true,
			errContains: "HH:MM",
		},
		{
			name:        "single digit hour rejected",
			windows:     []PeakRateWindow{{Start: "9:00", End: "10:00", Multiplier: 2}},
			wantErr:     true,
			errContains: "HH:MM",
		},
		{
			name:    "zero padded hour accepted",
			windows: []PeakRateWindow{{Start: "09:00", End: "10:00", Multiplier: 2}},
		},
		{
			name:        "negative multiplier rejected",
			windows:     []PeakRateWindow{{Start: "12:00", End: "14:00", Multiplier: -1}},
			wantErr:     true,
			errContains: "非负",
		},
		{
			name:        "invalid weekday rejected",
			windows:     []PeakRateWindow{{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{7}}},
			wantErr:     true,
			errContains: "0..6",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePeakRateWindows(tt.windows)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPeakMultiplierAt_MultipleWindowsAndDays(t *testing.T) {
	monday := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)  // 周一
	saturday := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC) // 周六
	group := newPeakWindowsGroup([]PeakRateWindow{
		{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
		{Start: "12:00", End: "14:00", Multiplier: 3, Days: []int{0, 6}},
		{Start: "18:00", End: "20:00", Multiplier: 1.5},
	}, true)

	atTime := func(day time.Time, hour, minute int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC)
	}

	require.Equal(t, 2.0, group.PeakMultiplierAt(atTime(monday, 13, 0)))
	require.Equal(t, 3.0, group.PeakMultiplierAt(atTime(saturday, 13, 0)))
	require.Equal(t, 1.5, group.PeakMultiplierAt(atTime(saturday, 19, 0)))
	require.Equal(t, 1.0, group.PeakMultiplierAt(atTime(monday, 14, 0)), "end is exclusive")
	require.Equal(t, 1.0, group.PeakMultiplierAt(atTime(monday, 11, 59)))
}

func TestPeakMultiplierAt_FallsBackToLegacyFields(t *testing.T) {
	group := newPeakGroup(true, "14:00", "18:00", 3.0)

	require.Equal(t, 3.0, group.PeakMultiplierAt(at(15, 30)))
	require.Equal(t, 1.0, group.PeakMultiplierAt(at(20, 0)))

	// 新数组优先于旧字段。
	group.PeakRateWindows = []PeakRateWindow{{Start: "14:00", End: "18:00", Multiplier: 5}}
	require.Equal(t, 5.0, group.PeakMultiplierAt(at(15, 30)))
}

func TestPeakMultiplierAt_NonSubscriptionDegradesToOne(t *testing.T) {
	group := newPeakWindowsGroup([]PeakRateWindow{{Start: "12:00", End: "14:00", Multiplier: 2}}, true)
	group.SubscriptionType = SubscriptionTypeStandard
	require.Equal(t, 1.0, group.PeakMultiplierAt(at(13, 0)))
}

func TestNormalizePeakRateWindows(t *testing.T) {
	windows := []PeakRateWindow{
		{Start: " 18:00 ", End: " 20:00 ", Multiplier: 2, Days: []int{5, 1, 5, 3}},
		{Start: "08:00", End: "09:00", Multiplier: 1.5},
	}

	got := NormalizePeakRateWindows(SubscriptionTypeSubscription, true, windows)
	require.Len(t, got, 2)
	require.Equal(t, "18:00", got[0].Start)
	require.Equal(t, "20:00", got[0].End)
	require.Equal(t, []int{1, 3, 5}, got[0].Days)
	require.Empty(t, got[1].Days)
	// 保持窗口原始顺序，不按时间排序。
	require.Equal(t, "08:00", got[1].Start)

	// 订阅分组关闭高峰时保留窗口配置，便于重新开启；只有非订阅分组才清空。
	require.Len(t, NormalizePeakRateWindows(SubscriptionTypeSubscription, false, windows), 2)
	require.Empty(t, NormalizePeakRateWindows(SubscriptionTypeStandard, true, windows))
	require.Empty(t, NormalizePeakRateWindows(SubscriptionTypeStandard, false, windows))
}

func TestAdminService_CreateGroup_LegacyPeakFieldsGenerateWindows(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:               "legacy-peak",
		Platform:           PlatformOpenAI,
		RateMultiplier:     1,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "14:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: ptrFloatPeakWindows(2.5),
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Equal(t, []PeakRateWindow{{Start: "14:00", End: "18:00", Multiplier: 2.5}}, group.PeakRateWindows)
	require.Equal(t, "14:00", group.PeakStart)
}

func TestAdminService_CreateGroup_PeakRateWindowsSyncLegacyFields(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "multi-peak",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  true,
		PeakRateWindows: []PeakRateWindow{
			{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
			{Start: "12:00", End: "14:00", Multiplier: 3, Days: []int{0, 6}},
		},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.Len(t, group.PeakRateWindows, 2)
	require.Equal(t, "12:00", group.PeakStart)
	require.Equal(t, "14:00", group.PeakEnd)
	require.Equal(t, 2.0, group.PeakRateMultiplier)
}

func TestAdminService_CreateGroup_EnabledWithoutWindowsRejected(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:               "empty-windows",
		Platform:           PlatformOpenAI,
		RateMultiplier:     1,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakRateWindows:    []PeakRateWindow{},
		PeakRateWindowsSet: true,
	})
	require.ErrorContains(t, err, "至少配置一个高峰时段")
	require.Nil(t, repo.created)
}

func TestAdminService_UpdateGroup_PreservesWindowsWhenNotProvided(t *testing.T) {
	existing := &Group{
		ID:                 1,
		Name:               "multi-peak",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "12:00",
		PeakEnd:            "14:00",
		PeakRateMultiplier: 2,
		PeakRateWindows: []PeakRateWindow{
			{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
			{Start: "12:00", End: "14:00", Multiplier: 3, Days: []int{0, 6}},
		},
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{Description: ptrString("new description")})
	require.NoError(t, err)
	require.Len(t, updated.PeakRateWindows, 2, "unrelated update must not wipe multi-window config")
}

func TestAdminService_UpdateGroup_WindowsSyncLegacyFields(t *testing.T) {
	existing := &Group{
		ID:                 1,
		Name:               "multi-peak",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "12:00",
		PeakEnd:            "14:00",
		PeakRateMultiplier: 2,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	windows := []PeakRateWindow{
		{Start: "08:00", End: "10:00", Multiplier: 1.2, Days: []int{1, 2, 3, 4, 5}},
		{Start: "20:00", End: "22:00", Multiplier: 2.5},
	}
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateWindows:    windows,
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.Equal(t, windows, updated.PeakRateWindows)
	require.Equal(t, "08:00", updated.PeakStart)
	require.Equal(t, "10:00", updated.PeakEnd)
	require.Equal(t, 1.2, updated.PeakRateMultiplier)

	// 传空数组：清空 windows 与旧字段（enabled 仍为 true 时会被拒绝）。
	_, err = svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateWindows:    []PeakRateWindow{},
		PeakRateWindowsSet: true,
	})
	require.ErrorContains(t, err, "至少配置一个高峰时段")
}

func TestAdminService_UpdateGroup_DisableKeepsWindowsForReenable(t *testing.T) {
	windows := []PeakRateWindow{
		{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
		{Start: "12:00", End: "14:00", Multiplier: 3, Days: []int{0, 6}},
	}
	existing := &Group{
		ID:                 1,
		Name:               "multi-peak",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "12:00",
		PeakEnd:            "14:00",
		PeakRateMultiplier: 2,
		PeakRateWindows:    windows,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	disabled := false
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateEnabled:    &disabled,
		PeakRateWindows:    []PeakRateWindow{},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.False(t, updated.PeakRateEnabled)
	require.Equal(t, windows, updated.PeakRateWindows, "关闭开关不得丢失已有多窗口配置")
	require.Equal(t, 1.0, updated.PeakMultiplierAt(at(13, 0)), "关闭后即使命中窗口也不生效")

	enabled := true
	reEnabled, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateEnabled:    &enabled,
		PeakRateWindows:    updated.PeakRateWindows,
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.True(t, reEnabled.PeakRateEnabled)
	require.Equal(t, windows, reEnabled.PeakRateWindows)
	require.Equal(t, 2.0, reEnabled.PeakMultiplierAt(at(13, 0)))
}

func TestAdminService_CreateGroup_DisabledClearsWindows(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	windows := []PeakRateWindow{{Start: "12:00", End: "14:00", Multiplier: 2}}
	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:               "disabled-peak",
		Platform:           PlatformOpenAI,
		RateMultiplier:     1,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakRateWindows:    windows,
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.False(t, group.PeakRateEnabled)
	require.Empty(t, group.PeakRateWindows, "enabled=false 创建时窗口一律清空")
	require.Empty(t, group.PeakStart)
	require.Empty(t, group.PeakEnd)
	require.Equal(t, 1.0, group.PeakRateMultiplier)
}

func TestAdminService_CreateGroup_DisabledInvalidWindowsAreCleared(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "disabled-invalid-peak",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  false,
		PeakRateWindows: []PeakRateWindow{
			{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1}},
			{Start: "13:00", End: "15:00", Multiplier: 3, Days: []int{1}},
		},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err, "enabled=false 不校验窗口")
	require.NotNil(t, repo.created)
	require.Empty(t, group.PeakRateWindows)
}

func TestAdminService_UpdateGroup_DisabledIgnoresInvalidWindows(t *testing.T) {
	existing := &Group{
		ID:                 1,
		Name:               "multi-peak",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakRateWindows:    []PeakRateWindow{{Start: "08:00", End: "10:00", Multiplier: 2}},
		PeakStart:          "08:00",
		PeakEnd:            "10:00",
		PeakRateMultiplier: 2,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	disabled := false
	_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateEnabled: &disabled,
		PeakRateWindows: []PeakRateWindow{
			{Start: "22:00", End: "02:00", Multiplier: 2},
		},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err, "enabled=false 更新不得校验提交的窗口")
	require.NotNil(t, repo.updated)
	require.Equal(t, existing.PeakRateWindows, repo.updated.PeakRateWindows, "停用时必须保留已有窗口")
	require.Equal(t, "08:00", repo.updated.PeakStart)
}

func TestAdminService_UpdateGroup_LegacyEnableRejectsPersistedInvalidWindows(t *testing.T) {
	existing := &Group{
		ID:                 1,
		Name:               "stale-invalid-peak",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakStart:          "12:00",
		PeakEnd:            "14:00",
		PeakRateMultiplier: 2,
		PeakRateWindows: []PeakRateWindow{
			{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1}},
			{Start: "13:00", End: "15:00", Multiplier: 3, Days: []int{1}},
		},
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	enabled := true
	_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{PeakRateEnabled: &enabled})
	require.ErrorContains(t, err, "重叠", "旧 API 仅打开开关也不能让历史非法窗口生效")
	require.Nil(t, repo.updated)
}

func TestAdminService_UpdateGroup_DisableThenLegacyEnableAppliesKeptWindows(t *testing.T) {
	windows := []PeakRateWindow{
		{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
		{Start: "12:00", End: "14:00", Multiplier: 3, Days: []int{0, 6}},
	}
	existing := &Group{
		ID:                 1,
		Name:               "multi-peak",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakStart:          "12:00",
		PeakEnd:            "14:00",
		PeakRateMultiplier: 2,
		PeakRateWindows:    windows,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	enabled := true
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{PeakRateEnabled: &enabled})
	require.NoError(t, err)
	require.True(t, updated.PeakRateEnabled)
	require.Equal(t, windows, updated.PeakRateWindows, "旧 API 启用时不得丢失保留的多窗口配置")
	require.Equal(t, 2.0, updated.PeakMultiplierAt(at(13, 0)))
}

func ptrFloatPeakWindows(v float64) *float64 { return &v }

func TestPeakRateWindowsFromLegacyConfig_NormalizesLegacyTime(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
		windows    []PeakRateWindow
	}{
		{
			name:  "single digit hour formatted",
			start: "9:00", end: "18:00",
			windows: []PeakRateWindow{{Start: "09:00", End: "18:00", Multiplier: 2}},
		},
		{
			name:  "both single digit formatted",
			start: "9:5", end: "18:30",
			windows: []PeakRateWindow{{Start: "09:05", End: "18:30", Multiplier: 2}},
		},
		{
			name:  "already padded unchanged",
			start: "09:00", end: "18:00",
			windows: []PeakRateWindow{{Start: "09:00", End: "18:00", Multiplier: 2}},
		},
		{
			name:  "invalid hour rejected",
			start: "25:00", end: "26:00",
			windows: nil,
		},
		{
			name:  "invalid minute rejected",
			start: "09:60", end: "18:00",
			windows: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.windows, peakRateWindowsFromLegacyConfig(tt.start, tt.end, 2))
		})
	}
}

func TestNormalizeStoredPeakRateWindows(t *testing.T) {
	windows := []PeakRateWindow{
		{Start: "9:00", End: "18:5", Multiplier: 2, Days: []int{1}},
		{Start: "09:00", End: "18:00", Multiplier: 3},
		{Start: "25:00", End: "09:60", Multiplier: 4},
	}
	got := NormalizeStoredPeakRateWindows(windows)
	require.Equal(t, "09:00", got[0].Start)
	require.Equal(t, "18:05", got[0].End)
	require.Equal(t, []int{1}, got[0].Days, "days and multiplier must be preserved")
	require.Equal(t, "09:00", got[1].Start)
	require.Equal(t, "25:00", got[2].Start, "invalid stored time must stay invalid for validation")
	require.Equal(t, "09:60", got[2].End)

	require.Empty(t, NormalizeStoredPeakRateWindows(nil))
}

func TestAdminService_CreateGroup_LegacySingleDigitHourNormalized(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:               "legacy-single-digit",
		Platform:           PlatformOpenAI,
		RateMultiplier:     1,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "9:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: ptrFloatPeakWindows(2),
	})
	require.NoError(t, err)
	require.Equal(t, []PeakRateWindow{{Start: "09:00", End: "18:00", Multiplier: 2}}, group.PeakRateWindows)
}

func TestAdminService_UpdateGroup_LegacySingleDigitHourOnlyEnableSucceeds(t *testing.T) {
	existing := &Group{
		ID:                 1,
		Name:               "legacy-single-digit",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakStart:          "9:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: 2,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	enabled := true
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{PeakRateEnabled: &enabled})
	require.NoError(t, err)
	require.Equal(t, []PeakRateWindow{{Start: "09:00", End: "18:00", Multiplier: 2}}, updated.PeakRateWindows,
		"旧字段的宽松格式必须回填为两位小时，后续新校验才不会误伤")
}

func TestAdminService_UpdateGroup_NormalizesStoredLegacyWindows(t *testing.T) {
	existing := &Group{
		ID:                 1,
		Name:               "stored-legacy-windows",
		Platform:           PlatformOpenAI,
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakStart:          "9:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: 2,
		PeakRateWindows:    []PeakRateWindow{{Start: "9:00", End: "18:00", Multiplier: 2}},
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	enabled := true
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{PeakRateEnabled: &enabled})
	require.NoError(t, err)
	require.Equal(t, []PeakRateWindow{{Start: "09:00", End: "18:00", Multiplier: 2}}, updated.PeakRateWindows)
}

func TestAdminService_CreateGroup_NewAPIRejectsLooseWindowTime(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "strict-window-time",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  true,
		PeakRateWindows: []PeakRateWindow{
			{Start: "9:00", End: "10:00", Multiplier: 2},
		},
		PeakRateWindowsSet: true,
	})
	require.ErrorContains(t, err, "HH:MM")
	require.Nil(t, repo.created, "新 API 显式提交的宽松时间必须被拒绝")
}
