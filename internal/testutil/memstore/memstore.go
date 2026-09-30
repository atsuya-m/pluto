package memstore

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/atsuya-m/pluto/internal/application/port"
)

type Store struct {
	mu    sync.Mutex
	last  map[string]port.StoredRequest
	saved map[string]port.StoredRequest
}

func New() *Store {
	return &Store{last: map[string]port.StoredRequest{}, saved: map[string]port.StoredRequest{}}
}

func (s *Store) LoadLast(_ context.Context, rpc string) (port.StoredRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.last[rpc]
	return r, ok, nil
}

func (s *Store) SaveLast(_ context.Context, req port.StoredRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last[req.RPC] = req
	return nil
}

func (s *Store) LoadNamed(_ context.Context, name string) (port.StoredRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.saved[name]
	return r, ok, nil
}

func (s *Store) SaveNamed(_ context.Context, req port.StoredRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[req.Name] = req
	return nil
}

func (s *Store) ListNamed(_ context.Context) ([]port.StoredRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]port.StoredRequest, 0, len(s.saved))
	for _, r := range s.saved {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) DeleteNamed(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.saved[name]; !ok {
		return fmt.Errorf("saved request %q not found", name)
	}
	delete(s.saved, name)
	return nil
}
