package ctx

import (
	"fmt"
	"io"
	"strings"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// RenderHuman writes the `pastlog context` report (SPEC §4): header, sparkline,
// findings, compactions, advice, precision note. Deterministic, no color.
func RenderHuman(w io.Writer, meta agentlog.SessionMeta, p Profile) {
	// header
	prec := "exact tokens"
	if !p.FinalExact {
		prec = "estimated tokens"
	}
	fmt.Fprintf(w, "context profile: %s (%s, %s, %d turns)\n\n",
		renderID(meta.ID), meta.Agent, fmtWindow(p.Final, p.FinalExact), p.Turns)

	// findings (already sorted by impact)
	if len(p.Findings) == 0 {
		fmt.Fprintln(w, "no context-bloat signals — the session stayed lean")
	} else {
		fmt.Fprintln(w, "findings:")
		for _, f := range p.Findings {
			fmt.Fprintf(w, "  %s  %s\n", f.Rule, f.Desc)
		}
	}

	// compactions summary
	if p.Compactions > 0 {
		n := p.Compactions
		fmt.Fprintf(w, "\ncompactions: %d\n", n)
	}

	// advice, dedup by rule, impact order
	if advice := p.Advice(); len(advice) > 0 {
		fmt.Fprintln(w, "\nadvice:")
		for _, a := range advice {
			fmt.Fprintf(w, "  • %s (%s)\n", a.Text, a.Rule)
		}
	}

	fmt.Fprintf(w, "\nprecision: %s (%s)\n", prec, meta.Agent)
}

// renderID keeps the listing convention: 8-char prefix.
func renderID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// fmtWindow renders the final window estimate: exact when the session had
// complete per-turn usage, tilde-estimated otherwise (SPEC §0).
func fmtWindow(tokens int64, exact bool) string {
	if exact {
		return fmt.Sprintf("~%s tokens final", humanTok(int(tokens)))
	}
	return fmt.Sprintf("~%s tokens final (estimated)", humanTok(int(tokens)))
}

// TurnCurve reduces an event stream to the per-turn context curve: the
// window proxy C(t) = Input + CacheRead + CacheWrite per TurnStart, and,
// parallel to it, whether a compaction happened since the previous turn —
// the inputs Sparkline renders. Agents without usage yield empty curves.
func TurnCurve(events []agentlog.CtxEvent) (turns []int64, compactBefore []bool) {
	pending := false
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxCompact:
			pending = true
		case agentlog.CtxTurnStart:
			turns = append(turns, ev.Tokens.Sum())
			compactBefore = append(compactBefore, pending)
			pending = false
		}
	}
	return turns, compactBefore
}

// DownsampleCurve pools a curve longer than bars into equal turn buckets,
// keeping the bucket maximum (spikes survive pooling) and OR-ing the
// compaction marks within a bucket. Curves at or under bars pass through,
// so short sessions keep their per-turn sparkline.
func DownsampleCurve(turns []int64, compactBefore []bool, bars int) ([]int64, []bool) {
	if bars <= 0 || len(turns) <= bars {
		return turns, compactBefore
	}
	size := (len(turns) + bars - 1) / bars
	ot := make([]int64, 0, bars)
	ob := make([]bool, 0, bars)
	for start := 0; start < len(turns); start += size {
		end := min(start+size, len(turns))
		peak := turns[start]
		gap := false
		for i := start; i < end; i++ {
			if turns[i] > peak {
				peak = turns[i]
			}
			if i < len(compactBefore) && compactBefore[i] {
				gap = true
			}
		}
		ot = append(ot, peak)
		ob = append(ob, gap)
	}
	return ot, ob
}

// Sparkline renders the per-turn context curve with 1/8-block glyphs,
// normalized to the session's peak. Compaction positions render as ▏gaps.
func Sparkline(turns []int64, compactBefore []bool) string {
	if len(turns) == 0 {
		return ""
	}
	peak := turns[0]
	for _, v := range turns {
		if v > peak {
			peak = v
		}
	}
	if peak <= 0 {
		return ""
	}
	glyphs := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for i, v := range turns {
		if i > 0 && compactBefore != nil && compactBefore[i] {
			b.WriteRune(' ') // visual gap at the compaction boundary
		}
		idx := len(glyphs) - 1
		if v < peak {
			idx = int(float64(v) / float64(peak) * float64(len(glyphs)-1))
		}
		b.WriteRune(glyphs[idx])
	}
	return b.String()
}
