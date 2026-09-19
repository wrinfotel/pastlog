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

// SearchOutcome is the `pastlog search` surface for the GUI.
type SearchOutcome struct {
	Results   []render.SearchResultJSON `json:"results"`
	Hits      int                       `json:"hits"`
	Truncated bool                      `json:"truncated"` // the max-hits cap was reached
	Cancelled bool                      `json:"cancelled"` // the user stopped the run
	Notes     []string                  `json:"notes"`
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
		Results:   render.NewSearchResults(results),
		Hits:      hits,
		Truncated: o.MaxHits > 0 && hits >= o.MaxHits,
		Cancelled: !a.stillActive(gen),
		Notes:     a.combineNotes(adapters, notes),
	}, nil
}
