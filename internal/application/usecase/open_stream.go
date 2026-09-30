package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/application/port"
)

type OpenStream struct {
	schemaLoader port.SchemaLoader
	opener       port.StreamOpener
}

type OpenStreamInput struct {
	RPCName string
	Headers http.Header
}

type OpenStreamOutput struct {
	RPC     RPCSummary
	Session *StreamSession
}

func NewOpenStream(schemaLoader port.SchemaLoader, opener port.StreamOpener) *OpenStream {
	return &OpenStream{schemaLoader: schemaLoader, opener: opener}
}

func (u *OpenStream) Execute(ctx context.Context, in OpenStreamInput) (OpenStreamOutput, error) {
	s, err := u.schemaLoader.Load(ctx)
	if err != nil {
		return OpenStreamOutput{}, err
	}
	r, err := s.ResolveRPC(in.RPCName)
	if err != nil {
		return OpenStreamOutput{}, err
	}
	if !r.ClientStreaming() {
		return OpenStreamOutput{}, fmt.Errorf("rpc %s is %s, not client or bidi streaming", r.FullName(), streamKind(r))
	}
	stream, err := u.opener.OpenStream(ctx, invocation.Request{RPC: r, Headers: in.Headers})
	if err != nil {
		return OpenStreamOutput{}, err
	}
	return OpenStreamOutput{
		RPC: toRPCSummary(r),
		Session: &StreamSession{
			stream: stream,
			input:  r.Input().Descriptor().FullName(),
			bidi:   r.ServerStreaming(),
		},
	}, nil
}

type StreamSession struct {
	stream invocation.Stream
	input  protoreflect.FullName
	bidi   bool

	mu       sync.Mutex
	sent     int
	received int
	closed   bool
}

func (s *StreamSession) Bidi() bool {
	return s.bidi
}

func (s *StreamSession) Send(msg proto.Message) error {
	if got := msg.ProtoReflect().Descriptor().FullName(); got != s.input {
		return fmt.Errorf("request message type mismatch: got %s, want %s", got, s.input)
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return errors.New("stream is already closed for sending")
	}
	if err := s.stream.Send(msg); err != nil {
		return err
	}
	s.mu.Lock()
	s.sent++
	s.mu.Unlock()
	return nil
}

func (s *StreamSession) CloseSend() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.stream.CloseSend()
}

func (s *StreamSession) Receive() (proto.Message, error) {
	msg, err := s.stream.Receive()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.received++
	s.mu.Unlock()
	return msg, nil
}

func (s *StreamSession) ReceiveAll(onMessage func(proto.Message) error) error {
	for {
		msg, err := s.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := onMessage(msg); err != nil {
			return err
		}
	}
}

func (s *StreamSession) Counts() (sent, received int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent, s.received
}

func (s *StreamSession) Headers() http.Header {
	return s.stream.ResponseHeaders()
}

func (s *StreamSession) Trailers() http.Header {
	return s.stream.ResponseTrailers()
}

func (s *StreamSession) Close() error {
	return s.stream.Close()
}
