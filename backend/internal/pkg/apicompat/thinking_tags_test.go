package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitLeadingThinkingBlocks(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantVisible   string
		wantReasoning string
	}{
		{
			name:          "single block with text",
			input:         "<thinking>a</thinking>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "multiple blocks",
			input:         "<thinking>a</thinking>\n<thinking>b</thinking>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a\nb",
		},
		{
			name:          "complete block then unclosed block drops unterminated tail",
			input:         "<thinking>SECRET</thinking>\n<thinking>oops",
			wantVisible:   "",
			wantReasoning: "SECRET",
		},
		{
			name:          "complete and unclosed blocks without separator",
			input:         "<thinking>SECRET</thinking><thinking>oops",
			wantVisible:   "",
			wantReasoning: "SECRET",
		},
		{
			name:          "complete blocks without separator and trailing text",
			input:         "<thinking>SECRET</thinking><thinking>oops</thinking>trailing",
			wantVisible:   "trailing",
			wantReasoning: "SECRET\noops",
		},
		{
			name:          "only block",
			input:         "<thinking>a</thinking>",
			wantVisible:   "",
			wantReasoning: "a",
		},
		{
			name:          "CRLF",
			input:         "\r\n<thinking>a</thinking>\r\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "unclosed",
			input:         "<thinking>a",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "leading whitespace then unclosed",
			input:         " \n<thinking>a",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "tag in the middle",
			input:         "prefix <thinking>a</thinking>\nVISIBLE",
			wantVisible:   "prefix <thinking>a</thinking>\nVISIBLE",
			wantReasoning: "",
		},
		{
			name:          "closing tag followed by space",
			input:         "<thinking>a</thinking> is a tag",
			wantVisible:   "<thinking>a</thinking> is a tag",
			wantReasoning: "",
		},
		{
			name:          "no separator before text",
			input:         "<thinking>a</thinking>VISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "case sensitive",
			input:         "<Thinking>a</Thinking>\nVISIBLE",
			wantVisible:   "<Thinking>a</Thinking>\nVISIBLE",
			wantReasoning: "",
		},
		{
			name:          "empty string",
			input:         "",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "non ASCII body",
			input:         "<thinking>推理</thinking>\n正文",
			wantVisible:   "正文",
			wantReasoning: "推理",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visible, reasoning := splitLeadingThinkingBlocks(tt.input)
			require.Equal(t, tt.wantVisible, visible)
			require.Equal(t, tt.wantReasoning, reasoning)
		})
	}
}

func TestMergeLegacyReasoning(t *testing.T) {
	require.Equal(t, "", mergeLegacyReasoning("", ""))
	require.Equal(t, "legacy", mergeLegacyReasoning("", "legacy"))
	require.Equal(t, "explicit", mergeLegacyReasoning("explicit", ""))
	require.Equal(t, "same", mergeLegacyReasoning("same", "same"))
	require.Equal(t, "explicit\nlegacy", mergeLegacyReasoning("explicit", "legacy"))
}
