package render

import (
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// GroupStatsKeys is the raw-key accumulator behind GroupStats: the desktop
// Projects view groups by the raw project path (its drill-down key must be
// the stored path, not the tilde-shortened display form). Same accumulation
// and ascending key order as GroupStats.
func TestGroupStatsKeys(t *testing.T) {
	rows := []agentlog.SessionUsage{
		{Session: agentlog.Session{Project: "/w/b"}, Messages: 2, Usage: agentlog.Usage{Input: 7, CacheRead: 3}},
		{Session: agentlog.Session{Project: "/w/a"}, Messages: 1, Usage: agentlog.Usage{Input: 10, Output: 2, HasCost: true, CostUSD: 1.5}},
		{Session: agentlog.Session{Project: "/w/a"}, Usage: agentlog.Usage{Input: 5, HasCost: true, CostUSD: 0.25}},
	}
	got := GroupStatsKeys(func(su agentlog.SessionUsage) string { return su.Project }, rows)
	if len(got) != 2 || got[0].Key != "/w/a" || got[1].Key != "/w/b" {
		t.Fatalf("keys = %+v, want ascending /w/a then /w/b", got)
	}
	a := got[0]
	if a.Sessions != 2 || a.Messages != 1 {
		t.Errorf("/w/a sessions/messages = %d/%d, want 2/1", a.Sessions, a.Messages)
	}
	if a.Usage.Input != 15 || a.Usage.Output != 2 || a.Usage.CacheRead != 0 ||
		a.Usage.CostUSD != 1.75 || !a.Usage.HasCost {
		t.Errorf("/w/a usage = %+v, want input 15, output 2, cost 1.75 with cost", a.Usage)
	}
	b := got[1]
	if b.Sessions != 1 || b.Messages != 2 || b.Usage.Input != 7 || b.Usage.CacheRead != 3 || b.Usage.HasCost {
		t.Errorf("/w/b row = %+v, want 1 session, 2 messages, input 7, cache read 3, no cost", b)
	}
	if empty := GroupStatsKeys(func(agentlog.SessionUsage) string { return "x" }, nil); len(empty) != 0 {
		t.Errorf("empty rows must give no groups, got %+v", empty)
	}
}
