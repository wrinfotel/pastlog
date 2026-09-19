package search

import (
	"errors"
	"sort"

	"github.com/wrinfotel/pastlog/internal/agentlog"
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
	// FastListing lets adapters implementing agentlog.FastMetaSource list
	// sessions from cheap sources (first record line + stat) instead of a
	// full parse — spec §7: the search flow must not spend most of its time
	// listing. Ids, projects, start timestamps, filters and sort order stay
	// identical to the full listing; fields beyond the first record line
	// (last timestamps, message counts) may be zero, so callers that render
	// them (the --json machine output) must leave this false. Default false.
	FastListing bool
	// Note (optional) receives at most one lowercase line per adapter whose
	// storage could not be read, whether while listing sessions or while
	// scanning one (spec §8): search stays best effort — partial results are
	// still returned and the exit code is unchanged.
	Note func(string)
	// Progress (optional) reports scan progress for long-running flows (the
	// desktop GUI, ruling R-D6): called once per completed session with
	// scanned = sessions processed so far and hits = total hits so far.
	// Returning false stops the scan before the next session; the partial
	// results collected so far are returned, mirroring the best-effort notes
	// path. Sessions skipped by the Sessions cap or the MaxHits break never
	// trigger the hook. The CLI leaves it nil and sees no behavior change.
	Progress func(scanned, hits int) bool
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
	scoped := enumerate(adapters, o.Filter, o.Note, o.FastListing)
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
	scanned := 0
	notedAdapters := map[string]bool{}
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
				sn := buildSnippet(prev, e.Text, start, end)
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
		// scan errors leave partial results behind: search is best effort,
		// but the user learns about it (one note per failing adapter). The
		// errStop sentinel is not a failure: it is how the iter signals that
		// the total-hits cap is reached (M6 finding — a capped scan must not
		// be reported as unreadable storage).
		if err := scanSession(sc.adapter, sc.meta.Session, keep, iter); err != nil && !errors.Is(err, errStop) {
			if o.Note != nil && !notedAdapters[sc.adapter.Name()] {
				notedAdapters[sc.adapter.Name()] = true
				o.Note(agentlog.UnreadableNote(sc.adapter.Name(), err))
			}
		}
		if len(res.Hits) > 0 {
			results = append(results, res)
		}
		// Progress fires after the completed session (R-D6): the tick counts
		// what has been scanned, and a false return stops before the next one.
		scanned++
		if o.Progress != nil && !o.Progress(scanned, total) {
			break // cancelled: partial results, no error
		}
	}
	return results
}

// enumerate streams session metadata from every adapter, applying the filter.
// A failing adapter yields whatever it managed to stream (best effort) and,
// when note is non-nil, one UnreadableNote line on the first failure. With
// fast set, adapters implementing agentlog.FastMetaSource list from their
// cheap pass first; used=false falls back to the full listing.
func enumerate(adapters []agentlog.Adapter, f agentlog.SessionFilter, note func(string), fast bool) []scoped {
	var out []scoped
	for _, a := range adapters {
		if f.Agent != "" && a.Name() != f.Agent {
			continue
		}
		noted := false
		noteOnce := func(err error) {
			if note != nil && !noted {
				noted = true
				note(agentlog.UnreadableNote(a.Name(), err))
			}
		}
		add := func(sm agentlog.SessionMeta) {
			sm.Agent = a.Name()
			if f.Match(sm.Session) {
				out = append(out, scoped{sm, a})
			}
		}
		if fast {
			if fms, ok := a.(agentlog.FastMetaSource); ok {
				used, err := fms.SessionsMetaFast(func(sm agentlog.SessionMeta) error {
					add(sm)
					return nil
				})
				if err != nil {
					noteOnce(err)
				}
				if used {
					continue
				}
			}
		}
		if ms, ok := a.(agentlog.MetaSource); ok {
			if err := ms.SessionsMeta(func(sm agentlog.SessionMeta) error {
				add(sm)
				return nil
			}); err != nil {
				noteOnce(err)
			}
		} else {
			if err := a.Sessions(func(s agentlog.Session) error {
				add(agentlog.SessionMeta{Session: s})
				return nil
			}); err != nil {
				noteOnce(err)
			}
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
