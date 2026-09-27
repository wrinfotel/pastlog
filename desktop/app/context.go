package app

import (
	"fmt"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/ctx"
	"github.com/wrinfotel/pastlog/internal/render"
)

// ContextOutcome is the `pastlog context` surface for the GUI — the session
// card's context panel. Status mirrors ShowOutcome ("ok" | "ambiguous" |
// "notfound"); "unsupported" covers a future adapter without a CtxSource.
type ContextOutcome struct {
	Status     string               `json:"status"`
	Session    *render.SessionJSON  `json:"session,omitempty"`
	Profile    *ContextProfileJSON  `json:"profile,omitempty"`
	Candidates []render.SessionJSON `json:"candidates,omitempty"`
	Notes      []string             `json:"notes"`
}

// ContextProfileJSON is ctx.Profile shaped for the frontend: numbers the UI
// formats itself, the sparkline pre-rendered by ctx.Sparkline (1/8-block
// glyphs, gaps at compactions), findings already in impact order.
type ContextProfileJSON struct {
	Final       int64                `json:"final"`
	FinalExact  bool                 `json:"final_exact"`
	Turns       int                  `json:"turns"`
	Compactions int                  `json:"compactions"`
	Sparkline   string               `json:"sparkline"`
	Findings    []ContextFindingJSON `json:"findings"`
	Advice      []ContextAdviceJSON  `json:"advice"`
	Precision   string               `json:"precision"` // "exact tokens" | "estimated tokens"
}

// ContextFindingJSON is one fired rule with the numbers behind it.
type ContextFindingJSON struct {
	Rule  string `json:"rule"` // R1..R5
	Tool  string `json:"tool,omitempty"`
	Desc  string `json:"desc"`
	Bytes int    `json:"bytes"`
}

// ContextAdviceJSON is one advice line keyed by the rule that fired it.
type ContextAdviceJSON struct {
	Rule string `json:"rule"`
	Text string `json:"text"`
}

// Context resolves a session id or prefix and analyzes why its context grew —
// the `pastlog context` contract: the same rules R1–R5 for every agent.
func (a *App) Context(idPrefix string) (ContextOutcome, error) {
	home, err := a.effectiveHome()
	if err != nil {
		return ContextOutcome{}, err
	}
	adapters := cli.NewRegistry(home).Adapters()
	// ResolveSession's error is the plain "no match" reason — it is true
	// exactly when !res.Found(), and every outcome below is resolution-driven.
	res, _ := agentlog.ResolveSession(adapters, idPrefix) //nolint:errcheck // see above
	if !res.Found() {
		out := ContextOutcome{Status: "notfound", Notes: a.combineNotes(adapters, nil)}
		if len(res.Candidates) > 0 {
			out.Status = "ambiguous"
			out.Candidates = render.SessionRowsJSON(res.Candidates)
		}
		return out, nil
	}

	src, ok := res.Adapter.(agentlog.CtxSource)
	session := render.NewSessionJSON(res.Meta)
	if !ok {
		return ContextOutcome{
			Status:  "unsupported",
			Session: &session,
			Notes:   a.combineNotes(adapters, nil),
		}, nil
	}
	events, err := src.ContextEvents(res.Meta.Session)
	if err != nil {
		return ContextOutcome{}, fmt.Errorf("cannot analyze session %s: %v", res.Meta.ID, err)
	}
	return ContextOutcome{
		Status:  "ok",
		Session: &session,
		Profile: contextProfile(events),
		Notes:   a.combineNotes(adapters, nil),
	}, nil
}

// contextProfile runs the shared rules and maps the result for the GUI.
// Glue only — the analysis semantics live in internal/ctx.
func contextProfile(events []agentlog.CtxEvent) *ContextProfileJSON {
	p := ctx.Analyze(events)
	out := &ContextProfileJSON{
		Final:       p.Final,
		FinalExact:  p.FinalExact,
		Turns:       p.Turns,
		Compactions: p.Compactions,
		Findings:    []ContextFindingJSON{},
		Advice:      []ContextAdviceJSON{},
	}
	turns, compactBefore := ctx.TurnCurve(events)
	// long sessions pool into ≤80 bars: an unpooled 608-turn sparkline is
	// ~600 glyphs and wraps far past the panel edge
	turns, compactBefore = ctx.DownsampleCurve(turns, compactBefore, 80)
	out.Sparkline = ctx.Sparkline(turns, compactBefore)
	for _, f := range p.Findings {
		out.Findings = append(out.Findings, ContextFindingJSON{Rule: f.Rule, Tool: f.Tool, Desc: f.Desc, Bytes: f.Bytes})
	}
	for _, adv := range p.Advice() {
		out.Advice = append(out.Advice, ContextAdviceJSON{Rule: adv.Rule, Text: adv.Text})
	}
	out.Precision = "exact tokens"
	if !p.FinalExact {
		out.Precision = "estimated tokens"
	}
	return out
}
