package agentlog

import (
	"encoding/json"
	"testing"
)

// TestCompactJSON pins the shared helper's contract (M4-B18): insignificant
// whitespace removed, empty/null/invalid input renders "".
func TestCompactJSON(t *testing.T) {
	tests := []struct{ raw, want string }{
		{`{"a": 1, "b": [2, 3]}`, `{"a":1,"b":[2,3]}`},
		{`  { "x" : "y" }  `, `{"x":"y"}`},
		{"", ""},
		{"   ", ""},
		{"null", ""},
		{"{not json", ""},
	}
	for _, tt := range tests {
		if got := CompactJSON(json.RawMessage(tt.raw)); got != tt.want {
			t.Errorf("CompactJSON(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
