package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

const showTimeLayout = "2006-01-02 15:04:05"

// ShowHuman writes one session as a readable transcript: a title line, a
// metadata line, then one line per entry with timestamp and role; multi-line
// texts continue indented to the text column. The role column width comes
// from a pre-pass over the entries (the M1 renderer pattern), so every row
// pads alike.
func ShowHuman(w io.Writer, home string, meta agentlog.SessionMeta, entries []agentlog.Entry) {
	fmt.Fprintf(w, "# %s\n", showTitle(meta))

	header := []string{meta.Agent, meta.ID, displayProject(home, meta.Project)}
	if meta.StartedAt.IsZero() {
		header = append(header, "-")
	} else {
		span := meta.StartedAt.Local().Format(showTimeLayout)
		if !meta.EndedAt.IsZero() {
			span += " → " + meta.EndedAt.Local().Format(showTimeLayout)
		}
		header = append(header, span)
	}
	header = append(header, countLabel(meta.Messages, "message", "messages"), HumanBytes(meta.SizeBytes))
	fmt.Fprintf(w, "%s\n\n", strings.Join(header, " · "))

	labels := make([]string, len(entries))
	labelW := 0
	for i, e := range entries {
		if e.Text == "" {
			continue
		}
		labels[i] = entryLabel(e)
		labelW = max(labelW, len(labels[i]))
	}
	pad := strings.Repeat(" ", len("00:00:00")+2+labelW+2)
	for i, e := range entries {
		if e.Text == "" {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(e.Text, "\r\n", "\n"), "\n")
		fmt.Fprintf(w, "%8s  %-*s  %s\n", entryTime(e.Timestamp), labelW, labels[i], lines[0])
		for _, ln := range lines[1:] {
			fmt.Fprintf(w, "%s%s\n", pad, ln)
		}
	}
}

// ShowMarkdown writes one session as markdown (for `show --export md`; the
// user redirects stdout into a file). Conversation texts are exported
// verbatim — best effort, not re-wrapped.
func ShowMarkdown(w io.Writer, meta agentlog.SessionMeta, entries []agentlog.Entry) {
	fmt.Fprintf(w, "# %s\n\n", showTitle(meta))
	fmt.Fprintf(w, "- **agent:** %s\n", meta.Agent)
	fmt.Fprintf(w, "- **session:** %s\n", meta.ID)
	if meta.Project != "" {
		fmt.Fprintf(w, "- **project:** %s\n", meta.Project)
	}
	if !meta.StartedAt.IsZero() {
		fmt.Fprintf(w, "- **started:** %s\n", meta.StartedAt.Local().Format(showTimeLayout))
	}
	if !meta.EndedAt.IsZero() {
		fmt.Fprintf(w, "- **ended:** %s\n", meta.EndedAt.Local().Format(showTimeLayout))
	}
	fmt.Fprintf(w, "- **messages:** %d\n", meta.Messages)
	fmt.Fprintf(w, "- **size:** %s\n\n", HumanBytes(meta.SizeBytes))
	for _, e := range entries {
		if e.Text == "" {
			continue
		}
		head := entryLabel(e)
		if !e.Timestamp.IsZero() {
			head += " · " + e.Timestamp.Local().Format(showTimeLayout)
		}
		fmt.Fprintf(w, "## %s\n\n%s\n\n", head, strings.ReplaceAll(e.Text, "\r\n", "\n"))
	}
}

// ShowJSON writes one session with its entries as JSON with a stable schema.
func ShowJSON(w io.Writer, meta agentlog.SessionMeta, entries []agentlog.Entry) error {
	return writeJSON(w, NewShowDoc(meta, entries))
}

// showTitle is the transcript heading: the session title, or a session
// placeholder when the format carries no title.
func showTitle(meta agentlog.SessionMeta) string {
	if meta.Title != "" {
		return meta.Title
	}
	return "session " + idPrefix(meta.ID)
}

// entryLabel picks the transcript role label of an entry: the role for
// messages, a kind label for tool activity and summaries.
func entryLabel(e agentlog.Entry) string {
	switch e.Kind {
	case agentlog.ToolCall:
		return "tool call"
	case agentlog.ToolResult:
		return "tool result"
	case agentlog.Summary:
		return "summary"
	default:
		if e.Role != "" {
			return e.Role
		}
		return "message"
	}
}

// entryTime renders an entry timestamp as local wall-clock time; zero times
// show "-".
func entryTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("15:04:05")
}

// countLabel renders a count with the right plural form, padded so following
// columns stay aligned for the singular.
func countLabel(n int, singular, plural string) string {
	word := plural
	if n == 1 {
		word = singular
	}
	return fmt.Sprintf("%d %s", n, word)
}
