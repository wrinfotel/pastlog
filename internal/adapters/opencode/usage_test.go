package opencode

import (
	"testing"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// listUsage streams SessionsUsage through a collecting iterator.
func listUsage(t *testing.T, a *Adapter) []agentlog.SessionUsage {
	t.Helper()
	var out []agentlog.SessionUsage
	if err := a.SessionsUsage(func(su agentlog.SessionUsage) error {
		out = append(out, su)
		return nil
	}); err != nil {
		t.Fatalf("SessionsUsage: %v", err)
	}
	return out
}

// TestSessionsUsageFixture pins the exact token/cost/model numbers of the
// generated fixture (M7): the session table's aggregate columns map 1:1, the
// cost column provides CostUSD with HasCost=true (even for a 0 cost), and
// Messages keeps the SessionsMeta semantic.
func TestSessionsUsageFixture(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	got := listUsage(t, a)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (parent + child)", len(got))
	}
	parent, child := got[0], got[1]
	if parent.ID != parentID || child.ID != childID {
		t.Fatalf("ids = %q, %q; want parent then child (time_created order)", parent.ID, child.ID)
	}
	if parent.Agent != "opencode" {
		t.Errorf("Agent = %q", parent.Agent)
	}
	// parent: 1523/412/87/10240/512, cost 0.42, qwen3-coder-480b
	if parent.Input != 1523 || parent.Output != 412 || parent.Reasoning != 87 ||
		parent.CacheRead != 10240 || parent.CacheWrite != 512 {
		t.Errorf("parent tokens = in %d out %d reasoning %d cr %d cw %d, want 1523/412/87/10240/512",
			parent.Input, parent.Output, parent.Reasoning, parent.CacheRead, parent.CacheWrite)
	}
	if !parent.HasCost || parent.CostUSD != 0.42 {
		t.Errorf("parent cost = %f (has=%v), want 0.42 (has=true)", parent.CostUSD, parent.HasCost)
	}
	if parent.Model != "qwen3-coder-480b" {
		t.Errorf("parent Model = %q", parent.Model)
	}
	if parent.Messages != 2 {
		t.Errorf("parent Messages = %d, want 2 (same semantic as SessionsMeta)", parent.Messages)
	}
	// child: 310/95/20/0/128 — the zero cache_read column must read as 0
	if child.Input != 310 || child.Output != 95 || child.Reasoning != 20 ||
		child.CacheRead != 0 || child.CacheWrite != 128 {
		t.Errorf("child tokens = in %d out %d reasoning %d cr %d cw %d, want 310/95/20/0/128",
			child.Input, child.Output, child.Reasoning, child.CacheRead, child.CacheWrite)
	}
	if !child.HasCost || child.CostUSD != 0.08 {
		t.Errorf("child cost = %f (has=%v), want 0.08 (has=true)", child.CostUSD, child.HasCost)
	}
	if child.Model != "qwen3-coder-480b" {
		t.Errorf("child Model = %q", child.Model)
	}
	if child.Messages != 1 {
		t.Errorf("child Messages = %d, want 1", child.Messages)
	}
	// the session fields stay identical to SessionsMeta
	metas := listMetas(t, a)
	for i, su := range got {
		if su.Session != metas[i].Session {
			t.Errorf("session[%d] fields drifted from SessionsMeta:\n%+v\nvs\n%+v", i, su.Session, metas[i].Session)
		}
		if su.Messages != metas[i].Messages {
			t.Errorf("session[%d] Messages = %d, want %d", i, su.Messages, metas[i].Messages)
		}
	}
}

// TestLockedDBSessionsUsageNoError pins the locked-DB fallback on the usage
// view: no sessions, no error, one warning (mirrors SessionsMeta exactly).
func TestLockedDBSessionsUsageNoError(t *testing.T) {
	dir := buildFixtureDB(t)
	release := lockDatabase(t, dir)
	defer release()

	a := NewDir(dir)
	var sessions int
	err := a.SessionsUsage(func(agentlog.SessionUsage) error { sessions++; return nil })
	if err != nil {
		t.Errorf("SessionsUsage on a locked DB must not fail: %v", err)
	}
	if sessions != 0 {
		t.Errorf("locked DB should yield no usage rows, got %d", sessions)
	}
	if a.Warning() == "" {
		t.Error("locked DB should produce a warning line on the usage path too")
	}
}
