// Package antigravity reads the local session store of Google Antigravity CLI.
package antigravity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

const name = "antigravity"

type Adapter struct {
	root    string
	skipped int
}

func New(home string) *Adapter {
	return &Adapter{root: filepath.Join(home, ".gemini", "antigravity-cli")}
}

func (a *Adapter) Name() string        { return name }
func (a *Adapter) StoragePath() string { return a.root }
func (a *Adapter) SkippedLines() int   { return a.skipped }

func (a *Adapter) Detect() bool {
	info, err := os.Stat(a.root)
	return err == nil && info.IsDir()
}

func (a *Adapter) Sessions(iter func(agentlog.Session) error) error {
	return a.SessionsMeta(func(m agentlog.SessionMeta) error { return iter(m.Session) })
}

func (a *Adapter) SessionsMeta(iter func(agentlog.SessionMeta) error) error {
	return a.walk(false, func(s agentlog.SessionUsage) error {
		return iter(agentlog.SessionMeta{Session: s.Session, Messages: s.Messages})
	})
}

func (a *Adapter) SessionsUsage(iter func(agentlog.SessionUsage) error) error {
	return a.walk(true, iter)
}

func (a *Adapter) walk(withUsage bool, iter func(agentlog.SessionUsage) error) error {
	ids, err := a.sessionIDs()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	parents := make(map[string]string)
	metadata := a.history()
	sessions := make([]agentlog.SessionUsage, 0, len(ids))
	for _, id := range ids {
		row := agentlog.SessionUsage{Session: agentlog.Session{ID: id, Agent: name}}
		var split agentlog.ModelSplit
		path, size := a.transcript(id)
		row.SizeBytes = size
		if path == "" {
			if info, err := os.Stat(a.databasePath(id)); err == nil {
				row.SizeBytes = info.Size()
			}
		}
		if err := a.readSteps(id, path, true, func(s step) error {
			at := s.timestamp()
			if !at.IsZero() && (row.StartedAt.IsZero() || at.Before(row.StartedAt)) {
				row.StartedAt = at
			}
			if at.After(row.EndedAt) {
				row.EndedAt = at
			}
			if row.Project == "" {
				row.Project = s.CWD
			}
			if row.Title == "" && s.Type == "USER_INPUT" {
				row.Title = shortTitle(userText(s.Content))
			}
			if role := s.messageRole(); role != "" && s.messageText() != "" {
				row.Messages++
			}
			for _, child := range s.children() {
				if child != id {
					parents[child] = id
				}
			}
			if withUsage && s.Type == "PLANNER_RESPONSE" {
				if u := s.usage(); u.Input != 0 || u.Output != 0 || u.Reasoning != 0 {
					split.Observe(s.Model, u)
				} else {
					split.Note(s.Model)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		meta := metadata[id]
		if meta.CWD != "" {
			row.Project = meta.CWD
		}
		if meta.Title != "" {
			row.Title = shortTitle(meta.Title)
		}
		if row.StartedAt.IsZero() {
			row.StartedAt = meta.At
			row.EndedAt = meta.At
		}
		if withUsage {
			if dbSplit, ok := databaseUsage(a.databasePath(id)); ok {
				split = dbSplit
			}
			row.Usage = split.Total()
			row.Model = split.Latest()
			row.Models = split.Split()
		}
		sessions = append(sessions, row)
	}
	for _, row := range sessions {
		row.ParentID = parents[row.ID]
		if err := iter(row); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) sessionIDs() ([]string, error) {
	brain, err := os.ReadDir(filepath.Join(a.root, "brain"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("cannot list Antigravity transcripts: %w", err)
	}
	ids := make([]string, 0, len(brain))
	seen := make(map[string]bool, len(brain))
	for _, dir := range brain {
		if dir.IsDir() {
			if path, _ := a.transcript(dir.Name()); path != "" {
				ids = append(ids, dir.Name())
				seen[dir.Name()] = true
			}
		}
	}
	files, err := os.ReadDir(filepath.Join(a.root, "conversations"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("cannot list Antigravity databases: %w", err)
	}
	for _, file := range files {
		if id, ok := strings.CutSuffix(file.Name(), ".db"); ok && file.Type().IsRegular() && id != "" && !seen[id] {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (a *Adapter) transcript(id string) (string, int64) {
	logs := filepath.Join(a.root, "brain", id, ".system_generated", "logs")
	for _, file := range []string{"transcript_full.jsonl", "transcript.jsonl"} {
		path := filepath.Join(logs, file)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return path, info.Size()
		}
	}
	return "", 0
}

func (a *Adapter) databasePath(id string) string {
	return filepath.Join(a.root, "conversations", id+".db")
}

func (a *Adapter) find(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\:`) {
		return "", fmt.Errorf("invalid Antigravity session ID %q", id)
	}
	if path, _ := a.transcript(id); path != "" {
		return path, nil
	}
	if info, err := os.Stat(a.databasePath(id)); err == nil && info.Mode().IsRegular() {
		return "", nil
	}
	return "", fmt.Errorf("session %s not found in Antigravity storage", id)
}

func (a *Adapter) Entries(session agentlog.Session, iter func(agentlog.Entry) error) error {
	path, err := a.find(session.ID)
	if err != nil {
		return err
	}
	return a.readSteps(session.ID, path, true, func(s step) error { return s.entries(iter) })
}

func (a *Adapter) readSteps(id, path string, countSkips bool, visit func(step) error) error {
	if path == "" {
		return readDatabaseSteps(a.databasePath(id), visit)
	}
	return scanLines(path, func(line []byte) error {
		var s step
		if json.Unmarshal(line, &s) != nil || s.Type == "" {
			if countSkips {
				a.skipped++
			}
			return nil
		}
		return visit(s)
	})
}

func scanLines(path string, visit func([]byte) error) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // a running CLI may remove a session between listing and reading
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scan.Scan() {
		if line := bytes.TrimSpace(scan.Bytes()); len(line) != 0 {
			if err := visit(line); err != nil {
				return err
			}
		}
	}
	return scan.Err()
}

type historyInfo struct {
	CWD, Title string
	At         time.Time
}

func (a *Adapter) history() map[string]historyInfo {
	result := make(map[string]historyInfo)
	// History is supplemental. A missing or partially written history line
	// does not prevent opening the conversation itself.
	_ = scanLines(filepath.Join(a.root, "history.jsonl"), func(line []byte) error { //nolint:errcheck // optional metadata
		var rec struct {
			ID        string          `json:"conversationId"`
			Workspace string          `json:"workspace"`
			Display   string          `json:"display"`
			Timestamp json.RawMessage `json:"timestamp"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.ID == "" {
			return nil
		}
		info := result[rec.ID]
		if info.Title == "" {
			info.Title = rec.Display
		}
		if rec.Workspace != "" {
			info.CWD = rec.Workspace
		}
		if info.At.IsZero() {
			timestamp := strings.Trim(string(rec.Timestamp), `"`)
			info.At = parseTime(timestamp)
			if ms, err := strconv.ParseInt(timestamp, 10, 64); err == nil && ms > 0 {
				info.At = time.UnixMilli(ms)
			}
		}
		result[rec.ID] = info
		return nil
	})
	var knownPaths map[string]string
	if data, err := os.ReadFile(filepath.Join(a.root, "cache", "last_conversations.json")); err == nil && json.Unmarshal(data, &knownPaths) == nil {
		for _, path := range slices.Sorted(maps.Keys(knownPaths)) {
			id := knownPaths[path]
			info := result[id]
			if id != "" && info.CWD == "" {
				info.CWD = path
				result[id] = info
			}
		}
	}
	return result
}

func shortTitle(text string) string {
	words := strings.Join(strings.Fields(text), " ")
	runes := []rune(words)
	if len(runes) > 80 {
		return string(runes[:79]) + "…"
	}
	return words
}
