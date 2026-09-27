package render

import (
	"fmt"
	"io"

	"github.com/fatih/color"
	"github.com/wrinfotel/pastlog/internal/agentlog"
)

var sectionColor = color.New(color.Faint)

// RelatedHuman writes the relatedness graph of one session: the anchor as a
// sessions-table row, then the parent, subagents and adjacent-in-project
// sections with their rows indented. All rows share one column alignment
// (the M1 renderer pattern); empty sections are omitted.
func RelatedHuman(w io.Writer, home string, rel agentlog.RelatedSessions) {
	var rows []agentlog.SessionMeta
	rows = append(rows, rel.Meta)
	if rel.Parent != nil {
		rows = append(rows, *rel.Parent)
	}
	rows = append(rows, rel.Children...)
	rows = append(rows, rel.Adjacent...)

	agentW, projW, countW, sizeW := 0, 0, 0, 0
	for _, r := range rows {
		agentW = max(agentW, len(r.Agent))
		projW = max(projW, len(displayProject(home, r.Project)))
		countW = max(countW, len(fmt.Sprint(r.Messages)))
		sizeW = max(sizeW, len(HumanBytes(r.SizeBytes)))
	}
	row := func(r agentlog.SessionMeta) string {
		plural := "messages"
		if r.Messages == 1 {
			plural = "message"
		}
		return fmt.Sprintf("%s  %-*s  %s  %*d %-8s  %*s  %s",
			nameColor.Sprintf("%-*s", agentW, r.Agent),
			projW, displayProject(home, r.Project), displayDate(r.StartedAt),
			countW, r.Messages, plural, sizeW, HumanBytes(r.SizeBytes), idPrefix(r.ID))
	}

	fmt.Fprintf(w, "%s\n", row(rel.Meta))

	section := func(label string, list []agentlog.SessionMeta) {
		if len(list) == 0 {
			return
		}
		fmt.Fprintln(w)
		sectionColor.Fprintf(w, "%s", label)
		fmt.Fprintln(w)
		for _, r := range list {
			fmt.Fprintf(w, "  %s\n", row(r))
		}
	}
	if rel.Parent != nil {
		section("parent", []agentlog.SessionMeta{*rel.Parent})
	}
	section(fmt.Sprintf("subagents (%d)", len(rel.Children)), rel.Children)
	section(fmt.Sprintf("adjacent in project (%d)", len(rel.Adjacent)), rel.Adjacent)
}

// RelatedJSON writes the relatedness graph as JSON with a stable schema.
func RelatedJSON(w io.Writer, rel agentlog.RelatedSessions) error {
	return writeJSON(w, NewRelatedDoc(rel))
}
