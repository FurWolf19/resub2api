//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func openAIProtocolTestAccount(accountType string, mode string, responses, chat *bool) *Account {
	extra := map[string]any{}
	if mode != "" {
		extra[openai_compat.ExtraKeyResponsesMode] = mode
	}
	caps := map[string]any{}
	if responses != nil {
		caps[openai_compat.ProtocolResponses] = map[string]any{"supported": *responses}
	}
	if chat != nil {
		caps[openai_compat.ProtocolChatCompletions] = map[string]any{"supported": *chat}
	}
	if len(caps) > 0 {
		extra[openai_compat.ExtraKeyProtocolCapabilities] = caps
	}
	return &Account{
		Platform:    PlatformOpenAI,
		Type:        accountType,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       extra,
	}
}

func autoBoolPtr(v bool) *bool { return &v }

func TestResolveUpstreamProtocolAutoFollowsInbound(t *testing.T) {
	cases := []struct {
		name      string
		responses *bool
		chat      *bool
		inbound   string
		want      string
	}{
		{"responses inbound with only cc", autoBoolPtr(false), autoBoolPtr(true), APIProtocolResponses, APIProtocolChatCompletions},
		{"cc inbound with only responses", autoBoolPtr(true), autoBoolPtr(false), APIProtocolChatCompletions, APIProtocolResponses},
		{"responses inbound unknown with cc supported stays responses", nil, autoBoolPtr(true), APIProtocolResponses, APIProtocolResponses},
		{"cc inbound unknown with responses supported stays cc", autoBoolPtr(true), nil, APIProtocolChatCompletions, APIProtocolChatCompletions},
		{"both supported responses inbound", autoBoolPtr(true), autoBoolPtr(true), APIProtocolResponses, APIProtocolResponses},
		{"both supported cc inbound", autoBoolPtr(true), autoBoolPtr(true), APIProtocolChatCompletions, APIProtocolChatCompletions},
		{"unknown responses inbound stays responses", nil, nil, APIProtocolResponses, APIProtocolResponses},
		{"unknown cc inbound stays cc", nil, nil, APIProtocolChatCompletions, APIProtocolChatCompletions},
		{"both unsupported follows inbound responses", autoBoolPtr(false), autoBoolPtr(false), APIProtocolResponses, APIProtocolResponses},
		{"both unsupported follows inbound cc", autoBoolPtr(false), autoBoolPtr(false), APIProtocolChatCompletions, APIProtocolChatCompletions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := openAIProtocolTestAccount(AccountTypeAPIKey, "auto", tc.responses, tc.chat)
			require.Equal(t, tc.want, resolveUpstreamProtocol(account, tc.inbound, "", nil))
		})
	}
}

func TestResolveUpstreamProtocolAutoHonorsHardConstraints(t *testing.T) {
	account := openAIProtocolTestAccount(AccountTypeAPIKey, "auto", autoBoolPtr(false), autoBoolPtr(true))
	for _, model := range []string{"gpt-6.1-sol", "gpt-6.1-terra", "gpt-6.1-sol-high"} {
		t.Run(model, func(t *testing.T) {
			require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolChatCompletions, model, nil, true))
			require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolChatCompletions, model, nil, false))
		})
	}
}

func TestResolveUpstreamProtocolForceModesUnchanged(t *testing.T) {
	forceResponses := openAIProtocolTestAccount(AccountTypeAPIKey, string(openai_compat.ResponsesSupportModeForceResponses), autoBoolPtr(false), autoBoolPtr(true))
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(forceResponses, APIProtocolChatCompletions, "", nil))

	forceChat := openAIProtocolTestAccount(AccountTypeAPIKey, string(openai_compat.ResponsesSupportModeForceChatCompletions), autoBoolPtr(true), autoBoolPtr(true))
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(forceChat, APIProtocolResponses, "", nil))
}

func TestResolveUpstreamProtocolOAuthAndSetupTokenUnchanged(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		account := openAIProtocolTestAccount(accountType, "auto", autoBoolPtr(false), autoBoolPtr(true))
		require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "", nil), accountType)
		require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolChatCompletions, "", nil), accountType)
	}
}

func TestResolveUpstreamProtocolLegacyProbeKey(t *testing.T) {
	legacyUnsupported := &Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(legacyUnsupported, APIProtocolResponses, "", nil))

	// Legacy true 只说明 Responses 可用；Chat 入站仍跟随入站（未知/推断支持均
	// 不等于明确不支持），不再静默升级到 Responses。
	legacySupported := &Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: true},
	}
	require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(legacySupported, APIProtocolChatCompletions, "", nil))
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(legacySupported, APIProtocolResponses, "", nil))
}

func TestOpenAIProtocolRequiresResponsesForSolTerraTools(t *testing.T) {
	account := openAIProtocolTestAccount(AccountTypeAPIKey, "auto", autoBoolPtr(false), autoBoolPtr(true))
	withTools := []byte(`{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`)
	withoutTools := []byte(`{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hi"}]}`)
	for _, model := range []string{"gpt-6.1-sol", "gpt-6.1-terra", "gpt-6.1-sol-high"} {
		withTools = []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`)
		withoutTools = []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`)
		require.True(t, openAIProtocolRequiresResponses(nil, account, model, withTools, false), model)
		require.False(t, openAIProtocolRequiresResponses(nil, account, model, withoutTools, false), model)
		require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolChatCompletions, model, nil, true), model)
	}
}
