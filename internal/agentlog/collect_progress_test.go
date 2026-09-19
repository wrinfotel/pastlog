package agentlog

import "testing"

// Progress-hook tests for CollectUsage (R-D6): the desktop GUI streams stats
// progress and cancels long scans; the CLI passes no hook and sees nothing.

func TestCollectUsageProgressTicks(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "a"}}, usage: sampleUsage()}
	var ticks []int
	rows := CollectUsage([]Adapter{a}, SessionFilter{}, "", nil, func(done int) bool {
		ticks = append(ticks, done)
		return true
	})
	if len(rows) != len(sampleUsage()) {
		t.Fatalf("all rows must survive a passing hook, got %d", len(rows))
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

func TestCollectUsageProgressCancelKeepsPartial(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "a"}}, usage: sampleUsage()}
	rows := CollectUsage([]Adapter{a}, SessionFilter{}, "", nil, func(done int) bool { return done < 2 })
	if len(rows) != 2 {
		t.Errorf("cancel after the second session must keep 2 rows, got %d", len(rows))
	}
}

func TestCollectUsageCancelAlwaysKeepsFirst(t *testing.T) {
	// Post-scan semantics: the first tick happens after the first session,
	// so an always-cancelling hook still keeps that session's contribution.
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "a"}}, usage: sampleUsage()}
	rows := CollectUsage([]Adapter{a}, SessionFilter{}, "", nil, func(int) bool { return false })
	if len(rows) != 1 {
		t.Errorf("always-cancel must keep exactly 1 row, got %d", len(rows))
	}
}

func TestCollectUsageNotCalledWithoutUsage(t *testing.T) {
	empty := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "a"}}}
	called := false
	CollectUsage([]Adapter{empty}, SessionFilter{}, "", nil, func(int) bool { called = true; return true })
	if called {
		t.Error("the hook must not fire when the adapter yields no usage rows")
	}
}
