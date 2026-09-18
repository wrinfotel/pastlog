package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/discovery"
	"github.com/wrinfotel/pastlog/internal/version"
)

// Execute runs the pastlog CLI and returns the process exit code:
// 0 ok, 1 search found no matches (grep-style), 2 real error.
func Execute(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if errors.Is(err, errNoMatches) {
			return 1
		}
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
			"across all projects on this machine.\n" +
			"100% local, read-only, zero config.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			printAgentSummary(cmd.OutOrStdout(), stderr, newRegistry(home))
			return nil
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.PersistentFlags().String("home", "", "user home directory holding the agent data (default: auto-detect)")
	root.PersistentFlags().Bool("no-color", false, "disable colored output (also honors NO_COLOR and non-TTY)")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		noColor, err := cmd.Flags().GetBool("no-color")
		if err != nil {
			return err
		}
		if noColor {
			color.NoColor = true
		}
		return nil
	}

	root.AddCommand(newAgentsCmd(stdout, stderr))
	root.AddCommand(newSessionsCmd(stdout, stderr))
	root.AddCommand(newStatsCmd(stdout, stderr))
	root.AddCommand(newSearchCmd(stdout, stderr))
	root.AddCommand(newShowCmd(stdout, stderr))
	root.AddCommand(newVersionCmd())

	return root
}

// homeDir resolves the effective home from the persistent --home flag.
func homeDir(cmd *cobra.Command) (string, error) {
	override, err := cmd.Flags().GetString("home")
	if err != nil {
		return "", err
	}
	return discovery.Home(override)
}

// printAgentSummary writes the one-line detected-agents summary shown by a
// bare `pastlog` invocation (spec §3). Adapter conditions — unreadable
// storage, locked databases, skipped lines — go to stderr through the same
// summarizer the other commands use (M4-B4).
func printAgentSummary(stdout, stderr io.Writer, reg *agentlog.Registry) {
	adapters := reg.Adapters()
	var parts []string
	for _, a := range adapters {
		if !a.Detect() {
			continue
		}
		n := 0
		if err := a.Sessions(func(agentlog.Session) error { n++; return nil }); err != nil {
			fmt.Fprintln(stderr, agentlog.UnreadableNote(a.Name(), err))
		}
		plural := "sessions"
		if n == 1 {
			plural = "session"
		}
		parts = append(parts, fmt.Sprintf("%s: %d %s", a.Name(), n, plural))
	}
	if len(parts) == 0 {
		fmt.Fprintln(stdout, "no agent data found — install an agent or pass --home <dir>")
		return
	}
	fmt.Fprintln(stdout, strings.Join(parts, ", "))
	noteStderr(stderr, adapters)
}

func newVersionCmd() *cobra.Command {
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
