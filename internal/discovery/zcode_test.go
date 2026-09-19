package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

// ZCode stores everything under a fixed ~/.zcode tree on every OS (it is a
// Node app with its own home), so the only candidate is <home>/.zcode/cli/db
// — the directory holding db.sqlite.
func TestZCodeDirHome(t *testing.T) {
	home := t.TempDir()
	want := mkdirAll(t, filepath.Join(home, ".zcode", "cli", "db"))
	if got := ZCodeDir(home); got != want {
		t.Errorf("ZCodeDir(home) = %q, want %q", got, want)
	}
}

func TestZCodeDirAbsent(t *testing.T) {
	home := t.TempDir()
	if got := ZCodeDir(home); got != "" {
		t.Errorf("ZCodeDir(home) = %q, want \"\" when no .zcode/cli/db exists", got)
	}
}

func TestZCodeDirFileIsNotADir(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".zcode", "cli", "db")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ZCodeDir(home); got != "" {
		t.Errorf("ZCodeDir(home) = %q, want \"\" when the candidate is a file", got)
	}
}

func TestZCodeDirEmptyHome(t *testing.T) {
	if got := ZCodeDir(""); got != "" {
		t.Errorf("ZCodeDir(\"\") = %q, want \"\"", got)
	}
}
