package apicompat

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const thinkingTagName = "thinking"

// thinkingTagForm captures how a tag was written: the exact tag name casing
// and whether whitespace appeared before the closing bracket. Closing tags in
// the same form as the opening tag are trusted to end the wrapper.
type thinkingTagForm struct {
	name   string
	spaced bool
}

func (f thinkingTagForm) matches(other thinkingTagForm) bool {
	return f.name == other.name && f.spaced == other.spaced
}

// thinkingCloseTag records the byte range and form of a closing thinking tag.
type thinkingCloseTag struct {
	index  int
	length int
	form   thinkingTagForm
}

// splitLeadingThinkingBlocks separates legacy <thinking>...</thinking> blocks
// that the gateway used to write at the start of assistant visible text.
//
// It only strips complete blocks at the very beginning. Leading whitespace,
// BOM, zero-width characters, optional spaces inside the tags, and tag casing
// are tolerated. Consecutive complete blocks are stripped in a loop. A complete
// leading block is always stripped, including when ordinary text follows the
// closing tag on the same line. Text that is not a legacy wrapper is returned
// unchanged so normal assistant text is not rewritten. A leading wrapper
// without a closing tag is untrusted reasoning: the block and everything after
// it are dropped so it cannot leak as visible content.
//
// Closing tags are matched case-insensitively with optional internal
// whitespace, and the last candidate is used. Reasoning bodies sometimes quote
// an earlier closing tag, and truncating there would expose the rest as visible
// text. The last close must also have the opening tag's exact case/space form:
// a mismatched close is only a pure wrapper when nothing but decorations
// follows it. If non-decoration text follows a mismatched close, the block is
// untrusted and everything from its opening tag is dropped. A close followed
// (after decorations) by another opening wrapper ends the current block so
// consecutive wrappers are handled separately.
func splitLeadingThinkingBlocks(text string) (visible, reasoning string) {
	if text == "" {
		return "", ""
	}

	cursor := skipThinkingDecorations(text, 0)
	var blocks []string
	for {
		openLen, openForm := matchThinkingOpenTagForm(text[cursor:])
		if openLen == 0 {
			break
		}

		bodyStart := cursor + openLen
		closeTag, ok := findThinkingCloseTag(text, bodyStart)
		if !ok {
			// No closing tag: the block is untrusted. Drop it and everything
			// after it so the tail cannot leak as visible content.
			if len(blocks) == 0 {
				return "", ""
			}
			return "", strings.Join(blocks, "\n")
		}

		afterClose := closeTag.index + closeTag.length
		next := skipThinkingDecorations(text, afterClose)
		nextIsBlock := false
		if next < len(text) {
			if nextOpen, _ := matchThinkingOpenTagForm(text[next:]); nextOpen > 0 {
				nextIsBlock = true
			}
		}

		if !closeTag.form.matches(openForm) {
			switch {
			case nextIsBlock:
				// A mismatched close that ends exactly before the next wrapper
				// is a block boundary. There is no visible tail to expose, so
				// continue with the next block.
				blocks = append(blocks, text[bodyStart:closeTag.index])
				cursor = next
				continue
			case next < len(text):
				// A mismatched close followed by ordinary text is not a trusted
				// wrapper boundary. Drop the block and its tail rather than
				// exposing the text after the close.
				if len(blocks) == 0 {
					return "", ""
				}
				return "", strings.Join(blocks, "\n")
			default:
				// Nothing but decorations follows a mismatched close: treat it
				// as a pure wrapper and hide the tail entirely.
				blocks = append(blocks, text[bodyStart:closeTag.index])
				return "", strings.Join(blocks, "\n")
			}
		}

		blocks = append(blocks, text[bodyStart:closeTag.index])
		cursor = afterClose

		// Another complete wrapper may follow. Whitespace between blocks is
		// stripped with the blocks; ordinary visible text keeps the separator
		// rules applied below.
		if nextIsBlock {
			cursor = next
			continue
		}
		break
	}

	if len(blocks) == 0 {
		return text, ""
	}

	visibleStart := skipThinkingBlockSeparator(text, cursor)
	return text[visibleStart:], strings.Join(blocks, "\n")
}

// matchThinkingOpenTagForm matches an opening thinking tag at the start of text.
// It returns the byte length and the tag's exact form, or zero when text does
// not start with one.
func matchThinkingOpenTagForm(text string) (int, thinkingTagForm) {
	return matchThinkingTagForm(text, false)
}

// matchThinkingCloseTagForm matches a closing thinking tag at the start of text.
func matchThinkingCloseTagForm(text string) (int, thinkingTagForm) {
	return matchThinkingTagForm(text, true)
}

func matchThinkingTagForm(text string, closing bool) (int, thinkingTagForm) {
	start := 1
	if closing {
		start = 2
		if len(text) < 2 || text[0] != '<' || text[1] != '/' {
			return 0, thinkingTagForm{}
		}
	} else if len(text) < 1 || text[0] != '<' {
		return 0, thinkingTagForm{}
	}
	pos := matchThinkingTagName(text, start)
	if pos == 0 {
		return 0, thinkingTagForm{}
	}
	form := thinkingTagForm{name: text[start:pos]}
	for pos < len(text) && isThinkingTagSpace(text[pos]) {
		form.spaced = true
		pos++
	}
	if pos < len(text) && text[pos] == '>' {
		return pos + 1, form
	}
	return 0, thinkingTagForm{}
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

// findThinkingCloseTag returns the last closing tag candidate for a block whose
// body starts at start.
//
// Every case-insensitive candidate with optional internal whitespace is
// considered. The last candidate wins: a reasoning body may quote an earlier
// closing tag, and truncating there would expose the rest as visible text. A
// candidate followed by another opening wrapper ends the current block, so
// consecutive complete wrappers do not absorb each other's closings.
func findThinkingCloseTag(text string, start int) (thinkingCloseTag, bool) {
	var last thinkingCloseTag
	has := false
	for pos := start; pos < len(text); pos++ {
		if text[pos] != '<' {
			continue
		}
		length, form := matchThinkingCloseTagForm(text[pos:])
		if length == 0 {
			continue
		}
		last = thinkingCloseTag{index: pos, length: length, form: form}
		has = true

		next := skipThinkingDecorations(text, pos+length)
		if next < len(text) {
			if openLen, _ := matchThinkingOpenTagForm(text[next:]); openLen > 0 {
				return last, true
			}
		}
	}
	return last, has
}

// skipThinkingBlockSeparator advances past the separator that may follow a
// stripped thinking block: spaces, tabs, BOM, and zero-width characters, then
// at most one line break. Indentation after that line break belongs to the
// visible text and is preserved.
func skipThinkingBlockSeparator(text string, pos int) int {
	for pos < len(text) {
		if text[pos] == ' ' || text[pos] == '\t' {
			pos++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[pos:])
		if !isThinkingZeroWidthRune(r) {
			break
		}
		pos += size
	}
	if pos >= len(text) {
		return pos
	}
	switch text[pos] {
	case '\n':
		return pos + 1
	case '\r':
		if pos+1 < len(text) && text[pos+1] == '\n' {
			return pos + 2
		}
		if pos+1 == len(text) {
			// A lone trailing CR is leftover whitespace, not visible text.
			return pos + 1
		}
	}
	return pos
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
