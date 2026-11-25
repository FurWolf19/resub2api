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
			name:          "only block with trailing spaces",
			input:         "<thinking>SECRET</thinking>  ",
			wantVisible:   "",
			wantReasoning: "SECRET",
		},
		{
			name:          "only block with trailing whitespace mix",
			input:         "<thinking>SECRET</thinking> \t\r\n",
			wantVisible:   "",
			wantReasoning: "SECRET",
		},
		{
			name:          "only block with trailing carriage return",
			input:         "<thinking>SECRET</thinking>\r",
			wantVisible:   "",
			wantReasoning: "SECRET",
		},
		{
			name:          "multiple blocks with whitespace separators",
			input:         "<thinking>a</thinking>  \n <thinking>b</thinking> \t",
			wantVisible:   "",
			wantReasoning: "a\nb",
		},
		{
			name:          "block then whitespace then visible text",
			input:         "<thinking>a</thinking>  \nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "BOM before block",
			input:         "\ufeff<thinking>SECRET</thinking>visible",
			wantVisible:   "visible",
			wantReasoning: "SECRET",
		},
		{
			name:          "zero width prefix before block",
			input:         "\u200b\u200c\u200d\u2060<thinking>SECRET</thinking>visible",
			wantVisible:   "visible",
			wantReasoning: "SECRET",
		},
		{
			name:          "whitespace BOM and zero width before block",
			input:         " \ufeff\t\u200b<thinking>a</thinking>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "unclosed block after prefix drops tail",
			input:         "\ufeff<thinking>SECRET\nVISIBLE",
			wantVisible:   "",
			wantReasoning: "",
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
			name:          "closing tag followed by same line prose is stripped",
			input:         "<thinking>a</thinking> is a tag",
			wantVisible:   "is a tag",
			wantReasoning: "a",
		},
		{
			name:          "same line space after block",
			input:         "<thinking>SECRET</thinking> visible",
			wantVisible:   "visible",
			wantReasoning: "SECRET",
		},
		{
			name:          "same line tab after block",
			input:         "<thinking>SECRET</thinking>\tvisible",
			wantVisible:   "visible",
			wantReasoning: "SECRET",
		},
		{
			name:          "indentation after newline is preserved",
			input:         "<thinking>plan</thinking>\n    code",
			wantVisible:   "    code",
			wantReasoning: "plan",
		},
		{
			name:          "CRLF indentation after newline is preserved",
			input:         "<thinking>plan</thinking>\r\n\tcode",
			wantVisible:   "\tcode",
			wantReasoning: "plan",
		},
		{
			name:          "spaces and zero width after block",
			input:         "<thinking>a</thinking> \u200bVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "spaced wrapper then indented text",
			input:         "<thinking >plan</thinking >\n    code",
			wantVisible:   "    code",
			wantReasoning: "plan",
		},
		{
			name:          "only one blank line after block is consumed",
			input:         "<thinking>SECRET</thinking>  \n\nVISIBLE",
			wantVisible:   "\nVISIBLE",
			wantReasoning: "SECRET",
		},
		{
			name:          "trailing whitespace after newline is preserved",
			input:         "<thinking>SECRET</thinking>\n  ",
			wantVisible:   "  ",
			wantReasoning: "SECRET",
		},
		{
			name:          "forged uppercase spaced close does not truncate the last close",
			input:         "<thinking>Use </THINKING >\nSECRET\n</thinking>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "Use </THINKING >\nSECRET\n",
		},
		{
			name:          "forged same form close does not truncate the last close",
			input:         "<thinking>Use </thinking>\nSECRET\n</thinking>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "Use </thinking>\nSECRET\n",
		},
		{
			name:          "forged same form close inside uppercase wrapper",
			input:         "<THINKING>Use </THINKING>\nSECRET\n</THINKING>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "Use </THINKING>\nSECRET\n",
		},
		{
			name:          "forged lowercase close inside uppercase wrapper",
			input:         "<THINKING>Use </thinking>\nSECRET\n</THINKING>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "Use </thinking>\nSECRET\n",
		},
		{
			name:          "same form forged close before real flexible close does not truncate",
			input:         "<thinking>Use </thinking>\nSECRET\n</THINKING >",
			wantVisible:   "",
			wantReasoning: "Use </thinking>\nSECRET\n",
		},
		{
			name:          "flexible close with trailing text is dropped",
			input:         "<thinking>a </THINKING >\nVISIBLE",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "single forged flexible close with trailing secret and text is dropped",
			input:         "<thinking>Use </THINKING >\nSECRET\nvisible",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "pure flexible close is stripped",
			input:         "<thinking>secret</THINKING>",
			wantVisible:   "",
			wantReasoning: "secret",
		},
		{
			name:          "pure flexible close with trailing whitespace is stripped",
			input:         "<thinking>secret</THINKING> \n\n",
			wantVisible:   "",
			wantReasoning: "secret",
		},
		{
			name:          "mixed case flexible close with trailing text is dropped",
			input:         "<thinking>secret</THINKING>\nvisible",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "mismatched close followed by another block continues",
			input:         "<thinking>a</THINKING >\n<thinking>b</thinking>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a\nb",
		},
		{
			name:          "orphan close after a complete block is absorbed into reasoning",
			input:         "<thinking>SECRET</thinking> visible </thinking>",
			wantVisible:   "",
			wantReasoning: "SECRET</thinking> visible ",
		},
		{
			name:          "no separator before text",
			input:         "<thinking>a</thinking>VISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "uppercase tags",
			input:         "<THINKING>a</THINKING>\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "mixed case tags with trailing text are dropped",
			input:         "<ThInKiNg>a</tHiNkInG>VISIBLE",
			wantVisible:   "",
			wantReasoning: "",
		},
		{
			name:          "tags with space before closing bracket",
			input:         "<thinking >a</thinking >\nVISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "uppercase tags with space before closing bracket",
			input:         "<THINKING >a</THINKING >VISIBLE",
			wantVisible:   "VISIBLE",
			wantReasoning: "a",
		},
		{
			name:          "BOM without thinking is unchanged",
			input:         "\ufeffplain visible",
			wantVisible:   "\ufeffplain visible",
			wantReasoning: "",
		},
		{
			name:          "zero width without thinking is unchanged",
			input:         "\u200bplain visible",
			wantVisible:   "\u200bplain visible",
			wantReasoning: "",
		},
		{
			name:          "orphan closing tag is unchanged",
			input:         "</thinking>visible",
			wantVisible:   "</thinking>visible",
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
