package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/ctx"
)

func newContextCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context <session-id-or-prefix>",
		Short: "analyze why one session's context grew (what fed the window)",
		Long: "analyze one session's context: the per-turn window curve, what fed it\n" +
			"(oversized tool results, repeated reads, error loops, compactions) and\n" +
			"advice. Accepts an unambiguous id prefix. The same rules run for every agent.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := NewRegistry(home)
			adapters := reg.Adapters()

			meta, adapter, err := resolveSession(adapters, args[0])
			if err != nil {
				noteStderr(stderr, adapters)
				return err
			}

			src, ok := adapter.(agentlog.CtxSource)
			if !ok {
				return fmt.Errorf("context analysis is not available for %s yet", meta.Agent)
			}
			events, err := src.ContextEvents(meta.Session)
			if err != nil {
				noteStderr(stderr, adapters)
				return fmt.Errorf("cannot analyze session %s: %v", meta.ID, err)
			}

			profile := ctx.Analyze(events)
			ctx.RenderHuman(stdout, meta, profile)
			noteStderr(stderr, adapters)
			return nil
		},
	}
	return cmd
}
