package render

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/wrinfotel/pastlog/internal/search"
)

var matchColor = color.New(color.Bold)

// SearchHuman writes search result cards (spec §3): one header line per
// session with hits — agent, project (last two path components), date, id
// prefix — followed by each hit's context line and matching line with the
// match highlighted. Cards are separated by a blank line.
func SearchHuman(w io.Writer, results []search.Result) {
	prevLine := "" // rendered match line of the previous hit, for dedupe
	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		nameColor.Fprintf(w, "%s", r.Session.Agent)
		fmt.Fprintf(w, " · %s · %s · sess %s\n",
			shortProject(r.Session.Project), shortDate(r.Session.StartedAt), idPrefix(r.Session.ID))
		for _, h := range r.Hits {
			line := highlightMatch(h)
			if h.Context != "" && h.Context != prevLine {
				fmt.Fprintf(w, "  %s\n", h.Context)
			}
			fmt.Fprintf(w, "  → %s\n", line)
			prevLine = h.Line
		}
	}
}

// highlightMatch wraps the matched span of the snippet line in the match
// color. Degenerate ranges render plain.
func highlightMatch(h search.Hit) string {
	line := h.Line
	if h.MatchStart < 0 || h.MatchEnd > len(line) || h.MatchStart >= h.MatchEnd {
		return line
	}
	return line[:h.MatchStart] + matchColor.Sprint(line[h.MatchStart:h.MatchEnd]) + line[h.MatchEnd:]
}

// shortProject renders a project path as its last two path components (spec
// §3 example: "myapp/api"); empty paths show "-".
func shortProject(project string) string {
	var parts []string
	for _, seg := range strings.Split(toSlash(project), "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	switch len(parts) {
	case 0:
		return "-"
	case 1:
		return parts[0]
	default:
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
}

// shortDate renders a session date as YYYY-MM-DD in the local zone; zero
// times show "-".
func shortDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02")
}

type searchHitJSON struct {
	Kind       string     `json:"kind"`
	Role       string     `json:"role"`
	Timestamp  *time.Time `json:"timestamp"`
	Context    string     `json:"context"`
	Line       string     `json:"line"`
	MatchStart int        `json:"match_start"` // rune offset into line
	MatchEnd   int        `json:"match_end"`   // rune offset into line
}

type searchResultJSON struct {
	Session sessionJSON     `json:"session"`
	Hits    []searchHitJSON `json:"hits"`
}

// SearchJSON writes search results as JSON with a stable schema. Match
// offsets are rune offsets into line, so they survive encoding and stay
// readable for non-Go consumers.
func SearchJSON(w io.Writer, results []search.Result) error {
	out := make([]searchResultJSON, 0, len(results))
	for _, r := range results {
		row := searchResultJSON{Session: newSessionJSON(r.Session), Hits: make([]searchHitJSON, 0, len(r.Hits))}
		for _, h := range r.Hits {
			row.Hits = append(row.Hits, searchHitJSON{
				Kind:       kindJSON(h.Entry.Kind),
				Role:       h.Entry.Role,
				Timestamp:  timePtr(h.Entry.Timestamp),
				Context:    h.Context,
				Line:       h.Line,
				MatchStart: runeOffset(h.Line, h.MatchStart),
				MatchEnd:   runeOffset(h.Line, h.MatchEnd),
			})
		}
		out = append(out, row)
	}
	return writeJSON(w, out)
}

// runeOffset converts a byte offset in s to a rune offset, clamped to the
// string bounds.
func runeOffset(s string, byteIdx int) int {
	if byteIdx < 0 {
		return 0
	}
	if byteIdx > len(s) {
		byteIdx = len(s)
	}
	return utf8.RuneCountInString(s[:byteIdx])
}
