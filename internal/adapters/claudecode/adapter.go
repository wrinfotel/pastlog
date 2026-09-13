// Package claudecode adapts Claude Code session logs: JSONL files under
// ~/.claude/projects/<escaped-cwd>/<session-uuid>.jsonl (see SCHEMA.md).
// Parsing is defensive and strictly read-only; files are streamed line by
// line, never loaded whole.
package claudecode

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
	agentName   = "claude-code"
	initialBuf  = 64 * 1024
	maxLineSize = 16 * 1024 * 1024
)

// Adapter reads Claude Code JSONL session logs from <home>/.claude/projects.
//
// Skipped-counter semantics (spec §8): the unreadable-line counter accumulates
// across every scan the adapter performs (listing, per-session entries, the
// search prefilter pass) and is never reset — report it after the runs you
// want it to cover, on a freshly built adapter. Because of that shared
// counter, an Adapter is NOT safe for concurrent scans: use one Adapter per
// goroutine. Reading SkippedLines concurrently with a scan races; read it
// after scans finish.
type Adapter struct {
	home    string
	skipped int // unreadable/unknown lines, accumulated across scans (see type doc)
}

// New builds an adapter rooted at the given user home directory.
func New(home string) *Adapter { return &Adapter{home: home} }

func (a *Adapter) Name() string { return agentName }

func (a *Adapter) Detect() bool {
	info, err := os.Stat(a.projectsDir())
	return err == nil && info.IsDir()
}

// SkippedLines implements agentlog.SkipCounter (spec §8 stderr summary).
// The total accumulates across scans and is never reset — see the Adapter
// type documentation for the exact semantics.
func (a *Adapter) SkippedLines() int { return a.skipped }

// StoragePath implements agentlog.PathSource: the directory scanned for
// sessions, for display in `agents`.
func (a *Adapter) StoragePath() string { return a.projectsDir() }

func (a *Adapter) projectsDir() string {
	return filepath.Join(a.home, ".claude", "projects")
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
		return fmt.Errorf("session %s not found in claude-code storage", s.ID)
	}
	_, err := a.scanFile(path, keep, iter)
	return err
}

// walk streams every session file, in sorted path order, through iter.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	files, err := a.sessionFiles()
	if err != nil {
		return fmt.Errorf("cannot list claude-code storage: %v", err)
	}
	for _, path := range files {
		sum, _ := a.scanFile(path, nil, nil)
		if !sum.sawLine {
			continue // empty (or blank) file: not a session
		}
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

// id prefers the records' sessionId field; the filename is the fallback.
func (f fileSummary) id(path string) string {
	if f.sessionID != "" {
		return f.sessionID
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// scanFile streams one JSONL file. keep may be nil (keep every line); when
// set, lines failing it are skipped before parsing (search prefilter).
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
		if info.summary != "" && sum.title == "" {
			sum.title = info.summary
		}
		if !info.ts.IsZero() {
			if sum.startedAt.IsZero() {
				sum.startedAt = info.ts
			}
			sum.endedAt = info.ts
		}
		for _, e := range entries {
			if e.Kind == agentlog.Message {
				sum.messages++
			}
			if emit != nil {
				if err := emit(e); err != nil {
					return sum, err
				}
			}
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

// sessionFiles lists all session JSONL paths, sorted for determinism
// (WalkDir visits entries lexically, and files sort inside their directory).
// It deliberately uses WalkDir instead of filepath.Glob: Glob silently
// matches nothing when the home path contains glob metacharacters ([, *, ?),
// while a directory walk is immune (M4). Only *.jsonl files directly inside
// project directories are considered (SCHEMA.md) — files directly under
// projects/ or nested deeper are ignored, exactly like the old
// projects/*/*.jsonl pattern.
func (a *Adapter) sessionFiles() ([]string, error) {
	root := a.projectsDir()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // unreadable subtree: surfaced, not silently truncated
		}
		if d.IsDir() {
			if path != root && filepath.Dir(path) != root {
				return fs.SkipDir // sessions live exactly one level deep
			}
			return nil
		}
		if filepath.Dir(path) == root {
			return nil // stray file directly under projects/: not a session
		}
		if strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// fileForSession locates the JSONL file backing a session: first by filename
// (the common case), then by scanning records for the sessionId field — the
// two may differ (SCHEMA.md). Filename matching goes through the WalkDir
// lister, so homes containing glob metacharacters resolve correctly too.
func (a *Adapter) fileForSession(id string) (string, bool) {
	files, err := a.sessionFiles()
	if err != nil {
		return "", false
	}
	if id != "" {
		want := id + ".jsonl"
		for _, path := range files {
			if filepath.Base(path) == want {
				return path, true
			}
		}
	}
	for _, path := range files {
		if a.fileHasSessionID(path, id) {
			return path, true
		}
	}
	return "", false
}

func (a *Adapter) fileHasSessionID(path, id string) bool {
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
		info, _, ok := processLine(line)
		if ok && info.sessionID == id {
			return true
		}
	}
	return false
}
