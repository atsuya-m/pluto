package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/atsuya-m/pluto/internal/application/invocation"
	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/domain/request"
	jsondecoder "github.com/atsuya-m/pluto/internal/infrastructure/codec/protojson"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
	"github.com/atsuya-m/pluto/internal/testutil/memstore"
)

type fakeInvokeRPC struct {
	mu        sync.Mutex
	calls     []usecase.InvokeRPCInput
	executeFn func(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error)
}

func (f *fakeInvokeRPC) Execute(ctx context.Context, in usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	f.mu.Unlock()
	return f.executeFn(ctx, in)
}

func (f *fakeInvokeRPC) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type harness struct {
	t       *testing.T
	model   tea.Model
	printed []string
	invoke  *fakeInvokeRPC
	late    chan tea.Msg
	store   *memstore.Store
	opener  *chanOpener
}

func newHarness(t *testing.T, invoke func(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error)) *harness {
	t.Helper()
	loader := fixture.NewSchemaLoader(t)
	fake := &fakeInvokeRPC{executeFn: invoke}
	store := memstore.New()
	opener := &chanOpener{t: t}
	m := NewModel(context.Background(), Dependencies{
		ListServices:    usecase.NewListServices(loader),
		ListRPCs:        usecase.NewListRPCs(loader),
		DescribeRPC:     usecase.NewDescribeRPC(loader),
		DescribeMessage: usecase.NewDescribeMessage(loader),
		PrepareRequest:  usecase.NewPrepareRequest(loader, jsondecoder.NewRequestDecoder(), usecase.WithRequestStore(store)),
		InvokeRPC:       fake,
		OpenStream:      usecase.NewOpenStream(loader, opener),
		SaveRequest:     usecase.NewSaveRequest(store),
		ListSaved:       usecase.NewListSavedRequests(store),
		DeleteSaved:     usecase.NewDeleteSavedRequest(store),
		Headers:         http.Header{"X-Initial": {"1"}},
		Target:          "http://test",
		Protocol:        "connect",
	})
	h := &harness{t: t, model: m, invoke: fake, late: make(chan tea.Msg, 64), store: store, opener: opener}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 40})
	h.run(loadRPCs(context.Background(), usecase.NewListRPCs(loader)))
	return h
}

func (h *harness) app() Model {
	return h.model.(Model)
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	var cmd tea.Cmd
	h.model, cmd = h.model.Update(msg)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	msg, ok := h.execute(cmd)
	if !ok {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			h.run(c)
		}
		return
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := 0; i < v.Len(); i++ {
			h.run(v.Index(i).Interface().(tea.Cmd))
		}
		return
	}
	h.handle(msg)
}

func (h *harness) handle(msg tea.Msg) {
	h.t.Helper()
	if isNoise(msg) {
		return
	}
	if strings.Contains(fmt.Sprintf("%T", msg), "printLineMessage") {
		h.printed = append(h.printed, fmt.Sprintf("%v", msg))
		return
	}
	if reflect.TypeOf(msg) == reflect.TypeOf(tea.Quit()) {
		return
	}
	h.send(msg)
}

func (h *harness) pump() {
	h.t.Helper()
	select {
	case msg := <-h.late:
		h.run(func() tea.Msg { return msg })
	case <-time.After(10 * time.Millisecond):
	}
}

func (h *harness) key(k tea.KeyType) {
	h.t.Helper()
	h.send(tea.KeyMsg{Type: k})
}

func (h *harness) typeText(s string) {
	h.t.Helper()
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

func (h *harness) submit(s string) {
	h.t.Helper()
	h.typeText(s)
	h.key(tea.KeyEnter)
}

func (h *harness) output() string {
	return strings.Join(h.printed, "\n")
}

func isNoise(msg tea.Msg) bool {
	t := fmt.Sprintf("%T", msg)
	return strings.HasPrefix(t, "spinner.") || strings.HasPrefix(t, "cursor.") || strings.Contains(t, "Blink")
}

func (h *harness) execute(cmd tea.Cmd) (tea.Msg, bool) {
	if cmd == nil {
		return nil, false
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg, msg != nil
	case <-time.After(200 * time.Millisecond):
		go func() {
			if m := <-done; m != nil && !isNoise(m) {
				h.late <- m
			}
		}()
		return nil, false
	}
}

func okResponse(t *testing.T) func(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
	return func(_ context.Context, in usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
		res := dynamicpb.NewMessage(fixture.Message(t, "user.v1.CreateUserResponse"))
		if err := protojson.Unmarshal([]byte(`{"user":{"id":"u-1","name":"Taro"}}`), res); err != nil {
			return usecase.InvokeRPCOutput{}, err
		}
		return usecase.InvokeRPCOutput{RPC: usecase.RPCSummary{FullName: in.RPCName}, Message: res}, nil
	}
}

func TestApp_CallFlow(t *testing.T) {
	h := newHarness(t, okResponse(t))
	if h.app().Mode() != ModeCommand || len(h.app().rpcs) != 10 {
		t.Fatalf("initial mode=%v rpcs=%d", h.app().Mode(), len(h.app().rpcs))
	}

	h.submit("call CreateUser")
	if h.app().Mode() != ModeRequestEditor {
		t.Fatalf("mode = %v, want editor; output:\n%s", h.app().Mode(), h.output())
	}

	h.key(tea.KeyEnter)
	h.typeText("Taro")
	h.key(tea.KeyEnter)

	h.key(tea.KeyCtrlS)
	if h.app().Mode() != ModePreview || !strings.Contains(h.app().View(), `"name": "Taro"`) {
		t.Fatalf("mode = %v, view:\n%s", h.app().Mode(), h.app().View())
	}

	h.key(tea.KeyEnter)
	if h.app().Mode() != ModeResponseViewer {
		t.Fatalf("mode = %v, want response viewer", h.app().Mode())
	}
	if h.invoke.callCount() != 1 || h.invoke.calls[0].RPCName != "user.v1.UserService.CreateUser" {
		t.Errorf("invoke calls = %+v", h.invoke.calls)
	}
	if !strings.Contains(h.output(), `"id": "u-1"`) {
		t.Errorf("response was not printed:\n%s", h.output())
	}
	if !strings.Contains(h.app().View(), "✔ OK") {
		t.Errorf("response view:\n%s", h.app().View())
	}

	h.typeText("c")
	if h.invoke.callCount() != 2 {
		t.Errorf("call again: calls = %d", h.invoke.callCount())
	}

	h.typeText("e")
	if h.app().Mode() != ModeRequestEditor {
		t.Errorf("e should return to the editor, mode = %v", h.app().Mode())
	}
	h.key(tea.KeyEsc)
	if h.app().Mode() != ModeCommand {
		t.Errorf("esc from editor root should return to command, mode = %v", h.app().Mode())
	}

	h.submit("edit")
	if h.app().Mode() != ModeRequestEditor {
		t.Errorf("edit should reopen the last request, mode = %v", h.app().Mode())
	}
}

func TestApp_ImplicitCall(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("CreateUser")
	if h.app().Mode() != ModeRequestEditor {
		t.Fatalf("mode = %v, want editor", h.app().Mode())
	}

	h = newHarness(t, okResponse(t))
	h.submit("Nope")
	if h.app().Mode() != ModeCommand || !strings.Contains(h.output(), `unknown command or rpc "Nope"`) {
		t.Errorf("mode = %v, output:\n%s", h.app().Mode(), h.output())
	}
}

func TestApp_AmbiguousRPCShowsCandidates(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("call GetUser")
	out := h.output()
	if h.app().Mode() != ModeCommand || !strings.Contains(out, "admin.v1.AdminService.GetUser") || !strings.Contains(out, "user.v1.UserService.GetUser") {
		t.Errorf("mode = %v, output:\n%s", h.app().Mode(), out)
	}
}

type fakeStream struct {
	executeFn func(context.Context, usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error)
}

func (f fakeStream) Execute(ctx context.Context, in usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error) {
	return f.executeFn(ctx, in)
}

func userMsg(t *testing.T, id string) *dynamicpb.Message {
	m := dynamicpb.NewMessage(fixture.Message(t, "user.v1.User"))
	m.Set(m.Descriptor().Fields().ByName("id"), protoreflect.ValueOfString(id))
	return m
}

func TestApp_ServerStreamFlow(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.model = withStream(h.app(), fakeStream{executeFn: func(_ context.Context, in usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error) {
		for i := 1; i <= 3; i++ {
			if err := in.OnMessage(userMsg(t, fmt.Sprintf("u-%d", i))); err != nil {
				return usecase.InvokeServerStreamOutput{}, err
			}
		}
		return usecase.InvokeServerStreamOutput{Count: 3}, nil
	}})

	h.submit("call WatchUsers")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	waitFor(t, h, func() bool { return h.app().Mode() == ModeResponseViewer })

	out := h.output()
	for _, want := range []string{"← #1", `"id": "u-1"`, "← #3", `"id": "u-3"`, "stream closed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, `"u-1"`) > strings.Index(out, `"u-3"`) {
		t.Errorf("messages printed out of order:\n%s", out)
	}
	if !strings.Contains(h.app().View(), "stream closed · 3 messages") {
		t.Errorf("view:\n%s", h.app().View())
	}
}

func TestApp_ServerStreamStopWithEsc(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.model = withStream(h.app(), fakeStream{executeFn: func(ctx context.Context, in usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error) {
		if err := in.OnMessage(userMsg(t, "u-1")); err != nil {
			return usecase.InvokeServerStreamOutput{}, err
		}
		<-ctx.Done()
		return usecase.InvokeServerStreamOutput{Count: 1}, ctx.Err()
	}})

	h.submit("call WatchUsers")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	waitFor(t, h, func() bool { return h.app().streamCount == 1 })
	if h.app().Mode() != ModeStreaming || !strings.Contains(h.app().View(), "1 messages") {
		t.Fatalf("mode = %v, view:\n%s", h.app().Mode(), h.app().View())
	}

	h.key(tea.KeyEsc)
	waitFor(t, h, func() bool { return h.app().Mode() == ModeResponseViewer })
	if !strings.Contains(h.app().View(), "stopped · 1 messages") {
		t.Errorf("view:\n%s", h.app().View())
	}
}

func TestApp_ServerStreamError(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.model = withStream(h.app(), fakeStream{executeFn: func(_ context.Context, in usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error) {
		_ = in.OnMessage(userMsg(t, "u-1"))
		return usecase.InvokeServerStreamOutput{Count: 1}, &invocation.Error{Code: "unavailable", Message: "stream broken"}
	}})

	h.submit("call WatchUsers")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	waitFor(t, h, func() bool { return h.app().Mode() == ModeResponseViewer })
	if !strings.Contains(h.app().View(), "✘ unavailable") || !strings.Contains(h.output(), "stream broken") {
		t.Errorf("view:\n%s\noutput:\n%s", h.app().View(), h.output())
	}
}

func withStream(m Model, uc InvokeServerStreamUseCase) Model {
	m.deps.InvokeStream = uc
	return m
}

func waitFor(t *testing.T, h *harness, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met; mode = %v\noutput:\n%s", h.app().Mode(), h.output())
		}
		h.pump()
	}
}

func TestApp_InvokeFailure(t *testing.T) {
	h := newHarness(t, func(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
		return usecase.InvokeRPCOutput{}, &invocation.Error{Code: "invalid_argument", Message: "name is required"}
	})
	h.submit("call CreateUser")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)

	if h.app().Mode() != ModeResponseViewer || !strings.Contains(h.app().View(), "✘ invalid_argument") {
		t.Errorf("mode = %v, view:\n%s", h.app().Mode(), h.app().View())
	}
	if !strings.Contains(h.output(), "name is required") {
		t.Errorf("error was not printed:\n%s", h.output())
	}
	h.typeText("q")
	if h.app().Mode() != ModeCommand {
		t.Errorf("q should return to command, mode = %v", h.app().Mode())
	}
}

func TestApp_EscCancelsInFlightInvocation(t *testing.T) {
	started := make(chan struct{})
	h := newHarness(t, func(ctx context.Context, _ usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
		close(started)
		<-ctx.Done()
		return usecase.InvokeRPCOutput{}, &invocation.Error{Code: "canceled", Message: ctx.Err().Error(), Cause: ctx.Err()}
	})
	h.submit("call CreateUser")
	h.key(tea.KeyCtrlS)

	var cmd tea.Cmd
	h.model, cmd = h.model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if h.app().Mode() != ModeInvoking {
		t.Fatalf("mode = %v, want invoking", h.app().Mode())
	}
	result := make(chan []tea.Msg, 1)
	go func() {
		batch := cmd().(tea.BatchMsg)
		var msgs []tea.Msg
		for _, c := range batch {
			if c == nil {
				continue
			}
			done := make(chan tea.Msg, 1)
			go func() { done <- c() }()
			select {
			case m := <-done:
				msgs = append(msgs, m)
			case <-time.After(2 * time.Second):
			}
		}
		result <- msgs
	}()
	<-started

	h.key(tea.KeyEsc)
	for _, msg := range <-result {
		if failed, ok := msg.(invokeFailedMsg); ok {
			if !errors.Is(failed.Err, context.Canceled) {
				t.Errorf("invoke error = %v, want context.Canceled", failed.Err)
			}
			h.send(msg)
			if h.app().Mode() != ModeResponseViewer {
				t.Errorf("mode = %v, want response viewer", h.app().Mode())
			}
			return
		}
	}
	t.Fatal("invocation was not cancelled")
}

func TestApp_Describe(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("desc CreateUser")
	h.submit("desc Profile")
	h.submit("desc message Address")
	h.submit("desc rpc Profile")
	out := h.output()
	for _, want := range []string{
		"rpc user.v1.UserService.CreateUser",
		"message user.v1.Profile {",
		"message user.v1.Address {",
		`rpc "Profile" not found`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestApp_SelectorFlow(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("rpcs")
	if h.app().Mode() != ModeRPCSelector {
		t.Fatalf("mode = %v, want selector", h.app().Mode())
	}
	h.key(tea.KeyCtrlN)
	h.key(tea.KeyEnter)
	if h.app().Mode() != ModeRequestEditor || h.app().editor.RPC().FullName != "admin.v1.AdminService.GetUser" {
		t.Errorf("mode = %v, rpc = %s", h.app().Mode(), h.app().editor.RPC().FullName)
	}

	h.key(tea.KeyEsc)
	h.submit("rpcs")
	h.key(tea.KeyCtrlG)
	if h.app().Mode() != ModeCommand {
		t.Errorf("ctrl+g should leave the selector, mode = %v", h.app().Mode())
	}
}

func TestApp_SchemaLoadFailure(t *testing.T) {
	h := newHarness(t, okResponse(t))
	loader := &fixture.SchemaLoader{Err: errors.New("no proto files")}
	h.run(loadRPCs(context.Background(), usecase.NewListRPCs(loader)))
	if !strings.Contains(h.output(), "no proto files") {
		t.Errorf("load error not printed:\n%s", h.output())
	}
	h.submit("rpcs")
	if h.app().Mode() != ModeCommand {
		t.Errorf("selector should not open without a schema, mode = %v", h.app().Mode())
	}
}

type chanStream struct {
	ctx      context.Context
	bidi     bool
	replies  chan proto.Message
	closed   chan struct{}
	once     bool
	reply    func(proto.Message) proto.Message
	final    func(n int) proto.Message
	count    int
	finalOut bool
}

func (s *chanStream) Send(msg proto.Message) error {
	s.count++
	if s.bidi {
		s.replies <- s.reply(msg)
	}
	return nil
}

func (s *chanStream) CloseSend() error {
	if !s.once {
		s.once = true
		close(s.closed)
	}
	return nil
}

func (s *chanStream) Receive() (proto.Message, error) {
	if s.bidi {
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case m := <-s.replies:
			return m, nil
		case <-s.closed:
			select {
			case m := <-s.replies:
				return m, nil
			default:
				return nil, io.EOF
			}
		}
	}
	select {
	case <-s.closed:
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
	if s.finalOut {
		return nil, io.EOF
	}
	s.finalOut = true
	return s.final(s.count), nil
}

func (s *chanStream) ResponseHeaders() http.Header  { return http.Header{} }
func (s *chanStream) ResponseTrailers() http.Header { return http.Header{} }
func (s *chanStream) Close() error                  { return s.CloseSend() }

type chanOpener struct {
	t       *testing.T
	mu      sync.Mutex
	headers []http.Header
}

func (o *chanOpener) OpenStream(ctx context.Context, req invocation.Request) (invocation.Stream, error) {
	o.mu.Lock()
	o.headers = append(o.headers, req.Headers.Clone())
	o.mu.Unlock()
	md := req.RPC.Descriptor()
	return &chanStream{
		ctx:     ctx,
		bidi:    md.IsStreamingServer(),
		replies: make(chan proto.Message, 16),
		closed:  make(chan struct{}),
		reply: func(msg proto.Message) proto.Message {
			in := msg.ProtoReflect()
			out := dynamicpb.NewMessage(md.Output())
			text := in.Get(in.Descriptor().Fields().ByName("text")).String()
			out.Set(out.Descriptor().Fields().ByName("text"), protoreflect.ValueOfString("echo: "+text))
			return out
		},
		final: func(n int) proto.Message {
			out := dynamicpb.NewMessage(md.Output())
			out.Set(out.Descriptor().Fields().ByName("imported"), protoreflect.ValueOfInt32(int32(n)))
			return out
		},
	}, nil
}

func (h *harness) setField(name, value string) {
	h.t.Helper()
	b := h.app().editor.Builder()
	fd, err := b.Field(request.Path(name))
	if err != nil {
		h.t.Fatal(err)
	}
	v, err := request.ParseScalar(fd, value)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := b.Set(request.Path(name), v); err != nil {
		h.t.Fatal(err)
	}
}

func TestApp_ClientStreamCompose(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("ImportUsers")
	if h.app().Mode() != ModeRequestEditor || !strings.Contains(h.app().View(), "C-s opens the stream") {
		t.Fatalf("mode = %v, view:\n%s", h.app().Mode(), h.app().View())
	}

	h.setField("name", "A")
	h.key(tea.KeyCtrlS)
	waitFor(t, h, func() bool { return h.app().sentCount == 1 })
	if h.app().Mode() != ModeRequestEditor || !strings.Contains(h.app().View(), "stream open · 1 sent") {
		t.Fatalf("mode = %v, view:\n%s", h.app().Mode(), h.app().View())
	}

	h.setField("name", "B")
	h.key(tea.KeyCtrlS)
	waitFor(t, h, func() bool { return h.app().sentCount == 2 })

	h.key(tea.KeyCtrlX)
	waitFor(t, h, func() bool { return h.app().Mode() == ModeResponseViewer })
	out := h.output()
	for _, want := range []string{"→ #1", `"name": "A"`, "→ #2", `"name": "B"`, "← #1", `"imported": 2`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(h.app().View(), "stream closed · 2 sent · 1 received") {
		t.Errorf("view:\n%s", h.app().View())
	}
	if h.opener.headers[0].Get("X-Initial") != "1" {
		t.Errorf("stream headers = %v", h.opener.headers[0])
	}

	h.typeText("c")
	waitFor(t, h, func() bool {
		return h.app().Mode() == ModeResponseViewer && len(h.opener.headers) == 2 && h.app().sentCount == 2
	})
	if !strings.Contains(h.app().View(), "2 sent · 1 received") {
		t.Errorf("call again should replay both messages:\n%s", h.app().View())
	}
}

func TestApp_BidiStreamCompose(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("Chat")
	h.setField("text", "hello")
	h.key(tea.KeyCtrlS)
	waitFor(t, h, func() bool { return h.app().streamCount == 1 })
	out := h.output()
	if !strings.Contains(out, `"text": "echo: hello"`) || h.app().Mode() != ModeRequestEditor {
		t.Fatalf("mode = %v, output:\n%s", h.app().Mode(), out)
	}
	if strings.Index(out, "→ #1") > strings.Index(out, "← #1") {
		t.Errorf("the sent message should be printed before the reply:\n%s", out)
	}

	h.key(tea.KeyEsc)
	waitFor(t, h, func() bool { return h.app().Mode() == ModeResponseViewer })
	if !strings.Contains(h.app().View(), "stopped · 1 sent · 1 received") {
		t.Errorf("view:\n%s", h.app().View())
	}
}

func TestApp_HeaderCommands(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("header set Authorization Bearer dummy-value-123")
	h.submit("header add X-Tag a")
	h.submit("header rm X-Initial")
	h.submit("headers")
	out := h.output()
	if strings.Contains(out, "dummy-value-123") {
		t.Errorf("header value leaked:\n%s", out)
	}
	if !strings.Contains(out, "X-Tag: a") || strings.Contains(out, "X-Initial: 1\n") {
		t.Errorf("output:\n%s", out)
	}

	h.submit("call CreateUser")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	got := h.invoke.calls[0].Headers
	if got.Get("Authorization") != "Bearer dummy-value-123" || got.Get("X-Tag") != "a" || got.Get("X-Initial") != "" {
		t.Errorf("invoke headers = %v", got)
	}

	h.typeText("q")
	h.submit("header bogus")
	if !strings.Contains(h.output(), "usage: header") {
		t.Errorf("usage error missing:\n%s", h.output())
	}
}

func TestApp_SaveLoadAndRestore(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("save x")
	if !strings.Contains(h.output(), "no request to save yet") {
		t.Errorf("output:\n%s", h.output())
	}

	h.submit("CreateUser")
	h.setField("name", "Taro")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	h.typeText("q")
	if _, ok, _ := h.store.LoadLast(context.Background(), "user.v1.UserService.CreateUser"); !ok {
		t.Fatal("sending should remember the last request")
	}

	h.submit("CreateUser")
	if !strings.Contains(h.output(), "restored the last request") || !h.app().editor.Builder().Has(request.Path("name")) {
		t.Fatalf("last request was not restored:\n%s", h.output())
	}
	h.key(tea.KeyEsc)

	h.submit("new CreateUser")
	if h.app().editor.Builder().Has(request.Path("name")) {
		t.Error("new should open an empty request")
	}
	h.setField("name", "Named")
	h.key(tea.KeyEsc)
	h.submit("save mine")
	h.submit("saved")
	if !strings.Contains(h.output(), "✔ saved") || !strings.Contains(h.output(), "mine  user.v1.UserService.CreateUser") {
		t.Errorf("output:\n%s", h.output())
	}

	h.submit("load mine")
	if h.app().Mode() != ModeRequestEditor || !strings.Contains(h.output(), `loaded "mine"`) {
		t.Fatalf("mode = %v, output:\n%s", h.app().Mode(), h.output())
	}
	v, _ := h.app().editor.Builder().Get(request.Path("name"))
	if v.String() != "Named" {
		t.Errorf("loaded name = %q", v.String())
	}
	h.key(tea.KeyEsc)

	h.submit("unsave mine")
	h.submit("load mine")
	if !strings.Contains(h.output(), "deleted saved request mine") || !strings.Contains(h.output(), `saved request "mine" not found`) {
		t.Errorf("output:\n%s", h.output())
	}
}

func bigResponse(t *testing.T, users int) func(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
	return func(_ context.Context, in usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
		res := dynamicpb.NewMessage(fixture.Message(t, "user.v1.ListUsersResponse"))
		var b strings.Builder
		b.WriteString(`{"users":[`)
		for i := range users {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"id":"u-%d","name":"user %d","tags":["a","b"]}`, i, i)
		}
		b.WriteString(`]}`)
		if err := protojson.Unmarshal([]byte(b.String()), res); err != nil {
			return usecase.InvokeRPCOutput{}, err
		}
		return usecase.InvokeRPCOutput{RPC: usecase.RPCSummary{FullName: in.RPCName}, Message: res}, nil
	}
}

func TestApp_LargeResponseOpensExplorer(t *testing.T) {
	h := newHarness(t, bigResponse(t, 50))
	h.submit("ListUsers")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)

	if !h.app().exploring {
		t.Fatalf("large response should open the explorer:\n%s", h.app().View())
	}
	out := h.output()
	if strings.Contains(out, `"u-49"`) || !strings.Contains(out, "exploring below") {
		t.Errorf("the full body should not be printed:\n%s", out)
	}
	view := h.app().View()
	if !strings.Contains(view, "users") || !strings.Contains(view, "[50 items]") || strings.Count(view, "\n") > 40 {
		t.Errorf("view:\n%s", view)
	}

	h.key(tea.KeyDown)
	h.key(tea.KeyEnter)
	h.typeText("/")
	h.typeText("q")
	if h.app().Mode() != ModeResponseViewer || !h.app().viewer.Searching() {
		t.Fatalf("q while searching must not leave the explorer, mode = %v", h.app().Mode())
	}
	h.key(tea.KeyEsc)
	if h.app().Mode() != ModeResponseViewer {
		t.Fatalf("esc should only cancel the search, mode = %v", h.app().Mode())
	}

	h.typeText("p")
	if !strings.Contains(h.output(), `"u-49"`) {
		t.Error("p should print the full body")
	}

	h.typeText("q")
	if h.app().Mode() != ModeCommand || h.app().exploring {
		t.Errorf("q should leave the response, mode = %v", h.app().Mode())
	}
}

func TestApp_SmallResponseCanBeExplored(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.submit("call CreateUser")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	if h.app().exploring || !strings.Contains(h.output(), `"id": "u-1"`) {
		t.Fatalf("small response should be printed as before")
	}
	if !strings.Contains(h.app().View(), "v explore") {
		t.Errorf("view:\n%s", h.app().View())
	}
	h.typeText("v")
	if !h.app().exploring || !strings.Contains(h.app().View(), ".user") {
		t.Errorf("v should open the explorer:\n%s", h.app().View())
	}
	h.typeText("e")
	if h.app().Mode() != ModeRequestEditor || h.app().exploring {
		t.Errorf("e should go back to the editor, mode = %v", h.app().Mode())
	}
}

func TestApp_ExploreLastStreamMessage(t *testing.T) {
	h := newHarness(t, okResponse(t))
	h.model = withStream(h.app(), fakeStream{executeFn: func(_ context.Context, in usecase.InvokeServerStreamInput) (usecase.InvokeServerStreamOutput, error) {
		for i := 1; i <= 2; i++ {
			_ = in.OnMessage(userMsg(t, fmt.Sprintf("u-%d", i)))
		}
		return usecase.InvokeServerStreamOutput{Count: 2}, nil
	}})
	h.submit("call WatchUsers")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	waitFor(t, h, func() bool { return h.app().Mode() == ModeResponseViewer })
	h.typeText("v")
	if !h.app().exploring || !strings.Contains(h.app().View(), `"u-2"`) {
		t.Errorf("v should explore the last message:\n%s", h.app().View())
	}
}

func TestApp_FailedResponseHasNoExplorer(t *testing.T) {
	h := newHarness(t, func(context.Context, usecase.InvokeRPCInput) (usecase.InvokeRPCOutput, error) {
		return usecase.InvokeRPCOutput{}, &invocation.Error{Code: "internal", Message: "boom"}
	})
	h.submit("call CreateUser")
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	h.typeText("v")
	if h.app().exploring || strings.Contains(h.app().View(), "v explore") {
		t.Errorf("errors have nothing to explore:\n%s", h.app().View())
	}
}
