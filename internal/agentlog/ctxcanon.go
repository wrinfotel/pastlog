package agentlog

// Shared helpers behind the per-adapter context extractors (CtxSource,
// SPEC-context-analysis §1.1 and §2.2). One implementation keeps ArgsKey
// normalization, call labels and error-shape detection identical across
// agents — the cross-agent parity of the context findings depends on it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// CanonArgsKey returns the normalized identity key of a tool call:
// `tool + "\x00" + canonical(rawInput)` (SPEC §1.1). Two calls share an
// ArgsKey iff they asked the same tool for the same thing.
func CanonArgsKey(tool string, rawInput json.RawMessage) string {
	return tool + "\x00" + CanonJSON(rawInput)
}

// CanonJSON canonicalizes a raw JSON argument object: object keys are sorted
// recursively, known-noise keys are dropped, rendering is compact. Strings
// render bare (no quotes) to keep labels short. Non-JSON input passes
// through trimmed; missing input renders as "{}".
func CanonJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return string(trimmed)
	}
	return canonValue(v)
}

func canonValue(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		for i := 1; i < len(keys); i++ { // insertion sort; arg objects are tiny
			for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			}
		}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if IsNoiseKey(k) {
				continue
			}
			parts = append(parts, k+"="+canonValue(t[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, canonValue(e))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// IsNoiseKey reports keys that never define what a call asked for.
func IsNoiseKey(k string) bool {
	switch k {
	case "session_id", "sessionId", "call_id", "callId", "id", "uuid", "timestamp":
		return true
	}
	return false
}

// ToolLabel picks a short human label for a tool call: the natural field
// when the tool has one (path, command, pattern, …), else compact canonical
// JSON, else the tool name. Commands may be string or string-array (codex
// shell); newlines collapse so one finding stays one line.
func ToolLabel(tool string, rawInput json.RawMessage) string {
	for _, name := range []string{"file_path", "path", "notebook_path", "command", "pattern", "url", "query"} {
		if v := labelFieldValue(rawInput, name); v != "" {
			return v
		}
	}
	if c := CanonJSON(rawInput); c != "{}" {
		return strings.ReplaceAll(c, "\n", " ")
	}
	return tool
}

// labelFieldValue reads one top-level field as a short label string: a JSON
// string verbatim, an array of strings joined with spaces (codex shell
// commands), anything else not usable as a label.
func labelFieldValue(raw json.RawMessage, name string) string {
	v := rawFieldValue(raw, name)
	trimmed := bytes.TrimSpace(v)
	if len(trimmed) == 0 {
		return ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return strings.ReplaceAll(s, "\n", " ")
		}
	case '[':
		var items []string
		if err := json.Unmarshal(trimmed, &items); err == nil {
			joined := strings.Join(items, " ")
			if strings.TrimSpace(joined) != "" {
				return strings.ReplaceAll(joined, "\n", " ")
			}
		}
	}
	return ""
}

// rawFieldValue reads one top-level field of a raw JSON object.
func rawFieldValue(raw json.RawMessage, name string) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &m); err != nil {
		return nil
	}
	return m[name]
}

// FirstLine returns the first non-empty line of s, capped at 120 runes —
// the headline of a tool result for the R3 finding.
func FirstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if len(ln) > 120 {
			return ln[:119] + "…"
		}
		return ln
	}
	return ""
}

// errShapedRe is the uniform error-shape fallback (SPEC §2.2): scanned on
// the first 2 KB of a result's text when the format carries no exact error
// flag for that result.
var errShapedRe = regexp.MustCompile(`(?i)error|failed|exception|traceback`)

// ErrShaped reports whether a tool result's text looks like an error. The
// scan covers the first 2 KB only, matching SPEC §2.2.
func ErrShaped(text string) bool {
	if len(text) > 2048 {
		text = text[:2048]
	}
	return errShapedRe.MatchString(text)
}

// ErrFromStatus applies SPEC §2.2 when a format carries a free-form tool
// status (opencode/zcode state.status, gemini toolCalls status): a status
// naming an error is an exact flag; an absent status falls back to the
// uniform error-shape regex; any other observed status ("completed",
// "pending", "confirmed", "executed") passes through as a non-error.
func ErrFromStatus(status, text string) bool {
	st := strings.ToLower(strings.TrimSpace(status))
	if st == "" {
		return ErrShaped(text)
	}
	return strings.Contains(st, "error") || strings.Contains(st, "fail")
}
