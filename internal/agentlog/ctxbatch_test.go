package agentlog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Fixtures for the optimize streaming pass: a fake adapter that also emits
// the context IR for selected sessions.

type ctxFakeAdapter struct {
	metaFake
	events  map[string][]CtxEvent
	loadErr map[string]error // per-session ContextEvents failure
}

func (f *ctxFakeAdapter) ContextEvents(s Session) ([]CtxEvent, error) {
	if err, ok := f.loadErr[s.ID]; ok {
		return nil, err
	}
	return f.events[s.ID], nil
}

func ctxBatchFake(ids ...string) *ctxFakeAdapter {
	f := &ctxFakeAdapter{
		metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}},
		events:   map[string][]CtxEvent{},
		loadErr:  map[string]error{},
	}
	for _, id := range ids {
		f.metas = append(f.metas, SessionMeta{Session: Session{ID: id, StartedAt: t1}})
		f.events[id] = []CtxEvent{{Seq: 1, Kind: CtxToolCall, Tool: "Read", ArgsKey: "Read\x00" + id, Label: id}}
	}
	return f
}

func TestCollectCtxBatchesDeliversInOrder(t *testing.T) {
	a := ctxBatchFake("s1", "s2", "s3")
	var got []string
	n, err := CollectCtxBatches([]Adapter{a}, SessionFilter{}, nil, func(m SessionMeta, ev []CtxEvent) error {
		got = append(got, m.ID)
		if len(ev) != 1 {
			t.Errorf("session %s: %d events, want 1", m.ID, len(ev))
		}
		if m.Agent != "claude-code" {
			t.Errorf("session %s: agent %q", m.ID, m.Agent)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CollectCtxBatches: %v", err)
	}
	if n != 3 || fmt.Sprint(got) != "[s1 s2 s3]" {
		t.Errorf("delivered %d %v, want 3 [s1 s2 s3]", n, got)
	}
}

func TestCollectCtxBatchesSkipsNonCtxSourceSilently(t *testing.T) {
	a := &fakeAdapter{name: "plain", sessions: []Session{{ID: "x", StartedAt: t1}}}
	noted := false
	n, err := CollectCtxBatches([]Adapter{a}, SessionFilter{}, func(string) { noted = true }, func(SessionMeta, []CtxEvent) error {
		t.Fatal("no batch may be delivered from a non-CtxSource adapter")
		return nil
	})
	if err != nil || n != 0 {
		t.Errorf("n=%d err=%v, want 0 nil", n, err)
	}
	if noted {
		t.Error("an adapter without the IR must stay silent, like CollectUsage")
	}
}

func TestCollectCtxBatchesSessionLoadErrorNotesOnceAndContinues(t *testing.T) {
	a := ctxBatchFake("s1", "s2", "s3")
	a.loadErr["s2"] = errors.New("truncated line")
	var notes []string
	var got []string
	n, err := CollectCtxBatches([]Adapter{a}, SessionFilter{}, func(s string) { notes = append(notes, s) }, func(m SessionMeta, _ []CtxEvent) error {
		got = append(got, m.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("a per-session load error must not abort the scan: %v", err)
	}
	if n != 2 || fmt.Sprint(got) != "[s1 s3]" {
		t.Errorf("delivered %d %v, want s1 and s3", n, got)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "storage unreadable") {
		t.Errorf("notes = %v, want one unreadable-storage note", notes)
	}
}

func TestCollectCtxBatchesProgressTicksAndCancel(t *testing.T) {
	a := ctxBatchFake("s1", "s2", "s3")
	var ticks []int
	n, _ := CollectCtxBatches([]Adapter{a}, SessionFilter{}, nil, func(SessionMeta, []CtxEvent) error { return nil },
		func(done int) bool { ticks = append(ticks, done); return true })
	if n != 3 || fmt.Sprint(ticks) != "[1 2 3]" {
		t.Errorf("ticks %v / delivered %d, want [1 2 3] / 3", ticks, n)
	}

	a2 := ctxBatchFake("s1", "s2", "s3")
	n2, _ := CollectCtxBatches([]Adapter{a2}, SessionFilter{}, nil, func(SessionMeta, []CtxEvent) error { return nil },
		func(done int) bool { return done < 2 })
	if n2 != 2 {
		t.Errorf("cancel after the second tick must keep 2 batches, got %d", n2)
	}
}

func TestCollectCtxBatchesConsumeErrorAborts(t *testing.T) {
	a := ctxBatchFake("s1", "s2")
	boom := errors.New("boom")
	n, err := CollectCtxBatches([]Adapter{a}, SessionFilter{}, nil, func(SessionMeta, []CtxEvent) error { return boom })
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	if n != 0 {
		t.Errorf("the aborted batch must not count as delivered, got %d", n)
	}
}
