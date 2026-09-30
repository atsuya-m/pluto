package reflection

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type Client interface {
	ListServices(ctx context.Context) ([]string, error)
	FileContainingSymbol(ctx context.Context, symbol string) ([]*descriptorpb.FileDescriptorProto, error)
	FileByFilename(ctx context.Context, filename string) ([]*descriptorpb.FileDescriptorProto, error)
}

type SchemaLoader struct {
	client Client
}

func NewSchemaLoader(client Client) *SchemaLoader {
	return &SchemaLoader{client: client}
}

func (l *SchemaLoader) Load(ctx context.Context) (*schema.Schema, error) {
	services, err := l.client.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("server reflection: list services: %w", err)
	}

	var exposed []string
	files := map[string]*descriptorpb.FileDescriptorProto{}
	add := func(fdps []*descriptorpb.FileDescriptorProto) {
		for _, fdp := range fdps {
			if _, ok := files[fdp.GetName()]; !ok {
				files[fdp.GetName()] = fdp
			}
		}
	}

	for _, svc := range services {
		if isReflectionService(svc) {
			continue
		}
		fdps, err := l.client.FileContainingSymbol(ctx, svc)
		if err != nil {
			return nil, fmt.Errorf("server reflection: resolve %s: %w", svc, err)
		}
		add(fdps)
		exposed = append(exposed, svc)
	}

	if err := l.fillMissingDeps(ctx, files, add); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	set := &descriptorpb.FileDescriptorSet{}
	for _, name := range names {
		set.File = append(set.File, files[name])
	}

	registry, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("server reflection: build descriptors: %w", err)
	}
	return schema.New(registry, schema.OnlyServices(exposed...))
}

func (l *SchemaLoader) fillMissingDeps(ctx context.Context, files map[string]*descriptorpb.FileDescriptorProto, add func([]*descriptorpb.FileDescriptorProto)) error {
	for {
		missing := missingDeps(files)
		if len(missing) == 0 {
			return nil
		}
		for _, name := range missing {
			if fdps, err := l.client.FileByFilename(ctx, name); err == nil && len(fdps) > 0 {
				add(fdps)
				continue
			}
			fd, err := protoregistry.GlobalFiles.FindFileByPath(name)
			if err != nil {
				return fmt.Errorf("server reflection: dependency %s is not available", name)
			}
			add([]*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(fd)})
		}
	}
}

func missingDeps(files map[string]*descriptorpb.FileDescriptorProto) []string {
	seen := map[string]bool{}
	var missing []string
	for _, fdp := range files {
		for _, dep := range fdp.GetDependency() {
			if _, ok := files[dep]; !ok && !seen[dep] {
				seen[dep] = true
				missing = append(missing, dep)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func isReflectionService(name string) bool {
	return strings.HasPrefix(name, "grpc.reflection.")
}
