package apicompat

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const thinkingTagName = "thinking"

// splitLeadingThinkingBlocks separates legacy <thinking>...</thinking> blocks
// that the gateway used to write at the start of assistant visible text.
//
// It only strips complete blocks at the very beginning. Leading whitespace,
// BOM, zero-width characters, optional spaces inside the tags, and tag casing
// are tolerated. Consecutive complete blocks are stripped in a loop, and
// whitespace after a block does not prevent stripping. Text that is not a
// legacy wrapper is returned unchanged so normal assistant text is not
// rewritten. A leading wrapper without a closing tag is untrusted reasoning:
// the block and everything after it are dropped so it cannot leak as visible
// content.
func splitLeadingThinkingBlocks(text string) (visible, reasoning string) {
	if text == "" {
		return "", ""
	}

	cursor := skipThinkingDecorations(text, 0)
	var blocks []string
	for {
		openLen := matchThinkingOpenTag(text[cursor:])
		if openLen == 0 {
			break
		}

		bodyStart := cursor + openLen
		closeStart, closeLen, ok := findThinkingCloseTag(text, bodyStart)
		if !ok {
			// A leading wrapper without a closing tag is untrusted reasoning.
			// Returning the original text would leak it (and any complete blocks
			// already stripped) as visible content, so drop the unterminated
			// block and everything after it.
			if len(blocks) == 0 {
				return "", ""
			}
			return "", strings.Join(blocks, "\n")
		}

		after := closeStart + closeLen
		next := skipThinkingDecorations(text, after)
		if next < len(text) && matchThinkingOpenTag(text[next:]) > 0 {
			blocks = append(blocks, text[bodyStart:closeStart])
			cursor = next
			continue
		}
		if !validThinkingBlockEnd(text, after) {
			// The closing tag belongs to ordinary prose, not a legacy wrapper.
			// Keep the text as-is instead of partially stripping it.
			return text, ""
		}
		blocks = append(blocks, text[bodyStart:closeStart])
		cursor = after
		break
	}

	if len(blocks) == 0 {
		return text, ""
	}

	visibleStart := skipThinkingDecorations(text, cursor)
	return text[visibleStart:], strings.Join(blocks, "\n")
}

// matchThinkingOpenTag returns the byte length of an opening thinking tag at
// the start of text, or zero when text does not start with one. The tag name is
// matched case-insensitively and spaces may appear before the closing bracket.
func matchThinkingOpenTag(text string) int {
	if len(text) < 1 || text[0] != '<' {
		return 0
	}
	pos := matchThinkingTagName(text, 1)
	if pos == 0 {
		return 0
	}
	for pos < len(text) && isThinkingTagSpace(text[pos]) {
		pos++
	}
	if pos < len(text) && text[pos] == '>' {
		return pos + 1
	}
	return 0
}

// matchThinkingCloseTag returns the byte length of a closing thinking tag at
// the start of text, or zero when text does not start with one.
func matchThinkingCloseTag(text string) int {
	if len(text) < 2 || text[0] != '<' || text[1] != '/' {
		return 0
	}
	pos := matchThinkingTagName(text, 2)
	if pos == 0 {
		return 0
	}
	for pos < len(text) && isThinkingTagSpace(text[pos]) {
		pos++
	}
	if pos < len(text) && text[pos] == '>' {
		return pos + 1
	}
	return 0
}

func matchThinkingTagName(text string, pos int) int {
	if len(text) < pos+len(thinkingTagName) {
		return 0
	}
	if !strings.EqualFold(text[pos:pos+len(thinkingTagName)], thinkingTagName) {
		return 0
	}
	return pos + len(thinkingTagName)
}

func findThinkingCloseTag(text string, start int) (index, length int, ok bool) {
	for pos := start; pos < len(text); pos++ {
		if text[pos] != '<' {
			continue
		}
		if n := matchThinkingCloseTag(text[pos:]); n > 0 {
			return pos, n, true
		}
	}
	return 0, 0, false
}

// validThinkingBlockEnd reports whether the character after a closing tag can
// delimit a legacy wrapper block. Trailing whitespace at the end of the string
// is accepted, and a line break separates a wrapper from following text. A
// same-line space followed by ordinary prose (for example "</thinking> is a
// tag") is not a wrapper and must not strip.
func validThinkingBlockEnd(text string, after int) bool {
	if after >= len(text) {
		return true
	}
	switch text[after] {
	case '\n':
		return true
	case '\r':
		if after+1 < len(text) && text[after+1] == '\n' {
			return true
		}
		return skipThinkingDecorations(text, after) >= len(text)
	case ' ', '\t':
		rest := after
		sawLineBreak := false
		for rest < len(text) {
			r, size := utf8.DecodeRuneInString(text[rest:])
			if r == '\r' || r == '\n' {
				sawLineBreak = true
			}
			if !isThinkingTagSpace(text[rest]) && !isThinkingZeroWidthRune(r) {
				break
			}
			rest += size
		}
		return rest >= len(text) || sawLineBreak
	default:
		return true
	}
}

// skipThinkingDecorations advances past whitespace, BOM, and zero-width
// characters. These can prefix or separate legacy thinking blocks.
func skipThinkingDecorations(text string, pos int) int {
	for pos < len(text) {
		if isThinkingTagSpace(text[pos]) {
			pos++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[pos:])
		if !isThinkingZeroWidthRune(r) {
			break
		}
		pos += size
	}
	return pos
}

func isThinkingTagSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func isThinkingZeroWidthRune(r rune) bool {
	switch r {
	case '\ufeff', '\u200b', '\u200c', '\u200d', '\u2060':
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
