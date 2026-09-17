package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tanq16/inoichi/internal/mindmap"
)

const testID = "abcdef0123456789abcdef01"

func newStore(t *testing.T, root string, flushDelay time.Duration) *Store {
	t.Helper()
	s, err := newWithDelay(root, flushDelay)
	if err != nil {
		t.Fatalf("newWithDelay(%q) = %v", root, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleMap(id string) *mindmap.Map {
	root := mindmap.Node{ID: "rootnode", Text: "Root", Width: 200, Height: 56}
	m := &mindmap.Map{ID: id, Title: "Round trip", RootID: root.ID, Nodes: []mindmap.Node{root}}
	m.Normalize()
	return m
}

func fileOf(s *Store, id string) string { return filepath.Join(s.Dir(), id+".json") }

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
			s := newStore(t, t.TempDir(), FlushDelay)
			if _, err := s.Load(tt.id); err == nil {
				t.Errorf("Load(%q) returned no error", tt.id)
			}
			if err := s.Delete(tt.id); err == nil {
				t.Errorf("Delete(%q) returned no error", tt.id)
			}
			if err := s.Save(sampleMap(tt.id)); err == nil {
				t.Errorf("Save() accepted the id %q", tt.id)
			}
		})
	}
}

func TestDeleteDropsADirtyMap(t *testing.T) {
	s := newStore(t, t.TempDir(), FlushDelay)
	if err := s.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := s.Delete(testID); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	if _, err := s.Load(testID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load() after Delete = %v, want ErrNotFound", err)
	}
	if err := s.Delete(testID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete() = %v, want ErrNotFound", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if _, err := os.Stat(fileOf(s, testID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a deleted map reached disk, Stat = %v", err)
	}
}
