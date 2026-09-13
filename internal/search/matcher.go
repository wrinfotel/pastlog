// Package search implements pastlog's search engine: literal and regex
// matchers, the ASCII raw-line prefilter that keeps JSON parsing off the hot
// path, snippet building (match highlight + one line of context) and the hit
// caps of spec §3/§7.
package search

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// MatchOptions selects the query mode.
type MatchOptions struct {
	CaseSensitive bool
	Regex         bool
}

// Matcher finds a query inside parsed entry text. Locate returns byte offsets
// into the original text for highlight ranges. A Matcher is immutable and safe
// for concurrent use.
type Matcher struct {
	opts      MatchOptions
	literal   string         // literal mode: the raw query
	folded    string         // literal mode, prefilterable: ASCII-lowered query
	re        *regexp.Regexp // regex mode
	prefilter bool           // raw-line prefilter enabled (literal mode only)
}

// NewMatcher compiles the query. Literal matching is case-insensitive by
// default; MatchOptions selects case-sensitive or regex mode. An invalid
// regex pattern is a user-facing error (exit 2 per spec §3).
func NewMatcher(query string, o MatchOptions) (*Matcher, error) {
	if query == "" {
		return nil, errors.New("search query must not be empty")
	}
	m := &Matcher{opts: o, literal: query}
	if o.Regex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, fmt.Errorf("invalid --regex pattern: %v", err)
		}
		m.re = re
		return m, nil
	}
	// The raw-line prefilter is enabled only for ASCII needles containing no
	// JSON-escaped characters (control bytes, `"` or `\`) — controller
	// ruling. Such bytes appear in the raw JSONL line verbatim whenever they
	// appear in the parsed text, so rejecting a raw line can never lose a
	// match. Documented limitations of that trade-off:
	//   - encoders that escape printable ASCII (`\/`, HTML-escaped `< > &`)
	//     can hide raw-line occurrences; such needles still match whenever
	//     the raw line contains them verbatim, but the prefilter may skip
	//     the line unparsed;
	//   - case-insensitivity differs per path: a prefilter-eligible needle is
	//     pure ASCII and matches via ASCII folding only (so exotic Unicode
	//     case pairs like KELVIN SIGN in the text never match an ASCII
	//     needle); a needle that is not prefilter-eligible is matched with
	//     full Unicode case-folding instead, where such pairs do match.
	m.prefilter = prefilterEligible(query)
	if m.prefilter && !o.CaseSensitive {
		m.folded = asciiFold(query)
	}
	return m, nil
}

// Match reports whether the parsed text contains the query.
func (m *Matcher) Match(text string) bool {
	_, _, ok := m.Locate(text)
	return ok
}

// Locate returns the byte offsets of the first match in text.
func (m *Matcher) Locate(text string) (start, end int, ok bool) {
	switch {
	case m.re != nil:
		loc := m.re.FindStringIndex(text)
		if loc == nil {
			return 0, 0, false
		}
		return loc[0], loc[1], true
	case m.opts.CaseSensitive:
		i := strings.Index(text, m.literal)
		if i < 0 {
			return 0, 0, false
		}
		return i, i + len(m.literal), true
	case m.prefilter:
		// ASCII-fold both sides; the fold preserves byte length, so the
		// offsets index the original text.
		ft := asciiFold(text)
		i := strings.Index(ft, m.folded)
		if i < 0 {
			return 0, 0, false
		}
		return i, i + len(m.folded), true
	default:
		return locateFold(text, []rune(m.literal))
	}
}

// KeepRaw returns the raw-line prefilter for the engine to hand to adapters
// implementing LineFilteredAdapter, or nil when the prefilter is disabled
// (then every line must be parsed).
func (m *Matcher) KeepRaw() func(rawLine []byte) bool {
	if !m.prefilter {
		return nil
	}
	if m.opts.CaseSensitive {
		needle := []byte(m.literal)
		return func(line []byte) bool { return bytes.Contains(line, needle) }
	}
	folded := []byte(m.folded)
	return func(line []byte) bool { return containsASCIIFold(line, folded) }
}

// prefilterEligible reports whether the query is ASCII with no JSON-escaped
// characters: every byte printable (0x20..0x7E) except `"` and `\`.
func prefilterEligible(query string) bool {
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c < 0x20 || c > 0x7E || c == '"' || c == '\\' {
			return false
		}
	}
	return true
}

// asciiFold lowercases ASCII letters, preserving byte length so offsets into
// the folded string index the original. Non-ASCII bytes pass through.
func asciiFold(s string) string {
	i := 0
	for i < len(s) && (s[i] < 'A' || s[i] > 'Z') {
		i++
	}
	if i == len(s) {
		return s // no uppercase: hot path stays allocation-free
	}
	b := []byte(s)
	for ; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// containsASCIIFold reports whether line contains needle, comparing ASCII
// letters case-insensitively (needle must already be ASCII-lowered). It never
// allocates: this is the search hot path (spec §7).
func containsASCIIFold(line, needle []byte) bool {
	n := len(needle)
	if n == 0 {
		return true
	}
	if len(line) < n {
		return false
	}
	first := lowerByte(needle[0])
	for i := 0; i+n <= len(line); i++ {
		if lowerByte(line[i]) != first {
			continue
		}
		j := 1
		for j < n && lowerByte(line[i+j]) == needle[j] {
			j++
		}
		if j == n {
			return true
		}
	}
	return false
}

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// locateFold finds the first run of runes in s that case-folds (Unicode
// simple folding) to needle. Used only when the ASCII prefilter does not
// apply, so its cost is acceptable. Offsets index the original string.
func locateFold(s string, needle []rune) (int, int, bool) {
	if len(needle) == 0 {
		return 0, 0, true
	}
	rs := []rune(s)
	for i := 0; i+len(needle) <= len(rs); i++ {
		match := true
		for j, want := range needle {
			if !foldEqual(rs[i+j], want) {
				match = false
				break
			}
		}
		if match {
			start := len(string(rs[:i]))
			end := start + len(string(rs[i:i+len(needle)]))
			return start, end, true
		}
	}
	return 0, 0, false
}

// foldEqual compares two runes under Unicode simple folding.
func foldEqual(a, b rune) bool {
	if a == b {
		return true
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}
