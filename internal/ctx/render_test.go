package ctx

import (
	"reflect"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

func turn(seq int, in int64) agentlog.CtxEvent {
	return agentlog.CtxEvent{
		Kind: agentlog.CtxTurnStart, Seq: seq,
		Tokens: agentlog.CtxTokens{Input: in},
	}
}

func TestTurnCurveMarksCompactions(t *testing.T) {
	events := []agentlog.CtxEvent{
		turn(1, 100),
		{Kind: agentlog.CtxToolCall, Seq: 2},
		turn(3, 200),
		{Kind: agentlog.CtxCompact, Seq: 4},
		turn(5, 50),
		turn(6, 60),
	}
	turns, compactBefore := TurnCurve(events)
	if want := []int64{100, 200, 50, 60}; !reflect.DeepEqual(turns, want) {
		t.Errorf("turns = %v, want %v", turns, want)
	}
	if want := []bool{false, false, true, false}; !reflect.DeepEqual(compactBefore, want) {
		t.Errorf("compactBefore = %v, want %v", compactBefore, want)
	}
}

func TestTurnCurveEmpty(t *testing.T) {
	turns, compactBefore := TurnCurve(nil)
	if len(turns) != 0 || len(compactBefore) != 0 {
		t.Errorf("empty events: turns=%v compactBefore=%v, want empty", turns, compactBefore)
	}
}

func TestDownsampleCurvePoolsMaxAndKeepsGaps(t *testing.T) {
	turns := []int64{10, 30, 20, 5, 40, 15}
	before := []bool{false, false, false, true, false, false}
	got, gotBefore := DownsampleCurve(turns, before, 3)
	if want := []int64{30, 20, 40}; !reflect.DeepEqual(got, want) {
		t.Errorf("downsampled turns = %v, want %v", got, want)
	}
	// the gap at turn 4 lands inside bucket 2 (0-based) and must survive
	if want := []bool{false, true, false}; !reflect.DeepEqual(gotBefore, want) {
		t.Errorf("downsampled compactBefore = %v, want %v", gotBefore, want)
	}
}

func TestDownsampleCurvePassthrough(t *testing.T) {
	turns := []int64{1, 2, 3}
	before := []bool{false, true, false}
	got, gotBefore := DownsampleCurve(turns, before, 8)
	if !reflect.DeepEqual(got, turns) || !reflect.DeepEqual(gotBefore, before) {
		t.Errorf("short curve must pass through unchanged: %v %v", got, gotBefore)
	}
	same, sameBefore := DownsampleCurve(turns, before, 0)
	if !reflect.DeepEqual(same, turns) || !reflect.DeepEqual(sameBefore, before) {
		t.Errorf("non-positive bars must pass through unchanged: %v %v", same, sameBefore)
	}
}

func TestSparklineRendersCurve(t *testing.T) {
	s := Sparkline([]int64{100, 200, 50}, []bool{false, false, true})
	rs := []rune(s)
	if got, want := len(rs), 4; got != want { // 3 bars + one gap space
		t.Errorf("sparkline %q has %d runes, want %d", s, got, want)
	}
	if rs[0] == '█' {
		t.Errorf("first bar %q should not hit the peak glyph at value 100/200", s)
	}
}
