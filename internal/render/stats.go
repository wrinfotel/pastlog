// Stats renderers for `pastlog stats` (M7): one aggregated row per group,
// a human table with a header and thousands separators, and a stable JSON
// schema with struct-defined key order.
package render

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// Grouping modes for `pastlog stats --by` (a single grouping; multi-grouping
// is deferred per the M7 rulings).
const (
	StatsByAgent   = "agent"
	StatsByProject = "project"
	StatsByDay     = "day"
	StatsByModel   = "model"
)

const dayLayout = "2006-01-02"

// StatsRow is one aggregated group row of `pastlog stats`. Key is the
// display form of the grouping key; Usage carries the summed token totals
// with HasCost=true when at least one session in the group provided a cost.
type StatsRow struct {
	Key      string
	Sessions int
	Messages int
	Usage    agentlog.Usage
}

// GroupStats aggregates usage rows into groups keyed by the display form of
// the --by dimension: the agent name; the tilde-shortened project path
// ("-" when empty); the local date 2006-01-02 of the session start
// ("-" for a zero start time); or — for --by model — one row per model the
// session used ("-" for an unknown model). Rows come back in ascending key
// order — deterministic, and chronological for --by day.
func GroupStats(home, by string, rows []agentlog.SessionUsage) []StatsRow {
	if by == StatsByModel {
		return groupStatsPerUsage(rows, modelUsageEntries, modelKey)
	}
	return GroupStatsKeys(func(su agentlog.SessionUsage) string {
		return statsKey(home, by, su)
	}, rows)
}

// GroupStatsKeys is the raw-key accumulator behind GroupStats: rows are
// grouped by whatever key the caller derives (GroupStats derives the display
// form of --by; the desktop Projects view groups by the raw project path,
// whose exact value is its drill-down key). Same accumulation semantics and
// ascending key order as GroupStats.
func GroupStatsKeys(keyOf func(agentlog.SessionUsage) string, rows []agentlog.SessionUsage) []StatsRow {
	acc := statsAccumulator{}
	for _, su := range rows {
		acc.add(keyOf(su), su.Messages, su.Usage)
	}
	return acc.rows()
}

// groupStatsPerUsage aggregates rows into one group per (session × usage
// entry) — the model view: a session contributes its sessions/messages
// counters to every entry's group ("sessions that used this model"), while
// tokens and cost come from the entry itself, never the session total.
// Ascending key order, like every grouping.
func groupStatsPerUsage(rows []agentlog.SessionUsage, entriesOf func(agentlog.SessionUsage) []agentlog.Usage, keyOf func(agentlog.Usage) string) []StatsRow {
	acc := statsAccumulator{}
	for _, su := range rows {
		for _, u := range entriesOf(su) {
			acc.add(keyOf(u), su.Messages, u)
		}
	}
	return acc.rows()
}

// modelUsageEntries returns the usage entries a session feeds to the model
// view: the adapter's per-model breakdown when one was provided, the whole
// session usage otherwise (single-model sessions and adapters that cannot
// split keep the exact pre-split behavior).
func modelUsageEntries(su agentlog.SessionUsage) []agentlog.Usage {
	if len(su.Models) > 0 {
		return su.Models
	}
	return []agentlog.Usage{su.Usage}
}

// modelKey is the display key of one usage entry in the model view.
func modelKey(u agentlog.Usage) string {
	if u.Model == "" {
		return "-"
	}
	return u.Model
}

// statsAccumulator folds (group key, counters, usage) triples into StatsRows
// shared by the single-key and per-usage aggregations; rows() releases them
// in ascending key order.
type statsAccumulator map[string]*StatsRow

func (acc statsAccumulator) add(key string, messages int, u agentlog.Usage) {
	g, ok := acc[key]
	if !ok {
		g = &StatsRow{Key: key}
		acc[key] = g
	}
	g.Sessions++
	g.Messages += messages
	g.Usage.Input += u.Input
	g.Usage.Output += u.Output
	g.Usage.Reasoning += u.Reasoning
	g.Usage.CacheRead += u.CacheRead
	g.Usage.CacheWrite += u.CacheWrite
	if u.HasCost {
		g.Usage.HasCost = true
		g.Usage.CostUSD += u.CostUSD // usages without cost contribute nothing
	}
}

func (acc statsAccumulator) rows() []StatsRow {
	out := make([]StatsRow, 0, len(acc))
	for _, g := range acc {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// statsKey derives one row's display key for the grouping mode (--by model
// keys per used model instead, via groupStatsPerUsage).
func statsKey(home, by string, su agentlog.SessionUsage) string {
	switch by {
	case StatsByProject:
		return displayProject(home, su.Project)
	case StatsByDay:
		if su.StartedAt.IsZero() {
			return "-"
		}
		return su.StartedAt.Local().Format(dayLayout)
	default:
		return su.Agent
	}
}

// statsColumn is one numeric table column: its header and, per row, the
// formatted cell (cost cells may be "-" when the group has no cost).
type statsColumn struct {
	header string
	cell   func(StatsRow) string
}

// statsColumns lists the numeric columns after the key; the cost column is
// appended only when some row carries a cost (StatsHuman).
func statsColumns(withCost bool) []statsColumn {
	cols := []statsColumn{
		{"sessions", func(r StatsRow) string { return strconv.Itoa(r.Sessions) }},
		{"messages", func(r StatsRow) string { return strconv.Itoa(r.Messages) }},
		{"input", func(r StatsRow) string { return groupDigits(r.Usage.Input) }},
		{"output", func(r StatsRow) string { return groupDigits(r.Usage.Output) }},
		{"reasoning", func(r StatsRow) string { return groupDigits(r.Usage.Reasoning) }},
		{"cache read", func(r StatsRow) string { return groupDigits(r.Usage.CacheRead) }},
		{"cache write", func(r StatsRow) string { return groupDigits(r.Usage.CacheWrite) }},
		{"total", func(r StatsRow) string { // ruling 3: input+output+reasoning
			return groupDigits(r.Usage.Input + r.Usage.Output + r.Usage.Reasoning)
		}},
	}
	if withCost {
		cols = append(cols, statsColumn{"cost usd", func(r StatsRow) string {
			if !r.Usage.HasCost {
				return "-"
			}
			return fmt.Sprintf("%.2f", r.Usage.CostUSD)
		}})
	}
	return cols
}

// StatsHuman writes the stats table: the grouping key column (header = the
// --by mode), the numeric token columns, and — only when at least one
// aggregated session has a cost — the `cost usd` column (groups without cost
// show "-"). Numbers carry thousands separators; every column width comes
// from a pre-pass over header and rows. An empty selection prints nothing,
// matching the `sessions` behavior (M7 ruling 8).
func StatsHuman(w io.Writer, by string, rows []StatsRow) {
	if len(rows) == 0 {
		return
	}
	withCost := false
	for _, r := range rows {
		if r.Usage.HasCost {
			withCost = true
			break
		}
	}
	cols := statsColumns(withCost)

	keyW := len(by) // the header cell of the key column is the mode name
	for _, r := range rows {
		keyW = max(keyW, len(r.Key))
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c.header)
		for _, r := range rows {
			widths[i] = max(widths[i], len(c.cell(r)))
		}
	}

	nameColor.Fprintf(w, "%-*s", keyW, by)
	for i, c := range cols {
		fmt.Fprintf(w, "  %*s", widths[i], c.header)
	}
	fmt.Fprintln(w)
	for _, r := range rows {
		nameColor.Fprintf(w, "%-*s", keyW, r.Key)
		for i, c := range cols {
			fmt.Fprintf(w, "  %*s", widths[i], c.cell(r))
		}
		fmt.Fprintln(w)
	}
}

// StatsJSON writes the stats data as JSON with a stable schema: an array of
// row objects keyed in struct order; `cost_usd` is null when no session in
// the group provided a cost and a number (even 0) when any did. An empty
// selection prints `[]`.
func StatsJSON(w io.Writer, rows []StatsRow) error {
	return writeJSON(w, NewStatsRows(rows))
}

// groupDigits renders an integer with thousands separators (1,234,567) so
// large token counts stay readable.
func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	start := len(s) % 3
	if start == 0 {
		start = 3
	}
	var b strings.Builder
	b.WriteString(s[:start])
	for i := start; i < len(s); i += 3 {
		b.WriteString(",")
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
