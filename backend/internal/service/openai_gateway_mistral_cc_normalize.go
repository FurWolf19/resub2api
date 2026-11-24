package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Mistral 的 Chat Completions 把 assistant 内容返回为字符串或 typed part 数组。
// 数组形态没有 reasoning_content，思考放在 thinking part 里：
//
//	{"type":"thinking","thinking":[{"type":"text","text":"..."}],"closed":true}
//	{"type":"text","text":"..."}
//
// 原生 CC 直转路径不做协议解码，必须在这里把数组换成 OpenAI 客户端认识的
// content / reasoning_content，否则客户端收到原生数组无法渲染。Responses /
// Anthropic 回退路径由 apicompat 解码层归一化，不走这里。
//
// 仅在目标 host 为 Mistral 时启用；非 Mistral 响应保持原始字节不变。

// applyMistralRawChatCompletionsResponse 归一化非流式响应中
// choices[*].message.content 的数组形态。顶层非法 JSON、非数组 content、
// 非对象 message 原样返回。content 之外字段（tool_calls、finish_reason、
// usage、response_error 等）由 sjson 原样保留。
func applyMistralRawChatCompletionsResponse(body []byte) []byte {
	updated, changed := normalizeMistralRawChatChoices(body, "message", apicompat.NormalizeChatContentParts)
	if !changed {
		return body
	}
	return updated
}

// applyMistralRawChatCompletionsSSELine 归一化 `data: <json>` 行中
// choices[*].delta.content 的数组形态。空行、注释行、[DONE]、非法 JSON 与
// 非 data 行原样返回；只替换 JSON payload，保留 `data:` 前缀与行尾换行。
func applyMistralRawChatCompletionsSSELine(line []byte) []byte {
	if len(line) == 0 {
		return line
	}
	end := len(line)
	for end > 0 && (line[end-1] == '\n' || line[end-1] == '\r') {
		end--
	}
	body := line[:end]
	payload, ok := extractOpenAISSEDataLine(string(body))
	if !ok {
		return line
	}
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "[DONE]" || !gjson.Valid(trimmed) {
		return line
	}
	updated, changed := normalizeMistralRawChatChoices([]byte(trimmed), "delta", apicompat.NormalizeChatContentPartsDelta)
	if !changed {
		return line
	}

	prefix := body[:len(body)-len(payload)]
	rewritten := make([]byte, 0, len(prefix)+len(updated)+len(line)-end)
	rewritten = append(rewritten, prefix...)
	rewritten = append(rewritten, updated...)
	rewritten = append(rewritten, line[end:]...)
	return rewritten
}

// normalizeMistralRawChatChoices 按容器名（message / delta）归一化 content 数组。
// normalize 决定多个可见 text part 如何拼接（非流式用段落分隔，流式不插分隔）。
func normalizeMistralRawChatChoices(
	body []byte,
	container string,
	normalize func(json.RawMessage) (content string, reasoning string, isArray bool, err error),
) ([]byte, bool) {
	if len(body) == 0 || !bytes.Contains(body, []byte(`"choices"`)) {
		return body, false
	}
	if !gjson.ValidBytes(body) {
		return body, false
	}
	choices := gjson.GetBytes(body, "choices")
	if !choices.IsArray() {
		return body, false
	}

	updated := body
	changed := false
	for i, choice := range choices.Array() {
		message := choice.Get(container)
		if !message.IsObject() {
			continue
		}
		rawContent := message.Get("content")
		if !rawContent.IsArray() {
			continue
		}
		content, reasoning, isArray, err := normalize(json.RawMessage(rawContent.Raw))
		if err != nil || !isArray {
			continue
		}

		basePath := "choices." + strconv.Itoa(i) + "." + container
		next, err := sjson.SetBytes(updated, basePath+".content", content)
		if err != nil {
			return body, false
		}
		updated = next
		if merged := appendMistralReasoningContent(message.Get("reasoning_content"), reasoning); merged != "" {
			next, err = sjson.SetBytes(updated, basePath+".reasoning_content", merged)
			if err != nil {
				return body, false
			}
			updated = next
		}
		changed = true
	}
	return updated, changed
}

// appendMistralReasoningContent 把数组里的 reasoning 追加到已有
// reasoning_content，任一侧都不丢。数组没有 reasoning 时返回空串，调用方
// 保持原字段不变。
func appendMistralReasoningContent(existing gjson.Result, reasoning string) string {
	if reasoning == "" {
		return ""
	}
	base := ""
	if existing.Type == gjson.String {
		base = existing.Str
	}
	if base == "" {
		return reasoning
	}
	return base + "\n" + reasoning
}
