// Package openai_compat 提供 OpenAI 协议族在不同上游间的能力差异判定工具。
//
// 背景：sub2api 的 OpenAI APIKey 账号通过 base_url 接入多种第三方 OpenAI 兼容上游
// （DeepSeek、Kimi、GLM、Qwen 等）。这些上游普遍只支持 /v1/chat/completions，
// 不存在 /v1/responses 端点。但网关历史代码无差别走 CC→Responses 转换并打到
// /v1/responses，导致兼容上游 404。
//
// 本包提供基于"账号探测标记"的能力判定，配合
// internal/service/openai_apikey_responses_probe.go 在创建/修改账号时一次性
// 探测并落标。
//
// 设计取舍：
//   - 不维护静态 host 白名单——避免新增厂商时必须改代码（讨论沉淀于
//     pensieve/short-term/knowledge/upstream-capability-detection-design-tradeoffs）
//   - 标记缺失时默认 true（即"走 Responses"），保持与重构前老代码完全一致的存量
//     账号行为（"现状即证据"原则；详见
//     pensieve/short-term/maxims/preserve-existing-runtime-behavior-when-replacing-logic-in-stateful-systems）
package openai_compat

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// AccountResponsesSupport 描述账号上游对 OpenAI Responses API 的有效支持状态。
//
// 仅用于 platform=openai + type=apikey 的账号；其他账号类型不应调用本包判定。
type AccountResponsesSupport int

const (
	// ResponsesSupportUnknown 表示账号尚未完成能力探测（extra 字段缺失）。
	// 上游路由层应按"现状即证据"原则默认走 Responses，保持与重构前一致。
	ResponsesSupportUnknown AccountResponsesSupport = iota

	// ResponsesSupportYes 探测确认上游支持 /v1/responses。
	ResponsesSupportYes

	// ResponsesSupportNo 探测确认上游不支持 /v1/responses，应走
	// /v1/chat/completions 直转路径。
	ResponsesSupportNo
)

// ResponsesSupportMode 描述账号级 Responses API 路由覆盖模式。
type ResponsesSupportMode string

const (
	// ResponsesSupportModeAuto 表示跟随自动探测结果。
	ResponsesSupportModeAuto ResponsesSupportMode = "auto"

	// ResponsesSupportModeForceResponses 强制使用 /v1/responses。
	ResponsesSupportModeForceResponses ResponsesSupportMode = "force_responses"

	// ResponsesSupportModeForceChatCompletions 强制使用 /v1/chat/completions。
	ResponsesSupportModeForceChatCompletions ResponsesSupportMode = "force_chat_completions"
)

// ExtraKeyResponsesMode 是 accounts.extra JSON 中存储手动覆盖模式的键名。
// 值类型为 string：auto=跟随探测，force_responses=强制 Responses，
// force_chat_completions=强制 Chat Completions。
const ExtraKeyResponsesMode = "openai_responses_mode"

// ExtraKeyResponsesSupported 是 accounts.extra JSON 中存储自动探测结果的键名。
// 值类型为 bool：true=支持、false=不支持、键缺失=未探测。
const ExtraKeyResponsesSupported = "openai_responses_supported"

// NormalizeResponsesSupportMode 归一化账号级 Responses API 路由覆盖模式。
// 缺失或非法值按 auto 处理，以保持存量行为。
func NormalizeResponsesSupportMode(mode string) ResponsesSupportMode {
	switch ResponsesSupportMode(mode) {
	case ResponsesSupportModeForceResponses:
		return ResponsesSupportModeForceResponses
	case ResponsesSupportModeForceChatCompletions:
		return ResponsesSupportModeForceChatCompletions
	default:
		return ResponsesSupportModeAuto
	}
}

// ResolveResponsesSupport 从账号的 extra map 中读取手动覆盖模式与探测标记。
//
// 标记缺失或类型不匹配时返回 ResponsesSupportUnknown——调用方应按
// "未探测=保留旧行为=走 Responses" 处理（参见 ShouldUseResponsesAPI）。
func ResolveResponsesSupport(extra map[string]any) AccountResponsesSupport {
	if extra == nil {
		return ResponsesSupportUnknown
	}
	if mode, ok := extra[ExtraKeyResponsesMode].(string); ok {
		switch NormalizeResponsesSupportMode(mode) {
		case ResponsesSupportModeForceResponses:
			return ResponsesSupportYes
		case ResponsesSupportModeForceChatCompletions:
			return ResponsesSupportNo
		}
	}
	v, ok := extra[ExtraKeyResponsesSupported]
	if !ok {
		return ResponsesSupportUnknown
	}
	supported, ok := v.(bool)
	if !ok {
		return ResponsesSupportUnknown
	}
	if supported {
		return ResponsesSupportYes
	}
	return ResponsesSupportNo
}

// ShouldUseResponsesAPI 判断 OpenAI APIKey 账号的入站 /v1/chat/completions 请求
// 是否应走"CC→Responses 转换 + 上游 /v1/responses"路径。
//
// 返回 true 的两种情况：
//  1. 账号已探测确认支持 Responses
//  2. 账号未探测（标记缺失）——按"现状即证据"原则保留旧行为
//
// 仅当账号已探测且确认不支持时返回 false，此时调用方应走 CC 直转路径
// （详见 internal/service/openai_gateway_chat_completions_raw.go）。
func ShouldUseResponsesAPI(extra map[string]any) bool {
	return ResolveResponsesSupport(extra) != ResponsesSupportNo
}

// ---------------------------------------------------------------------------
// Dual-protocol capability (auto mode)
// ---------------------------------------------------------------------------

// Protocol identifiers used by OpenAI protocol capability probing.
const (
	ProtocolResponses       = "responses"
	ProtocolChatCompletions = "chat_completions"
)

// ExtraKeyProtocolCapabilities 是 accounts.extra 中双协议能力探测结果的键名。
const ExtraKeyProtocolCapabilities = "openai_protocol_capabilities"

// ProtocolCapabilityTTL 是探测结果的有效期。过期后先用旧值转发，再后台异步重探。
const ProtocolCapabilityTTL = 30 * time.Minute

// ProtocolCapability 描述单个上游协议的探测结论。
type ProtocolCapability struct {
	Known     bool
	Supported bool
	ProbedAt  time.Time
}

// Fresh 报告该能力结论是否仍在 TTL 内。旧键迁移来的结论没有 probed_at，
// 视为已过期（会触发一次后台重探，但不影响本次转发）。
func (c ProtocolCapability) Fresh(now time.Time) bool {
	if !c.Known || c.ProbedAt.IsZero() {
		return false
	}
	age := now.Sub(c.ProbedAt)
	return age >= 0 && age < ProtocolCapabilityTTL
}

// ProtocolCapabilities 描述 OpenAI APIKey auto 模式下的双协议能力。
type ProtocolCapabilities struct {
	Responses       ProtocolCapability
	ChatCompletions ProtocolCapability
}

// HasKnown 报告是否至少有一个协议已有结论。
func (c ProtocolCapabilities) HasKnown() bool {
	return c.Responses.Known || c.ChatCompletions.Known
}

// NeedsRefresh 报告是否应重新探测：任一协议未知或结论已过期。
//
// 旧键迁移来的完整快照（两个协议都已知但缺少 probed_at）视为“刚探测过”：
// 网关会在首次请求时补写 probed_at，之后按 TTL 低频重探。若这里仍返回
// true，活跃账号会在每个请求上触发后台双探测。
func (c ProtocolCapabilities) NeedsRefresh(now time.Time) bool {
	if c.Responses.Known && c.ChatCompletions.Known &&
		(c.Responses.ProbedAt.IsZero() || c.ChatCompletions.ProbedAt.IsZero()) {
		return false
	}
	return !c.Responses.Fresh(now) || !c.ChatCompletions.Fresh(now)
}

// Lookup 返回指定协议的能力结论。
func (c ProtocolCapabilities) Lookup(protocol string) (known, supported bool) {
	switch normalizeProtocolName(protocol) {
	case ProtocolResponses:
		return c.Responses.Known, c.Responses.Supported
	case ProtocolChatCompletions:
		return c.ChatCompletions.Known, c.ChatCompletions.Supported
	default:
		return false, false
	}
}

// Supports 报告已知且支持指定协议。
func (c ProtocolCapabilities) Supports(protocol string) bool {
	known, supported := c.Lookup(protocol)
	return known && supported
}

// ChooseProtocol 在 auto 模式下选择本次请求实际使用的上游协议。
//
//  1. 入站协议已知支持 → 跟随入站（少一次转换）。
//  2. 入站协议未知 → 跟随入站（即使另一种已知支持），由真实请求与
//     404/405 回退决定。
//  3. 入站协议明确不支持、另一种明确支持 → 用另一种。
//  4. 入站协议明确不支持、另一种未知 → 用另一种（真实请求会给出答案）。
//  5. 两种都明确不支持 → 跟随入站，让真实请求产生明确错误。
//
// 关键约束：未知不能被另一种协议的已知支持抢走。Codex 等客户端只认
// Responses 协议，若把未知的 Responses 入站静默改成 Chat Completions，
// 会在真正的能力差异出现时产生难以察觉的降级。
func (c ProtocolCapabilities) ChooseProtocol(inbound string) string {
	inbound = normalizeProtocolName(inbound)
	other := otherProtocol(inbound)
	if other == "" {
		return inbound
	}
	inKnown, inSupported := c.Lookup(inbound)
	if !inKnown || inSupported {
		return inbound
	}
	otherKnown, otherSupported := c.Lookup(other)
	if otherKnown && !otherSupported {
		return inbound
	}
	return other
}

// ToExtra 返回可写入 accounts.extra 的 JSON 结构。
func (c ProtocolCapabilities) ToExtra() map[string]any {
	out := map[string]any{}
	if c.Responses.Known {
		out[ProtocolResponses] = capabilityEntryToExtra(c.Responses)
	}
	if c.ChatCompletions.Known {
		out[ProtocolChatCompletions] = capabilityEntryToExtra(c.ChatCompletions)
	}
	return out
}

func capabilityEntryToExtra(c ProtocolCapability) map[string]any {
	entry := map[string]any{"supported": c.Supported}
	if !c.ProbedAt.IsZero() {
		// RFC3339Nano 保留亚秒精度，避免同一秒内乱序 patch 覆盖较新的结论。
		entry["probed_at"] = c.ProbedAt.UTC().Format(time.RFC3339Nano)
	}
	return entry
}

// ResolveProtocolCapabilities 从 accounts.extra 读取双协议能力，并兼容旧键
// openai_responses_supported：
//   - 旧键 true → responses 已知支持；同时把 chat_completions 视为支持（旧探测
//     只记录了一个 bit，而 OpenAI 兼容上游在支持 Responses 的同时均提供 Chat
//     Completions；保持该协议未知会让 NeedsRefresh 永久为真，导致每分钟重复探测）。
//   - 旧键 false → responses 已知不支持，并隐含 chat_completions 支持（旧路由语义）。
//
// 旧键没有 probed_at，调用方应通过 StampMissingProbedAt 落一次时间戳，让后台
// 按 TTL 低频重探，而不是每个请求都触发。
// 新结构存在时按协议覆盖旧键结论。
func ResolveProtocolCapabilities(extra map[string]any) ProtocolCapabilities {
	var out ProtocolCapabilities
	if extra == nil {
		return out
	}
	if raw, ok := extra[ExtraKeyProtocolCapabilities]; ok {
		out = parseProtocolCapabilities(raw)
	}
	if v, ok := extra[ExtraKeyResponsesSupported]; ok {
		if supported, ok := v.(bool); ok {
			if !out.Responses.Known {
				out.Responses = ProtocolCapability{Known: true, Supported: supported}
			}
			// 旧键只记录 Responses 一个 bit，Chat Completions 是 OpenAI 兼容
			// 上游的基线能力。补全它可避免 legacy 账号的 NeedsRefresh 恒真。
			if !out.ChatCompletions.Known {
				out.ChatCompletions = ProtocolCapability{Known: true, Supported: true}
			}
		}
	}
	return out
}

// StampMissingProbedAt 给已知但缺少 probed_at 的结论补上时间戳。
//
// 旧键迁移来的结论没有 probed_at，Fresh 恒为 false，会让活跃账号在每个
// 请求上触发后台重探。把旧值视为“刚探测过”并持久化，后续按
// ProtocolCapabilityTTL 低频重探即可。未知协议不会被补时间戳。
func (c ProtocolCapabilities) StampMissingProbedAt(now time.Time) (ProtocolCapabilities, bool) {
	changed := false
	if c.Responses.Known && c.Responses.ProbedAt.IsZero() {
		c.Responses.ProbedAt = now
		changed = true
	}
	if c.ChatCompletions.Known && c.ChatCompletions.ProbedAt.IsZero() {
		c.ChatCompletions.ProbedAt = now
		changed = true
	}
	return c, changed
}

type rawProtocolCapabilities struct {
	Responses       *rawProtocolCapability `json:"responses"`
	ChatCompletions *rawProtocolCapability `json:"chat_completions"`
}

type rawProtocolCapability struct {
	Supported bool            `json:"supported"`
	ProbedAt  json.RawMessage `json:"probed_at"`
}

func parseProtocolCapabilities(raw any) ProtocolCapabilities {
	data, err := extraValueJSON(raw)
	if err != nil || len(data) == 0 {
		return ProtocolCapabilities{}
	}
	var parsed rawProtocolCapabilities
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ProtocolCapabilities{}
	}
	return ProtocolCapabilities{
		Responses:       parseRawProtocolCapability(parsed.Responses),
		ChatCompletions: parseRawProtocolCapability(parsed.ChatCompletions),
	}
}

func parseRawProtocolCapability(raw *rawProtocolCapability) ProtocolCapability {
	if raw == nil {
		return ProtocolCapability{}
	}
	return ProtocolCapability{
		Known:     true,
		Supported: raw.Supported,
		ProbedAt:  parseProbedAt(raw.ProbedAt),
	}
}

func parseProbedAt(raw json.RawMessage) time.Time {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		// 某些实现会把 probed_at 写成 Unix 秒。
		var num float64
		if err := json.Unmarshal(raw, &num); err != nil || num <= 0 {
			return time.Time{}
		}
		return time.Unix(int64(num), 0).UTC()
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, text); err == nil {
			return ts
		}
	}
	if seconds, err := strconv.ParseInt(text, 10, 64); err == nil && seconds > 0 {
		return time.Unix(seconds, 0).UTC()
	}
	return time.Time{}
}

func extraValueJSON(raw any) ([]byte, error) {
	switch value := raw.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return value, nil
	case []byte:
		return value, nil
	case string:
		trimmed := strings.TrimSpace(value)
		if strings.HasPrefix(trimmed, "{") {
			return []byte(trimmed), nil
		}
		return json.Marshal(value)
	default:
		return json.Marshal(value)
	}
}

func normalizeProtocolName(protocol string) string {
	switch strings.TrimSpace(protocol) {
	case ProtocolResponses:
		return ProtocolResponses
	case ProtocolChatCompletions:
		return ProtocolChatCompletions
	default:
		return strings.TrimSpace(protocol)
	}
}

func otherProtocol(protocol string) string {
	switch normalizeProtocolName(protocol) {
	case ProtocolResponses:
		return ProtocolChatCompletions
	case ProtocolChatCompletions:
		return ProtocolResponses
	default:
		return ""
	}
}
