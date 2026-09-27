package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// relatedHome builds two claude-code sessions in the same project: no
// parent/children (claude-code records none), but the adjacency fires.
func relatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for i, id := range []string{
		"r1111111-1111-4111-8111-111111111111",
		"r2222222-2222-4222-8222-222222222222",
	} {
		content := fmt.Sprintf(`{"type":"user","sessionId":%q,"cwd":"/home/dev/app","timestamp":"2026-08-0%dT09:00:00Z","message":{"role":"user","content":"session %d"}}
`, id, i+1, i+1)
		dst := filepath.Join(home, ".claude", "projects", "-home-dev-app", id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestRelatedOutcomeAdjacent(t *testing.T) {
	a := homeApp(t, relatedHome(t))
	out, err := a.Related("r2222222")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %s, candidates = %d", out.Status, len(out.Candidates))
	}
	if out.Session == nil || out.Session.ID != "r2222222-2222-4222-8222-222222222222" {
		t.Fatalf("session = %+v", out.Session)
	}
	if out.Parent != nil {
		t.Errorf("parent = %+v, want nil for a top-level session", out.Parent)
	}
	if len(out.Adjacent) != 1 || out.Adjacent[0].ID != "r1111111-1111-4111-8111-111111111111" {
		t.Errorf("adjacent = %+v, want the r1 session", out.Adjacent)
	}
	if out.Notes == nil {
		t.Error("notes must never marshal as null")
	}
}

func TestRelatedOutcomeNotFound(t *testing.T) {
	a := homeApp(t, relatedHome(t))
	out, err := a.Related("zzzz")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "notfound" {
		t.Errorf("status = %s, want notfound", out.Status)
	}
}
