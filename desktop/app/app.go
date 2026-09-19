// Package app holds pastlog Desktop's bound services: thin glue over the
// internal/* data layer. No business logic here — filter structs in, render
// JSON structs out (TASK-DESKTOP.md §4.1). This package must never import
// wails or any network package (enforced by TestNoNetworkDeps, ruling R-D3).
package app

import (
	"sync"
	"sync/atomic"

	"github.com/wrinfotel/pastlog/internal/version"
)

// EventSink receives streaming progress of long operations (search, stats).
// main.go adapts it to Wails events; tests capture the calls directly.
type EventSink interface {
	Progress(op string, scanned, hits int)
}

// App is the service surface bound to the frontend. One App serves the whole
// GUI; the effective home can change at runtime through the settings.
type App struct {
	cfgDir string
	cfg    Config
	sink   EventSink

	mu  sync.Mutex // guards cfg swaps from the Set* methods
	gen atomic.Int64
}

// Option configures an App at construction time.
type Option func(*App)

// WithConfigDir overrides the settings directory (tests inject a temp dir).
func WithConfigDir(dir string) Option {
	return func(a *App) { a.cfgDir = dir }
}

// WithSink attaches the progress sink (main.go wires the Wails adapter).
func WithSink(s EventSink) Option {
	return func(a *App) { a.sink = s }
}

// New builds the App. Construction never fails and never writes: settings
// load defensively and land on disk only when the user changes one.
func New(options ...Option) *App {
	a := &App{}
	for _, o := range options {
		o(a)
	}
	a.cfg = loadConfig(a.cfgDir)
	return a
}

// Settings is the persisted settings surface plus the about box (spec §2.4:
// the only user-facing knobs are the home override and the theme).
type Settings struct {
	Home    string `json:"home"`  // persisted override; "" = auto-discover
	Theme   string `json:"theme"` // "" | "system" | "dark" | "light"
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// GetSettings returns the current settings and build metadata.
func (a *App) GetSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Settings{
		Home:    a.cfg.Home,
		Theme:   a.cfg.Theme,
		Version: version.Version,
		Commit:  version.Commit,
		Date:    version.Date,
	}
}

// SetHome validates and persists the home-directory override.
func (a *App) SetHome(home string) (Settings, error) {
	if _, err := a.effectiveHomeFor(home); err != nil {
		return a.GetSettings(), err
	}
	a.mu.Lock()
	a.cfg.Home = home
	err := a.cfg.save(a.cfgDir)
	a.mu.Unlock()
	if err != nil {
		return a.GetSettings(), err
	}
	return a.GetSettings(), nil
}

// SetTheme validates and persists the theme ("system", "dark" or "light").
func (a *App) SetTheme(theme string) (Settings, error) {
	switch theme {
	case "", "system", "dark", "light":
	default:
		return a.GetSettings(), errInvalidTheme(theme)
	}
	a.mu.Lock()
	a.cfg.Theme = theme
	err := a.cfg.save(a.cfgDir)
	a.mu.Unlock()
	if err != nil {
		return a.GetSettings(), err
	}
	return a.GetSettings(), nil
}

// Cancel stops the active long operation (search or stats) at its next
// progress tick: the hooks compare their generation against the counter.
func (a *App) Cancel() { a.gen.Add(1) }

// stillActive reports whether the generation is the newest one, i.e. not
// cancelled and not superseded by a later operation.
func (a *App) stillActive(gen int64) bool { return a.gen.Load() == gen }

// begin starts a long operation: it returns the generation plus the search-
// and stats-shaped progress hooks bound to it. Hooks are nil-safe on the
// sink and emit ("search", scanned, hits) / ("stats", scanned, hits).
func (a *App) begin(op string) (int64, func(scanned, hits int) bool, func(done int) bool) {
	gen := a.gen.Add(1)
	searchHook := func(scanned, hits int) bool {
		if !a.stillActive(gen) {
			return false
		}
		if a.sink != nil {
			a.sink.Progress(op, scanned, hits)
		}
		return true
	}
	statsHook := func(done int) bool { return searchHook(done, 0) }
	return gen, searchHook, statsHook
}
