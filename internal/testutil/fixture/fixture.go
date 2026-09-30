package fixture

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/atsuya-m/pluto/internal/domain/schema"
)

//go:embed proto
var protoFS embed.FS

var (
	once     sync.Once
	registry *protoregistry.Files
	loadErr  error
)

func Files() []string {
	var files []string
	_ = fs.WalkDir(protoFS, "proto", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".proto") {
			files = append(files, strings.TrimPrefix(path, "proto/"))
		}
		return nil
	})
	return files
}

func Source(path string) ([]byte, error) {
	return protoFS.ReadFile("proto/" + path)
}

func Registry(t testing.TB) *protoregistry.Files {
	t.Helper()
	once.Do(func() {
		compiler := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				Accessor: func(path string) (io.ReadCloser, error) {
					return protoFS.Open("proto/" + path)
				},
			}),
		}
		compiled, err := compiler.Compile(context.Background(), Files()...)
		if err != nil {
			loadErr = err
			return
		}
		registry = new(protoregistry.Files)
		for _, f := range compiled {
			if err := register(registry, f); err != nil {
				loadErr = err
				return
			}
		}
	})
	if loadErr != nil {
		t.Fatalf("compile fixture: %v", loadErr)
	}
	return registry
}

func register(r *protoregistry.Files, fd protoreflect.FileDescriptor) error {
	if _, err := r.FindFileByPath(fd.Path()); err == nil {
		return nil
	}
	for i := 0; i < fd.Imports().Len(); i++ {
		if err := register(r, fd.Imports().Get(i).FileDescriptor); err != nil {
			return err
		}
	}
	return r.RegisterFile(fd)
}

func Schema(t testing.TB) *schema.Schema {
	t.Helper()
	s, err := schema.New(Registry(t))
	if err != nil {
		t.Fatalf("schema.New: %v", err)
	}
	return s
}

func Message(t testing.TB, name string) protoreflect.MessageDescriptor {
	t.Helper()
	d, err := Registry(t).FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		t.Fatalf("find %s: %v", name, err)
	}
	md, ok := d.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("%s is not a message", name)
	}
	return md
}

func RPC(t testing.TB, name string) schema.RPC {
	t.Helper()
	r, err := Schema(t).ResolveRPC(name)
	if err != nil {
		t.Fatalf("resolve %s: %v", name, err)
	}
	return r
}

type SchemaLoader struct {
	Schema *schema.Schema
	Err    error
	Calls  int
}

func NewSchemaLoader(t testing.TB) *SchemaLoader {
	return &SchemaLoader{Schema: Schema(t)}
}

func (l *SchemaLoader) Load(context.Context) (*schema.Schema, error) {
	l.Calls++
	return l.Schema, l.Err
}
