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

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const reasoningReplayTestUpstreamError = `{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`

func reasoningReplayTestAccount(platform, baseURL string) *Account {
	account := rawChatCompletionsTestAccount()
	account.Platform = platform
	account.Credentials["base_url"] = baseURL
	account.Credentials["api_protocol"] = APIProtocolResponses
	account.Extra = responsesOnlyProtocolExtra("")
	return account
}

func forwardChatForReasoningReplayTest(t *testing.T, account *Account, body []byte) *httpUpstreamRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(reasoningReplayTestUpstreamError)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	return upstream
}

func reasoningReplayTestChatBody() []byte {
	return []byte(`{"model":"deepseek-v4-flash","stream":false,"messages":[` +
		`{"role":"user","content":"first"},` +
		`{"role":"assistant","reasoning_content":"R3_SECRET","content":"VISIBLE","tool_calls":[{"id":"call_r3","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},` +
		`{"role":"tool","tool_call_id":"call_r3","content":"{\"value\":\"ok\"}"},` +
		`{"role":"user","content":"second"}` +
		`],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"key":{"type":"string"}}}}}]}`)
}

func TestForwardAsChatCompletions_DeepSeekNativeResponsesReplaysReasoningText(t *testing.T) {
	account := reasoningReplayTestAccount(PlatformDeepseek, "https://api.deepseek.com")
	upstream := forwardChatForReasoningReplayTest(t, account, reasoningReplayTestChatBody())

	require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/responses"), "upstream path = %s", upstream.lastReq.URL.Path)
	require.NotContains(t, upstream.lastReq.URL.Path, "/chat/completions")

	body := upstream.lastBody
	require.Equal(t, "reasoning", gjson.GetBytes(body, "input.1.type").String())
	require.Equal(t, "reasoning_text", gjson.GetBytes(body, "input.1.content.0.type").String())
	require.Equal(t, "R3_SECRET", gjson.GetBytes(body, "input.1.content.0.text").String())
	require.False(t, gjson.GetBytes(body, "input.1.summary").Exists())
	require.False(t, gjson.GetBytes(body, "input.1.encrypted_content").Exists())

	require.Equal(t, "message", gjson.GetBytes(body, "input.2.type").String())
	require.Equal(t, "assistant", gjson.GetBytes(body, "input.2.role").String())
	require.Equal(t, "VISIBLE", gjson.GetBytes(body, "input.2.content.0.text").String())
	require.Equal(t, "function_call", gjson.GetBytes(body, "input.3.type").String())
	require.Equal(t, "call_r3", gjson.GetBytes(body, "input.3.call_id").String())
	require.Equal(t, "lookup", gjson.GetBytes(body, "input.3.name").String())
	require.Equal(t, `{"key":"alpha"}`, gjson.GetBytes(body, "input.3.arguments").String())
	require.NotContains(t, string(body), "<thinking>")
}

func TestForwardAsChatCompletions_DeepSeekHostDetection(t *testing.T) {
	t.Run("openai platform pointed at deepseek host replays reasoning", func(t *testing.T) {
		account := reasoningReplayTestAccount(PlatformOpenAI, "https://api.deepseek.com")
		upstream := forwardChatForReasoningReplayTest(t, account, reasoningReplayTestChatBody())

		require.Equal(t, "reasoning_text", gjson.GetBytes(upstream.lastBody, "input.1.content.0.type").String())
		require.Equal(t, "R3_SECRET", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
	})

	t.Run("other host drops reasoning", func(t *testing.T) {
		account := reasoningReplayTestAccount(PlatformOpenAI, "https://example.com")
		upstream := forwardChatForReasoningReplayTest(t, account, reasoningReplayTestChatBody())

		require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="reasoning")`).Exists())
		require.NotContains(t, string(upstream.lastBody), "R3_SECRET")
	})
}

func TestForwardAsChatCompletions_OpenAIResponsesDropsPlaintextReasoning(t *testing.T) {
	account := reasoningReplayTestAccount(PlatformOpenAI, "https://api.openai.com")
	upstream := forwardChatForReasoningReplayTest(t, account, reasoningReplayTestChatBody())

	require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/responses"), "upstream path = %s", upstream.lastReq.URL.Path)
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="reasoning")`).Exists())
	require.NotContains(t, string(upstream.lastBody), "R3_SECRET")
	require.NotContains(t, string(upstream.lastBody), "<thinking>")
	require.Equal(t, "VISIBLE", gjson.GetBytes(upstream.lastBody, `input.#(role=="assistant").content.0.text`).String())
}

func TestForwardResponses_ReasoningOnlyBufferedHasNoMessageText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_reasoning_only_buffered"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_reasoning_only","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"","reasoning_content":"SECRET"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	response := rec.Body.String()
	require.Equal(t, int64(1), gjson.Get(response, "output.#").Int())
	require.Equal(t, "reasoning", gjson.Get(response, "output.0.type").String())
	require.Equal(t, "SECRET", gjson.Get(response, "output.0.summary.0.text").String())
	require.False(t, gjson.Get(response, `output.#(type=="message")`).Exists())
}

func TestForwardResponses_MistralThinkingArrayStreamStaysOutOfVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-1-flash-1017","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_mistral_array","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_array","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"SEC"}],"closed":true}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_array","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"RET"}],"closed":false}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_array","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{"content":[{"type":"text","text":"VISIBLE"}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_mistral_array","object":"chat.completion.chunk","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_mistral_array_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)

	response := rec.Body.String()
	require.Contains(t, response, "event: response.reasoning_summary_text.delta")
	require.Contains(t, response, `"delta":"SEC"`)
	require.Contains(t, response, `"delta":"RET"`)
	require.Contains(t, response, `"delta":"VISIBLE"`)
	require.NotContains(t, response, `"delta":"SECRET"`)
	require.Contains(t, response, `"text":"SECRET"`)
	require.Contains(t, response, "data: [DONE]")
}

func TestForwardResponses_MistralThinkingArrayBufferedStaysOutOfVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-1-flash-1017","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_mistral_array_buffered"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_mistral_array","object":"chat.completion","model":"deepseek-v4-1-flash-1017","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true},{"type":"text","text":"VISIBLE"}],"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	response := rec.Body.String()
	require.Equal(t, int64(3), gjson.Get(response, "output.#").Int())
	require.Equal(t, "reasoning", gjson.Get(response, "output.0.type").String())
	require.Equal(t, "SECRET", gjson.Get(response, "output.0.summary.0.text").String())
	require.Equal(t, "message", gjson.Get(response, "output.1.type").String())
	require.Equal(t, "VISIBLE", gjson.Get(response, "output.1.content.0.text").String())
	require.Equal(t, "function_call", gjson.Get(response, "output.2.type").String())
	require.Equal(t, `{"key":"alpha"}`, gjson.Get(response, "output.2.arguments").String())
	require.NotContains(t, response, `"content":"SECRET"`)
}

func TestChatCompletionsToResponsesForAccount_OpenCodeZenDeepSeekReplaysReasoning(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "deepseek-v4.1-flash",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: []byte(`"hi"`)},
			{Role: "assistant", Content: []byte(`[
				{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true},
				{"type":"text","text":"VISIBLE"}
			]`)},
		},
	}

	tests := []struct {
		name       string
		account    *Account
		model      string
		wantReason bool
	}{
		{
			name:       "opencode go deepseek uses text replay",
			account:    &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test", "base_url": DefaultOpenCodeGoBaseURL}},
			model:      "deepseek-v4.1-flash",
			wantReason: true,
		},
		{
			name:       "openai account pointed at opencode zen deepseek uses text replay",
			account:    &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test", "base_url": DefaultOpenCodeZenBaseURL}},
			model:      "deepseek-v4.1-flash",
			wantReason: true,
		},
		{
			name:       "opencode go prefixed deepseek uses text replay",
			account:    &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test", "base_url": DefaultOpenCodeGoBaseURL}},
			model:      "opencode-go/DeepSeek-V4-Pro",
			wantReason: true,
		},
		{
			name:       "opencode go non deepseek drops reasoning",
			account:    &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test", "base_url": DefaultOpenCodeGoBaseURL}},
			model:      "gpt-5.6-luna",
			wantReason: false,
		},
		{
			name:       "third party deepseek name drops reasoning",
			account:    &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com"}},
			model:      "deepseek-v4.1-flash",
			wantReason: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testReq := *req
			testReq.Model = tt.model
			out, err := chatCompletionsToResponsesForAccount(nil, tt.account, &testReq)
			require.NoError(t, err)

			input := string(out.Input)
			if tt.wantReason {
				require.Equal(t, "reasoning", gjson.Get(input, "1.type").String())
				require.Equal(t, "reasoning_text", gjson.Get(input, "1.content.0.type").String())
				require.Equal(t, "SECRET", gjson.Get(input, "1.content.0.text").String())
				require.Equal(t, "VISIBLE", gjson.Get(input, "2.content.0.text").String())
			} else {
				require.False(t, gjson.Get(input, `#(type=="reasoning")`).Exists())
				require.NotContains(t, input, "SECRET")
				require.Equal(t, "VISIBLE", gjson.Get(input, `#(role=="assistant").content.0.text`).String())
			}
		})
	}
}
