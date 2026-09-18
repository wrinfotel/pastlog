// Package render turns agentlog data into human tables and stable JSON
// output. JSON keys keep a fixed order (struct order) so schemas stay stable
// across milestones (spec §6).
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/wrinfotel/pastlog/internal/agentlog"
)

const (
	dateLayout  = "2006-01-02 15:04"
	idPrefixLen = 8
)

var nameColor = color.New(color.FgCyan)

// AgentRow is one line of `pastlog agents` output.
type AgentRow struct {
	Name     string
	Detected bool
	Path     string // absolute storage path; empty when not detected
	Sessions int
	Bytes    int64
}

// AgentsHuman writes the agents table: name, sessions, size, path (spec §3).
// Rows with absent storage show "(not found)" instead of size and path.
func AgentsHuman(w io.Writer, home string, rows []AgentRow) {
	nameW, countW := 0, 0
	for _, r := range rows {
		nameW = max(nameW, len(r.Name))
		countW = max(countW, len(fmt.Sprint(r.Sessions)))
	}
	for _, r := range rows {
		nameColor.Fprintf(w, "%-*s", nameW, r.Name)
		plural := "sessions"
		if r.Sessions == 1 {
			plural = "session"
		}
		// %-8s keeps following columns aligned for the singular form
		fmt.Fprintf(w, "  %*d %-8s  ", countW, r.Sessions, plural)
		if !r.Detected {
			fmt.Fprintln(w, "(not found)")
			continue
		}
		fmt.Fprintf(w, "%s  %s\n", HumanBytes(r.Bytes), tildePath(home, r.Path))
	}
}

type agentJSON struct {
	Name     string  `json:"name"`
	Detected bool    `json:"detected"`
	Path     *string `json:"path"`
	Sessions int     `json:"sessions"`
	Bytes    int64   `json:"bytes"`
}

// AgentsJSON writes the agents data as JSON with a stable schema.
func AgentsJSON(w io.Writer, rows []AgentRow) error {
	out := make([]agentJSON, len(rows))
	for i, r := range rows {
		row := agentJSON{Name: r.Name, Detected: r.Detected, Sessions: r.Sessions, Bytes: r.Bytes}
		if r.Detected {
			row.Path = &r.Path
		}
		out[i] = row
	}
	return writeJSON(w, out)
}

// SessionsHuman writes the sessions table, newest first (already sorted by
// the caller): agent, project, local date, messages, size, id prefix.
func SessionsHuman(w io.Writer, home string, rows []agentlog.SessionMeta) {
	agentW, projW, countW, sizeW := 0, 0, 0, 0
	for _, r := range rows { // column widths first, so every row pads alike
		agentW = max(agentW, len(r.Agent))
		projW = max(projW, len(displayProject(home, r.Project)))
		countW = max(countW, len(fmt.Sprint(r.Messages)))
		sizeW = max(sizeW, len(HumanBytes(r.SizeBytes)))
	}
	for _, r := range rows {
		plural := "messages"
		if r.Messages == 1 {
			plural = "message"
		}
		nameColor.Fprintf(w, "%-*s", agentW, r.Agent)
		fmt.Fprintf(w, "  %-*s  %s  %*d %-8s  %*s  %s\n",
			projW, displayProject(home, r.Project), displayDate(r.StartedAt),
			countW, r.Messages, plural, sizeW, HumanBytes(r.SizeBytes), idPrefix(r.ID))
	}
}

type sessionJSON struct {
	ID        string     `json:"id"`
	Agent     string     `json:"agent"`
	Project   string     `json:"project"`
	Title     string     `json:"title"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Messages  int        `json:"messages"`
	SizeBytes int64      `json:"size_bytes"`
}

// newSessionJSON maps a SessionMeta to its stable JSON shape; shared by the
// sessions list, search results and show.
func newSessionJSON(m agentlog.SessionMeta) sessionJSON {
	return sessionJSON{
		ID:        m.ID,
		Agent:     m.Agent,
		Project:   m.Project,
		Title:     m.Title,
		StartedAt: timePtr(m.StartedAt),
		EndedAt:   timePtr(m.EndedAt),
		Messages:  m.Messages,
		SizeBytes: m.SizeBytes,
	}
}

// SessionsJSON writes the sessions data as JSON with a stable schema; absent
// timestamps render as null.
func SessionsJSON(w io.Writer, rows []agentlog.SessionMeta) error {
	out := make([]sessionJSON, len(rows))
	for i, r := range rows {
		out[i] = newSessionJSON(r)
	}
	return writeJSON(w, out)
}

// kindJSON maps an entry kind to its stable JSON label.
func kindJSON(k agentlog.EntryKind) string {
	switch k {
	case agentlog.ToolCall:
		return "tool_call"
	case agentlog.ToolResult:
		return "tool_result"
	case agentlog.Summary:
		return "summary"
	default:
		return "message"
	}
}

// writeJSON emits indented JSON with a trailing newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func displayProject(home, project string) string {
	if project == "" {
		return "-"
	}
	return tildePath(home, project)
}

// displayDate renders a session timestamp in the local zone; zero times
// (records without any timestamp) show as "-".
func displayDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(dateLayout)
}

func idPrefix(id string) string {
	if len(id) <= idPrefixLen {
		return id
	}
	return id[:idPrefixLen]
}

// HumanBytes renders byte counts like "512 B" / "2.4 KB" / "1.4 GB".
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	val := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB", "PB"} {
		val /= unit
		if val < unit {
			return fmt.Sprintf("%.1f %s", val, u)
		}
	}
	return fmt.Sprintf("%.1f EB", val)
}

// tildePath shortens a path under the home directory to ~/... with forward
// slashes, keeping output identical across platforms. Both separators are
// accepted: agent records may use forward slashes on Windows too.
func tildePath(home, path string) string {
	if home == "" || path == "" {
		return path
	}
	h := filepath.ToSlash(strings.TrimSuffix(home, string(filepath.Separator)))
	p := filepath.ToSlash(path)
	if p == h {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, h+"/"); ok {
		return "~/" + rest
	}
	return path
}
