package agentlog

// ModelSplit accumulates usage per model in stream order — the per-model
// breakdown behind the Models field of SessionUsage and the model view of
// stats (TASK.md backlog: tokens from mid-session model switches must land
// on the model that actually consumed them, not on the session's latest
// model). The zero value is ready to use; Observe is safe on a value inside
// a larger struct accessed through a pointer.
type ModelSplit struct {
	total    Usage
	inEffect string
	order    []string
	byModel  map[string]Usage
}

// Observe folds one request's usage into its model. An empty model keeps the
// model in effect (the last non-empty one seen, "" before any) — a record
// that does not name its model belongs to the turn that was running — so the
// split always sums to Total. First sight puts a model into first-use order.
func (s *ModelSplit) Observe(model string, u Usage) {
	if model != "" {
		s.inEffect = model
	} else {
		model = s.inEffect
	}
	s.total.Input += u.Input
	s.total.Output += u.Output
	s.total.Reasoning += u.Reasoning
	s.total.CacheRead += u.CacheRead
	s.total.CacheWrite += u.CacheWrite
	if u.HasCost {
		s.total.HasCost = true
		s.total.CostUSD += u.CostUSD
	}
	if s.byModel == nil {
		s.byModel = map[string]Usage{}
	}
	if _, seen := s.byModel[model]; !seen {
		s.order = append(s.order, model)
	}
	cur := s.byModel[model]
	cur.Input += u.Input
	cur.Output += u.Output
	cur.Reasoning += u.Reasoning
	cur.CacheRead += u.CacheRead
	cur.CacheWrite += u.CacheWrite
	if u.HasCost {
		cur.HasCost = true
		cur.CostUSD += u.CostUSD
	}
	s.byModel[model] = cur
}

// Note records the model in effect without any usage — a record that names
// its model but carries no usage (codex turn_context, a usage-less message)
// keeps later unattributed usage on track. A model that never consumes
// anything stays out of Split.
func (s *ModelSplit) Note(model string) {
	if model != "" {
		s.inEffect = model
	}
}

// Total returns the usage summed over every observation, with Model unset —
// the session model is Latest.
func (s ModelSplit) Total() Usage { return s.total }

// Latest returns the last non-empty model observed, "" when none was.
func (s ModelSplit) Latest() string { return s.inEffect }

// Split renders one Usage entry per model in first-use order, nil when
// nothing was observed (adapters that cannot split simply never Observe).
func (s ModelSplit) Split() []Usage {
	if len(s.order) == 0 {
		return nil
	}
	out := make([]Usage, 0, len(s.order))
	for _, model := range s.order {
		u := s.byModel[model]
		u.Model = model
		out = append(out, u)
	}
	return out
}
