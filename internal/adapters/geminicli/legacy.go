package geminicli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// legacyRaw mirrors one ConversationRecord element of a legacy monolithic
// chats.json — the same shape as a JSONL session, with the messages inline
// (see SCHEMA.md).
type legacyRaw struct {
	SessionID   string            `json:"sessionId"`
	StartTime   string            `json:"startTime"`
	LastUpdated string            `json:"lastUpdated"`
	Directories []string          `json:"directories"`
	Summary     string            `json:"summary"`
	Messages    []json.RawMessage `json:"messages"`
}

// walkLegacy streams the sessions array of one monolithic chats.json. In
// listing mode (wantID "") every session goes through iter with emit nil; in
// entries mode only the session whose id matches wantID is mapped and its
// entries go through emit. Unreadable wrappers/elements are skipped and
// counted; a non-nil iter error aborts the walk. iter receives the usage
// view (M7) — SessionsMeta wraps it back to the meta view.
func (a *Adapter) walkLegacy(path, wantID string, iter func(agentlog.SessionUsage) error, emit func(agentlog.Entry) error) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // unreadable file: skip silently
	}
	defer f.Close()

	dec := json.NewDecoder(bufio.NewReaderSize(f, initialBuf))
	if !delim(dec, '{') {
		a.skipped++ // not the expected object wrapper
		return nil
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			a.skipped++ // truncated wrapper
			return nil
		}
		key, _ := keyTok.(string)
		if key != "sessions" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				a.skipped++
				return nil
			}
			continue
		}
		if !delim(dec, '[') {
			a.skipped++ // "sessions" is not an array
			return nil
		}
		for dec.More() {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				a.skipped++ // truncated element: the rest is unreadable
				return nil
			}
			su, entries, ok := a.legacySession(raw)
			if !ok {
				a.skipped++
				continue
			}
			a.registerIDs(path, su.ID, true) // feed the id→store index (idempotent)
			if wantID != "" && su.ID != wantID {
				continue
			}
			if emit != nil {
				for _, e := range entries {
					if err := emit(e); err != nil {
						return err
					}
				}
			}
			if iter != nil {
				if err := iter(su); err != nil {
					return err
				}
			}
		}
		if !delim(dec, ']') {
			a.skipped++
			return nil
		}
	}
	return nil // closing '}' consumed by the decoder on Close/EOF
}

// delim consumes one expected delimiter token, reporting a mismatch.
func delim(dec *json.Decoder, want json.Delim) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	d, ok := tok.(json.Delim)
	return ok && d == want
}

// legacySession maps one raw ConversationRecord element to the session usage
// view and its entries. Unusable message elements count as skipped. ok=false
// marks a session pastlog cannot name (no sessionId).
func (a *Adapter) legacySession(raw []byte) (agentlog.SessionUsage, []agentlog.Entry, bool) {
	var rec legacyRaw
	if err := json.Unmarshal(bytes.TrimSpace(raw), &rec); err != nil {
		return agentlog.SessionUsage{}, nil, false
	}
	if rec.SessionID == "" {
		return agentlog.SessionUsage{}, nil, false
	}
	var entries []agentlog.Entry
	var usage agentlog.Usage
	for _, msg := range rec.Messages {
		// same record processing as the JSONL layout: token facts accumulate
		// per session (M7)
		if sub, ru, ok := processRecord(msg, &a.skipped); ok {
			entries = append(entries, sub...)
			if ru.model != "" {
				usage.Model = ru.model
			}
			if ru.tokens != nil {
				usage.Input += ptrVal(ru.tokens.Input)
				usage.Output += ptrVal(ru.tokens.Output)
				usage.CacheRead += ptrVal(ru.tokens.Cached)
				usage.Reasoning += ptrVal(ru.tokens.Thoughts)
			}
		} else {
			a.skipped++
		}
	}
	su := agentlog.SessionUsage{
		Session: agentlog.Session{
			ID:        rec.SessionID,
			Agent:     agentName,
			Project:   firstString(rec.Directories),
			Title:     rec.Summary,
			StartedAt: parseTS(rec.StartTime),
			EndedAt:   parseTS(rec.LastUpdated),
			SizeBytes: int64(len(bytes.TrimSpace(raw))), // approximation: the raw element (SCHEMA.md)
		},
		Messages: countMessages(entries),
		Usage:    usage,
	}
	return su, entries, true
}

func countMessages(entries []agentlog.Entry) int {
	n := 0
	for _, e := range entries {
		if e.Kind == agentlog.Message {
			n++
		}
	}
	return n
}

// legacySessionIDs lists the session ids inside one monolithic chats.json,
// in stored order (used to fill the id→store index in one pass). Unreadable
// wrappers or elements yield whatever was decoded before the failure.
func (a *Adapter) legacySessionIDs(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var ids []string
	dec := json.NewDecoder(bufio.NewReaderSize(f, initialBuf))
	if !delim(dec, '{') {
		return nil
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return ids
		}
		key, _ := keyTok.(string)
		if key != "sessions" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return ids
			}
			continue
		}
		if !delim(dec, '[') {
			return ids
		}
		for dec.More() {
			var rec legacyRaw
			if err := dec.Decode(&rec); err != nil {
				return ids
			}
			if rec.SessionID != "" {
				ids = append(ids, rec.SessionID)
			}
		}
		if !delim(dec, ']') {
			return ids
		}
	}
	return ids
}
