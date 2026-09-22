package agentlog

import "time"

// CtxSource is optionally implemented by adapters that can emit the
// normalized context-event stream for a session (SPEC-context-analysis.md
// §1). The context analysis runs ONLY on this stream: same rules, same
// detail level for every agent; agent differences show up as token
// precision (Exact vs Estimated), never as a different rule set.
type CtxSource interface {
	ContextEvents(s Session) ([]CtxEvent, error)
}

// CtxKind discriminates one normalized context event.
type CtxKind int

const (
	CtxTurnStart CtxKind = iota // one per model turn carrying usage
	CtxToolCall
	CtxToolResult
	CtxMessage
	CtxCompact // compaction boundary (explicit marker or heuristic)
)

// CtxTokens is one turn's usage. Fields an agent does not expose stay zero.
// The context-window proxy is C(t) = Input + CacheRead + CacheWrite.
type CtxTokens struct {
	Input, Output, Reasoning int64
	CacheRead, CacheWrite    int64
}

// Sum returns the context-window proxy C(t) for one turn's usage.
func (t CtxTokens) Sum() int64 { return t.Input + t.CacheRead + t.CacheWrite }

// CtxTokensKind marks how CtxEvent.Tokens was obtained.
type CtxTokensKind int

const (
	CtxTokensNone CtxTokensKind = iota // event carries no usage
	CtxTokensExact                     // from the agent's own reported usage
	CtxTokensEstimated                 // derived from bytes (bytes/4 proxy)
)

// CtxEvent is one normalized event of the context IR. ResBytes is the raw
// UTF-8 length of the text a tool returned or a message carried — every
// format provides it, so size-based rules stay uniform across agents.
type CtxEvent struct {
	Seq        int
	At         time.Time
	Kind       CtxKind
	Role       string // user / assistant / tool / system (best effort)
	Tool       string // tool name, "" when not a tool event
	ArgsKey    string // normalized call identity (SPEC §1.1), tool calls only
	Label      string // short human label for the call (file path, command, …)
	ResBytes   int    // raw byte length of the result / message text
	Head       string // first line of a ToolResult (capped), for error headlines
	Err        bool   // ToolResult: the agent flagged it as an error
	Tokens     CtxTokens
	TokensKind CtxTokensKind
}
