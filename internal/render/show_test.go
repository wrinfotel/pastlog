package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

func showMeta(t *testing.T, loc *time.Location) agentlog.SessionMeta {
	t.Helper()
	return agentlog.SessionMeta{
		Session: agentlog.Session{
			ID:        "aaaa1111-1111-4111-8111-111111111111",
			Agent:     "claude-code",
			Project:   "/home/dev/myapp",
			Title:     "Fix jwt refresh token rotation",
			StartedAt: mustTime(t, "2026-08-02 14:03:22", loc),
			EndedAt:   mustTime(t, "2026-08-02 14:03:25", loc),
			SizeBytes: 614,
		},
		Messages: 2,
	}
}

func showEntries(t *testing.T, loc *time.Location) []agentlog.Entry {
	t.Helper()
	return []agentlog.Entry{
		{Kind: agentlog.Summary, Text: "Fix jwt refresh token rotation"},
		{Kind: agentlog.Message, Role: "user", Text: "the refresh token is stored in localStorage", Timestamp: mustTime(t, "2026-08-02 14:03:22", loc)},
		{Kind: agentlog.Message, Role: "assistant", Text: "moving it to an httpOnly cookie", Timestamp: mustTime(t, "2026-08-02 14:03:25", loc)},
	}
}

func TestShowHuman(t *testing.T) {
	out := &bytes.Buffer{}
	ShowHuman(out, "/home/dev", showMeta(t, time.Local), showEntries(t, time.Local))
	want := `# Fix jwt refresh token rotation
claude-code · aaaa1111-1111-4111-8111-111111111111 · ~/myapp · 2026-08-02 14:03:22 → 2026-08-02 14:03:25 · 2 messages · 614 B

       -  summary    Fix jwt refresh token rotation
14:03:22  user       the refresh token is stored in localStorage
14:03:25  assistant  moving it to an httpOnly cookie
`
	if got := out.String(); got != want {
		t.Errorf("show human output:\n%s\nwant:\n%s", got, want)
	}
}

func TestShowHumanMultilineEntry(t *testing.T) {
	// continuation lines indent to the text column, which the role pre-pass
	// computes (assistant = 9 runes)
	meta := agentlog.SessionMeta{Session: agentlog.Session{
		ID: "bbbb2222-2222-4222-8222-222222222222", Agent: "claude-code",
		StartedAt: mustTime(t, "2026-08-02 10:00:00", time.Local),
	}}
	entries := []agentlog.Entry{
		{Kind: agentlog.Message, Role: "assistant", Text: "line one\nline two", Timestamp: mustTime(t, "2026-08-02 10:00:05", time.Local)},
	}
	out := &bytes.Buffer{}
	ShowHuman(out, "/home/dev", meta, entries)
	want := `# session bbbb2222
claude-code · bbbb2222-2222-4222-8222-222222222222 · - · 2026-08-02 10:00:00 · 0 messages · 0 B

10:00:05  assistant  line one
                     line two
`
	if got := out.String(); got != want {
		t.Errorf("multiline transcript:\n%s\nwant:\n%s", got, want)
	}
}

func TestShowJSON(t *testing.T) {
	out := &bytes.Buffer{}
	if err := ShowJSON(out, showMeta(t, time.UTC), showEntries(t, time.UTC)); err != nil {
		t.Fatal(err)
	}
	want := `{
  "id": "aaaa1111-1111-4111-8111-111111111111",
  "agent": "claude-code",
  "project": "/home/dev/myapp",
  "title": "Fix jwt refresh token rotation",
  "started_at": "2026-08-02T14:03:22Z",
  "ended_at": "2026-08-02T14:03:25Z",
  "messages": 2,
  "size_bytes": 614,
  "entries": [
    {
      "kind": "summary",
      "role": "",
      "text": "Fix jwt refresh token rotation",
      "timestamp": null
    },
    {
      "kind": "message",
      "role": "user",
      "text": "the refresh token is stored in localStorage",
      "timestamp": "2026-08-02T14:03:22Z"
    },
    {
      "kind": "message",
      "role": "assistant",
      "text": "moving it to an httpOnly cookie",
      "timestamp": "2026-08-02T14:03:25Z"
    }
  ]
}
`
	if got := out.String(); got != want {
		t.Errorf("show json output:\n%s\nwant:\n%s", got, want)
	}
}

func TestShowJSONToolKinds(t *testing.T) {
	entries := []agentlog.Entry{
		{Kind: agentlog.ToolCall, Role: "assistant", Text: `{"cmd":"ls"}`},
		{Kind: agentlog.ToolResult, Role: "tool", Text: "file list"},
	}
	out := &bytes.Buffer{}
	if err := ShowJSON(out, agentlog.SessionMeta{}, entries); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"kind": "tool_call"`, `"kind": "tool_result"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %s in:\n%s", want, out.String())
		}
	}
}

func TestShowMarkdown(t *testing.T) {
	out := &bytes.Buffer{}
	ShowMarkdown(out, showMeta(t, time.Local), showEntries(t, time.Local))
	want := `# Fix jwt refresh token rotation

- **agent:** claude-code
- **session:** aaaa1111-1111-4111-8111-111111111111
- **project:** /home/dev/myapp
- **started:** 2026-08-02 14:03:22
- **ended:** 2026-08-02 14:03:25
- **messages:** 2
- **size:** 614 B

## summary

Fix jwt refresh token rotation

## user · 2026-08-02 14:03:22

the refresh token is stored in localStorage

## assistant · 2026-08-02 14:03:25

moving it to an httpOnly cookie

`
	if got := out.String(); got != want {
		t.Errorf("show markdown output:\n%s\nwant:\n%s", got, want)
	}
}

func TestShowMarkdownTitlelessSession(t *testing.T) {
	meta := agentlog.SessionMeta{Session: agentlog.Session{
		ID: "bbbb2222-2222-4222-8222-222222222222", Agent: "codex",
	}}
	out := &bytes.Buffer{}
	ShowMarkdown(out, meta, nil)
	want := `# session bbbb2222

- **agent:** codex
- **session:** bbbb2222-2222-4222-8222-222222222222
- **messages:** 0
- **size:** 0 B

`
	if got := out.String(); got != want {
		t.Errorf("titleless markdown:\n%s\nwant:\n%s", got, want)
	}
}
