//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const mistralCCNormalizeThinkingAndText = `[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true},{"type":"text","text":"VISIBLE"}]`

func TestIsMistralChatUpstream(t *testing.T) {
	t.Parallel()

	require.True(t, isMistralChatUpstream("https://api.mistral.ai/v1/chat/completions"))
	require.True(t, isMistralChatUpstream("https://API.MISTRAL.AI:443/v1/chat/completions"))
	require.True(t, isMistralChatUpstream("https://api.mistral.ai./v1/chat/completions"))
	require.True(t, isMistralChatUpstream("https://API.MISTRAL.AI.:443/v1/chat/completions"))
	require.False(t, isMistralChatUpstream("https://api.mistral.ai.evil.example/v1/chat/completions"))
	require.False(t, isMistralChatUpstream("https://evil.api.mistral.ai/v1/chat/completions"))
	require.False(t, isMistralChatUpstream("https://mistral.ai/v1/chat/completions"))
	require.False(t, isMistralChatUpstream("https://api.deepseek.com/v1/chat/completions"))
	require.False(t, isMistralChatUpstream("not a url"))
	require.False(t, isMistralChatUpstream(""))
}

func TestApplyMistralRawChatCompletionsResponse(t *testing.T) {
	t.Parallel()

	t.Run("thinking and text with tool calls and usage", func(t *testing.T) {
		body := []byte(`{"id":"chatcmpl_1","object":"chat.completion","model":"deepseek-v4-1-flash-1017",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":` + mistralCCNormalizeThinkingAndText + `,` +
			`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]},` +
			`"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7},"response_error":null}`)

		got := applyMistralRawChatCompletionsResponse(body)

		require.Equal(t, gjson.String, gjson.GetBytes(got, "choices.0.message.content").Type)
		require.Equal(t, "VISIBLE", gjson.GetBytes(got, "choices.0.message.content").String())
		require.Equal(t, "SECRET", gjson.GetBytes(got, "choices.0.message.reasoning_content").String())
		require.Equal(t, "call_1", gjson.GetBytes(got, "choices.0.message.tool_calls.0.id").String())
		require.Equal(t, "get_weather", gjson.GetBytes(got, "choices.0.message.tool_calls.0.function.name").String())
		require.Equal(t, `{"city":"SF"}`, gjson.GetBytes(got, "choices.0.message.tool_calls.0.function.arguments").String())
		require.Equal(t, "tool_calls", gjson.GetBytes(got, "choices.0.finish_reason").String())
		require.Equal(t, int64(7), gjson.GetBytes(got, "usage.total_tokens").Int())
		require.True(t, gjson.GetBytes(got, "response_error").Exists())
		require.False(t, gjson.GetBytes(got, "choices.0.message.content").IsArray())
	})

	t.Run("text only array", func(t *testing.T) {
		body := []byte(`{"choices":[{"message":{"role":"assistant","content":[{"type":"text","text":"VISIBLE"}]}}]}`)
		got := applyMistralRawChatCompletionsResponse(body)
		require.Equal(t, "VISIBLE", gjson.GetBytes(got, "choices.0.message.content").String())
		require.False(t, gjson.GetBytes(got, "choices.0.message.reasoning_content").Exists())
	})

	t.Run("reasoning only keeps empty string content", func(t *testing.T) {
		body := []byte(`{"choices":[{"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true}]}}]}`)
		got := applyMistralRawChatCompletionsResponse(body)
		require.Equal(t, gjson.String, gjson.GetBytes(got, "choices.0.message.content").Type)
		require.Equal(t, "", gjson.GetBytes(got, "choices.0.message.content").String())
		require.Equal(t, "SECRET", gjson.GetBytes(got, "choices.0.message.reasoning_content").String())
	})

	t.Run("existing reasoning content is appended not overwritten", func(t *testing.T) {
		body := []byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":"EARLIER","content":` + mistralCCNormalizeThinkingAndText + `}}]}`)
		got := applyMistralRawChatCompletionsResponse(body)
		require.Equal(t, "EARLIER\nSECRET", gjson.GetBytes(got, "choices.0.message.reasoning_content").String())
		require.Equal(t, "VISIBLE", gjson.GetBytes(got, "choices.0.message.content").String())
	})

	t.Run("multiple choices normalized independently", func(t *testing.T) {
		body := []byte(`{"choices":[` +
			`{"message":{"role":"assistant","content":[{"type":"text","text":"A"}]}},` +
			`{"message":{"role":"assistant","content":"plain"}}]}`)
		got := applyMistralRawChatCompletionsResponse(body)
		require.Equal(t, "A", gjson.GetBytes(got, "choices.0.message.content").String())
		require.Equal(t, "plain", gjson.GetBytes(got, "choices.1.message.content").String())
	})

	unchanged := map[string]string{
		"invalid json":           `{"choices":[`,
		"string content":         `{"choices":[{"message":{"role":"assistant","content":"plain"}}]}`,
		"missing choices":        `{"id":"chatcmpl_1"}`,
		"choices not array":      `{"choices":"oops"}`,
		"message not object":     `{"choices":[{"message":"oops"}]}`,
		"root array":             `[{"choices":[]}]`,
		"invalid part container": `{"choices":[{"message":{"content":{"type":"text","text":"VISIBLE"}}}]}`,
	}
	for name, body := range unchanged {
		t.Run("unchanged "+name, func(t *testing.T) {
			require.Equal(t, body, string(applyMistralRawChatCompletionsResponse([]byte(body))))
		})
	}
}

func TestApplyMistralRawChatCompletionsSSELine(t *testing.T) {
	t.Parallel()

	t.Run("thinking and text delta preserves data prefix and newline", func(t *testing.T) {
		line := []byte(`data: {"id":"chatcmpl_1","choices":[{"index":0,"delta":{"role":"assistant","content":` + mistralCCNormalizeThinkingAndText + `},"finish_reason":null}]}` + "\r\n")
		got := applyMistralRawChatCompletionsSSELine(line)
		require.True(t, bytes.HasPrefix(got, []byte("data: ")))
		require.True(t, bytes.HasSuffix(got, []byte("\r\n")))

		payload := strings.TrimSpace(string(got))
		payload = strings.TrimPrefix(payload, "data: ")
		require.Equal(t, gjson.String, gjson.Get(payload, "choices.0.delta.content").Type)
		require.Equal(t, "VISIBLE", gjson.Get(payload, "choices.0.delta.content").String())
		require.Equal(t, "SECRET", gjson.Get(payload, "choices.0.delta.reasoning_content").String())
		require.Equal(t, "assistant", gjson.Get(payload, "choices.0.delta.role").String())
		require.True(t, gjson.Get(payload, "choices.0.finish_reason").Exists())
	})

	t.Run("data prefix without space stays without space", func(t *testing.T) {
		line := []byte(`data:{"choices":[{"delta":{"content":[{"type":"text","text":"VISIBLE"}]}}]}`)
		got := applyMistralRawChatCompletionsSSELine(line)
		require.True(t, bytes.HasPrefix(got, []byte("data:{")))
		require.Equal(t, "VISIBLE", gjson.GetBytes(got[len("data:"):], "choices.0.delta.content").String())
	})

	t.Run("reasoning only delta", func(t *testing.T) {
		line := []byte(`data: {"choices":[{"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true}]}}]}`)
		got := applyMistralRawChatCompletionsSSELine(line)
		require.Equal(t, "", gjson.GetBytes(got[len("data: "):], "choices.0.delta.content").String())
		require.Equal(t, "SECRET", gjson.GetBytes(got[len("data: "):], "choices.0.delta.reasoning_content").String())
	})

	t.Run("tool calls preserved", func(t *testing.T) {
		line := []byte(`data: {"choices":[{"index":0,"delta":{"content":[{"type":"text","text":"VISIBLE"}],` +
			`"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
		got := applyMistralRawChatCompletionsSSELine(line)
		payload := string(got[len("data: "):])
		require.Equal(t, "VISIBLE", gjson.Get(payload, "choices.0.delta.content").String())
		require.Equal(t, "call_1", gjson.Get(payload, "choices.0.delta.tool_calls.0.id").String())
		require.Equal(t, "get_weather", gjson.Get(payload, "choices.0.delta.tool_calls.0.function.name").String())
		require.Equal(t, "tool_calls", gjson.Get(payload, "choices.0.finish_reason").String())
	})

	t.Run("usage chunk preserved", func(t *testing.T) {
		line := `data: {"id":"chatcmpl_1","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`
		require.Equal(t, line, string(applyMistralRawChatCompletionsSSELine([]byte(line))))
	})

	unchanged := map[string]string{
		"empty line":     "",
		"blank data":     "data: ",
		"done":           "data: [DONE]",
		"comment":        ": keep-alive",
		"event line":     "event: message",
		"invalid json":   `data: {"choices":[`,
		"no data prefix": `{"choices":[{"delta":{"content":[{"type":"text","text":"VISIBLE"}]}}]}`,
		"string delta":   `data: {"choices":[{"delta":{"content":"VISIBLE"}}]}`,
	}
	for name, line := range unchanged {
		t.Run("unchanged "+name, func(t *testing.T) {
			require.Equal(t, line, string(applyMistralRawChatCompletionsSSELine([]byte(line))))
		})
	}
}

func TestForwardAsRawChatCompletions_MistralNormalizesNonStreamingContentArray(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-1-flash-1017","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamJSON := `{"id":"chatcmpl_mistral_raw","object":"chat.completion","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"message":{"role":"assistant","content":` + mistralCCNormalizeThinkingAndText + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_mistral_raw_json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://api.mistral.ai/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)

	downstream := rec.Body.String()
	require.Equal(t, "VISIBLE", gjson.Get(downstream, "choices.0.message.content").String())
	require.Equal(t, "SECRET", gjson.Get(downstream, "choices.0.message.reasoning_content").String())
	require.Equal(t, int64(7), gjson.Get(downstream, "usage.total_tokens").Int())
	require.NotContains(t, downstream, `"type":"thinking"`)
}

func TestForwardAsRawChatCompletions_MistralNormalizesStreamingContentArray(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-1-flash-1017","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_mistral_stream","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_stream","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_stream","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"content":[{"type":"text","text":"VISIBLE"}],"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_stream","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_mistral_raw_sse"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)

	downstream := rec.Body.String()
	require.Contains(t, downstream, `"reasoning_content":"SECRET"`)
	require.Contains(t, downstream, `"content":"VISIBLE"`)
	require.Contains(t, downstream, `"tool_calls"`)
	require.Contains(t, downstream, `"id":"call_1"`)
	require.Contains(t, downstream, `"role":"assistant"`)
	require.Contains(t, downstream, `"finish_reason":"tool_calls"`)
	require.Contains(t, downstream, `"usage"`)
	require.Contains(t, downstream, "data: [DONE]")
	require.NotContains(t, downstream, `"type":"thinking"`)

	for _, line := range strings.Split(downstream, "\n") {
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(payload)
		if trimmed == "" || trimmed == "[DONE]" {
			continue
		}
		content := gjson.Get(trimmed, "choices.0.delta.content")
		if content.Exists() {
			require.NotEqual(t, gjson.JSON, content.Type, "downstream delta.content must not stay an array: %s", trimmed)
		}
	}
}

func TestForwardAsRawChatCompletions_NonMistralKeepsContentArraysByteForByte(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("non streaming", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		upstreamJSON := `{"id":"chatcmpl_other","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":` + mistralCCNormalizeThinkingAndText + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_non_mistral_json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
		}}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

		result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, upstreamJSON, rec.Body.String())
	})

	t.Run("streaming", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		upstreamBody := strings.Join([]string{
			`data: {"id":"chatcmpl_other","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_other","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":` + mistralCCNormalizeThinkingAndText + `},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_other","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_non_mistral_sse"}},
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

		result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, upstreamBody, rec.Body.String())
	})
}
