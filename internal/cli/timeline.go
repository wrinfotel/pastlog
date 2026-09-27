package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/dates"
	"github.com/wrinfotel/pastlog/internal/mask"
	"github.com/wrinfotel/pastlog/internal/render"
)

// defaultMaxTimelineRows caps the --messages stream: a merged timeline is a
// navigation view, not a transcript dump.
const defaultMaxTimelineRows = 500

func newTimelineCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonOut     bool
		messages    bool
		agentName   string
		project     string
		since, till string
		limit       int
		maxRows     int
		noMask      bool
	)
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "merged chronological view across sessions and agents",
		Long: "merged chronological view across sessions and agents: by default one row per " +
			"session, oldest first, showing how work interleaved across projects. With " +
			"--messages the user/assistant messages of every matching session merge into " +
			"one stream (newest --max-rows kept) — the \"how did we get here\" view.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := NewRegistry(home)
			adapters := reg.Adapters()

			if limit < 0 {
				return fmt.Errorf("invalid --limit value %d: use a non-negative number", limit)
			}
			if maxRows < 0 {
				return fmt.Errorf("invalid --max-rows value %d: use a non-negative number", maxRows)
			}
			if agentName != "" {
				if _, ok := reg.Get(agentName); !ok {
					return fmt.Errorf("unknown agent %q (available: %s)", agentName, availableAgents(reg))
				}
			}

			filter := agentlog.SessionFilter{Agent: agentName, Project: project, Limit: limit}
			if since != "" {
				if filter.Since, err = dates.ParseCutoff("since", since, false); err != nil {
					return err
				}
			}
			if till != "" {
				if filter.Until, err = dates.ParseCutoff("until", till, true); err != nil {
					return err
				}
			}
			note := func(note string) { fmt.Fprintln(stderr, note) }

			if !messages {
				// CollectSessions sorts newest first and its limit keeps the
				// newest N; the timeline reads the same selection oldest first.
				rows := agentlog.CollectSessions(adapters, filter, note)
				for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
					rows[i], rows[j] = rows[j], rows[i]
				}
				if jsonOut {
					err = render.TimelineSessionsJSON(stdout, rows)
				} else {
					render.TimelineSessionsHuman(stdout, home, rows)
				}
			} else {
				events := agentlog.CollectTimelineEvents(adapters, filter, maxRows, note)
				// Masking is a human-output concern: --json is the machine
				// channel and stays verbatim.
				if !jsonOut && !noMask {
					for i := range events {
						events[i].Text = mask.Mask(events[i].Text)
					}
				}
				if jsonOut {
					err = render.TimelineEventsJSON(stdout, events)
				} else {
					render.TimelineEventsHuman(stdout, events)
				}
			}
			if err != nil {
				return err
			}
			noteStderr(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().BoolVar(&messages, "messages", false, "merge user/assistant messages of every session into one stream")
	cmd.Flags().StringVar(&agentName, "agent", "", "filter by agent name (e.g. claude-code)")
	cmd.Flags().StringVar(&project, "project", "", "filter by working dir substring")
	cmd.Flags().StringVar(&since, "since", "", "only sessions started after Nd, Nw or YYYY-MM-DD")
	cmd.Flags().StringVar(&till, "until", "", "only sessions started before Nd, Nw or YYYY-MM-DD")
	cmd.Flags().IntVar(&limit, "limit", 0, "include at most N sessions, newest first (0 = no limit)")
	cmd.Flags().IntVar(&maxRows, "max-rows", defaultMaxTimelineRows, "with --messages, keep the newest N events (0 = no limit)")
	cmd.Flags().BoolVar(&noMask, "no-mask", false, "print secrets verbatim (default: masked like ghp_…ABCD)")
	return cmd
}
