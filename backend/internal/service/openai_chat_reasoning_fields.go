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
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Hostname(), mistralChatRejectsReasoningFieldsHost)
}

// stripChatReasoningFields 删除 assistant 消息里的 reasoning_content 与
// reasoning 字段。
//
// Responses→Chat / Anthropic→Chat 桥接会把历史 thinking 写成 assistant 的
// reasoning_content；Mistral 的 Chat Completions 对未知字段返回 422
// extra_forbidden，导致多轮与工具续轮失败。这里只处理顶层 messages 数组，
// content、tool_calls 及其他字段保持不变。
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
	for i, raw := range messages {
		var message map[string]json.RawMessage
		if err := json.Unmarshal(raw, &message); err != nil || message == nil {
			continue
		}
		var role string
		if err := json.Unmarshal(message["role"], &role); err != nil || strings.TrimSpace(role) != "assistant" {
			continue
		}
		_, hasReasoningContent := message["reasoning_content"]
		_, hasReasoning := message["reasoning"]
		if !hasReasoningContent && !hasReasoning {
			continue
		}
		delete(message, "reasoning_content")
		delete(message, "reasoning")
		updated, err := json.Marshal(message)
		if err != nil {
			return body, err
		}
		messages[i] = updated
		changed = true
	}
	if !changed {
		return body, nil
	}

	updatedMessages, err := json.Marshal(messages)
	if err != nil {
		return body, err
	}
	root["messages"] = updatedMessages
	return json.Marshal(root)
}
