package search

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustMatcher(t *testing.T, query string, o MatchOptions) *Matcher {
	t.Helper()
	m, err := NewMatcher(query, o)
	if err != nil {
		t.Fatalf("NewMatcher(%q): %v", query, err)
	}
	return m
}

func TestLiteralCaseInsensitive(t *testing.T) {
	m := mustMatcher(t, "jwt refresh", MatchOptions{})
	if !m.Match("The JWT Refresh flow") {
		t.Error("default matching should fold ASCII case")
	}
	if m.Match("unrelated text") {
		t.Error("unexpected match")
	}
	start, end, ok := m.Locate("the JWT refresh flow")
	if !ok || start != 4 || end != 15 {
		t.Errorf("Locate = %d, %d, %v; want 4, 15, true", start, end, ok)
	}
}

func TestLiteralCaseSensitive(t *testing.T) {
	m := mustMatcher(t, "JWT", MatchOptions{CaseSensitive: true})
	if m.Match("the jwt token") {
		t.Error("case-sensitive matching must not fold case")
	}
	if !m.Match("the JWT token") {
		t.Error("case-sensitive literal should match exact case")
	}
}

func TestRegexMode(t *testing.T) {
	m := mustMatcher(t, `jw[tT]`, MatchOptions{Regex: true})
	if !m.Match("jwt") || !m.Match("jwT") {
		t.Error("regex alternation should match")
	}
	if m.Match("jww") {
		t.Error("unexpected regex match")
	}
	start, end, ok := m.Locate("say jwT now")
	if !ok || start != 4 || end != 7 {
		t.Errorf("Locate = %d, %d, %v; want 4, 7, true", start, end, ok)
	}
}

func TestRegexCompileError(t *testing.T) {
	_, err := NewMatcher("([", MatchOptions{Regex: true})
	if err == nil {
		t.Fatal("expected compile error")
	}
	if !strings.Contains(err.Error(), "invalid --regex pattern") {
		t.Errorf("error should name the flag: %v", err)
	}
}

func TestEmptyQueryRejected(t *testing.T) {
	if _, err := NewMatcher("", MatchOptions{}); err == nil {
		t.Fatal("empty query should be rejected")
	}
}

func TestNonASCIIQueryFolds(t *testing.T) {
	m := mustMatcher(t, "café", MatchOptions{})
	if !m.Match("CAFÉ opened") {
		t.Error("Unicode case folding should apply on the non-prefilter path")
	}
}

func TestPrefilterEligibility(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"jwt refresh", true},
		{`a"b`, false},  // JSON-escaped char (controller ruling)
		{`a\b`, false},  // JSON-escaped char
		{"a\tb", false}, // control byte
		{"café", false}, // non-ASCII: may appear \uXXXX-escaped
		{"a.b", true},   // regex-special chars are plain literals, JSON-safe
	}
	for _, tt := range tests {
		m := mustMatcher(t, tt.query, MatchOptions{})
		if m.prefilter != tt.want {
			t.Errorf("prefilter(%q) = %v, want %v", tt.query, m.prefilter, tt.want)
		}
	}
}

// TestKeepRawSoundForEligibleNeedles pins the prefilter's core safety
// property: whenever the matcher matches the parsed text, the raw JSONL line
// must pass the prefilter. A prefilter rejection may never lose a hit.
func TestKeepRawSoundForEligibleNeedles(t *testing.T) {
	needles := []string{"jwt", "a.b", "x y z"}
	texts := []string{"the JWT token", "JWT", "no match", "a.b here", "x y z!", "plain"}
	for _, q := range needles {
		m := mustMatcher(t, q, MatchOptions{})
		keep := m.KeepRaw()
		if keep == nil {
			t.Fatalf("prefilter should be enabled for %q", q)
		}
		for _, tx := range texts {
			line, err := json.Marshal(map[string]string{"text": tx})
			if err != nil {
				t.Fatal(err)
			}
			if m.Match(tx) && !keep(line) {
				t.Errorf("prefilter would lose a hit: needle %q, text %q, line %s", q, tx, line)
			}
		}
	}
}

func TestKeepRawCaseSensitive(t *testing.T) {
	m := mustMatcher(t, "JWT", MatchOptions{CaseSensitive: true})
	keep := m.KeepRaw()
	if keep == nil {
		t.Fatal("case-sensitive literals are also prefilterable")
	}
	if keep([]byte(`{"text":"the jwt token"}`)) {
		t.Error("case-sensitive prefilter should reject a lowercase line")
	}
	if !keep([]byte(`{"text":"the JWT token"}`)) {
		t.Error("case-sensitive prefilter should keep a matching line")
	}
}

func TestKeepRawDisabledForEscapedNeedle(t *testing.T) {
	m := mustMatcher(t, `say "hi"`, MatchOptions{})
	if m.KeepRaw() != nil {
		t.Error("JSON-escaped needle must disable the prefilter (parse every line)")
	}
	if !m.Match(`you say "hi" back`) {
		t.Error("quoted needle should still match parsed text")
	}
}

func TestKeepRawNilForRegex(t *testing.T) {
	m := mustMatcher(t, "a+", MatchOptions{Regex: true})
	if m.KeepRaw() != nil {
		t.Error("regex mode must not prefilter raw lines")
	}
}

func TestKeepRawRejectsNonMatchingLine(t *testing.T) {
	m := mustMatcher(t, "jwt", MatchOptions{})
	keep := m.KeepRaw()
	if keep([]byte(`{"role":"user","text":"plain notes"}`)) {
		t.Error("prefilter should reject lines without the needle")
	}
}
