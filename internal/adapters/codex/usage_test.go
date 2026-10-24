package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// writeRolloutLines writes the given lines into one rollout file inside a
// fresh home (defensive-case tests with inline records).
func writeRolloutLines(t *testing.T, lines ...string) *Adapter {
	t.Helper()
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "07", "01")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "rollout-2026-07-01T10-00-00-ddd0e1f2-7777-4777-8777-bbbb22223333.jsonl"),
		[]byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(home)
}

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

// TestSessionsUsageLastWins pins the M7 token semantics: total_token_usage
// values are cumulative over the rollout, so the LAST token_count record is
// the session's usage; the model comes from the LAST turn_context record.
// The fixture deliberately makes the later records strictly greater.
func TestSessionsUsageLastWins(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/08/02/rollout-2026-08-02T14-03-20-f3a7b8c9-3333-4333-8333-777777777777.jsonl": "usage.jsonl",
	})
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.ID != "f3a7b8c9-3333-4333-8333-777777777777" {
		t.Errorf("ID = %q", su.ID)
	}
	if su.Agent != "codex" {
		t.Errorf("Agent = %q", su.Agent)
	}
	// the LAST token_count (250/45/80/12), NOT the first (100/20/30/5) and
	// not a sum (350/65/110/17): values are cumulative
	want := agentlog.Usage{
		Input: 250, Output: 80, Reasoning: 12,
		CacheRead: 45, CacheWrite: 0,
		Model: "gpt-5.3-codex", // LAST turn_context, not the first
	}
	if su.Input != want.Input || su.Output != want.Output || su.Reasoning != want.Reasoning ||
		su.CacheRead != want.CacheRead || su.CacheWrite != want.CacheWrite {
		t.Errorf("tokens = in %d out %d reasoning %d cacheRead %d cacheWrite %d, want %+v (last total_token_usage wins)",
			su.Input, su.Output, su.Reasoning, su.CacheRead, su.CacheWrite, want)
	}
	if su.Model != want.Model {
		t.Errorf("Model = %q, want %q (last turn_context wins)", su.Model, want.Model)
	}
	if su.CostUSD != 0 || su.HasCost {
		t.Errorf("codex provides no cost, got %f/%v", su.CostUSD, su.HasCost)
	}
	if su.Messages != 1 { // same semantic as SessionsMeta: 1 user message
		t.Errorf("Messages = %d, want 1", su.Messages)
	}
	// the session bounds/size/project stay identical to SessionsMeta
	metas := listMetas(t, a)
	if su.Session != metas[0].Session {
		t.Errorf("SessionsUsage session fields drifted from SessionsMeta:\n%+v\nvs\n%+v", su.Session, metas[0].Session)
	}
}

// TestSessionsUsageWithoutUsage pins the zero case: a rollout without
// token_count/turn_context records yields zeroes and an empty model — and is
// still yielded (zero-usage sessions count in the aggregates).
func TestSessionsUsageWithoutUsage(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/01/rollout-2026-07-01T10-00-00-a5b8c9d0-4444-4444-8444-888888888888.jsonl": "no-usage.jsonl",
	})
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.Input != 0 || su.Output != 0 || su.Reasoning != 0 || su.CacheRead != 0 || su.CacheWrite != 0 {
		t.Errorf("usage should be all zero, got %+v", su.Usage)
	}
	if su.Model != "" {
		t.Errorf("Model = %q, want empty", su.Model)
	}
	if su.Messages != 1 {
		t.Errorf("Messages = %d, want 1 (zero-usage sessions still count messages)", su.Messages)
	}
}

// TestTokenCountAndTurnContextRecognizedSilently pins the M7 ruling (codex
// skip-count semantics change): token_count and turn_context records move
// from skip+counted to recognized-silent — no entries, NOT counted — while
// event_msg and unknown payload types stay skip+counted.
func TestTokenCountAndTurnContextRecognizedSilently(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/08/02/rollout-2026-08-02T14-03-20-f3a7b8c9-3333-4333-8333-777777777777.jsonl": "usage.jsonl",
	})
	_ = listUsage(t, a)
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0 (token_count/turn_context are recognized-silent)", a.SkippedLines())
	}
	// recognized-silent means no entries either: 1 message total
	metas := listMetas(t, a)
	if len(metas) != 1 || metas[0].Messages != 1 {
		t.Errorf("got %+v, want 1 session with 1 message (token/turn records add no entries)", metas)
	}
}

// TestTokenCountMalformedPayloadCountsSkipped pins that a token_count record
// whose payload is unusable stays in the skip+counted class (codex convention
// for known types with unusable payloads), while a payload without the
// total_token_usage object is readable and contributes zero.
func TestTokenCountMalformedPayloadCountsSkipped(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"id":"b6c9d0e1-5555-4555-8555-999999999999","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-07-01T10:00:05Z","type":"token_count","payload":{"info":42}}`,                                       // unusable payload shape
		`{"timestamp":"2026-07-01T10:00:10Z","type":"token_count","payload":{"info":{}}}`,                                       // no total_token_usage: zero, readable
		`{"timestamp":"2026-07-01T10:00:15Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":7}}}}`, // partial totals fill what they can
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].Input != 7 || got[0].Output != 0 {
		t.Errorf("Input = %d Output = %d, want 7/0 (missing fields are zero)", got[0].Input, got[0].Output)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (info:42 is an unusable payload)", a.SkippedLines())
	}
}

// TestTurnContextMalformedPayloadCountsSkipped pins the same for
// turn_context: an unusable payload skips and counts; a usable one is
// recognized-silent and carries the model.
func TestTurnContextMalformedPayloadCountsSkipped(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"id":"c7d0e1f2-6666-4666-8666-aaaa11112222","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-07-01T10:00:05Z","type":"turn_context","payload":"gpt"}`,        // unusable payload shape
		`{"timestamp":"2026-07-01T10:00:10Z","type":"turn_context","payload":{"cwd":"/x"}}`, // no model: readable, contributes none
		`{"timestamp":"2026-07-01T10:00:15Z","type":"turn_context","payload":{"model":"gpt-5.3"}}`,
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].Model != "gpt-5.3" {
		t.Errorf("Model = %q, want gpt-5.3", got[0].Model)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (payload \"gpt\" is an unusable payload)", a.SkippedLines())
	}
}

// newFormatLines is one rollout in the CURRENT codex CLI shape (observed on
// real 0.5x rollouts, 2026-10): token counts ride event_msg envelopes and
// token_usage_record lines, tool calls use custom_tool_call, and world_state /
// inter_agent_communication_metadata / item_completed lines interleave. Two
// turns, cumulative 100→250; every turn carries BOTH an event_msg token_count
// and a token_usage_record with identical values (the real writers emit both).
func newFormatLines() []string {
	return []string{
		`{"timestamp":"2026-10-24T10:00:00Z","ordinal":1,"type":"session_meta","payload":{"id":"1a2b3c4d-1111-4111-8111-111111111111","cwd":"/home/dev/app","cli_version":"0.50.0"}}`,
		`{"timestamp":"2026-10-24T10:00:01Z","ordinal":2,"type":"turn_context","payload":{"cwd":"/home/dev/app","model":"gpt-5.3-codex","effort":"medium"}}`,
		`{"timestamp":"2026-10-24T10:00:02Z","ordinal":3,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"count the tokens"}]}}`,
		`{"timestamp":"2026-10-24T10:00:03Z","ordinal":4,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":5},"last_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":5},"model_context_window":272000},"rate_limits":{"limit_id":"codex","plan_type":"plus"}}}`,
		`{"timestamp":"2026-10-24T10:00:04Z","ordinal":5,"type":"token_usage_record","payload":{"thread_id":"1a2b3c4d-1111-4111-8111-111111111111","turn_id":"turn-1","usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":5},"turn_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":5},"thread_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":30,"reasoning_output_tokens":5}}}`,
		`{"timestamp":"2026-10-24T10:00:05Z","ordinal":6,"type":"response_item","payload":{"type":"custom_tool_call","call_id":"call_1","name":"exec","input":"const r = await tools.exec_command({cmd:\"ls\"});\ntext(r.output);\n"}}`,
		`{"timestamp":"2026-10-24T10:00:06Z","ordinal":7,"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_1","output":[{"type":"input_text","text":"main.go"},{"type":"input_text","text":"go.mod"}]}}`,
		`{"timestamp":"2026-10-24T10:00:07Z","ordinal":8,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"custom_tool_call","call_id":"call_1"}}}`,
		`{"timestamp":"2026-10-24T10:00:08Z","ordinal":9,"type":"world_state","payload":{"threads":{}}}`,
		`{"timestamp":"2026-10-24T10:00:09Z","ordinal":10,"type":"inter_agent_communication_metadata","payload":{"source":"codex_cli"}}`,
		`{"timestamp":"2026-10-24T10:00:10Z","ordinal":11,"type":"turn_context","payload":{"cwd":"/home/dev/app","model":"gpt-5.3-codex","effort":"medium"}}`,
		`{"timestamp":"2026-10-24T10:00:11Z","ordinal":12,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":250,"cached_input_tokens":45,"output_tokens":80,"reasoning_output_tokens":12},"last_token_usage":{"input_tokens":150,"cached_input_tokens":25,"output_tokens":50,"reasoning_output_tokens":7},"model_context_window":272000}}}`,
		`{"timestamp":"2026-10-24T10:00:12Z","ordinal":13,"type":"token_usage_record","payload":{"thread_id":"1a2b3c4d-1111-4111-8111-111111111111","turn_id":"turn-2","usage":{"input_tokens":150,"cached_input_tokens":25,"output_tokens":50,"reasoning_output_tokens":7},"turn_token_usage":{"input_tokens":150,"cached_input_tokens":25,"output_tokens":50,"reasoning_output_tokens":7},"thread_token_usage":{"input_tokens":250,"cached_input_tokens":45,"output_tokens":80,"reasoning_output_tokens":12}}}`,
		`{"timestamp":"2026-10-24T10:00:13Z","ordinal":14,"type":"event_msg","payload":{"type":"task_complete","last_agent_message":"done"}}`,
		`{"timestamp":"2026-10-24T10:00:14Z","ordinal":15,"type":"response_item","payload":{"type":"agent_message","id":"amsg_1","author":"/root","recipient":"/root/helper","content":[{"type":"input_text","text":"Message Type: NEW_TASK"},{"type":"encrypted_content","encrypted_content":"gAAAA-expired"}]}}`,
		`{"timestamp":"2026-10-24T10:00:15Z","ordinal":16,"type":"response_item","payload":{"type":"additional_tools","role":"developer","tools":[{"type":"custom","name":"exec"}]}}`,
		`{"timestamp":"2026-10-24T10:00:16Z","ordinal":17,"type":"response_item","payload":{"type":"compaction","id":"cmp_1","encrypted_content":"gAAAA-expired"}}`,
	}
}

// TestSessionsUsageNewRolloutFormat pins the 0.2.6 regression fix: current
// codex rollouts carry usage in event_msg token_count envelopes and
// token_usage_record lines (the top-level token_count record is gone), so
// sessions showed all-zero usage. Totals are last-wins cumulative from either
// series; the per-model split takes deltas only from the token_count family
// (see observeDelta) — turn_token_usage never feeds it.
func TestSessionsUsageNewRolloutFormat(t *testing.T) {
	a := writeRolloutLines(t, newFormatLines()...)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.Input != 250 || su.CacheRead != 45 || su.Output != 80 || su.Reasoning != 12 {
		t.Errorf("tokens = in %d cacheRead %d out %d reasoning %d, want 250/45/80/12 (last cumulative wins)",
			su.Input, su.CacheRead, su.Output, su.Reasoning)
	}
	if su.Model != "gpt-5.3-codex" {
		t.Errorf("Model = %q, want gpt-5.3-codex (turn_context is still top-level)", su.Model)
	}
	if su.Messages != 1 {
		t.Errorf("Messages = %d, want 1", su.Messages)
	}
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0 (every line is a recognized shape)", a.SkippedLines())
	}
	// one delta source only: turn deltas 100+150=250, NOT 500
	if len(su.Models) != 1 {
		t.Fatalf("Models = %d entries, want 1 (single model)", len(su.Models))
	}
	m := su.Models[0]
	if m.Input != 250 || m.CacheRead != 45 || m.Output != 80 || m.Reasoning != 12 {
		t.Errorf("Models[0] = %+v, want 250/45/80/12 — deltas must not double-count the two usage series", m)
	}
}

// TestSessionsUsageRecordOnlyRollout pins the record series' role, measured
// on real rollouts: thread_token_usage is a cumulative totals source (so a
// rollout with records but no token_count envelopes still shows usage), but
// turn_token_usage is per-response gross usage — the same window is counted
// once per API call inside a turn, so it must never feed the per-model
// split (summing it overshoots the totals ~15×).
func TestSessionsUsageRecordOnlyRollout(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-10-24T10:00:00Z","type":"session_meta","payload":{"id":"2b3c4d5e-2222-4222-8222-222222222222","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-10-24T10:00:01Z","type":"turn_context","payload":{"model":"gpt-5.3-codex"}}`,
		`{"timestamp":"2026-10-24T10:00:02Z","type":"token_usage_record","payload":{"turn_token_usage":{"input_tokens":900000,"cached_input_tokens":10,"output_tokens":20},"thread_token_usage":{"input_tokens":90,"cached_input_tokens":10,"output_tokens":20}}}`,
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.Input != 90 || su.CacheRead != 10 || su.Output != 20 {
		t.Errorf("tokens = %+v, want 90/10/20 from thread_token_usage", su.Usage)
	}
	if len(su.Models) != 0 {
		t.Errorf("Models = %+v, want none (turn_token_usage never feeds the split)", su.Models)
	}
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0", a.SkippedLines())
	}
}

// TestEventMsgAndMetadataRecognizedSilently pins the skip-accounting ruling
// for the current format: event_msg envelopes (item_completed, task_started,
// task_complete, user_message, …), world_state and
// inter_agent_communication_metadata are recognized-silent — no entries, not
// counted — and their timestamps advance the session bounds like every
// classified record. An unusable event_msg payload and a genuinely unknown
// top-level type stay skip+counted.
func TestEventMsgAndMetadataRecognizedSilently(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-10-24T10:00:00Z","type":"session_meta","payload":{"id":"3c4d5e6f-3333-4333-8333-333333333333","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-10-24T10:00:01Z","type":"event_msg","payload":{"type":"task_started"}}`,
		`{"timestamp":"2026-10-24T10:00:02Z","type":"event_msg","payload":{"type":"user_message","message":"hi"}}`,
		`{"timestamp":"2026-10-24T10:00:03Z","type":"world_state","payload":{"anything":{"nested":true}}}`,
		`{"timestamp":"2026-10-24T10:00:04Z","type":"inter_agent_communication_metadata","payload":{}}`,
		`{"timestamp":"2026-10-24T10:00:05Z","type":"event_msg","payload":"not-an-object"}`, // unusable payload
		`{"timestamp":"2026-10-24T10:00:06Z","type":"future_unknown_type","payload":{}}`,    // unknown top-level type
	)
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].Messages != 0 {
		t.Errorf("Messages = %d, want 0 (event_msg user_message adds no entries — response_item stays the message source)", metas[0].Messages)
	}
	if want := "2026-10-24 10:00:04 +0000 UTC"; metas[0].EndedAt.String() != want {
		// 10:00:04 = the last USABLE recognized line; the two counted-skipped
		// lines after it contribute nothing, the silent ones before it do
		t.Errorf("EndedAt = %v, want %v (recognized-silent lines count toward the bounds)", metas[0].EndedAt, want)
	}
	if a.SkippedLines() != 2 {
		t.Errorf("SkippedLines = %d, want 2 (unusable event_msg payload + unknown top-level type)", a.SkippedLines())
	}
}

// TestCustomToolCallEntries pins the response_item payload types of the
// current format: custom_tool_call / custom_tool_call_output map to
// ToolCall / ToolResult like their function_call counterparts, with the
// output array flattened to its text items.
func TestCustomToolCallEntries(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-10-24T10:00:00Z","type":"session_meta","payload":{"id":"4d5e6f70-4444-4444-8444-444444444444","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-10-24T10:00:01Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call_1","name":"exec","input":"const r = await tools.exec_command({cmd:\"ls\"});\ntext(r.output);\n"}}`,
		`{"timestamp":"2026-10-24T10:00:02Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_1","output":[{"type":"input_text","text":"main.go\n"},{"type":"input_text","text":"go.mod"}]}}`,
		`{"timestamp":"2026-10-24T10:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call_2","name":"exec","input":""}}`, // empty input falls back to the name
		`{"timestamp":"2026-10-24T10:00:04Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_2","output":"plain string output"}}`,
	)
	var got []agentlog.Entry
	if err := a.Entries(agentlog.Session{ID: "4d5e6f70-4444-4444-8444-444444444444"}, func(e agentlog.Entry) error {
		got = append(got, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	type want struct {
		kind agentlog.EntryKind
		role string
		text string
	}
	wants := []want{
		{agentlog.ToolCall, "assistant", "const r = await tools.exec_command({cmd:\"ls\"});\ntext(r.output);"},
		{agentlog.ToolResult, "tool", "main.go\n\ngo.mod"},
		{agentlog.ToolCall, "assistant", "exec"},
		{agentlog.ToolResult, "tool", "plain string output"},
	}
	if len(got) != len(wants) {
		for i, e := range got {
			t.Logf("got[%d] kind=%d role=%q text=%.60q", i, e.Kind, e.Role, e.Text)
		}
		t.Fatalf("got %d entries, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		if got[i].Kind != w.kind || got[i].Role != w.role || got[i].Text != w.text {
			t.Errorf("entry[%d] = kind %d role %q text %.60q, want kind %d role %q text %.60q",
				i, got[i].Kind, got[i].Role, got[i].Text, w.kind, w.role, w.text)
		}
	}
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0", a.SkippedLines())
	}
}

// TestSessionsUsageModelBreakdown pins the per-model split (TASK.md
// backlog): each turn's last_token_usage delta lands on the model its
// turn_context named, in first-use order, and — on a healthy rollout where
// the deltas accumulate — the split sums to the last-wins session totals.
func TestSessionsUsageModelBreakdown(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/08/02/rollout-2026-08-02T14-03-20-f3a7b8c9-3333-4333-8333-777777777777.jsonl": "usage.jsonl",
	})
	su := listUsage(t, a)[0]
	want := []agentlog.Usage{
		// the first turn ran on gpt-5.3-mini, the second switched to
		// gpt-5.3-codex: each keeps its own delta
		{Model: "gpt-5.3-mini", Input: 100, Output: 30, Reasoning: 5, CacheRead: 20},
		{Model: "gpt-5.3-codex", Input: 150, Output: 50, Reasoning: 7, CacheRead: 25},
	}
	if len(su.Models) != len(want) {
		t.Fatalf("Models = %d entries, want %d", len(su.Models), len(want))
	}
	for i, w := range want {
		if su.Models[i] != w {
			t.Errorf("Models[%d] = %+v, want %+v", i, su.Models[i], w)
		}
	}
	// healthy rollout: the deltas accumulate to the last-wins totals
	if su.Models[0].Input+su.Models[1].Input != su.Input {
		t.Errorf("breakdown input %d does not sum to the session total %d",
			su.Models[0].Input+su.Models[1].Input, su.Input)
	}
	if su.Model != "gpt-5.3-codex" {
		t.Errorf("Model = %q, want the last turn_context model (split must not touch it)", su.Model)
	}
}
