package agentlog

import (
	"sort"
	"strings"
	"time"
)

// SessionFilter selects sessions for listing.
type SessionFilter struct {
	Agent   string    // exact adapter name, "" = all agents
	Project string    // case-insensitive substring of the working dir, "" = all
	Since   time.Time // inclusive lower bound on StartedAt, zero = none
	Until   time.Time // inclusive upper bound on StartedAt, zero = none
	Limit   int       // max sessions returned, 0 = unlimited
}

// Match reports whether a session passes the filter.
func (f SessionFilter) Match(s Session) bool {
	if f.Agent != "" && s.Agent != f.Agent {
		return false
	}
	if f.Project != "" && !strings.Contains(strings.ToLower(s.Project), strings.ToLower(f.Project)) {
		return false
	}
	if !f.Since.IsZero() && s.StartedAt.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && s.StartedAt.After(f.Until) {
		return false
	}
	return true
}

// CollectSessions streams sessions from the adapters, applies the filter,
// sorts newest first (ties broken by ID for determinism) and applies the
// limit. Adapters implementing MetaSource provide message counts in one
// pass; the others are counted via a second streaming pass over Entries.
func CollectSessions(adapters []Adapter, f SessionFilter) []SessionMeta {
	var out []SessionMeta
	for _, a := range adapters {
		if f.Agent != "" && a.Name() != f.Agent {
			continue
		}
		collect := func(m SessionMeta) error {
			m.Agent = a.Name()
			if f.Match(m.Session) {
				out = append(out, m)
			}
			return nil
		}
		if ms, ok := a.(MetaSource); ok {
			_ = ms.SessionsMeta(collect) // scan errors leave partial results; listing is best effort
		} else {
			_ = a.Sessions(func(s Session) error {
				m := SessionMeta{Session: s}
				n := 0
				_ = a.Entries(s, func(e Entry) error {
					if e.Kind == Message {
						n++
					}
					return nil
				})
				m.Messages = n
				_ = collect(m)
				return nil
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.After(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

// TotalSkipped sums unreadable-line counts over adapters implementing
// SkipCounter.
func TotalSkipped(adapters []Adapter) int {
	n := 0
	for _, a := range adapters {
		if sc, ok := a.(SkipCounter); ok {
			n += sc.SkippedLines()
		}
	}
	return n
}
