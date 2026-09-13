package render

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/pastlog/pastlog/internal/agentlog"
)

// TestMain pins colors off so golden output is deterministic regardless of
// the host terminal (CI, piped output, developer TTY).
func TestMain(m *testing.M) {
	color.NoColor = true
	m.Run()
}

func mustTime(t *testing.T, s string, loc *time.Location) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation(time.DateTime, s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// sampleSessions uses wall-clock times in the local zone: the human renderer
// formats in local time, so the golden below is machine-independent.
func sampleSessions(t *testing.T) []agentlog.SessionMeta {
	t.Helper()
	loc := time.Local
	return []agentlog.SessionMeta{
		{
			Session: agentlog.Session{
				ID:        "3f9c81a2-1111-4222-8333-cccccccccccc",
				Agent:     "claude-code",
				Project:   "/home/dev/myapp",
				Title:     "Fix jwt refresh token rotation",
				StartedAt: mustTime(t, "2026-08-02 14:03:22", loc),
				EndedAt:   mustTime(t, "2026-08-02 14:04:05", loc),
				SizeBytes: 2489,
			},
			Messages: 4,
		},
		{
			Session: agentlog.Session{
				ID:        "aaa2b3c4-0000-4000-8000-000000000002",
				Agent:     "claude-code",
				Project:   "/home/dev/app",
				StartedAt: mustTime(t, "2026-07-01 10:00:00", loc),
				SizeBytes: 427,
			},
			// double-digit count guards the messages-column width (regression)
			Messages: 12,
		},
	}
}

// sampleSessionsUTC feeds the JSON tests: RFC3339 rendering of UTC instants
// is machine-independent.
func sampleSessionsUTC(t *testing.T) []agentlog.SessionMeta {
	t.Helper()
	loc := time.UTC
	return []agentlog.SessionMeta{
		{
			Session: agentlog.Session{
				ID:        "3f9c81a2-1111-4222-8333-cccccccccccc",
				Agent:     "claude-code",
				Project:   "/home/dev/myapp",
				Title:     "Fix jwt refresh token rotation",
				StartedAt: mustTime(t, "2026-08-02 14:03:22", loc),
				EndedAt:   mustTime(t, "2026-08-02 14:04:05", loc),
				SizeBytes: 2489,
			},
			Messages: 4,
		},
		{
			Session: agentlog.Session{
				ID:        "aaa2b3c4-0000-4000-8000-000000000002",
				Agent:     "claude-code",
				Project:   "/home/dev/app",
				StartedAt: mustTime(t, "2026-07-01 10:00:00", loc),
				SizeBytes: 427,
			},
			Messages: 2,
		},
	}
}

func TestAgentsHuman(t *testing.T) {
	rows := []AgentRow{
		{Name: "claude-code", Detected: true, Path: "/home/dev/.claude/projects", Sessions: 3, Bytes: 2489},
		{Name: "codex", Detected: false},
	}
	out := &bytes.Buffer{}
	AgentsHuman(out, "/home/dev", rows)
	want := `claude-code  3 sessions  2.4 KB  ~/.claude/projects
codex        0 sessions  (not found)
`
	if got := out.String(); got != want {
		t.Errorf("agents human output:\n%q\nwant:\n%q", got, want)
	}
}

func TestAgentsHumanSingular(t *testing.T) {
	rows := []AgentRow{
		{Name: "claude-code", Detected: true, Path: "/h/.claude/projects", Sessions: 1, Bytes: 42},
	}
	out := &bytes.Buffer{}
	AgentsHuman(out, "/h", rows)
	want := "claude-code  1 session   42 B  ~/.claude/projects\n"
	if got := out.String(); got != want {
		t.Errorf("singular output:\n%q\nwant:\n%q", got, want)
	}
}

func TestAgentsHumanEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	AgentsHuman(out, "", nil)
	if out.Len() != 0 {
		t.Errorf("empty rows should print nothing, got %q", out.String())
	}
}

func TestAgentsJSON(t *testing.T) {
	rows := []AgentRow{
		{Name: "claude-code", Detected: true, Path: "/home/dev/.claude/projects", Sessions: 3, Bytes: 2489},
		{Name: "codex", Detected: false},
	}
	out := &bytes.Buffer{}
	if err := AgentsJSON(out, rows); err != nil {
		t.Fatal(err)
	}
	want := `[
  {
    "name": "claude-code",
    "detected": true,
    "path": "/home/dev/.claude/projects",
    "sessions": 3,
    "bytes": 2489
  },
  {
    "name": "codex",
    "detected": false,
    "path": null,
    "sessions": 0,
    "bytes": 0
  }
]
`
	if got := out.String(); got != want {
		t.Errorf("agents json output:\n%q\nwant:\n%q", got, want)
	}
}

func TestSessionsHuman(t *testing.T) {
	out := &bytes.Buffer{}
	SessionsHuman(out, "/home/dev", sampleSessions(t))
	want := `claude-code  ~/myapp  2026-08-02 14:03   4 messages  2.4 KB  3f9c81a2
claude-code  ~/app    2026-07-01 10:00  12 messages   427 B  aaa2b3c4
`
	if got := out.String(); got != want {
		t.Errorf("sessions human output:\n%q\nwant:\n%q", got, want)
	}
}

func TestSessionsHumanEdgeValues(t *testing.T) {
	out := &bytes.Buffer{}
	SessionsHuman(out, "/home/dev", []agentlog.SessionMeta{
		{Session: agentlog.Session{ID: "abcdefgh-1234", Agent: "claude-code"}}, // no project, no dates
	})
	want := "claude-code  -  -  0 messages  0 B  abcdefgh\n"
	if got := out.String(); got != want {
		t.Errorf("edge output:\n%q\nwant:\n%q", got, want)
	}
}

func TestSessionsJSON(t *testing.T) {
	out := &bytes.Buffer{}
	if err := SessionsJSON(out, sampleSessionsUTC(t)); err != nil {
		t.Fatal(err)
	}
	want := `[
  {
    "id": "3f9c81a2-1111-4222-8333-cccccccccccc",
    "agent": "claude-code",
    "project": "/home/dev/myapp",
    "title": "Fix jwt refresh token rotation",
    "started_at": "2026-08-02T14:03:22Z",
    "ended_at": "2026-08-02T14:04:05Z",
    "messages": 4,
    "size_bytes": 2489
  },
  {
    "id": "aaa2b3c4-0000-4000-8000-000000000002",
    "agent": "claude-code",
    "project": "/home/dev/app",
    "title": "",
    "started_at": "2026-07-01T10:00:00Z",
    "ended_at": null,
    "messages": 2,
    "size_bytes": 427
  }
]
`
	if got := out.String(); got != want {
		t.Errorf("sessions json output:\n%q\nwant:\n%q", got, want)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{2489, "2.4 KB"},
		{1048576, "1.0 MB"},
		{1536918576, "1.4 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tt := range tests {
		if got := HumanBytes(tt.n); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestTildePath(t *testing.T) {
	tests := []struct {
		home, path, want string
	}{
		{"/home/dev", "/home/dev/.claude/projects", "~/.claude/projects"},
		{"/home/dev", "/home/dev", "~"},
		{"/home/dev", "/etc/passwd", "/etc/passwd"},
		{"", "/x/y", "/x/y"},
		{"C:\\Users\\dev", "C:\\Users\\dev\\.claude", "~/.claude"},
	}
	for _, tt := range tests {
		if got := tildePath(tt.home, tt.path); got != tt.want {
			t.Errorf("tildePath(%q, %q) = %q, want %q", tt.home, tt.path, got, tt.want)
		}
	}
}
