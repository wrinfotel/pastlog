// Package ctx implements the context-bloat analysis of SPEC
// context-analysis: rules R1–R5 run on the normalized agentlog.CtxEvent
// stream and produce findings + advice. Every agent feeds the same rules
// through its adapter's CtxSource — agent differences surface only as token
// precision (Exact vs Estimated), never as a different rule set.
package ctx

import (
	"fmt"
	"sort"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// Finding is one rule firing, with the numbers that made it fire.
type Finding struct {
	Rule    string // R1..R5
	Tool    string // primary tool involved ("")
	Desc    string // one-line description with numbers
	Bytes   int    // estimated share of the session's context, bytes
	Tokens  int64  // exact tokens when available (R3 burn, R5 delta)
	Exact   bool   // Tokens/Tokens-derived numbers are exact (not ~)
	Details []string
}

// Profile is the per-session analysis result.
type Profile struct {
	Final       int64 // C(last turn) — final window estimate, tokens
	FinalExact  bool
	Turns       int
	Compactions int
	Findings    []Finding
}

// Advice is one advice line keyed by rule id.
type Advice struct {
	Rule string
	Text string
}

// advicePool is keyed by rule id; one advice per fired rule (SPEC §4).
var advicePool = map[string]Advice{
	"R1": {Rule: "R1", Text: "redirect long tool output to a file, then read back only what you need"},
	"R2": {Rule: "R2", Text: "re-reads re-enter the file in full — ask for diffs or line ranges instead"},
	"R3": {Rule: "R3", Text: "fix the failing command before retrying the suite"},
	"R4": {Rule: "R4", Text: "compact earlier: a window near the limit makes every later turn slower and costlier"},
	"R5": {Rule: "R5", Text: "one turn moved a third of the window — inspect what ran there"},
}

// Analyze runs rules R1–R5 over one session's event stream.
func Analyze(events []agentlog.CtxEvent) Profile {
	p := Profile{}
	if len(events) == 0 {
		return p
	}

	// ---- per-turn context curve (C(t) = Input + CacheRead + CacheWrite)
	var turns []agentlog.CtxEvent // TurnStart events, in order
	exactTurns := 0
	for _, ev := range events {
		if ev.Kind == agentlog.CtxTurnStart {
			turns = append(turns, ev)
			if ev.TokensKind == agentlog.CtxTokensExact {
				exactTurns++
			}
		}
	}
	p.Turns = len(turns)
	p.FinalExact = p.Turns > 0 && exactTurns == p.Turns
	if n := len(turns); n > 0 {
		p.Final = turns[n-1].Tokens.Sum()
	}

	// ---- R4 growth & compactions
	p.analyzeGrowth(events, turns)

	// ---- R1 oversized results
	p.analyzeOversized(events)

	// ---- R2 repeated calls
	p.analyzeRepeats(events)

	// ---- R3 error loops
	p.analyzeErrorLoops(events)

	// ---- R5 anomalous turns
	p.analyzeAnomalous(turns)

	sort.SliceStable(p.Findings, func(i, j int) bool {
		return p.Findings[i].Bytes > p.Findings[j].Bytes
	})
	return p
}

// Advice returns the deduplicated advice lines for the fired rules,
// ordered by the biggest finding's impact (SPEC §4).
func (p Profile) Advice() []Advice {
	seen := map[string]bool{}
	order := []string{}
	for _, f := range p.Findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			order = append(order, f.Rule)
		}
	}
	out := make([]Advice, 0, len(order))
	for _, r := range order {
		if a, ok := advicePool[r]; ok {
			out = append(out, a)
		}
	}
	return out
}

// analyzeGrowth implements R4: compaction stats (drop %, regrow) and the
// no-plateau flag. Window sizes come from exact usage when present and are
// estimated from byte deltas otherwise.
func (p *Profile) analyzeGrowth(events, turns []agentlog.CtxEvent) {
	if len(turns) < 2 {
		return
	}
	// compact events between turns, in stream order
	var compacts []agentlog.CtxEvent
	for _, ev := range events {
		if ev.Kind == agentlog.CtxCompact {
			compacts = append(compacts, ev)
		}
	}
	p.Compactions = len(compacts)
	if p.Compactions == 0 {
		monotonic := true
		for i := 1; i < len(turns); i++ {
			if turns[i].Tokens.Sum() < turns[i-1].Tokens.Sum() {
				monotonic = false
				break
			}
		}
		if monotonic {
			p.Findings = append(p.Findings, Finding{
				Rule: "R4",
				Desc: "context grew monotonically all session — no plateau, plan compactions for long runs",
				Bytes: int(p.Final),
			})
		}
		return
	}
	// per-compaction: drop % and turns to regrow ≥ pre-drop level
	seqToTurnIdx := map[int]int{}
	for i, t := range turns {
		seqToTurnIdx[t.Seq] = i
	}
	for _, c := range compacts {
		// last turn before / first turn after the compact event
		pre, post := -1, -1
		for i, t := range turns {
			if t.Seq < c.Seq {
				pre = i
			}
			if t.Seq > c.Seq && post == -1 {
				post = i
			}
		}
		if pre < 0 || post < 0 {
			continue
		}
		before := turns[pre].Tokens.Sum()
		after := turns[post].Tokens.Sum()
		if before <= 0 {
			continue
		}
		drop := 100 * (before - after) / before
		if drop < 0 {
			drop = 0
		}
		regrow := 0
		if post >= 0 {
			for i := post + 1; i < len(turns); i++ {
				regrow++
				if turns[i].Tokens.Sum() >= before {
					break
				}
			}
		}
		p.Findings = append(p.Findings, Finding{
			Rule:  "R4",
			Desc:  fmt.Sprintf("compact at turn %d: −%d%%, back at pre-drop level after %d turns", pre+1, drop, regrow),
			Bytes: int(before),
			Exact: p.FinalExact,
		})
	}
}

// analyzeOversized implements R1 (SPEC §2): a result ≥10% of all result
// bytes fires individually (top-3 scan); dedup keeps one finding per
// (tool,label). Trio rule fires only when no giant was found.
func (p *Profile) analyzeOversized(events []agentlog.CtxEvent) {
	total := 0
	var sizes []int
	var tools []string
	var labels []string
	for _, ev := range events {
		if ev.Kind == agentlog.CtxToolResult && ev.ResBytes > 0 {
			total += ev.ResBytes
			sizes = append(sizes, ev.ResBytes)
			tools = append(tools, ev.Tool)
			labels = append(labels, "")
		}
	}
	if total == 0 {
		return
	}
	// label the i-th result with the nearest preceding same-tool call's label
	ri := 0
	last := map[string]string{}
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxToolCall:
			if ev.Label != "" {
				last[ev.Tool] = ev.Label
			}
		case agentlog.CtxToolResult:
			labels[ri] = last[ev.Tool]
			ri++
		}
	}

	order := make([]int, len(sizes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return sizes[order[i]] > sizes[order[j]] })

	seen := map[string]bool{}
	fired := 0
	for i := 0; i < 3 && i < len(order); i++ {
		ix := order[i]
		pct := 100 * sizes[ix] / total
		if pct < 10 {
			break
		}
		key := tools[ix] + "\x00" + labels[ix]
		if seen[key] {
			continue
		}
		seen[key] = true
		fired++
		who := tools[ix]
		if who == "" {
			who = "a tool" // result not linked to a call (cross-file or legacy)
		}
		p.Findings = append(p.Findings, Finding{
			Rule:  "R1",
			Tool:  tools[ix],
			Desc:  fmt.Sprintf("%s returned %s (~%d%% of all result bytes)", who, dispLabel(labels[ix], sizes[ix]), pct),
			Bytes: sizes[ix],
		})
	}
	if fired == 0 {
		top3 := sizes[order[0]]
		for i := 1; i < 3 && i < len(order); i++ {
			top3 += sizes[order[i]]
		}
		if 100*top3 >= 40*total {
			ix := order[0]
			p.Findings = append(p.Findings, Finding{
				Rule:  "R1",
				Tool:  tools[ix],
				Desc:  fmt.Sprintf("top-3 results hold ~%d%% of all result bytes, e.g. %s from %s", 100*top3/total, dispLabel(labels[ix], sizes[ix]), tools[ix]),
				Bytes: top3,
			})
		}
	}
}

func dispLabel(label string, bytes int) string {
	name := label
	if name == "" {
		name = "output"
	}
	if len(name) > 40 {
		name = name[:37] + "…"
	}
	return fmt.Sprintf("%s %s", humanTok(bytes), name)
}

func humanTok(b int) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0fk", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d", b)
	}
}

// analyzeRepeats implements R2 (SPEC §1.1, §2.4): the same ArgsKey called
// again after at least one assistant turn in between; back-to-back calls
// are retries and belong to R3. Reports count and total bytes of instances.
func (p *Profile) analyzeRepeats(events []agentlog.CtxEvent) {
	type stat struct {
		tool    string
		label   string
		count   int  // calls seen
		repeats int  // calls with ≥1 turn since the previous same-key call
		bytes   int  // total result bytes attributed to this key
		lastSeq int  // seq of the previous call
		turnGap bool // a turn separated this call from the previous one
	}
	order := []string{}
	stats := map[string]*stat{}
	lastTurnSeq := 0
	var openKey []string // per-result stack: keys of open calls awaiting results

	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxTurnStart:
			lastTurnSeq = ev.Seq
		case agentlog.CtxToolCall:
			if ev.ArgsKey == "" {
				openKey = append(openKey, "")
				continue
			}
			st, ok := stats[ev.ArgsKey]
			if !ok {
				st = &stat{tool: ev.Tool, label: ev.Label}
				stats[ev.ArgsKey] = st
				order = append(order, ev.ArgsKey)
			} else if lastTurnSeq > st.lastSeq {
				st.repeats++
			}
			st.count++
			st.lastSeq = ev.Seq
			openKey = append(openKey, ev.ArgsKey)
		case agentlog.CtxToolResult:
			var key string
			if len(openKey) > 0 {
				key, openKey = openKey[len(openKey)-1], openKey[:len(openKey)-1]
			}
			if st, ok := stats[key]; ok {
				st.bytes += ev.ResBytes
			}
		}
	}
	for _, k := range order {
		st := stats[k]
		if st.repeats >= 1 {
			p.Findings = append(p.Findings, Finding{
				Rule:  "R2",
				Tool:  st.tool,
				Desc:  fmt.Sprintf("%s called ×%d (%s) — every re-read re-enters the window", dispLabel(st.label, st.bytes), st.count, st.tool),
				Bytes: st.bytes,
			})
		}
	}
}

// analyzeErrorLoops implements R3 (SPEC §2.2): the same tool erroring 3+
// consecutive attempts (results may be back-to-back or interleaved with one
// user message). The loop's headline is the first error's own first line
// when the adapter could not attribute a tool name (regex-flagged agents).
func (p *Profile) analyzeErrorLoops(events []agentlog.CtxEvent) {
	type loop struct {
		tool  string
		head  string // first error's first line, for nameless attribution
		fails int
		bytes int
	}
	flush := func(l loop) {
		if l.fails >= 3 {
			who := l.tool
			if who == "" {
				who = "\"" + truncateOneLine(l.head, 40) + "\""
			}
			p.Findings = append(p.Findings, Finding{
				Rule:  "R3",
				Tool:  l.tool,
				Desc:  fmt.Sprintf("%s failed %d× in a row (~%s of output burned on retries)", who, l.fails, humanTok(l.bytes)),
				Bytes: l.bytes,
			})
		}
	}
	cur := loop{}
	for _, ev := range events {
		switch {
		case ev.Kind == agentlog.CtxToolResult && ev.Err:
			sameLoop := ev.Tool != "" && ev.Tool == cur.tool
			if !sameLoop && cur.fails > 0 {
				flush(cur)
				cur = loop{}
			}
			if cur.head == "" {
				cur.head = ev.Head
			}
			cur.tool = ev.Tool
			cur.fails++
			cur.bytes += ev.ResBytes
		case ev.Kind == agentlog.CtxToolResult, ev.Kind == agentlog.CtxMessage && ev.Role == "user":
			flush(cur)
			cur = loop{}
		default:
			// tool calls, assistant text, compactions: part of the loop
		}
	}
	flush(cur)
}

// truncateOneLine clips s to max runes on one line.
func truncateOneLine(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// analyzeAnomalous implements R5 (SPEC §2.5): one turn's delta > 30% of the
// session's peak window (exact path only; byte-based deltas are too noisy
// to accuse). Peak — not final: after a compaction the final window is
// small, and a big pre-compaction turn would be mislabelled (>100%).
func (p *Profile) analyzeAnomalous(turns []agentlog.CtxEvent) {
	if len(turns) < 2 {
		return
	}
	peak := int64(0)
	for _, t := range turns {
		if s := t.Tokens.Sum(); s > peak {
			peak = s
		}
	}
	if peak <= 0 {
		return
	}
	for i := 1; i < len(turns); i++ {
		delta := turns[i].Tokens.Sum() - turns[i-1].Tokens.Sum()
		if delta > 0 && delta*100 > 30*peak {
			p.Findings = append(p.Findings, Finding{
				Rule:   "R5",
				Desc:   fmt.Sprintf("turn %d added ~%s tokens (%d%% of the peak window) — inspect what ran there", i+1, humanTok(int(delta)), 100*delta/peak),
				Tokens: delta,
				Exact:  true,
			})
		}
	}
}

// BytesPerTokenFallback is used by the renderer when a session had no exact
// tokens at all: bytes/4 (SPEC §6 non-goals keep the proxy simple).
const BytesPerTokenFallback = 4
