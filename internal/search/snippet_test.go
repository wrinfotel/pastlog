package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSnippetShortText(t *testing.T) {
	m := mustMatcher(t, "quick", MatchOptions{})
	start, end, ok := m.Locate("the quick brown fox")
	if !ok {
		t.Fatal("expected a match")
	}
	sn := buildSnippet("previous entry", "the quick brown fox", start, end)
	if sn.context != "previous entry" {
		t.Errorf("context = %q", sn.context)
	}
	if sn.line != "the quick brown fox" {
		t.Errorf("line = %q", sn.line)
	}
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != "quick" {
		t.Errorf("highlight = %q, want quick", got)
	}
}

func TestSnippetNoContext(t *testing.T) {
	m := mustMatcher(t, "quick", MatchOptions{})
	start, end, _ := m.Locate("the quick fox")
	sn := buildSnippet("", "the quick fox", start, end)
	if sn.context != "" {
		t.Errorf("context = %q, want empty", sn.context)
	}
}

func TestSnippetWindowCentersOnMatch(t *testing.T) {
	m := mustMatcher(t, "NEEDLE", MatchOptions{})
	text := strings.Repeat("a", 300) + "NEEDLE" + strings.Repeat("b", 300)
	start, end, _ := m.Locate(text)
	sn := buildSnippet("", text, start, end)
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != "NEEDLE" {
		t.Errorf("highlight = %q, want NEEDLE", got)
	}
	if !strings.HasPrefix(sn.line, "…") || !strings.HasSuffix(sn.line, "…") {
		t.Errorf("windowed line should carry both ellipses: %.20q", sn.line)
	}
	if n := utf8.RuneCountInString(sn.line); n != snippetWidth+2 {
		t.Errorf("windowed line = %d runes, want %d", n, snippetWidth+2)
	}
}

func TestSnippetWindowUnicodeBoundaries(t *testing.T) {
	m := mustMatcher(t, "JWT", MatchOptions{})
	text := strings.Repeat("é", 150) + "JWT" + strings.Repeat("é", 150)
	start, end, _ := m.Locate(text)
	sn := buildSnippet("", text, start, end)
	if !utf8.ValidString(sn.line) {
		t.Fatal("window cut through a rune")
	}
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != "JWT" {
		t.Errorf("highlight = %q, want JWT", got)
	}
}

func TestSnippetContextTruncated(t *testing.T) {
	m := mustMatcher(t, "jwt", MatchOptions{})
	start, end, _ := m.Locate("jwt here")
	sn := buildSnippet(strings.Repeat("w", 300), "jwt here", start, end)
	want := strings.Repeat("w", snippetWidth) + "…"
	if sn.context != want {
		t.Errorf("context = %d runes, want %d runes + ellipsis", utf8.RuneCountInString(sn.context), snippetWidth)
	}
}

func TestSnippetContextFirstLineOnly(t *testing.T) {
	m := mustMatcher(t, "jwt", MatchOptions{})
	start, end, _ := m.Locate("jwt here")
	sn := buildSnippet("first line\nsecond line", "jwt here", start, end)
	if sn.context != "first line" {
		t.Errorf("context = %q, want first line only", sn.context)
	}
}

func TestSnippetTakesMatchingLineOfMultilineText(t *testing.T) {
	m := mustMatcher(t, "jwt", MatchOptions{})
	text := "line one\njwt line\nline three"
	start, end, _ := m.Locate(text)
	sn := buildSnippet("", text, start, end)
	if sn.line != "jwt line" {
		t.Errorf("line = %q, want the line containing the match", sn.line)
	}
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != "jwt" {
		t.Errorf("highlight = %q", got)
	}
}

func TestSnippetMultilineMatchCollapses(t *testing.T) {
	// a needle containing a newline disables the prefilter and can match
	// across lines; the snippet collapses the matched span to one line
	m := mustMatcher(t, "first\nsecond", MatchOptions{})
	text := "first\nsecond\nthird"
	start, end, ok := m.Locate(text)
	if !ok {
		t.Fatal("newline needle should match across the literal newline")
	}
	sn := buildSnippet("", text, start, end)
	if sn.line != "first second" {
		t.Errorf("line = %q, want collapsed match", sn.line)
	}
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != "first second" {
		t.Errorf("highlight = %q", got)
	}
}

// Regression tests: a match longer than the snippet window used to slice
// rs[high:low] inside window() and panic (the startR centering subtraction
// goes negative when matchLen > snippetWidth).

func TestSnippetRegexMatchLongerThanWindow(t *testing.T) {
	m := mustMatcher(t, `x+`, MatchOptions{Regex: true})
	text := strings.Repeat("a", 100) + strings.Repeat("x", 250) + strings.Repeat("b", 100)
	start, end, ok := m.Locate(text)
	if !ok {
		t.Fatal("expected a match")
	}
	sn := buildSnippet("", text, start, end) // must not panic
	if sn.matchStart < 0 || sn.matchEnd > len(sn.line) || sn.matchStart > sn.matchEnd {
		t.Fatalf("highlight outside the window: %d..%d of %d bytes", sn.matchStart, sn.matchEnd, len(sn.line))
	}
	// the 250-rune match overflows the window: the highlight is clipped to
	// the first snippetWidth runes of the match, which fill the whole window
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != strings.Repeat("x", snippetWidth) {
		t.Errorf("highlight = %d runes, want the %d window runes of the match", len([]rune(got)), snippetWidth)
	}
}

func TestSnippetMultilineMatchLongerThanWindow(t *testing.T) {
	m := mustMatcher(t, strings.Repeat("n", 120)+"\n"+strings.Repeat("n", 120), MatchOptions{})
	text := "before " + m.literal + " after"
	start, end, ok := m.Locate(text)
	if !ok {
		t.Fatal("expected a match")
	}
	sn := buildSnippet("", text, start, end) // must not panic
	if sn.matchStart < 0 || sn.matchEnd > len(sn.line) || sn.matchStart > sn.matchEnd {
		t.Fatalf("highlight outside the window: %d..%d of %d bytes", sn.matchStart, sn.matchEnd, len(sn.line))
	}
	// the collapsed 241-rune match overflows the window: the highlight is
	// clipped to its first snippetWidth runes (120 n, the collapsed newline,
	// then 79 n)
	want := strings.Repeat("n", 120) + " " + strings.Repeat("n", snippetWidth-121)
	if got := sn.line[sn.matchStart:sn.matchEnd]; got != want {
		t.Errorf("highlight = %d runes, want the first %d runes of the collapsed match",
			len([]rune(got)), snippetWidth)
	}
}
