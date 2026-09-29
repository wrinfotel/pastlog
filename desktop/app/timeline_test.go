package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Timeline fixtures: two claude sessions whose messages interleave
// chronologically, so the merged stream's order is observable.

func emptyFilterOpts() FilterOptions { return FilterOptions{} }

const (
	timelineHomeA = "uuuu1111-1111-4111-8111-111111111111"
	timelineHomeB = "uuuu2222-2222-4222-8222-222222222222"
)

// timelineContent carries a __TOKEN__ placeholder in the first session's
// user message; the masking test substitutes a realistic-shaped secret.
const timelineContent = `{"type":"user","sessionId":"__ID__","cwd":"/home/dev/app","timestamp":"2026-08-03T09:0__M__:00Z","message":{"role":"user","content":"__MSG__"}}
`

func writeTimelineSession(t *testing.T, home, id, minute, msg string) {
	t.Helper()
	content := strings.NewReplacer("__ID__", id, "__M__", minute, "__MSG__", msg).Replace(timelineContent)
	dst := filepath.Join(home, ".claude", "projects", "-home-dev-app", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func timelineHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	writeTimelineSession(t, home, timelineHomeA, "0", "first user message")
	writeTimelineSession(t, home, timelineHomeB, "1", "second session opens")
	return home
}

func TestTimelineMergesChronologically(t *testing.T) {
	a := homeApp(t, timelineHome(t))
	out, err := a.Timeline(emptyFilterOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("events = %d, want 2 (one per session)", len(out.Events))
	}
	if out.Events[0].Session != timelineHomeA || out.Events[0].Text != "first user message" {
		t.Errorf("first event = %+v, want session A's 09:00 message", out.Events[0])
	}
	if out.Events[1].Session != timelineHomeB || out.Events[1].Text != "second session opens" {
		t.Errorf("second event = %+v, want session B's 09:01 message", out.Events[1])
	}
	if out.Events[0].Agent != "claude-code" || out.Events[0].Role != "user" {
		t.Errorf("event header = %s/%s, want claude-code/user", out.Events[0].Agent, out.Events[0].Role)
	}
	if len(out.Notes) != 0 {
		t.Errorf("notes = %v, want none", out.Notes)
	}
}

func TestTimelineProjectFilter(t *testing.T) {
	a := homeApp(t, timelineHome(t))
	f := emptyFilterOpts()
	f.Project = "/home/dev/app"
	out, err := a.Timeline(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(out.Events))
	}
	f.Project = "/somewhere/else"
	out, err = a.Timeline(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 0 {
		t.Errorf("events = %d, want none outside the project", len(out.Events))
	}
}

func TestTimelineMasksByDefaultAndHonorsSetting(t *testing.T) {
	secret := "ghp_" + "0123456789abcdefABCD"
	home := t.TempDir()
	writeTimelineSession(t, home, timelineHomeA, "0", "my token is "+secret+" rotate")
	a := homeApp(t, home)

	out, err := a.Timeline(emptyFilterOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 || !strings.Contains(out.Events[0].Text, "ghp_…ABCD") {
		t.Errorf("masked timeline should render ghp_…ABCD, got %+v", out.Events)
	}
	if strings.Contains(out.Events[0].Text, secret) {
		t.Errorf("masked timeline must not carry the raw token: %+v", out.Events[0])
	}
	if _, err := a.SetMaskSecrets(false); err != nil {
		t.Fatal(err)
	}
	out, err = a.Timeline(emptyFilterOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 || !strings.Contains(out.Events[0].Text, secret) {
		t.Errorf("masking off must print the token verbatim, got %+v", out.Events)
	}
}

// cancelSink cancels the app at the first progress tick, exercising the
// generation check end to end (the sink contract main.go implements).
type cancelSink struct{ a *App }

func (s *cancelSink) Progress(op string, scanned, hits int) {
	s.a.Cancel()
}

func TestTimelineCancelKeepsPartial(t *testing.T) {
	dir := t.TempDir()
	sink := &cancelSink{}
	a := New(WithConfigDir(filepath.Join(t.TempDir(), "cfg")), WithSink(sink))
	sink.a = a // the sink cancels the app that owns it; wire after construction
	if _, err := a.SetHome(dir); err != nil {
		t.Fatal(err)
	}
	writeTimelineSession(t, dir, timelineHomeA, "0", "first user message")
	writeTimelineSession(t, dir, timelineHomeB, "1", "second session opens")

	out, err := a.Timeline(emptyFilterOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 {
		t.Errorf("cancel after the first session must keep 1 event, got %d", len(out.Events))
	}
	if len(out.Notes) == 0 || !strings.Contains(strings.Join(out.Notes, " "), "cancelled") {
		t.Errorf("notes must record the cancellation, got %v", out.Notes)
	}
}
