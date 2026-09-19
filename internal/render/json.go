// Exported JSON shapes of the stable --json schemas (ruling R-D4): the
// render functions below and the desktop services share these exact types,
// so GUI output and CLI --json output stay byte-identical by construction.
// Tags are part of the public contract — never rename or reorder them
// without a schema note in the CHANGELOG.
package render

import (
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/search"
)

// AgentJSON is one `agents` row: path is null when the storage was not found.
type AgentJSON struct {
	Name     string  `json:"name"`
	Detected bool    `json:"detected"`
	Path     *string `json:"path"`
	Sessions int     `json:"sessions"`
	Bytes    int64   `json:"bytes"`
}

// AgentRowsJSON maps agents rows to their stable JSON shape.
func AgentRowsJSON(rows []AgentRow) []AgentJSON {
	out := make([]AgentJSON, len(rows))
	for i, r := range rows {
		row := AgentJSON{Name: r.Name, Detected: r.Detected, Sessions: r.Sessions, Bytes: r.Bytes}
		if r.Detected {
			row.Path = &r.Path
		}
		out[i] = row
	}
	return out
}

// SessionJSON is one session of the sessions/search/show schemas; absent
// timestamps render as null.
type SessionJSON struct {
	ID        string     `json:"id"`
	Agent     string     `json:"agent"`
	Project   string     `json:"project"`
	Title     string     `json:"title"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Messages  int        `json:"messages"`
	SizeBytes int64      `json:"size_bytes"`
}

// NewSessionJSON maps a SessionMeta to its stable JSON shape; shared by the
// sessions list, search results and show.
func NewSessionJSON(m agentlog.SessionMeta) SessionJSON {
	return SessionJSON{
		ID:        m.ID,
		Agent:     m.Agent,
		Project:   m.Project,
		Title:     m.Title,
		StartedAt: timePtr(m.StartedAt),
		EndedAt:   timePtr(m.EndedAt),
		Messages:  m.Messages,
		SizeBytes: m.SizeBytes,
	}
}

// SessionRowsJSON maps a session listing to its stable JSON shape.
func SessionRowsJSON(rows []agentlog.SessionMeta) []SessionJSON {
	out := make([]SessionJSON, len(rows))
	for i, r := range rows {
		out[i] = NewSessionJSON(r)
	}
	return out
}

// EntryJSON is one transcript entry of the show schema.
type EntryJSON struct {
	Kind      string     `json:"kind"`
	Role      string     `json:"role"`
	Text      string     `json:"text"`
	Timestamp *time.Time `json:"timestamp"`
}

// ShowDoc is the show schema: the session keys keep their stable order and
// "entries" comes last.
type ShowDoc struct {
	SessionJSON
	Entries []EntryJSON `json:"entries"`
}

// NewShowDoc maps one session with its entries to the show schema.
func NewShowDoc(meta agentlog.SessionMeta, entries []agentlog.Entry) ShowDoc {
	out := ShowDoc{SessionJSON: NewSessionJSON(meta), Entries: make([]EntryJSON, 0, len(entries))}
	for _, e := range entries {
		out.Entries = append(out.Entries, EntryJSON{
			Kind:      kindJSON(e.Kind),
			Role:      e.Role,
			Text:      e.Text,
			Timestamp: timePtr(e.Timestamp),
		})
	}
	return out
}

// SearchHitJSON is one hit of the search schema. Match offsets are rune
// offsets into Line, so they survive encoding and stay readable for non-Go
// consumers.
type SearchHitJSON struct {
	Kind       string     `json:"kind"`
	Role       string     `json:"role"`
	Timestamp  *time.Time `json:"timestamp"`
	Context    string     `json:"context"`
	Line       string     `json:"line"`
	MatchStart int        `json:"match_start"` // rune offset into line
	MatchEnd   int        `json:"match_end"`   // rune offset into line
}

// SearchResultJSON is one session with its hits of the search schema.
type SearchResultJSON struct {
	Session SessionJSON     `json:"session"`
	Hits    []SearchHitJSON `json:"hits"`
}

// NewSearchResults maps engine results to the stable search schema.
func NewSearchResults(results []search.Result) []SearchResultJSON {
	out := make([]SearchResultJSON, 0, len(results))
	for _, r := range results {
		row := SearchResultJSON{Session: NewSessionJSON(r.Session), Hits: make([]SearchHitJSON, 0, len(r.Hits))}
		for _, h := range r.Hits {
			row.Hits = append(row.Hits, SearchHitJSON{
				Kind:       kindJSON(h.Entry.Kind),
				Role:       h.Entry.Role,
				Timestamp:  timePtr(h.Entry.Timestamp),
				Context:    h.Context,
				Line:       h.Line,
				MatchStart: runeOffset(h.Line, h.MatchStart),
				MatchEnd:   runeOffset(h.Line, h.MatchEnd),
			})
		}
		out = append(out, row)
	}
	return out
}

// StatsTokens is the stable token block of the stats JSON schema.
type StatsTokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	Reasoning  int64 `json:"reasoning"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Total      int64 `json:"total"` // input+output+reasoning (M7 ruling 3)
}

// StatsRowJSON is one aggregated group of the stats schema; CostUSD is null
// when no session in the group provided a cost and a number (even 0) when
// any did.
type StatsRowJSON struct {
	Key      string      `json:"key"`
	Sessions int         `json:"sessions"`
	Messages int         `json:"messages"`
	Tokens   StatsTokens `json:"tokens"`
	CostUSD  *float64    `json:"cost_usd"`
}

// NewStatsRows maps aggregated stats rows to the stable stats schema.
func NewStatsRows(rows []StatsRow) []StatsRowJSON {
	out := make([]StatsRowJSON, len(rows))
	for i, r := range rows {
		var cost *float64
		if r.Usage.HasCost {
			c := r.Usage.CostUSD
			cost = &c
		}
		out[i] = StatsRowJSON{
			Key:      r.Key,
			Sessions: r.Sessions,
			Messages: r.Messages,
			Tokens: StatsTokens{
				Input:      r.Usage.Input,
				Output:     r.Usage.Output,
				Reasoning:  r.Usage.Reasoning,
				CacheRead:  r.Usage.CacheRead,
				CacheWrite: r.Usage.CacheWrite,
				Total:      r.Usage.Input + r.Usage.Output + r.Usage.Reasoning,
			},
			CostUSD: cost,
		}
	}
	return out
}
