package app

import (
	"fmt"
	"strings"

	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
	"github.com/wrinfotel/pastlog/internal/search"
)

// SearchOptions mirrors the CLI's search flags 1:1 (spec §4.2). MaxHits 0
// means unlimited, like `--max-hits 0`.
type SearchOptions struct {
	Filter        FilterOptions `json:"filter"`
	MaxHits       int           `json:"maxHits"`
	CaseSensitive bool          `json:"caseSensitive"`
	Regex         bool          `json:"regex"`
}

// entryHeadRunes bounds the viewer anchor: the first N runes of the hit
// entry's full text — enough to pin one entry without shipping multi-KB tool
// outputs per hit.
const entryHeadRunes = 120

// guiHit is a stable-schema hit plus the GUI-only viewer anchor (R-D11): the
// head of the hit entry's full text. The Line snippet is windowed (or, for
// multi-line matches, collapsed) and is therefore usually NOT a substring of
// the entry, so the snippet itself cannot anchor the scroll. CLI --json stays
// byte-identical: render.SearchHitJSON is untouched.
type guiHit struct {
	render.SearchHitJSON
	EntryHead string `json:"entry_head"`
}

// guiResult is one search-schema result row whose hits carry the anchor.
type guiResult struct {
	Session render.SessionJSON `json:"session"`
	Hits    []guiHit           `json:"hits"`
}

// SearchOutcome is the `pastlog search` surface for the GUI.
type SearchOutcome struct {
	Results   []guiResult `json:"results"`
	Hits      int         `json:"hits"`
	Truncated bool        `json:"truncated"` // the max-hits cap was reached
	Cancelled bool        `json:"cancelled"` // the user stopped the run
	Notes     []string    `json:"notes"`
}

// Search runs the streaming engine over the filtered sessions (FastListing
// per R-D10: cards never render fields living beyond record line 1) with
// progress events and cancellation. Long-op #1 of the GUI.
func (a *App) Search(query string, o SearchOptions) (SearchOutcome, error) {
	if strings.TrimSpace(query) == "" {
		return SearchOutcome{}, fmt.Errorf("empty query")
	}
	home, err := a.effectiveHome()
	if err != nil {
		return SearchOutcome{}, err
	}
	reg := cli.NewRegistry(home)
	if err := validateAgent(reg, o.Filter.Agent); err != nil {
		return SearchOutcome{}, err
	}
	filter, err := a.buildFilter(o.Filter)
	if err != nil {
		return SearchOutcome{}, err
	}
	m, err := search.NewMatcher(query, search.MatchOptions{CaseSensitive: o.CaseSensitive, Regex: o.Regex})
	if err != nil {
		return SearchOutcome{}, err
	}

	gen, searchHook, _ := a.begin("search")
	adapters := reg.Adapters()
	var notes []string
	results := search.Run(adapters, m, search.EngineOptions{
		Filter:      filter,
		Sessions:    o.Filter.Limit,
		MaxHits:     o.MaxHits,
		FastListing: true,
		Progress:    searchHook,
		Note:        func(note string) { notes = append(notes, note) },
	})
	hits := 0
	for _, r := range results {
		hits += len(r.Hits)
	}
	return SearchOutcome{
		Results:   guiResults(results),
		Hits:      hits,
		Truncated: o.MaxHits > 0 && hits >= o.MaxHits,
		Cancelled: !a.stillActive(gen),
		Notes:     a.combineNotes(adapters, notes),
	}, nil
}

// guiResults maps engine results to the GUI surface: the stable search-schema
// rows plus the per-hit viewer anchor.
func guiResults(results []search.Result) []guiResult {
	base := render.NewSearchResults(results)
	out := make([]guiResult, len(base))
	for i, b := range base {
		hits := make([]guiHit, len(b.Hits))
		for j, h := range b.Hits {
			hits[j] = guiHit{SearchHitJSON: h, EntryHead: entryHead(results[i].Hits[j].Entry.Text)}
		}
		out[i] = guiResult{Session: b.Session, Hits: hits}
	}
	return out
}

// entryHead returns the first entryHeadRunes runes of the entry text, rune
// safe (never splits a UTF-8 sequence).
func entryHead(text string) string {
	rs := []rune(text)
	if len(rs) > entryHeadRunes {
		rs = rs[:entryHeadRunes]
	}
	return string(rs)
}
