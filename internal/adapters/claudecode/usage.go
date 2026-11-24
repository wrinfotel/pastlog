package claudecode

import "github.com/wrinfotel/pastlog/internal/agentlog"

// A later block can carry updated usage. Missing or malformed fields leave
// the previous value intact. An explicitly reported zero is a value.
func (u usageRecord) apply(tokens *agentlog.CtxTokens) {
	if u.InputTokens != nil {
		tokens.Input = *u.InputTokens
	}
	if u.OutputTokens != nil {
		tokens.Output = *u.OutputTokens
	}
	if u.CacheCreationInputTokens != nil {
		tokens.CacheWrite = *u.CacheCreationInputTokens
	}
	if u.CacheReadInputTokens != nil {
		tokens.CacheRead = *u.CacheReadInputTokens
	}
}

type responseUsage struct {
	model    string
	explicit bool
	reported bool
	tokens   agentlog.CtxTokens
}

// sessionUsage retains one usage snapshot per response, in first-use order.
// Transcript listing and search do not need to keep these snapshots.
type sessionUsage struct {
	latest    string
	responses []responseUsage
	byKey     map[string]int
}

func (s *sessionUsage) observe(info lineInfo) {
	if info.model != "" {
		s.latest = info.model
	}
	if info.response == "" && info.usage == nil {
		return
	}
	idx, found := s.byKey[info.response]
	if !found {
		idx = len(s.responses)
		s.responses = append(s.responses, responseUsage{model: s.latest})
		if info.response != "" {
			if s.byKey == nil {
				s.byKey = make(map[string]int)
			}
			s.byKey[info.response] = idx
		}
	}
	sample := &s.responses[idx]
	if !sample.explicit && info.model != "" {
		sample.model = info.model
		sample.explicit = true
	}
	if info.usage != nil {
		info.usage.apply(&sample.tokens)
		sample.reported = true
	}
}

func (s *sessionUsage) split() agentlog.ModelSplit {
	var split agentlog.ModelSplit
	for _, response := range s.responses {
		if !response.reported {
			continue
		}
		t := response.tokens
		split.Observe(response.model, agentlog.Usage{
			Input: t.Input, Output: t.Output, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite,
		})
	}
	split.Note(s.latest)
	return split
}
