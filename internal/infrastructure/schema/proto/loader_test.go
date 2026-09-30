package proto_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atsuya-m/pluto/internal/infrastructure/schema/proto"
)

var testdata = filepath.Join("..", "..", "..", "..", "testdata", "proto")

func TestSchemaLoader_Directory(t *testing.T) {
	s, err := proto.NewSchemaLoader([]string{testdata}, nil).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Services()) != 2 || len(s.RPCs()) != 10 {
		t.Errorf("services = %d, rpcs = %d", len(s.Services()), len(s.RPCs()))
	}
	if _, err := s.ResolveMessage("google.protobuf.Timestamp"); err != nil {
		t.Errorf("well-known imports should be resolved: %v", err)
	}
}

func TestSchemaLoader_SingleFileWithImportPath(t *testing.T) {
	file := filepath.Join(testdata, "admin", "v1", "admin.proto")
	s, err := proto.NewSchemaLoader([]string{file}, []string{testdata}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveRPC("BanUser"); err != nil {
		t.Error(err)
	}
}

func TestSchemaLoader_SkipsHiddenAndVendorDirs(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/v1/a.proto", `syntax = "proto3"; package a.v1; service A { rpc Do(Req) returns (Req); } message Req {}`)
	write(".git/x.proto", `this is not proto`)
	write("vendor/y.proto", `this is not proto`)
	write("node_modules/z.proto", `this is not proto`)

	s, err := proto.NewSchemaLoader([]string{dir}, nil).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.RPCs()) != 1 {
		t.Errorf("rpcs = %d, want 1", len(s.RPCs()))
	}
}

func TestSchemaLoader_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := proto.NewSchemaLoader([]string{"does-not-exist"}, nil).Load(ctx); err == nil {
		t.Error("expected error for missing path")
	}
	if _, err := proto.NewSchemaLoader([]string{t.TempDir()}, nil).Load(ctx); err == nil || !strings.Contains(err.Error(), "no .proto files") {
		t.Errorf("empty dir error = %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.proto"), []byte(`syntax = "proto3"; message {`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := proto.NewSchemaLoader([]string{dir}, nil).Load(ctx); err == nil || !strings.Contains(err.Error(), "compile proto") {
		t.Errorf("syntax error = %v", err)
	}
}
