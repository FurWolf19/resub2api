//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestForwardAsChatCompletionsAutoConvertsCursorResponsesShapeForCCUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_1","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		))},
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	// Responses 明确不支持 + Chat 支持 + auto 模式：最终选择 CC 上游。
	account := openAIProtocolFallbackAccount(false, true)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/v1/chat/completions", upstream.requests[0].URL.Path)
	require.NotEmpty(t, gjson.GetBytes(upstream.bodies[0], "messages").Array(), "CC upstream must receive messages")
	require.False(t, gjson.GetBytes(upstream.bodies[0], "input").Exists(), "CC upstream must not receive Responses input")
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

func TestOpenAIProtocolProbeEligibility(t *testing.T) {
	openAI := func(platform string, accountType string, credentials map[string]any, extra map[string]any) *Account {
		return &Account{Platform: platform, Type: accountType, Credentials: credentials, Extra: extra}
	}

	require.True(t, openAIProtocolProbeEligible(openAI(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"api_key": "sk"}, map[string]any{})))
	require.False(t, openAIProtocolProbeEligible(openAI(PlatformOpenAI, AccountTypeOAuth, map[string]any{"access_token": "tok"}, map[string]any{})), "OAuth must not probe")
	require.False(t, openAIProtocolProbeEligible(openAI(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"api_key": "sk"}, map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
	})), "force modes must not probe")
	require.False(t, openAIProtocolProbeEligible(openAI(PlatformOpenCodeGo, AccountTypeAPIKey, map[string]any{"api_key": "sk"}, map[string]any{})), "model-routed provider must not probe")
	require.False(t, openAIProtocolProbeEligible(openAI(PlatformDeepseek, AccountTypeAPIKey, map[string]any{"api_key": "sk"}, map[string]any{})), "CN provider must not probe")
	require.False(t, openAIProtocolProbeEligible(openAI(PlatformKimi, AccountTypeAPIKey, map[string]any{"api_key": "sk", "api_protocol": APIProtocolAdaptive}, map[string]any{})), "adaptive provider must not probe")
}

func TestEnsureOpenAIProtocolCapabilitiesSkipsIneligibleAccounts(t *testing.T) {
	for name, account := range map[string]*Account{
		"model-routed": {
			ID: 9921, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-test"},
		},
		"cn": {
			ID: 9922, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-test"},
		},
		"adaptive": {
			ID: 9923, Platform: PlatformKimi, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-test", "api_protocol": APIProtocolAdaptive},
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &snapshotUpdateAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}}}
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{
				openAIProtocolCapabilityProbeEnabled: true,
				accountRepo:                          repo,
				httpUpstream:                         upstream,
				cfg:                                  rawChatCompletionsTestConfig(),
			}
			got := svc.ensureOpenAIProtocolCapabilities(context.Background(), account)
			require.Same(t, account, got)
			require.Empty(t, upstream.requests)
		})
	}
}

func TestEnsureOpenAIProtocolCapabilitiesLegacyStampsProbedAtWithoutProbing(t *testing.T) {
	account := Account{
		ID: 9931, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: true},
	}
	updateCalls := make(chan map[string]any, 2)
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		openAIProtocolCapabilityProbeEnabled: true,
		accountRepo:                          repo,
		httpUpstream:                         upstream,
		cfg:                                  rawChatCompletionsTestConfig(),
	}

	got := svc.ensureOpenAIProtocolCapabilities(context.Background(), &account)
	require.NotSame(t, &account, got)
	require.Empty(t, upstream.requests, "legacy capabilities must not trigger a blocking probe")
	caps := openai_compat.ResolveProtocolCapabilities(got.Extra)
	require.False(t, caps.NeedsRefresh(time.Now()), "legacy capabilities must be stamped as fresh")

	updates := <-updateCalls
	persisted := openai_compat.ResolveProtocolCapabilities(updates)
	require.False(t, persisted.NeedsRefresh(time.Now()), "probed_at must be persisted")
	require.True(t, persisted.Supports(openai_compat.ProtocolResponses))
	require.True(t, persisted.Supports(openai_compat.ProtocolChatCompletions))
}

func TestEnsureOpenAIProtocolCapabilitiesInconclusiveProbeUsesNegativeCache(t *testing.T) {
	account := Account{
		ID: 9941, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
	}
	repo := &snapshotUpdateAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	// 两个端点都返回“模型不可用”的 404：有 HTTP 响应但无法下结论。
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIProtocolProbeTestResponse(http.StatusNotFound, `{"error":{"code":"model_not_found"}}`),
		newOpenAIProtocolProbeTestResponse(http.StatusNotFound, `{"error":{"code":"model_not_found"}}`),
	}}
	svc := &OpenAIGatewayService{
		openAIProtocolCapabilityProbeEnabled: true,
		accountRepo:                          repo,
		httpUpstream:                         upstream,
		cfg:                                  rawChatCompletionsTestConfig(),
	}

	got := svc.ensureOpenAIProtocolCapabilities(context.Background(), &account)
	require.Same(t, &account, got)
	require.Len(t, upstream.requests, 2, "first request probes both protocols")

	got = svc.ensureOpenAIProtocolCapabilities(context.Background(), &account)
	require.Same(t, &account, got)
	require.Len(t, upstream.requests, 2, "inconclusive result must not be re-probed during backoff")
}

func TestForwardAsChatCompletionsForceChatCompletionsConvertsCursorResponsesShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_1","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		))},
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/v1/chat/completions", upstream.requests[0].URL.Path)
	require.NotEmpty(t, gjson.GetBytes(upstream.bodies[0], "messages").Array(), "force_chat_completions 的 CC 上游也必须收到 messages")
	require.False(t, gjson.GetBytes(upstream.bodies[0], "input").Exists(), "Responses 形状的 input 不得原样发给 CC 上游")
}

func waitForOpenAIProtocolPersistIdle(t *testing.T, accountID int64) {
	t.Helper()
	require.Eventually(t, func() bool {
		// 只读查询，避免测试在队列已被清理后又创造出新的空闲条目。
		openAIProtocolPersistQueuesMu.Lock()
		queue := openAIProtocolPersistQueues[accountID]
		openAIProtocolPersistQueuesMu.Unlock()
		if queue == nil {
			return true
		}
		queue.mu.Lock()
		defer queue.mu.Unlock()
		return !queue.running && queue.pending == nil
	}, 2*time.Second, 5*time.Millisecond)
}

func openAIProtocolPersistQueueExists(accountID int64) bool {
	openAIProtocolPersistQueuesMu.Lock()
	defer openAIProtocolPersistQueuesMu.Unlock()
	_, ok := openAIProtocolPersistQueues[accountID]
	return ok
}

func TestForwardAsChatCompletionsRetryRecordsActualResponsesCapability(t *testing.T) {
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
	account := openAIProtocolFallbackAccount(true, true)
	// Responses 未知、Chat 已知支持：首次按入站走 Chat，404 后重试切到 Responses。
	account.Extra[openai_compat.ExtraKeyProtocolCapabilities] = map[string]any{
		openai_compat.ProtocolChatCompletions: map[string]any{"supported": true},
	}
	updateCalls := make(chan map[string]any, 4)
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}},
		updateExtraCalls:      updateCalls,
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, accountRepo: repo}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/v1/chat/completions", upstream.requests[0].URL.Path)
	require.Equal(t, "/v1/responses", upstream.requests[1].URL.Path)
	require.Equal(t, "/v1/responses", result.UpstreamEndpoint)

	var persisted map[string]any
	deadline := time.After(2 * time.Second)
	for persisted == nil {
		select {
		case updates := <-updateCalls:
			caps := openai_compat.ResolveProtocolCapabilities(updates)
			if caps.Responses.Known && caps.Responses.Supported {
				persisted = updates
			}
		case <-deadline:
			t.Fatal("timed out waiting for persisted protocol capabilities")
		}
	}
	caps := openai_compat.ResolveProtocolCapabilities(persisted)
	require.True(t, caps.Responses.Known)
	require.True(t, caps.Responses.Supported, "重试实际命中 Responses，必须记录 Responses=true")
	require.True(t, caps.ChatCompletions.Known)
	require.False(t, caps.ChatCompletions.Supported, "Chat 的 404 结论不得被重试成功覆盖")
	waitForOpenAIProtocolPersistIdle(t, account.ID)
}

func TestOpenAIProtocolProbeOneConclusiveOneInconclusiveBacksOff(t *testing.T) {
	const accountID int64 = 9951
	reset := func() {
		openAIProtocolProbeBackoffMu.Lock()
		delete(openAIProtocolProbeBackoffTill, accountID)
		openAIProtocolProbeBackoffMu.Unlock()
	}
	reset()
	t.Cleanup(reset)

	account := Account{
		ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      make(chan map[string]any, 2),
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		// Chat：普通 404 → 有结论（不支持）。
		newOpenAIProtocolProbeTestResponse(http.StatusNotFound, `{"error":{"message":"not found"}}`),
		// Responses：模型不可用 400 → 无结论。
		newOpenAIProtocolProbeTestResponse(http.StatusBadRequest, `{"error":{"code":"model_not_found","message":"The model does not exist"}}`),
	}}
	deps := openAIProtocolProbeDeps{
		accountRepo:     repo,
		httpUpstream:    upstream,
		cfg:             rawChatCompletionsTestConfig(),
		validateBaseURL: func(raw string) (string, error) { return raw, nil },
	}

	result := probeOpenAIProtocolCapabilities(context.Background(), deps, &account, 2*time.Second)
	require.True(t, result.Chat.Conclusive)
	require.False(t, result.Chat.Supported)
	require.False(t, result.Responses.Conclusive)
	require.True(t, openAIProtocolProbeInBackoff(accountID, time.Now()), "一协议有结论、另一协议 401/模型不可用也必须短退避")
}

type blockingProtocolUpdateAccountRepo struct {
	stubOpenAIAccountRepo
	started chan struct{}
	release chan struct{}

	mu        sync.Mutex
	active    int
	maxActive int
	calls     int
}

func (r *blockingProtocolUpdateAccountRepo) MergeOpenAIProtocolCapabilities(ctx context.Context, _ int64, _ []OpenAIProtocolCapabilityPatch) error {
	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()

	select {
	case r.started <- struct{}{}:
	default:
	}
	select {
	case <-r.release:
	case <-ctx.Done():
	}

	r.mu.Lock()
	r.active--
	r.calls++
	r.mu.Unlock()
	return nil
}

func (r *blockingProtocolUpdateAccountRepo) snapshot() (active, maxActive, calls int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active, r.maxActive, r.calls
}

func protocolCapabilityPatches(caps openai_compat.ProtocolCapabilities) []OpenAIProtocolCapabilityPatch {
	patches := make([]OpenAIProtocolCapabilityPatch, 0, 2)
	if caps.Responses.Known {
		supported := caps.Responses.Supported
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolResponses, Supported: &supported, ProbedAt: caps.Responses.ProbedAt,
		})
	}
	if caps.ChatCompletions.Known {
		supported := caps.ChatCompletions.Supported
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolChatCompletions, Supported: &supported, ProbedAt: caps.ChatCompletions.ProbedAt,
		})
	}
	return patches
}

func TestPersistOpenAIProtocolCapabilitiesDoesNotBlockRequestPath(t *testing.T) {
	repo := &blockingProtocolUpdateAccountRepo{
		started: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(repo.release) }) }
	defer releaseAll()

	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 9961}
	now := time.Now()
	first := openai_compat.ProtocolCapabilities{
		Responses:       openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: now},
		ChatCompletions: openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: now},
	}
	second := openai_compat.ProtocolCapabilities{
		Responses:       openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: now.Add(time.Second)},
		ChatCompletions: openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: now.Add(time.Second)},
	}

	returned := make(chan struct{})
	go func() {
		svc.persistOpenAIProtocolCapabilityPatches(context.Background(), account.ID, protocolCapabilityPatches(first), first.Responses.ProbedAt)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("persist must return without waiting for the DB write")
	}
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("background writer never started")
	}

	// 第一次 DB 写仍被阻塞时，第二次投递也必须立即返回，不能串行等待。
	returned = make(chan struct{})
	go func() {
		svc.persistOpenAIProtocolCapabilityPatches(context.Background(), account.ID, protocolCapabilityPatches(second), second.Responses.ProbedAt)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second persist must not block on the in-flight DB write")
	}

	releaseAll()
	require.Eventually(t, func() bool {
		_, _, calls := repo.snapshot()
		return calls >= 2
	}, 2*time.Second, 10*time.Millisecond, "both snapshots must be written")
	_, maxActive, _ := repo.snapshot()
	require.Equal(t, 1, maxActive, "同一账号的 DB 写必须串行")
	waitForOpenAIProtocolPersistIdle(t, account.ID)
}

func TestOpenAIProtocolPersistQueueReleasedAfterDrain(t *testing.T) {
	const accountID int64 = 9971
	openAIProtocolPersistQueuesMu.Lock()
	delete(openAIProtocolPersistQueues, accountID)
	openAIProtocolPersistQueuesMu.Unlock()
	t.Cleanup(func() {
		openAIProtocolPersistQueuesMu.Lock()
		delete(openAIProtocolPersistQueues, accountID)
		openAIProtocolPersistQueuesMu.Unlock()
	})

	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 4)}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: accountID}
	caps := openai_compat.ProtocolCapabilities{
		Responses:       openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: time.Now()},
		ChatCompletions: openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: time.Now()},
	}

	svc.persistOpenAIProtocolCapabilityPatches(context.Background(), account.ID, protocolCapabilityPatches(caps), caps.Responses.ProbedAt)
	select {
	case <-repo.updateExtraCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("background writer never persisted the snapshot")
	}
	require.Eventually(t, func() bool { return !openAIProtocolPersistQueueExists(accountID) },
		2*time.Second, 5*time.Millisecond, "idle queue must be removed after the writer drains")

	// 清理后再次写入必须能重新创建队列并正常落库。
	caps.Responses.ProbedAt = caps.Responses.ProbedAt.Add(time.Second)
	svc.persistOpenAIProtocolCapabilityPatches(context.Background(), account.ID, protocolCapabilityPatches(caps), caps.Responses.ProbedAt)
	select {
	case <-repo.updateExtraCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("recreated queue must persist the new snapshot")
	}
	require.Eventually(t, func() bool { return !openAIProtocolPersistQueueExists(accountID) },
		2*time.Second, 5*time.Millisecond, "recreated idle queue must be removed again")
}

func TestOpenAIProtocolPersistQueueKeepsOlderOtherProtocolPatch(t *testing.T) {
	const accountID int64 = 9973
	openAIProtocolPersistQueuesMu.Lock()
	delete(openAIProtocolPersistQueues, accountID)
	openAIProtocolPersistQueuesMu.Unlock()
	t.Cleanup(func() {
		openAIProtocolPersistQueuesMu.Lock()
		delete(openAIProtocolPersistQueues, accountID)
		openAIProtocolPersistQueuesMu.Unlock()
	})

	now := time.Now()
	chatSupported := true
	queue, start := enqueueOpenAIProtocolPersistJob(accountID, openAIProtocolPersistJob{
		patches: []OpenAIProtocolCapabilityPatch{{
			Protocol: openai_compat.ProtocolChatCompletions, Supported: &chatSupported, ProbedAt: now,
		}},
		probedAt: now,
	})
	require.True(t, start)
	require.NotNil(t, queue.take(), "writer 取走 chat 结论")

	// writer 仍在处理 chat 时，较旧的 responses patch 不能被整包丢弃。
	responsesSupported := false
	queue, start = enqueueOpenAIProtocolPersistJob(accountID, openAIProtocolPersistJob{
		patches: []OpenAIProtocolCapabilityPatch{{
			Protocol: openai_compat.ProtocolResponses, Supported: &responsesSupported, ProbedAt: now.Add(-time.Minute),
		}},
		probedAt: now.Add(-time.Minute),
	})
	require.False(t, start, "writer is already running")

	pending := queue.take()
	require.NotNil(t, pending, "旧 job 的其它协议 patch 必须保留给下一次合并")
	require.Len(t, pending.patches, 1)
	require.Equal(t, openai_compat.ProtocolResponses, pending.patches[0].Protocol)
}

func TestOpenAIProtocolPersistQueueTouchKeepsConclusion(t *testing.T) {
	now := time.Now()
	queue := &openAIProtocolPersistQueue{running: true}
	supported := false
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &supported, ProbedAt: now}},
		probedAt: now,
	})
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, ProbedAt: now.Add(time.Second)}},
		probedAt: now.Add(time.Second),
	})

	pending := queue.take()
	require.NotNil(t, pending)
	require.Len(t, pending.patches, 2, "结论与 touch 必须分开提交")
	require.NotNil(t, pending.patches[0].Supported, "touch 不得抹掉同协议的既有结论")
	require.False(t, *pending.patches[0].Supported)
	require.Equal(t, now, pending.patches[0].ProbedAt, "结论必须保留自己的 probed_at，不能被 touch 时间提升")
	require.Nil(t, pending.patches[1].Supported)
	require.Equal(t, now.Add(time.Second), pending.patches[1].ProbedAt, "touch 只刷新 probed_at")
}

func TestQueuedTouchDoesNotPromoteOldConclusionOverNewerDBValue(t *testing.T) {
	t1 := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	t15 := t1.Add(30 * time.Second)
	t2 := t1.Add(time.Minute)
	queue := &openAIProtocolPersistQueue{running: true}
	supported := false
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &supported, ProbedAt: t1}},
		probedAt: t1,
	})
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, ProbedAt: t2}},
		probedAt: t2,
	})
	pending := queue.take()
	require.NotNil(t, pending)

	// 其它实例已写入 true@t1.5；本实例队列里是 false@t1 + touch@t2。
	current := openai_compat.ProtocolCapabilities{
		Responses: openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: t15},
	}
	merged, legacy, _, err := MergeOpenAIProtocolCapabilities(current, pending.patches)
	require.NoError(t, err)
	require.True(t, merged.Responses.Supported, "旧结论不得被 touch 提升后覆盖其它实例的新结论")
	require.Equal(t, t2, merged.Responses.ProbedAt, "touch 仍应刷新 probed_at")
	require.NotNil(t, legacy)
	require.True(t, *legacy)
}

func TestOpenAIProtocolPersistQueueKeepsDifferentProtocolsWhenPending(t *testing.T) {
	now := time.Now()
	queue := &openAIProtocolPersistQueue{running: true}
	chatSupported := true
	responsesSupported := false
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolChatCompletions, Supported: &chatSupported, ProbedAt: now}},
		probedAt: now,
	})
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &responsesSupported, ProbedAt: now.Add(time.Second)}},
		probedAt: now.Add(time.Second),
	})

	pending := queue.take()
	require.NotNil(t, pending)
	require.Len(t, pending.patches, 2, "不同协议必须同时保留")
	protocols := map[string]bool{}
	for _, patch := range pending.patches {
		protocols[patch.Protocol] = true
	}
	require.True(t, protocols[openai_compat.ProtocolChatCompletions])
	require.True(t, protocols[openai_compat.ProtocolResponses])
}

func TestOpenAIProtocolPersistQueueCoalescesSameProtocolByProbedAt(t *testing.T) {
	now := time.Now()
	queue := &openAIProtocolPersistQueue{running: true}
	older := false
	newer := true
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &older, ProbedAt: now}},
		probedAt: now,
	})
	queue.enqueue(openAIProtocolPersistJob{
		patches:  []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &newer, ProbedAt: now.Add(time.Second)}},
		probedAt: now.Add(time.Second),
	})

	pending := queue.take()
	require.NotNil(t, pending)
	require.Len(t, pending.patches, 1)
	require.NotNil(t, pending.patches[0].Supported)
	require.True(t, *pending.patches[0].Supported, "同协议应保留 probed_at 更新的结论")
}

func TestOpenAIProtocolPersistQueueReleaseKeepsPendingWork(t *testing.T) {
	const accountID int64 = 9972
	openAIProtocolPersistQueuesMu.Lock()
	delete(openAIProtocolPersistQueues, accountID)
	openAIProtocolPersistQueuesMu.Unlock()
	t.Cleanup(func() {
		openAIProtocolPersistQueuesMu.Lock()
		delete(openAIProtocolPersistQueues, accountID)
		openAIProtocolPersistQueuesMu.Unlock()
	})

	now := time.Now()
	patches := []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, ProbedAt: now}}
	queue, start := enqueueOpenAIProtocolPersistJob(accountID, openAIProtocolPersistJob{
		patches:  patches,
		probedAt: now,
	})
	require.True(t, start)
	require.NotNil(t, queue.take(), "first job must be taken by the writer")

	// writer 正在写第一条、且已有第二条待写时，不得清理。
	queue, start = enqueueOpenAIProtocolPersistJob(accountID, openAIProtocolPersistJob{
		patches:  patches,
		probedAt: now.Add(time.Second),
	})
	require.False(t, start, "writer is already running")
	openAIProtocolPersistQueueRelease(accountID, queue)
	require.True(t, openAIProtocolPersistQueueExists(accountID), "queue with pending work must not be removed")

	require.NotNil(t, queue.take(), "pending job must still be written")
	require.Nil(t, queue.take())
	openAIProtocolPersistQueueRelease(accountID, queue)
	require.False(t, openAIProtocolPersistQueueExists(accountID), "drained queue must be removed")

	// 清理后再次写入会创建新队列，而不是复用已退出的旧队列。
	recreated, start := enqueueOpenAIProtocolPersistJob(accountID, openAIProtocolPersistJob{
		patches:  patches,
		probedAt: now.Add(2 * time.Second),
	})
	require.True(t, start)
	require.NotSame(t, queue, recreated)
	require.True(t, openAIProtocolPersistQueueExists(accountID))
	require.NotNil(t, recreated.take())
	require.Nil(t, recreated.take())
	openAIProtocolPersistQueueRelease(accountID, recreated)
	require.False(t, openAIProtocolPersistQueueExists(accountID))
}
