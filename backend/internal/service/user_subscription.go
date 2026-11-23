package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	subscriptionDayDuration    = 24 * time.Hour
	subscriptionFiveHourWindow = 5 * time.Hour
)

type UserSubscription struct {
	ID      int64
	UserID  int64
	GroupID int64

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string

	DailyWindowStart    *time.Time
	WeeklyWindowStart   *time.Time
	MonthlyWindowStart  *time.Time
	FiveHourWindowStart *time.Time

	DailyUsageUSD    float64
	WeeklyUsageUSD   float64
	MonthlyUsageUSD  float64
	FiveHourUsageUSD float64

	// QuotaMultiplier 是订阅额度倍率：有效限额 = 分组限额 × QuotaMultiplier。
	// 0 是合法值（额度为 0）；空字段仅可能出现在迁移前数据，读库默认 1.0。
	QuotaMultiplier float64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	User           *User
	Group          *Group
	AssignedByUser *User
}

func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	return s.daysRemainingAt(time.Now())
}

func (s *UserSubscription) daysRemainingAt(now time.Time) int {
	remaining := s.ExpiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}

	days := int(remaining / subscriptionDayDuration)
	if remaining%subscriptionDayDuration != 0 {
		days++
	}
	return days
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) HasOneTimeDailyQuota() bool {
	if s == nil || s.StartsAt.IsZero() || s.ExpiresAt.IsZero() {
		return false
	}
	return !s.ExpiresAt.After(s.StartsAt.AddDate(0, 0, 1))
}

func (s *UserSubscription) NeedsDailyReset() bool {
	return s.NeedsDailyResetAt(time.Now())
}

func (s *UserSubscription) NeedsDailyResetAt(now time.Time) bool {
	_, ok := s.automaticDailyWindowStartAt(now)
	return ok
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	return s.NeedsWeeklyResetAt(time.Now())
}

func (s *UserSubscription) NeedsWeeklyResetAt(now time.Time) bool {
	if s.WeeklyWindowStart == nil {
		return false
	}
	return !now.Before(s.WeeklyWindowStart.Add(7 * 24 * time.Hour))
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	return s.NeedsMonthlyResetAt(time.Now())
}

func (s *UserSubscription) NeedsMonthlyResetAt(now time.Time) bool {
	if s.MonthlyWindowStart == nil {
		return false
	}
	return !now.Before(s.MonthlyWindowStart.Add(30 * 24 * time.Hour))
}

// FiveHourWindowExpiredAt 判断固定 5 小时窗口是否已经到期。
// 5h 窗口不会自动重新开始：到期后用量归零，但 window_start 置空，
// 必须等下一笔 cost>0 的消费才开启新窗口。
func (s *UserSubscription) FiveHourWindowExpiredAt(now time.Time) bool {
	if s == nil || s.FiveHourWindowStart == nil {
		return false
	}
	return !now.Before(s.FiveHourWindowStart.Add(subscriptionFiveHourWindow))
}

// FiveHourResetTime 返回当前 5h 窗口的结束时间；窗口未开启时返回 nil。
func (s *UserSubscription) FiveHourResetTime() *time.Time {
	if s == nil || s.FiveHourWindowStart == nil {
		return nil
	}
	t := s.FiveHourWindowStart.Add(subscriptionFiveHourWindow)
	return &t
}

// QuotaMultiplierOrDefault 返回订阅额度倍率。0 是合法倍率，不能被当作缺省值；
// 只有字段完全未加载（nil 接收者）时才回退到 1.0。
func (s *UserSubscription) QuotaMultiplierOrDefault() float64 {
	if s == nil {
		return 1
	}
	return s.QuotaMultiplier
}

// EffectiveLimit 计算分组限额乘以订阅额度倍率后的有效限额。
// 分组未配置该窗口限额时返回 0（调用方应先判断 HasXxxLimit，保持无限语义）。
func (s *UserSubscription) EffectiveLimit(groupLimit *float64) float64 {
	if groupLimit == nil {
		return 0
	}
	return *groupLimit * s.QuotaMultiplierOrDefault()
}

func (s *UserSubscription) canAutomaticallyResetDailyAt(now time.Time) bool {
	_, ok := s.automaticDailyWindowStartAt(now)
	return ok
}

// automaticDailyWindowStartAt 计算日窗口按“配置时区日历日”对齐后的当前窗口起点。
// 日额度固定在每天 0 点刷新（与周/月的期限对齐滚动窗口语义不同），因此只要持久化
// 的窗口起点落在更早的日历日，就允许推进到今天 0 点。手动重置、激活等写入的任何
// 非 0 点锚点都会在下一个 0 点被拉回日历日边界，不会永久漂移刷新时刻。
func (s *UserSubscription) automaticDailyWindowStartAt(now time.Time) (time.Time, bool) {
	if s.DailyWindowStart == nil {
		return time.Time{}, false
	}
	if s.HasOneTimeDailyQuota() {
		return time.Time{}, false
	}
	today := timezone.StartOfDay(now)
	if !today.After(timezone.StartOfDay(*s.DailyWindowStart)) {
		return time.Time{}, false
	}
	return today, true
}

func (s *UserSubscription) canAutomaticallyResetWeeklyAt(now time.Time) bool {
	_, ok := s.automaticWindowStartAt(s.WeeklyWindowStart, 7*24*time.Hour, now)
	return ok
}

func (s *UserSubscription) canAutomaticallyResetMonthlyAt(now time.Time) bool {
	_, ok := s.automaticWindowStartAt(s.MonthlyWindowStart, 30*24*time.Hour, now)
	return ok
}

// windowResetAnchor 返回周/月窗口实际推进所依据的锚点。
// 早期订阅把首个窗口初始化在开通日零点；只有这个初始值是无歧义的，之后出现的
// 零点锚点可能来自手动重置，必须保持权威。
// 自动推进（automaticWindowStartAt）与对外展示的重置时间（WeeklyResetTime/
// MonthlyResetTime）必须共用这一修正，否则仪表盘显示的重置时间会早于窗口实际
// 滚动的时间。
// 日窗口按日历日对齐（automaticDailyWindowStartAt），不走这里。
func (s *UserSubscription) windowResetAnchor(previous time.Time) time.Time {
	legacyAnchor := startOfDay(s.StartsAt)
	if legacyAnchor.Before(s.StartsAt) && previous.Equal(legacyAnchor) {
		return s.StartsAt
	}
	return previous
}

// automaticWindowStartAt 计算周/月窗口（期限对齐滚动窗口）的当前窗口起点。
// 窗口从锚点按整数个 period 步进，且不越过订阅到期时间，避免最后一个不完整
// 周期重复发放额度（issue #5051）。日窗口不走此函数，见 automaticDailyWindowStartAt。
func (s *UserSubscription) automaticWindowStartAt(previous *time.Time, period time.Duration, now time.Time) (time.Time, bool) {
	if previous == nil {
		return time.Time{}, false
	}

	anchor := s.windowResetAnchor(*previous)
	next := anchor.Add(period)
	if now.Before(next) || !next.Before(s.ExpiresAt) {
		return time.Time{}, false
	}

	periods := now.Sub(anchor) / period
	lastPeriodBeforeExpiry := (s.ExpiresAt.Sub(anchor) - 1) / period
	if periods > lastPeriodBeforeExpiry {
		periods = lastPeriodBeforeExpiry
	}
	return anchor.Add(periods * period), true
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	if s.DailyWindowStart == nil {
		return nil
	}
	if s.HasOneTimeDailyQuota() {
		t := s.ExpiresAt
		return &t
	}
	// 日窗口按日历日对齐：下次刷新固定在窗口起点所在日的次日 0 点。
	t := timezone.StartOfDay(*s.DailyWindowStart).AddDate(0, 0, 1)
	return &t
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	if s.WeeklyWindowStart == nil {
		return nil
	}
	t := s.windowResetAnchor(*s.WeeklyWindowStart).Add(7 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	if s.MonthlyWindowStart == nil {
		return nil
	}
	t := s.windowResetAnchor(*s.MonthlyWindowStart).Add(30 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	if !group.HasDailyLimit() {
		return true
	}
	return s.DailyUsageUSD+additionalCost <= s.EffectiveLimit(group.DailyLimitUSD)
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if !group.HasWeeklyLimit() {
		return true
	}
	return s.WeeklyUsageUSD+additionalCost <= s.EffectiveLimit(group.WeeklyLimitUSD)
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if !group.HasMonthlyLimit() {
		return true
	}
	return s.MonthlyUsageUSD+additionalCost <= s.EffectiveLimit(group.MonthlyLimitUSD)
}

func (s *UserSubscription) CheckFiveHourLimit(group *Group, additionalCost float64) bool {
	if !group.HasFiveHourLimit() {
		return true
	}
	return s.FiveHourUsageUSD+additionalCost <= s.EffectiveLimit(group.FiveHourLimitUSD)
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}
