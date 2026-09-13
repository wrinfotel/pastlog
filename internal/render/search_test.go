package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/pastlog/pastlog/internal/agentlog"
	"github.com/pastlog/pastlog/internal/search"
)

func searchSampleUTC(t *testing.T) []search.Result {
	t.Helper()
	loc := time.UTC
	return []search.Result{
		{
			Session: agentlog.SessionMeta{
				Session: agentlog.Session{
					ID:        "3f9c81a2-1111-4222-8333-cccccccccccc",
					Agent:     "codex",
					Project:   "/home/dev/myapp/api",
					Title:     "the refresh token is stored in localStorage, is that safe?",
					StartedAt: mustTime(t, "2026-08-02 14:10:00", loc),
					SizeBytes: 2011,
				},
				Messages: 4,
			},
			Hits: []search.Hit{
				{
					Entry:      agentlog.Entry{Kind: agentlog.Message, Role: "user"},
					Line:       "jwt refresh keeps failing",
					MatchStart: 0,
					MatchEnd:   11,
				},
			},
		},
		{
			Session: agentlog.SessionMeta{
				Session: agentlog.Session{
					ID:        "aaaa1111-1111-4111-8111-111111111111",
					Agent:     "claude-code",
					Project:   "/home/dev/myapp",
					StartedAt: mustTime(t, "2026-08-02 14:03:22", loc),
					SizeBytes: 614,
				},
				Messages: 2,
			},
			Hits: []search.Hit{
				{
					Entry:      agentlog.Entry{Kind: agentlog.Message, Role: "assistant"},
					Line:       "fixing jwt refresh rotation now",
					MatchStart: 8,
					MatchEnd:   19,
				},
				{
					Entry:      agentlog.Entry{Kind: agentlog.Message, Role: "user"},
					Context:    "the refresh token is stored in localStorage",
					Line:       "we moved jwt refresh to a cookie",
					MatchStart: 9,
					MatchEnd:   20,
				},
			},
		},
	}
}

func TestSearchHuman(t *testing.T) {
	loc := time.Local
	results := searchSampleUTC(t)
	for i := range results { // human cards render in local time
		results[i].Session.StartedAt = results[i].Session.StartedAt.In(loc)
	}
	out := &bytes.Buffer{}
	SearchHuman(out, results)
	want := `codex · myapp/api · 2026-08-02 · sess 3f9c81a2
  → jwt refresh keeps failing

claude-code · dev/myapp · 2026-08-02 · sess aaaa1111
  → fixing jwt refresh rotation now
  the refresh token is stored in localStorage
  → we moved jwt refresh to a cookie
`
	if got := out.String(); got != want {
		t.Errorf("search human output:\n%s\nwant:\n%s", got, want)
	}
}

func TestSearchHumanEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	SearchHuman(out, nil)
	if out.Len() != 0 {
		t.Errorf("no results should print nothing, got %q", out.String())
	}
}

func TestSearchHumanSkipsDuplicateContext(t *testing.T) {
	// the context of a hit often is the previous hit's line; showing it
	// twice would be noise
	results := []search.Result{{
		Session: agentlog.SessionMeta{Session: agentlog.Session{
			ID: "bbbb2222-2222-4222-8222-222222222222", Agent: "codex", Project: "/p", StartedAt: mustTime(t, "2026-08-02 10:00:00", time.UTC),
		}},
		Hits: []search.Hit{
			{Line: "jwt one", MatchStart: 0, MatchEnd: 3},
			{Context: "jwt one", Line: "jwt two", MatchStart: 0, MatchEnd: 3},
		},
	}}
	out := &bytes.Buffer{}
	SearchHuman(out, results)
	want := "codex · p · 2026-08-02 · sess bbbb2222\n" +
		"  → jwt one\n" +
		"  → jwt two\n"
	if got := out.String(); got != want {
		t.Errorf("duplicate context not collapsed:\n%s\nwant:\n%s", got, want)
	}
}

func TestSearchHumanHighlightsMatch(t *testing.T) {
	results := []search.Result{{
		Session: agentlog.SessionMeta{Session: agentlog.Session{
			ID: "cccc3333-3333-4333-8333-333333333333", Agent: "codex", Project: "/p", StartedAt: mustTime(t, "2026-08-02 10:00:00", time.UTC),
		}},
		Hits: []search.Hit{{Line: "the jwt token", MatchStart: 4, MatchEnd: 7}},
	}}

	out := &bytes.Buffer{}
	// restore the global NoColor on every exit path — mutating package
	// globals leaks into parallel/later tests otherwise (test hygiene, M4)
	prevNoColor := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = prevNoColor })
	SearchHuman(out, results)
	if !strings.Contains(out.String(), "\x1b[1mjwt") {
		t.Errorf("match should be wrapped in the bold color, got %q", out.String())
	}

	color.NoColor = true
	plain := &bytes.Buffer{}
	SearchHuman(plain, results)
	if strings.Contains(plain.String(), "\x1b[") {
		t.Errorf("no color expected, got %q", plain.String())
	}
}

func TestShortProject(t *testing.T) {
	tests := []struct{ project, want string }{
		{"/home/dev/myapp/api", "myapp/api"},
		{"/home/dev/myapp", "dev/myapp"},
		{"C:\\Users\\dev\\myapp", "dev/myapp"},
		{"myapp", "myapp"},
		{"/", "-"},
		{"", "-"},
	}
	for _, tt := range tests {
		if got := shortProject(tt.project); got != tt.want {
			t.Errorf("shortProject(%q) = %q, want %q", tt.project, got, tt.want)
		}
	}
}

func TestShortDate(t *testing.T) {
	if got := shortDate(mustTime(t, "2026-08-02 14:03:22", time.Local)); got != "2026-08-02" {
		t.Errorf("shortDate = %q", got)
	}
	if got := shortDate(time.Time{}); got != "-" {
		t.Errorf("shortDate(zero) = %q, want -", got)
	}
}

func TestSearchJSON(t *testing.T) {
	out := &bytes.Buffer{}
	if err := SearchJSON(out, searchSampleUTC(t)); err != nil {
		t.Fatal(err)
	}
	want := `[
  {
    "session": {
      "id": "3f9c81a2-1111-4222-8333-cccccccccccc",
      "agent": "codex",
      "project": "/home/dev/myapp/api",
      "title": "the refresh token is stored in localStorage, is that safe?",
      "started_at": "2026-08-02T14:10:00Z",
      "ended_at": null,
      "messages": 4,
      "size_bytes": 2011
    },
    "hits": [
      {
        "kind": "message",
        "role": "user",
        "timestamp": null,
        "context": "",
        "line": "jwt refresh keeps failing",
        "match_start": 0,
        "match_end": 11
      }
    ]
  },
  {
    "session": {
      "id": "aaaa1111-1111-4111-8111-111111111111",
      "agent": "claude-code",
      "project": "/home/dev/myapp",
      "title": "",
      "started_at": "2026-08-02T14:03:22Z",
      "ended_at": null,
      "messages": 2,
      "size_bytes": 614
    },
    "hits": [
      {
        "kind": "message",
        "role": "assistant",
        "timestamp": null,
        "context": "",
        "line": "fixing jwt refresh rotation now",
        "match_start": 8,
        "match_end": 19
      },
      {
        "kind": "message",
        "role": "user",
        "timestamp": null,
        "context": "the refresh token is stored in localStorage",
        "line": "we moved jwt refresh to a cookie",
        "match_start": 9,
        "match_end": 20
      }
    ]
  }
]
`
	if got := out.String(); got != want {
		t.Errorf("search json output:\n%s\nwant:\n%s", got, want)
	}
}

func TestSearchJSONEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	if err := SearchJSON(out, nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[]\n" {
		t.Errorf("empty results should render [], got %q", got)
	}
}

func TestSearchJSONUnicodeOffsetsAreRunes(t *testing.T) {
	// "héllo jwt" — the match starts after a 2-byte rune: match_start must
	// count runes (6), not bytes (7)
	results := []search.Result{{
		Session: agentlog.SessionMeta{Session: agentlog.Session{
			ID: "dddd4444-4444-4444-8444-444444444444", Agent: "codex", StartedAt: mustTime(t, "2026-08-02 10:00:00", time.UTC),
		}},
		Hits: []search.Hit{{Line: "héllo jwt", MatchStart: 7, MatchEnd: 10}},
	}}
	out := &bytes.Buffer{}
	if err := SearchJSON(out, results); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"match_start": 6`) || !strings.Contains(out.String(), `"match_end": 9`) {
		t.Errorf("match offsets should be rune-based, got:\n%s", out.String())
	}
}
