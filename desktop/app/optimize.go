package app

import (
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/ctx"
	"github.com/wrinfotel/pastlog/internal/mask"
	"github.com/wrinfotel/pastlog/internal/render"
)

// OptimizeOptions mirrors the CLI's optimize flags 1:1.
type OptimizeOptions struct {
	Filter FilterOptions `json:"filter"`
}

// OptimizeOutcome is the `pastlog optimize` surface for the GUI: the
// project-level waste report. Findings arrive in impact order; labels and
// descriptions are masked like every other human surface.
type OptimizeOutcome struct {
	Report *render.OptimizeReportJSON `json:"report,omitempty"`
	Notes  []string                   `json:"notes"`
}

// Optimize folds the context analysis across every matching session and
// reports what repeats (SPEC-optimize.md). Long-op semantics like stats:
// progress events under the "optimize" op, cancellation keeps the partial
// report.
func (a *App) Optimize(o OptimizeOptions) (OptimizeOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return OptimizeOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, o.Filter.Agent); err != nil {
		return OptimizeOutcome{}, err
	}
	filter, err := a.buildFilter(o.Filter)
	if err != nil {
		return OptimizeOutcome{}, err
	}

	gen, _, statsHook := a.begin("optimize")
	adapters := reg.Adapters()
	var notes []string
	opt := ctx.NewOptimizer()
	_, err = agentlog.CollectCtxBatches(adapters, filter, func(note string) {
		notes = append(notes, note)
	}, func(m agentlog.SessionMeta, events []agentlog.CtxEvent) error {
		opt.Observe(m.ID, events)
		return nil
	}, agentlog.Progress(statsHook))
	if err != nil {
		return OptimizeOutcome{}, err
	}
	if !a.stillActive(gen) {
		notes = append(notes, "cancelled — partial report")
	}
	report := render.NewOptimizeReport(opt.Report())
	if a.masking() {
		for i := range report.Findings {
			report.Findings[i].Label = mask.Mask(report.Findings[i].Label)
			report.Findings[i].Desc = mask.Mask(report.Findings[i].Desc)
		}
	}
	return OptimizeOutcome{
		Report: &report,
		Notes:  a.combineNotes(adapters, notes),
	}, nil
}
