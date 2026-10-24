package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// This file implements agentlog.CtxSource for codex (SPEC context-analysis
// §3): one streaming pass over the rollout JSONL mapped to the normalized
// IR. The adapter's own scans (listing, entries, search prefilter) are
// untouched and the shared skipped counter is not used here — the context
// flow is best-effort by nature and skips unusable lines silently.
//
// Tokens: a token_count's last_token_usage is the per-request usage, so
// Input+CacheRead is that turn's window proxy (the repo maps
// cached_input_tokens to CacheRead, SCHEMA.md). The record rides the
// top-level token_count type in pre-0.5 rollouts and an event_msg envelope
// in current ones — both map here, while token_usage_record lines (which
// repeat the same per-turn usage) emit nothing: double-sourcing would
// double every turn. Rollouts missing last_token_usage fall back to the
// cumulative totals' diff (SPEC §6.2). The `compacted` record type is
// recognized HERE as a Compact event; the stats flow keeps counting it as
// skipped (SCHEMA.md). Codex exposes no error flag, so R3 runs on the
// uniform error-shape regex (SPEC §2.2).

// ctxRecord mirrors one JSONL line for context extraction: only IR-relevant
// fields are declared; everything is optional (shapes are undocumented).
type ctxRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// ContextEvents implements agentlog.CtxSource.
func (a *Adapter) ContextEvents(s agentlog.Session) ([]agentlog.CtxEvent, error) {
	path, found := a.fileForSession(s.ID)
	if !found {
		return nil, fmt.Errorf("session %s not found in codex storage", s.ID)
	}
	events := []agentlog.CtxEvent{}
	seq := 0
	next := func() int { seq++; return seq }
	pending := map[string]string{} // call_id -> tool name
	var unclaimed []string         // FIFO of tool names from id-less calls
	var prevTotal *tokenUsage      // cumulative totals of the previous token_count

	err := scanCtxLines(path, func(line []byte) {
		var rec ctxRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return // corrupt line: skip silently (best-effort flow)
		}
		ts := parseCtxTimestamp(rec.Timestamp)

		switch rec.Type {
		case "compacted":
			events = append(events, agentlog.CtxEvent{
				Seq: next(), At: ts, Kind: agentlog.CtxCompact, Role: "system",
			})
			return
		case "token_count":
			var tp tokenCountPayload
			if err := json.Unmarshal(bytes.TrimSpace(rec.Payload), &tp); err != nil {
				return
			}
			events = append(events, agentlog.CtxEvent{
				Seq: next(), At: ts, Kind: agentlog.CtxTurnStart, Role: "assistant",
				Tokens:     ctxTurnTokens(&tp.Info, &prevTotal),
				TokensKind: agentlog.CtxTokensExact,
			})
			return
		case "event_msg":
			// current rollouts wrap token_count in an event envelope; every
			// other event (item echoes, task lifecycle, …) maps to nothing
			var ep eventMsgPayload
			if err := json.Unmarshal(bytes.TrimSpace(rec.Payload), &ep); err != nil {
				return
			}
			if ep.Type != "token_count" {
				return
			}
			events = append(events, agentlog.CtxEvent{
				Seq: next(), At: ts, Kind: agentlog.CtxTurnStart, Role: "assistant",
				Tokens:     ctxTurnTokens(&ep.Info, &prevTotal),
				TokensKind: agentlog.CtxTokensExact,
			})
			return
		case "token_usage_record":
			// stats-flow source only (see the file comment): the per-turn
			// usage repeats the token_count envelope's last_token_usage
			return
		case "response_item":
			var ip itemPayload
			if err := json.Unmarshal(bytes.TrimSpace(rec.Payload), &ip); err != nil {
				return
			}
			switch ip.Type {
			case "message":
				entries, ok := messageEntries(ip, ts)
				if !ok {
					return
				}
				for _, e := range entries {
					if e.Kind != agentlog.Message || e.Text == "" {
						continue
					}
					events = append(events, agentlog.CtxEvent{
						Seq: next(), At: e.Timestamp, Kind: agentlog.CtxMessage,
						Role: e.Role, ResBytes: len(e.Text),
					})
				}
			case "function_call", "custom_tool_call":
				name := strings.TrimSpace(ip.Name)
				if name == "" {
					return // readable line, nothing to record
				}
				if ip.CallID != "" {
					pending[ip.CallID] = name
				} else {
					unclaimed = append(unclaimed, name)
				}
				// function_call logs JSON arguments; custom_tool_call a raw
				// input string (CanonJSON passes non-JSON through trimmed)
				input := json.RawMessage(ip.Arguments)
				if ip.Type == "custom_tool_call" || strings.TrimSpace(string(input)) == "" {
					if in := strings.TrimSpace(ip.Input); in != "" {
						input = json.RawMessage(in)
					}
				}
				events = append(events, agentlog.CtxEvent{
					Seq: next(), At: ts, Kind: agentlog.CtxToolCall, Role: "assistant",
					Tool: name, ArgsKey: agentlog.CanonArgsKey(name, input),
					Label: agentlog.ToolLabel(name, input),
				})
			case "compaction":
				// the current format's compaction boundary (encrypted
				// payload); the old top-level `compacted` spelling maps the
				// same way
				events = append(events, agentlog.CtxEvent{
					Seq: next(), At: ts, Kind: agentlog.CtxCompact, Role: "system",
				})
			case "function_call_output", "custom_tool_call_output":
				text, ok := rawText(ip.Output)
				if !ok {
					return // malformed output array: skip silently
				}
				ev := agentlog.CtxEvent{
					Seq: next(), At: ts, Kind: agentlog.CtxToolResult, Role: "tool",
					Err:      agentlog.ErrShaped(text),
					ResBytes: len(text),
					Head:     agentlog.FirstLine(text),
				}
				if ip.CallID != "" {
					if name, ok := pending[ip.CallID]; ok {
						ev.Tool = name
						delete(pending, ip.CallID)
					}
				} else if len(unclaimed) > 0 {
					ev.Tool = unclaimed[0]
					unclaimed = unclaimed[1:]
				}
				events = append(events, ev)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// ctxTurnTokens extracts one turn's window proxy: the record's
// last_token_usage (per-request usage) when present, else the diff of the
// cumulative totals against the previous record — the first record's totals
// stand in for its turn. Negative diffs (a compaction reset in the
// cumulative counter) clamp to zero. The cumulative totals are remembered
// either way.
func ctxTurnTokens(ti *tokenInfo, prevTotal **tokenUsage) agentlog.CtxTokens {
	total := ti.TotalTokenUsage.usage()
	defer func() { *prevTotal = &total }()

	if ti.LastTokenUsage.present() {
		last := ti.LastTokenUsage.usage()
		return agentlog.CtxTokens{
			Input:     last.input,
			CacheRead: last.cached,
			Output:    last.output,
			Reasoning: last.reasoning,
		}
	}

	per := total
	if p := *prevTotal; p != nil {
		per = tokenUsage{
			input:     ctxNonNeg(total.input - p.input),
			cached:    ctxNonNeg(total.cached - p.cached),
			output:    ctxNonNeg(total.output - p.output),
			reasoning: ctxNonNeg(total.reasoning - p.reasoning),
		}
	}
	return agentlog.CtxTokens{
		Input:     per.input,
		CacheRead: per.cached,
		Output:    per.output,
		Reasoning: per.reasoning,
	}
}

// ctxNonNeg clamps a usage delta to ≥0: last_token_usage is a cumulative
// total, so a negative diff means the counter went backwards (a compaction
// reset) and the turn contributed nothing observable.
func ctxNonNeg(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// scanCtxLines streams one JSONL file line by line; fn gets each non-empty
// line. Unreadable files are silent (best-effort flow, mirrors scanFile).
func scanCtxLines(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initialBuf), maxLineSize)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		fn(line)
	}
	return nil
}

// parseCtxTimestamp parses RFC 3339 best effort; zero on failure.
func parseCtxTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts
	}
	return time.Time{}
}
