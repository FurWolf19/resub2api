package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type OpenAIMessagesDispatchModelConfig = domain.OpenAIMessagesDispatchModelConfig
type GroupCodexModelsManifestConfig = domain.GroupCodexModelsManifestConfig
type ReasoningEffortMapping = domain.ReasoningEffortMapping
type PeakRateWindow = domain.PeakRateWindow

type Group struct {
	ID             int64
	Name           string
	Description    string
	I18n           GroupI18n // 名称与描述的界面语言译文；缺失的语言沿用 Name/Description
	Platform       string
	RateMultiplier float64
	// 高峰时段倍率：peak_rate_enabled 为 true 且当前时刻命中某个高峰时段时，
	// token 计费倍率额外乘以该时段的 Multiplier。详见 PeakMultiplierAt。
	// PeakRateWindows 非空时优先生效；为空时回退旧单时段字段 PeakStart/PeakEnd/PeakRateMultiplier。
	// 旧字段在保存时始终同步第一个窗口，仅供旧客户端与降级读取。
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	PeakRateWindows    []PeakRateWindow
	IsExclusive        bool
	Status             string
	Hydrated           bool // indicates the group was loaded from a trusted repository source
	// DuplicateOperationID is internal persistence metadata used only to recover
	// an already committed one-click copy. It must never be mapped to API DTOs.
	DuplicateOperationID string

	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	FiveHourLimitUSD    *float64
	DefaultValidityDays int

	// 图片生成计费配置（antigravity 和 gemini 平台使用）
	AllowImageGeneration         bool
	AllowBatchImageGeneration    bool
	ImageCountBilling            bool
	ImageRateIndependent         bool
	ImageRateMultiplier          float64
	ImagePrice1K                 *float64
	ImagePrice2K                 *float64
	ImagePrice4K                 *float64
	BatchImageDiscountMultiplier float64
	BatchImageHoldMultiplier     float64
	VideoRateIndependent         bool
	VideoRateMultiplier          float64
	VideoPrice480P               *float64
	VideoPrice720P               *float64
	VideoPrice1080P              *float64
	// VideoModelPrices is optional per-model-family per-second pricing
	// (groups.video_model_prices JSONB). Shape: family → resolution → USD/s.
	// When set for a model, overrides VideoPrice* for that model only.
	VideoModelPrices map[string]map[string]float64
	// Codex alpha/search 网页搜索单次价格（USD/次，仅 openai 平台使用）；
	// nil 表示使用默认价 defaultWebSearchPricePerCall（官方 $10/1000 次）。
	WebSearchPricePerCall *float64

	// 搜索工具显式定价（per 1k calls）。
	SearchPricePer1k *float64
	// Grok Voice 显式定价（分组级，不按文本 RateMultiplier）。
	AudioRealtimePricePerMin     *float64
	AudioTTSPricePerMillionChars *float64
	AudioSTTPricePerHour         *float64

	// ModelPricing overrides channel and built-in prices for matching models.
	// Token intervals are selected only when LongContextPricingEnabled is true.
	LongContextPricingEnabled bool
	ModelPricing              []ChannelModelPricing

	// Claude Code 客户端限制
	ClaudeCodeOnly  bool
	FallbackGroupID *int64
	// 无效请求兜底分组（仅 anthropic 平台使用）
	FallbackGroupIDOnInvalidRequest *int64

	// 模型路由配置
	// key: 模型匹配模式（支持 * 通配符，如 "claude-opus-*"）
	// value: 优先账号 ID 列表
	ModelRouting        map[string][]int64
	ModelRoutingEnabled bool

	// MCP XML 协议注入开关（仅 antigravity 平台使用）
	MCPXMLInject bool

	// 支持的模型系列（仅 antigravity 平台使用）
	// 可选值: claude, gemini_text, gemini_image
	SupportedModelScopes []string

	// 分组排序
	SortOrder int

	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch       bool
	AllowLive                   bool
	ForceOpenAIFast             bool // 强制 OpenAI 网关请求使用 service_tier=priority
	ForceOpenAIUltrafast        bool // 对支持 Ultrafast 的模型强制 service_tier=ultrafast，优先于 ForceOpenAIFast
	FreeOpenAIFast              bool // OpenAI Fast 请求按 Standard 价格向用户计费
	RequireOAuthOnly            bool // 仅允许非 apikey 类型账号关联（OpenAI/Antigravity/Anthropic/Gemini）
	RequirePrivacySet           bool // 调度时仅允许 privacy 已成功设置的账号（OpenAI/Antigravity/Anthropic/Gemini）
	DefaultMappedModel          string
	MessagesDispatchModelConfig OpenAIMessagesDispatchModelConfig
	ModelAllowlist              GroupModelAllowlist
	// CodexModelsManifestConfig 开启后，普通模型列表与 Codex manifest 优先使用
	// 固定账号列表拉取并合并，不经过调度器（仅 openai 平台）。
	CodexModelsManifestConfig GroupCodexModelsManifestConfig

	// RPMLimit 分组级每分钟请求数上限（0 = 不限制）。
	// 一旦设置即接管该分组用户的限流（覆盖用户级 rpm_limit），可被 user-group rpm_override 进一步覆盖。
	RPMLimit int

	// MaxReasoningEffort limits the effective Anthropic/OpenAI reasoning effort.
	// Empty means unlimited; supported values are minimal/low/medium/high/xhigh/max.
	MaxReasoningEffort string
	// MaxReasoningEffortOverLimit is the access control when an explicit effort
	// exceeds the ceiling: downgrade (default) or deny.
	MaxReasoningEffortOverLimit string
	// ReasoningEffortMappings rewrites explicit request values before applying the ceiling.
	ReasoningEffortMappings []ReasoningEffortMapping

	// 分组利润控制（五个 token 计费平台可启用）。
	// 调度准入条件：账号倍率 U 满足 U <= D*(1-margin-buffer)，
	// D 为请求用户当刻有效下游倍率（用户覆盖 ?? 分组默认，再乘高峰因子）。
	// 只过滤候选账号，不改变既有排序/评分/粘性/熔断。
	ProfitControlEnabled bool
	ProfitMinMargin      float64 // 最低毛利率，小数存储（0.30=30%）
	ProfitSafetyBuffer   float64 // 安全缓冲，小数，与 margin 相加后从 D 中扣除

	CreatedAt time.Time
	UpdatedAt time.Time

	AccountGroups           []AccountGroup
	AccountCount            int64
	ActiveAccountCount      int64
	RateLimitedAccountCount int64
}

// IsGroupBindableInSimpleMode is the shared policy for groups that may be
// surfaced and bound to accounts while running in simple mode.
func IsGroupBindableInSimpleMode(group *Group) bool {
	return group != nil && group.Platform != PlatformComposite
}

func (g *Group) IsActive() bool {
	return g.Status == StatusActive
}

func (g *Group) IsSubscriptionType() bool {
	return g.SubscriptionType == SubscriptionTypeSubscription
}

func (g *Group) HasDailyLimit() bool {
	return g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
}

func (g *Group) HasWeeklyLimit() bool {
	return g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
}

func (g *Group) HasMonthlyLimit() bool {
	return g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
}

func (g *Group) HasFiveHourLimit() bool {
	return g.FiveHourLimitUSD != nil && *g.FiveHourLimitUSD > 0
}

// NormalizeFiveHourLimitUSD 把 5h 限额的 <=0 统一为 NULL（无限制）。
// 日/周/月限额仍使用 admin_group.go 的 normalizeLimit，0 表示零额度；
// 历史数据中的 0 不做迁移，读取与计费继续由 HasFiveHourLimit 的 >0 判断兼容。
func NormalizeFiveHourLimitUSD(limit *float64) *float64 {
	if limit == nil || *limit <= 0 {
		return nil
	}
	return limit
}

// GetImagePrice 根据 image_size 返回对应的图片生成价格
// 如果分组未配置价格，返回 nil（调用方应使用默认值）
func (g *Group) GetImagePrice(imageSize string) *float64 {
	switch imageSize {
	case "1K":
		return g.ImagePrice1K
	case "2K":
		return g.ImagePrice2K
	case "4K":
		return g.ImagePrice4K
	default:
		// 未知尺寸默认按 2K 计费
		return g.ImagePrice2K
	}
}

// GetVideoPrice 根据 resolution 返回对应的视频生成价格。
// 如果分组未配置价格，返回 nil（调用方应使用默认值）。
func (g *Group) GetVideoPrice(resolution string) *float64 {
	switch NormalizeVideoBillingResolutionOrDefault(resolution) {
	case VideoBillingResolution480P:
		return g.VideoPrice480P
	case VideoBillingResolution720P:
		return g.VideoPrice720P
	case VideoBillingResolution1080P:
		return g.VideoPrice1080P
	default:
		return g.VideoPrice480P
	}
}

// GetVideoPriceForModel prefers VideoModelPrices for the model family, then flat columns.
func (g *Group) GetVideoPriceForModel(model, resolution string) *float64 {
	if g == nil {
		return nil
	}
	if price := LookupVideoModelPrice(g.VideoModelPrices, model, resolution); price != nil {
		return price
	}
	return g.GetVideoPrice(resolution)
}

// VideoPriceConfig builds billing config including optional per-model map.
func (g *Group) VideoPriceConfig() *VideoPriceConfig {
	if g == nil {
		return nil
	}
	return &VideoPriceConfig{
		Price480P:   g.VideoPrice480P,
		Price720P:   g.VideoPrice720P,
		Price1080P:  g.VideoPrice1080P,
		ModelPrices: NormalizeVideoModelPrices(g.VideoModelPrices),
	}
}

// IsGroupContextValid reports whether a group from context has the fields required for routing decisions.
func IsGroupContextValid(group *Group) bool {
	if group == nil {
		return false
	}
	if group.ID <= 0 {
		return false
	}
	if !group.Hydrated {
		return false
	}
	if group.Platform == "" || group.Status == "" {
		return false
	}
	return true
}

// GetRoutingAccountIDs 根据请求模型获取路由账号 ID 列表
// 返回匹配的优先账号 ID 列表，如果没有匹配规则则返回 nil
func (g *Group) GetRoutingAccountIDs(requestedModel string) []int64 {
	if !g.ModelRoutingEnabled || len(g.ModelRouting) == 0 || requestedModel == "" {
		return nil
	}

	// 1. 精确匹配优先
	if accountIDs, ok := g.ModelRouting[requestedModel]; ok && len(accountIDs) > 0 {
		return accountIDs
	}

	// 2. 通配符匹配（前缀匹配）
	for pattern, accountIDs := range g.ModelRouting {
		if matchModelPattern(pattern, requestedModel) && len(accountIDs) > 0 {
			return accountIDs
		}
	}

	return nil
}

// matchModelPattern 检查模型是否匹配模式
// 支持 * 通配符，如 "claude-opus-*" 匹配 "claude-opus-4-20250514"
func matchModelPattern(pattern, model string) bool {
	if pattern == model {
		return true
	}

	// 处理 * 通配符（仅支持末尾通配符）
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(model, prefix)
	}

	return false
}

// parseMinutes 把 "HH:MM" 解析为当日分钟数（0..1439），格式非法返回 (0,false)。
// 手工解析而非 time.Parse：本函数位于每请求的计费热路径（PeakMultiplierAt），
// 避免对静态配置字符串重复走 layout 解析与 time.Time 分配。
// 接受集与 time.Parse("15:04", s) 完全一致（存量数据按旧解析写入，不得收窄）：
// 小时 1–2 位数字（0..23，允许不补零如 "1:30"），分钟固定 2 位数字（00..59）。
func parseMinutes(hhmm string) (int, bool) {
	colon := strings.IndexByte(hhmm, ':')
	if (colon != 1 && colon != 2) || len(hhmm)-colon-1 != 2 {
		return 0, false
	}
	return parseMinuteDigits(hhmm, colon)
}

// parsePeakRateWindowMinutes 严格解析新数组（peak_rate_windows）的 "HH:MM"：
// 小时与分钟都必须两位，小时 00..23、分钟 00..59。
// 旧字段 peak_start/peak_end 仍使用宽松的 parseMinutes，兼容存量数据（例如 "9:00"）。
func parsePeakRateWindowMinutes(hhmm string) (int, bool) {
	if len(hhmm) != 5 || hhmm[2] != ':' {
		return 0, false
	}
	return parseMinuteDigits(hhmm, 2)
}

// parseLegacyPeakMinutes 解析存量数据的宽松时间格式："H:M"、"H:MM"、"HH:M"、
// "HH:MM" 均可（小时 0..23、分钟 0..59）。仅用于旧字段与数据库存量窗口的归一化，
// 新 API 的 peak_rate_windows 仍由 parsePeakRateWindowMinutes 严格校验。
func parseLegacyPeakMinutes(hhmm string) (int, bool) {
	s := strings.TrimSpace(hhmm)
	colon := strings.IndexByte(s, ':')
	if colon < 1 || colon > 2 {
		return 0, false
	}
	minutePart := s[colon+1:]
	if len(minutePart) < 1 || len(minutePart) > 2 {
		return 0, false
	}
	h := 0
	for i := 0; i < colon; i++ {
		d := s[i] - '0'
		if d > 9 {
			return 0, false
		}
		h = h*10 + int(d)
	}
	m := 0
	for i := 0; i < len(minutePart); i++ {
		d := minutePart[i] - '0'
		if d > 9 {
			return 0, false
		}
		m = m*10 + int(d)
	}
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// formatPeakMinutes 把当日分钟数格式化为严格的两位 "HH:MM"。
func formatPeakMinutes(total int) string {
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// parseMinuteDigits 解析 hhmm 中位于 colon 之前的小时与之后固定的两位分钟。
func parseMinuteDigits(hhmm string, colon int) (int, bool) {
	h := 0
	for i := 0; i < colon; i++ {
		d := hhmm[i] - '0'
		if d > 9 {
			return 0, false
		}
		h = h*10 + int(d)
	}
	m1, m2 := hhmm[colon+1]-'0', hhmm[colon+2]-'0'
	if m1 > 9 || m2 > 9 {
		return 0, false
	}
	m := int(m1)*10 + int(m2)
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// effectivePeakRateWindows 返回实际生效的高峰时段列表。
// 新数组非空时以数组为准；数组为空时回退旧单时段字段（等价于一个 days=all 的窗口）。
// 旧字段非法或数组为空且旧字段未配置时返回 nil，调用方按"无高峰"处理。
func (g *Group) effectivePeakRateWindows() []PeakRateWindow {
	if g == nil {
		return nil
	}
	if len(g.PeakRateWindows) > 0 {
		return g.PeakRateWindows
	}
	return peakRateWindowsFromLegacyConfig(g.PeakStart, g.PeakEnd, g.PeakRateMultiplier)
}

// peakRateWindowsFromLegacyConfig 把旧单时段字段归一化为一个 days=all 的窗口。
// 旧字段使用宽松解析（例如 "9:00"），生成的新数组统一格式化为 strict "HH:MM"，
// 避免旧 API 写入的窗口在后续新校验中被误判为非法。
// 字段缺失或非法（格式错误、start>=end、负倍率）时返回 nil。
func peakRateWindowsFromLegacyConfig(start, end string, multiplier float64) []PeakRateWindow {
	st, okStart := parseLegacyPeakMinutes(start)
	en, okEnd := parseLegacyPeakMinutes(end)
	if !okStart || !okEnd || st >= en || multiplier < 0 {
		return nil
	}
	return []PeakRateWindow{{Start: formatPeakMinutes(st), End: formatPeakMinutes(en), Multiplier: multiplier}}
}

// PeakMultiplierAt 返回指定时刻 now 的高峰因子。
//   - 未启用 / 非订阅分组 / 未配置 / 配置非法 / 非高峰时段 → 返回 1.0（安全降级）
//   - 每个窗口区间为左闭右开 [Start, End)，仅支持当日区间，不支持跨天（如 22:00-次日02:00）
//   - Days 为 0=周日 … 6=周六；空数组表示每天
//   - 时刻基于全局系统时区（timezone.Location）判定
//
// 校验保证窗口互不重叠，因此命中第一个窗口即返回，不做叠乘。
// 该方法是纯函数，不读取任何外部状态，便于单测。
func (g *Group) PeakMultiplierAt(now time.Time) float64 {
	if g == nil || !g.IsSubscriptionType() || !g.PeakRateEnabled {
		return 1.0
	}
	windows := g.effectivePeakRateWindows()
	if len(windows) == 0 {
		return 1.0
	}
	t := now.In(timezone.Location())
	cur := t.Hour()*60 + t.Minute()
	weekday := int(t.Weekday())
	for _, w := range windows {
		if !peakRateWindowDayMatches(w.Days, weekday) {
			continue
		}
		start, okStart := parseMinutes(w.Start)
		end, okEnd := parseMinutes(w.End)
		if !okStart || !okEnd || start >= end || w.Multiplier < 0 {
			continue
		}
		if cur >= start && cur < end {
			return w.Multiplier
		}
	}
	return 1.0
}

// peakRateWindowDayMatches 判断某个窗口是否在给定星期生效；空 Days 表示每天。
func peakRateWindowDayMatches(days []int, weekday int) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == weekday {
			return true
		}
	}
	return false
}

// normalizePeakRateDays 去重并升序排序，保持空数组语义（每天）。
func normalizePeakRateDays(days []int) []int {
	if len(days) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(days))
	out := make([]int, 0, len(days))
	for _, d := range days {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Ints(out)
	return out
}

var peakRateWeekdayNames = [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// ValidatePeakRateWindows 校验多时段高峰配置。
//   - start/end 必须是合法 HH:MM，且 start < end（不支持跨天）；
//   - multiplier 必须为非负有限数；
//   - days 仅允许 0..6；
//   - 同一星期几上的区间不得重叠（相邻边界允许），不同星期几允许同时段不同倍率。
//
// 错误信息包含冲突窗口序号与星期，便于前端直接展示。
func ValidatePeakRateWindows(windows []PeakRateWindow) error {
	type expandedSpan struct {
		start   int
		end     int
		window  int
		weekday int
	}
	spans := make([]expandedSpan, 0, len(windows)*7)
	for i, w := range windows {
		start, ok := parsePeakRateWindowMinutes(strings.TrimSpace(w.Start))
		if !ok {
			return fmt.Errorf("第 %d 个高峰时段的开始时间 %q 格式应为 HH:MM", i+1, w.Start)
		}
		end, ok := parsePeakRateWindowMinutes(strings.TrimSpace(w.End))
		if !ok {
			return fmt.Errorf("第 %d 个高峰时段的结束时间 %q 格式应为 HH:MM", i+1, w.End)
		}
		if start >= end {
			return fmt.Errorf("第 %d 个高峰时段 %s-%s 的结束时间必须晚于开始时间（不支持跨天区间，如 22:00-次日02:00）", i+1, w.Start, w.End)
		}
		if math.IsNaN(w.Multiplier) || math.IsInf(w.Multiplier, 0) || w.Multiplier < 0 {
			return fmt.Errorf("第 %d 个高峰时段的倍率必须为非负数，got %v", i+1, w.Multiplier)
		}
		days := normalizePeakRateDays(w.Days)
		if len(days) == 0 {
			days = []int{0, 1, 2, 3, 4, 5, 6}
		}
		for _, d := range days {
			if d < 0 || d > 6 {
				return fmt.Errorf("第 %d 个高峰时段的星期取值必须为 0..6（0=周日），got %d", i+1, d)
			}
			spans = append(spans, expandedSpan{
				start:   d*1440 + start,
				end:     d*1440 + end,
				window:  i,
				weekday: d,
			})
		}
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})
	maxEndIdx := -1
	for i := range spans {
		if maxEndIdx >= 0 && spans[i].start < spans[maxEndIdx].end {
			prev := spans[maxEndIdx]
			cur := spans[i]
			return fmt.Errorf(
				"高峰时段重叠：第 %d 个时段（%s %s-%s）与第 %d 个时段（%s %s-%s）在%s重叠",
				prev.window+1, peakRateWeekdayNames[prev.weekday], windows[prev.window].Start, windows[prev.window].End,
				cur.window+1, peakRateWeekdayNames[cur.weekday], windows[cur.window].Start, windows[cur.window].End,
				peakRateWeekdayNames[cur.weekday],
			)
		}
		if maxEndIdx < 0 || spans[i].end > spans[maxEndIdx].end {
			maxEndIdx = i
		}
	}
	return nil
}

// NormalizePeakRateWindows 归一化最终落库的多时段高峰配置：
//   - 仅非订阅分组清空（返回 nil）；订阅分组即使暂时关闭高峰也保留窗口配置，
//     便于重新开启时恢复，运行期由 PeakRateEnabled 控制是否生效；
//   - 去重并升序 days、trim start/end；保持窗口原始顺序（不按时间重排，避免改变前端顺序）。
//
// 调用顺序为先归一化、后校验；enabled=true 时校验保证各字段合法。
func NormalizePeakRateWindows(subscriptionType string, _ bool, windows []PeakRateWindow) []PeakRateWindow {
	if subscriptionType != SubscriptionTypeSubscription {
		return nil
	}
	out := make([]PeakRateWindow, 0, len(windows))
	for _, w := range windows {
		out = append(out, PeakRateWindow{
			Start:      strings.TrimSpace(w.Start),
			End:        strings.TrimSpace(w.End),
			Multiplier: w.Multiplier,
			Days:       normalizePeakRateDays(w.Days),
		})
	}
	return out
}

// NormalizeStoredPeakRateWindows 归一化从数据库读出的存量窗口时间：
// 把旧版本可能写入的宽松格式（"9:00"、"9:5" 等）格式化为严格 "HH:MM"，
// 让后续 ValidatePeakRateWindows 不再误伤存量数据。非法时间（"25:00"、
// "09:60"）保持原样，交由校验拒绝。
//
// 注意：新 API 显式提交的 windows 不走本函数，仍按严格格式校验。
func NormalizeStoredPeakRateWindows(windows []PeakRateWindow) []PeakRateWindow {
	if len(windows) == 0 {
		return windows
	}
	out := make([]PeakRateWindow, len(windows))
	for i, w := range windows {
		out[i] = w
		if total, ok := parseLegacyPeakMinutes(w.Start); ok {
			out[i].Start = formatPeakMinutes(total)
		}
		if total, ok := parseLegacyPeakMinutes(w.End); ok {
			out[i].End = formatPeakMinutes(total)
		}
	}
	return out
}

// ValidatePeakRateConfig 是高峰倍率配置的唯一校验来源，供 handler 与 service 层共用。
// enabled=true 时仅允许订阅类型分组；并要求 start/end 合法且 end>start（不支持跨天），multiplier>=0。
// multiplier=0 是允许的，表示高峰 token 请求按 0 倍计费，可用于折扣/免费策略。
// enabled=false 时放行（不关心类型）。subscriptionType 为空按 standard 处理。
func ValidatePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) error {
	if !enabled {
		return nil
	}
	if subscriptionType != SubscriptionTypeSubscription {
		return errors.New("高峰时段倍率仅支持订阅类型分组")
	}
	if start == "" || end == "" {
		return errors.New("peak_rate_enabled 为 true 时 peak_start 与 peak_end 必填")
	}
	st, okStart := parseMinutes(start)
	if !okStart {
		return fmt.Errorf("peak_start 格式应为 HH:MM，got %q", start)
	}
	en, okEnd := parseMinutes(end)
	if !okEnd {
		return fmt.Errorf("peak_end 格式应为 HH:MM，got %q", end)
	}
	if st >= en {
		return errors.New("peak_end 必须大于 peak_start（不支持跨天区间，如 22:00-02:00）")
	}
	if multiplier < 0 {
		return errors.New("peak_rate_multiplier 不能为负")
	}
	return nil
}

// NormalizePeakRateConfig 归一化最终落库的高峰配置，CreateGroup 与 UpdateGroup 两条写路径共用（唯一收口）：
//   - 非订阅类型分组不携带任何高峰配置，一律清空（enabled=false、窗口置空、倍率归 1.0）；
//   - 订阅分组关闭高峰时保留已配置的合法窗口（便于临时停用后再启用），
//     但清掉无法解析的脏字符串与负倍率，避免脏数据入库。
//
// 与 ValidatePeakRateConfig 的分工：enabled=true 时校验已保证各字段合法，本函数为无操作；
// enabled=false 时校验放行，由本函数兜底清洗。调用顺序为先归一化、后校验，
// 使"订阅转标准"这类更新能静默清空高峰配置而不是被校验拒绝。
func NormalizePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) (bool, string, string, float64) {
	if subscriptionType != SubscriptionTypeSubscription {
		return false, "", "", 1.0
	}
	if !enabled {
		if _, ok := parseMinutes(start); !ok {
			start = ""
		}
		if _, ok := parseMinutes(end); !ok {
			end = ""
		}
		if multiplier < 0 {
			multiplier = 1.0
		}
	}
	return enabled, start, end, multiplier
}

// computePeakAwareMultipliers 把"基础 token 倍率 base"（已含系统/分组/用户级倍率，但不含高峰）
// 拆分为最终 token 倍率与图片按次倍率：图片按次倍率基于 base 现算、不受高峰影响；token 倍率在 base 上叠加高峰因子。
// gateway_service.recordUsageCore 与 openai_gateway_service.RecordUsage 共用此函数，
// 锁死"高峰因子只乘入 token 倍率、图片按次倍率不受影响"这一叠加顺序——任何调换都会被 group_peak_rate_test 覆盖。
func computePeakAwareMultipliers(apiKey *APIKey, base float64, now time.Time) (text, image float64) {
	image = resolveImageRateMultiplier(apiKey, base)
	peak := 1.0
	if apiKey != nil && apiKey.Group != nil {
		peak = apiKey.Group.PeakMultiplierAt(now)
	}
	text = base * peak
	return
}

// validProfitControlRatio 判定 margin/buffer 是否为可落库的合法小数：[0,1) 且非 NaN/Inf。
func validProfitControlRatio(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v < 1
}

// NormalizeGroupPlatform 把创建分组时省略的 platform 归一化为默认平台。
// handler 的入参预校验必须与 CreateGroup 落库时用同一个归一化结果，否则
// 「省略 platform + 启用利润控制」会被 handler 以「平台不支持」400 掉，
// 而该分组本会被建成受支持的 anthropic 分组。
func NormalizeGroupPlatform(platform string) string {
	if platform == "" {
		return PlatformAnthropic
	}
	return platform
}

// ValidateProfitControlConfig 是分组利润控制配置的唯一校验来源，handler 与 service 层共用。
// enabled=true 时仅允许五个可计费平台分组；margin/buffer 各自 ∈ [0,1)，且 margin+buffer < 1
// （相加 >=1 时阈值 <=0，所有可核价账号都会被排除，视为配置错误而不是静默全黑）。
// enabled=false 时放行（不关心平台），由 Normalize 兜底清洗数值。
func ValidateProfitControlConfig(platform string, enabled bool, minMargin, safetyBuffer float64) error {
	if !enabled {
		return nil
	}
	if !profitControlPlatformSupported(platform) {
		return errors.New("利润控制仅支持 openai、anthropic、gemini、grok、antigravity 平台分组")
	}
	if !validProfitControlRatio(minMargin) {
		return fmt.Errorf("profit_min_margin 应为 [0,1) 的小数，got %v", minMargin)
	}
	if !validProfitControlRatio(safetyBuffer) {
		return fmt.Errorf("profit_safety_buffer 应为 [0,1) 的小数，got %v", safetyBuffer)
	}
	if minMargin+safetyBuffer >= 1 {
		return errors.New("profit_min_margin 与 profit_safety_buffer 之和必须小于 1，否则将排除全部账号")
	}
	return nil
}

// NormalizeProfitControlConfig 归一化最终落库的利润控制配置，CreateGroup 与 UpdateGroup 共用（唯一收口）：
//   - 非五个平台分组不携带利润控制，一律重置为默认（关、0、0）；
//   - 支持平台关闭开关时保留合法数值（便于再次启用），清洗 NaN/Inf/越界脏值。
//
// 与 ValidateProfitControlConfig 的分工同高峰倍率：先归一化、后校验，
// 使"openai 转其他平台"这类更新能静默清空利润配置而不是被校验拒绝。
func NormalizeProfitControlConfig(platform string, enabled bool, minMargin, safetyBuffer float64) (bool, float64, float64) {
	if !profitControlPlatformSupported(platform) {
		return false, 0, 0
	}
	if !enabled {
		if !validProfitControlRatio(minMargin) {
			minMargin = 0
		}
		if !validProfitControlRatio(safetyBuffer) {
			safetyBuffer = 0
		}
	}
	return enabled, minMargin, safetyBuffer
}

func profitControlPlatformSupported(platform string) bool {
	switch platform {
	case PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformGrok, PlatformAntigravity:
		return true
	default:
		return false
	}
}

// GetSearchPricePer1k returns explicit search/tool price per 1k calls if configured.
func (g *Group) GetSearchPricePer1k() *float64 {
	if g == nil {
		return nil
	}
	return g.SearchPricePer1k
}
