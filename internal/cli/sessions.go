package cli

import (
	"fmt"
	"io"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/render"
	"github.com/spf13/cobra"
)

func newSessionsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonOut     bool
		agentName   string
		project     string
		since, till string
		limit       int
	)
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "list sessions, newest first",
		Long:  "list sessions, newest first: agent, project, date, message count, size and id prefix",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := newRegistry(home)
			adapters := reg.Adapters()

			if limit < 0 {
				return fmt.Errorf("invalid --limit value %d: use a non-negative number", limit)
			}
			if agentName != "" {
				if _, ok := reg.Get(agentName); !ok {
					return fmt.Errorf("unknown agent %q (available: %s)", agentName, availableAgents(reg))
				}
			}

			filter := agentlog.SessionFilter{Agent: agentName, Project: project, Limit: limit}
			if since != "" {
				if filter.Since, err = parseCutoff("since", since, false); err != nil {
					return err
				}
			}
			if till != "" {
				if filter.Until, err = parseCutoff("until", till, true); err != nil {
					return err
				}
			}

			rows := agentlog.CollectSessions(adapters, filter, func(note string) {
				fmt.Fprintln(stderr, note)
			})
			if jsonOut {
				if err := render.SessionsJSON(stdout, rows); err != nil {
					return err
				}
			} else {
				render.SessionsHuman(stdout, home, rows)
			}
			noteStderr(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().StringVar(&agentName, "agent", "", "filter by agent name (e.g. claude-code)")
	cmd.Flags().StringVar(&project, "project", "", "filter by working dir substring")
	cmd.Flags().StringVar(&since, "since", "", "only sessions started after Nd, Nw or YYYY-MM-DD")
	cmd.Flags().StringVar(&till, "until", "", "only sessions started before Nd, Nw or YYYY-MM-DD")
	cmd.Flags().IntVar(&limit, "limit", 0, "show at most N sessions (0 = no limit)")
	return cmd
}

func availableAgents(reg *agentlog.Registry) string {
	names := ""
	for i, a := range reg.Adapters() {
		if i > 0 {
			names += ", "
		}
		names += a.Name()
	}
	return names
}
