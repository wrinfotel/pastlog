package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRelatedGolden(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "related", "aaaa1111-1111-4111-8111-111111111111")
	if code != 0 {
		t.Fatalf("related exit = %d, stderr: %s", code, errOut)
	}
	anchorStart := mustUTC(t, "2026-08-02T14:03:22Z").Local().Format("2006-01-02 15:04")
	otherStart := mustUTC(t, "2026-06-15T09:00:00Z").Local().Format("2006-01-02 15:04")
	// aaaa1111 and aaaa9999 share /home/dev/myapp; no parent/children in the
	// claude-code fixture, so only the adjacent section fires
	want := fmt.Sprintf(`claude-code  /home/dev/myapp  %s  2 messages  %d B  aaaa1111

adjacent in project (1)
  claude-code  /home/dev/myapp  %s  1 message   %d B  aaaa9999
`,
		anchorStart, fixtureFileSize(t, home, ".claude/projects/C--Users-dev-myapp/aaaa1111-1111-4111-8111-111111111111.jsonl"),
		otherStart, fixtureFileSize(t, home, ".claude/projects/C--Users-dev-myapp/aaaa9999-9999-4999-8999-999999999999.jsonl"))
	if out != want {
		t.Errorf("related output:\n%s\nwant:\n%s", out, want)
	}
}

func TestRelatedJSON(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "related", "aaaa1111", "--json")
	if code != 0 {
		t.Fatalf("related --json exit = %d, stderr: %s", code, errOut)
	}
	var doc struct {
		Session struct {
			ID       string `json:"id"`
			ParentID string `json:"parent_id"`
		} `json:"session"`
		Parent   json.RawMessage `json:"parent"`
		Children []struct{}      `json:"children"`
		Adjacent []struct{}      `json:"adjacent"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if doc.Session.ID != "aaaa1111-1111-4111-8111-111111111111" {
		t.Errorf("session id = %q", doc.Session.ID)
	}
	if string(doc.Parent) != "null" {
		t.Errorf("parent = %s, want null", doc.Parent)
	}
	if len(doc.Adjacent) != 1 {
		t.Errorf("adjacent = %d entries, want 1 (aaaa9999 shares the project)", len(doc.Adjacent))
	}
}

func TestRelatedNoMatchExit2(t *testing.T) {
	home := searchShowHome(t)
	code, _, errOut := run(t, "--home", home, "related", "zzzz")
	if code != 2 {
		t.Errorf("no match exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, `no session matches id prefix "zzzz"`) {
		t.Errorf("stderr should explain the miss, got %q", errOut)
	}
}

func TestRelatedAmbiguousPrefixExit2(t *testing.T) {
	home := searchShowHome(t)
	code, _, errOut := run(t, "--home", home, "related", "aaaa")
	if code != 2 {
		t.Errorf("ambiguous prefix exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, `ambiguous session id prefix "aaaa"`) {
		t.Errorf("stderr should explain the ambiguity, got %q", errOut)
	}
}
