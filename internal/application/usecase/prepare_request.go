package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/application/port"
	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type PrepareRequest struct {
	schemaLoader port.SchemaLoader
	decoder      port.RequestDecoder
}

type PrepareRequestInput struct {
	RPCName string
	Data    []byte
}

type PrepareRequestOutput struct {
	RPC      RPCSummary
	Input    schema.Message
	Builder  *request.DynamicMessageBuilder
	Messages []proto.Message
}

func NewPrepareRequest(schemaLoader port.SchemaLoader, decoder port.RequestDecoder) *PrepareRequest {
	return &PrepareRequest{schemaLoader: schemaLoader, decoder: decoder}
}

func (u *PrepareRequest) Execute(ctx context.Context, in PrepareRequestInput) (PrepareRequestOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return PrepareRequestOutput{}, err
	}
	r, err := s.ResolveRPC(in.RPCName)
	if err != nil {
		return PrepareRequestOutput{}, err
	}
	out := PrepareRequestOutput{
		RPC:     toRPCSummary(r),
		Input:   r.Input(),
		Builder: request.NewDynamicMessageBuilder(r.Input().Descriptor()),
	}
	if len(bytes.TrimSpace(in.Data)) == 0 {
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
