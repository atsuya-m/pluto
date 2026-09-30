package cached

import (
	"context"
	"sync"

	"github.com/atsuya-m/pluto/internal/application/port"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type SchemaLoader struct {
	loader port.SchemaLoader

	once   sync.Once
	schema *schema.Schema
	err    error
}

func NewSchemaLoader(loader port.SchemaLoader) *SchemaLoader {
	return &SchemaLoader{loader: loader}
}

func (l *SchemaLoader) Load(ctx context.Context) (*schema.Schema, error) {
	l.once.Do(func() {
		l.schema, l.err = l.loader.Load(ctx)
	})
	return l.schema, l.err
}
