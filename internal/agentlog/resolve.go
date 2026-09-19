package agentlog

import (
	"fmt"
	"sort"
	"strings"
)

// Resolution is the structured outcome of resolving a session id or prefix
// (ruling R-D6). Exactly one of three states holds:
//
//   - Found(): Meta/Adapter carry the resolved session;
//   - Candidates is non-empty: the prefix is ambiguous — the GUI shows a
//     picker, the CLI composes its error text from this list;
//   - neither: no match — the error is "no session matches id prefix %q".
type Resolution struct {
	Meta       SessionMeta
	Adapter    Adapter
	Candidates []SessionMeta // ambiguous case; newest first, ties by ID (listing order)
	Unreadable []string      // "<name>: <err>" notes from adapters whose listing failed
}

// Found reports whether an exact or unambiguous session was resolved.
func (r Resolution) Found() bool { return r.Adapter != nil }

// ResolveSession finds a session by exact id or unambiguous prefix across
// all adapters. It is the structured core of `pastlog show` prefix
// resolution (extracted from the CLI so the desktop viewer inherits the same
// semantics, R-D6): listing order, the newest-first/ID-tiebreak sort, the
// exact-match pass before the prefix pass and the unreadable-storage notes
// all live here. Callers compose their own user-facing error text; the
// returned error (no match) is deliberately plain.
func ResolveSession(adapters []Adapter, idOrPrefix string) (Resolution, error) {
	type found struct {
		meta    SessionMeta
		adapter Adapter
	}
	res := Resolution{}
	var all []found
	for _, a := range adapters {
		add := func(m SessionMeta) {
			m.Agent = a.Name()
			all = append(all, found{m, a})
		}
		noted := false
		noteOnce := func(err error) {
			if !noted {
				noted = true
				res.Unreadable = append(res.Unreadable, a.Name()+": "+err.Error())
			}
		}
		if ms, ok := a.(MetaSource); ok {
			if err := ms.SessionsMeta(func(m SessionMeta) error {
				add(m)
				return nil
			}); err != nil {
				noteOnce(err)
			}
		} else {
			if err := a.Sessions(func(s Session) error {
				add(SessionMeta{Session: s})
				return nil
			}); err != nil {
				noteOnce(err)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].meta.StartedAt.Equal(all[j].meta.StartedAt) {
			return all[i].meta.StartedAt.After(all[j].meta.StartedAt)
		}
		return all[i].meta.ID < all[j].meta.ID
	})

	for _, f := range all {
		if f.meta.ID == idOrPrefix {
			res.Meta, res.Adapter = f.meta, f.adapter
			return res, nil
		}
	}
	var cands []found
	for _, f := range all {
		if strings.HasPrefix(f.meta.ID, idOrPrefix) {
			cands = append(cands, f)
		}
	}
	switch len(cands) {
	case 1:
		res.Meta, res.Adapter = cands[0].meta, cands[0].adapter
		return res, nil
	case 0:
		return res, fmt.Errorf("no session matches id prefix %q", idOrPrefix)
	default:
		for _, c := range cands {
			res.Candidates = append(res.Candidates, c.meta)
		}
		return res, nil
	}
}
