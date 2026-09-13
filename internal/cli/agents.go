package cli

import (
	"fmt"
	"io"

	"github.com/pastlog/pastlog/internal/agentlog"
	"github.com/pastlog/pastlog/internal/render"
	"github.com/spf13/cobra"
)

func newAgentsCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "list detected agent sources with session counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			adapters := newRegistry(home).Adapters()

			rows := make([]render.AgentRow, len(adapters))
			for i, a := range adapters {
				row := render.AgentRow{Name: a.Name()}
				if ps, ok := a.(agentlog.PathSource); ok {
					row.Path = ps.StoragePath()
				}
				if a.Detect() {
					row.Detected = true
					_ = a.Sessions(func(s agentlog.Session) error {
						row.Sessions++
						row.Bytes += s.SizeBytes
						return nil
					})
				}
				rows[i] = row
			}

			if jsonOut {
				if err := render.AgentsJSON(stdout, rows); err != nil {
					return err
				}
			} else {
				render.AgentsHuman(stdout, home, rows)
				if detectedCount(rows) == 0 {
					fmt.Fprintln(stdout, "nothing found — install an agent or pass --home <dir>")
				}
			}
			noteSkipped(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	return cmd
}

func detectedCount(rows []render.AgentRow) int {
	n := 0
	for _, r := range rows {
		if r.Detected {
			n++
		}
	}
	return n
}

// noteSkipped summarizes unreadable lines on stderr (spec §8).
func noteSkipped(stderr io.Writer, adapters []agentlog.Adapter) {
	if n := agentlog.TotalSkipped(adapters); n > 0 {
		fmt.Fprintf(stderr, "%d unreadable lines skipped\n", n)
	}
}
