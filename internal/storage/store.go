package storage

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tanq16/inoichi/internal/mindmap"
)

const (
	dirMode  = 0o700
	fileMode = 0o600

	FlushDelay = 2 * time.Second

	maxRetryDelay = 30 * time.Second
)

var (
	ErrNotFound = errors.New("map not found")
	ErrTooLarge = errors.New("map is larger than the document limit")
	ErrClosed   = errors.New("store is closed")
)

type Store struct {
	OnWriteError func(id string, err error)

	dir        string
	flushDelay time.Duration

	mu     sync.Mutex
	maps   map[string]*mindmap.Map
	dirty  map[string]struct{}
	broken map[string]error
	closed bool

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func New(dir string) (*Store, error) {
	return newWithDelay(dir, FlushDelay)
}

func newWithDelay(dir string, flushDelay time.Duration) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	mapsDir := filepath.Join(abs, "maps")
	if err := os.MkdirAll(mapsDir, dirMode); err != nil {
		return nil, err
	}
	s := &Store{
		dir:        mapsDir,
		flushDelay: flushDelay,
		maps:       make(map[string]*mindmap.Map),
		dirty:      make(map[string]struct{}),
		broken:     make(map[string]error),
		wake:       make(chan struct{}, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	go s.run()
	return s, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) path(id string) (string, error) {
	if !mindmap.ValidID(id) {
		return "", fmt.Errorf("map id %q is not in the allowed format", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *Store) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		m, err := s.readFile(id)
		if err != nil {
			s.broken[e.Name()] = err
			continue
		}
		s.maps[id] = m
	}
	return nil
}

func (s *Store) readFile(id string) (*mindmap.Map, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
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

func (s *Store) List() ([]mindmap.Summary, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]mindmap.Summary, 0, len(s.maps))
	for _, m := range s.maps {
		out = append(out, m.Summary())
	}
	slices.SortFunc(out, func(a, b mindmap.Summary) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	unreadable := slices.Sorted(maps.Keys(s.broken))
	return out, unreadable, nil
}

func (s *Store) Load(id string) (*mindmap.Map, error) {
	if _, err := s.path(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.maps[id]; ok {
		return clone(m), nil
	}
	if err, ok := s.broken[id+".json"]; ok {
		return nil, err
	}
	return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
}

func (s *Store) Save(m *mindmap.Map) error {
	if _, err := s.path(m.ID); err != nil {
		return err
	}
	data, err := encode(m)
	if err != nil {
		return err
	}
	if len(data) > mindmap.MaxDocumentBytes {
		return fmt.Errorf("%w: %d bytes", ErrTooLarge, len(data))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	s.maps[m.ID] = clone(m)
	s.dirty[m.ID] = struct{}{}
	delete(s.broken, m.ID+".json")
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *Store) Delete(id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, known := s.maps[id]
	delete(s.maps, id)
	delete(s.dirty, id)
	delete(s.broken, id+".json")
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		if known {
			return nil
		}
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return err
}

func (s *Store) Count() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.maps), nil
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.closeErr = s.flush()
	})
	return s.closeErr
}

func (s *Store) run() {
	defer close(s.done)
	delay := s.flushDelay
	for {
		select {
		case <-s.stop:
			return
		case <-s.wake:
		}
		select {
		case <-s.stop:
			return
		case <-time.After(delay):
		}
		if err := s.flush(); err != nil {
			delay = min(delay*2, maxRetryDelay)
			select {
			case s.wake <- struct{}{}:
			default:
			}
			continue
		}
		delay = s.flushDelay
	}
}

// The write stays under the lock so a Delete cannot interleave with the rename that would bring a removed file back.
func (s *Store) flush() error {
	s.mu.Lock()
	ids := slices.Sorted(maps.Keys(s.dirty))
	s.mu.Unlock()

	var first error
	for _, id := range ids {
		s.mu.Lock()
		m, ok := s.maps[id]
		if _, still := s.dirty[id]; !ok || !still {
			s.mu.Unlock()
			continue
		}
		err := s.writeFile(m)
		if err == nil {
			delete(s.dirty, id)
		}
		cb := s.OnWriteError
		s.mu.Unlock()
		if err != nil {
			if first == nil {
				first = err
			}
			if cb != nil {
				cb(id, err)
			}
		}
	}
	return first
}

func (s *Store) writeFile(m *mindmap.Map) error {
	p, err := s.path(m.ID)
	if err != nil {
		return err
	}
	data, err := encode(m)
	if err != nil {
		return err
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

func encode(m *mindmap.Map) ([]byte, error) {
	return json.Marshal(m, jsontext.WithIndent("  "))
}

func clone(m *mindmap.Map) *mindmap.Map {
	c := *m
	c.Nodes = slices.Clone(m.Nodes)
	c.Links = slices.Clone(m.Links)
	return &c
}
