package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/version"
)

// newTestApp builds an App over an isolated config dir.
func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cfg")
	a := New(WithConfigDir(dir))
	return a, dir
}

func TestGetSettingsFreshConfigDir(t *testing.T) {
	a, dir := newTestApp(t)
	s := a.GetSettings()
	if s.Theme != "" {
		t.Errorf("fresh theme = %q, want \"\" (system)", s.Theme)
	}
	if s.Home != "" {
		t.Errorf("fresh home = %q, want \"\"", s.Home)
	}
	if s.Version == "" || s.Version != version.Version {
		t.Errorf("version = %q, want internal/version.Version %q", s.Version, version.Version)
	}
	// zero-config posture: nothing is written until the user changes a setting
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Error("config.json must not exist before the first Set call")
	}
}

func TestSetHomePersistsAndReloads(t *testing.T) {
	a, dir := newTestApp(t)
	home := t.TempDir()
	if _, err := a.SetHome(home); err != nil {
		t.Fatalf("SetHome: %v", err)
	}
	if got := a.GetSettings().Home; got != home {
		t.Errorf("home after SetHome = %q, want %q", got, home)
	}
	// a new App over the same dir must see the persisted override
	b := New(WithConfigDir(dir))
	if got := b.GetSettings().Home; got != home {
		t.Errorf("reloaded home = %q, want %q", got, home)
	}
}

func TestSetHomeValidation(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.SetHome(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing home accepted")
	} else if want := "home directory not found"; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want prefix %q", err.Error(), want)
	}
	file := filepath.Join(t.TempDir(), "f")
	os.WriteFile(file, []byte("x"), 0o644)
	if _, err := a.SetHome(file); err == nil {
		t.Error("file home accepted")
	}
}

func TestSetTheme(t *testing.T) {
	a, dir := newTestApp(t)
	for _, theme := range []string{"dark", "light", "system"} {
		if _, err := a.SetTheme(theme); err != nil {
			t.Fatalf("SetTheme(%q): %v", theme, err)
		}
		if got := a.GetSettings().Theme; got != theme {
			t.Errorf("theme = %q, want %q", got, theme)
		}
	}
	b := New(WithConfigDir(dir))
	if got := b.GetSettings().Theme; got != "system" {
		t.Errorf("reloaded theme = %q, want system", got)
	}
	if _, err := a.SetTheme("purple"); err == nil {
		t.Error("invalid theme accepted")
	} else if want := `invalid theme "purple": use system, dark or light`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestCorruptConfigFallsBackToDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o644)
	a := New(WithConfigDir(dir))
	s := a.GetSettings()
	if s.Home != "" || s.Theme != "" {
		t.Errorf("corrupt config must fall back to defaults, got %+v", s)
	}
}

func TestEffectiveHomePrefersOverride(t *testing.T) {
	a, _ := newTestApp(t)
	override := t.TempDir()
	a.cfg.Home = override
	got, err := a.effectiveHome()
	if err != nil || got != override {
		t.Errorf("effectiveHome = %q, %v; want the override", got, err)
	}
}

func TestEffectiveHomeFallsBackToDiscovery(t *testing.T) {
	a, _ := newTestApp(t)
	got, err := a.effectiveHome()
	if err != nil || got == "" {
		t.Errorf("effectiveHome = %q, %v; want the discovered home", got, err)
	}
}

func TestCancelBumpsGeneration(t *testing.T) {
	a, _ := newTestApp(t)
	gen, _, _ := a.begin("test")
	if !a.stillActive(gen) {
		t.Fatal("fresh generation must be active")
	}
	a.Cancel()
	if a.stillActive(gen) {
		t.Error("generation must be inactive after Cancel")
	}
}

func TestBeginHooksEmitAndCancel(t *testing.T) {
	var gotOp string
	var gotScanned, gotHits int
	sink := &fakeSink{onProgress: func(op string, scanned, hits int) {
		gotOp, gotScanned, gotHits = op, scanned, hits
	}}
	a := New(WithConfigDir(filepath.Join(t.TempDir(), "cfg")), WithSink(sink))
	gen, searchHook, statsHook := a.begin("search")

	if !searchHook(3, 7) {
		t.Fatal("active generation must not cancel")
	}
	if gotOp != "search" || gotScanned != 3 || gotHits != 7 {
		t.Errorf("sink got op=%q scanned=%d hits=%d", gotOp, gotScanned, gotHits)
	}
	if !statsHook(1) {
		t.Fatal("the stats hook of the same generation must be active too")
	}

	a.Cancel()
	if searchHook(4, 8) {
		t.Error("hook must report cancellation after Cancel")
	}
	_ = gen
}

type fakeSink struct {
	onProgress func(op string, scanned, hits int)
}

func (f *fakeSink) Progress(op string, scanned, hits int) { f.onProgress(op, scanned, hits) }
