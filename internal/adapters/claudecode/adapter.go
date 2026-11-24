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

	"github.com/wrinfotel/pastlog/internal/agentlog"
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
	dir     string
	skipped int               // unreadable/unknown lines, accumulated across scans (see type doc)
	index   map[string]string // session id -> file path, built lazily (see fileForSession)
}

// New builds an adapter rooted at the given user home directory.
func New(home string) *Adapter { return NewProjectsDir(filepath.Join(home, ".claude", "projects")) }

// NewProjectsDir uses an explicit Claude projects directory.
func NewProjectsDir(dir string) *Adapter { return &Adapter{dir: dir} }

func (a *Adapter) Name() string { return agentName }

func (a *Adapter) Detect() bool {
	info, err := os.Stat(a.dir)
	return err == nil && info.IsDir()
}

// SkippedLines implements agentlog.SkipCounter (spec §8 stderr summary).
// The total accumulates across scans and is never reset — see the Adapter
// type documentation for the exact semantics.
func (a *Adapter) SkippedLines() int { return a.skipped }

// StoragePath implements agentlog.PathSource: the directory scanned for
// sessions, for display in `agents`.
func (a *Adapter) StoragePath() string { return a.dir }

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
// the same pass that yields sessions. Blocks sharing a response ID update one
// usage snapshot. The last non-empty model remains the session model.
func (a *Adapter) SessionsUsage(iter func(agentlog.SessionUsage) error) error {
	return a.walkSummaries(true, func(meta agentlog.SessionMeta, split agentlog.ModelSplit) error {
		usage := split.Total()
		usage.Model = split.Latest()
		return iter(agentlog.SessionUsage{
			Session:  meta.Session,
			Messages: meta.Messages,
			Usage:    usage,
			Models:   split.Split(),
		})
	})
}

// SessionsMetaFast implements agentlog.FastMetaSource: the search flow lists
// sessions from the FIRST record line of each file plus a stat, instead of
// parsing every line. Leading bookkeeping is skipped. If the first
// transcript record lacks required metadata, the full parse provides it.
func (a *Adapter) SessionsMetaFast(iter func(agentlog.SessionMeta) error) (bool, error) {
	files, err := a.sessionFiles()
	if err != nil {
		return true, fmt.Errorf("cannot list claude-code storage: %v", err)
	}
	parents := make(map[string]agentlog.Session)
	for _, file := range files {
		m, fast := a.fastMeta(file)
		if !fast {
			sum, err := a.scanFile(file.path, nil, nil, false)
			if err != nil {
				return true, err
			}
			if !sum.sawLine {
				continue // empty (or blank) file: not a session
			}
			m = sum.meta(file)
		}
		a.linkSession(file, &m, parents)
		if err := iter(m); err != nil {
			return true, err
		}
	}
	return true, nil
}

// fastMeta builds session metadata from the file's first non-empty line plus
// a stat. fast=false when that line is not a classified record carrying all
// of sessionId, cwd and timestamp — the caller then does a full parse.
func (a *Adapter) fastMeta(file sessionFile) (agentlog.SessionMeta, bool) {
	f, err := os.Open(file.path)
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
	for scanner.Scan() { // first non-empty line is the first record
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		res := processLine(line)
		if res.ignored {
			continue
		}
		if !res.ok || (res.info.sessionID == "" && file.parentPath == "") || res.info.cwd == "" || res.info.ts.IsZero() {
			return agentlog.SessionMeta{}, false
		}
		return agentlog.SessionMeta{
			Session: agentlog.Session{
				ID:        file.id(res.info.sessionID),
				Agent:     agentName,
				Project:   res.info.cwd,
				StartedAt: res.info.ts,
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
	_, err := a.scanFile(path, keep, iter, false)
	return err
}

// walk streams every session file, in sorted path order, through iter.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	return a.walkSummaries(false, func(meta agentlog.SessionMeta, _ agentlog.ModelSplit) error {
		return iter(meta)
	})
}

// walkSummaries streams every session file's aggregate through iter — the
// one shared storage walk behind Sessions, SessionsMeta (meta view) and
// SessionsUsage (usage view), so all listing passes scan the files once, in
// the same sorted order, and skip/usage accounting stays identical.
func (a *Adapter) walkSummaries(withUsage bool, iter func(agentlog.SessionMeta, agentlog.ModelSplit) error) error {
	files, err := a.sessionFiles()
	if err != nil {
		return fmt.Errorf("cannot list claude-code storage: %v", err)
	}
	parents := make(map[string]agentlog.Session)
	for _, file := range files {
		sum, err := a.scanFile(file.path, nil, nil, withUsage)
		if err != nil {
			return err // with a nil emit this cannot fire today; propagation keeps listing honest if scanFile ever gains an error path
		}
		if !sum.sawLine {
			continue // empty (or blank) file: not a session
		}
		meta := sum.meta(file)
		a.linkSession(file, &meta, parents)
		if err := iter(meta, sum.split); err != nil {
			return err
		}
	}
	return nil
}

// linkSession uses the parent's canonical ID, which can differ from its
// filename. A child without cwd inherits its parent's project.
func (a *Adapter) linkSession(file sessionFile, meta *agentlog.SessionMeta, parents map[string]agentlog.Session) {
	a.registerIDs(file, meta.ID)
	if file.parentPath == "" {
		parents[file.path] = meta.Session
		return
	}
	parent, found := parents[file.parentPath]
	meta.ParentID = file.parentStem()
	if found {
		meta.ParentID = parent.ID
		if meta.Project == "" {
			meta.Project = parent.Project
		}
	}
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
	split              agentlog.ModelSplit
}

func (f fileSummary) meta(file sessionFile) agentlog.SessionMeta {
	return agentlog.SessionMeta{
		Session: agentlog.Session{
			ID:        file.id(f.sessionID),
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

// scanFile streams one JSONL file. keep may be nil (keep every line); when
// set, lines failing it are skipped before parsing (search prefilter).
// emit may be nil (listing mode); when set, every classified entry is
// forwarded and a non-nil error aborts the scan (early cutoff). Unreadable
// lines increment the adapter-wide skipped counter. Never panics on
// malformed input.
func (a *Adapter) scanFile(path string, keep func([]byte) bool, emit func(agentlog.Entry) error, withUsage bool) (fileSummary, error) {
	var sum fileSummary
	var usage sessionUsage

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
		res := processLine(line)
		if res.ignored {
			continue
		}
		if !res.ok {
			a.skipped++
			continue
		}
		if res.skip {
			// hybrid: the line was readable, but a malformed element
			// truncated its content array — emit what parsed, count the line
			// (SCHEMA.md "hybrid semantics")
			a.skipped++
		}
		info, entries := res.info, res.entries
		if info.sessionID != "" && sum.sessionID == "" {
			sum.sessionID = info.sessionID
		}
		if info.cwd != "" && sum.cwd == "" {
			sum.cwd = info.cwd
		}
		if info.summary != "" && sum.title == "" {
			sum.title = info.summary
		}
		if withUsage {
			usage.observe(info)
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
	if withUsage {
		sum.split = usage.split()
	}
	return sum, nil
}

type sessionFile struct {
	path       string
	parentPath string // sibling transcript of a subagent's containing directory
}

func (f sessionFile) parentStem() string {
	return strings.TrimSuffix(filepath.Base(f.parentPath), ".jsonl")
}

func (f sessionFile) id(recordID string) string {
	stem := strings.TrimSuffix(filepath.Base(f.path), ".jsonl")
	if f.parentPath != "" {
		// Some Claude versions reuse the parent's sessionId. The filename
		// identifies the child, scoped by the containing parent session.
		return stem + "@" + f.parentStem()
	}
	if recordID != "" {
		return recordID
	}
	return stem
}

// sessionFiles walks the two transcript layouts and skips other sidecars.
// Main sessions are returned first so each child can resolve its parent.
func (a *Adapter) sessionFiles() ([]sessionFile, error) {
	root := a.dir
	var main, children []sessionFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return fs.SkipAll // storage absent: no sessions, not an error (spec §8)
			}
			return err // unreadable storage: surfaced, not silently truncated
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if d.IsDir() {
			if len(parts) > 3 || (len(parts) == 3 && parts[2] != "subagents") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		switch {
		case len(parts) == 2:
			main = append(main, sessionFile{path: path})
		case len(parts) == 4 && parts[2] == "subagents" && strings.HasPrefix(d.Name(), "agent-"):
			parentPath := filepath.Join(root, parts[0], parts[1]+".jsonl")
			children = append(children, sessionFile{path: path, parentPath: parentPath})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return append(main, children...), nil
}

// fileForSession locates the JSONL file backing a session: first by filename
// (the common case), then by the records' sessionId field — the two may
// differ (SCHEMA.md).
//
// The id→path index is built lazily (see registerIDs): ONE pass over the
// storage, instead of a per-session directory walk plus a per-session rescan
// of every candidate file (which made per-session Entries quadratic in the
// session count — surfaced by the M4 benchmark). Sessions obtained from
// listing resolve through the index that listing already filled; both id
// spellings listing assigns (filename fallback and first-seen sessionId) are
// indexed.
func (a *Adapter) fileForSession(id string) (string, bool) {
	if a.index == nil {
		a.buildIndex()
	}
	path, ok := a.index[id]
	return path, ok
}

// registerIDs indexes the canonical ID and the filename fallback. The first
// registration wins when copied transcripts share a main session ID.
func (a *Adapter) registerIDs(file sessionFile, id string) {
	if a.index == nil {
		a.index = map[string]string{}
	}
	for _, key := range []string{file.id(""), id} {
		if _, ok := a.index[key]; !ok {
			a.index[key] = file.path
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
	for _, file := range files {
		id := file.id("")
		if file.parentPath == "" {
			id = file.id(a.fileSessionID(file.path))
		}
		a.registerIDs(file, id)
	}
}

// fileSessionID returns the first sessionId field found in the file, "" when
// none (defensive parse). This mirrors the id walk() assigns to the session.
func (a *Adapter) fileSessionID(path string) string {
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
		res := processLine(line)
		if res.ok && res.info.sessionID != "" {
			return res.info.sessionID
		}
	}
	return ""
}
