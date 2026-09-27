package agentlog

import "errors"

// errStopSessionUsage aborts the usage stream once the wanted session was
// seen — it never reaches the caller.
var errStopSessionUsage = errors.New("pastlog/agentlog: session usage matched")

// SessionModelUsage returns one session's per-model token breakdown: the
// owning adapter's usage view is streamed once and matched by exact id.
// Adapters without a per-model split (session-level aggregates, like
// opencode) fall back to a single row attributed to the session's model when
// the session consumed anything; ok=false when the adapter yields no usage
// row for the id.
func SessionModelUsage(a Adapter, id string) ([]Usage, bool) {
	us, ok := a.(UsageSource)
	if !ok {
		return nil, false
	}
	var rows []Usage
	found := false
	err := us.SessionsUsage(func(su SessionUsage) error {
		if su.ID != id {
			return nil
		}
		found = true
		rows = modelRows(su)
		return errStopSessionUsage
	})
	if err != nil && !errors.Is(err, errStopSessionUsage) {
		return nil, false // storage unreadable before the match: no data
	}
	return rows, found
}

// modelRows shapes one session's usage into per-model rows: the split when
// the adapter keeps one, otherwise a single row attributed to the session's
// model — honest to what the storage records. A zero-usage session yields no
// rows.
func modelRows(su SessionUsage) []Usage {
	if len(su.Models) > 0 {
		return su.Models
	}
	u := su.Usage
	if u.Input == 0 && u.Output == 0 && u.Reasoning == 0 && u.CacheRead == 0 && u.CacheWrite == 0 {
		return nil
	}
	u.Model = su.Model
	return []Usage{u}
}
