// Package claudecode adapts Claude Code session logs: JSONL files under
// ~/.claude/projects/<escaped-cwd>/<session-uuid>.jsonl (see SCHEMA.md).
// Parsing is defensive and strictly read-only; files are streamed line by
// line, never loaded whole.
package claudecode

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
	agentName   = "claude-code"
	initialBuf  = 64 * 1024
	maxLineSize = 16 * 1024 * 1024
)

// Adapter reads Claude Code JSONL session logs from <home>/.claude/projects.
type Adapter struct {
	home    string
	skipped int // unreadable/unknown lines, counted across scans
}

// New builds an adapter rooted at the given user home directory.
func New(home string) *Adapter { return &Adapter{home: home} }

func (a *Adapter) Name() string { return agentName }

func (a *Adapter) Detect() bool {
	info, err := os.Stat(a.projectsDir())
	return err == nil && info.IsDir()
}

// SkippedLines implements agentlog.SkipCounter (spec §8 stderr summary).
func (a *Adapter) SkippedLines() int { return a.skipped }

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
	path, found := a.fileForSession(s.ID)
	if !found {
		return fmt.Errorf("session %s not found in claude-code storage", s.ID)
	}
	_, err := a.scanFile(path, iter)
	return err
}

// walk streams every session file, in sorted path order, through iter.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	files, err := a.sessionFiles()
	if err != nil {
		return fmt.Errorf("cannot list claude-code storage: %v", err)
	}
	for _, path := range files {
		sum, _ := a.scanFile(path, nil)
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

// scanFile streams one JSONL file. emit may be nil (listing mode); when set,
// every classified entry is forwarded and a non-nil error aborts the scan
// (early cutoff). Unreadable lines increment the adapter-wide skipped
// counter. Never panics on malformed input.
func (a *Adapter) scanFile(path string, emit func(agentlog.Entry) error) (fileSummary, error) {
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
		// A line beyond maxLineSize exhausts the buffer; the rest of this
		// file is unreadable to us. Count it and move on — never fatal.
		a.skipped++
	}
	return sum, nil
}

// sessionFiles lists all session JSONL paths, sorted for determinism.
func (a *Adapter) sessionFiles() ([]string, error) {
	return filepath.Glob(filepath.Join(a.projectsDir(), "*", "*.jsonl"))
}

// fileForSession locates the JSONL file backing a session: first by filename
// (the common case), then by scanning records for the sessionId field — the
// two may differ (SCHEMA.md).
func (a *Adapter) fileForSession(id string) (string, bool) {
	if id != "" {
		matches, err := filepath.Glob(filepath.Join(a.projectsDir(), "*", id+".jsonl"))
		if err == nil && len(matches) > 0 {
			return matches[0], true
		}
	}
	files, err := a.sessionFiles()
	if err != nil {
		return "", false
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
