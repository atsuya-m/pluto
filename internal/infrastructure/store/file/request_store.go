package file

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/atsuya-m/pluto/internal/application/port"
)

type entry struct {
	RPC     string          `json:"rpc"`
	Data    json.RawMessage `json:"data"`
	SavedAt time.Time       `json:"saved_at"`
}

type document struct {
	Version int              `json:"version"`
	Last    map[string]entry `json:"last"`
	Saved   map[string]entry `json:"saved"`
}

type RequestStore struct {
	path string
	mu   sync.Mutex
}

func NewRequestStore(path string) *RequestStore {
	return &RequestStore{path: path}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pluto", "requests.json"), nil
}

func (s *RequestStore) LoadLast(_ context.Context, rpc string) (port.StoredRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read()
	if err != nil {
		return port.StoredRequest{}, false, err
	}
	e, ok := doc.Last[rpc]
	return toStored("", e), ok, nil
}

func (s *RequestStore) SaveLast(_ context.Context, req port.StoredRequest) error {
	return s.update(func(doc *document) error {
		doc.Last[req.RPC] = toEntry(req)
		return nil
	})
}

func (s *RequestStore) LoadNamed(_ context.Context, name string) (port.StoredRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read()
	if err != nil {
		return port.StoredRequest{}, false, err
	}
	e, ok := doc.Saved[name]
	return toStored(name, e), ok, nil
}

func (s *RequestStore) SaveNamed(_ context.Context, req port.StoredRequest) error {
	return s.update(func(doc *document) error {
		doc.Saved[req.Name] = toEntry(req)
		return nil
	})
}

func (s *RequestStore) ListNamed(_ context.Context) ([]port.StoredRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read()
	if err != nil {
		return nil, err
	}
	out := make([]port.StoredRequest, 0, len(doc.Saved))
	for name, e := range doc.Saved {
		out = append(out, toStored(name, e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *RequestStore) DeleteNamed(_ context.Context, name string) error {
	return s.update(func(doc *document) error {
		if _, ok := doc.Saved[name]; !ok {
			return fmt.Errorf("saved request %q not found", name)
		}
		delete(doc.Saved, name)
		return nil
	})
}

func (s *RequestStore) update(fn func(*document) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read()
	if err != nil {
		return err
	}
	if err := fn(&doc); err != nil {
		return err
	}
	return s.write(doc)
}

func (s *RequestStore) read() (document, error) {
	doc := document{Version: 1, Last: map[string]entry{}, Saved: map[string]entry{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, fmt.Errorf("read request store: %w", err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return doc, fmt.Errorf("parse request store %s: %w", s.path, err)
	}
	if doc.Last == nil {
		doc.Last = map[string]entry{}
	}
	if doc.Saved == nil {
		doc.Saved = map[string]entry{}
	}
	return doc, nil
}

func (s *RequestStore) write(doc document) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create request store dir: %w", err)
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".requests-*.json")
	if err != nil {
		return fmt.Errorf("write request store: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write request store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write request store: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("write request store: %w", err)
	}
	return os.Rename(tmp.Name(), s.path)
}

func toEntry(req port.StoredRequest) entry {
	return entry{RPC: req.RPC, Data: json.RawMessage(req.Data), SavedAt: req.SavedAt.UTC()}
}

func toStored(name string, e entry) port.StoredRequest {
	data := []byte(e.Data)
	var compact bytes.Buffer
	if err := json.Compact(&compact, e.Data); err == nil {
		data = compact.Bytes()
	}
	return port.StoredRequest{Name: name, RPC: e.RPC, Data: data, SavedAt: e.SavedAt}
}
