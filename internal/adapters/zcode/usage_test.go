package zcode

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
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

// TestSessionsUsageFixture pins the exact token/model numbers of the
// generated fixture: ZCode reports usage per model request in model_usage,
// so the adapter sums the rows per session, and the model of the latest
// request (by started_at) becomes the session model. ZCode exposes no cost,
// so HasCost stays false. Messages keeps the SessionsMeta semantic.
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
	if parent.Agent != "zcode" {
		t.Errorf("Agent = %q", parent.Agent)
	}
	// parent: two requests, 100+50 / 20+10 / 5+0 / cache read 200+80, write 30+0
	if parent.Input != 150 || parent.Output != 30 || parent.Reasoning != 5 ||
		parent.CacheRead != 280 || parent.CacheWrite != 30 {
		t.Errorf("parent tokens = in %d out %d reasoning %d cr %d cw %d, want 150/30/5/280/30",
			parent.Input, parent.Output, parent.Reasoning, parent.CacheRead, parent.CacheWrite)
	}
	if parent.HasCost {
		t.Errorf("parent HasCost = true, want false (zcode reports no cost)")
	}
	if parent.CostUSD != 0 {
		t.Errorf("parent CostUSD = %f, want 0", parent.CostUSD)
	}
	// the model of the latest request (started_at 1786220980000) wins
	if parent.Model != "fixture-model-b" {
		t.Errorf("parent Model = %q, want fixture-model-b (latest request)", parent.Model)
	}
	if parent.Messages != 2 {
		t.Errorf("parent Messages = %d, want 2 (same semantic as SessionsMeta)", parent.Messages)
	}
	// child: one request, 10/2/1/16/4 — the zero-valued splits must read as 0
	if child.Input != 10 || child.Output != 2 || child.Reasoning != 1 ||
		child.CacheRead != 16 || child.CacheWrite != 4 {
		t.Errorf("child tokens = in %d out %d reasoning %d cr %d cw %d, want 10/2/1/16/4",
			child.Input, child.Output, child.Reasoning, child.CacheRead, child.CacheWrite)
	}
	if child.HasCost {
		t.Errorf("child HasCost = true, want false (zcode reports no cost)")
	}
	if child.Model != "fixture-model-c" {
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

// TestSessionsUsageWithoutModelUsageTable is the defensive path against older
// ZCode builds: a database without the model_usage table still lists sessions
// (with zero usage) instead of failing.
func TestSessionsUsageWithoutModelUsageTable(t *testing.T) {
	dir := buildFixtureDB(t)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE model_usage`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	a := NewDir(dir)
	got := listUsage(t, a)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (usage degrades to zeros, not an error)", len(got))
	}
	parent := got[0]
	if parent.Input != 0 || parent.Output != 0 || parent.Reasoning != 0 ||
		parent.CacheRead != 0 || parent.CacheWrite != 0 {
		t.Errorf("parent tokens = %+v, want all zero", agentlog.Usage{
			Input: parent.Input, Output: parent.Output, Reasoning: parent.Reasoning,
			CacheRead: parent.CacheRead, CacheWrite: parent.CacheWrite,
		})
	}
	if parent.Model != "" {
		t.Errorf("parent Model = %q, want \"\" without model_usage", parent.Model)
	}
}

// TestSessionsUsageWithoutRows pins the realistic mixed case: a session with
// no model_usage rows yet (freshly created) yields zero usage while other
// sessions keep theirs.
func TestSessionsUsageWithoutRows(t *testing.T) {
	dir := buildFixtureDB(t)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM model_usage WHERE session_id = ?`, childID); err != nil {
		t.Fatal(err)
	}

	a := NewDir(dir)
	got := listUsage(t, a)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if got[0].ID != parentID || got[0].Input != 150 {
		t.Fatalf("parent usage drifted: %+v", got[0])
	}
	if got[1].ID != childID {
		t.Fatalf("second session = %q, want child", got[1].ID)
	}
	if got[1].Input != 0 || got[1].Output != 0 || got[1].Model != "" {
		t.Errorf("child usage = in %d out %d model %q, want zeros and \"\"",
			got[1].Input, got[1].Output, got[1].Model)
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

// TestWarningMentionsZCode pins the actionable stderr line (spec §8 style).
func TestWarningMentionsZCode(t *testing.T) {
	a := &Adapter{dir: buildFixtureDB(t), locked: true}
	msg := a.Warning()
	if !strings.Contains(msg, "zcode") {
		t.Errorf("warning should name the agent, got %q", msg)
	}
	if strings.ContainsAny(msg[:1], "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Errorf("warning should be lowercase: %q", msg)
	}
}
