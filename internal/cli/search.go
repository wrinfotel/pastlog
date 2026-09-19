package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/dates"
	"github.com/wrinfotel/pastlog/internal/render"
	"github.com/wrinfotel/pastlog/internal/search"
)

// errNoMatches marks a grep-style "no matches" outcome: Execute returns 1
// and nothing is printed (controller ruling).
var errNoMatches = errors.New("no matches")

func newSearchCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonOut       bool
		agentName     string
		project       string
		since, till   string
		limit, maxHit int
		caseSensitive bool
		regexMode     bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "search all session entries across agents",
		Long:  "search user/assistant messages, tool-call inputs and outputs of every session, across all agents",
		Args:  cobra.ExactArgs(1),
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
			if limit < 0 {
				return fmt.Errorf("invalid --limit value %d: use a non-negative number", limit)
			}
			if maxHit < 0 {
				return fmt.Errorf("invalid --max-hits value %d: use a non-negative number", maxHit)
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
			m, err := search.NewMatcher(args[0], search.MatchOptions{CaseSensitive: caseSensitive, Regex: regexMode})
			if err != nil {
				return err
			}

			// FastListing on the human path: search listing reads only the
			// first record line per file (spec §7 budget). --json keeps the
			// exact full-parse listing, because its schema carries fields
			// that live beyond line 1 (ended_at, message counts).
			results := search.Run(adapters, m, search.EngineOptions{
				Filter:      filter,
				Sessions:    limit,
				MaxHits:     maxHit,
				FastListing: !jsonOut,
				Note: func(note string) {
					fmt.Fprintln(stderr, note)
				},
			})
			// human output prints nothing without matches; --json still
			// prints a valid (empty) document
			if jsonOut {
				if err := render.SearchJSON(stdout, results); err != nil {
					return err
				}
			} else {
				render.SearchHuman(stdout, results)
			}
			noteStderr(stderr, adapters)
			if totalHits(results) == 0 {
				return errNoMatches
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON")
	cmd.Flags().StringVar(&agentName, "agent", "", "filter by agent name (e.g. claude-code)")
	cmd.Flags().StringVar(&project, "project", "", "filter by working dir substring")
	cmd.Flags().StringVar(&since, "since", "", "only sessions started after Nd, Nw or YYYY-MM-DD")
	cmd.Flags().StringVar(&till, "until", "", "only sessions started before Nd, Nw or YYYY-MM-DD")
	cmd.Flags().IntVar(&limit, "limit", 0, "scan at most N sessions (0 = no limit)")
	cmd.Flags().IntVar(&maxHit, "max-hits", search.DefaultMaxHits, "print at most N hits in total (0 = no limit)")
	cmd.Flags().BoolVar(&caseSensitive, "case-sensitive", false, "match case-sensitively (default: case-insensitive)")
	cmd.Flags().BoolVar(&regexMode, "regex", false, "interpret the query as a regular expression")
	return cmd
}

func totalHits(results []search.Result) int {
	n := 0
	for _, r := range results {
		n += len(r.Hits)
	}
	return n
}
