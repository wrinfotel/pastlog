// Package app holds pastlog Desktop's bound services: thin glue over the
// internal/* data layer. No business logic here — filter structs in, render
// JSON structs out (TASK-DESKTOP.md §4.1). This package must never import
// wails or any network package (enforced by TestNoNetworkDeps, ruling R-D3).
package app

import (
	"fmt"

	"github.com/wrinfotel/pastlog/internal/adapters/claudecode"
	"github.com/wrinfotel/pastlog/internal/adapters/codex"
	"github.com/wrinfotel/pastlog/internal/adapters/geminicli"
	"github.com/wrinfotel/pastlog/internal/adapters/opencode"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/discovery"
)

// App is the service surface bound to the frontend.
type App struct{}

func New() *App { return &App{} }

// Ping exists only for the D0 spike; removed by T09.
func (a *App) Ping() string { return "pong" }

// SpikeOverview returns per-agent summaries for the D0 spike window; the
// T09 services replace it with tested, structured output.
func (a *App) SpikeOverview() []string {
	home, err := discovery.Home("")
	if err != nil {
		return []string{"home error: " + err.Error()}
	}
	reg := agentlog.NewRegistry()
	reg.Register(claudecode.New(home))
	reg.Register(codex.New(home))
	reg.Register(geminicli.New(home))
	reg.Register(opencode.New(home))
	var out []string
	for _, ad := range reg.Adapters() {
		n := 0
		if ad.Detect() {
			_ = ad.Sessions(func(agentlog.Session) error { n++; return nil })
			out = append(out, fmt.Sprintf("%s: %d sessions", ad.Name(), n))
		} else {
			out = append(out, ad.Name()+": not found")
		}
	}
	return out
}
