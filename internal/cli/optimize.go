package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/ctx"
	"github.com/wrinfotel/pastlog/internal/dates"
	"github.com/wrinfotel/pastlog/internal/mask"
	"github.com/wrinfotel/pastlog/internal/render"
)

// newOptimizeCmd builds `pastlog optimize` (SPEC-optimize.md): the
// cross-session waste report. Same filters as sessions; advice-only by
// design — it never touches agent storage or config.
func newOptimizeCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonOut     bool
		noMask      bool
		agentName   string
		project     string
		since, till string
	)
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "find token waste recurring across sessions (cross-session context report)",
		Long: "fold the context analysis across every matching session and report what\n" +
			"repeats: commands whose oversized output is paid again every session (R6),\n" +
			"content re-discovered from scratch each time (R7), commands that keep failing\n" +
			"(R8), and the always-loaded context prefix (R9). Same filters as sessions;\n" +
			"advice only — pastlog never writes to agent storage or config.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := NewRegistry(home)
			adapters := reg.Adapters()
			if agentName != "" {
				if _, ok := reg.Get(agentName); !ok {
					return fmt.Errorf("unknown agent %q (available: %s)", agentName, availableAgents(reg))
				}
			}
			filter := agentlog.SessionFilter{Agent: agentName, Project: project}
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

			opt := ctx.NewOptimizer()
			_, err = agentlog.CollectCtxBatches(adapters, filter, func(note string) {
				fmt.Fprintln(stderr, note)
			}, func(m agentlog.SessionMeta, events []agentlog.CtxEvent) error {
				opt.Observe(m.ID, events)
				return nil
			})
			if err != nil {
				return err
			}
			report := opt.Report()
			if !jsonOut && !noMask {
				for i := range report.Findings {
					report.Findings[i].Label = mask.Mask(report.Findings[i].Label)
					report.Findings[i].Desc = mask.Mask(report.Findings[i].Desc)
				}
			}
			if jsonOut {
				if err := render.OptimizeJSON(stdout, report); err != nil {
					return err
				}
			} else {
				render.OptimizeHuman(stdout, report)
			}
			noteStderr(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().BoolVar(&noMask, "no-mask", false, "print secrets verbatim (default: masked like ghp_…ABCD)")
	cmd.Flags().StringVar(&agentName, "agent", "", "filter by agent name (e.g. claude-code)")
	cmd.Flags().StringVar(&project, "project", "", "filter by working dir substring")
	cmd.Flags().StringVar(&since, "since", "", "only sessions started after Nd, Nw or YYYY-MM-DD")
	cmd.Flags().StringVar(&till, "until", "", "only sessions started before Nd, Nw or YYYY-MM-DD")
	return cmd
}
