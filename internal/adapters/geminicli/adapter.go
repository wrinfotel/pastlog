// Package geminicli adapts Google Gemini CLI session logs: JSONL files under
// ~/.gemini/tmp/<project-hash>/chats/ (see SCHEMA.md), including the legacy
// monolithic chats.json format. The format is undocumented and
// version-dependent, so parsing is defensive: unknown shapes are skipped and
// counted, never fatal, never a panic. Files are streamed, never loaded whole.
package geminicli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
//
// Like the skipped counter (which accumulates across scans and is never
// reset), the session id→store index built by sessionFor is cached on the
// adapter: an Adapter is NOT safe for concurrent use — one Adapter per
// goroutine, and resolve sessions through a single instance per run.
type Adapter struct {
	home    string
	skipped int                   // unreadable/unknown records, counted across scans
	index   map[string]sessionRef // session id -> backing store, built lazily (see sessionFor)
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

// SessionsUsage implements agentlog.UsageSource (M7): token usage comes from
// the same streaming pass that yields sessions — the `tokens` summaries of
// the session's records accumulate (input/output/cached/thoughts; `tool` and
// `total` are ignored) and Model is the LAST non-empty record model. Usage
// rides the shared walkAll pass; gemini-cli provides no per-session cost.
func (a *Adapter) SessionsUsage(iter func(agentlog.SessionUsage) error) error {
	return a.walkAll(iter)
}

// SessionsMetaFast implements agentlog.FastMetaSource: the search flow lists
// sessions from the FIRST line of each JSONL file plus a stat — gemini-cli
// files open with a metadata record carrying sessionId, project, title and
// both timestamps (SCHEMA.md) — instead of parsing every line (spec §7).
// When line 1 is not a metadata record with a sessionId and a startTime, the
// file falls back to the full parse so ids, projects, start timestamps,
// filters and sort order stay identical to SessionsMeta (a metadata record
// without startTime would yield a zero StartedAt on the fast path while the
// full parse backfills it from the record timestamps). Message counts are
// zero on this path. Legacy chats.json stores keep their full listing (they
// are rare and monolithic); every listed session feeds the id→store index
// that Entries resolves through.
func (a *Adapter) SessionsMetaFast(iter func(agentlog.SessionMeta) error) (bool, error) {
	files, err := a.sessionFiles()
	if err != nil {
		return true, fmt.Errorf("cannot list gemini-cli storage: %v", err)
	}
	for _, path := range files {
		m, fast := a.fastMeta(path)
		if !fast {
			sum, err := a.scanFile(path, nil, nil)
			if err != nil {
				return true, err
			}
			if !sum.sawLine {
				continue // empty (or blank) file: not a session
			}
			a.registerIDs(path, sum.sessionID, false)
			m = sum.meta(path)
		} else {
			a.registerIDs(path, m.ID, false)
		}
		if err := iter(m); err != nil {
			return true, err
		}
	}
	legacy, err := a.legacyFiles()
	if err != nil {
		return true, fmt.Errorf("cannot list gemini-cli storage: %v", err)
	}
	for _, path := range legacy {
		if err := a.walkLegacy(path, "", func(su agentlog.SessionUsage) error {
			return iter(agentlog.SessionMeta{Session: su.Session, Messages: su.Messages})
		}, nil); err != nil {
			return true, err
		}
	}
	return true, nil
}

// fastMeta builds session metadata from the file's first line plus a stat.
// fast=false when that line is not a metadata record carrying a sessionId
// and a startTime — the caller then does a full parse, which backfills the
// start timestamp from the record timestamps and keeps listing parity with
// SessionsMeta (FastMetaSource contract).
func (a *Adapter) fastMeta(path string) (agentlog.SessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return agentlog.SessionMeta{}, false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return agentlog.SessionMeta{}, false
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initialBuf), maxLineSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		meta, ok := parseMeta(line)
		if !ok || meta.sessionID == "" || meta.startedAt.IsZero() {
			return agentlog.SessionMeta{}, false
		}
		return agentlog.SessionMeta{
			Session: agentlog.Session{
				ID:        meta.sessionID,
				Agent:     agentName,
				Project:   meta.project,
				Title:     meta.title,
				StartedAt: meta.startedAt,
				EndedAt:   meta.endedAt,
				SizeBytes: info.Size(),
			},
		}, true
	}
	return agentlog.SessionMeta{}, false
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
	return a.walkAll(func(su agentlog.SessionUsage) error {
		return iter(agentlog.SessionMeta{Session: su.Session, Messages: su.Messages})
	})
}

// walkAll streams every session's usage view through iter — the one shared
// walk behind Sessions/SessionsMeta (meta view) and SessionsUsage (usage
// view), so all listing passes scan the stores once, in the same sorted
// order, and skip/usage accounting stays identical. JSONL main/subagent
// files go first, then the legacy monolithic chats.json files.
func (a *Adapter) walkAll(iter func(agentlog.SessionUsage) error) error {
	files, err := a.sessionFiles()
	if err != nil {
		return fmt.Errorf("cannot list gemini-cli storage: %v", err)
	}
	for _, path := range files {
		sum, err := a.scanFile(path, nil, nil)
		if err != nil {
			return err // with a nil emit this cannot fire today; propagation keeps listing honest if scanFile ever gains an error path
		}
		if !sum.sawLine {
			continue // empty (or blank) file: not a session
		}
		a.registerIDs(path, sum.sessionID, false)
		if err := iter(sum.sessionUsage(path)); err != nil {
			return err
		}
	}
	legacy, err := a.legacyFiles()
	if err != nil {
		return fmt.Errorf("cannot list gemini-cli storage: %v", err)
	}
	for _, path := range legacy {
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
	metaStart, metaEnd bool           // metadata startTime / lastUpdated seen: they win over record timestamps
	usage              agentlog.Usage // token totals accumulated over the records (M7)
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

// sessionUsage lifts the scan aggregate into the agentlog.SessionUsage view
// (M7): same session fields and message count as meta, plus the usage.
func (f fileSummary) sessionUsage(path string) agentlog.SessionUsage {
	meta := f.meta(path)
	return agentlog.SessionUsage{
		Session:  meta.Session,
		Messages: meta.Messages,
		Usage:    f.usage,
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
		entries, usage, ok := processRecord(line, &a.skipped)
		if !ok {
			a.skipped++
			continue
		}
		if usage.model != "" {
			sum.usage.Model = usage.model // last non-empty record model wins
		}
		if usage.tokens != nil {
			// input/output/cached/thoughts accumulate; `tool` and `total`
			// are recognized but ignored (SCHEMA.md)
			sum.usage.Input += ptrVal(usage.tokens.Input)
			sum.usage.Output += ptrVal(usage.tokens.Output)
			sum.usage.CacheRead += ptrVal(usage.tokens.Cached)
			sum.usage.Reasoning += ptrVal(usage.tokens.Thoughts)
		}
		if err := a.forward(entries, emit, &sum); err != nil {
			return sum, err
		}
	}
	if err := scanner.Err(); err != nil {
		// Two distinct failure kinds leave the rest of this file unreadable;
		// never fatal (spec §8 best effort):
		//   - bufio.ErrTooLong: a line beyond maxLineSize exhausted the
		//     buffer. The line itself is unreadable content — count exactly
		//     one skipped line per oversized file (SCHEMA.md).
		//   - anything else is an I/O read error: the file became unreadable
		//     mid-scan. Its lines are not corrupt — this is the "unreadable
		//     file" class, skipped silently like an unopenable file and not
		//     counted (SCHEMA.md).
		if errors.Is(err, bufio.ErrTooLong) {
			a.skipped++
		}
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

// sessionFiles lists all main and subagent JSONL paths in sorted order,
// main sessions first (session-*.jsonl directly under chats/), then any
// *.jsonl in chats/<parent-session-id>/ subdirectories — exactly the split
// order the lister had before the metacharacter fix (M4). It deliberately
// uses WalkDir instead of filepath.Glob: Glob silently matches nothing when
// the home path contains glob metacharacters ([, *, ?) — it does NOT error
// on them — while a directory walk is immune.
func (a *Adapter) sessionFiles() ([]string, error) {
	root := a.tmpDir()
	var main, sub []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return fs.SkipAll // storage absent: no sessions, not an error (spec §8)
			}
			return err // unreadable storage: surfaced, not silently truncated
		}
		depth := relDepth(root, path)
		if d.IsDir() {
			if depth >= 4 {
				return fs.SkipDir // session files live at most two levels below chats/
			}
			return nil
		}
		switch {
		case depth == 3 && filepath.Base(filepath.Dir(path)) == "chats" &&
			strings.HasPrefix(d.Name(), "session-") && strings.HasSuffix(d.Name(), ".jsonl"):
			main = append(main, path)
		case depth == 4 && filepath.Base(filepath.Dir(filepath.Dir(path))) == "chats" &&
			strings.HasSuffix(d.Name(), ".jsonl"):
			sub = append(sub, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return append(main, sub...), nil
}

// legacyFiles lists the monolithic chats.json paths, one per project dir,
// sorted for determinism — same WalkDir rationale as sessionFiles.
func (a *Adapter) legacyFiles() ([]string, error) {
	root := a.tmpDir()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return fs.SkipAll // storage absent: no sessions, not an error (spec §8)
			}
			return err // unreadable storage: surfaced, not silently truncated
		}
		if d.IsDir() {
			if relDepth(root, path) >= 3 {
				return fs.SkipDir // legacy stores sit directly in the project dir
			}
			return nil
		}
		if relDepth(root, path) == 2 && d.Name() == legacyName {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// relDepth counts the path components below root (0 for root itself).
func relDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// sessionFor locates the backing store of a session id through the lazily
// built id→store index: JSONL files register their filename-fallback id and
// metadata sessionId, legacy chats.json files register their session ids.
// The index is filled as a side effect of listing (walk / SessionsMetaFast)
// and built in one storage pass when Entries is called first (see
// registerIDs) — per-session rescans of every file were quadratic in the
// session count and missed the spec §7 budget. Files are visited in sorted
// order and the first registration of an id wins; JSONL ids are registered
// before legacy ids, matching the resolution order this method had before
// the index.
func (a *Adapter) sessionFor(id string) (sessionRef, bool) {
	if id == "" {
		return sessionRef{}, false
	}
	if a.index == nil {
		a.buildIndex()
	}
	ref, ok := a.index[id]
	return ref, ok
}

// registerIDs records one store's id spellings in the index.
func (a *Adapter) registerIDs(path, recordID string, legacy bool) {
	if a.index == nil {
		a.index = map[string]sessionRef{}
	}
	ref := sessionRef{path: path, legacy: legacy}
	if !legacy {
		base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if _, ok := a.index[base]; !ok {
			a.index[base] = ref
		}
	}
	if recordID != "" {
		if _, ok := a.index[recordID]; !ok {
			a.index[recordID] = ref
		}
	}
}

// buildIndex fills the id→store index when Entries is called without a
// preceding listing pass.
func (a *Adapter) buildIndex() {
	a.index = map[string]sessionRef{}
	files, err := a.sessionFiles()
	if err != nil {
		return
	}
	for _, path := range files {
		a.registerIDs(path, a.fileMetaID(path), false)
	}
	legacy, err := a.legacyFiles()
	if err != nil {
		return
	}
	for _, path := range legacy {
		for _, id := range a.legacySessionIDs(path) {
			a.registerIDs(path, id, true)
		}
	}
}

// fileMetaID returns the metadata record's sessionId (the first non-empty
// line), "" when the file opens with something else (defensive parse).
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
		meta, ok := parseMeta(line)
		if ok && meta.sessionID != "" {
			return meta.sessionID
		}
		return "" // only the first non-empty line can be the metadata record
	}
	return ""
}
