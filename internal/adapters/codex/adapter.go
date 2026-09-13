// Package codex adapts OpenAI Codex CLI session logs: JSONL rollout files
// under ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl (see SCHEMA.md). The
// format is undocumented and version-dependent, so parsing is defensive:
// unknown shapes are skipped and counted, never fatal, never a panic. Files
// are streamed line by line, never loaded whole.
package codex

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

const (
	agentName   = "codex"
	initialBuf  = 64 * 1024
	maxLineSize = 16 * 1024 * 1024
)

// Adapter reads Codex CLI JSONL rollout files from <home>/.codex/sessions.
//
// Like the skipped counter (which accumulates across scans and is never
// reset), the session id→file index built by fileForSession is cached on the
// adapter: an Adapter is NOT safe for concurrent use — one Adapter per
// goroutine, and resolve sessions through a single instance per run.
type Adapter struct {
	home    string
	skipped int               // unreadable/unknown lines, counted across scans
	index   map[string]string // session id -> rollout path, built lazily (see fileForSession)
}

// New builds an adapter rooted at the given user home directory.
func New(home string) *Adapter { return &Adapter{home: home} }

func (a *Adapter) Name() string { return agentName }

func (a *Adapter) Detect() bool {
	info, err := os.Stat(a.sessionsDir())
	return err == nil && info.IsDir()
}

// SkippedLines implements agentlog.SkipCounter (spec §8 stderr summary).
func (a *Adapter) SkippedLines() int { return a.skipped }

// StoragePath implements agentlog.PathSource: the directory scanned for
// sessions, for display in `agents`.
func (a *Adapter) StoragePath() string { return a.sessionsDir() }

func (a *Adapter) sessionsDir() string {
	return filepath.Join(a.home, ".codex", "sessions")
}

func (a *Adapter) Sessions(iter func(agentlog.Session) error) error {
	return a.walk(func(m agentlog.SessionMeta) error {
		return iter(m.Session)
	})
}

// SessionsMeta implements agentlog.MetaSource: message counts come from the
// same streaming pass that yields sessions.
func (a *Adapter) SessionsMeta(iter func(agentlog.SessionMeta) error) error {
	return a.walk(iter)
}

func (a *Adapter) Entries(s agentlog.Session, iter func(agentlog.Entry) error) error {
	return a.entriesFor(s, nil, iter)
}

// EntriesFiltered serves the search engine's raw-line prefilter hook
// (internal/search.LineFilteredAdapter): when keep is non-nil, lines failing
// the predicate are skipped before any JSON parsing (spec §7 hot path).
// Rejected lines are not counted as skipped — they are merely unsearched.
func (a *Adapter) EntriesFiltered(s agentlog.Session, keep func(rawLine []byte) bool, iter func(agentlog.Entry) error) error {
	return a.entriesFor(s, keep, iter)
}

func (a *Adapter) entriesFor(s agentlog.Session, keep func([]byte) bool, iter func(agentlog.Entry) error) error {
	path, found := a.fileForSession(s.ID)
	if !found {
		return fmt.Errorf("session %s not found in codex storage", s.ID)
	}
	_, err := a.scanFile(path, keep, iter)
	return err
}

// walk streams every rollout file, in sorted path order, through iter.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	files, err := a.sessionFiles()
	if err != nil {
		return fmt.Errorf("cannot list codex storage: %v", err)
	}
	for _, path := range files {
		sum, err := a.scanFile(path, nil, nil)
		if err != nil {
			return err // with a nil emit this cannot fire today; propagation keeps listing honest if scanFile ever gains an error path
		}
		if !sum.sawLine {
			continue // empty (or blank) file: not a session
		}
		a.registerIDs(path, sum.sessionID)
		if err := iter(sum.meta(path)); err != nil {
			return err
		}
	}
	return nil
}

// fileSummary aggregates one scanned file.
type fileSummary struct {
	sessionID          string
	cwd                string
	title              string
	startedAt, endedAt time.Time
	messages           int
	size               int64
	sawLine            bool
}

func (f fileSummary) meta(path string) agentlog.SessionMeta {
	return agentlog.SessionMeta{
		Session: agentlog.Session{
			ID:        f.id(path),
			Agent:     agentName,
			Project:   f.cwd,
			Title:     f.title,
			StartedAt: f.startedAt,
			EndedAt:   f.endedAt,
			SizeBytes: f.size,
		},
		Messages: f.messages,
	}
}

// id prefers the session_meta payload id; the rollout filename is the
// fallback (SCHEMA.md).
func (f fileSummary) id(path string) string {
	if f.sessionID != "" {
		return f.sessionID
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// scanFile streams one JSONL rollout file. keep may be nil (keep every line);
// when set, lines failing it are skipped before parsing (search prefilter).
// emit may be nil (listing mode); when set, every classified entry is
// forwarded and a non-nil error aborts the scan (early cutoff). Unreadable
// lines increment the adapter-wide skipped counter. Never panics on
// malformed input.
func (a *Adapter) scanFile(path string, keep func([]byte) bool, emit func(agentlog.Entry) error) (fileSummary, error) {
	var sum fileSummary

	f, err := os.Open(path)
	if err != nil {
		return sum, nil // unreadable file: skip silently
	}
	defer f.Close()

	if info, err := f.Stat(); err == nil {
		sum.size = info.Size()
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initialBuf), maxLineSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		sum.sawLine = true
		if keep != nil && !keep(line) {
			continue // prefilter: not a candidate, not an error
		}
		info, entries, ok := processLine(line)
		if !ok {
			a.skipped++
			continue
		}
		if info.sessionID != "" && sum.sessionID == "" {
			sum.sessionID = info.sessionID
		}
		if info.cwd != "" && sum.cwd == "" {
			sum.cwd = info.cwd
		}
		for _, e := range entries {
			if e.Kind == agentlog.Message && e.Role == "user" && sum.title == "" {
				sum.title = truncateTitle(e.Text)
			}
			if e.Kind == agentlog.Message {
				sum.messages++
			}
			if emit != nil {
				if err := emit(e); err != nil {
					return sum, err
				}
			}
		}
		if !info.ts.IsZero() {
			if sum.startedAt.IsZero() {
				sum.startedAt = info.ts
			}
			sum.endedAt = info.ts
		}
	}
	if err := scanner.Err(); err != nil {
		// A line beyond maxLineSize exhausts the buffer; the rest of this
		// file is unreadable to us. Count it and move on — never fatal.
		a.skipped++
	}
	return sum, nil
}

// sessionFiles lists all rollout JSONL paths under
// sessions/YYYY/MM/DD, sorted for determinism.
func (a *Adapter) sessionFiles() ([]string, error) {
	return filepath.Glob(filepath.Join(a.sessionsDir(), "*", "*", "*", "rollout-*.jsonl"))
}

// fileForSession locates the JSONL file backing a session: first by rollout
// filename (the fallback-id case), then by scanning session_meta records for
// the payload id — the two may differ (SCHEMA.md).
//
// The id→path index is built lazily (see registerIDs): ONE pass over the
// storage, instead of a per-session tree walk plus a per-session rescan of
// every rollout's meta line (which made per-session Entries quadratic in the
// session count — surfaced by the M4 benchmark). Sessions obtained from
// listing resolve through the index that listing already filled.
func (a *Adapter) fileForSession(id string) (string, bool) {
	if a.index == nil {
		a.buildIndex()
	}
	path, ok := a.index[id]
	return path, ok
}

// registerIDs records a file's id spellings — filename minus .jsonl (the
// fallback id) and the first session_meta payload id — in the index. Files
// are visited in sorted order and the first registration of an id wins.
func (a *Adapter) registerIDs(path, recordID string) {
	if a.index == nil {
		a.index = map[string]string{}
	}
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if _, ok := a.index[base]; !ok {
		a.index[base] = path
	}
	if recordID != "" {
		if _, ok := a.index[recordID]; !ok {
			a.index[recordID] = path
		}
	}
}

// buildIndex fills the id→path index when Entries is called without a
// preceding listing pass.
func (a *Adapter) buildIndex() {
	files, err := a.sessionFiles()
	if err != nil {
		a.index = map[string]string{}
		return
	}
	for _, path := range files {
		a.registerIDs(path, a.fileMetaID(path))
	}
}

// fileMetaID returns the first session_meta payload id in the file, "" when
// the file has none (defensive parse).
func (a *Adapter) fileMetaID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initialBuf), maxLineSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		info, _, ok := processLine(line)
		if ok && info.sessionID != "" {
			return info.sessionID
		}
	}
	return ""
}

const maxTitleRunes = 80

// truncateTitle builds the best-effort session title from the first user
// message: whitespace runs collapse to single spaces, and anything past 80
// runes becomes an ellipsis.
func truncateTitle(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= maxTitleRunes {
		return text
	}
	return string(runes[:maxTitleRunes-1]) + "…"
}
