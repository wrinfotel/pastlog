package agentlog

import "testing"

// Progress-hook tests for CollectTimelineEvents (R-D6): the desktop GUI
// streams timeline progress and cancels long scans; the CLI passes no hook
// and sees nothing.

// timelineMetaFake builds a MetaSource adapter whose three sessions each
// carry one message entry.
func timelineMetaFake() *metaFake {
	a := &metaFake{fakeAdapter: fakeAdapter{name: "claude-code", entries: map[string][]Entry{}}}
	for _, id := range []string{"s1", "s2", "s3"} {
		s := Session{ID: id, Agent: "claude-code", StartedAt: t1}
		a.sessions = append(a.sessions, s)
		a.metas = append(a.metas, SessionMeta{Session: s})
		a.entries[id] = []Entry{{Kind: Message, Role: "user", Timestamp: t1, Text: "msg " + id}}
	}
	return a
}

func TestCollectTimelineEventsProgressTicks(t *testing.T) {
	a := timelineMetaFake()
	var ticks []int
	events := CollectTimelineEvents([]Adapter{a}, SessionFilter{}, 0, nil, func(done int) bool {
		ticks = append(ticks, done)
		return true
	})
	if len(events) != 3 {
		t.Fatalf("all events must survive a passing hook, got %d", len(events))
	}
	want := []int{1, 2, 3}
	if len(ticks) != len(want) {
		t.Fatalf("ticks = %v, want %v", ticks, want)
	}
	for i := range want {
		if ticks[i] != want[i] {
			t.Fatalf("ticks = %v, want %v", ticks, want)
		}
	}
}

func TestCollectTimelineEventsProgressCancelKeepsPartial(t *testing.T) {
	a := timelineMetaFake()
	events := CollectTimelineEvents([]Adapter{a}, SessionFilter{}, 0, nil, func(done int) bool { return done < 2 })
	if len(events) != 2 {
		t.Errorf("cancel after the second session must keep 2 events, got %d", len(events))
	}
	for _, e := range events {
		if e.SessionID == "s3" {
			t.Errorf("cancelled session s3 must not appear: %+v", events)
		}
	}
}

func TestCollectTimelineEventsCancelAlwaysKeepsFirst(t *testing.T) {
	// The non-MetaSource fallback ticks after each scanned session too, so an
	// always-cancelling hook keeps exactly the first session's contribution.
	a := timelineMetaFake().fakeAdapter
	events := CollectTimelineEvents([]Adapter{&a}, SessionFilter{}, 0, nil, func(int) bool { return false })
	if len(events) != 1 || events[0].SessionID != "s1" {
		t.Errorf("always-cancel must keep exactly the first session's event, got %+v", events)
	}
}

func TestCollectTimelineEventsCancelStopsLaterAdapters(t *testing.T) {
	a := timelineMetaFake()
	b := &metaFake{fakeAdapter: fakeAdapter{name: "codex", entries: map[string][]Entry{
		"z1": {{Kind: Message, Role: "user", Timestamp: t2, Text: "codex msg"}},
	}, sessions: []Session{{ID: "z1", Agent: "codex", StartedAt: t2}}}}
	events := CollectTimelineEvents([]Adapter{a, b}, SessionFilter{}, 0, nil, func(done int) bool { return done < 1 })
	for _, e := range events {
		if e.Agent == "codex" {
			t.Errorf("cancelled scan must not reach later adapters: %+v", events)
		}
	}
}
