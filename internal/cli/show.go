package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/render"
)

func newShowCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonOut bool
		export  string
	)
	cmd := &cobra.Command{
		Use:   "show <session-id-or-prefix>",
		Short: "print one session as a readable transcript",
		Long:  "print one session as a readable transcript; accepts an unambiguous id prefix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if export != "" && export != "md" {
				return fmt.Errorf("unsupported --export format %q (only md)", export)
			}
			if jsonOut && export != "" {
				return errors.New("--json and --export cannot be combined")
			}
			home, err := homeDir(cmd)
			if err != nil {
				return err
			}
			reg := NewRegistry(home)
			adapters := reg.Adapters()

			meta, adapter, err := resolveSession(adapters, args[0])
			if err != nil {
				// adapter conditions (locked databases, skipped lines) are
				// also reported on the failure path (M4): a locked opencode
				// DB must not read as a plain "no session" without context
				noteStderr(stderr, adapters)
				return err
			}

			var entries []agentlog.Entry
			if err := adapter.Entries(meta.Session, func(e agentlog.Entry) error {
				entries = append(entries, e)
				return nil
			}); err != nil {
				noteStderr(stderr, adapters)
				return fmt.Errorf("cannot read session %s: %v", meta.ID, err)
			}

			switch {
			case jsonOut:
				err = render.ShowJSON(stdout, meta, entries)
			case export == "md":
				render.ShowMarkdown(stdout, meta, entries)
			default:
				render.ShowHuman(stdout, home, meta, entries)
			}
			if err != nil {
				return err
			}
			noteStderr(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().StringVar(&export, "export", "", "export format: md prints the session as markdown")
	return cmd
}

// resolveSession adapts the structured core resolution (agentlog.ResolveSession,
// R-D6) to the CLI's error contract: not-found gains the unreadable-storage
// context (M4-B3); ambiguity lists candidates in the core's order. Golden CLI
// output must not change.
func resolveSession(adapters []agentlog.Adapter, arg string) (agentlog.SessionMeta, agentlog.Adapter, error) {
	res, err := agentlog.ResolveSession(adapters, arg)
	if res.Found() {
		return res.Meta, res.Adapter, nil
	}
	if len(res.Candidates) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "ambiguous session id prefix %q; candidates:", arg)
		for _, c := range res.Candidates {
			fmt.Fprintf(&b, "\n  %s  %s  %s  %s", c.ID, c.Agent, candidateDate(c.StartedAt), candidateProject(c.Project))
		}
		return agentlog.SessionMeta{}, nil, errors.New(b.String())
	}
	if err == nil { // unreachable today, but never report success without a session
		err = fmt.Errorf("no session matches id prefix %q", arg)
	}
	if len(res.Unreadable) > 0 {
		return agentlog.SessionMeta{}, nil, fmt.Errorf(
			"no session matches id prefix %q (some agent storage was unreadable: %s)",
			arg, strings.Join(res.Unreadable, "; "))
	}
	return agentlog.SessionMeta{}, nil, err
}

func candidateDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02")
}

func candidateProject(project string) string {
	if project == "" {
		return "-"
	}
	return project
}
