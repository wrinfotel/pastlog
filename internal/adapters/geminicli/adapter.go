// Package geminicli adapts Google Gemini CLI session logs: JSONL files under
// ~/.gemini/tmp/<project-hash>/chats/ (see SCHEMA.md), including the legacy
// monolithic chats.json format. The format is undocumented and
// version-dependent, so parsing is defensive: unknown shapes are skipped and
// counted, never fatal, never a panic. Files are streamed, never loaded whole.
package geminicli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

const (
	agentName   = "gemini-cli"
	initialBuf  = 64 * 1024
	maxLineSize = 16 * 1024 * 1024
	legacyName  = "chats.json" // legacy monolithic format, one per project dir
)

// Adapter reads Gemini CLI session logs from <home>/.gemini/tmp.
type Adapter struct {
	home    string
	skipped int // unreadable/unknown records, counted across scans
}

// New builds an adapter rooted at the given user home directory.
func New(home string) *Adapter { return &Adapter{home: home} }

func (a *Adapter) Name() string { return agentName }

func (a *Adapter) Detect() bool {
	info, err := os.Stat(a.tmpDir())
	return err == nil && info.IsDir()
}

// SkippedLines implements agentlog.SkipCounter (spec §8 stderr summary).
func (a *Adapter) SkippedLines() int { return a.skipped }

// StoragePath implements agentlog.PathSource: the directory scanned for
// sessions, for display in `agents`.
func (a *Adapter) StoragePath() string { return a.tmpDir() }

func (a *Adapter) tmpDir() string {
	return filepath.Join(a.home, ".gemini", "tmp")
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
// (internal/search.LineFilteredAdapter) for the line-oriented JSONL files.
// For sessions inside the legacy monolithic chats.json the predicate is
// ignored — a raw line is not a record boundary there, so correctness wins
// over the hot path (documented in SCHEMA.md).
func (a *Adapter) EntriesFiltered(s agentlog.Session, keep func(rawLine []byte) bool, iter func(agentlog.Entry) error) error {
	return a.entriesFor(s, keep, iter)
}

// sessionRef locates a session's backing store: a JSONL file, or an element
// inside a legacy monolithic chats.json.
type sessionRef struct {
	path   string
	legacy bool
}

func (a *Adapter) entriesFor(s agentlog.Session, keep func([]byte) bool, iter func(agentlog.Entry) error) error {
	ref, found := a.sessionFor(s.ID)
	if !found {
		return fmt.Errorf("session %s not found in gemini-cli storage", s.ID)
	}
	if ref.legacy {
		// legacy: re-stream the monolithic file, emit only this session's
		// entries; the raw-line prefilter does not apply (see EntriesFiltered)
		return a.walkLegacy(ref.path, s.ID, nil, iter)
	}
	_, err := a.scanFile(ref.path, keep, iter)
	return err
}

// walk streams every session, in sorted path order, through iter: main and
// subagent JSONL files first, then the legacy monolithic chats.json files.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	for _, path := range a.sessionFiles() {
		sum, _ := a.scanFile(path, nil, nil)
		if !sum.sawLine {
			continue // empty (or blank) file: not a session
		}
		if err := iter(sum.meta(path)); err != nil {
			return err
		}
	}
	for _, path := range a.legacyFiles() {
		if err := a.walkLegacy(path, "", iter, nil); err != nil {
			return err
		}
	}
	return nil
}

// fileSummary aggregates one scanned JSONL file.
type fileSummary struct {
	sessionID          string
	project            string
	title              string
	startedAt, endedAt time.Time
	messages           int
	size               int64
	sawLine            bool
	metaStart, metaEnd bool // metadata startTime / lastUpdated seen: they win over record timestamps
}

func (f fileSummary) meta(path string) agentlog.SessionMeta {
	return agentlog.SessionMeta{
		Session: agentlog.Session{
			ID:        f.id(path),
			Agent:     agentName,
			Project:   f.project,
			Title:     f.title,
			StartedAt: f.startedAt,
			EndedAt:   f.endedAt,
			SizeBytes: f.size,
		},
		Messages: f.messages,
	}
}

// id prefers the metadata record's sessionId; the filename is the fallback.
func (f fileSummary) id(path string) string {
	if f.sessionID != "" {
		return f.sessionID
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// scanFile streams one JSONL session file. keep may be nil (keep every line);
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
	first := true
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		sum.sawLine = true
		if first {
			first = false
			if meta, ok := parseMeta(line); ok {
				sum.sessionID = meta.sessionID
				sum.project = meta.project
				sum.title = meta.title
				sum.startedAt = meta.startedAt
				sum.endedAt = meta.endedAt
				sum.metaStart = !meta.startedAt.IsZero()
				sum.metaEnd = !meta.endedAt.IsZero()
				continue
			}
			// line 1 without metadata: fall through, treat it as a record
		}
		if keep != nil && !keep(line) {
			continue // prefilter: not a candidate, not an error
		}
		entries, ok := processRecord(line, &a.skipped)
		if !ok {
			a.skipped++
			continue
		}
		if err := a.forward(entries, emit, &sum); err != nil {
			return sum, err
		}
	}
	if err := scanner.Err(); err != nil {
		// A line beyond maxLineSize exhausts the buffer; the rest of this
		// file is unreadable to us. Count it and move on — never fatal.
		a.skipped++
	}
	return sum, nil
}

// forward hands classified entries to emit while counting messages and
// filling the session bounds from record timestamps where the metadata record
// had none (SCHEMA.md).
func (a *Adapter) forward(entries []agentlog.Entry, emit func(agentlog.Entry) error, sum *fileSummary) error {
	for _, e := range entries {
		if e.Kind == agentlog.Message {
			sum.messages++
		}
		if ts := e.Timestamp; !ts.IsZero() {
			if !sum.metaStart && sum.startedAt.IsZero() {
				sum.startedAt = ts
			}
			if !sum.metaEnd {
				sum.endedAt = ts
			}
		}
		if emit != nil {
			if err := emit(e); err != nil {
				return err
			}
		}
	}
	return nil
}

// sessionFiles lists all main and subagent JSONL paths, sorted for
// determinism: session-*.jsonl directly under chats/, then any *.jsonl in
// chats/<parent-session-id>/ subdirectories.
func (a *Adapter) sessionFiles() []string {
	main, _ := filepath.Glob(filepath.Join(a.tmpDir(), "*", "chats", "session-*.jsonl"))
	sub, _ := filepath.Glob(filepath.Join(a.tmpDir(), "*", "chats", "*", "*.jsonl"))
	sort.Strings(main)
	sort.Strings(sub)
	return append(main, sub...)
}

// legacyFiles lists the monolithic chats.json paths, one per project dir.
func (a *Adapter) legacyFiles() []string {
	files, _ := filepath.Glob(filepath.Join(a.tmpDir(), "*", legacyName))
	sort.Strings(files)
	return files
}

// sessionFor locates the backing store of a session id: by exact filename
// first (subagent files and filename-fallback ids), then by scanning the
// metadata record of every JSONL file, then inside the legacy chats.json
// files.
func (a *Adapter) sessionFor(id string) (sessionRef, bool) {
	if id == "" {
		return sessionRef{}, false
	}
	for _, pattern := range []string{
		filepath.Join(a.tmpDir(), "*", "chats", id+".jsonl"),      // main fallback id / stray file
		filepath.Join(a.tmpDir(), "*", "chats", "*", id+".jsonl"), // subagent file
	} {
		if matches, err := filepath.Glob(pattern); err == nil && len(matches) > 0 {
			sort.Strings(matches)
			return sessionRef{path: matches[0]}, true
		}
	}
	for _, path := range a.sessionFiles() {
		if a.fileHasMetaID(path, id) {
			return sessionRef{path: path}, true
		}
	}
	for _, path := range a.legacyFiles() {
		if a.legacyHasSession(path, id) {
			return sessionRef{path: path, legacy: true}, true
		}
	}
	return sessionRef{}, false
}

// fileHasMetaID reports whether the file's metadata line (first non-empty
// line) carries sessionId == id.
func (a *Adapter) fileHasMetaID(path, id string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initialBuf), maxLineSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		meta, ok := parseMeta(line)
		if ok && meta.sessionID == id {
			return true
		}
		return false // only the first non-empty line can be the metadata record
	}
	return false
}
