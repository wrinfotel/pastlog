// Package mask redacts secret-looking substrings (API keys, tokens,
// passwords) from text before it is shown to a human. Masking is a display
// concern only: JSON output and the underlying session data stay verbatim.
package mask

import (
	"regexp"
	"strings"
)

// The ellipsis marks already-masked text, so value classes that accept
// arbitrary runes exclude it and Mask stays idempotent.
const ellipsis = "…"

// tokenRule matches one secret-token family anywhere in a line. keep is the
// number of leading runes the shorthand preserves — enough to tell the key
// type (ghp_, sk-a, eyJh, AKIA…) apart without revealing it.
type tokenRule struct {
	re   *regexp.Regexp
	keep int
}

var tokenRules = []tokenRule{
	// JWTs: three base64url segments separated by dots. Must run before the
	// bearer rule, which would otherwise swallow the same span.
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), 4},
	// GitHub classic (ghp_/gho_/ghu_/ghs_/ghr_) and fine-grained tokens.
	{regexp.MustCompile(`(?:gh[pousr]|github_pat)_[A-Za-z0-9_]{16,}`), 4},
	// OpenAI / Anthropic API keys (sk-, sk-proj-, sk-ant-…).
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`), 4},
	// AWS access key ids.
	{regexp.MustCompile(`(?:AKIA|ASIA)[0-9A-Z]{16}`), 4},
	// Google API keys (AIza + 35 chars).
	{regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`), 4},
	// Slack tokens (xoxb/xoxa/xoxp/xoxr/xoxs-…).
	{regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`), 4},
	// Bearer headers: keep the word's first runes like any other family.
	{regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{16,}`), 4},
}

// Fine-grained GitHub tokens keep their full marker — four runes would read
// "gith…" and hide the key type.
const githubPatKeep = len("github_pat_")

// urlPasswordRule masks the password of a URL credential pair
// (scheme://user:pass@host) with a bare marker: passwords are rarely keys
// worth telling apart.
var urlPasswordRule = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^/\s:]+:)([^@\s/]+)@`)

// queryValueRule masks the value of secret-carrying query parameters, keeping
// the parameter name and the value's last runes for correlation.
var queryValueRule = regexp.MustCompile(`(?i)([?&](?:api_?key|access_?token|token|client_?secret|secret|passwd|password)=)([^&\s#` + ellipsis + `]+)`)

// Mask replaces secret-looking substrings in s with short handshapes: tokens
// keep their family prefix and last four runes ("ghp_…9fZx"), query-parameter
// values keep name and last four runes ("token=…9fZx"), URL passwords become
// "***". Plain text passes through unchanged.
func Mask(s string) string {
	for _, r := range tokenRules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			return tokenShort(m, r.keep)
		})
	}
	s = urlPasswordRule.ReplaceAllString(s, "$1***@")
	s = queryValueRule.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.LastIndex(m, "=")
		return m[:i+1] + valueShort(m[i+1:])
	})
	return s
}

// tokenShort renders a matched token as its first keep runes, an ellipsis and
// its last four runes; tokens too short for that collapse to "***".
func tokenShort(s string, keep int) string {
	r := []rune(s)
	if strings.HasPrefix(s, "github_pat_") {
		keep = githubPatKeep
	}
	if len(r) <= keep+5 {
		return "***"
	}
	return string(r[:keep]) + ellipsis + string(r[len(r)-4:])
}

// valueShort renders a parameter value as an ellipsis plus its last four
// runes; short values collapse to "***".
func valueShort(s string) string {
	r := []rune(s)
	if len(r) <= 6 {
		return "***"
	}
	return ellipsis + string(r[len(r)-4:])
}
