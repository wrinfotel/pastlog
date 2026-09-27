package agentlog

import "sort"

// RelatedSessions is the relatedness graph of one session (0.2.3): its
// subagent parent, its subagent children, and the adjacent sessions of the
// same agent and project — the nearest by start time, two per side, the
// honest continuation heuristic for agents without explicit links.
type RelatedSessions struct {
	Meta       SessionMeta  // the anchor
	Parent     *SessionMeta // set when the session is a subagent child
	Children   []SessionMeta
	Adjacent   []SessionMeta // same agent+project neighbors, chronological
	Unreadable []string      // "<name>: <err>" notes from adapters whose listing failed
}

// Related builds the relatedness graph of an already-resolved session. The
// anchor itself, its parent and its children never appear in Adjacent — each
// relation shows in exactly one section. Sessions without a project have no
// adjacency (nothing to relate on); ids are the only keys.
func Related(adapters []Adapter, meta SessionMeta) RelatedSessions {
	rel := RelatedSessions{Meta: meta}
	skip := map[string]bool{meta.ID: true}
	var neighbors []SessionMeta

	for _, a := range adapters {
		noted := false
		noteOnce := func(err error) {
			if !noted {
				noted = true
				rel.Unreadable = append(rel.Unreadable, a.Name()+": "+err.Error())
			}
		}
		consider := func(m SessionMeta) {
			if m.ID == meta.ID {
				return // the anchor itself
			}
			m.Agent = a.Name()
			switch {
			case m.ID == meta.ParentID && meta.ParentID != "":
				p := m
				rel.Parent = &p
			case m.ParentID == meta.ID:
				rel.Children = append(rel.Children, m)
				skip[m.ID] = true
			case m.Agent == meta.Agent && m.Project != "" && m.Project == meta.Project:
				neighbors = append(neighbors, m)
			}
		}
		if ms, ok := a.(MetaSource); ok {
			if err := ms.SessionsMeta(func(m SessionMeta) error {
				consider(m)
				return nil
			}); err != nil {
				noteOnce(err)
			}
		} else {
			if err := a.Sessions(func(s Session) error {
				consider(SessionMeta{Session: s})
				return nil
			}); err != nil {
				noteOnce(err)
			}
		}
	}
	if rel.Parent != nil {
		skip[rel.Parent.ID] = true
	}

	// neighbors in (start, id) order; the anchor sits at the search position
	sort.SliceStable(neighbors, func(i, j int) bool {
		if !neighbors[i].StartedAt.Equal(neighbors[j].StartedAt) {
			return neighbors[i].StartedAt.Before(neighbors[j].StartedAt)
		}
		return neighbors[i].ID < neighbors[j].ID
	})
	pos := sort.Search(len(neighbors), func(i int) bool {
		return !neighbors[i].StartedAt.Before(meta.StartedAt)
	})

	before := adjacentSide(neighbors, pos-1, -1, skip, 2)
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}
	rel.Adjacent = append(before, adjacentSide(neighbors, pos, +1, skip, 2)...)
	return rel
}

// adjacentSide walks neighbors from index i in direction step (+1/-1),
// collecting up to n sessions whose ids are not in skip.
func adjacentSide(neighbors []SessionMeta, i, step int, skip map[string]bool, n int) []SessionMeta {
	var out []SessionMeta
	for i >= 0 && i < len(neighbors) && len(out) < n {
		if !skip[neighbors[i].ID] {
			out = append(out, neighbors[i])
		}
		i += step
	}
	return out
}
