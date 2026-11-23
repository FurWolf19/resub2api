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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func openAIProtocolFallbackAccount(responses, chat bool) *Account {
	return &Account{
		ID:          8801,
		Name:        "auto-protocol",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
		Extra: map[string]any{
			openai_compat.ExtraKeyProtocolCapabilities: map[string]any{
				openai_compat.ProtocolResponses:       map[string]any{"supported": responses},
				openai_compat.ProtocolChatCompletions: map[string]any{"supported": chat},
			},
		},
	}
}

func TestForwardAsChatCompletionsAutoFallsBackToResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"not found"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1794055887.0,\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\ndata: [DONE]\n\n",
		))},
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := openAIProtocolFallbackAccount(true, true)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/v1/chat/completions", upstream.requests[0].URL.Path)
	require.Equal(t, "/v1/responses", upstream.requests[1].URL.Path)
	require.Contains(t, rec.Body.String(), "Hello")
}

func TestForwardResponsesAutoFallsBackToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"not found"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_1","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		))},
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := openAIProtocolFallbackAccount(true, true)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
	require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
}

func TestEnsureOpenAIProtocolCapabilitiesBlocksOnFirstRequest(t *testing.T) {
	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          9911,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIProtocolProbeTestResponse(http.StatusNotFound, `{"error":{"message":"not found"}}`),
		newOpenAIProtocolProbeTestResponse(http.StatusOK, `{"output":[{"type":"function_call","name":"probe_ping"}]}`),
	}}
	svc := &OpenAIGatewayService{
		openAIProtocolCapabilityProbeEnabled: true,
		accountRepo:                          repo,
		httpUpstream:                         upstream,
		cfg:                                  rawChatCompletionsTestConfig(),
	}

	accountWithCaps := svc.ensureOpenAIProtocolCapabilities(context.Background(), &account)
	require.NotSame(t, &account, accountWithCaps)
	require.Len(t, upstream.requests, 2)

	caps := openai_compat.ResolveProtocolCapabilities(accountWithCaps.Extra)
	require.True(t, caps.Responses.Known)
	require.True(t, caps.Responses.Supported)
	require.True(t, caps.ChatCompletions.Known)
	require.False(t, caps.ChatCompletions.Supported)
	require.False(t, caps.NeedsRefresh(nowForProtocolCapabilityTest()))

	updates := <-updateCalls
	require.Equal(t, true, updates[openai_compat.ExtraKeyResponsesSupported])
	require.Contains(t, updates, openai_compat.ExtraKeyProtocolCapabilities)
}

func nowForProtocolCapabilityTest() time.Time { return time.Now() }
