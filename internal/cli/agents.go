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
					if err := a.Sessions(func(s agentlog.Session) error {
						row.Sessions++
						row.Bytes += s.SizeBytes
						return nil
					}); err != nil {
						// best effort (spec §8): keep the row, explain the
						// empty count on stderr
						fmt.Fprintln(stderr, agentlog.UnreadableNote(a.Name(), err))
					}
					// adapters backed by one shared file (e.g. the opencode
					// database) report their on-disk footprint instead of
					// the per-session sum
					if ts, ok := a.(agentlog.TotalSizer); ok {
						row.Bytes = ts.TotalBytes()
					}
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
			noteStderr(stderr, adapters)
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

// noteStderr summarizes adapter conditions on stderr (spec §4, §8): one
// warning line per unavailable storage (locked databases), then the total of
// unreadable records.
func noteStderr(stderr io.Writer, adapters []agentlog.Adapter) {
	for _, a := range adapters {
		if ws, ok := a.(agentlog.WarningSource); ok {
			if msg := ws.Warning(); msg != "" {
				fmt.Fprintln(stderr, msg)
			}
		}
	}
	if n := agentlog.TotalSkipped(adapters); n > 0 {
		fmt.Fprintf(stderr, "%d unreadable lines skipped\n", n)
	}
}
