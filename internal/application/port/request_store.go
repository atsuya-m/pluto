package port

import (
	"context"
	"time"
)

type StoredRequest struct {
	Name    string
	RPC     string
	Data    []byte
	SavedAt time.Time
}

type RequestStore interface {
	LoadLast(ctx context.Context, rpc string) (StoredRequest, bool, error)
	SaveLast(ctx context.Context, req StoredRequest) error
	LoadNamed(ctx context.Context, name string) (StoredRequest, bool, error)
	SaveNamed(ctx context.Context, req StoredRequest) error
	ListNamed(ctx context.Context) ([]StoredRequest, error)
	DeleteNamed(ctx context.Context, name string) error
}
