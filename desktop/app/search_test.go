package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

func cliSearchJSON(t *testing.T, home string, args ...string) []render.SearchResultJSON {
	t.Helper()
	var out, errOut bytes.Buffer
	full := append([]string{"search", "--json", "--home", home}, args...)
	if code := cli.Execute(full, &out, &errOut); code > 1 {
		t.Fatalf("pastlog %v exited %d (stderr: %s)", full, code, errOut.String())
	}
	var want []render.SearchResultJSON
	if err := json.Unmarshal(out.Bytes(), &want); err != nil {
		t.Fatalf("CLI output unmarshal: %v", err)
	}
	return want
}

func TestSearchParityWithCLI(t *testing.T) {
	home := claudeHome(t, "realistic.jsonl")
	a := homeApp(t, home)
	cases := []struct {
		name  string
		query string
		args  []string
		opts  SearchOptions
	}{
		{"literal", "localStorage", nil, SearchOptions{}},
		{"case-insensitive default", "LOCALSTORAGE", nil, SearchOptions{}},
		{"case-sensitive miss", "LOCALSTORAGE", []string{"--case-sensitive"}, SearchOptions{CaseSensitive: true}},
		{"regex hit", `refresh\s+token`, []string{"--regex"}, SearchOptions{Regex: true}},
		{"agent filter", "token", []string{"--agent", "claude-code"}, SearchOptions{Filter: FilterOptions{Agent: "claude-code"}}},
		{"agent filter empty", "token", []string{"--agent", "codex"}, SearchOptions{Filter: FilterOptions{Agent: "codex"}}},
		{"project filter", "token", []string{"--project", "myapp"}, SearchOptions{Filter: FilterOptions{Project: "myapp"}}},
		{"since", "token", []string{"--since", "2026-08-03"}, SearchOptions{Filter: FilterOptions{Since: "2026-08-03"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := cliSearchJSON(t, home, append(tc.args, tc.query)...)
			got, err := a.Search(tc.query, tc.opts)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if !reflect.DeepEqual(got.Results, want) {
				t.Errorf("search drift:\n GUI %+v\n CLI %+v", got.Results, want)
			}
		})
	}
}

func TestSearchMaxHitsTruncates(t *testing.T) {
	home := claudeHome(t, "realistic.jsonl", "usage.jsonl")
	a := homeApp(t, home)
	got, err := a.Search("token", SearchOptions{MaxHits: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.Hits != 2 || !got.Truncated {
		t.Errorf("hits=%d truncated=%v, want 2 hits and truncated", got.Hits, got.Truncated)
	}
	want := cliSearchJSON(t, home, "--max-hits", "2", "token")
	if !reflect.DeepEqual(got.Results, want) {
		t.Errorf("capped search drift:\n GUI %+v\n CLI %+v", got.Results, want)
	}
}

func TestSearchNoMatches(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	got, err := a.Search("zzznomatchzzz", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.Hits != 0 || len(got.Results) != 0 || got.Truncated || got.Cancelled {
		t.Errorf("no-match outcome wrong: %+v", got)
	}
}

func TestSearchInvalidRegex(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	_, err := a.Search("([bad", SearchOptions{Regex: true})
	if err == nil {
		t.Fatal("invalid regex accepted")
	}
	if !strings.Contains(err.Error(), "regex") && !strings.Contains(err.Error(), "expression") {
		t.Errorf("error should name the regex problem, got %q", err.Error())
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	if _, err := a.Search("", SearchOptions{}); err == nil || err.Error() != "empty query" {
		t.Errorf("empty query error = %v", err)
	}
}

func TestSearchUnknownAgent(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	_, err := a.Search("x", SearchOptions{Filter: FilterOptions{Agent: "nope"}})
	if err == nil || !strings.Contains(err.Error(), `unknown agent "nope"`) {
		t.Errorf("unknown agent error = %v", err)
	}
}

func TestSearchCancelStopsAndReports(t *testing.T) {
	home := claudeHome(t, "realistic.jsonl", "usage.jsonl")

	// the uncapped run for comparison
	full := homeApp(t, home)
	reference, err := full.Search("token", SearchOptions{})
	if err != nil {
		t.Fatalf("reference Search: %v", err)
	}
	if reference.Hits < 2 {
		t.Skipf("fixture yields %d hits; the cancel test needs a multi-hit corpus", reference.Hits)
	}

	// cancel right after the first progress tick: the scan must stop early
	var a *App
	sink := &fakeSink{onProgress: func(op string, scanned, hits int) {
		if a != nil {
			a.Cancel()
		}
	}}
	a = New(WithConfigDir(filepath.Join(t.TempDir(), "cfg")), WithSink(sink))
	a.cfg.Home = home

	got, err := a.Search("token", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !got.Cancelled {
		t.Error("outcome must report cancellation")
	}
	if got.Hits >= reference.Hits {
		t.Errorf("cancelled run has %d hits, the uncapped run had %d — cancel did not stop the scan",
			got.Hits, reference.Hits)
	}
	if got.Truncated {
		t.Error("a cancelled run must not report truncation")
	}
}
