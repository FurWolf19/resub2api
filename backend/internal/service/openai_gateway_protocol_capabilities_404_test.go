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

func newFallback404TestService(t *testing.T, account *Account, firstStatus int, firstBody string, contentType string) (*OpenAIGatewayService, *snapshotUpdateAccountRepo, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: firstStatus, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(firstBody))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_1","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		))},
	}}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}},
		mergeCalls:            make(chan []OpenAIProtocolCapabilityPatch, 8),
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, accountRepo: repo}
	return svc, repo, c
}

func collectProtocolPatches(t *testing.T, repo *snapshotUpdateAccountRepo) []OpenAIProtocolCapabilityPatch {
	t.Helper()
	var out []OpenAIProtocolCapabilityPatch
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case patches := <-repo.mergeCalls:
			out = append(out, patches...)
		case <-deadline:
			return out
		}
	}
}

func assertNoUnsupportedResponsesPatch(t *testing.T, patches []OpenAIProtocolCapabilityPatch) {
	t.Helper()
	for _, patch := range patches {
		if patch.Protocol == openai_compat.ProtocolResponses && patch.Supported != nil && !*patch.Supported {
			t.Fatalf("cannot persist responses supported=false without JSON endpoint-not-found evidence: %+v", patch)
		}
	}
}

func TestHTML404DoesNotRecordUnsupportedButStillFallsBack(t *testing.T) {
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
	svc, repo, c := newFallback404TestService(t, account, http.StatusNotFound, `<html><body>not found</body></html>`, "text/html")

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err, "HTML 404 仍应 fallback 到 Chat")
	require.NotNil(t, result)

	patches := collectProtocolPatches(t, repo)
	assertNoUnsupportedResponsesPatch(t, patches)
}

func TestEmpty404DoesNotRecordUnsupported(t *testing.T) {
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
	svc, repo, c := newFallback404TestService(t, account, http.StatusNotFound, "", "text/plain")

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	assertNoUnsupportedResponsesPatch(t, collectProtocolPatches(t, repo))
}

func TestPlainText404DoesNotRecordUnsupported(t *testing.T) {
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
	svc, repo, c := newFallback404TestService(t, account, http.StatusNotFound, "404 page not found", "text/plain")

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	assertNoUnsupportedResponsesPatch(t, collectProtocolPatches(t, repo))
}

func TestKnownJSONEndpoint404RecordsUnsupported(t *testing.T) {
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
	svc, repo, c := newFallback404TestService(t, account, http.StatusNotFound,
		`{"error":{"code":"endpoint_not_found"}}`, "application/json")

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	patches := collectProtocolPatches(t, repo)
	found := false
	for _, patch := range patches {
		if patch.Protocol == openai_compat.ProtocolResponses && patch.Supported != nil && !*patch.Supported {
			found = true
		}
	}
	require.True(t, found, "可识别的 JSON endpoint 404 必须记录 Responses=false")
}

func TestModelNameNotFound404DoesNotRecordUnsupported(t *testing.T) {
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
	svc, repo, c := newFallback404TestService(t, account, http.StatusNotFound,
		`{"error":{"message":"Model gpt-4o not found"}}`, "application/json")

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err, "模型不存在仍可 fallback，但不得记录协议不支持")
	assertNoUnsupportedResponsesPatch(t, collectProtocolPatches(t, repo))
}

func TestModelNotFound404DoesNotRecordUnsupported(t *testing.T) {
	account := openAIProtocolFallbackAccount(true, true)
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
	svc, repo, c := newFallback404TestService(t, account, http.StatusNotFound,
		`{"error":{"code":"model_not_found"}}`, "application/json")

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err, "模型不可用不触发协议 fallback")
	require.Empty(t, collectProtocolPatches(t, repo), "模型不可用不得写协议结论")
}
