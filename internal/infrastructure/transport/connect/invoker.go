package connect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/infrastructure/transport/httpclient"
)

type Protocol string

const (
	ProtocolConnect Protocol = "connect"
	ProtocolGRPC    Protocol = "grpc"
	ProtocolGRPCWeb Protocol = "grpcweb"
)

func ParseProtocol(s string) (Protocol, error) {
	switch p := Protocol(strings.ToLower(s)); p {
	case ProtocolConnect, ProtocolGRPC, ProtocolGRPCWeb:
		return p, nil
	case "grpc-web":
		return ProtocolGRPCWeb, nil
	default:
		return "", fmt.Errorf("unknown protocol %q (want connect, grpc or grpcweb)", s)
	}
}

type Invoker struct {
	client       *http.Client
	streamClient *http.Client
	baseURL      *url.URL
	protocol     Protocol
	json         bool
	timeout      time.Duration
}

type Option func(*Invoker)

func WithProtocol(p Protocol) Option {
	return func(i *Invoker) { i.protocol = p }
}

func WithJSON(enabled bool) Option {
	return func(i *Invoker) { i.json = enabled }
}

func WithTimeout(d time.Duration) Option {
	return func(i *Invoker) { i.timeout = d }
}

func WithHTTPClient(c *http.Client) Option {
	return func(i *Invoker) {
		i.client = c
		i.streamClient = c
	}
}

func NewInvoker(target string, opts ...Option) (*Invoker, error) {
	u, err := httpclient.ParseTarget(target)
	if err != nil {
		return nil, err
	}
	inv := &Invoker{baseURL: u, protocol: ProtocolConnect}
	for _, opt := range opts {
		opt(inv)
	}
	if inv.client == nil {
		if inv.protocol == ProtocolGRPC {
			inv.client = httpclient.NewHTTP2(u)
		} else {
			inv.client = httpclient.NewHTTP1()
		}
	}
	if inv.streamClient == nil {
		inv.streamClient = httpclient.NewHTTP2(u)
	}
	return inv, nil
}

func (i *Invoker) newClient(req invocation.Request) *connect.Client[dynamicpb.Message, dynamicpb.Message] {
	return i.newClientWith(i.client, req)
}

func (i *Invoker) newClientWith(httpClient *http.Client, req invocation.Request) *connect.Client[dynamicpb.Message, dynamicpb.Message] {
	md := req.RPC.Descriptor()
	endpoint := strings.TrimSuffix(i.baseURL.String(), "/") + req.RPC.Procedure()

	opts := []connect.ClientOption{
		connect.WithSchema(md),
		connect.WithResponseInitializer(initializer(md.Output())),
	}
	switch i.protocol {
	case ProtocolGRPC:
		opts = append(opts, connect.WithGRPC())
	case ProtocolGRPCWeb:
		opts = append(opts, connect.WithGRPCWeb())
	}
	if i.json {
		opts = append(opts, connect.WithProtoJSON())
	}
	return connect.NewClient[dynamicpb.Message, dynamicpb.Message](httpClient, endpoint, opts...)
}

func newRequest(req invocation.Request) (*connect.Request[dynamicpb.Message], error) {
	msg, err := toDynamic(req.RPC.Descriptor().Input(), req.Message)
	if err != nil {
		return nil, err
	}
	creq := connect.NewRequest(msg)
	for k, vs := range req.Headers {
		for _, v := range vs {
			creq.Header().Add(k, v)
		}
	}
	return creq, nil
}

func (i *Invoker) Invoke(ctx context.Context, req invocation.Request) (invocation.Response, error) {
	creq, err := newRequest(req)
	if err != nil {
		return invocation.Response{}, err
	}
	if i.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, i.timeout)
		defer cancel()
	}
	res, err := i.newClient(req).CallUnary(ctx, creq)
	if err != nil {
		return invocation.Response{}, toInvocationError(err)
	}
	return invocation.Response{
		Message:  res.Msg,
		Headers:  res.Header(),
		Trailers: res.Trailer(),
	}, nil
}

func (i *Invoker) InvokeServerStream(ctx context.Context, req invocation.Request, onMessage func(proto.Message) error) (invocation.StreamResult, error) {
	creq, err := newRequest(req)
	if err != nil {
		return invocation.StreamResult{}, err
	}
	stream, err := i.newClient(req).CallServerStream(ctx, creq)
	if err != nil {
		return invocation.StreamResult{}, toInvocationError(err)
	}
	defer func() { _ = stream.Close() }()

	for stream.Receive() {
		if err := onMessage(stream.Msg()); err != nil {
			return invocation.StreamResult{Headers: stream.ResponseHeader()}, err
		}
	}
	result := invocation.StreamResult{Headers: stream.ResponseHeader(), Trailers: stream.ResponseTrailer()}
	if err := stream.Err(); err != nil {
		return result, toInvocationError(err)
	}
	return result, nil
}

func (i *Invoker) OpenStream(ctx context.Context, req invocation.Request) (invocation.Stream, error) {
	client := i.newClientWith(i.streamClient, req)
	md := req.RPC.Descriptor()
	if md.IsStreamingServer() {
		s := client.CallBidiStream(ctx)
		addHeaders(s.RequestHeader(), req.Headers)
		return &bidiStream{stream: s, input: md.Input()}, nil
	}
	s := client.CallClientStream(ctx)
	addHeaders(s.RequestHeader(), req.Headers)
	return &clientStream{ctx: ctx, stream: s, input: md.Input(), done: make(chan struct{})}, nil
}

func addHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

type bidiStream struct {
	stream *connect.BidiStreamForClient[dynamicpb.Message, dynamicpb.Message]
	input  protoreflect.MessageDescriptor
}

func (s *bidiStream) Send(msg proto.Message) error {
	dm, err := toDynamic(s.input, msg)
	if err != nil {
		return err
	}
	if err := s.stream.Send(dm); err != nil {
		return toInvocationError(err)
	}
	return nil
}

func (s *bidiStream) CloseSend() error {
	return s.stream.CloseRequest()
}

func (s *bidiStream) Receive() (proto.Message, error) {
	msg, err := s.stream.Receive()
	if errors.Is(err, io.EOF) {
		return nil, io.EOF
	}
	if err != nil {
		return nil, toInvocationError(err)
	}
	return msg, nil
}

func (s *bidiStream) ResponseHeaders() http.Header  { return s.stream.ResponseHeader() }
func (s *bidiStream) ResponseTrailers() http.Header { return s.stream.ResponseTrailer() }

func (s *bidiStream) Close() error {
	_ = s.stream.CloseRequest()
	return s.stream.CloseResponse()
}

type clientStream struct {
	ctx    context.Context
	stream *connect.ClientStreamForClient[dynamicpb.Message, dynamicpb.Message]
	input  protoreflect.MessageDescriptor

	once      sync.Once
	done      chan struct{}
	response  *connect.Response[dynamicpb.Message]
	err       error
	delivered bool
}

func (s *clientStream) Send(msg proto.Message) error {
	dm, err := toDynamic(s.input, msg)
	if err != nil {
		return err
	}
	if err := s.stream.Send(dm); err != nil {
		return toInvocationError(err)
	}
	return nil
}

func (s *clientStream) CloseSend() error {
	s.once.Do(func() {
		s.response, s.err = s.stream.CloseAndReceive()
		close(s.done)
	})
	return nil
}

func (s *clientStream) Receive() (proto.Message, error) {
	select {
	case <-s.done:
	case <-s.ctx.Done():
		return nil, toInvocationError(connect.NewError(connect.CodeCanceled, s.ctx.Err()))
	}
	if s.err != nil {
		return nil, toInvocationError(s.err)
	}
	if s.delivered {
		return nil, io.EOF
	}
	s.delivered = true
	return s.response.Msg, nil
}

func (s *clientStream) ResponseHeaders() http.Header {
	select {
	case <-s.done:
		if s.response != nil {
			return s.response.Header()
		}
	default:
	}
	return http.Header{}
}

func (s *clientStream) ResponseTrailers() http.Header {
	select {
	case <-s.done:
		if s.response != nil {
			return s.response.Trailer()
		}
	default:
	}
	return http.Header{}
}

func (s *clientStream) Close() error {
	return s.CloseSend()
}

func initializer(desc protoreflect.MessageDescriptor) func(connect.Spec, any) error {
	return func(_ connect.Spec, msg any) error {
		dm, ok := msg.(*dynamicpb.Message)
		if !ok {
			return fmt.Errorf("unexpected message type %T", msg)
		}
		*dm = *dynamicpb.NewMessage(desc)
		return nil
	}
}

func toDynamic(desc protoreflect.MessageDescriptor, msg proto.Message) (*dynamicpb.Message, error) {
	if dm, ok := msg.(*dynamicpb.Message); ok && dm.Descriptor().FullName() == desc.FullName() {
		return dm, nil
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	dm := dynamicpb.NewMessage(desc)
	if err := proto.Unmarshal(b, dm); err != nil {
		return nil, err
	}
	return dm, nil
}

func toInvocationError(err error) error {
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return &invocation.Error{Code: "unknown", Message: err.Error(), Cause: err}
	}
	out := &invocation.Error{
		Code:    cerr.Code().String(),
		Message: cerr.Message(),
		Headers: cerr.Meta(),
		Cause:   err,
	}
	for _, d := range cerr.Details() {
		v, derr := d.Value()
		if derr != nil {
			out.Details = append(out.Details, d.Type())
			continue
		}
		out.Details = append(out.Details, fmt.Sprintf("%s: %v", d.Type(), v))
	}
	return out
}
