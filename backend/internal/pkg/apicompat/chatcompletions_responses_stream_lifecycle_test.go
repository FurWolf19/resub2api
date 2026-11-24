package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func collectStreamEvents(t *testing.T, chunks []string) []ResponsesStreamEvent {
	t.Helper()
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	var events []ResponsesStreamEvent
	for _, payload := range chunks {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
		events = append(events, ChatCompletionsChunkToResponsesEvents(&chunk, state)...)
	}
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)
	return events
}

// TestStream_ReasoningOpensItemBeforeDelta guards the bug where a strict client
// (Codex) drops reasoning deltas that reference an item not yet opened.
func TestStream_ReasoningOpensItemBeforeDelta(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"think"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	open := map[int]string{} // output_index -> item type
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			require.NotNil(t, e.Item)
			open[e.OutputIndex] = e.Item.Type
		case "response.reasoning_summary_text.delta":
			require.Equalf(t, "reasoning", open[e.OutputIndex], "reasoning delta before its item was opened")
		case "response.output_text.delta":
			require.Equalf(t, "message", open[e.OutputIndex], "text delta before its item was opened")
		}
	}
}

func TestStream_ReasoningOnlyDoesNotSynthesizeVisibleText(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"thinking before final"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	open := map[int]string{}
	var sawReasoningDelta bool
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			require.NotNil(t, e.Item)
			open[e.OutputIndex] = e.Item.Type
			require.NotEqual(t, "message", e.Item.Type, "reasoning-only stream must not open a message item")
		case "response.reasoning_summary_text.delta":
			sawReasoningDelta = true
			require.Equalf(t, "reasoning", open[e.OutputIndex], "reasoning delta before its item was opened")
			require.Equal(t, "thinking before final", e.Delta)
		case "response.output_text.delta", "response.output_text.done":
			require.Failf(t, "unexpected visible text event", "reasoning-only stream emitted %s", e.Type)
		case "response.output_item.done":
			if e.Item != nil {
				require.NotEqual(t, "message", e.Item.Type, "reasoning-only stream must not close a message item")
			}
		case "response.completed":
			require.NotNil(t, e.Response)
			require.Equal(t, "incomplete", e.Response.Status)
			require.NotNil(t, e.Response.IncompleteDetails)
			require.Equal(t, "max_output_tokens", e.Response.IncompleteDetails.Reason)
			require.Len(t, e.Response.Output, 1)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "thinking before final", e.Response.Output[0].Summary[0].Text)
		}
	}
	require.True(t, sawReasoningDelta, "reasoning-only stream must keep reasoning deltas")
}

func TestStream_CompletedOutputReusesStreamedItemIDs(t *testing.T) {
	t.Run("reasoning and message", func(t *testing.T) {
		events := collectStreamEvents(t, []string{
			`{"choices":[{"index":0,"delta":{"reasoning_content":"plan"}}]}`,
			`{"choices":[{"index":0,"delta":{"content":"answer"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		})

		added := map[string]string{}
		var completed *ResponsesResponse
		for _, e := range events {
			switch e.Type {
			case "response.output_item.added":
				require.NotNil(t, e.Item)
				added[e.Item.Type] = e.Item.ID
			case "response.completed":
				require.NotNil(t, e.Response)
				completed = e.Response
			}
		}
		require.NotNil(t, completed)
		require.Len(t, completed.Output, 2)
		require.Equal(t, "reasoning", completed.Output[0].Type)
		require.Equal(t, added["reasoning"], completed.Output[0].ID, "completed reasoning item must reuse the streamed output_item.added id")
		require.NotEmpty(t, completed.Output[0].ID)
		require.Equal(t, "message", completed.Output[1].Type)
		require.Equal(t, added["message"], completed.Output[1].ID)
	})

	t.Run("reasoning and tool call", func(t *testing.T) {
		events := collectStreamEvents(t, []string{
			`{"choices":[{"index":0,"delta":{"reasoning_content":"plan"}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"exec","arguments":"{}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		})

		added := map[string]string{}
		var completed *ResponsesResponse
		for _, e := range events {
			switch e.Type {
			case "response.output_item.added":
				require.NotNil(t, e.Item)
				added[e.Item.Type] = e.Item.ID
			case "response.completed":
				require.NotNil(t, e.Response)
				completed = e.Response
			}
		}
		require.NotNil(t, completed)
		require.Len(t, completed.Output, 2)
		require.Equal(t, "reasoning", completed.Output[0].Type)
		require.Equal(t, added["reasoning"], completed.Output[0].ID, "completed reasoning item must reuse the streamed output_item.added id")
		require.NotEmpty(t, completed.Output[0].ID)
		require.Equal(t, "function_call", completed.Output[1].Type)
	})

	t.Run("reasoning only", func(t *testing.T) {
		events := collectStreamEvents(t, []string{
			`{"choices":[{"index":0,"delta":{"reasoning_content":"plan"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		})

		var addedID string
		var completed *ResponsesResponse
		for _, e := range events {
			switch e.Type {
			case "response.output_item.added":
				if e.Item != nil && e.Item.Type == "reasoning" {
					addedID = e.Item.ID
				}
			case "response.completed":
				completed = e.Response
			}
		}
		require.NotNil(t, completed)
		require.Len(t, completed.Output, 1)
		require.Equal(t, "reasoning", completed.Output[0].Type)
		require.Equal(t, addedID, completed.Output[0].ID)
	})

	t.Run("no reasoning has no reasoning output", func(t *testing.T) {
		events := collectStreamEvents(t, []string{
			`{"choices":[{"index":0,"delta":{"content":"answer"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		})

		for _, e := range events {
			if e.Type == "response.completed" {
				require.NotNil(t, e.Response)
				require.Len(t, e.Response.Output, 1)
				require.Equal(t, "message", e.Response.Output[0].Type)
			}
		}
	})
}

func TestStream_WhitespaceReasoningKeepsEmptyMessage(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"   "}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	for _, e := range events {
		require.NotEqual(t, "response.output_text.delta", e.Type)
		if e.Type == "response.completed" {
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 2)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "message", e.Response.Output[1].Type)
			require.Equal(t, "", e.Response.Output[1].Content[0].Text)
		}
	}
}

func TestStream_ReasoningThenContentDoesNotDuplicateFallbackText(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"private plan"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"final answer"}}]}`,
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
			require.Equal(t, "private plan", e.Response.Output[0].Summary[0].Text)
			require.Equal(t, "final answer", e.Response.Output[1].Content[0].Text)
		}
	}
	require.Equal(t, []string{"final answer"}, textDeltas)
}

func TestStream_ReasoningThenToolCallDoesNotSynthesizeVisibleText(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"call a tool"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	for _, e := range events {
		require.NotEqual(t, "response.output_text.delta", e.Type)
		if e.Type == "response.completed" {
			require.NotNil(t, e.Response)
			require.Len(t, e.Response.Output, 2)
			require.Equal(t, "reasoning", e.Response.Output[0].Type)
			require.Equal(t, "function_call", e.Response.Output[1].Type)
		}
	}
}

// TestStream_ToolCallLifecycleComplete guards that a tool call is fully closed
// (function_call_arguments.done + output_item.done with full arguments), which
// codex needs to execute the call.
func TestStream_ToolCallLifecycleComplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\":\"ls\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	var sawAdded, sawArgsDone, sawItemDone bool
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawAdded = true
			}
		case "response.function_call_arguments.done":
			sawArgsDone = true
			require.Equal(t, `{"cmd":"ls"}`, e.Arguments)
		case "response.output_item.done":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawItemDone = true
				require.Equal(t, `{"cmd":"ls"}`, e.Item.Arguments)
				require.Equal(t, "call_a", e.Item.CallID)
			}
		}
	}
	require.True(t, sawAdded, "function_call output_item.added missing")
	require.True(t, sawArgsDone, "function_call_arguments.done missing")
	require.True(t, sawItemDone, "function_call output_item.done missing")
}

// TestStream_ToolCallArgumentsInFirstChunkNotDoubled guards the GLM/Zhipu shape
// where a single tool_call delta chunk carries id+name+arguments together.
// Earlier code copied the whole tool_call (including arguments) into state and
// then accumulated the same chunk's arguments again, producing a doubled,
// invalid JSON like {"cmd":"ls"}{"cmd":"ls"} that breaks Codex tool parsing
// ("trailing characters").
func TestStream_ToolCallArgumentsInFirstChunkNotDoubled(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	var argsDelta strings.Builder
	var sawArgsDone, sawItemDone bool
	for _, e := range events {
		switch e.Type {
		case "response.function_call_arguments.delta":
			_, _ = argsDelta.WriteString(e.Delta)
		case "response.function_call_arguments.done":
			sawArgsDone = true
			require.Equal(t, `{"cmd":"ls"}`, e.Arguments)
		case "response.output_item.done":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawItemDone = true
				require.Equal(t, `{"cmd":"ls"}`, e.Item.Arguments)
			}
		}
	}
	require.True(t, sawArgsDone, "function_call_arguments.done missing")
	require.True(t, sawItemDone, "function_call output_item.done missing")
	// Accumulated deltas must equal the final arguments exactly (no duplication).
	require.Equal(t, `{"cmd":"ls"}`, argsDelta.String())
}

func TestStream_InvalidToolArgumentsAreRejectedBeforeFinalize(t *testing.T) {
	idx := 0
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-flash")
	chunk := &ChatCompletionsChunk{
		Choices: []ChatChunkChoice{
			{
				Index: 0,
				Delta: ChatDelta{
					ToolCalls: []ChatToolCall{
						{
							Index: &idx,
							ID:    "call_bad",
							Type:  "function",
							Function: ChatFunctionCall{
								Name:      "exec_command",
								Arguments: `{"cmd": "ssh root@HOST`,
							},
						},
					},
				},
			},
		},
	}
	ChatCompletionsChunkToResponsesEvents(chunk, state)

	err := state.ValidateToolCallArguments()
	require.ErrorContains(t, err, "invalid JSON")
}

func TestStream_ValidToolCallAtOutputLimitKeepsIncompleteResponse(t *testing.T) {
	idx := 0
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-flash")
	chunk := &ChatCompletionsChunk{
		Choices: []ChatChunkChoice{
			{
				Index: 0,
				Delta: ChatDelta{
					ToolCalls: []ChatToolCall{
						{
							Index: &idx,
							ID:    "call_at_limit",
							Type:  "function",
							Function: ChatFunctionCall{
								Name:      "exec_command",
								Arguments: `{}`,
							},
						},
					},
				},
			},
		},
	}
	ChatCompletionsChunkToResponsesEvents(chunk, state)
	state.FinishReason = "length"

	require.NoError(t, state.ValidateToolCallArguments())
	events := FinalizeChatCompletionsResponsesStream(state)
	var sawArgsDone, sawIncomplete bool
	for _, event := range events {
		switch event.Type {
		case "response.function_call_arguments.done":
			sawArgsDone = true
			require.Equal(t, `{}`, event.Arguments)
		case "response.completed":
			require.NotNil(t, event.Response)
			sawIncomplete = event.Response.Status == "incomplete"
		}
	}
	require.True(t, sawArgsDone)
	require.True(t, sawIncomplete)
}

// TestStream_SSEWireComplete drives the full stream through SSE encoding and
// asserts the function_call events carry complete fields on the wire.
func TestStream_SSEWireComplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	var addedLine string
	for _, e := range events {
		sse, err := ResponsesEventToSSE(e)
		require.NoError(t, err)
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "function_call" {
			addedLine = sse
		}
	}
	require.NotEmpty(t, addedLine)
	// The function_call added event must carry arguments:"" on the wire.
	require.True(t, strings.Contains(addedLine, `"arguments":""`), "added line missing arguments: %s", addedLine)
	require.Contains(t, addedLine, `"call_id":"call_a"`)
}
