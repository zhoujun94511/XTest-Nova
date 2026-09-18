package configstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu   sync.RWMutex
	path string
	data map[string]any
}

func New(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]any{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}
	if s.data == nil {
		s.data = map[string]any{}
	}
	return s, nil
}

// NewEmpty returns a store without reading its path.
func NewEmpty(path string) *Store {
	return &Store{path: path, data: map[string]any{}}
}
func (s *Store) Get() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
func (s *Store) Replace(v map[string]any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := filepath.Dir(s.path); d != "." {
		if e = os.MkdirAll(d, 0755); e != nil {
			return e
		}
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, s.path); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	s.data = v
	return nil
}
