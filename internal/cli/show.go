package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
	"github.com/pastlog/pastlog/internal/render"
	"github.com/spf13/cobra"
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
			reg := newRegistry(home)
			adapters := reg.Adapters()

			meta, adapter, err := resolveSession(adapters, args[0])
			if err != nil {
				return err
			}

			var entries []agentlog.Entry
			if err := adapter.Entries(meta.Session, func(e agentlog.Entry) error {
				entries = append(entries, e)
				return nil
			}); err != nil {
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
			noteSkipped(stderr, adapters)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().StringVar(&export, "export", "", "export format: md prints the session as markdown")
	return cmd
}

// resolveSession finds a session by exact id or unambiguous prefix across all
// adapters. Misses and ambiguity are user-facing errors (spec §3, §8): the
// ambiguity error lists the candidates.
func resolveSession(adapters []agentlog.Adapter, arg string) (agentlog.SessionMeta, agentlog.Adapter, error) {
	type found struct {
		meta    agentlog.SessionMeta
		adapter agentlog.Adapter
	}
	var all []found
	for _, a := range adapters {
		add := func(m agentlog.SessionMeta) error {
			m.Agent = a.Name()
			all = append(all, found{m, a})
			return nil
		}
		if ms, ok := a.(agentlog.MetaSource); ok {
			_ = ms.SessionsMeta(add)
		} else {
			_ = a.Sessions(func(s agentlog.Session) error {
				return add(agentlog.SessionMeta{Session: s})
			})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].meta.StartedAt.Equal(all[j].meta.StartedAt) {
			return all[i].meta.StartedAt.After(all[j].meta.StartedAt)
		}
		return all[i].meta.ID < all[j].meta.ID
	})

	for _, f := range all {
		if f.meta.ID == arg {
			return f.meta, f.adapter, nil
		}
	}
	var cands []found
	for _, f := range all {
		if strings.HasPrefix(f.meta.ID, arg) {
			cands = append(cands, f)
		}
	}
	switch len(cands) {
	case 1:
		return cands[0].meta, cands[0].adapter, nil
	case 0:
		return agentlog.SessionMeta{}, nil, fmt.Errorf("no session matches id prefix %q", arg)
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "ambiguous session id prefix %q; candidates:", arg)
		for _, c := range cands {
			fmt.Fprintf(&b, "\n  %s  %s  %s  %s", c.meta.ID, c.meta.Agent,
				candidateDate(c.meta.StartedAt), candidateProject(c.meta.Project))
		}
		return agentlog.SessionMeta{}, nil, errors.New(b.String())
	}
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
