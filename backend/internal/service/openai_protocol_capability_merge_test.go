//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestMergeOpenAIProtocolCapabilities(t *testing.T) {
	t0 := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	trueValue := true
	falseValue := false

	tests := []struct {
		name       string
		current    openai_compat.ProtocolCapabilities
		patches    []OpenAIProtocolCapabilityPatch
		wantResp   openai_compat.ProtocolCapability
		wantChat   openai_compat.ProtocolCapability
		wantLegacy *bool
		wantChange bool
		wantErr    bool
	}{
		{
			name: "two protocols are independent",
			patches: []OpenAIProtocolCapabilityPatch{
				{Protocol: openai_compat.ProtocolResponses, Supported: &falseValue, ProbedAt: t0},
				{Protocol: openai_compat.ProtocolChatCompletions, Supported: &trueValue, ProbedAt: t0},
			},
			wantResp:   openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t0},
			wantChat:   openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: t0},
			wantLegacy: &falseValue,
			wantChange: true,
		},
		{
			name: "newer conclusion wins",
			current: openai_compat.ProtocolCapabilities{
				Responses: openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t0},
			},
			patches:    []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &trueValue, ProbedAt: t1}},
			wantResp:   openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: t1},
			wantLegacy: &trueValue,
			wantChange: true,
		},
		{
			name: "older conclusion ignored",
			current: openai_compat.ProtocolCapabilities{
				Responses: openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t1},
			},
			patches:    []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &trueValue, ProbedAt: t0}},
			wantResp:   openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t1},
			wantLegacy: &falseValue,
			wantChange: false,
		},
		{
			name: "touch preserves supported",
			current: openai_compat.ProtocolCapabilities{
				Responses: openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t0},
			},
			patches:    []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, ProbedAt: t1}},
			wantResp:   openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t1},
			wantLegacy: &falseValue,
			wantChange: true,
		},
		{
			name: "same second older patch ignored",
			current: openai_compat.ProtocolCapabilities{
				Responses: openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t0.Add(500 * time.Millisecond)},
			},
			patches:    []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, Supported: &trueValue, ProbedAt: t0}},
			wantResp:   openai_compat.ProtocolCapability{Known: true, Supported: false, ProbedAt: t0.Add(500 * time.Millisecond)},
			wantLegacy: &falseValue,
			wantChange: false,
		},
		{
			name:       "touch on unknown protocol is ignored",
			patches:    []OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolChatCompletions, ProbedAt: t1}},
			wantChange: false,
		},
		{
			name:    "unknown protocol errors",
			patches: []OpenAIProtocolCapabilityPatch{{Protocol: "bogus", ProbedAt: t1}},
			wantErr: true,
		},
		{
			name: "legacy key nil when responses unknown",
			patches: []OpenAIProtocolCapabilityPatch{
				{Protocol: openai_compat.ProtocolChatCompletions, Supported: &trueValue, ProbedAt: t0},
			},
			wantChat:   openai_compat.ProtocolCapability{Known: true, Supported: true, ProbedAt: t0},
			wantChange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, legacy, changed, err := MergeOpenAIProtocolCapabilities(tt.current, tt.patches)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantResp, merged.Responses)
			require.Equal(t, tt.wantChat, merged.ChatCompletions)
			require.Equal(t, tt.wantChange, changed)
			if tt.wantLegacy == nil {
				require.Nil(t, legacy)
			} else {
				require.NotNil(t, legacy)
				require.Equal(t, *tt.wantLegacy, *legacy)
			}
		})
	}
}

func TestRecordOpenAIProtocolCapabilityBuildsSingleProtocolPatch(t *testing.T) {
	account := &Account{ID: 7701}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}},
		mergeCalls:            make(chan []OpenAIProtocolCapabilityPatch, 1),
	}
	svc := &OpenAIGatewayService{accountRepo: repo}

	svc.recordOpenAIProtocolCapability(context.Background(), account, openai_compat.ProtocolResponses, false)

	select {
	case patches := <-repo.mergeCalls:
		require.Len(t, patches, 1)
		require.Equal(t, openai_compat.ProtocolResponses, patches[0].Protocol)
		require.NotNil(t, patches[0].Supported)
		require.False(t, *patches[0].Supported)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a single protocol patch to be persisted")
	}
	waitForOpenAIProtocolPersistIdle(t, account.ID)
}

func TestProbeResultBuildsConclusiveAndTouchPatches(t *testing.T) {
	now := time.Now()
	account := &Account{
		ID: 7702,
		Extra: map[string]any{
			openai_compat.ExtraKeyProtocolCapabilities: map[string]any{
				openai_compat.ProtocolResponses:       map[string]any{"supported": false, "probed_at": now.Add(-time.Minute).UTC().Format(time.RFC3339)},
				openai_compat.ProtocolChatCompletions: map[string]any{"supported": true, "probed_at": now.Add(-time.Minute).UTC().Format(time.RFC3339)},
			},
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}},
		mergeCalls:            make(chan []OpenAIProtocolCapabilityPatch, 1),
	}
	deps := openAIProtocolProbeDeps{accountRepo: repo}

	persistOpenAIProtocolProbeResult(context.Background(), deps, account, openAIProtocolProbeResult{
		Chat:      openAIProtocolProbeOutcome{Conclusive: true, Supported: false},
		Responses: openAIProtocolProbeOutcome{Conclusive: false},
	})

	select {
	case patches := <-repo.mergeCalls:
		require.Len(t, patches, 2)
		byProtocol := make(map[string]OpenAIProtocolCapabilityPatch, len(patches))
		for _, patch := range patches {
			byProtocol[patch.Protocol] = patch
		}
		chat := byProtocol[openai_compat.ProtocolChatCompletions]
		require.NotNil(t, chat.Supported)
		require.False(t, *chat.Supported, "有结论的协议提交 supported")
		responses := byProtocol[openai_compat.ProtocolResponses]
		require.Nil(t, responses.Supported, "无结论但已有旧值的协议只提交 probed_at touch")
	case <-time.After(2 * time.Second):
		t.Fatal("expected probe patches to be persisted")
	}
}

func TestProbeResultSkipsUnknownInconclusiveProtocol(t *testing.T) {
	account := &Account{ID: 7703}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}},
		mergeCalls:            make(chan []OpenAIProtocolCapabilityPatch, 1),
	}
	deps := openAIProtocolProbeDeps{accountRepo: repo}

	persistOpenAIProtocolProbeResult(context.Background(), deps, account, openAIProtocolProbeResult{
		Chat:      openAIProtocolProbeOutcome{Conclusive: false},
		Responses: openAIProtocolProbeOutcome{Conclusive: true, Supported: true},
	})

	patches := <-repo.mergeCalls
	require.Len(t, patches, 1, "未知且无结论的协议不得凭空创建 supported=false")
	require.Equal(t, openai_compat.ProtocolResponses, patches[0].Protocol)
}
