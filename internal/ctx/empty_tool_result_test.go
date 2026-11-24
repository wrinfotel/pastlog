package ctx

import (
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

func TestOversizedIgnoresEmptyToolResults(t *testing.T) {
	events := []agentlog.CtxEvent{
		{Kind: agentlog.CtxToolCall, Tool: "image", Label: "preview"},
		{Kind: agentlog.CtxToolResult, Tool: "image"},
		{Kind: agentlog.CtxToolCall, Tool: "read", Label: "file"},
		{Kind: agentlog.CtxToolResult, Tool: "read", ResBytes: 200},
	}
	report := Analyze(events)
	if len(report.Findings) != 1 || report.Findings[0].Tool != "read" {
		t.Fatalf("findings = %+v, want only the nonempty result", report.Findings)
	}
}
