package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func convertChatRequestForReasoningTest(t *testing.T, req *ChatCompletionsRequest, replay ChatCompletionsReasoningReplay) ([]ResponsesInputItem, string) {
	t.Helper()
	resp, err := ChatCompletionsToResponsesWithReasoningReplay(req, DefaultMinMaxOutputTokens, replay)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	return items, string(resp.Input)
}

func reasoningItemText(t *testing.T, item ResponsesInputItem) string {
	t.Helper()
	require.Equal(t, "reasoning", item.Type)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(item.Content, &parts))
	require.Len(t, parts, 1)
	require.Equal(t, "reasoning_text", parts[0].Type)
	return parts[0].Text
}

func messagePartText(t *testing.T, item ResponsesInputItem) string {
	t.Helper()
	require.Equal(t, "message", item.Type)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(item.Content, &parts))
	require.Len(t, parts, 1)
	require.Equal(t, "output_text", parts[0].Type)
	return parts[0].Text
}

func TestChatToResponses_DropsReasoningContentFromVisibleText(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
			{Role: "assistant", ReasoningContent: "SECRET", Content: json.RawMessage(`"VISIBLE"`)},
		},
	}

	items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

	require.Len(t, items, 2)
	assert.Equal(t, "VISIBLE", messagePartText(t, items[1]))
	assert.NotContains(t, raw, "SECRET")
	assert.NotContains(t, raw, "<thinking>")
	assert.NotContains(t, raw, `"type":"reasoning"`)
}

func TestChatToResponses_DropsStructuredThinkingParts(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "thinking field",
			content: `[{"type":"thinking","thinking":"SECRET"},{"type":"text","text":"VISIBLE"}]`,
		},
		{
			name:    "reasoning text field",
			content: `[{"type":"reasoning","text":"SECRET"},{"type":"text","text":"VISIBLE"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatCompletionsRequest{
				Model: "gpt-4o",
				Messages: []ChatMessage{
					{Role: "user", Content: json.RawMessage(`"Hi"`)},
					{Role: "assistant", Content: json.RawMessage(tt.content)},
				},
			}

			items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

			require.Len(t, items, 2)
			assert.Equal(t, "VISIBLE", messagePartText(t, items[1]))
			assert.NotContains(t, raw, "SECRET")
			assert.NotContains(t, raw, "<thinking>")
		})
	}
}

func TestChatToResponses_ReasoningAliasMatchesReasoningContent(t *testing.T) {
	base := func() *ChatCompletionsRequest {
		return &ChatCompletionsRequest{
			Model: "gpt-4o",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"Hi"`)},
				{Role: "assistant", Reasoning: "SECRET", Content: json.RawMessage(`"VISIBLE"`)},
			},
		}
	}

	items, raw := convertChatRequestForReasoningTest(t, base(), ChatCompletionsReasoningReplayDrop)
	require.Len(t, items, 2)
	assert.Equal(t, "VISIBLE", messagePartText(t, items[1]))
	assert.NotContains(t, raw, "SECRET")

	items, _ = convertChatRequestForReasoningTest(t, base(), ChatCompletionsReasoningReplayText)
	require.Len(t, items, 3)
	assert.Equal(t, "SECRET", reasoningItemText(t, items[1]))
	assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
}

func TestChatToResponses_DeepSeekUsesReasoningTextItem(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "deepseek-v4-flash",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Go"`)},
			{
				Role:             "assistant",
				ReasoningContent: "SECRET",
				Content:          json.RawMessage(`"VISIBLE"`),
				ToolCalls: []ChatToolCall{{
					ID: "call_1", Type: "function",
					Function: ChatFunctionCall{Name: "lookup", Arguments: `{"key":"alpha"}`},
				}},
			},
		},
	}

	items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayText)

	require.Len(t, items, 4)
	assert.Equal(t, "user", items[0].Role)
	assert.Equal(t, "reasoning", items[1].Type)
	assert.Equal(t, "SECRET", reasoningItemText(t, items[1]))
	assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
	require.Equal(t, "function_call", items[3].Type)
	assert.Equal(t, "call_1", items[3].CallID)
	assert.Equal(t, "lookup", items[3].Name)
	assert.Equal(t, `{"key":"alpha"}`, items[3].Arguments)

	reasoningJSON, err := json.Marshal(items[1])
	require.NoError(t, err)
	assert.NotContains(t, string(reasoningJSON), "summary")
	assert.NotContains(t, string(reasoningJSON), "encrypted_content")
	assert.NotContains(t, raw, "<thinking>")
}

func TestChatToResponses_DeepSeekReasoningBeforeToolCall(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "deepseek-v4-flash",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Go"`)},
			{
				Role:             "assistant",
				ReasoningContent: "SECRET",
				ToolCalls: []ChatToolCall{{
					ID: "call_1", Type: "function",
					Function: ChatFunctionCall{Name: "lookup", Arguments: `{}`},
				}},
			},
		},
	}

	items, _ := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayText)

	require.Len(t, items, 3)
	assert.Equal(t, "reasoning", items[1].Type)
	assert.Equal(t, "SECRET", reasoningItemText(t, items[1]))
	assert.Equal(t, "function_call", items[2].Type)
	assert.Equal(t, "call_1", items[2].CallID)
}

func TestChatToResponses_DeepSeekDropsDanglingReasoningOnlyTurn(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "deepseek-v4-flash",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"first"`)},
			{Role: "assistant", ReasoningContent: "SECRET"},
			{Role: "user", Content: json.RawMessage(`"second"`)},
		},
	}

	items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayText)

	require.Len(t, items, 2)
	for _, item := range items {
		require.Equal(t, "user", item.Role)
	}
	assert.NotContains(t, raw, "SECRET")
	assert.NotContains(t, raw, `"type":"reasoning"`)
}

func TestChatToResponses_NonDeepSeekEmitsNoReasoningItem(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Go"`)},
			{
				Role:             "assistant",
				ReasoningContent: "SECRET",
				ToolCalls: []ChatToolCall{{
					ID: "call_1", Type: "function",
					Function: ChatFunctionCall{Name: "lookup", Arguments: `{}`},
				}},
			},
		},
	}

	items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

	require.Len(t, items, 2)
	assert.Equal(t, "function_call", items[1].Type)
	assert.NotContains(t, raw, `"type":"reasoning"`)
	assert.NotContains(t, raw, "SECRET")
}

func TestChatToResponses_ToolCallsAndVisibleTextPreserved(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Go"`)},
			{
				Role:             "assistant",
				ReasoningContent: "SECRET",
				Content:          json.RawMessage(`"Answer"`),
				ToolCalls: []ChatToolCall{
					{ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "lookup", Arguments: `{"key":"a"}`}},
					{ID: "call_b", Type: "function", Function: ChatFunctionCall{Name: "lookup"}},
				},
			},
		},
	}

	items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

	require.Len(t, items, 4)
	assert.Equal(t, "message", items[1].Type)
	assert.Equal(t, "Answer", messagePartText(t, items[1]))
	assert.Equal(t, "call_a", items[2].CallID)
	assert.Equal(t, "call_b", items[3].CallID)
	assert.Equal(t, "{}", items[3].Arguments)
	assert.NotContains(t, raw, "SECRET")
}

func TestChatCompletionsResponseToResponses_ReasoningOnlyHasNoMessageText(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl_reasoning_only",
		Model: "deepseek-reasoner",
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:             "assistant",
				Content:          json.RawMessage(`""`),
				ReasoningContent: "SECRET",
			},
			FinishReason: "stop",
		}},
	}

	out := ChatCompletionsResponseToResponses(resp, "deepseek-reasoner", nil, nil, false, nil)

	require.Len(t, out.Output, 1)
	assert.Equal(t, "reasoning", out.Output[0].Type)
	require.Len(t, out.Output[0].Summary, 1)
	assert.Equal(t, "SECRET", out.Output[0].Summary[0].Text)
	assert.Equal(t, "completed", out.Status)
}

func TestChatCompletionsResponseToResponses_ReasoningAndTextStaySeparate(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl_reasoning_text",
		Model: "deepseek-reasoner",
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:             "assistant",
				Content:          json.RawMessage(`"VISIBLE"`),
				ReasoningContent: "SECRET",
			},
			FinishReason: "stop",
		}},
	}

	out := ChatCompletionsResponseToResponses(resp, "deepseek-reasoner", nil, nil, false, nil)

	require.Len(t, out.Output, 2)
	assert.Equal(t, "reasoning", out.Output[0].Type)
	assert.Equal(t, "SECRET", out.Output[0].Summary[0].Text)
	require.Equal(t, "message", out.Output[1].Type)
	require.Len(t, out.Output[1].Content, 1)
	assert.Equal(t, "VISIBLE", out.Output[1].Content[0].Text)
}

func TestChatCompletionsResponseToResponses_ReasoningWithToolCall(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl_reasoning_tool",
		Model: "deepseek-reasoner",
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:             "assistant",
				Content:          json.RawMessage(`""`),
				ReasoningContent: "SECRET",
				ToolCalls: []ChatToolCall{{
					ID:       "call_a",
					Type:     "function",
					Function: ChatFunctionCall{Name: "exec", Arguments: `{"cmd":"ls"}`},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}

	out := ChatCompletionsResponseToResponses(resp, "deepseek-reasoner", nil, nil, false, nil)

	require.Len(t, out.Output, 2)
	assert.Equal(t, "reasoning", out.Output[0].Type)
	require.Equal(t, "function_call", out.Output[1].Type)
	assert.Equal(t, `{"cmd":"ls"}`, out.Output[1].Arguments)
}

func TestStream_ReasoningOnlyCompletedHasNoMessage(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"SECRET"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	var sawReasoningDelta bool
	for _, e := range events {
		switch e.Type {
		case "response.reasoning_summary_text.delta":
			sawReasoningDelta = true
			require.Equal(t, "SECRET", e.Delta)
		case "response.output_text.delta", "response.output_text.done":
			require.Failf(t, "unexpected visible text event", "reasoning-only stream emitted %s", e.Type)
		case "response.output_item.done":
			if e.Item != nil {
				require.NotEqual(t, "message", e.Item.Type)
			}
		case "response.completed":
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 1)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "SECRET", e.Response.Output[0].Summary[0].Text)
			require.Equal(t, "completed", e.Response.Status)
		}
	}
	require.True(t, sawReasoningDelta)
}

func TestStream_ReasoningThenContentStaysSeparate(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"SECRET"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"VISIBLE"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	var textDeltas []string
	for _, e := range events {
		switch e.Type {
		case "response.output_text.delta":
			textDeltas = append(textDeltas, e.Delta)
		case "response.completed":
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 2)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "SECRET", e.Response.Output[0].Summary[0].Text)
			require.Equal(t, "message", e.Response.Output[1].Type)
			require.Equal(t, "VISIBLE", e.Response.Output[1].Content[0].Text)
		}
	}
	require.Equal(t, []string{"VISIBLE"}, textDeltas)
}

func TestStream_ReasoningWithToolCallLifecycleUnchanged(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"SECRET"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	var sawArgsDone, sawItemDone bool
	for _, e := range events {
		switch e.Type {
		case "response.output_text.delta", "response.output_text.done":
			require.Failf(t, "unexpected visible text event", "reasoning+tool stream emitted %s", e.Type)
		case "response.function_call_arguments.done":
			sawArgsDone = true
			require.Equal(t, `{"cmd":"ls"}`, e.Arguments)
		case "response.output_item.done":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawItemDone = true
				require.Equal(t, "call_a", e.Item.CallID)
			}
		case "response.completed":
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 2)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "function_call", e.Response.Output[1].Type)
		}
	}
	require.True(t, sawArgsDone)
	require.True(t, sawItemDone)
}

func TestStream_EmptyCompletionKeepsEmptyMessage(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	for _, e := range events {
		if e.Type == "response.completed" {
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 1)
			require.Equal(t, "message", e.Response.Output[0].Type)
			require.Len(t, e.Response.Output[0].Content, 1)
			require.Equal(t, "", e.Response.Output[0].Content[0].Text)
		}
	}
}

func TestChatToResponses_StripsLegacyThinkingPrefix(t *testing.T) {
	t.Run("drop mode", func(t *testing.T) {
		req := &ChatCompletionsRequest{
			Model: "gpt-4o",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"Hi"`)},
				{Role: "assistant", Content: json.RawMessage(`"<thinking>OLD</thinking>\nVISIBLE"`)},
			},
		}

		items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

		require.Len(t, items, 2)
		assert.Equal(t, "VISIBLE", messagePartText(t, items[1]))
		assert.NotContains(t, raw, "OLD")
		assert.NotContains(t, raw, "<thinking>")
	})

	t.Run("deepseek mode", func(t *testing.T) {
		req := &ChatCompletionsRequest{
			Model: "deepseek-v4-flash",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"Hi"`)},
				{Role: "assistant", Content: json.RawMessage(`"<thinking>OLD</thinking>\nVISIBLE"`)},
			},
		}

		items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayText)

		require.Len(t, items, 3)
		assert.Equal(t, "reasoning", items[1].Type)
		assert.Equal(t, "OLD", reasoningItemText(t, items[1]))
		assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
		assert.NotContains(t, raw, "<thinking>")
	})

	t.Run("deepseek mode does not duplicate reasoning", func(t *testing.T) {
		req := &ChatCompletionsRequest{
			Model: "deepseek-v4-flash",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"Hi"`)},
				{
					Role:             "assistant",
					ReasoningContent: "OLD",
					Content:          json.RawMessage(`"<thinking>OLD</thinking>\nVISIBLE"`),
				},
			},
		}

		items, _ := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayText)

		require.Len(t, items, 3)
		assert.Equal(t, "OLD", reasoningItemText(t, items[1]))
		assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
	})
}

func TestResponsesToChat_StripsLegacyThinkingPrefix(t *testing.T) {
	t.Run("backfills reasoning content", func(t *testing.T) {
		input := `[
			{"role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"<thinking>OLD</thinking>\nVISIBLE"}]},
			{"role":"user","content":[{"type":"input_text","text":"next"}]}
		]`
		req, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{Model: "deepseek-v4-flash", Input: json.RawMessage(input)})
		require.NoError(t, err)
		require.Len(t, req.Messages, 3)
		assistant := req.Messages[1]
		require.Equal(t, "assistant", assistant.Role)
		require.JSONEq(t, `"VISIBLE"`, string(assistant.Content))
		require.Equal(t, "OLD", assistant.ReasoningContent)
	})

	t.Run("keeps explicit reasoning item", func(t *testing.T) {
		input := `[
			{"role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"OLD2"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"<thinking>OLD</thinking>\nVISIBLE"}]},
			{"role":"user","content":[{"type":"input_text","text":"next"}]}
		]`
		req, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{Model: "deepseek-v4-flash", Input: json.RawMessage(input)})
		require.NoError(t, err)
		require.Len(t, req.Messages, 3)
		assistant := req.Messages[1]
		require.JSONEq(t, `"VISIBLE"`, string(assistant.Content))
		require.Equal(t, "OLD2", assistant.ReasoningContent)
	})
}

func TestThinkingStrip_DoesNotTouchNonWrapperText(t *testing.T) {
	assistantTexts := []string{
		"prefix <thinking>KEEP</thinking>\nVISIBLE",
		"<thinking>x</thinking> is a tag",
		"<Thinking>x</Thinking>\nVISIBLE",
	}

	for _, text := range assistantTexts {
		req := &ChatCompletionsRequest{
			Model: "gpt-4o",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"<thinking>KEEP</thinking>"`)},
				{Role: "assistant", Content: json.RawMessage(strconvQuote(text))},
			},
		}

		items, _ := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)
		require.Len(t, items, 2, "input text %q", text)
		assert.JSONEq(t, strconvQuote("<thinking>KEEP</thinking>"), string(items[0].Content))
		assert.Equal(t, text, messagePartText(t, items[1]))
	}
}

func TestChatToResponses_LeadingCompleteThinkingThenUnclosedDoesNotLeak(t *testing.T) {
	const content = "<thinking>SECRET</thinking>\n<thinking>OOPS"
	base := func() *ChatCompletionsRequest {
		return &ChatCompletionsRequest{
			Model: "deepseek-v4-flash",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"Hi"`)},
				{Role: "assistant", Content: json.RawMessage(strconvQuote(content))},
			},
		}
	}

	items, raw := convertChatRequestForReasoningTest(t, base(), ChatCompletionsReasoningReplayDrop)
	require.Len(t, items, 1, "an unterminated leading thinking block must not produce a visible assistant turn")
	assert.NotContains(t, raw, "SECRET")
	assert.NotContains(t, raw, "OOPS")
	assert.NotContains(t, raw, "<thinking>")

	withTool := base()
	withTool.Messages[1].ToolCalls = []ChatToolCall{{
		ID: "call_1", Type: "function",
		Function: ChatFunctionCall{Name: "lookup", Arguments: `{}`},
	}}
	items, raw = convertChatRequestForReasoningTest(t, withTool, ChatCompletionsReasoningReplayText)
	require.Len(t, items, 3)
	assert.Equal(t, "reasoning", items[1].Type)
	assert.Equal(t, "SECRET", reasoningItemText(t, items[1]))
	assert.Equal(t, "function_call", items[2].Type)
	assert.NotContains(t, raw, "OOPS")
	assert.NotContains(t, raw, "<thinking>")
}

func strconvQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestStream_ReasoningOnlySummaryEventsPreserved(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"SECRET"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	var eventTypes []string
	for _, e := range events {
		eventTypes = append(eventTypes, e.Type)
	}
	joined := strings.Join(eventTypes, "\n")
	for _, want := range []string{
		"response.output_item.added",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_text.delta",
		"response.reasoning_summary_text.done",
		"response.reasoning_summary_part.done",
		"response.output_item.done",
		"response.completed",
	} {
		require.Contains(t, joined, want)
	}
}
