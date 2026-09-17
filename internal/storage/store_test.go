package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tanq16/inoichi/internal/mindmap"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatalf("New(%q) = %v", root, err)
	}
	return s, root
}

func sampleMap(id string) *mindmap.Map {
	root := mindmap.Node{ID: "rootnode", Text: "Root", Width: 200, Height: 56}
	child := mindmap.Node{ID: "childnode", ParentID: "rootnode", Text: "Child", Width: 180, Height: 48}
	m := &mindmap.Map{
		ID:     id,
		Title:  "Round trip",
		RootID: root.ID,
		Nodes:  []mindmap.Node{root, child},
		Links:  []mindmap.Link{},
	}
	m.Normalize()
	return m
}

func TestIDsThatWouldEscapeTheDataDirectory(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"parent traversal", "../escape"},
		{"directory separator", "maps/other"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newStore(t)
			if _, err := s.Load(tt.id); err == nil {
				t.Errorf("Load(%q) returned no error", tt.id)
			}
			if err := s.Delete(tt.id); err == nil {
				t.Errorf("Delete(%q) returned no error", tt.id)
			}
			m := sampleMap(tt.id)
			if err := s.Save(m); err == nil {
				t.Errorf("Save() accepted the id %q", tt.id)
			}
		})
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s, _ := newStore(t)
	want := sampleMap("abcdef0123456789abcdef01")
	if err := s.Save(want); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	got, err := s.Load(want.ID)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got.Title != want.Title || got.RootID != want.RootID || len(got.Nodes) != len(want.Nodes) {
		t.Fatalf("round trip changed the map: %+v", got.Summary())
	}
	if got.Nodes[1].ParentID != "rootnode" || got.Nodes[1].Text != "Child" {
		t.Errorf("child node came back as %+v", got.Nodes[1])
	}
	if err := got.Validate(); err != nil {
		t.Errorf("the loaded map does not validate: %v", err)
	}

	list, unreadable, err := s.List()
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(list) != 1 || list[0].ID != want.ID || list[0].NodeCount != 2 {
		t.Errorf("List() = %+v, want one summary of the saved map", list)
	}
	if len(unreadable) != 0 {
		t.Errorf("List() reported %v as unreadable", unreadable)
	}
}

func TestLoadRejectsAFileWhoseIDDisagreesWithItsName(t *testing.T) {
	s, _ := newStore(t)
	m := sampleMap("abcdef0123456789abcdef01")
	if err := s.Save(m); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := os.Rename(
		filepath.Join(s.Dir(), m.ID+".json"),
		filepath.Join(s.Dir(), "bbbbbbbbbbbbbbbbbbbbbbbb.json"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("bbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("Load() accepted a file carrying a different id")
	}
}

func TestListSkipsAnUnreadableFile(t *testing.T) {
	s, _ := newStore(t)
	good := sampleMap("abcdef0123456789abcdef01")
	if err := s.Save(good); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir(), "cccccccccccccccccccccccc.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	list, unreadable, err := s.List()
	if err != nil {
		t.Fatalf("List() = %v, want the good map rather than an error", err)
	}
	if len(list) != 1 || list[0].ID != good.ID {
		t.Errorf("List() = %+v, want only the readable map", list)
	}
	if len(unreadable) != 1 {
		t.Errorf("List() named %v as unreadable, want the one broken file", unreadable)
	}
}
