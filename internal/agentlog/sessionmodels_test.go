package agentlog

import "testing"

// opencode-style session: session-level aggregates, no per-model split.
func sessionUsageNoSplit(id, model string, in, out int64) SessionUsage {
	return SessionUsage{
		Session: Session{ID: id, Agent: "opencode"},
		Usage:   Usage{Input: in, Output: out, Model: model},
	}
}

func TestSessionModelUsageReturnsSplitInFirstUseOrder(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: []SessionUsage{
		{
			Session: Session{ID: "s1"},
			Usage:   Usage{Input: 100, Output: 50},
			Models: []Usage{
				{Model: "m1", Input: 60, Output: 30},
				{Model: "m2", Input: 40, Output: 20},
			},
		},
		{Session: Session{ID: "s2"}}, // other session: must stay out
	}}

	rows, ok := SessionModelUsage(a, "s1")
	if !ok {
		t.Fatal("ok = false, want the session found")
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Model != "m1" || rows[0].Input != 60 || rows[0].Output != 30 {
		t.Errorf("rows[0] = %+v, want m1 60/30", rows[0])
	}
	if rows[1].Model != "m2" || rows[1].Input != 40 {
		t.Errorf("rows[1] = %+v, want m2 40/20", rows[1])
	}
}

func TestSessionModelUsageFallbackAttributesSessionModel(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "opencode"}}, usage: []SessionUsage{
		sessionUsageNoSplit("s1", "qwen3-coder-480b", 1523, 412),
	}}

	rows, ok := SessionModelUsage(a, "s1")
	if !ok {
		t.Fatal("ok = false, want the session found")
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (session-level aggregates)", len(rows))
	}
	if rows[0].Model != "qwen3-coder-480b" || rows[0].Input != 1523 || rows[0].Output != 412 {
		t.Errorf("row = %+v, want the session model with all tokens", rows[0])
	}
}

func TestSessionModelUsageZeroUsageYieldsNoRows(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "opencode"}}, usage: []SessionUsage{
		sessionUsageNoSplit("s1", "m", 0, 0),
	}}

	rows, ok := SessionModelUsage(a, "s1")
	if !ok {
		t.Fatal("ok = false, want the session found")
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want none for a zero-usage session", rows)
	}
}

func TestSessionModelUsageUnknownID(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: []SessionUsage{
		{Session: Session{ID: "s1"}},
	}}
	if _, ok := SessionModelUsage(a, "other"); ok {
		t.Error("ok = true, want false for an unknown id")
	}
}
