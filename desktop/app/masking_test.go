package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// secretClaudeHome builds a claude-code home whose single session carries a
// realistic GitHub token in a user message. __TOKEN__ is substituted at
// write time, so the secret-shaped literal never appears contiguously in
// source and GitHub push protection does not mistake the fixture for a key.
func secretClaudeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	content := `{"type":"user","sessionId":"eeee5555-5555-4555-8555-555555555555","cwd":"/home/dev/keys","timestamp":"2026-08-05T09:00:00Z","message":{"role":"user","content":"my token is __TOKEN__ please rotate"}}
`
	dst := filepath.Join(home, ".claude", "projects", "-home-dev-keys", "eeee5555-5555-4555-8555-555555555555.jsonl")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	content = strings.ReplaceAll(content, "__TOKEN__", rawSecret)
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

const (
	rawSecret    = "ghp_" + "0123456789abcdefABCD"
	maskedSecret = "ghp_…ABCD"
)

func TestSettingsMaskSecretsDefaultsOn(t *testing.T) {
	a := New(WithConfigDir(filepath.Join(t.TempDir(), "cfg")))
	if !a.GetSettings().MaskSecrets {
		t.Error("fresh config must default masking on")
	}
}

func TestOldConfigWithoutMaskFieldDefaultsOn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(`{"home":"/tmp"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(WithConfigDir(dir))
	if !a.GetSettings().MaskSecrets {
		t.Error("a config saved before 0.2.3 must default masking on")
	}
}

func TestSetMaskSecretsPersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	a := New(WithConfigDir(dir))
	if _, err := a.SetMaskSecrets(false); err != nil {
		t.Fatal(err)
	}
	if a.GetSettings().MaskSecrets {
		t.Error("masking should be off after SetMaskSecrets(false)")
	}
	b := New(WithConfigDir(dir))
	if b.GetSettings().MaskSecrets {
		t.Error("masking=false must survive a reload")
	}
	if _, err := b.SetMaskSecrets(true); err != nil {
		t.Fatal(err)
	}
	if !b.GetSettings().MaskSecrets {
		t.Error("masking should be back on")
	}
}

func TestEntriesMasksByDefault(t *testing.T) {
	a := homeApp(t, secretClaudeHome(t))
	out, err := a.Entries("eeee5555")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %s", out.Status)
	}
	var text strings.Builder
	for _, e := range out.Entries {
		text.WriteString(e.Text)
	}
	if !strings.Contains(text.String(), maskedSecret) {
		t.Errorf("entries should carry the masked token, got %q", text.String())
	}
	if strings.Contains(text.String(), rawSecret) {
		t.Errorf("entries must not leak the raw token")
	}
}

func TestEntriesVerbatimWhenMaskingOff(t *testing.T) {
	a := homeApp(t, secretClaudeHome(t))
	if _, err := a.SetMaskSecrets(false); err != nil {
		t.Fatal(err)
	}
	out, err := a.Entries("eeee5555")
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, e := range out.Entries {
		text.WriteString(e.Text)
	}
	if !strings.Contains(text.String(), rawSecret) {
		t.Errorf("masking off must show the raw token, got %q", text.String())
	}
}

func TestSearchMasksHitsByDefault(t *testing.T) {
	a := homeApp(t, secretClaudeHome(t))
	out, err := a.Search("please rotate", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Hits != 1 {
		t.Fatalf("hits = %d, want 1", out.Hits)
	}
	hit := out.Results[0].Hits[0]
	if !strings.Contains(hit.Line, maskedSecret) || strings.Contains(hit.Line, rawSecret) {
		t.Errorf("hit line should be masked, got %q", hit.Line)
	}
	if !strings.Contains(hit.EntryHead, maskedSecret) || strings.Contains(hit.EntryHead, rawSecret) {
		t.Errorf("entry head should be masked, got %q", hit.EntryHead)
	}
}

func TestExportMarkdownMasksByDefault(t *testing.T) {
	a := homeApp(t, secretClaudeHome(t))
	dest := filepath.Join(t.TempDir(), "out.md")
	if _, err := a.ExportSession("eeee5555", "md", dest); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), maskedSecret) || strings.Contains(string(raw), rawSecret) {
		t.Errorf("markdown export should mask by default (CLI parity), got:\n%s", raw)
	}
}

func TestExportJSONStaysVerbatim(t *testing.T) {
	a := homeApp(t, secretClaudeHome(t))
	dest := filepath.Join(t.TempDir(), "out.json")
	if _, err := a.ExportSession("eeee5555", "json", dest); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), rawSecret) {
		t.Errorf("json export is the machine channel and stays CLI-identical, got:\n%s", raw)
	}
}
