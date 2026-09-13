// Package agentlog defines the unified session/entry model and the Adapter
// interface every agent-specific backend conforms to (spec §5), plus the
// adapter registry.
package agentlog

import "time"

type Session struct {
	ID        string
	Agent     string // "claude-code" | "codex" | "gemini-cli" | "opencode"
	Project   string // working dir, "" if unknown
	Title     string // optional
	StartedAt time.Time
	EndedAt   time.Time // optional
	SizeBytes int64
}

type EntryKind int // Message, ToolCall, ToolResult, Summary

const (
	Message EntryKind = iota
	ToolCall
	ToolResult
	Summary
)

type Entry struct {
	Kind      EntryKind
	Role      string // user/assistant/tool — best effort
	Text      string
	Timestamp time.Time // optional
}

type Adapter interface {
	Name() string
	Detect() bool                                    // storage present?
	Sessions(iter func(Session) error) error         // streaming; never loads whole files
	Entries(s Session, iter func(Entry) error) error // streaming
}

// SessionMeta is a Session plus metadata adapters compute while scanning.
// Messages is the count of message-kind entries (user/assistant/system,
// excluding summary/snapshot records) per spec §9.
type SessionMeta struct {
	Session
	Messages int
}

// MetaSource is optionally implemented by adapters that can supply message
// counts in the same streaming pass that yields sessions. Callers fall back
// to counting via Entries when unavailable.
type MetaSource interface {
	SessionsMeta(iter func(SessionMeta) error) error
}

// FastMetaSource is optionally implemented by adapters whose storage allows a
// cheap session listing (JSONL: the first record line of each file plus a
// stat) for latency-sensitive flows like search. SessionsMetaFast reports
// used=false when the adapter has no fast path, and the caller must fall back
// to SessionsMeta for the full-fidelity pass.
//
// The fast listing keeps ids, projects, start timestamps and therefore
// filters (--agent/--project/--since/--until) and sort order identical to
// SessionsMeta; fields that live beyond the first record line (last
// timestamps, message counts, summary titles) may be zero. Callers that need
// those fields exactly (the --json machine output) must use SessionsMeta.
type FastMetaSource interface {
	SessionsMetaFast(iter func(SessionMeta) error) (used bool, err error)
}

// SkipCounter is optionally implemented by adapters that skip corrupt or
// unknown records; the CLI summarizes the total on stderr (spec §8).
type SkipCounter interface {
	SkippedLines() int
}

// PathSource is optionally implemented by adapters that can report their
// storage location for display in `agents`.
type PathSource interface {
	StoragePath() string
}

// WarningSource is optionally implemented by adapters that detected storage
// they cannot read (e.g. a database locked by a running agent instance, spec
// §4): Warning returns one actionable stderr line, "" when the adapter is
// healthy. The CLI prints it once per run and continues with other agents.
type WarningSource interface {
	Warning() string
}

// TotalSizer is optionally implemented by adapters whose on-disk footprint
// differs from the sum of their sessions' SizeBytes (e.g. a single database
// file shared by all sessions); `agents` reports TotalBytes then.
type TotalSizer interface {
	TotalBytes() int64
}
