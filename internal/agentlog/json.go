package agentlog

import (
	"bytes"
	"encoding/json"
)

// CompactJSON re-encodes raw without insignificant whitespace; "" when raw
// is empty, null, or not valid JSON. Shared by the adapters that render
// tool-call arguments as compact JSON (gemini-cli, opencode).
func CompactJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return ""
	}
	return buf.String()
}
