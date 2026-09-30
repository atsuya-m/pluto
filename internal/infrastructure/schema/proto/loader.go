package proto

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type SchemaLoader struct {
	paths       []string
	importPaths []string
}

func NewSchemaLoader(paths, importPaths []string) *SchemaLoader {
	return &SchemaLoader{paths: paths, importPaths: importPaths}
}

func (l *SchemaLoader) Load(ctx context.Context) (*schema.Schema, error) {
	files, importPaths, err := l.collect()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .proto files found in %s", strings.Join(l.paths, ", "))
	}

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: importPaths,
		}),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
	compiled, err := compiler.Compile(ctx, files...)
	if err != nil {
		return nil, fmt.Errorf("compile proto: %w", err)
	}

	registry := new(protoregistry.Files)
	for _, f := range compiled {
		if err := registerWithDeps(registry, f); err != nil {
			return nil, err
		}
	}
	return schema.New(registry)
}

func (l *SchemaLoader) collect() ([]string, []string, error) {
	importPaths := append([]string(nil), l.importPaths...)
	seen := map[string]bool{}
	var files []string

	addFile := func(name string) {
		if !seen[name] {
			seen[name] = true
			files = append(files, name)
		}
	}

	for _, p := range l.paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, nil, fmt.Errorf("schema path: %w", err)
		}
		if !info.IsDir() {
			rel, root := relativeTo(p, importPaths)
			if root == "" {
				root = filepath.Dir(p)
				rel = filepath.Base(p)
				importPaths = append(importPaths, root)
			}
			addFile(filepath.ToSlash(rel))
			continue
		}

		root := p
		if !slices.Contains(importPaths, root) {
			importPaths = append(importPaths, root)
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".proto" {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			addFile(filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	sort.Strings(files)
	return files, importPaths, nil
}

func relativeTo(path string, roots []string) (string, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", ""
	}
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, abs)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return rel, root
		}
	}
	return "", ""
}
