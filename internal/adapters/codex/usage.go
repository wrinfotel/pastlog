package codex

import "github.com/wrinfotel/pastlog/internal/agentlog"

func (u tokenUsage) asUsage() agentlog.Usage {
	return agentlog.Usage{
		Input:             u.input,
		Output:            u.output,
		CacheRead:         u.cached,
		Reasoning:         u.reasoning,
		ReasoningIncluded: u.reasoning,
	}
}

func (u tokenUsage) minus(previous tokenUsage) tokenUsage {
	return tokenUsage{
		input:     u.input - previous.input,
		cached:    u.cached - previous.cached,
		output:    u.output - previous.output,
		reasoning: u.reasoning - previous.reasoning,
	}
}

func (u tokenUsage) positive() tokenUsage {
	return tokenUsage{
		input: max(u.input, 0), cached: max(u.cached, 0),
		output: max(u.output, 0), reasoning: max(u.reasoning, 0),
	}
}

type usageSegment struct {
	model  string
	tokens tokenUsage
}

// cumulativeUsage attributes changes in one series. Repeated snapshots add
// nothing. If a counter goes backwards, remove the corrected amount from the
// latest allocations of that counter so the breakdown matches the snapshot.
type cumulativeUsage struct {
	total    tokenUsage
	segments []usageSegment
}

func (s *cumulativeUsage) observe(model string, fields usageFields) {
	previous := s.total
	fields.update(&s.total)
	delta := s.total.minus(previous)
	for i := len(s.segments) - 1; i >= 0 && delta != delta.positive(); i-- {
		t := &s.segments[i].tokens
		removeCorrection(&t.input, &delta.input)
		removeCorrection(&t.cached, &delta.cached)
		removeCorrection(&t.output, &delta.output)
		removeCorrection(&t.reasoning, &delta.reasoning)
	}
	if delta == (tokenUsage{}) {
		return
	}
	n := len(s.segments)
	if n > 0 && s.segments[n-1].model == model {
		t := &s.segments[n-1].tokens
		t.input += delta.input
		t.cached += delta.cached
		t.output += delta.output
		t.reasoning += delta.reasoning
	} else {
		s.segments = append(s.segments, usageSegment{model: model, tokens: delta})
	}
}

func removeCorrection(allocated, delta *int64) {
	remove := min(*allocated, max(-*delta, 0))
	*allocated -= remove
	*delta += remove
}

func (s *cumulativeUsage) modelSplit() agentlog.ModelSplit {
	var out agentlog.ModelSplit
	for _, segment := range s.segments {
		if segment.tokens != (tokenUsage{}) {
			out.Observe(segment.model, segment.tokens.asUsage())
		}
	}
	return out
}
