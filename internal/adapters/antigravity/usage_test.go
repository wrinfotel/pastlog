package antigravity

import (
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

func TestSQLiteUsageAndContext(t *testing.T) {
	a := New(t.TempDir())
	const id = "aaaabbbb-1111-4111-8111-111111111111"
	logs := filepath.Join(a.root, "brain", id, ".system_generated", "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	trace := `{"type":"USER_INPUT","step_index":0,"created_at":"2026-06-28T09:54:31Z","content":"hello"}
{"type":"PLANNER_RESPONSE","step_index":2,"created_at":"2026-06-28T09:54:32Z","content":"one","input_tokens":777}
{"type":"CHECKPOINT","step_index":3,"created_at":"2026-06-28T09:54:33Z"}
{"type":"PLANNER_RESPONSE","step_index":5,"created_at":"2026-06-28T09:54:34Z","content":"two","input_tokens":888}`
	if err := os.WriteFile(filepath.Join(logs, "transcript.jsonl"), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := a.databasePath(id)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE gen_metadata (idx INTEGER PRIMARY KEY, data BLOB)`); err != nil {
		t.Fatal(err)
	}
	first := testUsage(60, 20, 40, 5)
	second := testUsage(50, 10, 4, 3)
	for i, raw := range [][]byte{
		testGeneration(2, "model-a", first, first, testUsage(10, 0, 0, 0)),
		testGeneration(5, "model-b", second, second),
	} {
		if _, err := db.Exec(`INSERT INTO gen_metadata VALUES (?,?)`, i, raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.SessionsUsage(func(s agentlog.SessionUsage) error {
		if s.ID != id || s.Messages != 3 || s.Input != 164 || s.Output != 30 || s.Reasoning != 8 || s.CacheRead != 44 || s.TotalTokens() != 194 || s.Model != "model-b" {
			t.Errorf("unexpected session usage: %+v", s)
		}
		if len(s.Models) != 2 || s.Models[0].TotalTokens() != 130 || s.Models[1].TotalTokens() != 64 {
			t.Errorf("unexpected per-model usage: %+v", s.Models)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	events, err := a.ContextEvents(agentlog.Session{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var windows []int64
	var order []agentlog.CtxKind
	for _, event := range events {
		order = append(order, event.Kind)
		if event.Kind == agentlog.CtxTurnStart {
			windows = append(windows, event.Tokens.Sum())
		}
	}
	if !slices.Equal(windows, []int64{100, 54}) {
		t.Errorf("context windows=%v, want [100 54]", windows)
	}
	want := []agentlog.CtxKind{agentlog.CtxMessage, agentlog.CtxTurnStart, agentlog.CtxMessage, agentlog.CtxCompact, agentlog.CtxTurnStart, agentlog.CtxMessage}
	if !slices.Equal(order, want) {
		t.Errorf("event order=%v, want %v", order, want)
	}
}

func TestSQLiteTranscriptFallback(t *testing.T) {
	a := New(t.TempDir())
	const id = "aaaabbbb-2222-4222-8222-222222222222"
	dbPath := a.databasePath(id)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE steps (idx INTEGER PRIMARY KEY, step_type INTEGER, metadata BLOB, step_payload BLOB)`); err != nil {
		t.Fatal(err)
	}
	user := testProtoBytes(nil, 19, testProtoBytes(nil, 2, []byte("input")))
	response := testProtoBytes(nil, 20, testProtoBytes(nil, 1, []byte("reply")))
	if _, err := db.Exec(`INSERT INTO steps VALUES (0, 14, NULL, ?), (1, 15, NULL, ?)`, user, response); err != nil {
		t.Fatal(err)
	}
	seen := 0
	if err := a.SessionsMeta(func(s agentlog.SessionMeta) error {
		seen++
		if s.ID != id || s.Messages != 2 || s.Title != "input" || s.SizeBytes <= 0 {
			t.Errorf("unexpected SQLite-only session: %+v", s)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Errorf("sessions = %d, want 1", seen)
	}
	var messages []string
	if err := a.Entries(agentlog.Session{ID: id}, func(e agentlog.Entry) error {
		messages = append(messages, e.Role+":"+e.Text)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(messages, []string{"user:input", "assistant:reply"}) {
		t.Errorf("unexpected SQLite transcript: %v", messages)
	}
}

func testUsage(input, output, cache, reasoning uint64) []byte {
	var data []byte
	for _, field := range []struct{ number, value uint64 }{{2, input}, {3, output}, {5, cache}, {9, reasoning}} {
		data = binary.AppendUvarint(data, field.number<<3)
		data = binary.AppendUvarint(data, field.value)
	}
	return data
}

func testGeneration(index uint64, model string, usage []byte, retries ...[]byte) []byte {
	modelData := testProtoBytes(nil, 19, []byte(model))
	modelData = testProtoBytes(modelData, 4, usage)
	for _, retry := range retries {
		modelData = testProtoBytes(modelData, 17, testProtoBytes(nil, 2, retry))
	}
	data := testProtoBytes(nil, 1, modelData)
	return testProtoBytes(data, 2, binary.AppendUvarint(nil, index))
}

func testProtoBytes(dest []byte, number uint64, value []byte) []byte {
	dest = binary.AppendUvarint(dest, number<<3|2)
	dest = binary.AppendUvarint(dest, uint64(len(value)))
	return append(dest, value...)
}
