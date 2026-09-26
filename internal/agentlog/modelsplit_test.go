package agentlog

import "testing"

// TestModelSplit pins the per-model accumulator behind the Models breakdown
// (TASK.md backlog): usage folds under the model in effect — the last
// non-empty model — so a record without a model stays with the model that
// was running, the split always sums to the session total, entries come back
// in first-use order, and nothing observed yields nil.
func TestModelSplit(t *testing.T) {
	var s ModelSplit
	if got := s.Split(); got != nil {
		t.Errorf("empty split = %+v, want nil", got)
	}
	if s.Latest() != "" {
		t.Errorf("empty Latest = %q, want \"\"", s.Latest())
	}

	// the second record carries no model: its usage stays with model-a; the
	// last observation is model-a again — folding into the same entry, still
	// in first position, and itself the latest non-empty model
	s.Observe("model-a", Usage{Input: 120, Output: 45, CacheWrite: 30, CacheRead: 200})
	s.Observe("", Usage{Input: 80, CacheRead: 230})
	s.Observe("model-b", Usage{Input: 60, Output: 55, CacheWrite: 10, CacheRead: 300})
	s.Observe("model-a", Usage{Input: 1, HasCost: true, CostUSD: 0.5})

	want := []Usage{
		{Model: "model-a", Input: 201, Output: 45, CacheWrite: 30, CacheRead: 430, CostUSD: 0.5, HasCost: true},
		{Model: "model-b", Input: 60, Output: 55, CacheWrite: 10, CacheRead: 300},
	}
	got := s.Split()
	if len(got) != len(want) {
		t.Fatalf("split = %d entries, want %d (first-use order)", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("split[%d] = %+v, want %+v", i, got[i], w)
		}
	}
	total := s.Total()
	if total.Input != 261 || total.Output != 100 || total.CacheRead != 730 ||
		total.CacheWrite != 40 || total.CostUSD != 0.5 || !total.HasCost {
		t.Errorf("total = %+v, want the sum of every observation", total)
	}
	if s.Latest() != "model-a" {
		t.Errorf("Latest = %q, want model-a (the last non-empty observation)", s.Latest())
	}
}

// TestModelSplitUnattributedUsage pins the no-model-yet case: usage observed
// before any model folds under "" (the model view shows it as "-"), so the
// split still sums to the total.
func TestModelSplitUnattributedUsage(t *testing.T) {
	var s ModelSplit
	s.Observe("", Usage{Input: 9})
	s.Observe("model-a", Usage{Input: 11})
	got := s.Split()
	if len(got) != 2 || got[0].Model != "" || got[0].Input != 9 {
		t.Errorf("split = %+v, want the \"\" entry first with input 9", got)
	}
	if s.Total().Input != 20 {
		t.Errorf("total = %d, want 20 (unattributed usage is not lost)", s.Total().Input)
	}
}

// TestModelSplitNote pins the model-only record: Note names the model in
// effect without adding usage — later unattributed usage lands there, and a
// model that never consumed anything stays out of the split.
func TestModelSplitNote(t *testing.T) {
	var s ModelSplit
	s.Note("model-a")
	if got := s.Split(); got != nil {
		t.Errorf("a noted-but-unused model must stay out of the split, got %+v", got)
	}
	s.Observe("", Usage{Input: 7})
	got := s.Split()
	if len(got) != 1 || got[0].Model != "model-a" || got[0].Input != 7 {
		t.Errorf("split = %+v, want model-a with the unattributed usage", got)
	}
}
