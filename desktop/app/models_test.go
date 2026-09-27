package app

import (
	"os"
	"path/filepath"
	"testing"
)

// usageClaudeHome builds a claude-code home whose session consumes tokens
// under two models — the per-model split the MODELS panel shows.
const usageSessionID = "uuuu1111-1111-4111-8111-111111111111"

func usageClaudeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	content := `{"type":"user","sessionId":"uuuu1111-1111-4111-8111-111111111111","cwd":"/home/dev/app","timestamp":"2026-08-03T09:00:00Z","message":{"role":"user","content":"split usage across models"}}
{"type":"assistant","sessionId":"uuuu1111-1111-4111-8111-111111111111","cwd":"/home/dev/app","timestamp":"2026-08-03T09:00:10Z","message":{"role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"working on it"}],"usage":{"input_tokens":1000,"output_tokens":200,"cache_read_input_tokens":5000}}}
{"type":"assistant","sessionId":"uuuu1111-1111-4111-8111-111111111111","cwd":"/home/dev/app","timestamp":"2026-08-03T09:00:20Z","message":{"role":"assistant","model":"claude-haiku-4-5","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":300,"output_tokens":100}}}
`
	dst := filepath.Join(home, ".claude", "projects", "-home-dev-app", usageSessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestSessionModelsSplit(t *testing.T) {
	a := homeApp(t, usageClaudeHome(t))
	out, err := a.SessionModels(usageSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %s", out.Status)
	}
	if out.Session == nil || out.Session.ID != usageSessionID {
		t.Fatalf("session = %+v", out.Session)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(out.Rows))
	}
	first := out.Rows[0]
	if first.Model != "claude-sonnet-4-5" {
		t.Errorf("first model = %q, want the first model in use order", first.Model)
	}
	if first.Tokens.Input != 1000 || first.Tokens.Output != 200 || first.Tokens.CacheRead != 5000 {
		t.Errorf("first tokens = %+v", first.Tokens)
	}
	if first.Tokens.Total != 1200 {
		t.Errorf("total = %d, want input+output+reasoning", first.Tokens.Total)
	}
	if second := out.Rows[1]; second.Model != "claude-haiku-4-5" || second.Tokens.Input != 300 || second.Tokens.Output != 100 {
		t.Errorf("second row = %+v", second)
	}
}

func TestSessionModelsNoUsageData(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	out, err := a.SessionModels("3f9c81a2-1111-4222-8333-cccccccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %s", out.Status)
	}
	if len(out.Rows) != 0 {
		t.Errorf("rows = %v, want none for a session without usage records", out.Rows)
	}
}

func TestSessionModelsNotFound(t *testing.T) {
	a := homeApp(t, usageClaudeHome(t))
	out, err := a.SessionModels("zzzz")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "notfound" {
		t.Errorf("status = %s, want notfound", out.Status)
	}
}

func TestSessionModelsAmbiguous(t *testing.T) {
	// two sessions whose ids share the "3f9c81a2" prefix
	home := t.TempDir()
	for _, last := range []string{"c", "d"} {
		id := "3f9c81a2-1111-4222-8333-cccccccccc" + last
		content := `{"type":"user","sessionId":"` + id + `","cwd":"/home/dev/app","timestamp":"2026-08-03T09:00:00Z","message":{"role":"user","content":"ambiguous prefix"}}
`
		dst := filepath.Join(home, ".claude", "projects", "-home-dev-app", id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := homeApp(t, home)
	out, err := a.SessionModels("3f9c81a2")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ambiguous" {
		t.Errorf("status = %s, want ambiguous", out.Status)
	}
	if len(out.Candidates) != 2 {
		t.Errorf("candidates = %d, want 2", len(out.Candidates))
	}
}
