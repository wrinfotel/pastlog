package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// StatsOptions mirrors the CLI's stats flags 1:1 (spec §4.2).
type StatsOptions struct {
	Filter FilterOptions `json:"filter"`
	By     string        `json:"by"`    // agent | project | day | model (default agent)
	Model  string        `json:"model"` // case-insensitive substring, like --model
}

// StatsOutcome is the `pastlog stats` surface for the GUI.
type StatsOutcome struct {
	By    string                `json:"by"`
	Rows  []render.StatsRowJSON `json:"rows"`
	Notes []string              `json:"notes"`
}

// Stats aggregates token usage with the CLI's exact semantics (long-op #2
// of the GUI: progress events + cancellation over large histories). An empty
// selection is an empty result, never an error — stats has no grep semantics.
func (a *App) Stats(o StatsOptions) (StatsOutcome, error) {
	by, err := normalizeBy(o.By)
	if err != nil {
		return StatsOutcome{}, err
	}
	home, err := a.effectiveHome()
	if err != nil {
		return StatsOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, o.Filter.Agent); err != nil {
		return StatsOutcome{}, err
	}
	filter, err := a.buildFilter(o.Filter)
	if err != nil {
		return StatsOutcome{}, err
	}

	gen, _, statsHook := a.begin("stats")
	adapters := reg.Adapters()
	var notes []string
	rows := agentlog.CollectUsage(adapters, filter, o.Model, func(note string) {
		notes = append(notes, note)
	}, agentlog.Progress(statsHook))
	if !a.stillActive(gen) {
		notes = append(notes, "cancelled — partial aggregate")
	}
	groups := render.GroupStats(home, by, rows)
	return StatsOutcome{
		By:    by,
		Rows:  render.NewStatsRows(groups),
		Notes: a.combineNotes(adapters, notes),
	}, nil
}

// ExportStats writes the grouped stats as CLI-identical JSON
// (`pastlog stats --json` bytes).
func (a *App) ExportStats(o StatsOptions, destPath string) error {
	if destPath == "" {
		return fmt.Errorf("empty export path")
	}
	if !filepath.IsAbs(destPath) {
		return fmt.Errorf("export path must be absolute: %s", destPath)
	}
	by, err := normalizeBy(o.By)
	if err != nil {
		return err
	}
	home, err := a.effectiveHome()
	if err != nil {
		return err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, o.Filter.Agent); err != nil {
		return err
	}
	filter, err := a.buildFilter(o.Filter)
	if err != nil {
		return err
	}
	adapters := reg.Adapters()
	rows := agentlog.CollectUsage(adapters, filter, o.Model, nil)
	var buf bytes.Buffer
	if err := render.StatsJSON(&buf, render.GroupStats(home, by, rows)); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("cannot write the export: %v", err)
	}
	return nil
}

// normalizeBy validates the grouping mode and applies the default.
func normalizeBy(by string) (string, error) {
	switch by {
	case "":
		return render.StatsByAgent, nil
	case render.StatsByAgent, render.StatsByProject, render.StatsByDay, render.StatsByModel:
		return by, nil
	default:
		return "", fmt.Errorf("invalid --by value %q: use agent, project, day or model", by)
	}
}
