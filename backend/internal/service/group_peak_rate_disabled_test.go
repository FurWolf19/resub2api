//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateGroupDisabledValidWindowsAreAlsoCleared(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "disabled-valid-peak",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  false,
		PeakRateWindows: []PeakRateWindow{
			{Start: "08:00", End: "10:00", Multiplier: 2},
			{Start: "18:00", End: "20:00", Multiplier: 3},
		},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.Empty(t, group.PeakRateWindows, "enabled=false 不保留任何提交窗口")
	require.Empty(t, group.PeakStart)
	require.Empty(t, group.PeakEnd)
	require.Equal(t, 1.0, group.PeakRateMultiplier)
}

func TestCreateGroupEnabledInvalidWindowsStillRejected(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "enabled-invalid-peak",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  true,
		PeakRateWindows: []PeakRateWindow{
			{Start: "22:00", End: "02:00", Multiplier: 2},
		},
		PeakRateWindowsSet: true,
	})
	require.ErrorContains(t, err, "不支持跨天")
	require.Nil(t, repo.created)
}

func TestUpdateGroupDisabledValidWindowsAreIgnored(t *testing.T) {
	existing := []PeakRateWindow{
		{Start: "08:00", End: "10:00", Multiplier: 2},
		{Start: "18:00", End: "20:00", Multiplier: 3},
	}
	repo := &groupRepoStubForAdmin{getByID: &Group{
		ID: 1, Name: "disabled-ignores-valid", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		PeakRateEnabled:  false,
		PeakRateWindows:  existing,
		PeakStart:        "08:00", PeakEnd: "10:00", PeakRateMultiplier: 2,
	}}
	svc := &adminServiceImpl{groupRepo: repo}

	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateWindows: []PeakRateWindow{
			{Start: "12:00", End: "14:00", Multiplier: 4},
		},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err)
	require.Equal(t, existing, updated.PeakRateWindows)
	require.Equal(t, "08:00", updated.PeakStart)
	require.Equal(t, 2.0, updated.PeakRateMultiplier)
}

func TestUpdateGroupDisabledEmptyArrayWithInvalidExistingDoesNot400(t *testing.T) {
	existing := []PeakRateWindow{
		{Start: "12:00", End: "14:00", Multiplier: 2, Days: []int{1}},
		{Start: "13:00", End: "15:00", Multiplier: 3, Days: []int{1}},
	}
	repo := &groupRepoStubForAdmin{getByID: &Group{
		ID: 1, Name: "disabled-invalid-existing", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakRateWindows:    existing,
		PeakStart:          "12:00",
		PeakEnd:            "14:00",
		PeakRateMultiplier: 2,
	}}
	svc := &adminServiceImpl{groupRepo: repo}

	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateWindows:    []PeakRateWindow{},
		PeakRateWindowsSet: true,
	})
	require.NoError(t, err, "停用状态提交空数组不得因历史非法窗口返回 400")
	require.Equal(t, existing, updated.PeakRateWindows, "停用时必须保留已有配置")
	require.Equal(t, "12:00", updated.PeakStart)
	require.Equal(t, "14:00", updated.PeakEnd)
	require.Equal(t, 2.0, updated.PeakRateMultiplier)
}

func TestUpdateGroupEnabledInvalidWindowsStillRejected(t *testing.T) {
	existing := []PeakRateWindow{{Start: "08:00", End: "10:00", Multiplier: 2}}
	repo := &groupRepoStubForAdmin{getByID: &Group{
		ID: 1, Name: "enabled-invalid-update", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakRateWindows:    existing,
		PeakStart:          "08:00",
		PeakEnd:            "10:00",
		PeakRateMultiplier: 2,
	}}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateWindows: []PeakRateWindow{
			{Start: "22:00", End: "02:00", Multiplier: 2},
		},
		PeakRateWindowsSet: true,
	})
	require.ErrorContains(t, err, "不支持跨天")
	require.Nil(t, repo.updated, "enabled=true 非法窗口不得落库")
}

func TestUpdateGroupDisableThenLegacyEnableKeepsPeakConfig(t *testing.T) {
	windows := []PeakRateWindow{
		{Start: "08:00", End: "10:00", Multiplier: 2},
		{Start: "18:00", End: "20:00", Multiplier: 3},
	}
	existing := &Group{
		ID: 1, Name: "disable-then-legacy-enable", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakRateWindows:    windows,
		PeakStart:          "22:00",
		PeakEnd:            "02:00",
		PeakRateMultiplier: 9,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	// 新前端停用 payload：提交空窗口并主动清空旧字段。
	disabled := false
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		PeakRateEnabled:    &disabled,
		PeakRateWindows:    []PeakRateWindow{},
		PeakRateWindowsSet: true,
		PeakStart:          ptrString(""),
		PeakEnd:            ptrString(""),
		PeakRateMultiplier: ptrFloatPeakWindows(1),
	})
	require.NoError(t, err)
	require.False(t, updated.PeakRateEnabled)
	require.Equal(t, windows, updated.PeakRateWindows, "停用时必须保留已有窗口")
	require.Equal(t, "08:00", updated.PeakStart, "停用时旧字段从第一个保留窗口回填")
	require.Equal(t, "10:00", updated.PeakEnd)
	require.Equal(t, 2.0, updated.PeakRateMultiplier)

	// 旧客户端只发 enabled=true，不传窗口和旧字段。
	enabled := true
	reenabled, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{PeakRateEnabled: &enabled})
	require.NoError(t, err, "旧客户端只带开关重新启用必须成功")
	require.True(t, reenabled.PeakRateEnabled)
	require.Equal(t, windows, reenabled.PeakRateWindows, "重新启用不得丢失窗口")
	require.Equal(t, "08:00", reenabled.PeakStart)
	require.Equal(t, "10:00", reenabled.PeakEnd)
	require.Equal(t, 2.0, reenabled.PeakRateMultiplier)
}

func TestUpdateGroupLegacyEnableBackfillsLegacyFromWindows(t *testing.T) {
	windows := []PeakRateWindow{
		{Start: "09:00", End: "11:00", Multiplier: 1.5, Days: []int{1, 2, 3, 4, 5}},
		{Start: "09:00", End: "11:00", Multiplier: 2, Days: []int{0, 6}},
	}
	existing := &Group{
		ID: 1, Name: "legacy-enable-stale-legacy", Platform: PlatformOpenAI, Status: StatusActive,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    false,
		PeakRateWindows:    windows,
		PeakStart:          "22:00",
		PeakEnd:            "02:00",
		PeakRateMultiplier: -3,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	enabled := true
	updated, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{PeakRateEnabled: &enabled})
	require.NoError(t, err, "旧字段非法时先用合法窗口回填，再校验")
	require.True(t, updated.PeakRateEnabled)
	require.Equal(t, windows, updated.PeakRateWindows)
	require.Equal(t, "09:00", updated.PeakStart)
	require.Equal(t, "11:00", updated.PeakEnd)
	require.Equal(t, 1.5, updated.PeakRateMultiplier)
}
