package service

import (
	"encoding/json"
	"net/url"
	"strings"
)

// mistralChatRejectsReasoningFieldsHost 是已知拒绝 assistant 历史回放
// reasoning_content / reasoning 的 Chat Completions 上游。
//
// 只按目标 host 精确匹配，不按模型名：Mistral 上的托管模型也会使用 DeepSeek
// 等厂商的模型 ID（如 deepseek-v4-1-flash-1017），按前缀判定会误伤。
const mistralChatRejectsReasoningFieldsHost = "api.mistral.ai"

// chatUpstreamRejectsReasoningFields 报告该 Chat Completions 目标是否拒绝
// assistant 消息里的 reasoning 回放字段（Mistral 返回 422 extra_forbidden）。
// URL 解析失败时返回 false，保持原请求不变。
func chatUpstreamRejectsReasoningFields(targetURL string) bool {
	return isMistralChatUpstream(targetURL)
}

// isMistralChatUpstream 按 host 精确判定目标是否为 Mistral Chat Completions。
// 请求侧剥离 reasoning 字段与响应侧归一化 content 数组共用同一判定。
// 比较前去掉 FQDN 尾点并转小写，避免 "api.mistral.ai." 漏判。
func isMistralChatUpstream(targetURL string) bool {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return host == mistralChatRejectsReasoningFieldsHost
}

// stripChatReasoningFields 删除 assistant 消息里的 reasoning_content、
// reasoning、reasoning_details 与 thinking 字段。
//
// Responses→Chat / Anthropic→Chat 桥接会把历史 thinking 写成 assistant 的
// reasoning_content；OpenRouter 等客户端回放时还可能带 reasoning_details 或
// thinking。Mistral 的 Chat Completions 对未知字段返回 422 extra_forbidden，
// 导致多轮与工具续轮失败。这里只处理顶层 messages 数组，content、tool_calls
// 及其他字段保持不变。
//
// 剥离后如果 assistant 既没有非空 content、没有 tool_calls，也没有非空 refusal，
// 整条消息会被删除：Responses→Chat 的 reasoning-only 历史会留下空 assistant，
// Mistral 同样以 422 拒绝。user/system 消息以及带内容、tool_calls 或 refusal 的
// assistant 不受影响。
//
// 非法 JSON、缺失或非数组的 messages、非对象消息一律原样返回，不报错也不
// panic；只有无法编码已解析结构时才返回错误，调用方按 fail-open 处理。
func stripChatReasoningFields(body []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return body, nil
	}
	rawMessages, ok := root["messages"]
	if !ok {
		return body, nil
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		return body, nil
	}

	changed := false
	kept := make([]json.RawMessage, 0, len(messages))
	for _, raw := range messages {
		var message map[string]json.RawMessage
		if err := json.Unmarshal(raw, &message); err != nil || message == nil {
			kept = append(kept, raw)
			continue
		}
		var role string
		if err := json.Unmarshal(message["role"], &role); err != nil || strings.TrimSpace(role) != "assistant" {
			kept = append(kept, raw)
			continue
		}
		_, hasReasoningContent := message["reasoning_content"]
		_, hasReasoning := message["reasoning"]
		_, hasReasoningDetails := message["reasoning_details"]
		_, hasThinking := message["thinking"]
		if !hasReasoningContent && !hasReasoning && !hasReasoningDetails && !hasThinking {
			kept = append(kept, raw)
			continue
		}
		delete(message, "reasoning_content")
		delete(message, "reasoning")
		delete(message, "reasoning_details")
		delete(message, "thinking")
		changed = true
		if chatMessageContentIsEmpty(message["content"]) &&
			!chatMessageHasToolCalls(message["tool_calls"]) &&
			!chatMessageHasNonEmptyRefusal(message["refusal"]) {
			continue
		}
		updated, err := json.Marshal(message)
		if err != nil {
			return body, err
		}
		kept = append(kept, updated)
	}
	if !changed {
		return body, nil
	}

	updatedMessages, err := json.Marshal(kept)
	if err != nil {
		return body, err
	}
	root["messages"] = updatedMessages
	return json.Marshal(root)
}

// chatMessageContentIsEmpty reports whether a chat message content value has no
// usable text. Missing, null, empty or whitespace-only strings are empty. For
// content arrays, parts of type text/input_text/output_text (or an untyped part
// carrying only text) must contain non-whitespace text; any non-text part (for
// example image_url) counts as content. Malformed values are treated as content
// so a local parse quirk cannot drop a message.
func chatMessageContentIsEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text) == ""
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return false
	}
	for _, rawPart := range parts {
		var part struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(rawPart, &part); err != nil {
			return false
		}
		switch part.Type {
		case "", "text", "input_text", "output_text":
			if strings.TrimSpace(part.Text) != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// chatMessageHasToolCalls reports whether tool_calls contains at least one call.
// Malformed values are treated as present so a local parse quirk cannot drop a
// tool turn.
func chatMessageHasToolCalls(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var calls []json.RawMessage
	if err := json.Unmarshal(raw, &calls); err != nil {
		return true
	}
	return len(calls) > 0
}

// chatMessageHasNonEmptyRefusal reports whether an assistant message carries a
// non-empty refusal. A refusal-only assistant turn has no content or tool_calls
// but must still reach the upstream. Malformed values are treated as present.
func chatMessageHasNonEmptyRefusal(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var refusal string
	if err := json.Unmarshal(raw, &refusal); err != nil {
		return true
	}
	return strings.TrimSpace(refusal) != ""
}
