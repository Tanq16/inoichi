package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tanq16/inoichi/internal/mindmap"
)

const testID = "abcdef0123456789abcdef01"

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatalf("New(%q) = %v", root, err)
	}
	t.Cleanup(func() { s.Close() })
	return s, root
}

func newFastStore(t *testing.T, root string) *Store {
	t.Helper()
	s, err := newWithDelay(root, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("newWithDelay(%q) = %v", root, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
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

func fileOf(s *Store, id string) string { return filepath.Join(s.Dir(), id+".json") }

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", path, err)
	}
	return string(data)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
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
	want := sampleMap(testID)
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

func TestLoadAfterSaveAnswersBeforeAnyFlush(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if _, err := os.Stat(fileOf(s, testID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Save() reached disk on the call path, Stat = %v", err)
	}
	got, err := s.Load(testID)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got.Title != "Round trip" {
		t.Errorf("Load() = %+v, want the map just saved", got.Summary())
	}
	if n, _ := s.Count(); n != 1 {
		t.Errorf("Count() = %d, want 1", n)
	}
}

func TestLoadHandsOutACopy(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	first, _ := s.Load(testID)
	first.Title = "changed"
	first.Nodes[0].Text = "changed"
	second, _ := s.Load(testID)
	if second.Title != "Round trip" || second.Nodes[0].Text != "Root" {
		t.Errorf("mutating a loaded map changed the cache: %+v", second.Summary())
	}
}

func TestSaveThenCloseWritesTheFile(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	info, err := os.Stat(fileOf(s, testID))
	if err != nil {
		t.Fatalf("no file after Close: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Errorf("file mode = %o, want %o", info.Mode().Perm(), fileMode)
	}
	if !strings.Contains(readFile(t, fileOf(s, testID)), `"title": "Round trip"`) {
		t.Errorf("the file does not carry the saved map")
	}
	if err := s.Save(sampleMap(testID)); !errors.Is(err, ErrClosed) {
		t.Errorf("Save() after Close = %v, want ErrClosed", err)
	}
}

func TestRepeatedSavesLeaveTheFinalContent(t *testing.T) {
	s, _ := newStore(t)
	m := sampleMap(testID)
	if err := s.Save(m); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	m.Title = "Second"
	if err := s.Save(m); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if !strings.Contains(readFile(t, fileOf(s, testID)), `"title": "Second"`) {
		t.Errorf("the file does not carry the last save")
	}
}

func TestFlushReachesDiskWithoutClose(t *testing.T) {
	s := newFastStore(t, t.TempDir())
	if err := s.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	waitUntil(t, "the flush", func() bool {
		_, err := os.Stat(fileOf(s, testID))
		return err == nil
	})
}

func TestDeleteDropsADirtyMap(t *testing.T) {
	s, _ := newStore(t)
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

func TestNewLoadsExistingFiles(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, err := second.Load(testID)
	if err != nil {
		t.Fatalf("Load() from a fresh store = %v", err)
	}
	if got.Title != "Round trip" || len(got.Nodes) != 2 {
		t.Errorf("Load() = %+v, want the map written by the first store", got.Summary())
	}
	if n, _ := second.Count(); n != 1 {
		t.Errorf("Count() = %d, want 1", n)
	}
}

func TestLoadRejectsAFileWhoseIDDisagreesWithItsName(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	const other = "bbbbbbbbbbbbbbbbbbbbbbbb"
	if err := os.Rename(fileOf(first, testID), fileOf(first, other)); err != nil {
		t.Fatal(err)
	}

	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Load(other); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() = %v, want a parse error for a file carrying a different id", err)
	}
	if _, unreadable, _ := second.List(); len(unreadable) != 1 || unreadable[0] != other+".json" {
		t.Errorf("List() named %v as unreadable, want the renamed file", unreadable)
	}
}

func TestListSkipsAnUnreadableFile(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := os.WriteFile(filepath.Join(first.Dir(), "cccccccccccccccccccccccc.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(root)
	if err != nil {
		t.Fatalf("New() = %v, want the good map rather than an error", err)
	}
	defer s.Close()
	list, unreadable, err := s.List()
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(list) != 1 || list[0].ID != testID {
		t.Errorf("List() = %+v, want only the readable map", list)
	}
	if len(unreadable) != 1 {
		t.Errorf("List() named %v as unreadable, want the one broken file", unreadable)
	}
}

func TestWriteErrorIsReportedAndRetried(t *testing.T) {
	root := t.TempDir()
	s := newFastStore(t, root)
	reported := make(chan error, 8)
	s.OnWriteError = func(id string, err error) {
		if id == testID {
			reported <- err
		}
	}

	away := s.Dir() + ".away"
	if err := os.Rename(s.Dir(), away); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			os.Rename(away, s.Dir())
		}
	}
	defer restore()

	if err := s.Save(sampleMap(testID)); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	select {
	case err := <-reported:
		if err == nil {
			t.Fatal("OnWriteError was called with a nil error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnWriteError was never called")
	}
	if got, err := s.Load(testID); err != nil || got.Title != "Round trip" {
		t.Fatalf("Load() after a failed write = %+v, %v", got, err)
	}

	restore()
	waitUntil(t, "the retried write", func() bool {
		_, err := os.Stat(fileOf(s, testID))
		return err == nil
	})
	if !strings.Contains(readFile(t, fileOf(s, testID)), `"title": "Round trip"`) {
		t.Errorf("the retried file does not carry the saved map")
	}
}
