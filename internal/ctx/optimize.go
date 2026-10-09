package ctx

import (
	"fmt"
	"sort"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// Cross-session optimize analysis (SPEC-optimize.md): the same IR stream the
// per-session rules R1–R5 run on, folded across every matching session to
// find waste that *repeats* — one oversized command is R1's job, the same
// oversized command paying its tax in half the sessions is R6's. The pass
// streams sessions one at a time and keeps counters only (no-index
// philosophy, SPEC-context-analysis §5's memory note).

// Thresholds (SPEC-optimize.md §2). Every cross rule requires ≥2 sessions —
// the "cross" qualifier: single-session repeats belong to R1/R2/R3.
const (
	r6MinCount     = 3     // occurrences of the same call
	r6MinSessions  = 2     // distinct sessions behind those occurrences
	r6MinAvgBytes  = 2000  // per-occurrence result size to call it heavy
	r7MinSessions  = 3     // distinct sessions re-running the same small call
	r8MinFails     = 3     // failed occurrences of the same call
	r8MinSessions  = 2     // distinct sessions behind those failures
	r9MinSessions  = 3     // sessions with exact first-turn usage
	r9MinPrefixTok = 16000 // smallest first-turn window worth flagging
	maxFindings    = 10    // top patterns; the rest is counted, not printed
)

// OptimizeFinding is one cross-session pattern with the numbers behind it.
type OptimizeFinding struct {
	Rule     string // R6..R9
	Tool     string
	Label    string // the recurring call (command, file path)
	Desc     string
	Sessions int   // distinct sessions contributing
	Count    int   // occurrences (R9: turns carrying the prefix)
	Bytes    int   // total result bytes behind the pattern
	Tokens   int64 // exact-token exposure (R9 only)
}

// impact is the sort key: R6–R8 speak bytes, R9 speaks tokens; a finding
// carries exactly one of the two.
func (f OptimizeFinding) impact() int64 {
	if f.Tokens > 0 {
		return f.Tokens
	}
	return int64(f.Bytes)
}

// OptimizeReport is the cross-session analysis result: findings in impact
// order, advice keyed to the fired rules, and how many patterns did not fit
// the top list.
type OptimizeReport struct {
	Sessions  int // sessions analyzed
	Findings  []OptimizeFinding
	Truncated int
}

// Advice returns the deduplicated advice lines for the fired rules, in the
// findings' impact order (SPEC-context-analysis §4's shape).
func (r OptimizeReport) Advice() []Advice {
	seen := map[string]bool{}
	order := []string{}
	for _, f := range r.Findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			order = append(order, f.Rule)
		}
	}
	out := make([]Advice, 0, len(order))
	for _, rule := range order {
		if a, ok := advicePool[rule]; ok {
			out = append(out, a)
		}
	}
	return out
}

// Optimizer folds session after session into the cross-session accumulators.
// Not concurrency-safe: one scan at a time, like every collection here.
type Optimizer struct {
	sessions int // batches observed

	groups map[string]*optGroup
	order  []string

	r9Sessions int   // sessions with an exact first turn
	r9Turns    int   // turns of those sessions
	r9Min      int64 // smallest exact first-turn window seen
}

// optGroup accumulates one (tool, canonical args) call identity across
// sessions.
type optGroup struct {
	tool      string
	label     string
	count     int
	bytes     int
	fails     int
	failBytes int
	head      string // first error headline seen
	sessions  map[string]bool
}

// NewOptimizer builds an empty accumulator set.
func NewOptimizer() *Optimizer {
	return &Optimizer{groups: map[string]*optGroup{}}
}

// Observe folds one session's event stream into the accumulators. Empty
// streams (agents without usage/records) contribute their session count
// only.
func (o *Optimizer) Observe(sessionID string, events []agentlog.CtxEvent) {
	o.sessions++

	// ---- per-session call groups (the open-call stack mirrors R2: a
	// result belongs to the nearest open call of its key)
	local := map[string]*optGroup{}
	var openKey []string
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxToolCall:
			key := ev.ArgsKey
			if key == "" {
				openKey = append(openKey, "")
				continue // unattributable call: no identity, no group
			}
			st, ok := local[key]
			if !ok {
				st = &optGroup{tool: ev.Tool, label: ev.Label, sessions: map[string]bool{}}
				local[key] = st
			}
			st.count++
			openKey = append(openKey, key)
		case agentlog.CtxToolResult:
			var key string
			if len(openKey) > 0 {
				key, openKey = openKey[len(openKey)-1], openKey[:len(openKey)-1]
			}
			if st, ok := local[key]; ok {
				st.bytes += ev.ResBytes
				if ev.Err {
					st.fails++
					st.failBytes += ev.ResBytes
					if st.head == "" {
						st.head = ev.Head
					}
				}
			}
		}
	}
	for key, st := range local {
		g, ok := o.groups[key]
		if !ok {
			g = &optGroup{tool: st.tool, label: st.label, sessions: map[string]bool{}}
			o.groups[key] = g
			o.order = append(o.order, key)
		}
		g.count += st.count
		g.bytes += st.bytes
		g.fails += st.fails
		g.failBytes += st.failBytes
		if g.head == "" {
			g.head = st.head
		}
		g.sessions[sessionID] = true
	}

	// ---- R9: the smallest exact first-turn window across sessions
	var turns int
	firstSum := int64(0)
	firstExact := false
	for _, ev := range events {
		if ev.Kind == agentlog.CtxTurnStart {
			turns++
			if turns == 1 {
				firstSum = ev.Tokens.Sum()
				firstExact = ev.TokensKind == agentlog.CtxTokensExact
			}
		}
	}
	if turns > 0 && firstExact && firstSum > 0 {
		o.r9Sessions++
		o.r9Turns += turns
		if o.r9Min == 0 || firstSum < o.r9Min {
			o.r9Min = firstSum
		}
	}
}

// Report folds the accumulators into the findings list: one finding per
// call group at most (precedence R6 > R8 > R7, SPEC-optimize.md §2.4), the
// R9 prefix check on top, sorted by impact and capped at the top patterns.
func (o *Optimizer) Report() OptimizeReport {
	rep := OptimizeReport{Sessions: o.sessions}
	for _, key := range o.order {
		g := o.groups[key]
		distinct := len(g.sessions)
		avg := 0
		if g.count > 0 {
			avg = g.bytes / g.count
		}
		switch {
		case g.count >= r6MinCount && distinct >= r6MinSessions && avg >= r6MinAvgBytes:
			rep.Findings = append(rep.Findings, OptimizeFinding{
				Rule:  "R6",
				Tool:  g.tool,
				Label: g.label,
				Desc: fmt.Sprintf("(%s) returned ~%s per run ×%d across %d sessions — every run pays it again",
					optTool(g.tool), humanTok(avg), g.count, distinct),
				Sessions: distinct,
				Count:    g.count,
				Bytes:    g.bytes,
			})
		case g.fails >= r8MinFails && distinct >= r8MinSessions:
			rep.Findings = append(rep.Findings, OptimizeFinding{
				Rule:  "R8",
				Tool:  g.tool,
				Label: g.label,
				Desc: fmt.Sprintf("(%s) failed ×%d across %d sessions — retries burned ~%s of output",
					optTool(g.tool), g.fails, distinct, humanTok(g.failBytes)),
				Sessions: distinct,
				Count:    g.fails,
				Bytes:    g.failBytes,
			})
		case distinct >= r7MinSessions:
			rep.Findings = append(rep.Findings, OptimizeFinding{
				Rule:  "R7",
				Tool:  g.tool,
				Label: g.label,
				Desc: fmt.Sprintf("(%s) ×%d across %d sessions — the same content re-enters the window every session (~%s total)",
					optTool(g.tool), g.count, distinct, humanTok(g.bytes)),
				Sessions: distinct,
				Count:    g.count,
				Bytes:    g.bytes,
			})
		}
	}
	if o.r9Sessions >= r9MinSessions && o.r9Min >= r9MinPrefixTok {
		rep.Findings = append(rep.Findings, OptimizeFinding{
			Rule: "R9",
			Desc: fmt.Sprintf("static context ~%s tokens (smallest first turn across %d sessions) rides in every request — ~%s tokens of prefix across %d turns",
				humanTok(int(o.r9Min)), o.r9Sessions, humanTok(int(o.r9Min)*o.r9Turns), o.r9Turns),
			Sessions: o.r9Sessions,
			Count:    o.r9Turns,
			Tokens:   o.r9Min * int64(o.r9Turns),
		})
	}

	sort.SliceStable(rep.Findings, func(i, j int) bool {
		if rep.Findings[i].impact() != rep.Findings[j].impact() {
			return rep.Findings[i].impact() > rep.Findings[j].impact()
		}
		if rep.Findings[i].Rule != rep.Findings[j].Rule {
			return rep.Findings[i].Rule < rep.Findings[j].Rule
		}
		return rep.Findings[i].Label < rep.Findings[j].Label
	})
	if len(rep.Findings) > maxFindings {
		rep.Truncated = len(rep.Findings) - maxFindings
		rep.Findings = rep.Findings[:maxFindings]
	}
	return rep
}

// optTool names the tool behind a finding; a result without a linked call
// has none (cross-file or legacy streams).
func optTool(tool string) string {
	if tool == "" {
		return "a tool"
	}
	return tool
}
