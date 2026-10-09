package agentlog

import (
	"errors"
)

// CollectCtxBatches streams the normalized context-event batch (CtxSource)
// of every filter-passing session to consume, one session at a time, in
// adapter order — the streaming pass behind the cross-session optimize
// analysis (SPEC-optimize.md §1). Adapters without a CtxSource contribute
// nothing, silently (mirrors CollectUsage): an agent without the IR cannot
// feed the cross-session rules. A session whose stream fails to load notes
// at most once per adapter and is skipped — best effort, spec §8. The
// progress hooks (R-D6) tick after each delivered batch and may stop the
// scan; the partial selection stays delivered. Returns the number of
// delivered batches. The only error returned comes from consume, which
// aborts the scan.
func CollectCtxBatches(adapters []Adapter, f SessionFilter, note func(string), consume func(SessionMeta, []CtxEvent) error, progress ...Progress) (int, error) {
	delivered := 0
	for _, a := range adapters {
		if f.Agent != "" && a.Name() != f.Agent {
			continue
		}
		src, ok := a.(CtxSource)
		if !ok {
			continue // no context IR: contributes nothing, silently
		}
		noted := false
		noteOnce := func(err error) {
			if note != nil && !noted {
				noted = true
				note(UnreadableNote(a.Name(), err))
			}
		}
		stopped := false
		visit := func(s Session) error {
			if f.Match(s) {
				events, err := src.ContextEvents(s)
				if err != nil {
					noteOnce(err) // this session's stream is skipped, the scan continues
				} else {
					m := SessionMeta{Session: s}
					m.Agent = a.Name()
					if err := consume(m, events); err != nil {
						return err
					}
					delivered++
				}
			}
			for _, p := range progress {
				if !p(delivered) {
					stopped = true
					return errStopCollect
				}
			}
			return nil
		}
		var err error
		if ms, ok := a.(MetaSource); ok {
			err = ms.SessionsMeta(func(m SessionMeta) error {
				return visit(m.Session)
			})
		} else {
			err = a.Sessions(visit)
		}
		if err != nil && !errors.Is(err, errStopCollect) {
			return delivered, err
		}
		if stopped {
			break // cancelled (R-D6): keep the delivered batches
		}
	}
	return delivered, nil
}
