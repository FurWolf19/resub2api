package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Some OpenAI-compatible upstreams return reasoning inline in the Chat
// Completions content field, as a string or as an array of typed parts. The
// array shape has no reasoning_content field; reasoning is carried by
// {"type":"thinking","thinking":[{"type":"text","text":"..."}]} parts.
//
// Only the response path is normalized: ChatMessage is also used for client
// requests, where user/system content arrays must keep their multimodal parts
// (image_url, file) untouched. ChatChoice.UnmarshalJSON therefore normalizes
// the assistant message after the whole response has been decoded.
func (c *ChatChoice) UnmarshalJSON(data []byte) error {
	type chatChoiceAlias ChatChoice
	var alias chatChoiceAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*c = ChatChoice(alias)
	normalizeChatResponseMessageContent(&c.Message)
	return nil
}

func normalizeChatResponseMessageContent(m *ChatMessage) {
	if m == nil || m.Role != "assistant" {
		return
	}
	raw := bytes.TrimSpace(m.Content)
	if len(raw) == 0 || raw[0] != '[' {
		return
	}
	visible, reasoning := chatContentArrayParts(raw)
	if reasoning == "" {
		return
	}
	m.Content = marshalChatContentString(visible)
	m.ReasoningContent = joinChatReasoningText(m.reasoningText(), reasoning)
}

// NormalizeChatContentParts splits a Chat Completions response content value
// into visible text and reasoning text. Strings pass through unchanged. Arrays
// follow the same part rules as ChatChoice.UnmarshalJSON: thinking/reasoning
// parts become reasoning, text/output_text/input_text parts become visible
// content, and unknown parts are dropped. isArray reports whether raw was an
// array so callers can tell whether the value needs rewriting at all.
//
// This is for wire-level passthrough paths that cannot decode into ChatMessage;
// protocol conversion keeps using the decoders above.
func NormalizeChatContentParts(raw json.RawMessage) (content, reasoning string, isArray bool, err error) {
	return normalizeChatContentParts(raw, "\n\n")
}

// NormalizeChatContentPartsDelta is the streaming variant: visible parts are
// concatenated without a separator because they belong to one delta increment.
func NormalizeChatContentPartsDelta(raw json.RawMessage) (content, reasoning string, isArray bool, err error) {
	return normalizeChatContentParts(raw, "")
}

func normalizeChatContentParts(raw json.RawMessage, textSeparator string) (content, reasoning string, isArray bool, err error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", "", false, nil
	}
	if trimmed[0] == '[' {
		content, reasoning, _, _ = chatContentArrayPartsDetailed(trimmed, textSeparator)
		return content, reasoning, true, nil
	}
	if err := json.Unmarshal(trimmed, &content); err != nil {
		return "", "", false, fmt.Errorf("normalize chat content parts: %w", err)
	}
	return content, "", false, nil
}

// UnmarshalJSON accepts delta.content as a string or an array of typed parts.
// Reasoning parts are moved to reasoning_content and never become visible
// content.
func (d *ChatDelta) UnmarshalJSON(data []byte) error {
	var wire struct {
		Role             string          `json:"role,omitempty"`
		Content          json.RawMessage `json:"content"`
		Refusal          *string         `json:"refusal,omitempty"`
		ReasoningContent *string         `json:"reasoning_content,omitempty"`
		Reasoning        *string         `json:"reasoning,omitempty"`
		ToolCalls        []ChatToolCall  `json:"tool_calls,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*d = ChatDelta{
		Role:             wire.Role,
		Refusal:          wire.Refusal,
		ReasoningContent: wire.ReasoningContent,
		Reasoning:        wire.Reasoning,
		ToolCalls:        wire.ToolCalls,
	}

	raw := bytes.TrimSpace(wire.Content)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var content string
	if err := json.Unmarshal(raw, &content); err == nil {
		d.Content = &content
		return nil
	}
	if raw[0] != '[' {
		return nil
	}

	// A streaming delta's parts belong to the same increment: inserting a
	// separator would fabricate paragraph breaks inside one token span.
	visible, reasoning, hasVisible, hasReasoning := chatContentArrayPartsDetailed(raw, "")
	if hasVisible {
		d.Content = &visible
	}
	if hasReasoning && reasoning != "" {
		existing := ""
		if d.ReasoningContent != nil {
			existing = *d.ReasoningContent
		} else if d.Reasoning != nil {
			existing = *d.Reasoning
		}
		combined := joinChatReasoningText(existing, reasoning)
		d.ReasoningContent = &combined
	}
	return nil
}

func chatContentArrayParts(raw json.RawMessage) (visible, reasoning string) {
	visible, reasoning, _, _ = chatContentArrayPartsDetailed(raw, "\n\n")
	return visible, reasoning
}

// chatContentArrayPartsDetailed splits array content into visible text and
// reasoning text. Unknown part types are ignored: they are neither promoted to
// visible content nor turned into reasoning. textSeparator joins multiple
// visible parts: non-streaming messages use "\n\n" (matching
// chatMessageContentText), streaming deltas use "" to avoid injecting paragraph
// breaks into an increment.
func chatContentArrayPartsDetailed(raw json.RawMessage, textSeparator string) (visible, reasoning string, hasVisible, hasReasoning bool) {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", "", false, false
	}
	var visibleBuilder, reasoningBuilder strings.Builder
	for _, partRaw := range parts {
		var part map[string]json.RawMessage
		if err := json.Unmarshal(partRaw, &part); err != nil {
			continue
		}
		switch rawString(part["type"]) {
		case "thinking", "reasoning":
			text := chatContentPartReasoning(part)
			if text == "" {
				continue
			}
			hasReasoning = true
			reasoningBuilder.WriteString(text)
		case "text", "output_text", "input_text", "":
			text := rawString(part["text"])
			if text == "" {
				continue
			}
			if hasVisible {
				visibleBuilder.WriteString(textSeparator)
			}
			hasVisible = true
			visibleBuilder.WriteString(text)
		}
	}
	return visibleBuilder.String(), reasoningBuilder.String(), hasVisible, hasReasoning
}

// chatContentPartReasoning reads a reasoning part. The value may be a plain
// string or an array of {"type":"text","text":"..."} entries (Mistral). The
// "closed" flag is deliberately ignored.
func chatContentPartReasoning(part map[string]json.RawMessage) string {
	for _, key := range []string{"thinking", "reasoning"} {
		if raw, ok := part[key]; ok {
			if text := chatNestedText(raw); text != "" {
				return text
			}
		}
	}
	return chatNestedText(part["text"])
}

func chatNestedText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, partRaw := range parts {
		var part map[string]json.RawMessage
		if err := json.Unmarshal(partRaw, &part); err != nil {
			continue
		}
		b.WriteString(rawString(part["text"]))
	}
	return b.String()
}

func marshalChatContentString(text string) json.RawMessage {
	raw, err := json.Marshal(text)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return raw
}

func joinChatReasoningText(existing, extra string) string {
	if extra == "" {
		return existing
	}
	if existing == "" {
		return extra
	}
	return existing + "\n" + extra
}
