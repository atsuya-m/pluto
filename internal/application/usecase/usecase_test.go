package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/domain/schema"
	jsondecoder "github.com/atsuya-m/pluto/internal/infrastructure/codec/protojson"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

type fakeInvoker struct {
	got      invocation.Request
	response invocation.Response
	err      error
	calls    int
}

func (f *fakeInvoker) Invoke(_ context.Context, req invocation.Request) (invocation.Response, error) {
	f.calls++
	f.got = req
	return f.response, f.err
}

func TestListRPCs(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewListRPCs(fixture.NewSchemaLoader(t))

	out, err := uc.Execute(ctx, usecase.ListRPCsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.RPCs) != 10 {
		t.Errorf("len = %d, want 10", len(out.RPCs))
	}
	first := out.RPCs[0]
	if first.FullName != "admin.v1.AdminService.BanUser" || first.RequestType != "admin.v1.BanUserRequest" || first.Service != "admin.v1.AdminService" {
		t.Errorf("first = %+v", first)
	}

	filtered, err := uc.Execute(ctx, usecase.ListRPCsInput{Service: "AdminService"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range filtered.RPCs {
		names = append(names, r.Name)
	}
	if want := []string{"GetUser", "BanUser"}; !slices.Equal(names, want) {
		t.Errorf("filtered = %v, want %v", names, want)
	}

	if _, err := uc.Execute(ctx, usecase.ListRPCsInput{Service: "Nope"}); !errors.Is(err, schema.ErrSymbolNotFound) {
		t.Errorf("unknown service error = %v", err)
	}
}

func TestListServices(t *testing.T) {
	out, err := usecase.NewListServices(fixture.NewSchemaLoader(t)).Execute(context.Background(), usecase.ListServicesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Services) != 2 || out.Services[1].FullName != "user.v1.UserService" || len(out.Services[1].RPCs) != 8 {
		t.Errorf("services = %+v", out.Services)
	}
}

func TestSchemaLoadErrorPropagates(t *testing.T) {
	loader := &fixture.SchemaLoader{Err: errors.New("boom")}
	if _, err := usecase.NewListRPCs(loader).Execute(context.Background(), usecase.ListRPCsInput{}); err == nil || err.Error() != "boom" {
		t.Errorf("error = %v, want boom", err)
	}
}

func TestDescribeRPC(t *testing.T) {
	out, err := usecase.NewDescribeRPC(fixture.NewSchemaLoader(t)).Execute(context.Background(), usecase.DescribeRPCInput{Name: "CreateUser"})
	if err != nil {
		t.Fatal(err)
	}
	r := out.RPC
	if r.FullName != "user.v1.UserService.CreateUser" || r.Request.FullName != "user.v1.CreateUserRequest" || r.Response.FullName != "user.v1.CreateUserResponse" {
		t.Errorf("rpc = %+v", r.RPCSummary)
	}

	var status usecase.FieldDetail
	for _, f := range r.Request.Fields {
		if f.Name == "status" {
			status = f
		}
	}
	if want := []string{"USER_STATUS_UNSPECIFIED", "USER_STATUS_ACTIVE", "USER_STATUS_DISABLED"}; !slices.Equal(status.EnumValues, want) {
		t.Errorf("status enum values = %v", status.EnumValues)
	}
}

func TestDescribeMessage(t *testing.T) {
	out, err := usecase.NewDescribeMessage(fixture.NewSchemaLoader(t)).Execute(context.Background(), usecase.DescribeMessageInput{Name: "Address"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message.FullName != "user.v1.Address" || len(out.Message.Fields) != 2 {
		t.Errorf("message = %+v", out.Message)
	}
}

func TestPrepareRequest(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewPrepareRequest(fixture.NewSchemaLoader(t), jsondecoder.NewRequestDecoder())

	t.Run("empty", func(t *testing.T) {
		out, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "CreateUser"})
		if err != nil {
			t.Fatal(err)
		}
		if out.RPC.FullName != "user.v1.UserService.CreateUser" || out.Input.FullName() != "user.v1.CreateUserRequest" {
			t.Errorf("out = %+v", out.RPC)
		}
		if got := marshal(t, out.Builder.Message()); got != "{}" {
			t.Errorf("message = %s, want {}", got)
		}
	})

	t.Run("with data", func(t *testing.T) {
		out, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "CreateUser", Data: []byte(`{"name":"Taro","nickname":""}`)})
		if err != nil {
			t.Fatal(err)
		}
		if got := marshal(t, out.Builder.Message()); got != `{"name":"Taro","nickname":""}` {
			t.Errorf("message = %s", got)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "CreateUser", Data: []byte(`{"nope":1}`)}); err == nil {
			t.Error("expected decode error")
		}
	})

	t.Run("server streaming is allowed", func(t *testing.T) {
		out, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "WatchUsers"})
		if err != nil || !out.RPC.ServerStreaming {
			t.Errorf("out = %+v, err = %v", out.RPC, err)
		}
	})

	t.Run("client streaming splits newline separated JSON", func(t *testing.T) {
		out, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "ImportUsers", Data: []byte("{\"name\":\"A\"}\n{\"name\":\"B\"}\n")})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Messages) != 2 || marshal(t, out.Messages[1]) != `{"name":"B"}` || marshal(t, out.Builder.Message()) != `{"name":"A"}` {
			t.Errorf("messages = %d, builder = %s", len(out.Messages), marshal(t, out.Builder.Message()))
		}
	})

	t.Run("client streaming accepts a JSON array", func(t *testing.T) {
		out, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "Chat", Data: []byte(`[{"text":"a"},{"text":"b"},{"text":"c"}]`)})
		if err != nil || len(out.Messages) != 3 {
			t.Errorf("messages = %d, err = %v", len(out.Messages), err)
		}
	})

	t.Run("broken JSON stream", func(t *testing.T) {
		if _, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: "Chat", Data: []byte(`{"text":"a"} {`)}); err == nil {
			t.Error("expected error")
		}
	})
}

func TestInvokeRPC(t *testing.T) {
	ctx := context.Background()
	createReq := dynamicpb.NewMessage(fixture.Message(t, "user.v1.CreateUserRequest"))
	createRes := dynamicpb.NewMessage(fixture.Message(t, "user.v1.CreateUserResponse"))

	t.Run("success", func(t *testing.T) {
		inv := &fakeInvoker{response: invocation.Response{
			Message:  createRes,
			Headers:  http.Header{"X-Res": {"1"}},
			Trailers: http.Header{"X-Trailer": {"2"}},
		}}
		out, err := usecase.NewInvokeRPC(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeRPCInput{
			RPCName: "UserService.CreateUser",
			Message: createReq,
			Headers: http.Header{"Authorization": {"Bearer x"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if inv.got.RPC.FullName() != "user.v1.UserService.CreateUser" || inv.got.Headers.Get("Authorization") != "Bearer x" {
			t.Errorf("invoker got rpc=%s headers=%v", inv.got.RPC.FullName(), inv.got.Headers)
		}
		if out.RPC.FullName != "user.v1.UserService.CreateUser" || out.Headers.Get("X-Res") != "1" || out.Trailers.Get("X-Trailer") != "2" {
			t.Errorf("out = %+v", out)
		}
	})

	t.Run("invoker error passes through", func(t *testing.T) {
		want := &invocation.Error{Code: "not_found", Message: "nope"}
		inv := &fakeInvoker{err: want}
		_, err := usecase.NewInvokeRPC(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeRPCInput{RPCName: "CreateUser", Message: createReq})
		var got *invocation.Error
		if !errors.As(err, &got) || got != want {
			t.Errorf("error = %v, want %v", err, want)
		}
	})

	rejects := []struct {
		name  string
		input usecase.InvokeRPCInput
	}{
		{"streaming", usecase.InvokeRPCInput{RPCName: "WatchUsers", Message: dynamicpb.NewMessage(fixture.Message(t, "user.v1.WatchUsersRequest"))}},
		{"nil message", usecase.InvokeRPCInput{RPCName: "CreateUser"}},
		{"type mismatch", usecase.InvokeRPCInput{RPCName: "CreateUser", Message: createRes}},
		{"ambiguous", usecase.InvokeRPCInput{RPCName: "GetUser", Message: createReq}},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) {
			inv := &fakeInvoker{}
			if _, err := usecase.NewInvokeRPC(fixture.NewSchemaLoader(t), inv).Execute(ctx, tt.input); err == nil {
				t.Error("expected error")
			}
			if inv.calls != 0 {
				t.Error("invoker must not be called")
			}
		})
	}
}

func marshal(t *testing.T, m proto.Message) string {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		return "{}"
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

type fakeStreamInvoker struct {
	messages []proto.Message
	result   invocation.StreamResult
	err      error
	calls    int
}

func (f *fakeStreamInvoker) InvokeServerStream(_ context.Context, _ invocation.Request, onMessage func(proto.Message) error) (invocation.StreamResult, error) {
	f.calls++
	for _, m := range f.messages {
		if err := onMessage(m); err != nil {
			return f.result, err
		}
	}
	return f.result, f.err
}

func TestInvokeServerStream(t *testing.T) {
	ctx := context.Background()
	watchReq := dynamicpb.NewMessage(fixture.Message(t, "user.v1.WatchUsersRequest"))
	user := dynamicpb.NewMessage(fixture.Message(t, "user.v1.User"))

	t.Run("success", func(t *testing.T) {
		inv := &fakeStreamInvoker{
			messages: []proto.Message{user, user, user},
			result:   invocation.StreamResult{Headers: http.Header{"X-H": {"1"}}, Trailers: http.Header{"X-T": {"2"}}},
		}
		var received int
		out, err := usecase.NewInvokeServerStream(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeServerStreamInput{
			RPCName:   "WatchUsers",
			Message:   watchReq,
			OnMessage: func(proto.Message) error { received++; return nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != 3 || received != 3 || out.Headers.Get("X-H") != "1" || out.Trailers.Get("X-T") != "2" {
			t.Errorf("out = %+v, received = %d", out, received)
		}
	})

	t.Run("error after messages keeps the count", func(t *testing.T) {
		inv := &fakeStreamInvoker{messages: []proto.Message{user}, err: &invocation.Error{Code: "unavailable"}}
		out, err := usecase.NewInvokeServerStream(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeServerStreamInput{RPCName: "WatchUsers", Message: watchReq})
		if err == nil || out.Count != 1 {
			t.Errorf("out = %+v, err = %v", out, err)
		}
	})

	t.Run("callback error stops the stream", func(t *testing.T) {
		inv := &fakeStreamInvoker{messages: []proto.Message{user, user, user}}
		stop := errors.New("stop")
		out, err := usecase.NewInvokeServerStream(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeServerStreamInput{
			RPCName:   "WatchUsers",
			Message:   watchReq,
			OnMessage: func(proto.Message) error { return stop },
		})
		if !errors.Is(err, stop) || out.Count != 1 {
			t.Errorf("out = %+v, err = %v", out, err)
		}
	})

	for _, name := range []string{"CreateUser", "ImportUsers", "Chat"} {
		t.Run("rejects "+name, func(t *testing.T) {
			inv := &fakeStreamInvoker{}
			_, err := usecase.NewInvokeServerStream(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeServerStreamInput{RPCName: name, Message: watchReq})
			if err == nil || inv.calls != 0 {
				t.Errorf("err = %v, calls = %d", err, inv.calls)
			}
		})
	}

	t.Run("rejects mismatched message", func(t *testing.T) {
		inv := &fakeStreamInvoker{}
		_, err := usecase.NewInvokeServerStream(fixture.NewSchemaLoader(t), inv).Execute(ctx, usecase.InvokeServerStreamInput{RPCName: "WatchUsers", Message: user})
		if err == nil || inv.calls != 0 {
			t.Errorf("err = %v, calls = %d", err, inv.calls)
		}
	})
}

type fakeStream struct {
	sent      []proto.Message
	responses []proto.Message
	closed    bool
	sendErr   error
}

func (f *fakeStream) Send(msg proto.Message) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeStream) CloseSend() error {
	f.closed = true
	return nil
}

func (f *fakeStream) Receive() (proto.Message, error) {
	if len(f.responses) == 0 {
		return nil, io.EOF
	}
	m := f.responses[0]
	f.responses = f.responses[1:]
	return m, nil
}

func (f *fakeStream) ResponseHeaders() http.Header  { return http.Header{"X-H": {"1"}} }
func (f *fakeStream) ResponseTrailers() http.Header { return http.Header{"X-T": {"2"}} }
func (f *fakeStream) Close() error                  { return nil }

type fakeOpener struct {
	stream *fakeStream
	got    invocation.Request
	calls  int
}

func (f *fakeOpener) OpenStream(_ context.Context, req invocation.Request) (invocation.Stream, error) {
	f.calls++
	f.got = req
	return f.stream, nil
}

func TestOpenStream(t *testing.T) {
	ctx := context.Background()
	chatMsg := func(text string) proto.Message {
		m := dynamicpb.NewMessage(fixture.Message(t, "user.v1.ChatMessage"))
		m.Set(m.Descriptor().Fields().ByName("text"), protoreflect.ValueOfString(text))
		return m
	}

	opener := &fakeOpener{stream: &fakeStream{responses: []proto.Message{chatMsg("r1"), chatMsg("r2")}}}
	out, err := usecase.NewOpenStream(fixture.NewSchemaLoader(t), opener).Execute(ctx, usecase.OpenStreamInput{RPCName: "Chat", Headers: http.Header{"A": {"b"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Session.Bidi() || opener.got.RPC.FullName() != "user.v1.UserService.Chat" || opener.got.Headers.Get("A") != "b" {
		t.Errorf("bidi = %v, rpc = %s", out.Session.Bidi(), opener.got.RPC.FullName())
	}

	s := out.Session
	if err := s.Send(chatMsg("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(dynamicpb.NewMessage(fixture.Message(t, "user.v1.User"))); err == nil {
		t.Error("sending the wrong message type should fail")
	}
	if err := s.CloseSend(); err != nil || !opener.stream.closed {
		t.Errorf("CloseSend err = %v, closed = %v", err, opener.stream.closed)
	}
	if err := s.CloseSend(); err != nil {
		t.Errorf("CloseSend should be idempotent: %v", err)
	}
	if err := s.Send(chatMsg("late")); err == nil {
		t.Error("sending after CloseSend should fail")
	}

	var got []string
	if err := s.ReceiveAll(func(m proto.Message) error {
		r := m.ProtoReflect()
		got = append(got, r.Get(r.Descriptor().Fields().ByName("text")).String())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sent, received := s.Counts()
	if strings.Join(got, ",") != "r1,r2" || sent != 1 || received != 2 {
		t.Errorf("got = %v, sent = %d, received = %d", got, sent, received)
	}
	if s.Headers().Get("X-H") != "1" || s.Trailers().Get("X-T") != "2" {
		t.Error("headers/trailers should come from the stream")
	}

	for _, name := range []string{"CreateUser", "WatchUsers"} {
		o := &fakeOpener{stream: &fakeStream{}}
		if _, err := usecase.NewOpenStream(fixture.NewSchemaLoader(t), o).Execute(ctx, usecase.OpenStreamInput{RPCName: name}); err == nil || o.calls != 0 {
			t.Errorf("%s: err = %v, calls = %d", name, err, o.calls)
		}
	}

	client, err := usecase.NewOpenStream(fixture.NewSchemaLoader(t), &fakeOpener{stream: &fakeStream{}}).Execute(ctx, usecase.OpenStreamInput{RPCName: "ImportUsers"})
	if err != nil || client.Session.Bidi() {
		t.Errorf("ImportUsers: err = %v", err)
	}
}
