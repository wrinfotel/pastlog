package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/render"
)

// newStatsCmd builds `pastlog stats` (M7): token statistics across all four
// agents with --agent/--project/--model/--since/--until filters and one
// --by grouping (agent|project|day|model, default agent). Exit codes: 0 ok —
// including an empty selection (an aggregate has no grep-style "no matches")
// — and 2 for real errors; stats never exits 1.
func newStatsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		by          string
		jsonOut     bool
		agentName   string
		project     string
		model       string
		since, till string
	)
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "aggregate token usage across agents, projects, days and models",
		Long: "aggregate token usage across all agents: sessions, messages, input/output/reasoning,\n" +
			"cache tokens and cost, grouped by --by agent|project|day|model (default agent) and\n" +
			"filterable with --agent, --project, --model, --since and --until.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch by {
			case render.StatsByAgent, render.StatsByProject, render.StatsByDay, render.StatsByModel:
			default:
				return fmt.Errorf("invalid --by value %q: use agent, project, day or model", by)
			}
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := newRegistry(home)
			adapters := reg.Adapters()

			if agentName != "" {
				if _, ok := reg.Get(agentName); !ok {
					return fmt.Errorf("unknown agent %q (available: %s)", agentName, availableAgents(reg))
				}
			}

			filter := agentlog.SessionFilter{Agent: agentName, Project: project}
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

			// best effort like every listing: a failing adapter notes once on
			// stderr and the aggregate keeps the other agents (spec §8)
			rows := agentlog.CollectUsage(adapters, filter, model, func(note string) {
				fmt.Fprintln(stderr, note)
			})
			groups := render.GroupStats(home, by, rows)
			if jsonOut {
				if err := render.StatsJSON(stdout, groups); err != nil {
					return err
				}
			} else {
				render.StatsHuman(stdout, by, groups)
			}
			noteStderr(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", render.StatsByAgent, "group by agent, project, day or model")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().StringVar(&agentName, "agent", "", "filter by agent name (e.g. claude-code)")
	cmd.Flags().StringVar(&project, "project", "", "filter by working dir substring")
	cmd.Flags().StringVar(&model, "model", "", "filter by model name substring (case-insensitive)")
	cmd.Flags().StringVar(&since, "since", "", "only sessions started after Nd, Nw or YYYY-MM-DD")
	cmd.Flags().StringVar(&till, "until", "", "only sessions started before Nd, Nw or YYYY-MM-DD")
	return cmd
}
