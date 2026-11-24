package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/wrinfotel/pastlog/internal/agentlog"
)

func TestCompressedAndArchivedRollouts(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".codex")
	prefix := "rollout-2026-11-21T10-00-00-"
	parent := `{"type":"session_meta","timestamp":"2026-11-21T10:00:00Z","payload":{"id":"parent-id","cwd":"/demo"}}`
	child := `{"type":"session_meta","timestamp":"2026-11-21T10:00:00Z","payload":{"id":"child-id","cwd":"/demo","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-id"}}}}}`
	text := `{"type":"response_item","payload":{"type":"message","role":"user","content":"sample transcript"}}`
	fixtures := map[string]string{
		filepath.Join(base, "sessions", "2026", "11", "21", prefix+"parent.jsonl.zst"): parent,
		filepath.Join(base, "archived_sessions", prefix+"child.jsonl.zst"):             child,
		filepath.Join(base, "archived_sessions", prefix+"plain.jsonl"):                 strings.ReplaceAll(parent, "parent-id", "archived-plain"),
		filepath.Join(base, "archived_sessions", prefix+"parent.jsonl"):                parent,
	}
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	for path, head := range fixtures {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		data := []byte(head + "\n" + text + "\n")
		if strings.HasSuffix(path, ".zst") {
			data = enc.EncodeAll(data, nil)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New(home)
	metas := listMetas(t, a)
	if len(metas) != 3 {
		t.Fatalf("got %d sessions, want 3", len(metas))
	}
	var fast []agentlog.SessionMeta
	_, err = New(home).SessionsMetaFast(func(m agentlog.SessionMeta) error { fast = append(fast, m); return nil })
	if err != nil || len(fast) != len(metas) {
		t.Fatalf("fast listing = %+v, %v", fast, err)
	}
	for i, m := range metas {
		if fast[i].ID != m.ID || fast[i].ParentID != m.ParentID {
			t.Errorf("fast = %+v, full = %+v", fast[i], m)
		}
		if m.ID == "child-id" && m.ParentID != "parent-id" {
			t.Errorf("child parent = %q", m.ParentID)
		}
		var entries []agentlog.Entry
		if err := New(home).Entries(m.Session, func(e agentlog.Entry) error { entries = append(entries, e); return nil }); err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Text != "sample transcript" {
			t.Errorf("entries = %+v", entries)
		}
		events, err := New(home).ContextEvents(m.Session)
		if err != nil || len(events) != 1 || events[0].Kind != agentlog.CtxMessage {
			t.Errorf("context = %+v, %v", events, err)
		}
	}
}

func TestModelTotalsUseLatestSeriesAndHandleCorrections(t *testing.T) {
	a := writeRolloutLines(t,
		`{"type":"session_meta","payload":{"id":"test-id"}}`,
		`{"type":"turn_context","payload":{"model":"first"}}`,
		`{"type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":99,"output_tokens":20}}}}`,
		`{"type":"token_usage_record","payload":{"thread_token_usage":{"input_tokens":100,"output_tokens":20,"reasoning_output_tokens":5}}}`,
		`{"type":"turn_context","payload":{"model":"second"}}`,
		`{"type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":150,"output_tokens":55}}}}`,
		`{"type":"token_usage_record","payload":{"thread_token_usage":{"input_tokens":160,"output_tokens":60,"reasoning_output_tokens":15}}}`,
		`{"type":"token_usage_record","payload":{"thread_token_usage":{"input_tokens":140,"output_tokens":50}}}`,
	)
	row := listUsage(t, a)[0]
	if row.Input != 140 || row.TotalTokens() != 190 || len(row.Models) != 2 {
		t.Fatalf("usage = %+v, want 140 input, 190 total and two models", row)
	}
	if row.Models[0].Input != 100 || row.Models[0].TotalTokens() != 120 ||
		row.Models[1].Input != 40 || row.Models[1].TotalTokens() != 70 || row.Models[1].Reasoning != 10 {
		t.Errorf("models = %+v, want corrected usage from thread snapshots", row.Models)
	}
}

func TestRateLimitOnlyRecordAndRepeatedTotals(t *testing.T) {
	a := writeRolloutLines(t,
		`{"type":"session_meta","payload":{"id":"test-id","cwd":"/demo"}}`,
		`{"type":"turn_context","payload":{"model":"first"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20,"reasoning_output_tokens":5},"last_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20,"reasoning_output_tokens":5}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20,"reasoning_output_tokens":5},"last_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20,"reasoning_output_tokens":5}}}}`,
		`{"type":"turn_context","payload":{"model":"second"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":140,"cached_input_tokens":100,"output_tokens":25,"reasoning_output_tokens":6},"last_token_usage":{"input_tokens":40,"cached_input_tokens":20,"output_tokens":5,"reasoning_output_tokens":1}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":null}}`,
		`{"type":"token_usage_record","payload":{"thread_token_usage":null}}`,
		`{"type":"compacted","payload":{}}`,
		`{"type":"response_item","payload":{"type":"configuration_update"}}`,
	)
	row := listUsage(t, a)[0]
	if row.Input != 140 || row.Output != 25 || row.CacheRead != 100 || row.Reasoning != 6 {
		t.Errorf("usage = %+v, want 140/25/100/6", row.Usage)
	}
	if len(row.Models) != 2 || row.Models[0].Input != 100 || row.Models[1].Input != 40 {
		t.Errorf("model allocation = %+v, want 100 and 40", row.Models)
	}
	if a.SkippedLines() != 0 {
		t.Errorf("valid bookkeeping caused %d skipped lines", a.SkippedLines())
	}
	events, err := a.ContextEvents(row.Session)
	if err != nil {
		t.Fatal(err)
	}
	var sums []int64
	for _, event := range events {
		if event.Kind == agentlog.CtxTurnStart {
			sums = append(sums, event.Tokens.Sum())
		}
	}
	if len(sums) != 2 || sums[0] != 100 || sums[1] != 40 {
		t.Errorf("context windows = %v, want [100 40]", sums)
	}
}
