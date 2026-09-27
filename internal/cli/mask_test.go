package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// eeee5555 carries a realistic GitHub token so masking tests run against the
// same fixtures as search/show. It never matches "jwt refresh", keeping the
// other goldens stable. __TOKEN__ is substituted with rawSecret at write
// time, so the secret-shaped literal never appears contiguously in source
// and GitHub push protection does not mistake the fixture for a real key.
const claudeEContent = `{"type":"user","sessionId":"eeee5555-5555-4555-8555-555555555555","cwd":"/home/dev/keys","timestamp":"2026-08-05T09:00:00Z","message":{"role":"user","content":"my token is __TOKEN__ please rotate"}}
`

const (
	rawSecret    = "ghp_" + "0123456789abcdefABCD"
	maskedSecret = "ghp_…ABCD"
)

func maskHome(t *testing.T) string {
	t.Helper()
	home := searchShowHome(t)
	p := filepath.Join(home, ".claude", "projects", "-home-dev-keys", "eeee5555-5555-4555-8555-555555555555.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(claudeEContent, "__TOKEN__", rawSecret)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestShowMasksSecretsByDefault(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "show", "eeee5555")
	if code != 0 {
		t.Fatalf("show exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, maskedSecret) {
		t.Errorf("masked output should contain %q, got:\n%s", maskedSecret, out)
	}
	if strings.Contains(out, rawSecret) {
		t.Errorf("masked output must not contain the raw token:\n%s", out)
	}
}

func TestShowNoMaskFlag(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "show", "eeee5555", "--no-mask")
	if code != 0 {
		t.Fatalf("show --no-mask exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, rawSecret) {
		t.Errorf("--no-mask output should contain the raw token, got:\n%s", out)
	}
}

func TestShowJSONStaysVerbatim(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "show", "eeee5555", "--json")
	if code != 0 {
		t.Fatalf("show --json exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, rawSecret) {
		t.Errorf("--json is the machine channel and stays verbatim, got:\n%s", out)
	}
}

func TestShowExportMarkdownMasks(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "show", "eeee5555", "--export", "md")
	if code != 0 {
		t.Fatalf("show --export md exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, maskedSecret) || strings.Contains(out, rawSecret) {
		t.Errorf("markdown export should mask by default, got:\n%s", out)
	}
}

func TestSearchMasksHitLines(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "search", "please rotate")
	if code != 0 {
		t.Fatalf("search exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "my token is "+maskedSecret+" please rotate") {
		t.Errorf("search hit line should be masked, got:\n%s", out)
	}
	if strings.Contains(out, rawSecret) {
		t.Errorf("search output must not contain the raw token:\n%s", out)
	}
}

func TestSearchMatchesRawDataButPrintsMasked(t *testing.T) {
	home := maskHome(t)
	// The needle only exists inside the secret: matching runs on raw data,
	// printing on masked data.
	code, out, errOut := run(t, "--home", home, "search", "0123456789abcdef")
	if code != 0 {
		t.Fatalf("search exit = %d, stderr: %s", code, errOut)
	}
	if strings.Contains(out, rawSecret) {
		t.Errorf("search output must not leak the raw token:\n%s", out)
	}
	if !strings.Contains(out, maskedSecret) {
		t.Errorf("the masked line should still print, got:\n%s", out)
	}
}

func TestSearchNoMaskFlag(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "search", "please rotate", "--no-mask")
	if code != 0 {
		t.Fatalf("search --no-mask exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, rawSecret) {
		t.Errorf("--no-mask search should contain the raw token, got:\n%s", out)
	}
}

func TestSearchJSONStaysVerbatim(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "search", "please rotate", "--json")
	if code != 0 {
		t.Fatalf("search --json exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, rawSecret) {
		t.Errorf("--json is the machine channel and stays verbatim, got:\n%s", out)
	}
}
