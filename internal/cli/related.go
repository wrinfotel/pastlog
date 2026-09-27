package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/render"
)

func newRelatedCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "related <session-id-or-prefix>",
		Short: "show sessions related to one: parent, subagents, project neighbors",
		Long: "show the sessions related to one session: its subagent parent (where the " +
			"agent records one), its subagent children, and the nearest sessions of the " +
			"same agent and project — the continuation heuristic. Accepts an unambiguous id prefix.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := NewRegistry(home)
			adapters := reg.Adapters()

			meta, _, err := resolveSession(adapters, args[0])
			if err != nil {
				noteStderr(stderr, adapters)
				return err
			}

			rel := agentlog.Related(adapters, meta)
			for _, u := range rel.Unreadable {
				fmt.Fprintln(stderr, u)
			}
			if jsonOut {
				if err := render.RelatedJSON(stdout, rel); err != nil {
					return err
				}
			} else {
				render.RelatedHuman(stdout, home, rel)
			}
			noteStderr(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	return cmd
}
