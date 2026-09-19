package app

import (
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// ProjectRow is one project of an agent: the raw storage path (the GUI's
// drill-down key must be the exact path, while the CLI's --project is only
// a substring filter) plus the CLI's `stats --by project` aggregate for it.
type ProjectRow struct {
	Project  string             `json:"project"`
	Sessions int                `json:"sessions"`
	Messages int                `json:"messages"`
	Tokens   render.StatsTokens `json:"tokens"`
	CostUSD  *float64           `json:"cost_usd"`
}

// ProjectsOutcome is the per-agent project list behind the GUI's Projects
// view (`stats --by project --agent X` in-process).
type ProjectsOutcome struct {
	Agent string       `json:"agent"`
	Rows  []ProjectRow `json:"rows"`
	Notes []string     `json:"notes"`
}

// ProjectStatsOutcome is one exact project's per-model usage breakdown —
// `stats --by model --agent X` narrowed to the project's own sessions.
type ProjectStatsOutcome struct {
	Agent   string                `json:"agent"`
	Project string                `json:"project"`
	Rows    []render.StatsRowJSON `json:"rows"`
	Notes   []string              `json:"notes"`
}

// Projects lists one agent's projects with their usage aggregates, keyed by
// the raw project path. Long op: progress events and cancellation like Stats.
func (a *App) Projects(agent string) (ProjectsOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return ProjectsOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, agent); err != nil {
		return ProjectsOutcome{}, err
	}

	gen, _, statsHook := a.begin("projects")
	adapters := reg.Adapters()
	var notes []string
	rows := agentlog.CollectUsage(adapters, agentlog.SessionFilter{Agent: agent}, "", func(note string) {
		notes = append(notes, note)
	}, agentlog.Progress(statsHook))
	if !a.stillActive(gen) {
		notes = append(notes, "cancelled — partial project list")
	}

	groups := render.GroupStatsKeys(func(su agentlog.SessionUsage) string { return su.Project }, rows)
	out := ProjectsOutcome{
		Agent: agent,
		Rows:  make([]ProjectRow, 0, len(groups)),
		Notes: a.combineNotes(adapters, notes),
	}
	for _, g := range groups {
		out.Rows = append(out.Rows, projectRowOf(g))
	}
	return out, nil
}

// ProjectStats breaks one exact project down by model. The CLI's --project
// is a case-insensitive substring filter, so the scanned rows are narrowed
// to the project's own sessions before grouping — sibling paths that merely
// contain the project path as a substring stay out.
func (a *App) ProjectStats(agent, project string) (ProjectStatsOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return ProjectStatsOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, agent); err != nil {
		return ProjectStatsOutcome{}, err
	}

	gen, _, statsHook := a.begin("projects")
	adapters := reg.Adapters()
	var notes []string
	rows := agentlog.CollectUsage(adapters, agentlog.SessionFilter{Agent: agent, Project: project}, "", func(note string) {
		notes = append(notes, note)
	}, agentlog.Progress(statsHook))
	if !a.stillActive(gen) {
		notes = append(notes, "cancelled — partial project stats")
	}

	own := make([]agentlog.SessionUsage, 0, len(rows))
	for _, su := range rows {
		if su.Project == project {
			own = append(own, su)
		}
	}
	return ProjectStatsOutcome{
		Agent:   agent,
		Project: project,
		Rows:    render.NewStatsRows(render.GroupStats(home, render.StatsByModel, own)),
		Notes:   a.combineNotes(adapters, notes),
	}, nil
}

// projectRowOf maps one aggregated group to the project row schema.
func projectRowOf(g render.StatsRow) ProjectRow {
	var cost *float64
	if g.Usage.HasCost {
		c := g.Usage.CostUSD
		cost = &c
	}
	return ProjectRow{
		Project:  g.Key,
		Sessions: g.Sessions,
		Messages: g.Messages,
		Tokens: render.StatsTokens{
			Input:      g.Usage.Input,
			Output:     g.Usage.Output,
			Reasoning:  g.Usage.Reasoning,
			CacheRead:  g.Usage.CacheRead,
			CacheWrite: g.Usage.CacheWrite,
			Total:      g.Usage.Input + g.Usage.Output + g.Usage.Reasoning,
		},
		CostUSD: cost,
	}
}
