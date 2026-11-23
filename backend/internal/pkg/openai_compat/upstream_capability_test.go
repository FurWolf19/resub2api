package openai_compat

import (
	"testing"
	"time"
)

func TestResolveResponsesSupport(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  AccountResponsesSupport
	}{
		{"nil extra", nil, ResponsesSupportUnknown},
		{"empty extra", map[string]any{}, ResponsesSupportUnknown},
		{"key missing", map[string]any{"other": "value"}, ResponsesSupportUnknown},
		{"value true", map[string]any{ExtraKeyResponsesSupported: true}, ResponsesSupportYes},
		{"value false", map[string]any{ExtraKeyResponsesSupported: false}, ResponsesSupportNo},
		{"value wrong type string", map[string]any{ExtraKeyResponsesSupported: "true"}, ResponsesSupportUnknown},
		{"value wrong type number", map[string]any{ExtraKeyResponsesSupported: 1}, ResponsesSupportUnknown},
		{"value nil", map[string]any{ExtraKeyResponsesSupported: nil}, ResponsesSupportUnknown},
		{"force responses", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses)}, ResponsesSupportYes},
		{"force chat completions", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions)}, ResponsesSupportNo},
		{"auto follows probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto), ExtraKeyResponsesSupported: false}, ResponsesSupportNo},
		{"invalid mode follows probe", map[string]any{ExtraKeyResponsesMode: "bogus", ExtraKeyResponsesSupported: true}, ResponsesSupportYes},
		{"force responses overrides probe false", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: false}, ResponsesSupportYes},
		{"force chat completions overrides probe true", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: true}, ResponsesSupportNo},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveResponsesSupport(tc.extra)
			if got != tc.want {
				t.Errorf("ResolveResponsesSupport(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}

func TestShouldUseResponsesAPI(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		// 关键不变量：未探测必须返回 true（保留旧行为）
		{"unknown defaults to true (preserve old behavior)", nil, true},
		{"unknown empty defaults to true", map[string]any{}, true},
		{"unknown wrong type defaults to true", map[string]any{ExtraKeyResponsesSupported: "yes"}, true},

		// 已探测：标记决定
		{"explicitly supported", map[string]any{ExtraKeyResponsesSupported: true}, true},
		{"explicitly unsupported", map[string]any{ExtraKeyResponsesSupported: false}, false},

		// 手动覆盖：覆盖自动探测结果
		{"force responses overrides unsupported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: false}, true},
		{"force chat completions overrides supported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: true}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldUseResponsesAPI(tc.extra)
			if got != tc.want {
				t.Errorf("ShouldUseResponsesAPI(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}

func TestNormalizeResponsesSupportMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want ResponsesSupportMode
	}{
		{"empty", "", ResponsesSupportModeAuto},
		{"auto", "auto", ResponsesSupportModeAuto},
		{"force responses", "force_responses", ResponsesSupportModeForceResponses},
		{"force chat completions", "force_chat_completions", ResponsesSupportModeForceChatCompletions},
		{"invalid", "enabled", ResponsesSupportModeAuto},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeResponsesSupportMode(tc.mode)
			if got != tc.want {
				t.Errorf("NormalizeResponsesSupportMode(%q) = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestProtocolCapabilitiesRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	caps := ProtocolCapabilities{
		Responses:       ProtocolCapability{Known: true, Supported: true, ProbedAt: now},
		ChatCompletions: ProtocolCapability{Known: true, Supported: true, ProbedAt: now},
	}
	extra := map[string]any{ExtraKeyProtocolCapabilities: caps.ToExtra()}

	got := ResolveProtocolCapabilities(extra)
	if !got.Supports(ProtocolResponses) || !got.Supports(ProtocolChatCompletions) {
		t.Fatalf("round-trip lost support: %+v", got)
	}
	if got.Responses.ProbedAt.Unix() != now.Unix() {
		t.Fatalf("probed_at = %v, want %v", got.Responses.ProbedAt, now)
	}
	if got.NeedsRefresh(now.Add(29 * time.Minute)) {
		t.Fatal("fresh capabilities must not need refresh")
	}
	if !got.NeedsRefresh(now.Add(31 * time.Minute)) {
		t.Fatal("expired capabilities must need refresh")
	}
}

func TestResolveProtocolCapabilitiesLegacyKeys(t *testing.T) {
	t.Run("legacy supported", func(t *testing.T) {
		got := ResolveProtocolCapabilities(map[string]any{ExtraKeyResponsesSupported: true})
		if !got.Supports(ProtocolResponses) {
			t.Fatal("legacy true must mean responses supported")
		}
		if got.ChatCompletions.Known {
			t.Fatal("legacy true must leave chat_completions unknown")
		}
	})
	t.Run("legacy unsupported", func(t *testing.T) {
		got := ResolveProtocolCapabilities(map[string]any{ExtraKeyResponsesSupported: false})
		if got.Responses.Known && got.Responses.Supported {
			t.Fatal("legacy false must not mean responses supported")
		}
		if !got.Supports(ProtocolChatCompletions) {
			t.Fatal("legacy false must imply chat_completions supported")
		}
	})
	t.Run("new structure wins", func(t *testing.T) {
		got := ResolveProtocolCapabilities(map[string]any{
			ExtraKeyResponsesSupported: false,
			ExtraKeyProtocolCapabilities: ProtocolCapabilities{
				Responses: ProtocolCapability{Known: true, Supported: true},
			}.ToExtra(),
		})
		if !got.Supports(ProtocolResponses) {
			t.Fatal("new structure must override legacy key per protocol")
		}
	})
}

func TestProtocolCapabilitiesChooseProtocol(t *testing.T) {
	cases := []struct {
		name    string
		caps    ProtocolCapabilities
		inbound string
		want    string
	}{
		{"both supported follows inbound responses", ProtocolCapabilities{
			Responses: ProtocolCapability{Known: true, Supported: true}, ChatCompletions: ProtocolCapability{Known: true, Supported: true},
		}, ProtocolResponses, ProtocolResponses},
		{"both supported follows inbound cc", ProtocolCapabilities{
			Responses: ProtocolCapability{Known: true, Supported: true}, ChatCompletions: ProtocolCapability{Known: true, Supported: true},
		}, ProtocolChatCompletions, ProtocolChatCompletions},
		{"responses inbound only cc supported", ProtocolCapabilities{
			ChatCompletions: ProtocolCapability{Known: true, Supported: true},
		}, ProtocolResponses, ProtocolChatCompletions},
		{"cc inbound only responses supported", ProtocolCapabilities{
			Responses: ProtocolCapability{Known: true, Supported: true},
		}, ProtocolChatCompletions, ProtocolResponses},
		{"unknown follows inbound responses", ProtocolCapabilities{}, ProtocolResponses, ProtocolResponses},
		{"unknown follows inbound cc", ProtocolCapabilities{}, ProtocolChatCompletions, ProtocolChatCompletions},
		{"inbound unsupported uses other even if unknown", ProtocolCapabilities{
			Responses: ProtocolCapability{Known: true, Supported: false},
		}, ProtocolResponses, ProtocolChatCompletions},
		{"both unsupported follows inbound", ProtocolCapabilities{
			Responses: ProtocolCapability{Known: true, Supported: false}, ChatCompletions: ProtocolCapability{Known: true, Supported: false},
		}, ProtocolResponses, ProtocolResponses},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.caps.ChooseProtocol(tc.inbound); got != tc.want {
				t.Fatalf("ChooseProtocol(%q) = %q, want %q", tc.inbound, got, tc.want)
			}
		})
	}
}
