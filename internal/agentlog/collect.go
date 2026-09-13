package agentlog

import (
	"fmt"
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
	if f.Project != "" && !strings.Contains(normalizePath(s.Project), normalizePath(f.Project)) {
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

// normalizePath lowercases a path and folds backslashes to forward slashes,
// so `--project dev/myapp` matches a recorded cwd of `dev\myapp` on Windows
// (and vice versa). Matching stays a case-insensitive substring test.
func normalizePath(p string) string {
	return strings.ReplaceAll(strings.ToLower(p), `\`, "/")
}

// UnreadableNote renders the one-line stderr note for an adapter whose
// storage could not be read (spec §8 best effort): lowercase, one line,
// distinguishing "storage unreadable" from "no sessions". Callers keep the
// exit code unchanged — the note is informational.
func UnreadableNote(name string, err error) string {
	return fmt.Sprintf("%s: storage unreadable (%v) — its sessions are missing or partial in this run", name, err)
}

// CollectSessions streams sessions from the adapters, applies the filter,
// sorts newest first (ties broken by ID for determinism) and applies the
// limit. Adapters implementing MetaSource provide message counts in one
// pass; the others are counted via a second streaming pass over Entries.
//
// note (optional) receives at most one UnreadableNote line per adapter whose
// scan failed: listing stays best effort (spec §8) — partial results are
// still returned and the exit code is unchanged.
func CollectSessions(adapters []Adapter, f SessionFilter, note func(string)) []SessionMeta {
	var out []SessionMeta
	for _, a := range adapters {
		if f.Agent != "" && a.Name() != f.Agent {
			continue
		}
		noted := false
		noteOnce := func(err error) {
			if note != nil && !noted {
				noted = true
				note(UnreadableNote(a.Name(), err))
			}
		}
		if ms, ok := a.(MetaSource); ok {
			err := ms.SessionsMeta(func(m SessionMeta) error {
				m.Agent = a.Name()
				if f.Match(m.Session) {
					out = append(out, m)
				}
				return nil
			})
			if err != nil {
				noteOnce(err) // scan errors leave partial results; listing is best effort
			}
		} else {
			err := a.Sessions(func(s Session) error {
				m := SessionMeta{Session: s}
				n := 0
				if err := a.Entries(s, func(e Entry) error {
					if e.Kind == Message {
						n++
					}
					return nil
				}); err != nil {
					noteOnce(err)
				}
				m.Messages = n
				m.Agent = a.Name()
				if f.Match(m.Session) {
					out = append(out, m)
				}
				return nil
			})
			if err != nil {
				noteOnce(err)
			}
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
