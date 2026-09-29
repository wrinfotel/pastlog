package app

import (
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/mask"
	"github.com/wrinfotel/pastlog/internal/render"
)

// TimelineOutcome is the `pastlog timeline --messages` surface for the GUI:
// the merged cross-session message stream, oldest first, capped at the CLI's
// default max rows. The session field carries the full id so the viewer can
// open the source session directly.
type TimelineOutcome struct {
	Events []render.TimelineEventJSON `json:"events"`
	Notes  []string                   `json:"notes"`
}

// Timeline merges the message entries of every session under the filter into
// one chronological stream. A long op: progress events and cancellation like
// Stats. Masking follows the settings toggle — the GUI, like the CLI's human
// output, never shows raw secrets; the JSON schema itself stays verbatim and
// masking applies to the rendered Text only.
func (a *App) Timeline(f FilterOptions) (TimelineOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return TimelineOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, f.Agent); err != nil {
		return TimelineOutcome{}, err
	}
	filter, err := a.buildFilter(f)
	if err != nil {
		return TimelineOutcome{}, err
	}

	gen, _, statsHook := a.begin("timeline")
	adapters := reg.Adapters()
	var notes []string
	events := agentlog.CollectTimelineEvents(adapters, filter, cli.DefaultMaxTimelineRows, func(note string) {
		notes = append(notes, note)
	}, agentlog.Progress(statsHook))
	if !a.stillActive(gen) {
		notes = append(notes, "cancelled — partial timeline")
	}
	if a.masking() {
		for i := range events {
			events[i].Text = mask.Mask(events[i].Text)
		}
	}
	return TimelineOutcome{
		Events: render.NewTimelineEvents(events),
		Notes:  a.combineNotes(adapters, notes),
	}, nil
}
