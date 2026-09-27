package app

import (
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// RelatedOutcome is the `pastlog related` surface for the GUI: the anchor
// session, then its parent, subagent children and adjacent same-project
// sessions. The ambiguous prefix case becomes the candidate picker, like
// Entries.
type RelatedOutcome struct {
	Status     string               `json:"status"` // "ok" | "ambiguous" | "notfound"
	Session    *render.SessionJSON  `json:"session,omitempty"`
	Parent     *render.SessionJSON  `json:"parent,omitempty"`
	Children   []render.SessionJSON `json:"children,omitempty"`
	Adjacent   []render.SessionJSON `json:"adjacent,omitempty"`
	Candidates []render.SessionJSON `json:"candidates,omitempty"`
	Unreadable []string             `json:"unreadable,omitempty"`
	Notes      []string             `json:"notes"`
}

// Related resolves a session id or prefix and returns its relatedness graph.
func (a *App) Related(idPrefix string) (RelatedOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return RelatedOutcome{}, err
	}
	adapters := cli.NewRegistry(home).Adapters()
	res, err := agentlog.ResolveSession(adapters, idPrefix)
	if !res.Found() {
		out := RelatedOutcome{Status: "notfound", Notes: a.combineNotes(adapters, nil)}
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

	rel := agentlog.Related(adapters, res.Meta)
	doc := render.NewRelatedDoc(rel)
	out := RelatedOutcome{
		Status:   "ok",
		Session:  &doc.Session,
		Parent:   doc.Parent,
		Children: doc.Children,
		Adjacent: doc.Adjacent,
		Notes:    a.combineNotes(adapters, nil),
	}
	if out.Children == nil {
		out.Children = []render.SessionJSON{}
	}
	if out.Adjacent == nil {
		out.Adjacent = []render.SessionJSON{}
	}
	if len(rel.Unreadable) > 0 {
		out.Unreadable = rel.Unreadable
	} else {
		out.Unreadable = []string{}
	}
	return out, nil
}
