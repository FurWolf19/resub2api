//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestDuplicateAccountExtraDropsProtocolCapabilities(t *testing.T) {
	source := map[string]any{
		openai_compat.ExtraKeyProtocolCapabilities: map[string]any{
			openai_compat.ProtocolResponses: map[string]any{
				"supported": false,
				"probed_at": "2026-11-09T10:00:00Z",
			},
			openai_compat.ProtocolChatCompletions: map[string]any{
				"supported": true,
				"probed_at": "2026-11-09T10:00:00Z",
			},
		},
		openai_compat.ExtraKeyResponsesSupported: false,
		"custom":                                 "x",
	}

	cloned, err := duplicateAccountExtra(source)
	require.NoError(t, err)
	require.NotContains(t, cloned, openai_compat.ExtraKeyProtocolCapabilities)
	require.NotContains(t, cloned, openai_compat.ExtraKeyResponsesSupported)
	require.Equal(t, "x", cloned["custom"])
	require.False(t, openai_compat.ResolveProtocolCapabilities(cloned).HasKnown(), "复制账号必须按未探测处理")

	require.Contains(t, source, openai_compat.ExtraKeyProtocolCapabilities, "不得修改源 map")
}

func TestDuplicateAccountExtraKeepsUnrelatedOpenAIKeys(t *testing.T) {
	source := map[string]any{
		openai_compat.ExtraKeyProtocolCapabilities: map[string]any{
			openai_compat.ProtocolResponses: map[string]any{"supported": false},
		},
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeAuto),
		"ignored":                           "value",
	}

	cloned, err := duplicateAccountExtra(source)
	require.NoError(t, err)
	require.Equal(t, string(openai_compat.ResponsesSupportModeAuto), cloned[openai_compat.ExtraKeyResponsesMode])
	require.Equal(t, "value", cloned["ignored"])
	require.NotContains(t, cloned, openai_compat.ExtraKeyProtocolCapabilities)
}
