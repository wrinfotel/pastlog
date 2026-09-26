package render

import (
	"bytes"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// sampleUsageRows uses wall-clock times in the local zone: the day grouping
// formats in local time, so the goldens below are machine-independent.
func sampleUsageRows(t *testing.T) []agentlog.SessionUsage {
	t.Helper()
	loc := time.Local
	return []agentlog.SessionUsage{
		{
			Session: agentlog.Session{
				ID:        "a1",
				Agent:     "claude-code",
				Project:   "/home/dev/myapp",
				StartedAt: mustTime(t, "2026-08-02 14:03:22", loc),
			},
			Messages: 4,
			Usage: agentlog.Usage{
				Input: 1234567, Output: 950, Reasoning: 120,
				CacheRead: 2000, CacheWrite: 520, Model: "claude-sonnet-4-5",
			},
		},
		{
			Session: agentlog.Session{
				ID:        "a2",
				Agent:     "claude-code",
				Project:   "/home/dev/api",
				StartedAt: mustTime(t, "2026-08-01 09:00:00", loc),
			},
			Messages: 2,
			Usage:    agentlog.Usage{Input: 200, Output: 100, Model: "claude-sonnet-4-5"},
		},
		{
			Session: agentlog.Session{
				ID:        "o1",
				Agent:     "opencode",
				Project:   "C:/dev/fixture app",
				StartedAt: mustTime(t, "2026-08-03 10:00:00", loc),
			},
			Messages: 3,
			Usage: agentlog.Usage{
				Input: 50, Output: 20, Reasoning: 5, CacheWrite: 10,
				CostUSD: 0.42, HasCost: true, Model: "qwen3-coder-480b",
			},
		},
	}
}

func TestGroupStatsByAgent(t *testing.T) {
	rows := GroupStats("/home/dev", StatsByAgent, sampleUsageRows(t))
	if len(rows) != 2 {
		t.Fatalf("got %d groups, want 2", len(rows))
	}
	if rows[0].Key != "claude-code" || rows[1].Key != "opencode" {
		t.Errorf("keys = %q, %q; want ascending order", rows[0].Key, rows[1].Key)
	}
	c := rows[0]
	if c.Sessions != 2 || c.Messages != 6 {
		t.Errorf("claude-code sessions/messages = %d/%d, want 2/6", c.Sessions, c.Messages)
	}
	if c.Usage.Input != 1234767 || c.Usage.Output != 1050 || c.Usage.Reasoning != 120 ||
		c.Usage.CacheRead != 2000 || c.Usage.CacheWrite != 520 {
		t.Errorf("claude-code tokens = %+v, want 1234767/1050/120/2000/520", c.Usage)
	}
	if c.Usage.HasCost {
		t.Errorf("claude-code group must have no cost")
	}
	o := rows[1]
	if o.Sessions != 1 || o.Messages != 3 {
		t.Errorf("opencode sessions/messages = %d/%d, want 1/3", o.Sessions, o.Messages)
	}
	if !o.Usage.HasCost || o.Usage.CostUSD != 0.42 {
		t.Errorf("opencode cost = %f (has=%v), want 0.42 (has=true)", o.Usage.CostUSD, o.Usage.HasCost)
	}
}

func TestGroupStatsByProject(t *testing.T) {
	rows := GroupStats("/home/dev", StatsByProject, sampleUsageRows(t))
	// ascending on the display key; "C:…" sorts before the tilde paths
	want := []string{"C:/dev/fixture app", "~/api", "~/myapp"}
	if len(rows) != len(want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
	for i, w := range want {
		if rows[i].Key != w {
			t.Errorf("key[%d] = %q, want %q", i, rows[i].Key, w)
		}
	}
	// ~/myapp holds only a1 here (a2 lives in ~/api): 1234567
	if rows[0].Usage.Input != 50 || rows[1].Usage.Input != 200 || rows[2].Usage.Input != 1234567 {
		t.Errorf("per-project inputs drifted: %d/%d/%d, want 50/200/1234567", rows[0].Usage.Input, rows[1].Usage.Input, rows[2].Usage.Input)
	}
}

func TestGroupStatsByDay(t *testing.T) {
	rows := GroupStats("", StatsByDay, sampleUsageRows(t))
	want := []string{"2026-08-01", "2026-08-02", "2026-08-03"} // ascending = chronological
	if len(rows) != len(want) {
		t.Fatalf("got %d groups, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if rows[i].Key != w {
			t.Errorf("key[%d] = %q, want %q", i, rows[i].Key, w)
		}
	}
	if rows[0].Usage.Input != 200 || rows[1].Usage.Input != 1234567 || rows[2].Usage.Input != 50 {
		t.Errorf("per-day inputs drifted: %d/%d/%d", rows[0].Usage.Input, rows[1].Usage.Input, rows[2].Usage.Input)
	}
}

func TestGroupStatsByModel(t *testing.T) {
	rows := GroupStats("", StatsByModel, sampleUsageRows(t))
	want := []string{"claude-sonnet-4-5", "qwen3-coder-480b"}
	if len(rows) != len(want) {
		t.Fatalf("got %d groups, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if rows[i].Key != w {
			t.Errorf("key[%d] = %q, want %q", i, rows[i].Key, w)
		}
	}
	if rows[0].Sessions != 2 || rows[0].Messages != 6 {
		t.Errorf("model group sessions/messages = %d/%d, want 2/6", rows[0].Sessions, rows[0].Messages)
	}
}

// TestGroupStatsDashKeys pins the "-" placeholders: project "-" when empty,
// day "-" for a zero start time, model "-" when unknown — and that empty-key
// rows still count sessions/messages/tokens.
func TestGroupStatsDashKeys(t *testing.T) {
	rows := []agentlog.SessionUsage{
		{Session: agentlog.Session{ID: "bare", Agent: "codex"}, Messages: 1, Usage: agentlog.Usage{Input: 9}},
	}
	byProject := GroupStats("", StatsByProject, rows)
	if len(byProject) != 1 || byProject[0].Key != "-" || byProject[0].Usage.Input != 9 {
		t.Errorf("empty project should group under \"-\", got %+v", byProject)
	}
	byDay := GroupStats("", StatsByDay, rows)
	if len(byDay) != 1 || byDay[0].Key != "-" {
		t.Errorf("zero start time should group under \"-\", got %+v", byDay)
	}
	byModel := GroupStats("", StatsByModel, rows)
	if len(byModel) != 1 || byModel[0].Key != "-" {
		t.Errorf("empty model should group under \"-\", got %+v", byModel)
	}
}

// TestGroupStatsCostAggregation pins ruling 5: only sessions with a cost
// contribute; a group mixing known and unknown costs sums the known ones and
// reports HasCost; a group without any cost reports HasCost=false.
func TestGroupStatsCostAggregation(t *testing.T) {
	rows := []agentlog.SessionUsage{
		{Session: agentlog.Session{ID: "a", Agent: "opencode"}, Usage: agentlog.Usage{CostUSD: 0.42, HasCost: true}},
		{Session: agentlog.Session{ID: "b", Agent: "opencode"}, Usage: agentlog.Usage{CostUSD: 0.08, HasCost: true}},
	}
	got := GroupStats("", StatsByAgent, rows)
	if got[0].Usage.CostUSD != 0.5 || !got[0].Usage.HasCost {
		t.Errorf("cost sum = %f (has=%v), want 0.5 (has=true)", got[0].Usage.CostUSD, got[0].Usage.HasCost)
	}

	mixed := append(append([]agentlog.SessionUsage{}, rows...),
		agentlog.SessionUsage{Session: agentlog.Session{ID: "c", Agent: "opencode"}, Usage: agentlog.Usage{Input: 1}})
	got = GroupStats("", StatsByAgent, mixed)
	if got[0].Usage.CostUSD != 0.5 || !got[0].Usage.HasCost || got[0].Sessions != 3 {
		t.Errorf("mixed group = cost %f (has=%v, sessions %d), want 0.5 (true, 3)",
			got[0].Usage.CostUSD, got[0].Usage.HasCost, got[0].Sessions)
	}

	noCost := GroupStats("", StatsByAgent, []agentlog.SessionUsage{
		{Session: agentlog.Session{ID: "x", Agent: "codex"}, Usage: agentlog.Usage{Input: 2}},
	})
	if noCost[0].Usage.HasCost {
		t.Error("a group without cost-carrying sessions must have HasCost=false")
	}
}

func TestGroupStatsEmpty(t *testing.T) {
	for _, by := range []string{StatsByAgent, StatsByProject, StatsByDay, StatsByModel} {
		if got := GroupStats("", by, nil); len(got) != 0 {
			t.Errorf("GroupStats(%q, nil) = %v, want empty", by, got)
		}
	}
}

func TestStatsHumanGolden(t *testing.T) {
	rows := GroupStats("/home/dev", StatsByAgent, sampleUsageRows(t))
	out := &bytes.Buffer{}
	StatsHuman(out, StatsByAgent, rows)
	want := `agent        sessions  messages      input  output  reasoning  cache read  cache write      total  cost usd
claude-code         2         6  1,234,767   1,050        120       2,000          520  1,235,937         -
opencode            1         3         50      20          5           0           10         75      0.42
`
	if got := out.String(); got != want {
		t.Errorf("stats human output:\n%q\nwant:\n%q", got, want)
	}
}

// TestStatsHumanNoCostColumn pins that the `cost usd` column appears ONLY
// when at least one aggregated session carries a cost.
func TestStatsHumanNoCostColumn(t *testing.T) {
	rows := GroupStats("", StatsByAgent, []agentlog.SessionUsage{
		{Session: agentlog.Session{ID: "x", Agent: "codex"}, Messages: 1, Usage: agentlog.Usage{Input: 12, Output: 3}},
	})
	out := &bytes.Buffer{}
	StatsHuman(out, StatsByAgent, rows)
	want := `agent  sessions  messages  input  output  reasoning  cache read  cache write  total
codex         1         1     12       3          0           0            0     15
`
	if got := out.String(); got != want {
		t.Errorf("stats human without cost column:\n%q\nwant:\n%q", got, want)
	}
}

// TestStatsHumanThousands pins the separator helper on its own and through
// the renderer (column widths must count the commas).
func TestGroupDigits(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{12345678901, "12,345,678,901"},
		{-42, "-42"},
		{-1000, "-1,000"},
	}
	for _, tt := range tests {
		if got := groupDigits(tt.n); got != tt.want {
			t.Errorf("groupDigits(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestStatsHumanEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	StatsHuman(out, StatsByAgent, nil)
	if out.Len() != 0 {
		t.Errorf("empty rows should print nothing (sessions behavior), got %q", out.String())
	}
}

func TestStatsJSONGolden(t *testing.T) {
	rows := GroupStats("/home/dev", StatsByAgent, sampleUsageRows(t))
	out := &bytes.Buffer{}
	if err := StatsJSON(out, rows); err != nil {
		t.Fatal(err)
	}
	want := `[
  {
    "key": "claude-code",
    "sessions": 2,
    "messages": 6,
    "tokens": {
      "input": 1234767,
      "output": 1050,
      "reasoning": 120,
      "cache_read": 2000,
      "cache_write": 520,
      "total": 1235937
    },
    "cost_usd": null
  },
  {
    "key": "opencode",
    "sessions": 1,
    "messages": 3,
    "tokens": {
      "input": 50,
      "output": 20,
      "reasoning": 5,
      "cache_read": 0,
      "cache_write": 10,
      "total": 75
    },
    "cost_usd": 0.42
  }
]
`
	if got := out.String(); got != want {
		t.Errorf("stats json output:\n%q\nwant:\n%q", got, want)
	}
}

func TestStatsJSONEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	if err := StatsJSON(out, nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[]\n" {
		t.Errorf("empty selection should print [], got %q", got)
	}
}

// TestGroupStatsByModelSplitsMultiModelSessions pins the per-model split
// (TASK.md backlog): a session that switched models mid-way contributes one
// row per used model with that model's own tokens, and the sessions/messages
// counters on a model row mean "sessions that used this model" — a session
// with a split shows up in every model row it fed.
func TestGroupStatsByModelSplitsMultiModelSessions(t *testing.T) {
	rows := []agentlog.SessionUsage{
		{
			Session: agentlog.Session{
				ID:        "multi",
				Agent:     "zcode",
				StartedAt: mustTime(t, "2026-08-02 10:00:00", time.Local),
			},
			Messages: 3,
			// session totals stay the sum across models; Model is the latest
			Usage: agentlog.Usage{
				Input: 150, Output: 30, Reasoning: 5,
				CacheRead: 280, CacheWrite: 30, Model: "glm-5.3-flash",
			},
			Models: []agentlog.Usage{
				// earliest request first: the adapter yields models in
				// first-use order
				{Input: 100, Output: 20, Reasoning: 5, CacheRead: 200, CacheWrite: 30, Model: "gpt-6-astra"},
				{Input: 50, Output: 10, CacheRead: 80, Model: "glm-5.3-flash"},
			},
		},
	}
	got := GroupStats("", StatsByModel, rows)
	if len(got) != 2 {
		t.Fatalf("got %d model rows, want 2 (one per used model)", len(got))
	}
	if got[0].Key != "glm-5.3-flash" || got[1].Key != "gpt-6-astra" {
		t.Fatalf("keys = %q, %q; want ascending glm-5.3-flash then gpt-6-astra", got[0].Key, got[1].Key)
	}
	glm, astra := got[0], got[1]
	// "sessions that used this model": the split session counts on both rows
	if glm.Sessions != 1 || glm.Messages != 3 || astra.Sessions != 1 || astra.Messages != 3 {
		t.Errorf("sessions/messages = %d/%d and %d/%d, want 1/3 on both model rows",
			glm.Sessions, glm.Messages, astra.Sessions, astra.Messages)
	}
	// tokens come from the breakdown entry, not the session total
	if glm.Usage.Input != 50 || glm.Usage.Output != 10 || glm.Usage.Reasoning != 0 ||
		glm.Usage.CacheRead != 80 || glm.Usage.CacheWrite != 0 {
		t.Errorf("glm row tokens = %+v, want in 50 out 10 reasoning 0 cr 80 cw 0", glm.Usage)
	}
	if astra.Usage.Input != 100 || astra.Usage.Output != 20 || astra.Usage.Reasoning != 5 ||
		astra.Usage.CacheRead != 200 || astra.Usage.CacheWrite != 30 {
		t.Errorf("astra row tokens = %+v, want in 100 out 20 reasoning 5 cr 200 cw 30", astra.Usage)
	}
}

// TestGroupStatsMultiModelDoesNotInflateOtherViews pins the other side of the
// backlog ruling: outside --by model a split session still counts once, and
// its tokens are the session total — agent/project/day views are unchanged.
func TestGroupStatsMultiModelDoesNotInflateOtherViews(t *testing.T) {
	rows := []agentlog.SessionUsage{
		{
			Session:  agentlog.Session{ID: "multi", Agent: "zcode"},
			Messages: 3,
			Usage: agentlog.Usage{
				Input: 150, Output: 30, Reasoning: 5,
				CacheRead: 280, CacheWrite: 30, Model: "glm-5.3-flash",
			},
			Models: []agentlog.Usage{
				{Input: 100, Output: 20, Reasoning: 5, CacheRead: 200, CacheWrite: 30, Model: "gpt-6-astra"},
				{Input: 50, Output: 10, CacheRead: 80, Model: "glm-5.3-flash"},
			},
		},
	}
	for _, by := range []string{StatsByAgent, StatsByProject, StatsByDay} {
		got := GroupStats("", by, rows)
		if len(got) != 1 {
			t.Fatalf("--by %s: got %d rows, want 1 (a split session is still one session)", by, len(got))
		}
		if got[0].Sessions != 1 || got[0].Messages != 3 {
			t.Errorf("--by %s: sessions/messages = %d/%d, want 1/3", by, got[0].Sessions, got[0].Messages)
		}
		if got[0].Usage.Input != 150 || got[0].Usage.Output != 30 {
			t.Errorf("--by %s: tokens = in %d out %d, want the session totals 150/30", by, got[0].Usage.Input, got[0].Usage.Output)
		}
	}
}
