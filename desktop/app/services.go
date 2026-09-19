package app

import (
	"fmt"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/dates"
	"github.com/wrinfotel/pastlog/internal/render"
)

// FilterOptions mirrors the CLI's shared session filters 1:1 (TASK-DESKTOP.md
// §4.2): no GUI-only filtering, no CLI filter missing. Since/Until accept the
// CLI's Nd, Nw and YYYY-MM-DD forms.
type FilterOptions struct {
	Agent   string `json:"agent"`
	Project string `json:"project"`
	Since   string `json:"since"`
	Until   string `json:"until"`
	Limit   int    `json:"limit"`
}

// OverviewOutcome is the bare-`pastlog` surface: per-agent detection summary
// for the Home view. Error carries a home-resolution failure (the friendly
// empty state).
type OverviewOutcome struct {
	Home     string             `json:"home"`
	Agents   []render.AgentJSON `json:"agents"`
	Warnings []string           `json:"warnings"`
	Error    string             `json:"error,omitempty"`
}

// DiagnosticsOutcome is the `pastlog agents` surface plus the stderr
// conditions the CLI prints (locked storage warnings, skipped records).
type DiagnosticsOutcome struct {
	Home     string             `json:"home"`
	Agents   []render.AgentJSON `json:"agents"`
	Warnings []string           `json:"warnings"`
	Skipped  int                `json:"skipped"`
	Error    string             `json:"error,omitempty"`
}

// ListOutcome is the `pastlog sessions` surface.
type ListOutcome struct {
	Sessions []render.SessionJSON `json:"sessions"`
	Notes    []string             `json:"notes"`
}

// Overview returns the per-agent detection summary for the Home view.
func (a *App) Overview() OverviewOutcome {
	home, err := a.effectiveHome()
	if err != nil {
		return OverviewOutcome{Error: err.Error()}
	}
	rows, warnings := agentRows(cli.NewRegistry(home).Adapters())
	return OverviewOutcome{Home: home, Agents: render.AgentRowsJSON(rows), Warnings: warnings}
}

// Diagnostics returns the `agents` data with the CLI's stderr conditions
// structured into the outcome.
func (a *App) Diagnostics() DiagnosticsOutcome {
	home, err := a.effectiveHome()
	if err != nil {
		return DiagnosticsOutcome{Error: err.Error()}
	}
	adapters := cli.NewRegistry(home).Adapters()
	rows, warnings := agentRows(adapters)
	return DiagnosticsOutcome{
		Home:     home,
		Agents:   render.AgentRowsJSON(rows),
		Warnings: a.combineNotes(adapters, warnings),
		Skipped:  agentlog.TotalSkipped(adapters),
	}
}

// Sessions lists sessions, newest first, under the given filters — the
// `pastlog sessions` contract.
func (a *App) Sessions(f FilterOptions) (ListOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return ListOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, f.Agent); err != nil {
		return ListOutcome{}, err
	}
	filter, err := a.buildFilter(f)
	if err != nil {
		return ListOutcome{}, err
	}
	adapters := reg.Adapters()
	var notes []string
	rows := agentlog.CollectSessions(adapters, filter, func(note string) {
		notes = append(notes, note)
	})
	return ListOutcome{Sessions: render.SessionRowsJSON(rows), Notes: a.combineNotes(adapters, notes)}, nil
}

// agentRows mirrors the `pastlog agents` row computation (cli/agents.go):
// one row per adapter, detected storages counted in a streaming pass,
// shared-file adapters reporting their on-disk footprint via TotalSizer.
// Glue only — the counting semantics live in the adapters.
func agentRows(adapters []agentlog.Adapter) ([]render.AgentRow, []string) {
	rows := make([]render.AgentRow, len(adapters))
	var warnings []string
	for i, a := range adapters {
		row := render.AgentRow{Name: a.Name()}
		if ps, ok := a.(agentlog.PathSource); ok {
			row.Path = ps.StoragePath()
		}
		if a.Detect() {
			row.Detected = true
			if err := a.Sessions(func(s agentlog.Session) error {
				row.Sessions++
				row.Bytes += s.SizeBytes
				return nil
			}); err != nil {
				// best effort (spec §8): keep the row, explain the empty count
				warnings = append(warnings, agentlog.UnreadableNote(a.Name(), err))
			}
			if ts, ok := a.(agentlog.TotalSizer); ok {
				row.Bytes = ts.TotalBytes()
			}
		}
		rows[i] = row
	}
	return rows, warnings
}

// combineNotes mirrors the CLI's stderr summary order (cli noteStderr):
// locked-storage warnings, then the unreadable-lines total, then scan notes.
func (a *App) combineNotes(adapters []agentlog.Adapter, noteLines []string) []string {
	var out []string
	for _, a := range adapters {
		if ws, ok := a.(agentlog.WarningSource); ok {
			if msg := ws.Warning(); msg != "" {
				out = append(out, msg)
			}
		}
	}
	if n := agentlog.TotalSkipped(adapters); n > 0 {
		out = append(out, fmt.Sprintf("%d unreadable lines skipped", n))
	}
	return append(out, noteLines...)
}

// buildFilter converts the GUI filter into the core's SessionFilter with the
// CLI's exact cutoff semantics (dates.ParseCutoff).
func (a *App) buildFilter(f FilterOptions) (agentlog.SessionFilter, error) {
	if f.Limit < 0 {
		return agentlog.SessionFilter{}, fmt.Errorf("invalid limit %d: use a non-negative number", f.Limit)
	}
	filter := agentlog.SessionFilter{Agent: f.Agent, Project: f.Project, Limit: f.Limit}
	var err error
	if f.Since != "" {
		if filter.Since, err = dates.ParseCutoff("since", f.Since, false); err != nil {
			return agentlog.SessionFilter{}, err
		}
	}
	if f.Until != "" {
		if filter.Until, err = dates.ParseCutoff("until", f.Until, true); err != nil {
			return agentlog.SessionFilter{}, err
		}
	}
	return filter, nil
}

// validateAgent applies the CLI's unknown-agent rejection.
func validateAgent(reg *agentlog.Registry, name string) error {
	if name == "" {
		return nil
	}
	if _, ok := reg.Get(name); !ok {
		return fmt.Errorf("unknown agent %q (available: %s)", name, availableAgents(reg))
	}
	return nil
}

func availableAgents(reg *agentlog.Registry) string {
	names := ""
	for i, a := range reg.Adapters() {
		if i > 0 {
			names += ", "
		}
		names += a.Name()
	}
	return names
}
