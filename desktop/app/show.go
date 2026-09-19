package app

import (
	"fmt"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// ShowOutcome is the `pastlog show` surface for the GUI. The ambiguous
// prefix case (the CLI's exit-2) becomes a candidate picker: Status
// "ambiguous" with the candidate list, not an error (spec §4.2).
type ShowOutcome struct {
	Status     string               `json:"status"` // "ok" | "ambiguous" | "notfound"
	Session    *render.SessionJSON  `json:"session,omitempty"`
	Entries    []render.EntryJSON   `json:"entries,omitempty"`
	Candidates []render.SessionJSON `json:"candidates,omitempty"`
	Unreadable []string             `json:"unreadable,omitempty"`
	Notes      []string             `json:"notes"`
}

// Entries resolves a session id or prefix and returns the full transcript.
func (a *App) Entries(idPrefix string) (ShowOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return ShowOutcome{}, err
	}
	adapters := cli.NewRegistry(home).Adapters()
	res, err := agentlog.ResolveSession(adapters, idPrefix)
	if !res.Found() {
		out := ShowOutcome{Status: "notfound", Notes: a.combineNotes(adapters, nil)}
		if len(res.Candidates) > 0 {
			out.Status = "ambiguous"
			out.Candidates = render.SessionRowsJSON(res.Candidates)
		}
		if err != nil {
			out.Unreadable = res.Unreadable
		} else {
			out.Unreadable = []string{}
		}
		return out, nil
	}

	var entries []agentlog.Entry
	if err := res.Adapter.Entries(res.Meta.Session, func(e agentlog.Entry) error {
		entries = append(entries, e)
		return nil
	}); err != nil {
		return ShowOutcome{}, fmt.Errorf("cannot read session %s: %v", res.Meta.ID, err)
	}
	session := render.NewSessionJSON(res.Meta)
	return ShowOutcome{
		Status:     "ok",
		Session:    &session,
		Entries:    render.NewShowDoc(res.Meta, entries).Entries,
		Unreadable: []string{},
		Notes:      a.combineNotes(adapters, nil),
	}, nil
}
