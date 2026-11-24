package zcode

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/ctx"
)

func updateFixture(t *testing.T, dir, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, dbName)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestContextGrossInputIncludesCache(t *testing.T) {
	dir := buildFixtureDB(t)
	updateFixture(t, dir, `UPDATE model_usage SET input_tokens=420, output_tokens=20,
		cache_creation_input_tokens=30, cache_read_input_tokens=200,
		provider_total_tokens=440, computed_total_tokens=440 WHERE id='mru_fixture000001zc'`)
	for _, event := range ctxEvents(t, filepath.Join(dir, dbName), parentID) {
		if event.Kind != agentlog.CtxTurnStart {
			continue
		}
		if event.Tokens.Input != 190 || event.Tokens.CacheWrite != 30 || event.Tokens.CacheRead != 200 || event.Tokens.Sum() != 420 {
			t.Fatalf("first request tokens=%+v, window=%d, want 420 with cache counted once", event.Tokens, event.Tokens.Sum())
		}
		return
	}
	t.Fatal("no model request")
}

func TestContextIgnoresRequestsWithoutInputUsage(t *testing.T) {
	dir := t.TempDir()
	dbPath := writeCtxFixtureDB(t, dir)
	updateFixture(t, dir, `UPDATE model_usage SET status='error', input_tokens=0, output_tokens=0,
		reasoning_tokens=0, cache_creation_input_tokens=0, cache_read_input_tokens=0,
		computed_total_tokens=0 WHERE started_at=(SELECT MAX(started_at) FROM model_usage)`)
	updateFixture(t, dir, `UPDATE model_usage SET status='cancelled'
		WHERE started_at=(SELECT MIN(started_at) FROM model_usage)`)
	profile := ctx.Analyze(ctxEvents(t, dbPath, ctxSessionID))
	if profile.Turns != 7 || profile.Final != 18000 || !profile.FinalExact {
		t.Errorf("turns=%d final=%d exact=%t, want 7 turns and an exact final window of 18000", profile.Turns, profile.Final, profile.FinalExact)
	}
}

func TestUsageCountsReasoningOnceWhenIncludedInOutput(t *testing.T) {
	dir := buildFixtureDB(t)
	updateFixture(t, dir, `UPDATE model_usage SET input_tokens=420, output_tokens=20,
		provider_total_tokens=440, computed_total_tokens=440 WHERE id='mru_fixture000001zc'`)
	updateFixture(t, dir, `UPDATE model_usage SET reasoning_tokens=2,
		computed_total_tokens=142 WHERE id='mru_fixture000002zc'`)
	updateFixture(t, dir, `UPDATE model_usage SET input_tokens=25,
		computed_total_tokens=27 WHERE id='mru_fixture000003zc'`)
	rows := listUsage(t, NewDir(dir))
	if len(rows) != 2 {
		t.Fatalf("got %d sessions, want 2", len(rows))
	}
	parent, child := rows[0], rows[1]
	if parent.Input != 470 || parent.Reasoning != 7 || parent.TotalTokens() != 502 {
		t.Errorf("parent usage=%+v total=%d, want input=470 reasoning=7 total=502", parent.Usage, parent.TotalTokens())
	}
	if len(parent.Models) != 2 || parent.Models[0].TotalTokens() != 440 || parent.Models[1].TotalTokens() != 62 {
		t.Errorf("parent models=%+v, want totals 440 and 62", parent.Models)
	}
	if child.Reasoning != 1 || child.TotalTokens() != 27 {
		t.Errorf("child usage=%+v total=%d, want reasoning=1 total=27 with no provider total", child.Usage, child.TotalTokens())
	}
}

func TestLegacyUsageWithoutTotalColumns(t *testing.T) {
	dir := buildFixtureDB(t)
	updateFixture(t, dir, `ALTER TABLE model_usage DROP COLUMN provider_total_tokens`)
	updateFixture(t, dir, `ALTER TABLE model_usage DROP COLUMN computed_total_tokens`)
	rows := listUsage(t, NewDir(dir))
	if len(rows) != 2 || rows[0].TotalTokens() != 185 {
		t.Fatalf("legacy usage=%+v, want parent total 185", rows)
	}
	var windows []int64
	for _, event := range ctxEvents(t, filepath.Join(dir, dbName), parentID) {
		if event.Kind == agentlog.CtxTurnStart {
			windows = append(windows, event.Tokens.Sum())
		}
	}
	if len(windows) != 2 || windows[0] != 330 || windows[1] != 130 {
		t.Errorf("legacy windows=%v, want [330 130]", windows)
	}
}

func TestToolErrorTextInEntriesAndContext(t *testing.T) {
	dir := buildFixtureDB(t)
	const message = "command failed\nexit 1"
	updateFixture(t, dir, `UPDATE part SET data=? WHERE id=(SELECT id FROM part WHERE session_id=?
		AND json_valid(data) AND json_extract(data,'$.type')='tool' ORDER BY time_created, id LIMIT 1)`,
		`{"type":"tool","tool":"bash","state":{"status":"error","input":{},"error":"command failed\nexit 1"}}`, parentID)
	a := NewDir(dir)
	found := false
	if err := a.Entries(agentlog.Session{ID: parentID}, func(entry agentlog.Entry) error {
		found = found || (entry.Kind == agentlog.ToolResult && entry.Text == message)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("missing error result in transcript")
	}
	for _, event := range ctxEvents(t, filepath.Join(dir, dbName), parentID) {
		if event.Kind == agentlog.CtxToolResult && event.Tool == "bash" && event.Err && event.Head == "command failed" && event.ResBytes == len(message) {
			return
		}
	}
	t.Error("missing error result in context events")
}
