package search

import (
	"errors"
	"sort"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// DefaultMaxHits caps the total hits printed when --max-hits is not given
// (controller ruling).
const DefaultMaxHits = 200

// LineFilteredAdapter is optionally implemented by line-oriented adapters so
// the engine can apply the matcher's raw-line prefilter BEFORE any JSON
// parsing (spec §7 hot path). Adapters without it are searched via plain
// Entries.
type LineFilteredAdapter interface {
	EntriesFiltered(s agentlog.Session, keep func(rawLine []byte) bool, iter func(agentlog.Entry) error) error
}

// Hit is one matching entry with its rendered snippet: the matching line
// (highlightable between MatchStart and MatchEnd) plus one line of context.
type Hit struct {
	Entry      agentlog.Entry
	Context    string // first line of the previous scanned entry, truncated; "" if none. With the raw-line prefilter on, rejected lines are never parsed and cannot serve as context.
	Line       string // the line containing the match, windowed to snippetWidth
	MatchStart int    // byte offset of the match inside Line
	MatchEnd   int
}

// Result groups the hits of one session.
type Result struct {
	Session agentlog.SessionMeta
	Hits    []Hit
}

// EngineOptions bound one search run.
type EngineOptions struct {
	Filter   agentlog.SessionFilter
	Sessions int // max sessions scanned, 0 = unlimited
	MaxHits  int // total hits cap across all sessions, 0 = unlimited
}

var errStop = errors.New("pastlog/search: stop scan")

// scoped pairs a session with the adapter that owns it.
type scoped struct {
	meta    agentlog.SessionMeta
	adapter agentlog.Adapter
}

// Run streams every filtered session (newest first) through the matcher and
// returns one Result per session that produced hits, up to the caps: at most
// EngineOptions.Sessions sessions are scanned and at most EngineOptions.MaxHits
// hits are collected in total. Scan errors are ignored — search is best
// effort, like listing.
func Run(adapters []agentlog.Adapter, m *Matcher, o EngineOptions) []Result {
	scoped := enumerate(adapters, o.Filter)
	sort.SliceStable(scoped, func(i, j int) bool {
		if !scoped[i].meta.StartedAt.Equal(scoped[j].meta.StartedAt) {
			return scoped[i].meta.StartedAt.After(scoped[j].meta.StartedAt)
		}
		return scoped[i].meta.ID < scoped[j].meta.ID
	})
	if o.Sessions > 0 && len(scoped) > o.Sessions {
		scoped = scoped[:o.Sessions]
	}

	keep := m.KeepRaw()
	total := 0
	var results []Result
	for _, sc := range scoped {
		if o.MaxHits > 0 && total >= o.MaxHits {
			break
		}
		res := Result{Session: sc.meta}
		prev := ""
		iter := func(e agentlog.Entry) error {
			if e.Text == "" {
				return nil // nothing to match or show
			}
			if start, end, ok := m.Locate(e.Text); ok {
				sn := buildSnippet(m, prev, e.Text, start, end)
				res.Hits = append(res.Hits, Hit{
					Entry:      e,
					Context:    sn.context,
					Line:       sn.line,
					MatchStart: sn.matchStart,
					MatchEnd:   sn.matchEnd,
				})
				total++
				if o.MaxHits > 0 && total >= o.MaxHits {
					return errStop
				}
			}
			prev = e.Text
			return nil
		}
		// scan errors leave partial results behind: search is best effort
		_ = scanSession(sc.adapter, sc.meta.Session, keep, iter)
		if len(res.Hits) > 0 {
			results = append(results, res)
		}
	}
	return results
}

// enumerate streams session metadata from every adapter, applying the filter.
func enumerate(adapters []agentlog.Adapter, f agentlog.SessionFilter) []scoped {
	var out []scoped
	for _, a := range adapters {
		if f.Agent != "" && a.Name() != f.Agent {
			continue
		}
		add := func(sm agentlog.SessionMeta) error {
			sm.Agent = a.Name()
			if f.Match(sm.Session) {
				out = append(out, scoped{sm, a})
			}
			return nil
		}
		if ms, ok := a.(agentlog.MetaSource); ok {
			_ = ms.SessionsMeta(add)
		} else {
			_ = a.Sessions(func(s agentlog.Session) error {
				return add(agentlog.SessionMeta{Session: s})
			})
		}
	}
	return out
}

// scanSession scans one session's entries, through the prefilter when both
// the matcher and the adapter support it.
func scanSession(a agentlog.Adapter, s agentlog.Session, keep func([]byte) bool, iter func(agentlog.Entry) error) error {
	if keep != nil {
		if lfa, ok := a.(LineFilteredAdapter); ok {
			return lfa.EntriesFiltered(s, keep, iter)
		}
	}
	return a.Entries(s, iter)
}
