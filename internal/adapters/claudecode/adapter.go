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
	home    string
	skipped int               // unreadable/unknown lines, accumulated across scans (see type doc)
	index   map[string]string // session id -> file path, built lazily (see fileForSession)
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

// SessionsUsage implements agentlog.UsageSource (M7): token usage comes from
// the same streaming pass that yields sessions — message.usage fields
// accumulate per session (input/output/cache read/cache write), Model is the
// LAST non-empty message.model in the file. Usage of the wrong shape
// contributes zero and keeps the line readable (SCHEMA.md). claude-code
// records carry no per-session cost: HasCost stays false.
func (a *Adapter) SessionsUsage(iter func(agentlog.SessionUsage) error) error {
	return a.walkSummaries(func(sum fileSummary, path string) error {
		meta := sum.meta(path)
		return iter(agentlog.SessionUsage{
			Session:  meta.Session,
			Messages: meta.Messages,
			Usage:    sum.usage(),
			Models:   sum.split.Split(),
		})
	})
}

// SessionsMetaFast implements agentlog.FastMetaSource: the search flow lists
// sessions from the FIRST record line of each file plus a stat, instead of
// parsing every line (spec §7: listing must not dominate a search run).
// Line 1 carries sessionId, cwd and timestamp in practice; when it does not,
// the file falls back to the full parse so ids, projects, start timestamps,
// filters and sort order stay identical to SessionsMeta. Fields beyond line 1
// (last timestamp, message counts, summary titles) are zero on this path.
func (a *Adapter) SessionsMetaFast(iter func(agentlog.SessionMeta) error) (bool, error) {
	files, err := a.sessionFiles()
	if err != nil {
		return true, fmt.Errorf("cannot list claude-code storage: %v", err)
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
			a.registerIDs(path, sum.sessionID)
			m = sum.meta(path)
		} else {
			a.registerIDs(path, m.ID)
		}
		if err := iter(m); err != nil {
			return true, err
		}
	}
	return true, nil
}

// fastMeta builds session metadata from the file's first non-empty line plus
// a stat. fast=false when that line is not a classified record carrying all
// of sessionId, cwd and timestamp — the caller then does a full parse.
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
	for scanner.Scan() { // first non-empty line is the first record
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		res := processLine(line)
		if !res.ok || res.info.sessionID == "" || res.info.cwd == "" || res.info.ts.IsZero() {
			return agentlog.SessionMeta{}, false
		}
		return agentlog.SessionMeta{
			Session: agentlog.Session{
				ID:        res.info.sessionID,
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
	_, err := a.scanFile(path, keep, iter)
	return err
}

// walk streams every session file, in sorted path order, through iter.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	return a.walkSummaries(func(sum fileSummary, path string) error {
		return iter(sum.meta(path))
	})
}

// walkSummaries streams every session file's aggregate through iter — the
// one shared storage walk behind Sessions, SessionsMeta (meta view) and
// SessionsUsage (usage view), so all listing passes scan the files once, in
// the same sorted order, and skip/usage accounting stays identical.
func (a *Adapter) walkSummaries(iter func(sum fileSummary, path string) error) error {
	files, err := a.sessionFiles()
	if err != nil {
		return fmt.Errorf("cannot list claude-code storage: %v", err)
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
		if err := iter(sum, path); err != nil {
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
	split              agentlog.ModelSplit // per-model usage accumulation (M7, TASK.md backlog)
}

// usage renders the session's totals: every observed record summed, with
// Model = the last non-empty message.model (pre-split semantics preserved).
func (f fileSummary) usage() agentlog.Usage {
	u := f.split.Total()
	u.Model = f.split.Latest()
	return u
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
		res := processLine(line)
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
		// TASK.md backlog: each record's usage lands on its message.model, so
		// tokens from mid-session model switches stay with the model that
		// consumed them; a record without a model keeps the model in effect
		if u := info.usage; u != nil {
			sum.split.Observe(info.model, agentlog.Usage{
				Input:      ptrVal(u.InputTokens),
				Output:     ptrVal(u.OutputTokens),
				CacheWrite: ptrVal(u.CacheCreationInputTokens),
				CacheRead:  ptrVal(u.CacheReadInputTokens),
			})
		} else if info.model != "" {
			sum.split.Note(info.model) // last non-empty message.model wins
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
			if path == root && os.IsNotExist(err) {
				return fs.SkipAll // storage absent: no sessions, not an error (spec §8)
			}
			return err // unreadable storage: surfaced, not silently truncated
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

// registerIDs records a file's id spellings — filename minus .jsonl and the
// first sessionId seen in the records — in the index. Files are visited in
// sorted order and the first registration of an id wins.
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
		a.registerIDs(path, a.fileSessionID(path))
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
