package service

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeAccountTestMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: AccountTestModeDefault},
		{input: "default", want: AccountTestModeDefault},
		{input: " compact ", want: AccountTestModeCompact},
		{input: "COMPACT", want: AccountTestModeCompact},
		{input: "unknown", want: AccountTestModeDefault},
	}

	for _, tt := range tests {
		if got := normalizeAccountTestMode(tt.input); got != tt.want {
			t.Fatalf("normalizeAccountTestMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_SuccessMarksSupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusOK}, []byte(`{"id":"cmp_1"}`), nil, true, now)

	if got := updates["openai_compact_supported"]; got != true {
		t.Fatalf("openai_compact_supported = %v, want true", got)
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusOK {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusOK)
	}
	if got := updates["openai_compact_last_error"]; got != "" {
		t.Fatalf("openai_compact_last_error = %v, want empty string", got)
	}
	if got := updates["openai_compact_checked_at"]; got != now.Format(time.RFC3339) {
		t.Fatalf("openai_compact_checked_at = %v, want %s", got, now.Format(time.RFC3339))
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_404MarksUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`404 page not found`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusNotFound}, body, nil, false, now)

	if got := updates["openai_compact_supported"]; got != false {
		t.Fatalf("openai_compact_supported = %v, want false", got)
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusNotFound {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusNotFound)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_ModelNotFound404DoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"code":"model_not_found","message":"The model gpt-5.5-openai-compact does not exist"}}`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusNotFound}, body, nil, false, now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for model_not_found 404")
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusNotFound {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusNotFound)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_EndpointMissingStatusesMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented} {
		updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: status}, nil, nil, false, now)
		if got := updates["openai_compact_supported"]; got != false {
			t.Fatalf("status %d: openai_compact_supported = %v, want false", status, got)
		}
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_502DoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadGateway}, []byte(`Upstream request failed`), nil, false, now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for 502 response")
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusBadGateway {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusBadGateway)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_RequestErrorDoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(nil, nil, errors.New("dial tcp timeout"), false, now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for request error")
	}
	if got, exists := updates["openai_compact_last_status"]; !exists || got != nil {
		t.Fatalf("openai_compact_last_status = %v, want nil key", got)
	}
	if got := updates["openai_compact_last_error"]; got == "" {
		t.Fatalf("expected openai_compact_last_error to be populated")
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_NoResponseClearsLastStatus(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(nil, nil, nil, false, now)

	if got, exists := updates["openai_compact_last_status"]; !exists || got != nil {
		t.Fatalf("openai_compact_last_status = %v, want nil key", got)
	}
	if got := updates["openai_compact_last_error"]; got != "compact probe failed" {
		t.Fatalf("openai_compact_last_error = %v, want compact probe failed", got)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_UnknownModelDoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"message":"unknown model gpt-5.4-openai-compact"}}`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadRequest}, body, nil, false, now)

	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatalf("did not expect openai_compact_supported for unknown-model diagnostics")
	}
	if got := updates["openai_compact_last_status"]; got != http.StatusBadRequest {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusBadRequest)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_EmptyFailureBodyFallsBackToHTTPStatus(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusServiceUnavailable}, nil, nil, false, now)

	if got := updates["openai_compact_last_status"]; got != http.StatusServiceUnavailable {
		t.Fatalf("openai_compact_last_status = %v, want %d", got, http.StatusServiceUnavailable)
	}
	if got := updates["openai_compact_last_error"]; got != "HTTP 503" {
		t.Fatalf("openai_compact_last_error = %v, want HTTP 503", got)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_2xxWithoutCompactionItemMarksUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusOK}, []byte(`{"id":"resp_1","output":[]}`), nil, false, now)

	if got := updates["openai_compact_supported"]; got != false {
		t.Fatalf("openai_compact_supported = %v, want false（2xx 无 compaction item = v2 不可用）", got)
	}
	if got := updates["openai_compact_last_error"]; got == "" {
		t.Fatalf("expected openai_compact_last_error to explain the missing compaction item")
	}
}

func TestCreateOpenAICompactProbePayload_NativeV2Shape(t *testing.T) {
	payload := createOpenAICompactProbePayload("gpt-5.6-sol", true)
	if payload["stream"] != true {
		t.Fatalf("v2 probe payload must be streaming")
	}
	if payload["store"] != false {
		t.Fatalf("OAuth probe payload must carry store:false")
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected 2 input items, got %v", payload["input"])
	}
	last, ok := input[len(input)-1].(map[string]any)
	if !ok || last["type"] != "compaction_trigger" {
		t.Fatalf("last input item must be compaction_trigger, got %v", input[len(input)-1])
	}

	apiKeyPayload := createOpenAICompactProbePayload("gpt-5.6-sol", false)
	if _, has := apiKeyPayload["store"]; has {
		t.Fatalf("API-key probe payload must not force store")
	}
}

func TestOpenAICompactProbeFoundCompactionItem(t *testing.T) {
	sseWithItem := []byte("event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"id\":\"cmp_1\",\"encrypted_content\":\"blob\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[]}}\n\n")
	if !openAICompactProbeFoundCompactionItem(sseWithItem) {
		t.Fatalf("SSE output_item.done 携带 compaction item 应判定为支持")
	}

	sseAlias := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction_summary\",\"id\":\"cmp_2\"}}\n\n")
	if !openAICompactProbeFoundCompactionItem(sseAlias) {
		t.Fatalf("compaction_summary 别名应判定为支持")
	}

	sseNoItem := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"id\":\"msg_1\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"output\":[]}}\n\n")
	if openAICompactProbeFoundCompactionItem(sseNoItem) {
		t.Fatalf("无 compaction item 的流不应判定为支持")
	}

	jsonWithItem := []byte(`{"id":"resp_3","output":[{"type":"compaction","id":"cmp_3"}]}`)
	if !openAICompactProbeFoundCompactionItem(jsonWithItem) {
		t.Fatalf("JSON output[] 携带 compaction item 应判定为支持（降级链兜底）")
	}

	if openAICompactProbeFoundCompactionItem(nil) {
		t.Fatalf("空响应不应判定为支持")
	}
}

func TestOpenAICompactProbeFoundCompactionItem_TerminalResponseOutput(t *testing.T) {
	// 部分上游只在终态 response.completed 的 output[] 给出 compaction item，
	// 事件流里没有 output_item.done——探针同样必须判定为支持。
	sseTerminalOnly := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"id\":\"msg_1\"}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_t\",\"output\":[{\"type\":\"compaction\",\"id\":\"cmp_t\"}]}}\n\n")
	if !openAICompactProbeFoundCompactionItem(sseTerminalOnly) {
		t.Fatalf("终态 response.output 中的 compaction item 应判定为支持")
	}

	sseTerminalEmpty := []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_e\",\"output\":[]}}\n\n")
	if openAICompactProbeFoundCompactionItem(sseTerminalEmpty) {
		t.Fatalf("终态 output 为空且无 item 事件时不应判定为支持")
	}
}

func TestShouldReportOpenAICompactCapabilityUnsupported(t *testing.T) {
	unsupportedBody := []byte(`{"error":{"message":"compact is not supported by this upstream"}}`)

	tests := []struct {
		name            string
		status          int
		body            []byte
		compaction      bool
		official        bool
		known           bool
		wantUnsupported bool
	}{
		{name: "official 400 unsupported stays failure", status: http.StatusBadRequest, body: unsupportedBody, official: true, known: true, wantUnsupported: false},
		{name: "official 2xx without item stays failure", status: http.StatusOK, official: true, known: true, wantUnsupported: false},
		{name: "custom 400 explicit unsupported", status: http.StatusBadRequest, body: unsupportedBody, official: false, known: true, wantUnsupported: true},
		{name: "custom 403 explicit unsupported", status: http.StatusForbidden, body: unsupportedBody, official: false, known: true, wantUnsupported: true},
		{name: "custom 422 explicit unsupported", status: http.StatusUnprocessableEntity, body: unsupportedBody, official: false, known: true, wantUnsupported: true},
		{name: "custom 403 generic forbidden stays failure", status: http.StatusForbidden, body: []byte(`{"error":{"message":"forbidden"}}`), official: false, known: true, wantUnsupported: false},
		{name: "custom 404 capability unsupported", status: http.StatusNotFound, body: []byte(`404 page not found`), official: false, known: true, wantUnsupported: true},
		{name: "custom 405 capability unsupported", status: http.StatusMethodNotAllowed, body: []byte(`method not allowed`), official: false, known: true, wantUnsupported: true},
		{name: "custom 501 capability unsupported", status: http.StatusNotImplemented, body: []byte(`not implemented`), official: false, known: true, wantUnsupported: true},
		{name: "custom 404 model unavailable stays failure", status: http.StatusNotFound, body: []byte(`{"error":{"code":"model_not_found"}}`), official: false, known: true, wantUnsupported: false},
		{name: "official 404 stays failure", status: http.StatusNotFound, body: []byte(`404 page not found`), official: true, known: true, wantUnsupported: false},
		{name: "custom 401 stays failure", status: http.StatusUnauthorized, body: []byte(`unauthorized`), official: false, known: true, wantUnsupported: false},
		{name: "custom 500 stays failure", status: http.StatusInternalServerError, body: []byte(`internal error`), official: false, known: true, wantUnsupported: false},
		{name: "custom 2xx without item", status: http.StatusOK, compaction: false, official: false, known: true, wantUnsupported: true},
		{name: "custom 2xx with item", status: http.StatusOK, compaction: true, official: false, known: true, wantUnsupported: false},
		{name: "unknown target follows probe classification", status: http.StatusBadRequest, body: unsupportedBody, official: false, known: false, wantUnsupported: true},
		{name: "unknown target 404 follows probe classification", status: http.StatusNotFound, body: []byte(`404 page not found`), official: false, known: false, wantUnsupported: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldReportOpenAICompactCapabilityUnsupported(tt.status, tt.body, tt.compaction, tt.official, tt.known)
			if got != tt.wantUnsupported {
				t.Fatalf("shouldReportOpenAICompactCapabilityUnsupported() = %v, want %v", got, tt.wantUnsupported)
			}
		})
	}
}
