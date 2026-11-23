package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// A 400/404 that only says the probe model does not exist says nothing about
// the /v1/responses endpoint: the verdict must stay inconclusive and the
// account capability must not be overwritten with "unsupported".
func TestResponsesProbeModelUnavailableIsInconclusive(t *testing.T) {
	for i, body := range []string{
		`{"error":{"type":"model_not_found","message":"Model codex-auto-review is not supported by any configured account in this group"}}`,
		`{"error":{"code":"model_not_available"}}`,
		`{"error":{"message":"The model missing does not exist"}}`,
	} {
		require.False(t, responsesProbeVerdictIsConclusive(404, []byte(body)))
		require.True(t, decideResponsesProbeSupport(404, []byte(body)))
		updates := make(chan map[string]any, 1)
		account := Account{ID: int64(9600 + i), Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": "https://upstream.example"}}
		repo := &snapshotUpdateAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}, updateExtraCalls: updates}
		svc := &AccountTestService{accountRepo: repo, cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{responses: []*http.Response{
			newOpenAIProtocolProbeTestResponse(404, body),
			newOpenAIProtocolProbeTestResponse(404, body),
		}}}
		svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)
		select {
		case <-updates:
			t.Fatal("a model failure must not overwrite the account endpoint capability")
		default:
		}
	}
	// 只有可识别的 JSON endpoint-not-found 才是结论；HTML/空体/纯文本 404/405 保持未知。
	require.False(t, responsesProbeVerdictIsConclusive(404, []byte("404 page not found")))
	require.False(t, responsesProbeVerdictIsConclusive(404, []byte(`<html>not found</html>`)))
	require.False(t, responsesProbeVerdictIsConclusive(404, nil))
	require.False(t, responsesProbeVerdictIsConclusive(405, nil))
	require.True(t, responsesProbeVerdictIsConclusive(404, []byte(`{"error":{"message":"not found"}}`)))
	require.False(t, decideResponsesProbeSupport(404, []byte("404 page not found")))
	require.False(t, decideResponsesProbeSupport(405, nil))
}

func TestSelectResponsesProbeModelPrefersGeneralTextModel(t *testing.T) {
	account := &Account{Credentials: map[string]any{"model_mapping": map[string]any{
		"codex-auto-review": "codex-auto-review", "gpt-image-2": "gpt-image-2", "gpt-5.5": "gpt-5.5",
	}}}
	require.Equal(t, "gpt-5.5", selectResponsesProbeModel(account))
}
