package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesResponseCreatedAtAcceptsNumericForms(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int64
	}{
		{"integer", `{"created_at":1794055887}`, 1794055887},
		{"float", `{"created_at":1794055887.0}`, 1794055887},
		{"float with fraction", `{"created_at":1794055887.9}`, 1794055887},
		{"numeric string", `{"created_at":"1794055887"}`, 1794055887},
		{"float string", `{"created_at":"1794055887.0"}`, 1794055887},
		{"null", `{"created_at":null}`, 0},
		{"missing", `{}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp ResponsesResponse
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &resp))
			require.Equal(t, tc.want, resp.CreatedAt)
		})
	}
}

func TestResponsesResponseCreatedAtMarshalsAsInteger(t *testing.T) {
	resp := ResponsesResponse{ID: "resp_1", Object: "response", CreatedAt: 1794055887, Status: "completed"}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	value, ok := wire["created_at"].(float64)
	require.True(t, ok, "created_at must be a JSON number: %s", data)
	require.EqualValues(t, 1794055887, int64(value))
	require.NotContains(t, string(data), `"created_at":"`, "created_at must not be serialized as a string")
}

func TestResponsesStreamEventCreatedAtToleratesFloat(t *testing.T) {
	raw := []byte(`{"type":"response.created","response":{"id":"resp_1","object":"response",` +
		`"created_at":1794055887.0,"status":"in_progress"}}`)
	var event ResponsesStreamEvent
	require.NoError(t, json.Unmarshal(raw, &event))
	require.NotNil(t, event.Response)
	require.EqualValues(t, 1794055887, event.Response.CreatedAt)
}
