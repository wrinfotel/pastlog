package antigravity

import (
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

func (a *Adapter) ContextEvents(session agentlog.Session) ([]agentlog.CtxEvent, error) {
	path, err := a.find(session.ID)
	if err != nil {
		return nil, err
	}
	requests := readGenerations(a.databasePath(session.ID))
	hasDBWindows := false
	for _, request := range requests {
		if request.primary.Input > 0 {
			hasDBWindows = true
			break
		}
	}
	var events []agentlog.CtxEvent
	emit := func(event agentlog.CtxEvent) {
		event.Seq = len(events) + 1
		events = append(events, event)
	}
	turn := func(at time.Time, u agentlog.Usage) {
		if u.Input <= 0 {
			return
		}
		emit(agentlog.CtxEvent{
			Kind: agentlog.CtxTurnStart, At: at, Role: "assistant", TokensKind: agentlog.CtxTokensExact,
			Tokens: agentlog.CtxTokens{
				Input: u.Input - u.CacheRead - u.CacheWrite, CacheRead: u.CacheRead,
				CacheWrite: u.CacheWrite, Output: u.Output, Reasoning: u.Reasoning,
			},
		})
	}
	requestIndex := 0
	if err := a.readSteps(session.ID, path, false, func(s step) error {
		at := s.timestamp()
		for requestIndex < len(requests) {
			r := requests[requestIndex]
			if r.step != nil && s.Index != nil {
				if *r.step > *s.Index {
					break
				}
			} else if r.at.IsZero() || at.IsZero() || r.at.After(at) {
				break
			}
			when := r.at
			if when.IsZero() {
				when = at
			}
			turn(when, r.primary)
			requestIndex++
		}
		if !hasDBWindows && s.Type == "PLANNER_RESPONSE" {
			turn(at, s.usage())
		}
		if role, text := s.messageRole(), s.messageText(); role != "" && text != "" {
			emit(agentlog.CtxEvent{Kind: agentlog.CtxMessage, At: at, Role: role, ResBytes: len(text)})
		}
		if s.Type == "CHECKPOINT" {
			emit(agentlog.CtxEvent{Kind: agentlog.CtxCompact, At: at, Role: "system"})
		}
		if s.Type == "PLANNER_RESPONSE" {
			for _, tool := range s.Calls {
				if tool.Name != "" {
					args := contextArgs(tool.Args)
					emit(agentlog.CtxEvent{
						Kind: agentlog.CtxToolCall, At: at, Role: "assistant", Tool: tool.Name,
						ArgsKey: agentlog.CanonArgsKey(tool.Name, args), Label: agentlog.ToolLabel(tool.Name, args),
					})
				}
			}
		}
		if tool := s.resultTool(); tool != "" && s.Content != "" {
			emit(agentlog.CtxEvent{
				Kind: agentlog.CtxToolResult, At: at, Role: "tool", Tool: tool, ResBytes: len(s.Content),
				Head: agentlog.FirstLine(s.Content), Err: agentlog.ErrFromStatus(s.Status, s.Content),
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	for _, request := range requests[requestIndex:] {
		turn(request.at, request.primary)
	}
	return events, nil
}
