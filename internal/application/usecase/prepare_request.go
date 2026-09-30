package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/atsuya-m/pluto/internal/application/port"
	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type PrepareRequest struct {
	schemaLoader port.SchemaLoader
	decoder      port.RequestDecoder
	store        port.RequestStore
}

type PrepareOption func(*PrepareRequest)

func WithRequestStore(store port.RequestStore) PrepareOption {
	return func(u *PrepareRequest) { u.store = store }
}

type PrepareRequestInput struct {
	RPCName     string
	Data        []byte
	RestoreLast bool
	SavedName   string
}

type PrepareRequestOutput struct {
	RPC      RPCSummary
	Input    schema.Message
	Builder  *request.DynamicMessageBuilder
	Messages []proto.Message

	RestoredFrom   string
	RestoredAt     time.Time
	RestoreWarning string
}

func splitJSONValues(data []byte) ([][]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var values [][]byte
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return values, nil
		}
		if err != nil {
			return nil, fmt.Errorf("split request json stream: %w", err)
		}
		if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
			var items []json.RawMessage
			if err := json.Unmarshal(trimmed, &items); err != nil {
				return nil, fmt.Errorf("split request json array: %w", err)
			}
			for _, item := range items {
				values = append(values, item)
			}
			continue
		}
		values = append(values, raw)
	}
}

func NewPrepareRequest(schemaLoader port.SchemaLoader, decoder port.RequestDecoder, opts ...PrepareOption) *PrepareRequest {
	u := &PrepareRequest{schemaLoader: schemaLoader, decoder: decoder}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

func (u *PrepareRequest) Execute(ctx context.Context, in PrepareRequestInput) (PrepareRequestOutput, error) {
	var saved port.StoredRequest
	if in.SavedName != "" {
		if u.store == nil {
			return PrepareRequestOutput{}, ErrNoRequestStore
		}
		var ok bool
		var err error
		saved, ok, err = u.store.LoadNamed(ctx, in.SavedName)
		if err != nil {
			return PrepareRequestOutput{}, err
		}
		if !ok {
			return PrepareRequestOutput{}, fmt.Errorf("saved request %q not found", in.SavedName)
		}
		if in.RPCName == "" {
			in.RPCName = saved.RPC
		}
	}

	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return PrepareRequestOutput{}, err
	}
	r, err := s.ResolveRPC(in.RPCName)
	if err != nil {
		return PrepareRequestOutput{}, err
	}
	if in.SavedName != "" {
		if saved.RPC != r.FullName() {
			return PrepareRequestOutput{}, fmt.Errorf("saved request %q is for %s, not %s", in.SavedName, saved.RPC, r.FullName())
		}
		msg, err := u.decoder.Decode(r.Input().Descriptor(), saved.Data)
		if err != nil {
			return PrepareRequestOutput{}, fmt.Errorf("saved request %q no longer matches the schema: %w", in.SavedName, err)
		}
		return PrepareRequestOutput{
			RPC:          toRPCSummary(r),
			Input:        r.Input(),
			Builder:      request.NewDynamicMessageBuilderFrom(msg),
			Messages:     []proto.Message{msg},
			RestoredFrom: in.SavedName,
			RestoredAt:   saved.SavedAt,
		}, nil
	}
	out := PrepareRequestOutput{
		RPC:     toRPCSummary(r),
		Input:   r.Input(),
		Builder: request.NewDynamicMessageBuilder(r.Input().Descriptor()),
	}
	if len(bytes.TrimSpace(in.Data)) == 0 {
		if in.RestoreLast && u.store != nil {
			u.restoreLast(ctx, r.Input().Descriptor(), &out)
		}
		return out, nil
	}

	values := [][]byte{in.Data}
	if r.ClientStreaming() {
		if values, err = splitJSONValues(in.Data); err != nil {
			return PrepareRequestOutput{}, err
		}
	}
	for _, v := range values {
		msg, err := u.decoder.Decode(r.Input().Descriptor(), v)
		if err != nil {
			return PrepareRequestOutput{}, err
		}
		out.Messages = append(out.Messages, msg)
	}
	out.Builder = request.NewDynamicMessageBuilderFrom(out.Messages[0])
	return out, nil
}

func (u *PrepareRequest) restoreLast(ctx context.Context, desc protoreflect.MessageDescriptor, out *PrepareRequestOutput) {
	last, ok, err := u.store.LoadLast(ctx, out.RPC.FullName)
	if err != nil {
		out.RestoreWarning = err.Error()
		return
	}
	if !ok {
		return
	}
	msg, err := u.decoder.Decode(desc, last.Data)
	if err != nil {
		out.RestoreWarning = "last request no longer matches the schema: " + err.Error()
		return
	}
	out.Builder = request.NewDynamicMessageBuilderFrom(msg)
	out.RestoredFrom = "last"
	out.RestoredAt = last.SavedAt
}
