package discovery

import (
	"runtime"
	"testing"
)

func homeEnvVar() string {
	if runtime.GOOS == "windows" {
		return "USERPROFILE"
	}
	return "HOME"
}

func TestHomeFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(homeEnvVar(), dir)
	got, err := Home("")
	if err != nil {
		t.Fatalf("Home(\"\") error: %v", err)
	}
	if got != dir {
		t.Errorf("Home(\"\") = %q, want %q", got, dir)
	}
}

func TestHomeOverrideWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(homeEnvVar(), "/should-not-be-used")
	got, err := Home(dir)
	if err != nil {
		t.Fatalf("Home(override) error: %v", err)
	}
	if got != dir {
		t.Errorf("Home(override) = %q, want %q", got, dir)
	}
}

func TestHomeOverrideMustExist(t *testing.T) {
	t.Setenv(homeEnvVar(), t.TempDir())
	if _, err := Home("/no/such/dir-pastlog-test"); err == nil {
		t.Fatal("expected error for nonexistent --home override")
	}
}

func TestHomeErrorWhenUndeterminable(t *testing.T) {
	t.Setenv(homeEnvVar(), "")
	if _, err := Home(""); err == nil {
		t.Fatal("expected error when home env is empty and override missing")
	}
}
