package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mistralThinkingContent = `[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true},{"type":"text","text":"VISIBLE"}]`

func TestChatMessageContent_StringContentUnchanged(t *testing.T) {
	var msg ChatMessage
	require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"VISIBLE"}`), &msg))

	require.JSONEq(t, `"VISIBLE"`, string(msg.Content))
	require.Empty(t, msg.ReasoningContent)
}

func TestChatMessageContent_UserImageArrayPreserved(t *testing.T) {
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-4o",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"look"},
			{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}
		]}]
	}`), &req))

	var rawParts []ChatContentPart
	require.NoError(t, json.Unmarshal(req.Messages[0].Content, &rawParts))
	require.Len(t, rawParts, 2)
	require.NotNil(t, rawParts[1].ImageURL)

	resp, err := ChatCompletionsToResponses(&req)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 2)
	require.Equal(t, "input_text", parts[0].Type)
	require.Equal(t, "look", parts[0].Text)
	require.Equal(t, "input_image", parts[1].Type)
	require.Equal(t, "https://example.com/a.png", parts[1].ImageURL)
}

func TestChatMessageContent_AssistantImageArrayPreservedWithoutReasoning(t *testing.T) {
	var msg ChatMessage
	require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":[
		{"type":"text","text":"VISIBLE"},
		{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}
	]}`), &msg))

	var parts []ChatContentPart
	require.NoError(t, json.Unmarshal(msg.Content, &parts))
	require.Len(t, parts, 2)
	require.NotNil(t, parts[1].ImageURL)
}

func TestChatMessageContent_UserUnknownPartPreserved(t *testing.T) {
	var msg ChatMessage
	require.NoError(t, json.Unmarshal([]byte(`{"role":"user","content":[
		{"type":"text","text":"VISIBLE"},
		{"type":"custom","data":"X"}
	]}`), &msg))

	var parts []map[string]any
	require.NoError(t, json.Unmarshal(msg.Content, &parts))
	require.Len(t, parts, 2)
	require.Equal(t, "custom", parts[1]["type"])
}

func TestChatMessageContent_ArrayTextPart(t *testing.T) {
	var resp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl_array_text",
		"choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"VISIBLE"}]},"finish_reason":"stop"}]
	}`), &resp))

	out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)

	require.Len(t, out.Output, 1)
	require.Equal(t, "message", out.Output[0].Type)
	require.Equal(t, "VISIBLE", out.Output[0].Content[0].Text)
}

func TestChatMessageContent_MultipleTextPartsJoinWithBlankLine(t *testing.T) {
	var resp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl_array_text_parts",
		"choices":[{"index":0,"message":{"role":"assistant","content":[
			{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true},
			{"type":"text","text":"first"},
			{"type":"text","text":"second"}
		]},"finish_reason":"stop"}]
	}`), &resp))

	msg := resp.Choices[0].Message
	require.JSONEq(t, `"first\n\nsecond"`, string(msg.Content))
	require.Equal(t, "SECRET", msg.ReasoningContent)

	out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)
	require.Len(t, out.Output, 2)
	require.Equal(t, "reasoning", out.Output[0].Type)
	require.Equal(t, "first\n\nsecond", out.Output[1].Content[0].Text)
}

func TestChatToResponses_RequestMultipleTextPartsJoinWithBlankLine(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "deepseek-v4-flash",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
			{Role: "assistant", Content: json.RawMessage(`[
				{"type":"text","text":"first"},
				{"type":"text","text":"second"}
			]`)},
		},
	}

	items, _ := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

	require.Len(t, items, 2)
	assert.Equal(t, "first\n\nsecond", messagePartText(t, items[1]))
}

func TestChatMessageContent_MistralThinkingArray(t *testing.T) {
	var resp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl_mistral_thinking",
		"choices":[{"index":0,"message":{"role":"assistant","content":`+mistralThinkingContent+`},"finish_reason":"stop"}]
	}`), &resp))

	msg := resp.Choices[0].Message
	require.JSONEq(t, `"VISIBLE"`, string(msg.Content))
	require.Equal(t, "SECRET", msg.ReasoningContent)

	// A Chat client must receive the reasoning on reasoning_content, not as
	// inline thinking content.
	wire, err := json.Marshal(msg)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"content":"VISIBLE"`)
	require.Contains(t, string(wire), `"reasoning_content":"SECRET"`)
	require.NotContains(t, string(wire), `"type":"thinking"`)

	out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)
	require.Len(t, out.Output, 2)
	require.Equal(t, "reasoning", out.Output[0].Type)
	require.Equal(t, "SECRET", out.Output[0].Summary[0].Text)
	require.Equal(t, "message", out.Output[1].Type)
	require.Equal(t, "VISIBLE", out.Output[1].Content[0].Text)

	serialized, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), `"type":"thinking"`)
}

func TestChatMessageContent_MistralThinkingOnly(t *testing.T) {
	var resp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl_mistral_thinking_only",
		"choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":false}]},"finish_reason":"stop"}]
	}`), &resp))

	msg := resp.Choices[0].Message
	require.JSONEq(t, `""`, string(msg.Content))
	require.Equal(t, "SECRET", msg.ReasoningContent)

	out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)
	require.Len(t, out.Output, 1)
	require.Equal(t, "reasoning", out.Output[0].Type)
	require.Equal(t, "SECRET", out.Output[0].Summary[0].Text)
}

func TestChatMessageContent_MistralThinkingWithToolCalls(t *testing.T) {
	var resp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl_mistral_tool",
		"choices":[{"index":0,"message":{"role":"assistant","content":`+mistralThinkingContent+`,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},"finish_reason":"tool_calls"}]
	}`), &resp))

	out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)

	require.Len(t, out.Output, 3)
	require.Equal(t, "reasoning", out.Output[0].Type)
	require.Equal(t, "SECRET", out.Output[0].Summary[0].Text)
	require.Equal(t, "message", out.Output[1].Type)
	require.Equal(t, "VISIBLE", out.Output[1].Content[0].Text)
	require.Equal(t, "function_call", out.Output[2].Type)
	require.Equal(t, "call_1", out.Output[2].CallID)
	require.Equal(t, `{"key":"alpha"}`, out.Output[2].Arguments)
}

func TestChatMessageContent_UnknownArrayPartPolicy(t *testing.T) {
	t.Run("unknown part with text is visible", func(t *testing.T) {
		var resp ChatCompletionsResponse
		require.NoError(t, json.Unmarshal([]byte(`{
			"id":"chatcmpl_unknown_text_part",
			"choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"analysis","text":"ANALYSIS"},{"type":"text","text":"VISIBLE"}]},"finish_reason":"stop"}]
		}`), &resp))

		out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)
		require.Len(t, out.Output, 1)
		require.Equal(t, "ANALYSIS\n\nVISIBLE", out.Output[0].Content[0].Text)
	})

	t.Run("unknown part without text is dropped", func(t *testing.T) {
		var resp ChatCompletionsResponse
		require.NoError(t, json.Unmarshal([]byte(`{
			"id":"chatcmpl_unknown_meta_part",
			"choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"analysis","data":"HIDDEN"},{"type":"text","text":"VISIBLE"}]},"finish_reason":"stop"}]
		}`), &resp))

		out := ChatCompletionsResponseToResponses(&resp, "mistral-model", nil, nil, false, nil)
		require.Len(t, out.Output, 1)
		require.Equal(t, "VISIBLE", out.Output[0].Content[0].Text)

		serialized, err := json.Marshal(out)
		require.NoError(t, err)
		require.NotContains(t, string(serialized), "HIDDEN")
	})
}

func TestChatToResponses_RequestUnknownTextPartPreserved(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
			{Role: "assistant", Content: json.RawMessage(`[
				{"type":"markdown","text":"# Title"},
				{"type":"attachment","data":"HIDDEN"}
			]`)},
		},
	}

	items, raw := convertChatRequestForReasoningTest(t, req, ChatCompletionsReasoningReplayDrop)

	require.Len(t, items, 2)
	assert.Equal(t, "# Title", messagePartText(t, items[1]))
	assert.NotContains(t, raw, "HIDDEN")
	assert.NotContains(t, raw, `"type":"reasoning"`)
}

func TestChatDeltaContent_ThinkingArraySplit(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`{"choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SEC"}],"closed":true}]}}]}`,
		`{"choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"RET"}],"closed":false}]}}]}`,
		`{"choices":[{"index":0,"delta":{"content":[{"type":"text","text":"VISIBLE"}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	var reasoningDeltas []string
	var textDeltas []string
	for _, e := range events {
		switch e.Type {
		case "response.reasoning_summary_text.delta":
			reasoningDeltas = append(reasoningDeltas, e.Delta)
		case "response.output_text.delta":
			textDeltas = append(textDeltas, e.Delta)
			require.NotContains(t, e.Delta, "SECRET")
		case "response.completed":
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 2)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "SECRET", e.Response.Output[0].Summary[0].Text)
			require.Equal(t, "message", e.Response.Output[1].Type)
			require.Equal(t, "VISIBLE", e.Response.Output[1].Content[0].Text)
		}
	}
	require.Equal(t, []string{"SEC", "RET"}, reasoningDeltas)
	require.Equal(t, []string{"VISIBLE"}, textDeltas)
}

func TestChatDeltaContent_ThinkingArrayKeepsChatReasoningContent(t *testing.T) {
	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true}]}}]}`), &chunk))

	delta := chunk.Choices[0].Delta
	require.Nil(t, delta.Content)
	require.NotNil(t, delta.ReasoningContent)
	require.Equal(t, "SECRET", *delta.ReasoningContent)

	wire, err := json.Marshal(delta)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"reasoning_content":"SECRET"`)
	require.NotContains(t, string(wire), `"type":"thinking"`)
}

func TestChatDeltaContent_UnknownArrayPartPolicy(t *testing.T) {
	t.Run("unknown part with text is visible", func(t *testing.T) {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":[{"type":"analysis","text":"ANALYSIS"},{"type":"text","text":"VISIBLE"}]}}]}`), &chunk))

		delta := chunk.Choices[0].Delta
		require.NotNil(t, delta.Content)
		require.Equal(t, "ANALYSISVISIBLE", *delta.Content)
		require.Nil(t, delta.ReasoningContent)
	})

	t.Run("unknown part without text is dropped", func(t *testing.T) {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":[{"type":"analysis","data":"HIDDEN"},{"type":"text","text":"VISIBLE"}]}}]}`), &chunk))

		delta := chunk.Choices[0].Delta
		require.NotNil(t, delta.Content)
		require.Equal(t, "VISIBLE", *delta.Content)
		require.Nil(t, delta.ReasoningContent)

		wire, err := json.Marshal(delta)
		require.NoError(t, err)
		require.NotContains(t, string(wire), "HIDDEN")
	})
}

func TestChatToResponses_MistralNestedThinkingRequest(t *testing.T) {
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-4o",
		"messages":[
			{"role":"user","content":"hi"},
			{"role":"assistant","content":`+mistralThinkingContent+`}
		]
	}`), &req))

	t.Run("drop mode discards reasoning", func(t *testing.T) {
		items, raw := convertChatRequestForReasoningTest(t, &req, ChatCompletionsReasoningReplayDrop)
		require.Len(t, items, 2)
		assert.Equal(t, "VISIBLE", messagePartText(t, items[1]))
		assert.NotContains(t, raw, "SECRET")
		assert.NotContains(t, raw, `"type":"reasoning"`)
	})

	t.Run("deepseek text mode replays nested reasoning", func(t *testing.T) {
		items, raw := convertChatRequestForReasoningTest(t, &req, ChatCompletionsReasoningReplayText)
		require.Len(t, items, 3)
		assert.Equal(t, "reasoning", items[1].Type)
		assert.Equal(t, "SECRET", reasoningItemText(t, items[1]))
		assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
		assert.NotContains(t, raw, `<thinking>`)
	})

	t.Run("reasoning type with nested text replays", func(t *testing.T) {
		reasoningReq := &ChatCompletionsRequest{
			Model: "deepseek-v4-flash",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"hi"`)},
				{Role: "assistant", Content: json.RawMessage(`[
					{"type":"reasoning","thinking":[{"type":"text","text":"NESTED"}],"closed":true},
					{"type":"text","text":"VISIBLE"}
				]`)},
			},
		}
		items, _ := convertChatRequestForReasoningTest(t, reasoningReq, ChatCompletionsReasoningReplayText)
		require.Len(t, items, 3)
		assert.Equal(t, "NESTED", reasoningItemText(t, items[1]))
		assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
	})

	t.Run("reasoning field with nested text replays", func(t *testing.T) {
		reasoningReq := &ChatCompletionsRequest{
			Model: "deepseek-v4-flash",
			Messages: []ChatMessage{
				{Role: "user", Content: json.RawMessage(`"hi"`)},
				{Role: "assistant", Content: json.RawMessage(`[
					{"type":"reasoning","reasoning":[{"type":"text","text":"NESTED2"}],"closed":true},
					{"type":"text","text":"VISIBLE"}
				]`)},
			},
		}
		items, _ := convertChatRequestForReasoningTest(t, reasoningReq, ChatCompletionsReasoningReplayText)
		require.Len(t, items, 3)
		assert.Equal(t, "NESTED2", reasoningItemText(t, items[1]))
		assert.Equal(t, "VISIBLE", messagePartText(t, items[2]))
	})
}

func TestChatDeltaContent_StringStillParses(t *testing.T) {
	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":"VISIBLE","reasoning_content":"SECRET"}}]}`), &chunk))

	delta := chunk.Choices[0].Delta
	require.NotNil(t, delta.Content)
	require.Equal(t, "VISIBLE", *delta.Content)
	require.NotNil(t, delta.ReasoningContent)
	require.Equal(t, "SECRET", *delta.ReasoningContent)
}

func TestChatDeltaContent_MultipleTextPartsJoinWithoutSeparator(t *testing.T) {
	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":[
		{"type":"text","text":"first"},
		{"type":"text","text":"second"}
	]}}]}`), &chunk))

	delta := chunk.Choices[0].Delta
	require.NotNil(t, delta.Content)
	require.Equal(t, "firstsecond", *delta.Content, "streaming parts are one increment; no paragraph break may be inserted")

	events := ChatCompletionsChunkToResponsesEvents(&chunk, NewChatCompletionsToResponsesStreamState("deepseek-v4-pro"))
	var deltas []string
	for _, event := range events {
		if event.Type == "response.output_text.delta" {
			deltas = append(deltas, event.Delta)
		}
	}
	require.Equal(t, []string{"firstsecond"}, deltas)
}

func TestChatDeltaContent_ThinkingAndTextInSameChunk(t *testing.T) {
	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}]},{"type":"text","text":"VISIBLE"}]}}]}`), &chunk))

	delta := chunk.Choices[0].Delta
	require.NotNil(t, delta.Content)
	require.Equal(t, "VISIBLE", *delta.Content)
	require.NotNil(t, delta.ReasoningContent)
	require.Equal(t, "SECRET", *delta.ReasoningContent)
}

func TestChatMessageContent_MistralThinkingDoesNotLeakIntoTextEvents(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true}]}}]}`,
		`{"choices":[{"index":0,"delta":{"content":[{"type":"text","text":"VISIBLE"}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	for _, e := range events {
		if e.Type == "response.output_text.delta" || e.Type == "response.output_text.done" {
			require.NotContains(t, e.Delta, "SECRET")
			require.NotContains(t, e.Text, "SECRET")
			require.NotContains(t, strings.Join([]string{e.Delta, e.Text}, " "), "thinking")
		}
	}
}
