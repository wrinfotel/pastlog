package app

import (
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// SessionModelRow is one model's token share of a session — the row the
// MODELS panel renders like the project page's model table, without the
// sessions/messages/cost columns (a session has one row set, no pricing).
type SessionModelRow struct {
	Model  string             `json:"model"`
	Tokens render.StatsTokens `json:"tokens"`
}

// SessionModelsOutcome is one session's per-model token breakdown. The
// ambiguous prefix case becomes the candidate picker, like Entries.
type SessionModelsOutcome struct {
	Status     string               `json:"status"` // "ok" | "ambiguous" | "notfound"
	Session    *render.SessionJSON  `json:"session,omitempty"`
	Rows       []SessionModelRow    `json:"rows,omitempty"`
	Candidates []render.SessionJSON `json:"candidates,omitempty"`
	Unreadable []string             `json:"unreadable,omitempty"`
	Notes      []string             `json:"notes"`
}

// SessionModels resolves a session id or prefix and returns its per-model
// token usage: the engine's per-model split where the agent records one,
// otherwise a single row attributed to the session's model.
func (a *App) SessionModels(idPrefix string) (SessionModelsOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return SessionModelsOutcome{}, err
	}
	adapters := cli.NewRegistry(home).Adapters()
	res, err := agentlog.ResolveSession(adapters, idPrefix)
	if !res.Found() {
		out := SessionModelsOutcome{Status: "notfound", Notes: a.combineNotes(adapters, nil)}
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

	usages, _ := agentlog.SessionModelUsage(res.Adapter, res.Meta.ID)
	session := render.NewSessionJSON(res.Meta)
	out := SessionModelsOutcome{
		Status:  "ok",
		Session: &session,
		Rows:    make([]SessionModelRow, 0, len(usages)),
		Notes:   a.combineNotes(adapters, nil),
	}
	for _, u := range usages {
		out.Rows = append(out.Rows, SessionModelRow{
			Model: u.Model,
			Tokens: render.StatsTokens{
				Input:      u.Input,
				Output:     u.Output,
				Reasoning:  u.Reasoning,
				CacheRead:  u.CacheRead,
				CacheWrite: u.CacheWrite,
				Total:      u.Input + u.Output + u.Reasoning,
			},
		})
	}
	return out, nil
}
