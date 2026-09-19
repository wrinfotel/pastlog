package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/adapters/opencode"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// assertHomeUnchanged hashes the synthetic agent tree and returns a checker
// that fails when any file changed — the read-only guarantee (spec §2.1),
// enforced around every service call in these tests.
func assertHomeUnchanged(t *testing.T, home string) func() {
	t.Helper()
	before := hashTree(t, home)
	return func() {
		t.Helper()
		after := hashTree(t, home)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("agent storage changed under %s:\nbefore %v\nafter  %v", home, before, after)
		}
	}
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		h := sha256.New()
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatalf("hashTree: %v", err)
	}
	return out
}

// opencodeHome builds a synthetic home whose OpenCode storage is the
// generated fixture DB at <home>/.local/share/opencode (the adapter's
// home-relative candidate path, valid on every OS).
func opencodeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if _, err := opencode.GenerateTestDB(filepath.Join(home, ".local", "share", "opencode")); err != nil {
		t.Fatalf("GenerateTestDB: %v", err)
	}
	return home
}

func TestEntriesOKAndNotFoundAndAmbiguous(t *testing.T) {
	a := homeApp(t, opencodeHome(t))
	check := assertHomeUnchanged(t, a.cfg.Home)
	defer check()

	ok, err := a.Entries("ses_fixture1")
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if ok.Status != "ok" || ok.Session == nil || ok.Session.ID != "ses_fixture100001fix" {
		t.Errorf("ok outcome wrong: status=%q session=%+v", ok.Status, ok.Session)
	}
	if len(ok.Entries) == 0 {
		t.Error("ok outcome must carry the transcript entries")
	}

	nf, err := a.Entries("zzzzzzzz")
	if err != nil {
		t.Fatalf("not-found must not be a Go error: %v", err)
	}
	if nf.Status != "notfound" {
		t.Errorf("status = %q, want notfound", nf.Status)
	}

	amb, err := a.Entries("ses_fixture")
	if err != nil {
		t.Fatalf("ambiguity must not be a Go error: %v", err)
	}
	if amb.Status != "ambiguous" || len(amb.Candidates) != 2 {
		t.Errorf("ambiguous outcome wrong: status=%q candidates=%d", amb.Status, len(amb.Candidates))
	}
}

func TestExportSessionJSONParityWithCLI(t *testing.T) {
	home := claudeHome(t, "realistic.jsonl")
	a := homeApp(t, home)
	check := assertHomeUnchanged(t, home)
	defer check()

	dest := filepath.Join(t.TempDir(), "session.json")
	res, err := a.ExportSession("3f9c81a2", "json", dest)
	if err != nil || res.Status != "written" {
		t.Fatalf("ExportSession: %v %+v", err, res)
	}

	var cliOut, cliErr bytes.Buffer
	if code := cli.Execute([]string{"show", "3f9c81a2", "--json", "--home", home}, &cliOut, &cliErr); code != 0 {
		t.Fatalf("CLI show exited %d: %s", code, cliErr.String())
	}
	exported, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exported, cliOut.Bytes()) {
		t.Errorf("GUI export is not byte-equal to CLI --json:\nGUI %s\nCLI %s", exported, cliOut.Bytes())
	}
}

func TestExportSessionMDParityWithCLI(t *testing.T) {
	home := opencodeHome(t)
	a := homeApp(t, home)
	check := assertHomeUnchanged(t, home)
	defer check()

	dest := filepath.Join(t.TempDir(), "session.md")
	if _, err := a.ExportSession("ses_fixture2", "md", dest); err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	var cliOut, cliErr bytes.Buffer
	if code := cli.Execute([]string{"show", "ses_fixture200002fix", "--export", "md", "--home", home}, &cliOut, &cliErr); code != 0 {
		t.Fatalf("CLI show exited %d: %s", code, cliErr.String())
	}
	exported, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exported, cliOut.Bytes()) {
		t.Errorf("GUI md export is not byte-equal to CLI --export md:\nGUI %s\nCLI %s", exported, cliOut.Bytes())
	}
}

func TestExportSessionAmbiguousWritesNothing(t *testing.T) {
	home := opencodeHome(t)
	a := homeApp(t, home)
	dest := filepath.Join(t.TempDir(), "never.json")
	res, err := a.ExportSession("ses_fixture", "json", dest)
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	if res.Status != "ambiguous" || len(res.Candidates) != 2 {
		t.Errorf("status=%q candidates=%d, want ambiguous with 2 candidates", res.Status, len(res.Candidates))
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("an ambiguous export must not write the file")
	}
}

func TestExportSessionValidation(t *testing.T) {
	a := homeApp(t, opencodeHome(t))
	if _, err := a.ExportSession("ses_fixture1", "yaml", filepath.Join(t.TempDir(), "x")); err == nil ||
		!strings.Contains(err.Error(), `unsupported export format "yaml"`) {
		t.Errorf("format validation: %v", err)
	}
	if _, err := a.ExportSession("ses_fixture1", "json", ""); err == nil || err.Error() != "empty export path" {
		t.Errorf("empty path: %v", err)
	}
	if _, err := a.ExportSession("ses_fixture1", "json", "relative/out.json"); err == nil ||
		!strings.Contains(err.Error(), "export path must be absolute") {
		t.Errorf("relative path: %v", err)
	}
}

func TestExportSessionsParityWithCLI(t *testing.T) {
	home := claudeHome(t, "realistic.jsonl", "usage.jsonl")
	a := homeApp(t, home)
	check := assertHomeUnchanged(t, home)
	defer check()

	dest := filepath.Join(t.TempDir(), "sessions.json")
	if err := a.ExportSessions(FilterOptions{}, dest); err != nil {
		t.Fatalf("ExportSessions: %v", err)
	}
	var cliOut, cliErr bytes.Buffer
	if code := cli.Execute([]string{"sessions", "--json", "--home", home}, &cliOut, &cliErr); code != 0 {
		t.Fatalf("CLI sessions exited %d: %s", code, cliErr.String())
	}
	exported, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exported, cliOut.Bytes()) {
		t.Errorf("GUI sessions export is not byte-equal to CLI --json:\nGUI %s\nCLI %s", exported, cliOut.Bytes())
	}
}

func TestEntriesOutcomeNeverMarshalsNullLists(t *testing.T) {
	a := homeApp(t, opencodeHome(t))
	for _, probe := range []string{"ses_fixture1", "zzzz", "ses_fixture"} {
		raw, err := json.Marshal(mustEntries(t, a, probe))
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"entries", "candidates", "unreadable", "notes"} {
			if strings.Contains(string(raw), `"`+key+`":null`) {
				t.Errorf("Entries(%q) marshals %s as null: %s", probe, key, raw)
			}
		}
	}
}

func mustEntries(t *testing.T, a *App, prefix string) ShowOutcome {
	t.Helper()
	out, err := a.Entries(prefix)
	if err != nil {
		t.Fatalf("Entries(%q): %v", prefix, err)
	}
	return out
}

// BenchmarkMarshalSessions10k measures the binding-layer serialization
// cost (spec §6): 10k sessions marshaled to the GUI JSON — the payload size
// the Sessions view receives on a big machine.
func BenchmarkMarshalSessions10k(b *testing.B) {
	const n = 10_000
	start := time.Unix(1_786_220_000, 0)
	metas := make([]agentlog.SessionMeta, n)
	for i := range metas {
		metas[i] = agentlog.SessionMeta{
			Session: agentlog.Session{
				ID:        fmt.Sprintf("session-%06d-0000-0000-000000000000", i),
				Agent:     "claude-code",
				Project:   "C:/dev/project",
				StartedAt: start,
				SizeBytes: 123456,
			},
			Messages: 42,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := json.Marshal(render.SessionRowsJSON(metas))
		if err != nil {
			b.Fatal(err)
		}
		_ = raw
	}
}
