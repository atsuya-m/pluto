package testserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/atsuya-m/pluto/internal/domain/schema"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

type Server struct {
	URL      string
	Requests chan *dynamicpb.Message
	Headers  chan http.Header
}

func New(t testing.TB) *Server {
	t.Helper()
	s := fixture.Schema(t)
	srv := &Server{Requests: make(chan *dynamicpb.Message, 16), Headers: make(chan http.Header, 64)}

	mux := http.NewServeMux()
	create, err := s.ResolveRPC("user.v1.UserService.CreateUser")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle(create.Procedure(), unary(create, func(req *connect.Request[dynamicpb.Message]) (*connect.Response[dynamicpb.Message], error) {
		srv.Requests <- req.Msg
		srv.Headers <- req.Header().Clone()
		name := req.Msg.Get(field(req.Msg, "name")).String()
		if name == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
		}
		if name == "not-found" {
			cerr := connect.NewError(connect.CodeNotFound, errors.New("account not found"))
			cerr.Meta().Set("X-Error-Reason", "1001")
			return nil, cerr
		}
		res := dynamicpb.NewMessage(create.Output().Descriptor())
		user := res.Mutable(field(res, "user")).Message()
		user.Set(field(user, "id"), protoreflect.ValueOfString("u-1"))
		user.Set(field(user, "name"), protoreflect.ValueOfString(name))
		out := connect.NewResponse(res)
		out.Header().Set("X-Test", "1")
		out.Trailer().Set("X-Trailer", "2")
		return out, nil
	}))

	watch, err := s.ResolveRPC("user.v1.UserService.WatchUsers")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle(watch.Procedure(), serverStream(watch, func(ctx context.Context, req *connect.Request[dynamicpb.Message], stream *connect.ServerStream[dynamicpb.Message]) error {
		srv.Headers <- req.Header().Clone()
		stream.ResponseHeader().Set("X-Test", "stream")
		for i := 1; i <= 3; i++ {
			if req.Header().Get("X-Fail-After") == fmt.Sprint(i) {
				return connect.NewError(connect.CodeUnavailable, errors.New("stream broken"))
			}
			user := dynamicpb.NewMessage(watch.Output().Descriptor())
			user.Set(field(user, "id"), protoreflect.ValueOfString(fmt.Sprintf("u-%d", i)))
			if err := stream.Send(user); err != nil {
				return err
			}
		}
		if req.Header().Get("X-Hang") != "" {
			<-ctx.Done()
			return ctx.Err()
		}
		stream.ResponseTrailer().Set("X-Trailer", "done")
		return nil
	}))

	importUsers, err := s.ResolveRPC("user.v1.UserService.ImportUsers")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle(importUsers.Procedure(), connect.NewClientStreamHandler(importUsers.Procedure(), func(_ context.Context, stream *connect.ClientStream[dynamicpb.Message]) (*connect.Response[dynamicpb.Message], error) {
		srv.Headers <- stream.RequestHeader().Clone()
		var n int32
		for stream.Receive() {
			if stream.Msg().Get(field(stream.Msg(), "name")).String() == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
			}
			n++
		}
		if err := stream.Err(); err != nil {
			return nil, err
		}
		res := dynamicpb.NewMessage(importUsers.Output().Descriptor())
		res.Set(field(res, "imported"), protoreflect.ValueOfInt32(n))
		return connect.NewResponse(res), nil
	}, handlerOptions(importUsers)...))

	chat, err := s.ResolveRPC("user.v1.UserService.Chat")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle(chat.Procedure(), connect.NewBidiStreamHandler(chat.Procedure(), func(_ context.Context, stream *connect.BidiStream[dynamicpb.Message, dynamicpb.Message]) error {
		srv.Headers <- stream.RequestHeader().Clone()
		for {
			msg, err := stream.Receive()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			reply := dynamicpb.NewMessage(chat.Output().Descriptor())
			reply.Set(field(reply, "text"), protoreflect.ValueOfString("echo: "+msg.Get(field(msg, "text")).String()))
			if err := stream.Send(reply); err != nil {
				return err
			}
		}
	}, handlerOptions(chat)...))

	reflector := grpcreflect.NewReflector(
		grpcreflect.NamerFunc(func() []string {
			var names []string
			for _, svc := range s.Services() {
				names = append(names, svc.FullName())
			}
			return names
		}),
		grpcreflect.WithDescriptorResolver(s.Files()),
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	hs := httptest.NewUnstartedServer(mux)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	hs.Config.Protocols = protocols
	hs.Start()
	t.Cleanup(hs.Close)

	srv.URL = hs.URL
	return srv
}

func unary(rpc schema.RPC, fn func(*connect.Request[dynamicpb.Message]) (*connect.Response[dynamicpb.Message], error)) http.Handler {
	md := rpc.Descriptor()
	return connect.NewUnaryHandler(
		rpc.Procedure(),
		func(_ context.Context, req *connect.Request[dynamicpb.Message]) (*connect.Response[dynamicpb.Message], error) {
			return fn(req)
		},
		connect.WithSchema(md),
		connect.WithRequestInitializer(func(_ connect.Spec, msg any) error {
			dm, ok := msg.(*dynamicpb.Message)
			if !ok {
				return fmt.Errorf("unexpected message type %T", msg)
			}
			*dm = *dynamicpb.NewMessage(md.Input())
			return nil
		}),
	)
}

func handlerOptions(rpc schema.RPC) []connect.HandlerOption {
	md := rpc.Descriptor()
	return []connect.HandlerOption{
		connect.WithSchema(md),
		connect.WithRequestInitializer(func(_ connect.Spec, msg any) error {
			dm, ok := msg.(*dynamicpb.Message)
			if !ok {
				return fmt.Errorf("unexpected message type %T", msg)
			}
			*dm = *dynamicpb.NewMessage(md.Input())
			return nil
		}),
	}
}

func serverStream(rpc schema.RPC, fn func(context.Context, *connect.Request[dynamicpb.Message], *connect.ServerStream[dynamicpb.Message]) error) http.Handler {
	md := rpc.Descriptor()
	return connect.NewServerStreamHandler(
		rpc.Procedure(),
		fn,
		connect.WithSchema(md),
		connect.WithRequestInitializer(func(_ connect.Spec, msg any) error {
			dm, ok := msg.(*dynamicpb.Message)
			if !ok {
				return fmt.Errorf("unexpected message type %T", msg)
			}
			*dm = *dynamicpb.NewMessage(md.Input())
			return nil
		}),
	)
}

func field(m protoreflect.Message, name string) protoreflect.FieldDescriptor {
	return m.Descriptor().Fields().ByName(protoreflect.Name(name))
}
