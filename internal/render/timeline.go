package render

import (
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// timelineTimeLayout is the full wall-clock stamp of a timeline row: rows
// can span days, so the date is always shown.
const timelineTimeLayout = "2006-01-02 15:04:05"

var timelineAgentColor = color.New(color.FgCyan)

// TimelineSessionsHuman writes the chronological session stream, oldest
// first: started, agent, project, duration, messages, size, id prefix. It is
// the sessions table read as a timeline — time leads the row.
func TimelineSessionsHuman(w io.Writer, home string, rows []agentlog.SessionMeta) {
	agentW, projW, countW, sizeW, durW := 0, 0, 0, 0, 0
	for _, r := range rows {
		agentW = max(agentW, len(r.Agent))
		projW = max(projW, len(displayProject(home, r.Project)))
		countW = max(countW, len(fmt.Sprint(r.Messages)))
		sizeW = max(sizeW, len(HumanBytes(r.SizeBytes)))
		durW = max(durW, len(displayDuration(r.StartedAt, r.EndedAt)))
	}
	for _, r := range rows {
		plural := "messages"
		if r.Messages == 1 {
			plural = "message"
		}
		timelineAgentColor.Fprintf(w, "%s  %-*s", displayStamp(r.StartedAt), agentW, r.Agent)
		fmt.Fprintf(w, "  %-*s  %-*s  %*d %-8s  %*s  %s\n",
			projW, displayProject(home, r.Project), durW, displayDuration(r.StartedAt, r.EndedAt),
			countW, r.Messages, plural, sizeW, HumanBytes(r.SizeBytes), idPrefix(r.ID))
	}
}

// TimelineEventsHuman writes the merged message stream of many sessions:
// time, agent, role, snippet, session id prefix.
func TimelineEventsHuman(w io.Writer, events []agentlog.TimelineEvent) {
	agentW, roleW, textW := 0, 0, 0
	for _, e := range events {
		agentW = max(agentW, len(e.Agent))
		roleW = max(roleW, len(roleLabel(e.Role)))
		textW = max(textW, len(e.Text))
	}
	for _, e := range events {
		timelineAgentColor.Fprintf(w, "%s  %-*s", displayStampFull(e.Timestamp), agentW, e.Agent)
		fmt.Fprintf(w, "  %-*s  %-*s  %s\n", roleW, roleLabel(e.Role), textW, e.Text, idPrefix(e.SessionID))
	}
}

// TimelineSessionsJSON writes the chronological session stream with the
// sessions schema.
func TimelineSessionsJSON(w io.Writer, rows []agentlog.SessionMeta) error {
	return writeJSON(w, SessionRowsJSON(rows))
}

// TimelineEventsJSON writes merged timeline events with their stable schema.
func TimelineEventsJSON(w io.Writer, events []agentlog.TimelineEvent) error {
	return writeJSON(w, NewTimelineEvents(events))
}

// roleLabel is the entry-role display of an event; unknown roles fall back
// to "message" like the transcript labels.
func roleLabel(role string) string {
	if role != "" {
		return role
	}
	return "message"
}

// displayStamp renders a session start as date + minute; zero times show a
// fixed-width dash so columns stay aligned.
func displayStamp(t time.Time) string {
	if t.IsZero() {
		return "                "
	}
	return t.Local().Format(dateLayout)
}

// displayStampFull renders an event time as date + seconds, or a fixed-width
// dash for records without any timestamp.
func displayStampFull(t time.Time) string {
	if t.IsZero() {
		return "                   "
	}
	return t.Local().Format(timelineTimeLayout)
}

// displayDuration renders a session span as "2d 4h" / "3h 12m" / "5m 30s" /
// "42s"; open sessions (no end) show a dash.
func displayDuration(start, end time.Time) string {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return "-"
	}
	d := end.Sub(start)
	switch {
	case d >= 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dd %dh", h/24, h%24)
	case d >= time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dh %dm", m/60, m%60)
	case d >= time.Minute:
		s := int(d.Seconds())
		return fmt.Sprintf("%dm %ds", s/60, s%60)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
