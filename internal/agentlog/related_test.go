package agentlog

import (
	"errors"
	"testing"
	"time"
)

func mkMeta(id, parent, project string, at time.Time) SessionMeta {
	return SessionMeta{Session: Session{ID: id, Agent: "opencode", Project: project, ParentID: parent, StartedAt: at}}
}

func TestRelatedParentSession(t *testing.T) {
	anchor := mkMeta("parent2", "", "/dev/app", t2)
	a := &metaFake{fakeAdapter: fakeAdapter{name: "opencode"}, metas: []SessionMeta{
		mkMeta("parent1", "", "/dev/app", t1),
		anchor,
		mkMeta("child1", "parent2", "/dev/app", t2.Add(time.Minute)),
		mkMeta("next1", "", "/dev/app", t3),
		mkMeta("other1", "", "/dev/web", t3),
	}}

	rel := Related([]Adapter{a}, anchor)
	if rel.Parent != nil {
		t.Errorf("Parent = %v, want nil for a top-level session", rel.Parent.ID)
	}
	if len(rel.Children) != 1 || rel.Children[0].ID != "child1" {
		t.Errorf("Children = %v, want [child1]", rel.Children)
	}
	// adjacency: same agent+project neighbors by time, children excluded
	if len(rel.Adjacent) != 2 || rel.Adjacent[0].ID != "parent1" || rel.Adjacent[1].ID != "next1" {
		t.Errorf("Adjacent = %v, want [parent1 next1]", rel.Adjacent)
	}
}

func TestRelatedChildSession(t *testing.T) {
	anchor := mkMeta("child1", "parent2", "/dev/app", t2.Add(time.Minute))
	a := &metaFake{fakeAdapter: fakeAdapter{name: "opencode"}, metas: []SessionMeta{
		mkMeta("parent1", "", "/dev/app", t1),
		mkMeta("parent2", "", "/dev/app", t2),
		anchor,
		mkMeta("next1", "", "/dev/app", t3),
	}}

	rel := Related([]Adapter{a}, anchor)
	if rel.Parent == nil || rel.Parent.ID != "parent2" {
		t.Errorf("Parent = %v, want parent2", rel.Parent)
	}
	if len(rel.Children) != 0 {
		t.Errorf("Children = %v, want none", rel.Children)
	}
	// the parent is shown in its own section, so adjacency skips it
	if len(rel.Adjacent) != 2 || rel.Adjacent[0].ID != "parent1" || rel.Adjacent[1].ID != "next1" {
		t.Errorf("Adjacent = %v, want [parent1 next1]", rel.Adjacent)
	}
}

func TestRelatedAdjacentCapsAtTwoPerSide(t *testing.T) {
	anchor := mkMeta("anchor", "", "/dev/app", t2)
	a := &metaFake{fakeAdapter: fakeAdapter{name: "opencode"}, metas: []SessionMeta{
		mkMeta("b1", "", "/dev/app", t1),
		mkMeta("b2", "", "/dev/app", t1.Add(time.Minute)),
		mkMeta("b3", "", "/dev/app", t1.Add(2*time.Minute)),
		anchor,
		mkMeta("f1", "", "/dev/app", t3),
		mkMeta("f2", "", "/dev/app", t3.Add(time.Minute)),
		mkMeta("f3", "", "/dev/app", t3.Add(2*time.Minute)),
	}}

	rel := Related([]Adapter{a}, anchor)
	if len(rel.Adjacent) != 4 {
		t.Fatalf("Adjacent = %v, want 4 (two per side)", rel.Adjacent)
	}
	for _, id := range []string{"b2", "b3", "f1", "f2"} {
		found := false
		for _, m := range rel.Adjacent {
			if m.ID == id {
				found = true
			}
		}
		if !found {
			t.Errorf("Adjacent should contain %s, got %v", id, rel.Adjacent)
		}
	}
}

func TestRelatedSkipsUnknownProject(t *testing.T) {
	anchor := mkMeta("anchor", "", "", t2) // unknown project
	a := &metaFake{fakeAdapter: fakeAdapter{name: "opencode"}, metas: []SessionMeta{
		mkMeta("other", "", "", t1),
		anchor,
	}}
	rel := Related([]Adapter{a}, anchor)
	if len(rel.Adjacent) != 0 {
		t.Errorf("Adjacent = %v, want none without a project", rel.Adjacent)
	}
}

func TestRelatedNotesUnreadableAdapters(t *testing.T) {
	anchor := mkMeta("anchor", "", "/dev/app", t2)
	ok := &metaFake{metas: []SessionMeta{anchor}}
	broken := &metaFake{scanErr: errors.New("boom")}
	broken.name = "gemini-cli"

	rel := Related([]Adapter{ok, broken}, anchor)
	if len(rel.Unreadable) != 1 || rel.Unreadable[0] != "gemini-cli: boom" {
		t.Errorf("Unreadable = %v, want one gemini-cli note", rel.Unreadable)
	}
}
