package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/application/port"
)

var ErrNoRequestStore = errors.New("request store is not configured")

var savedNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type SavedRequestSummary struct {
	Name    string
	RPC     string
	SavedAt time.Time
}

type SaveRequest struct {
	store port.RequestStore
	now   func() time.Time
}

type SaveRequestInput struct {
	RPCName string
	Name    string
	Message proto.Message
}

func NewSaveRequest(store port.RequestStore) *SaveRequest {
	return &SaveRequest{store: store, now: time.Now}
}

func (u *SaveRequest) Execute(ctx context.Context, in SaveRequestInput) error {
	if u.store == nil {
		return ErrNoRequestStore
	}
	data, err := marshalCompact(in.Message)
	if err != nil {
		return err
	}
	req := port.StoredRequest{Name: in.Name, RPC: in.RPCName, Data: data, SavedAt: u.now()}
	if in.Name == "" {
		return u.store.SaveLast(ctx, req)
	}
	if !savedNamePattern.MatchString(in.Name) {
		return fmt.Errorf("invalid name %q (use letters, digits, '.', '_' or '-')", in.Name)
	}
	return u.store.SaveNamed(ctx, req)
}

type ListSavedRequests struct {
	store port.RequestStore
}

func NewListSavedRequests(store port.RequestStore) *ListSavedRequests {
	return &ListSavedRequests{store: store}
}

func (u *ListSavedRequests) Execute(ctx context.Context) ([]SavedRequestSummary, error) {
	if u.store == nil {
		return nil, nil
	}
	stored, err := u.store.ListNamed(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SavedRequestSummary, 0, len(stored))
	for _, s := range stored {
		out = append(out, SavedRequestSummary{Name: s.Name, RPC: s.RPC, SavedAt: s.SavedAt})
	}
	return out, nil
}

type DeleteSavedRequest struct {
	store port.RequestStore
}

func NewDeleteSavedRequest(store port.RequestStore) *DeleteSavedRequest {
	return &DeleteSavedRequest{store: store}
}

func (u *DeleteSavedRequest) Execute(ctx context.Context, name string) error {
	if u.store == nil {
		return ErrNoRequestStore
	}
	return u.store.DeleteNamed(ctx, name)
}

func marshalCompact(msg proto.Message) ([]byte, error) {
	b, err := protojson.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return []byte("{}"), nil
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
