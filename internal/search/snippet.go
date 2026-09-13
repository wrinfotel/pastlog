package search

import (
	"strings"
	"unicode/utf8"
)

// snippetWidth bounds every snippet line and context line, in runes, so
// multi-KB entries cannot flood a result card.
const snippetWidth = 200

// snippet is the rendered fragment of one hit: the line containing the match
// (windowed when the entry line exceeds snippetWidth) plus one line of
// context from the previous entry.
type snippet struct {
	context    string
	line       string
	matchStart int // byte offset of the match inside line
	matchEnd   int
}

// buildSnippet renders one hit. start/end are byte offsets of the match in
// text, as returned by Matcher.Locate.
func buildSnippet(m *Matcher, prev, text string, start, end int) snippet {
	lineStart, lineEnd := lineBounds(text, start)
	if end <= lineEnd { // the match lies within one line of the entry text
		line := text[lineStart:lineEnd]
		l, ms, me := window(line, start-lineStart, end-lineStart)
		return snippet{context: contextLine(prev), line: l, matchStart: ms, matchEnd: me}
	}
	// The match spans lines (only possible on the non-prefilter paths, e.g. a
	// newline in a literal query or a multiline regex): collapse the matched
	// span to a single line, best effort.
	collapsed := strings.ReplaceAll(text[start:end], "\n", " ")
	l, ms, me := window(collapsed, 0, len(collapsed))
	return snippet{context: contextLine(prev), line: l, matchStart: ms, matchEnd: me}
}

// lineBounds returns the bounds of the \n-delimited line containing byte
// offset start.
func lineBounds(text string, start int) (ls, le int) {
	ls = 0
	if i := strings.LastIndexByte(text[:start], '\n'); i >= 0 {
		ls = i + 1
	}
	le = strings.IndexByte(text[ls:], '\n')
	if le < 0 {
		return ls, len(text)
	}
	return ls, le + ls
}

// contextLine renders one line of context from the previous entry's text:
// its first line, truncated to snippetWidth runes.
func contextLine(prev string) string {
	if prev == "" {
		return ""
	}
	line := prev
	if i := strings.IndexByte(prev, '\n'); i >= 0 {
		line = prev[:i]
	}
	line = strings.TrimSuffix(line, "\r")
	runes := []rune(line)
	if len(runes) > snippetWidth {
		return string(runes[:snippetWidth]) + "…"
	}
	return line
}

// window cuts line down to at most snippetWidth runes, centered on the match,
// with ellipses marking the cuts. The returned offsets locate the match
// inside the windowed string.
func window(line string, ms, me int) (string, int, int) {
	rs := []rune(line)
	if len(rs) <= snippetWidth {
		return line, ms, me
	}
	matchRunes := utf8.RuneCountInString(line[:ms]) // rune index of the match
	matchLen := utf8.RuneCountInString(line[ms:me])
	startR := matchRunes - (snippetWidth-matchLen)/2
	if startR < 0 {
		startR = 0
	}
	if max := len(rs) - snippetWidth; startR > max {
		startR = max
	}
	endR := startR + snippetWidth
	core := string(rs[startR:endR])
	newMS := len(string(rs[startR:matchRunes]))
	newME := newMS + len(string(rs[matchRunes:matchRunes+matchLen]))
	prefix, suffix := "", ""
	if startR > 0 {
		prefix = "…"
	}
	if endR < len(rs) {
		suffix = "…"
	}
	out := prefix + core + suffix
	newMS += len(prefix)
	newME += len(prefix)
	if newME > len(out) {
		newME = len(out)
	}
	if newMS > newME {
		newMS = newME
	}
	return out, newMS, newME
}
