package storage

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tanq16/inoichi/internal/mindmap"
)

const (
	dirMode  = 0o700
	fileMode = 0o600
)

var (
	ErrNotFound = errors.New("map not found")
	ErrTooLarge = errors.New("map is larger than the document limit")
)

type Store struct {
	dir string
}

func New(dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	mapsDir := filepath.Join(abs, "maps")
	if err := os.MkdirAll(mapsDir, dirMode); err != nil {
		return nil, err
	}
	return &Store{dir: mapsDir}, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) path(id string) (string, error) {
	if !mindmap.ValidID(id) {
		return "", fmt.Errorf("map id %q is not in the allowed format", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *Store) List() ([]mindmap.Summary, []string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, nil, err
	}
	out := make([]mindmap.Summary, 0, len(entries))
	var unreadable []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		m, err := s.Load(id)
		if err != nil {
			unreadable = append(unreadable, e.Name())
			continue
		}
		out = append(out, m.Summary())
	}
	slices.SortFunc(out, func(a, b mindmap.Summary) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return out, unreadable, nil
}

func (s *Store) Load(id string) (*mindmap.Map, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var m mindmap.Map
	if err := json.UnmarshalRead(io.LimitReader(f, mindmap.MaxDocumentBytes), &m); err != nil {
		return nil, fmt.Errorf("map %q is not readable JSON: %w", id, err)
	}
	if m.ID != id {
		return nil, fmt.Errorf("map file %q carries id %q", id, m.ID)
	}
	return &m, nil
}

func (s *Store) Save(m *mindmap.Map) error {
	p, err := s.path(m.ID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(m, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	if len(data) > mindmap.MaxDocumentBytes {
		return fmt.Errorf("%w: %d bytes", ErrTooLarge, len(data))
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(fileMode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}

func (s *Store) Delete(id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Store) Count() (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n, nil
}
