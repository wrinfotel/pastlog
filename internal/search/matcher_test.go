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

// TestKelvinSignKnownSemantics documents the known asymmetry between the two
// case-insensitive paths (M4-B13): an ASCII needle ("k", prefilter-eligible)
// folds ASCII only, so a KELVIN SIGN in the text never matches it; a KELVIN
// needle is not prefilter-eligible and goes through full Unicode folding,
// where KELVIN SIGN and "k" share a fold orbit and do match.
func TestKelvinSignKnownSemantics(t *testing.T) {
	asciiNeedle := mustMatcher(t, "0 k", MatchOptions{}) // pure ASCII: prefilter path
	if asciiNeedle.Match("0 \u212a") {
		t.Error("ASCII needle must not match a KELVIN SIGN (ASCII folding only)")
	}
	kelvinNeedle := mustMatcher(t, "0 \u212a", MatchOptions{}) // non-ASCII: fold path
	if !kelvinNeedle.Match("0 k") {
		t.Error("KELVIN SIGN needle should match plain k via Unicode case folding")
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

// TestProbeIndexPicksRarestByte pins the probe heuristic: the byte with the
// lowest frequency in the (lowered) needle wins, first on ties.
func TestProbeIndexPicksRarestByte(t *testing.T) {
	tests := []struct {
		needle string
		want   int
	}{
		{"jwt", 0},     // all unique: first byte
		{"err err", 3}, // space is the rarest? no: counts are equal except r/e appear twice → first single byte at index 3 (' ')
		{"aaab", 3},    // 'b' unique
		{"~~", 0},      // non-letters fine
	}
	for _, tt := range tests {
		if got := probeIndex([]byte(tt.needle)); got != tt.want {
			t.Errorf("probeIndex(%q) = %d, want %d", tt.needle, got, tt.want)
		}
	}
}

// TestContainsASCIIFoldProbeEquivalence verifies the SIMD-probe scan agrees
// with a naive per-byte reference on tricky boundaries (needle at line start
// or end, probe near the edges, mixed case, non-letter probes) and on
// deterministic pseudo-random inputs.
func TestContainsASCIIFoldProbeEquivalence(t *testing.T) {
	naive := func(line, needle []byte) bool {
		for i := 0; i+len(needle) <= len(line); i++ {
			ok := true
			for j, w := range needle {
				if lowerByte(line[i+j]) != w {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
		return len(needle) == 0
	}

	needles := []string{"jwt", "jwt refresh", "refresh token", "0 k", "e", "x", "~~", "aBc"}
	lines := []string{
		"",
		"j",
		"jwt",
		"jwtjwt",
		"JWT refresh",
		"the jwt refresh token",
		"xjwt",
		"jw",
		"wt",
		"jwtx",
		"tjwtjwtj",
		"REFRESH TOKEN",
		"…®jwt†",
		"0 K",
		"abcabcabc",
	}
	rng := uint32(1)
	nextByte := func() byte { // deterministic xorshift, printable-ish bytes
		rng ^= rng << 13
		rng ^= rng >> 17
		rng ^= rng << 5
		return byte(rng%95) + 32
	}
	for i := 0; i < 200; i++ {
		b := make([]byte, int(nextByte())+int(nextByte()%32))
		for j := range b {
			b[j] = nextByte()
		}
		lines = append(lines, string(b))
	}

	for _, n := range needles {
		needle := []byte(asciiFold(n))
		probe := probeIndex(needle)
		for _, l := range lines {
			line := []byte(l)
			want := naive(line, needle)
			if got := containsASCIIFold(line, needle, probe); got != want {
				t.Errorf("containsASCIIFold(%q, %q) = %v, want %v", l, n, got, want)
			}
		}
	}
}
