package agentlog

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// TimelineEvent is one message-kind entry of the merged cross-session
// timeline (0.2.3). The timeline is a navigation view, so Text carries a
// first-line snippet, not the full message — `show` prints the whole entry.
type TimelineEvent struct {
	Timestamp time.Time // entry time; zero when the record carries none
	Agent     string
	Role      string
	Text      string // first-line snippet
	SessionID string
}

// snippetLen caps a timeline snippet: long enough to recognize a message,
// short enough that ten thousand events stay readable and cheap.
const snippetLen = 120

// CollectTimelineEvents streams the message-kind entries of every session
// that passes the filter, merges them chronologically (ties: agent, session
// id, then stream order within a session) and keeps the newest maxRows when
// maxRows > 0. Unreadable storage degrades per adapter with one note, like
// CollectSessions. The optional progress hooks (R-D6) report the running
// session count and may stop the scan, returning the partial selection —
// the CLI passes no hook and sees no behavior change.
func CollectTimelineEvents(adapters []Adapter, f SessionFilter, maxRows int, note func(string), progress ...Progress) []TimelineEvent {
	var out []TimelineEvent
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
		done := 0
		stopped := false
		collect := func(s Session) error {
			if f.Match(s) {
				if err := a.Entries(s, func(e Entry) error {
					if e.Kind != Message {
						return nil
					}
					out = append(out, TimelineEvent{
						Timestamp: e.Timestamp,
						Agent:     a.Name(),
						Role:      e.Role,
						Text:      snippet(e.Text),
						SessionID: s.ID,
					})
					return nil
				}); err != nil {
					return err
				}
			}
			done++
			for _, p := range progress {
				if !p(done) {
					stopped = true
					return errStopCollect
				}
			}
			return nil
		}
		var err error
		if ms, ok := a.(MetaSource); ok {
			err = ms.SessionsMeta(func(m SessionMeta) error {
				return collect(m.Session)
			})
		} else {
			err = a.Sessions(collect)
		}
		if err != nil && !errors.Is(err, errStopCollect) {
			noteOnce(err)
		}
		if stopped {
			break // cancelled (R-D6): return the partial selection
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].Timestamp.Before(out[j].Timestamp)
		}
		if out[i].Agent != out[j].Agent {
			return out[i].Agent < out[j].Agent
		}
		return out[i].SessionID < out[j].SessionID
	})
	if maxRows > 0 && len(out) > maxRows {
		out = out[len(out)-maxRows:]
	}
	return out
}

// snippet reduces a message to its first line capped at snippetLen runes,
// marking the cut with an ellipsis.
func snippet(text string) string {
	s := strings.ReplaceAll(text, "\r\n", "\n")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) <= snippetLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:snippetLen]) + "…"
}
