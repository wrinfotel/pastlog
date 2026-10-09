package render

import (
	"fmt"
	"io"

	"github.com/wrinfotel/pastlog/internal/ctx"
)

// The `pastlog optimize` surface (SPEC-optimize.md §4): the cross-session
// waste patterns with their numbers, and the advice pool keyed to the fired
// rules. The desktop binding reuses these structs, like the timeline's.

// OptimizeFindingJSON is one cross-session pattern. Bytes speaks result
// bytes (R6–R8); Tokens carries R9's exact-token exposure and is omitted
// otherwise.
type OptimizeFindingJSON struct {
	Rule     string `json:"rule"`
	Tool     string `json:"tool,omitempty"`
	Label    string `json:"label,omitempty"`
	Desc     string `json:"desc"`
	Sessions int    `json:"sessions"`
	Count    int    `json:"count"`
	Bytes    int    `json:"bytes"`
	Tokens   int64  `json:"tokens,omitempty"`
}

// OptimizeAdviceJSON is one advice line keyed by the rule that fired it.
type OptimizeAdviceJSON struct {
	Rule string `json:"rule"`
	Text string `json:"text"`
}

// OptimizeReportJSON is the report: findings in impact order, advice deduped
// by rule, and how many smaller patterns did not fit the top list.
type OptimizeReportJSON struct {
	Sessions  int                   `json:"sessions"`
	Findings  []OptimizeFindingJSON `json:"findings"`
	Advice    []OptimizeAdviceJSON  `json:"advice"`
	Truncated int                   `json:"truncated,omitempty"`
}

// NewOptimizeReport maps the analysis result to its stable JSON shape.
func NewOptimizeReport(r ctx.OptimizeReport) OptimizeReportJSON {
	out := OptimizeReportJSON{
		Sessions:  r.Sessions,
		Findings:  []OptimizeFindingJSON{},
		Advice:    []OptimizeAdviceJSON{},
		Truncated: r.Truncated,
	}
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, OptimizeFindingJSON{
			Rule: f.Rule, Tool: f.Tool, Label: f.Label, Desc: f.Desc,
			Sessions: f.Sessions, Count: f.Count, Bytes: f.Bytes, Tokens: f.Tokens,
		})
	}
	for _, a := range r.Advice() {
		out.Advice = append(out.Advice, OptimizeAdviceJSON{Rule: a.Rule, Text: a.Text})
	}
	return out
}

// OptimizeJSON writes the report with its stable schema.
func OptimizeJSON(w io.Writer, r ctx.OptimizeReport) error {
	return writeJSON(w, NewOptimizeReport(r))
}

// labelCap caps a finding's label in human output. Truncation happens here,
// after the caller has masked the label, so a printed line is always a
// substring of the masked text — a cut can never resurrect a secret.
const labelCap = 80

// OptimizeHuman writes the cross-session report: header, findings, advice,
// truncation note. Deterministic, no color — same discipline as the
// per-session context report.
func OptimizeHuman(w io.Writer, r ctx.OptimizeReport) {
	fmt.Fprintf(w, "optimize report: %d sessions analyzed\n\n", r.Sessions)
	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "no cross-session waste patterns — the history looks lean")
		return
	}
	fmt.Fprintln(w, "findings:")
	for _, f := range r.Findings {
		if f.Label != "" {
			fmt.Fprintf(w, "  %s  %s %s\n", f.Rule, capLabel(f.Label), f.Desc)
		} else {
			fmt.Fprintf(w, "  %s  %s\n", f.Rule, f.Desc)
		}
	}
	if advice := r.Advice(); len(advice) > 0 {
		fmt.Fprintln(w, "\nadvice:")
		for _, a := range advice {
			fmt.Fprintf(w, "  • %s (%s)\n", a.Text, a.Rule)
		}
	}
	if r.Truncated > 0 {
		fmt.Fprintf(w, "\n(+%d smaller patterns omitted)\n", r.Truncated)
	}
}

// capLabel clips a label to labelCap runes on one line.
func capLabel(s string) string {
	runes := []rune(s)
	if len(runes) <= labelCap {
		return s
	}
	return string(runes[:labelCap-1]) + "…"
}
