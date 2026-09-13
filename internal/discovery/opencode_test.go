package discovery

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// xdgEnvVar is the env var that overrides the XDG data dir on each OS
// (controller ruling: LOCALAPPDATA on Windows, XDG_DATA_HOME on unix).
func xdgEnvVar() string {
	if runtime.GOOS == "windows" {
		return "LOCALAPPDATA"
	}
	return "XDG_DATA_HOME"
}

func mkdirAll(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenCodeDirFromDataEnv(t *testing.T) {
	data := t.TempDir()
	t.Setenv(xdgEnvVar(), data)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	want := mkdirAll(t, filepath.Join(data, "opencode"))
	if got := OpenCodeDir(""); got != want {
		t.Errorf("OpenCodeDir(\"\") = %q, want %q", got, want)
	}
}

func TestOpenCodeDirHomeFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv(xdgEnvVar(), "") // no override: the home path wins
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	want := mkdirAll(t, filepath.Join(home, ".local", "share", "opencode"))
	if got := OpenCodeDir(home); got != want {
		t.Errorf("OpenCodeDir(home) = %q, want %q", got, want)
	}
}

func TestOpenCodeDirDataEnvWinsOverHome(t *testing.T) {
	home := t.TempDir()
	data := t.TempDir()
	t.Setenv(xdgEnvVar(), data)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mkdirAll(t, filepath.Join(home, ".local", "share", "opencode"))

	want := mkdirAll(t, filepath.Join(data, "opencode"))
	if got := OpenCodeDir(""); got != want {
		t.Errorf("OpenCodeDir(\"\") = %q, want %q (first existing wins)", got, want)
	}
}

func TestOpenCodeDirEmptyEnvIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv(xdgEnvVar(), "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mkdirAll(t, filepath.Join(home, ".local", "share", "opencode"))

	if got := OpenCodeDir(home); got == "" {
		t.Fatal("empty env var must not stop the home fallback")
	}
}

func TestOpenCodeDirAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv(xdgEnvVar(), t.TempDir()) // exists but holds no opencode dir
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if got := OpenCodeDir(""); got != "" {
		t.Errorf("OpenCodeDir(\"\") = %q, want \"\" when no candidate exists", got)
	}
}

func TestOpenCodeDirFileIsNotADir(t *testing.T) {
	home := t.TempDir()
	data := t.TempDir()
	t.Setenv(xdgEnvVar(), data)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	notDir := filepath.Join(data, "opencode")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := OpenCodeDir(""); got != "" {
		t.Errorf("OpenCodeDir(\"\") = %q, want \"\" when the candidate is a file", got)
	}
}

// TestOpenCodeDirEnvVarIsPlatformSpecific pins the controller ruling that only
// the per-OS env var is honored: the other platform's variable must be ignored.
func TestOpenCodeDirEnvVarIsPlatformSpecific(t *testing.T) {
	home := t.TempDir()
	other := t.TempDir()
	data := t.TempDir()
	mkdirAll(t, filepath.Join(other, "opencode"))

	t.Setenv(xdgEnvVar(), "")
	if runtime.GOOS == "windows" {
		t.Setenv("XDG_DATA_HOME", other) // must be ignored on Windows
	} else {
		t.Setenv("LOCALAPPDATA", other) // must be ignored on unix
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mkdirAll(t, filepath.Join(home, ".local", "share", "opencode"))

	want := filepath.Join(home, ".local", "share", "opencode")
	if got := OpenCodeDir(home); got != want {
		t.Errorf("OpenCodeDir(\"\") = %q, want %q (foreign env var ignored; %q would be %q)",
			got, want, data, filepath.Join(data, "opencode"))
	}
}

func TestOpenCodeDirUsesHomeArgument(t *testing.T) {
	t.Setenv(xdgEnvVar(), "")
	home := t.TempDir()
	want := mkdirAll(t, filepath.Join(home, ".local", "share", "opencode"))
	if got := OpenCodeDir(home); got != want {
		t.Errorf("OpenCodeDir(home) = %q, want %q", got, want)
	}
}
