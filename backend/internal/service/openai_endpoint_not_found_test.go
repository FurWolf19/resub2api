//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIEndpointNotFoundJSON(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "html 404", status: 404, body: `<html><body>not found</body></html>`, want: false},
		{name: "empty 404", status: 404, body: "", want: false},
		{name: "plain text 404", status: 404, body: "404 page not found", want: false},
		{name: "unknown json 404", status: 404, body: `{"error":{"message":"quota exceeded"}}`, want: false},
		{name: "json array 404", status: 404, body: `[{"error":{"code":"not_found"}}]`, want: false},
		{name: "known error code", status: 404, body: `{"error":{"code":"endpoint_not_found"}}`, want: true},
		{name: "explicit endpoint code beats model-looking message", status: 404, body: `{"error":{"code":"endpoint_not_found","message":"model route not found"}}`, want: true},
		{name: "generic not_found with model message stays unknown", status: 404, body: `{"error":{"code":"not_found","message":"model route not found"}}`, want: false},
		{name: "generic not_found alone is endpoint evidence", status: 404, body: `{"error":{"code":"not_found"}}`, want: true},
		{name: "known top-level code", status: 404, body: `{"code":"route_not_found"}`, want: true},
		{name: "known error type", status: 405, body: `{"error":{"type":"method_not_allowed"}}`, want: true},
		{name: "known message", status: 404, body: `{"error":{"message":"endpoint does not exist"}}`, want: true},
		{name: "known top-level message", status: 404, body: `{"message":"no route matched"}`, want: true},
		{name: "known detail", status: 405, body: `{"detail":"Method Not Allowed"}`, want: true},
		{name: "method not allowed message", status: 405, body: `{"error":{"message":"method not allowed"}}`, want: true},
		{name: "model not found", status: 404, body: `{"error":{"code":"model_not_found"}}`, want: false},
		{name: "model name not found message", status: 404, body: `{"error":{"message":"Model gpt-4o not found"}}`, want: false},
		{name: "model name does not exist message", status: 404, body: `{"error":{"message":"The model ` + "`gpt-4o`" + ` does not exist"}}`, want: false},
		{name: "model unavailable message on 405", status: 405, body: `{"error":{"message":"model is not available"}}`, want: false},
		{name: "model unavailable", status: 404, body: `{"error":{"message":"The model does not exist"}}`, want: false},
		{name: "invalid url message", status: 404, body: `{"error":{"message":"Invalid URL (POST /v1/responses)"}}`, want: true},
		{name: "invalid url top-level detail", status: 405, body: `{"detail":"Invalid URL (POST /v1/chat/completions)"}`, want: true},
		{name: "unknown_url code", status: 404, body: `{"error":{"code":"unknown_url"}}`, want: true},
		{name: "unknown request url message", status: 404, body: `{"error":{"message":"Unknown request URL: POST /v1/responses"}}`, want: true},
		{name: "unknown url message", status: 405, body: `{"message":"unknown url"}`, want: true},
		{name: "400 is not endpoint evidence", status: 400, body: `{"error":{"code":"endpoint_not_found"}}`, want: false},
		{name: "401 is not endpoint evidence", status: 401, body: `{"error":{"code":"endpoint_not_found"}}`, want: false},
		{name: "200 is not endpoint evidence", status: 200, body: `{"error":{"code":"endpoint_not_found"}}`, want: false},
		{name: "whitespace around json", status: 404, body: "  {\"error\":{\"code\":\"not_found\"}}\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIEndpointNotFoundJSON(tt.status, []byte(tt.body)))
		})
	}
}
