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

func mistralChatReasoningAccount(baseURL string) *Account {
	account := forceChatResponsesFallbackAccount()
	account.ID = 211
	account.Name = "mistral-chat"
	account.Credentials = map[string]any{
		"api_key":  "sk-mistral-test",
		"base_url": baseURL,
	}
	return account
}

// requireAssistantReasoningFieldsAbsent 断言 messages 中不存在 assistant 的
// reasoning 回放字段，同时要求至少有一条 assistant 消息，避免空数组空转。
func requireAssistantReasoningFieldsAbsent(t *testing.T, body []byte) {
	t.Helper()
	assistantCount := 0
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		assistantCount++
		require.False(t, msg.Get("reasoning_content").Exists(), "reasoning_content 必须剥离: %s", body)
		require.False(t, msg.Get("reasoning").Exists(), "reasoning 必须剥离: %s", body)
		require.False(t, msg.Get("reasoning_details").Exists(), "reasoning_details 必须剥离: %s", body)
		require.False(t, msg.Get("thinking").Exists(), "thinking 必须剥离: %s", body)
	}
	require.Positive(t, assistantCount, "用例必须包含 assistant 消息")
}

func TestChatUpstreamRejectsReasoningFields(t *testing.T) {
	require.True(t, chatUpstreamRejectsReasoningFields("https://api.mistral.ai/v1/chat/completions"))
	require.True(t, chatUpstreamRejectsReasoningFields("https://API.MISTRAL.AI:443/v1/chat/completions"))
	require.True(t, chatUpstreamRejectsReasoningFields("https://api.mistral.ai./v1/chat/completions"))
	require.True(t, chatUpstreamRejectsReasoningFields("https://API.MISTRAL.AI.:443/v1/chat/completions"))
	require.False(t, chatUpstreamRejectsReasoningFields("https://api.mistral.ai.evil.example/v1/chat/completions"))
	require.False(t, chatUpstreamRejectsReasoningFields("https://mistral.ai/v1/chat/completions"))
	require.False(t, chatUpstreamRejectsReasoningFields("https://api.deepseek.com/v1/chat/completions"))
	require.False(t, chatUpstreamRejectsReasoningFields("http://upstream.example/v1/chat/completions"))
	require.False(t, chatUpstreamRejectsReasoningFields("not a url"))
	require.False(t, chatUpstreamRejectsReasoningFields(""))
}

func TestStripChatReasoningFields(t *testing.T) {
	t.Run("removes_assistant_fields_and_keeps_payload", func(t *testing.T) {
		body := []byte(`{"model":"mistral-large-latest","messages":[` +
			`{"role":"system","thinking":"system thinking","reasoning_details":[{"type":"reasoning.text","text":"system details"}],"content":"rules"},` +
			`{"role":"assistant","reasoning_content":"secret","reasoning":"secret alias","reasoning_details":[{"type":"reasoning.text","text":"secret details"}],"thinking":"secret thinking","content":"answer","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]},` +
			`{"role":"user","content":"hi"},` +
			`{"role":"user","reasoning_content":"keep for non-assistant","thinking":"keep thinking","reasoning_details":[{"type":"reasoning.text","text":"keep details"}]}]}`)

		got, err := stripChatReasoningFields(body)
		require.NoError(t, err)
		requireAssistantReasoningFieldsAbsent(t, got)
		require.Equal(t, "answer", gjson.GetBytes(got, "messages.1.content").String())
		require.Equal(t, "call_1", gjson.GetBytes(got, "messages.1.tool_calls.0.id").String())
		require.Equal(t, `{"city":"SF"}`, gjson.GetBytes(got, "messages.1.tool_calls.0.function.arguments").String())
		require.Equal(t, "rules", gjson.GetBytes(got, "messages.0.content").String())
		require.Equal(t, "system thinking", gjson.GetBytes(got, "messages.0.thinking").String())
		require.Equal(t, "system details", gjson.GetBytes(got, "messages.0.reasoning_details.0.text").String())
		require.Equal(t, "mistral-large-latest", gjson.GetBytes(got, "model").String())
		require.Equal(t, "keep for non-assistant", gjson.GetBytes(got, "messages.3.reasoning_content").String())
		require.Equal(t, "keep thinking", gjson.GetBytes(got, "messages.3.thinking").String())
		require.Equal(t, "keep details", gjson.GetBytes(got, "messages.3.reasoning_details.0.text").String())
	})

	unchanged := map[string]string{
		"invalid_json":                       `{"messages":[`,
		"root_array":                         `[{"role":"assistant","reasoning_content":"x"}]`,
		"json_null":                          `null`,
		"missing_messages":                   `{"model":"mistral-large-latest"}`,
		"messages_not_array":                 `{"messages":"oops","reasoning_content":"x"}`,
		"non_object_messages":                `{"messages":[null,"oops"]}`,
		"assistant_without_reasoning_fields": `{"messages":[{"role":"assistant","content":"ok"}]}`,
	}
	for name, body := range unchanged {
		t.Run(name, func(t *testing.T) {
			got, err := stripChatReasoningFields([]byte(body))
			require.NoError(t, err)
			require.Equal(t, body, string(got))
		})
	}
}

func TestStripChatReasoningFields_DropsEmptyAssistantAfterStrip(t *testing.T) {
	body := []byte(`{"model":"mistral-large-latest","messages":[` +
		`{"role":"system","content":"rules"},` +
		`{"role":"assistant","reasoning_content":"secret"},` +
		`{"role":"user","content":"hi"},` +
		`{"role":"assistant","reasoning_content":"secret","content":""},` +
		`{"role":"assistant","reasoning_content":"secret","content":[]},` +
		`{"role":"assistant","reasoning_content":"secret","tool_calls":[]},` +
		`{"role":"assistant","reasoning_content":"secret","content":"answer"},` +
		`{"role":"assistant","reasoning_content":"secret","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}` +
		`]}`)

	got, err := stripChatReasoningFields(body)
	require.NoError(t, err)

	messages := gjson.GetBytes(got, "messages").Array()
	require.Len(t, messages, 4, "empty assistant turns must be removed: %s", got)
	require.Equal(t, "system", messages[0].Get("role").String())
	require.Equal(t, "user", messages[1].Get("role").String())
	require.Equal(t, "answer", messages[2].Get("content").String())
	require.Equal(t, "call_1", messages[3].Get("tool_calls.0.id").String())
	requireAssistantReasoningFieldsAbsent(t, got)
	require.NotContains(t, string(got), "secret")
}

func TestStripChatReasoningFields_EmptyContentShapes(t *testing.T) {
	tests := []struct {
		name            string
		fields          string
		keep            bool
		wantRefusal     string
		wantContentType string
		wantContentText string
	}{
		{name: "missing content", fields: "", keep: false},
		{name: "empty string", fields: `,"content":""`, keep: false},
		{name: "whitespace string", fields: `,"content":"   "`, keep: false},
		{name: "null content", fields: `,"content":null`, keep: false},
		{name: "empty array", fields: `,"content":[]`, keep: false},
		{name: "whitespace text part", fields: `,"content":[{"type":"text","text":" "}]`, keep: false},
		{name: "non-empty text part", fields: `,"content":[{"type":"text","text":"answer"}]`, keep: true, wantContentType: "text", wantContentText: "answer"},
		{name: "image part", fields: `,"content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]`, keep: true, wantContentType: "image_url"},
		{name: "refusal only", fields: `,"refusal":"blocked"`, keep: true, wantRefusal: "blocked"},
		{name: "tool calls", fields: `,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]`, keep: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"mistral-large-latest","messages":[` +
				`{"role":"user","content":"hi"},` +
				`{"role":"assistant","reasoning_content":"secret"` + tt.fields + `}]}`)

			got, err := stripChatReasoningFields(body)
			require.NoError(t, err)

			messages := gjson.GetBytes(got, "messages").Array()
			if !tt.keep {
				require.Len(t, messages, 1, "empty assistant must be removed: %s", got)
				require.Equal(t, "user", messages[0].Get("role").String())
				require.NotContains(t, string(got), "secret")
				return
			}

			require.Len(t, messages, 2, "assistant with usable content must be kept: %s", got)
			require.Equal(t, "assistant", messages[1].Get("role").String())
			requireAssistantReasoningFieldsAbsent(t, got)
			if tt.wantRefusal != "" {
				require.Equal(t, tt.wantRefusal, messages[1].Get("refusal").String())
			}
			if tt.wantContentType != "" {
				require.Equal(t, tt.wantContentType, messages[1].Get("content.0.type").String())
			}
			if tt.wantContentText != "" {
				require.Equal(t, tt.wantContentText, messages[1].Get("content.0.text").String())
			}
		})
	}
}

func TestForwardResponses_MistralChatFallbackDropsReasoningOnlyAssistant(t *testing.T) {
	body := []byte(`{
		"model":"mistral-large-latest",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_plain","summary":[{"type":"summary_text","text":"plain thinking"}]},
			{"type":"message","role":"assistant","content":[]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]}
		]
	}`)
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_mistral_empty_assistant", deepSeekChatFallbackOKBody)
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	messages := gjson.GetBytes(upstream.lastBody, "messages").Array()
	require.Len(t, messages, 1, "reasoning-only assistant turn must not reach Mistral: %s", upstream.lastBody)
	require.Equal(t, "user", messages[0].Get("role").String())
	require.Equal(t, "go on", messages[0].Get("content").String())
	require.NotContains(t, string(upstream.lastBody), "plain thinking")
	require.NotContains(t, string(upstream.lastBody), `"role":"assistant"`)
}

func TestForwardAsRawChatCompletions_NonMistralKeepsReasoningFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"deepseek-v4-flash","messages":[` +
		`{"role":"assistant","reasoning_content":"a","reasoning":"b","reasoning_details":[{"type":"reasoning.text","text":"c"}],"thinking":"d","content":"answer"}],` +
		`"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamJSON := `{"id":"chatcmpl_non_mistral_reasoning","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_non_mistral_reasoning"}},
		Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
	require.NoError(t, err)
	require.Equal(t, string(body), string(upstream.lastBody), "非 Mistral 上游必须原样保留 reasoning 回放字段")
}

func TestForwardResponses_MistralChatFallbackStripsReasoningFields(t *testing.T) {
	body := []byte(`{
		"model":"mistral-large-latest",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_plain","summary":[{"type":"summary_text","text":"plain thinking"}]},
			{"type":"function_call","call_id":"call_0","name":"get_value","arguments":"{\"key\":\"a\"}"},
			{"type":"function_call_output","call_id":"call_0","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]}
		]
	}`)
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_mistral_plain", deepSeekChatFallbackOKBody)
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://api.mistral.ai/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, int64(len(upstream.lastBody)), upstream.lastReq.ContentLength, "剥离后 Content-Length 必须与实际 body 一致")

	require.Equal(t, "assistant", gjson.GetBytes(upstream.lastBody, "messages.0.role").String())
	requireAssistantReasoningFieldsAbsent(t, upstream.lastBody)
	require.NotContains(t, string(upstream.lastBody), "reasoning_content")
	require.Equal(t, "call_0", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
	require.Equal(t, `{"key":"a"}`, gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.function.arguments").String())
}

func TestForwardResponses_MistralChatFallbackStripsCachedReasoningOnToolTurn(t *testing.T) {
	body := []byte(`{
		"model":"mistral-large-latest",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_enc1","summary":[],"encrypted_content":"opaque"},
			{"type":"function_call","call_id":"call_1","name":"exec","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]}
		]
	}`)
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_mistral_cached", deepSeekChatFallbackOKBody)
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		cache:        &reasoningRecordingCache{getResp: map[string]string{"item_enc1": "cached thinking"}},
	}

	result, err := svc.Forward(context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	requireAssistantReasoningFieldsAbsent(t, upstream.lastBody)
	require.NotContains(t, string(upstream.lastBody), "cached thinking")
	require.Equal(t, "call_1", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
}

func TestForwardResponses_NonMistralChatFallbackKeepsReasoningFields(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_plain","summary":[{"type":"summary_text","text":"plain thinking"}]},
			{"type":"function_call","call_id":"call_0","name":"get_value","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_0","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]}
		]
	}`)
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_non_mistral_plain", deepSeekChatFallbackOKBody)
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	// 非 Mistral 上游（OpenAI 兼容），reasoning_content 回放必须保持原样。
	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "plain thinking", gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
	require.Equal(t, "call_0", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
}

func TestForwardAnthropicMessages_MistralChatFallbackStripsReasoningFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"mistral-large-latest",
		"max_tokens":128,
		"messages":[
			{"role":"user","content":"weather?"},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"need the weather tool","signature":"sig"},
				{"type":"text","text":"checking"},
				{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}
			]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny"}]}
		]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_mistral_messages"}},
		Body:       io.NopCloser(strings.NewReader(deepSeekChatFallbackOKBody)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.forwardAnthropicViaRawChatCompletions(
		context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body, "",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://api.mistral.ai/v1/chat/completions", upstream.lastReq.URL.String())
	requireAssistantReasoningFieldsAbsent(t, upstream.lastBody)
	require.NotContains(t, string(upstream.lastBody), "reasoning_content")
	require.Equal(t, "assistant", gjson.GetBytes(upstream.lastBody, "messages.1.role").String())
	require.Equal(t, "toolu_1", gjson.GetBytes(upstream.lastBody, "messages.1.tool_calls.0.id").String())
	require.Equal(t, `{"city":"SF"}`, gjson.GetBytes(upstream.lastBody, "messages.1.tool_calls.0.function.arguments").String())
}

// /v1/messages 的 Chat fallback 非流式路径必须与流式一致：reasoning-only
// 只产出 thinking 块，不能再把推理复制成可见 text 块。
func TestForwardAnthropicMessages_MistralReasoningOnlyHasNoVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"mistral-large-latest",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hi"}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_mistral_messages_reasoning_only"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_mistral_reasoning_only","object":"chat.completion","model":"mistral-large-latest","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"SECRET"}],"closed":true}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.forwardAnthropicViaRawChatCompletions(
		context.Background(), c, mistralChatReasoningAccount("https://api.mistral.ai"), body, "",
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	response := rec.Body.String()
	require.Equal(t, int64(1), gjson.Get(response, "content.#").Int())
	require.Equal(t, "thinking", gjson.Get(response, "content.0.type").String())
	require.Equal(t, "SECRET", gjson.Get(response, "content.0.thinking").String())
	require.False(t, gjson.Get(response, `content.#(type=="text")`).Exists(), "reasoning must not become visible text: %s", response)
	require.NotContains(t, response, `"text":"SECRET"`)
}
