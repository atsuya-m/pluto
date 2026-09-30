package port

import (
	"context"

	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type SchemaLoader interface {
	Load(ctx context.Context) (*schema.Schema, error)
}
