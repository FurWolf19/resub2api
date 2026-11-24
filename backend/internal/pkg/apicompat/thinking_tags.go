package apicompat

import (
	"encoding/json"
	"strings"
)

const (
	thinkingOpenTag  = "<thinking>"
	thinkingCloseTag = "</thinking>"
)

// splitLeadingThinkingBlocks separates legacy <thinking>...</thinking> blocks
// that the gateway used to write at the start of assistant visible text.
//
// It only strips complete, lowercase blocks at the very beginning (leading
// whitespace allowed) whose closing tag is followed by a newline, the end of
// the string, or a non-whitespace character. Text that is not a legacy wrapper
// is returned unchanged so normal assistant text is not rewritten. A leading
// wrapper without a closing tag is untrusted reasoning: the block and
// everything after it are dropped so it cannot leak as visible content.
func splitLeadingThinkingBlocks(text string) (visible, reasoning string) {
	if text == "" {
		return "", ""
	}

	pos := 0
	for pos < len(text) && isThinkingTagSpace(text[pos]) {
		pos++
	}

	var blocks []string
	cursor := pos
	for strings.HasPrefix(text[cursor:], thinkingOpenTag) {
		bodyStart := cursor + len(thinkingOpenTag)
		rel := strings.Index(text[bodyStart:], thinkingCloseTag)
		if rel < 0 {
			// A leading wrapper without a closing tag is untrusted reasoning.
			// Returning the original text would leak it (and any complete blocks
			// already stripped) as visible content, so drop the unterminated
			// block and everything after it.
			if len(blocks) == 0 {
				return "", ""
			}
			return "", strings.Join(blocks, "\n")
		}
		bodyEnd := bodyStart + rel
		after := bodyEnd + len(thinkingCloseTag)
		if !validThinkingBlockEnd(text, after) {
			// The closing tag belongs to ordinary prose, not a legacy wrapper.
			// Keep the text as-is instead of partially stripping it.
			return text, ""
		}
		blocks = append(blocks, text[bodyStart:bodyEnd])
		cursor = after

		next := after
		for next < len(text) && isThinkingTagSpace(text[next]) {
			next++
		}
		if next < len(text) && strings.HasPrefix(text[next:], thinkingOpenTag) {
			cursor = next
			continue
		}
		break
	}

	if len(blocks) == 0 {
		return text, ""
	}

	rest := cursor
	for rest < len(text) && (text[rest] == '\r' || text[rest] == '\n') {
		rest++
	}
	return text[rest:], strings.Join(blocks, "\n")
}

// validThinkingBlockEnd reports whether the character after a closing tag can
// delimit a legacy wrapper block. A space or tab means the tag was part of
// ordinary prose (for example "</thinking> is a tag") and must not strip.
func validThinkingBlockEnd(text string, after int) bool {
	if after >= len(text) {
		return true
	}
	switch text[after] {
	case '\n':
		return true
	case '\r':
		return after+1 < len(text) && text[after+1] == '\n'
	case ' ', '\t':
		return false
	default:
		return true
	}
}

func isThinkingTagSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

// stripLeadingThinkingFromChatContent strips legacy thinking wrappers from a
// JSON string content value. Non-string content and unchanged strings are
// returned as-is.
func stripLeadingThinkingFromChatContent(raw json.RawMessage) (json.RawMessage, string) {
	if len(raw) == 0 {
		return raw, ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return raw, ""
	}
	visible, reasoning := splitLeadingThinkingBlocks(s)
	if visible == s {
		return raw, ""
	}
	stripped, err := json.Marshal(visible)
	if err != nil {
		return raw, ""
	}
	return stripped, reasoning
}

// mergeLegacyReasoning combines an explicit reasoning field with reasoning
// recovered from legacy thinking wrappers without duplicating identical text.
func mergeLegacyReasoning(explicit, legacy string) string {
	if legacy == "" {
		return explicit
	}
	if explicit == "" || explicit == legacy {
		return legacy
	}
	return explicit + "\n" + legacy
}
