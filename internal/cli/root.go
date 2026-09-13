package cli

import (
	"fmt"
	"io"

	"github.com/pastlog/pastlog/internal/version"
	"github.com/spf13/cobra"
)

// Execute runs the pastlog CLI and returns the process exit code:
// 0 ok, 1 reserved for search no-matches (not used yet), 2 real error.
func Execute(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(stderr, "pastlog: %v\n", err)
		return 2
	}
	return 0
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "pastlog",
		Short: "search the full history of your AI coding-agent sessions",
		Long: "pastlog searches the full history of your AI coding-agent sessions " +
			"(claude-code and more) across all projects on this machine.\n" +
			"100% local, read-only, zero config.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(newVersionCmd(stdout))

	return root
}

func newVersionCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "pastlog %s (commit %s, date %s)\n",
				version.Version, version.Commit, version.Date)
			return nil
		},
	}
}
