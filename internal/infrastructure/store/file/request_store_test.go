package file_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atsuya-m/pluto/internal/application/port"
	"github.com/atsuya-m/pluto/internal/infrastructure/store/file"
)

func TestRequestStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "requests.json")
	s := file.NewRequestStore(path)

	if _, ok, err := s.LoadLast(ctx, "a.B"); ok || err != nil {
		t.Fatalf("missing file: ok = %v, err = %v", ok, err)
	}

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if err := s.SaveLast(ctx, port.StoredRequest{RPC: "a.B", Data: []byte(`{"x":1}`), SavedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveNamed(ctx, port.StoredRequest{Name: "two", RPC: "a.B", Data: []byte(`{"x":2}`), SavedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveNamed(ctx, port.StoredRequest{Name: "one", RPC: "a.C", Data: []byte(`{}`), SavedAt: now}); err != nil {
		t.Fatal(err)
	}

	reopened := file.NewRequestStore(path)
	last, ok, err := reopened.LoadLast(ctx, "a.B")
	if err != nil || !ok || string(last.Data) != `{"x":1}` || !last.SavedAt.Equal(now) {
		t.Errorf("last = %+v, ok = %v, err = %v", last, ok, err)
	}
	named, ok, err := reopened.LoadNamed(ctx, "two")
	if err != nil || !ok || named.RPC != "a.B" || named.Name != "two" {
		t.Errorf("named = %+v, ok = %v, err = %v", named, ok, err)
	}
	list, err := reopened.ListNamed(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "one" || list[1].Name != "two" {
		t.Errorf("list = %+v, err = %v", list, err)
	}

	if err := reopened.DeleteNamed(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteNamed(ctx, "one"); err == nil {
		t.Error("deleting a missing name should fail")
	}
	if list, _ := s.ListNamed(ctx); len(list) != 1 {
		t.Errorf("list after delete = %+v", list)
	}
}

func TestRequestStore_Permissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pluto")
	path := filepath.Join(dir, "requests.json")
	if err := file.NewRequestStore(path).SaveLast(context.Background(), port.StoredRequest{RPC: "a.B", Data: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files should not be left behind: %v", entries)
	}
}

func TestRequestStore_CorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := file.NewRequestStore(path)
	if _, _, err := s.LoadLast(context.Background(), "a.B"); err == nil || !strings.Contains(err.Error(), "parse request store") {
		t.Errorf("err = %v", err)
	}
	if err := s.SaveLast(context.Background(), port.StoredRequest{RPC: "a.B"}); err == nil {
		t.Error("a corrupted file must not be overwritten silently")
	}
}
