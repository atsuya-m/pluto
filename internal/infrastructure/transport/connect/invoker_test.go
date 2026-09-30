package connect_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/infrastructure/transport/connect"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
	"github.com/atsuya-m/pluto/internal/testutil/testserver"
)

func TestInvoker_Protocols(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "CreateUser")

	for _, tt := range []struct {
		protocol connect.Protocol
		json     bool
	}{
		{connect.ProtocolConnect, false},
		{connect.ProtocolConnect, true},
		{connect.ProtocolGRPC, false},
		{connect.ProtocolGRPCWeb, false},
	} {
		name := string(tt.protocol)
		if tt.json {
			name += "+json"
		}
		t.Run(name, func(t *testing.T) {
			inv, err := connect.NewInvoker(srv.URL, connect.WithProtocol(tt.protocol), connect.WithJSON(tt.json))
			if err != nil {
				t.Fatal(err)
			}
			req := dynamicpb.NewMessage(rpc.Input().Descriptor())
			if err := protojson.Unmarshal([]byte(`{"name":"Taro"}`), req); err != nil {
				t.Fatal(err)
			}

			res, err := inv.Invoke(context.Background(), invocation.Request{
				RPC:     rpc,
				Message: req,
				Headers: http.Header{"Authorization": {"Bearer token"}},
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if got := (<-srv.Headers).Get("Authorization"); got != "Bearer token" {
				t.Errorf("server saw Authorization = %q", got)
			}
			<-srv.Requests

			user := res.Message.ProtoReflect().Get(rpc.Output().Descriptor().Fields().ByName("user")).Message()
			if got := user.Get(user.Descriptor().Fields().ByName("name")).String(); got != "Taro" {
				t.Errorf("response name = %q", got)
			}
			if res.Headers.Get("X-Test") != "1" {
				t.Errorf("response header X-Test = %q", res.Headers.Get("X-Test"))
			}
			if res.Trailers.Get("X-Trailer") != "2" {
				t.Errorf("response trailer X-Trailer = %q", res.Trailers.Get("X-Trailer"))
			}
		})
	}
}

func TestInvoker_ErrorIsConvertedToInvocationError(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "CreateUser")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = inv.Invoke(context.Background(), invocation.Request{RPC: rpc, Message: dynamicpb.NewMessage(rpc.Input().Descriptor())})

	var invErr *invocation.Error
	if !errors.As(err, &invErr) {
		t.Fatalf("error = %v, want *invocation.Error", err)
	}
	if invErr.Code != "invalid_argument" || invErr.Message != "name is required" {
		t.Errorf("error = %+v", invErr)
	}
}

func TestInvoker_UnimplementedRPC(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "UserService.GetUser")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = inv.Invoke(context.Background(), invocation.Request{RPC: rpc, Message: dynamicpb.NewMessage(rpc.Input().Descriptor())})

	var invErr *invocation.Error
	if !errors.As(err, &invErr) || invErr.Code != "unimplemented" {
		t.Errorf("error = %v, want unimplemented", err)
	}
}

func TestInvoker_ConnectionError(t *testing.T) {
	rpc := fixture.RPC(t, "CreateUser")
	inv, err := connect.NewInvoker("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = inv.Invoke(context.Background(), invocation.Request{RPC: rpc, Message: dynamicpb.NewMessage(rpc.Input().Descriptor())})

	var invErr *invocation.Error
	if !errors.As(err, &invErr) || invErr.Code != "unavailable" {
		t.Errorf("error = %v, want unavailable", err)
	}
}

func TestInvoker_ServerStream(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "WatchUsers")
	req := dynamicpb.NewMessage(rpc.Input().Descriptor())

	for _, p := range []connect.Protocol{connect.ProtocolConnect, connect.ProtocolGRPC, connect.ProtocolGRPCWeb} {
		t.Run(string(p), func(t *testing.T) {
			inv, err := connect.NewInvoker(srv.URL, connect.WithProtocol(p))
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			res, err := inv.InvokeServerStream(context.Background(), invocation.Request{RPC: rpc, Message: req}, func(m proto.Message) error {
				r := m.ProtoReflect()
				ids = append(ids, r.Get(r.Descriptor().Fields().ByName("id")).String())
				return nil
			})
			<-srv.Headers
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(ids, ",") != "u-1,u-2,u-3" {
				t.Errorf("ids = %v", ids)
			}
			if res.Headers.Get("X-Test") != "stream" || res.Trailers.Get("X-Trailer") != "done" {
				t.Errorf("headers = %v, trailers = %v", res.Headers, res.Trailers)
			}
		})
	}
}

func TestInvoker_ServerStreamErrorAfterMessages(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "WatchUsers")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	_, err = inv.InvokeServerStream(context.Background(), invocation.Request{
		RPC:     rpc,
		Message: dynamicpb.NewMessage(rpc.Input().Descriptor()),
		Headers: http.Header{"X-Fail-After": {"2"}},
	}, func(proto.Message) error { count++; return nil })
	<-srv.Headers

	var invErr *invocation.Error
	if !errors.As(err, &invErr) || invErr.Code != "unavailable" || invErr.Message != "stream broken" {
		t.Errorf("error = %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestInvoker_ServerStreamCallbackErrorStops(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "WatchUsers")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop")
	_, err = inv.InvokeServerStream(context.Background(), invocation.Request{
		RPC:     rpc,
		Message: dynamicpb.NewMessage(rpc.Input().Descriptor()),
		Headers: http.Header{"X-Hang": {"1"}},
	}, func(proto.Message) error { return stop })
	<-srv.Headers
	if !errors.Is(err, stop) {
		t.Errorf("error = %v, want stop", err)
	}
}

func chatMessage(t *testing.T, text string) *dynamicpb.Message {
	m := dynamicpb.NewMessage(fixture.Message(t, "user.v1.ChatMessage"))
	m.Set(m.Descriptor().Fields().ByName("text"), protoreflect.ValueOfString(text))
	return m
}

func text(m proto.Message) string {
	r := m.ProtoReflect()
	return r.Get(r.Descriptor().Fields().ByName("text")).String()
}

func TestInvoker_BidiStream(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "Chat")
	for _, p := range []connect.Protocol{connect.ProtocolConnect, connect.ProtocolGRPC} {
		t.Run(string(p), func(t *testing.T) {
			inv, err := connect.NewInvoker(srv.URL, connect.WithProtocol(p))
			if err != nil {
				t.Fatal(err)
			}
			stream, err := inv.OpenStream(context.Background(), invocation.Request{RPC: rpc, Headers: http.Header{"X-Test": {"bidi"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stream.Close() }()

			for _, s := range []string{"a", "b"} {
				if err := stream.Send(chatMessage(t, s)); err != nil {
					t.Fatal(err)
				}
				got, err := stream.Receive()
				if err != nil {
					t.Fatal(err)
				}
				if text(got) != "echo: "+s {
					t.Errorf("reply = %q", text(got))
				}
			}
			if h := <-srv.Headers; h.Get("X-Test") != "bidi" {
				t.Errorf("server saw X-Test = %q", h.Get("X-Test"))
			}
			if err := stream.CloseSend(); err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Receive(); !errors.Is(err, io.EOF) {
				t.Errorf("after CloseSend Receive err = %v, want EOF", err)
			}
		})
	}
}

func TestInvoker_ClientStream(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "ImportUsers")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := inv.OpenStream(context.Background(), invocation.Request{RPC: rpc})
	if err != nil {
		t.Fatal(err)
	}

	received := make(chan proto.Message, 1)
	go func() {
		m, err := stream.Receive()
		if err != nil {
			t.Errorf("Receive: %v", err)
		}
		received <- m
	}()
	for _, name := range []string{"A", "B", "C"} {
		req := dynamicpb.NewMessage(rpc.Input().Descriptor())
		req.Set(req.Descriptor().Fields().ByName("name"), protoreflect.ValueOfString(name))
		if err := stream.Send(req); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	res := <-received
	<-srv.Headers
	if got := res.ProtoReflect().Get(rpc.Output().Descriptor().Fields().ByName("imported")).Int(); got != 3 {
		t.Errorf("imported = %d", got)
	}
	if _, err := stream.Receive(); !errors.Is(err, io.EOF) {
		t.Errorf("second Receive err = %v, want EOF", err)
	}
}

func TestInvoker_ClientStreamError(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "ImportUsers")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := inv.OpenStream(context.Background(), invocation.Request{RPC: rpc})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Send(dynamicpb.NewMessage(rpc.Input().Descriptor()))
	_ = stream.CloseSend()
	_, err = stream.Receive()
	<-srv.Headers

	var invErr *invocation.Error
	if !errors.As(err, &invErr) || invErr.Code != "invalid_argument" {
		t.Errorf("err = %v", err)
	}
}

func TestInvoker_ClientStreamCancel(t *testing.T) {
	srv := testserver.New(t)
	rpc := fixture.RPC(t, "ImportUsers")
	inv, err := connect.NewInvoker(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := inv.OpenStream(ctx, invocation.Request{RPC: rpc})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, err = stream.Receive()

	var invErr *invocation.Error
	if !errors.As(err, &invErr) || invErr.Code != "canceled" {
		t.Errorf("err = %v, want canceled", err)
	}
}

func TestParseProtocol(t *testing.T) {
	for in, want := range map[string]connect.Protocol{
		"connect":  connect.ProtocolConnect,
		"GRPC":     connect.ProtocolGRPC,
		"grpcweb":  connect.ProtocolGRPCWeb,
		"grpc-web": connect.ProtocolGRPCWeb,
	} {
		if got, err := connect.ParseProtocol(in); err != nil || got != want {
			t.Errorf("ParseProtocol(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := connect.ParseProtocol("http3"); err == nil {
		t.Error("expected error for unknown protocol")
	}
}
