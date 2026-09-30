package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/atsuya-m/pluto/internal/domain/schema"
	protoloader "github.com/atsuya-m/pluto/internal/infrastructure/schema/proto"
)

type handlerFunc func(ctx context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error)

type server struct {
	schema *schema.Schema

	mu       sync.Mutex
	nextID   int
	users    map[string]*dynamicpb.Message
	user     protoreflect.MessageDescriptor
	watchers map[chan *dynamicpb.Message]struct{}
}

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	protoDir := flag.String("proto", "testdata/proto", "proto directory")
	flag.Parse()

	s, err := protoloader.NewSchemaLoader([]string{*protoDir}, nil).Load(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	userMsg, err := s.ResolveMessage("user.v1.User")
	if err != nil {
		log.Fatal(err)
	}
	srv := &server{schema: s, users: map[string]*dynamicpb.Message{}, user: userMsg.Descriptor(), watchers: map[chan *dynamicpb.Message]struct{}{}}

	mux := http.NewServeMux()
	routes := map[string]handlerFunc{
		"user.v1.UserService.CreateUser": srv.createUser,
		"user.v1.UserService.GetUser":    srv.getUser,
		"user.v1.UserService.ListUsers":  srv.listUsers,
		"user.v1.UserService.UpdateUser": srv.updateUser,
		"user.v1.UserService.DeleteUser": srv.deleteUser,
		"admin.v1.AdminService.GetUser":  srv.getUser,
		"admin.v1.AdminService.BanUser":  srv.banUser,
	}
	for name, fn := range routes {
		rpc, err := s.ResolveRPC(name)
		if err != nil {
			log.Fatal(err)
		}
		mux.Handle(rpc.Procedure(), unaryHandler(rpc, fn))
	}

	watch, err := s.ResolveRPC("user.v1.UserService.WatchUsers")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle(watch.Procedure(), serverStreamHandler(watch, srv.watchUsers))

	importUsers, err := s.ResolveRPC("user.v1.UserService.ImportUsers")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle(importUsers.Procedure(), connect.NewClientStreamHandler(importUsers.Procedure(), srv.importUsers, streamOptions(importUsers)...))

	chat, err := s.ResolveRPC("user.v1.UserService.Chat")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle(chat.Procedure(), connect.NewBidiStreamHandler(chat.Procedure(), srv.chat, streamOptions(chat)...))

	reflector := grpcreflect.NewReflector(
		grpcreflect.NamerFunc(func() []string {
			names := make([]string, 0)
			for _, svc := range s.Services() {
				names = append(names, svc.FullName())
			}
			return names
		}),
		grpcreflect.WithDescriptorResolver(s.Files()),
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	httpServer := &http.Server{Addr: *addr, Handler: logging(mux), Protocols: protocols}

	log.Printf("example user server listening on http://%s (connect, grpc, grpc-web, reflection)", *addr)
	log.Fatal(httpServer.ListenAndServe())
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s (%s)", r.Proto, r.Method, r.URL.Path, time.Since(start).Round(time.Microsecond))
	})
}

func unaryHandler(rpc schema.RPC, fn handlerFunc) http.Handler {
	md := rpc.Descriptor()
	return connect.NewUnaryHandler(
		rpc.Procedure(),
		func(ctx context.Context, req *connect.Request[dynamicpb.Message]) (*connect.Response[dynamicpb.Message], error) {
			res, err := fn(ctx, req.Msg)
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(res), nil
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

func serverStreamHandler(rpc schema.RPC, fn func(context.Context, *connect.Request[dynamicpb.Message], *connect.ServerStream[dynamicpb.Message]) error) http.Handler {
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

func streamOptions(rpc schema.RPC) []connect.HandlerOption {
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

func (s *server) importUsers(ctx context.Context, stream *connect.ClientStream[dynamicpb.Message]) (*connect.Response[dynamicpb.Message], error) {
	var imported int32
	for stream.Receive() {
		if _, err := s.createUser(ctx, stream.Msg()); err != nil {
			return nil, err
		}
		imported++
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	res := newResponse("user.v1.ImportUsersResponse", s.schema)
	res.Set(field(res, "imported"), protoreflect.ValueOfInt32(imported))
	return connect.NewResponse(res), nil
}

func (s *server) chat(_ context.Context, stream *connect.BidiStream[dynamicpb.Message, dynamicpb.Message]) error {
	for {
		msg, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		reply := newResponse("user.v1.ChatMessage", s.schema)
		reply.Set(field(reply, "text"), protoreflect.ValueOfString("echo: "+str(msg, "text")))
		if err := stream.Send(reply); err != nil {
			return err
		}
	}
}

func (s *server) watchUsers(ctx context.Context, _ *connect.Request[dynamicpb.Message], stream *connect.ServerStream[dynamicpb.Message]) error {
	updates := make(chan *dynamicpb.Message, 16)
	s.mu.Lock()
	ids := make([]string, 0, len(s.users))
	for id := range s.users {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	existing := make([]*dynamicpb.Message, 0, len(ids))
	for _, id := range ids {
		existing = append(existing, proto.Clone(s.users[id]).(*dynamicpb.Message))
	}
	s.watchers[updates] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.watchers, updates)
		s.mu.Unlock()
	}()

	for _, u := range existing {
		if err := stream.Send(u); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case u := <-updates:
			if err := stream.Send(u); err != nil {
				return err
			}
		}
	}
}

func (s *server) notify(user *dynamicpb.Message) {
	for w := range s.watchers {
		select {
		case w <- proto.Clone(user).(*dynamicpb.Message):
		default:
		}
	}
}

func field(m protoreflect.Message, name string) protoreflect.FieldDescriptor {
	return m.Descriptor().Fields().ByName(protoreflect.Name(name))
}

func str(m protoreflect.Message, name string) string {
	return m.Get(field(m, name)).String()
}

func newResponse(name string, s *schema.Schema) *dynamicpb.Message {
	md, err := s.ResolveMessage(name)
	if err != nil {
		panic(err)
	}
	return dynamicpb.NewMessage(md.Descriptor())
}

func (s *server) withUser(name string, user *dynamicpb.Message) *dynamicpb.Message {
	res := newResponse(name, s.schema)
	if user != nil {
		res.Set(field(res, "user"), protoreflect.ValueOfMessage(proto.Clone(user).(*dynamicpb.Message)))
	}
	return res
}

func (s *server) createUser(_ context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	if str(req, "name") == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	user := dynamicpb.NewMessage(s.user)
	copyCommonFields(req, user)
	user.Set(field(user, "id"), protoreflect.ValueOfString(fmt.Sprintf("u-%03d", s.nextID)))
	if user.Get(field(user, "status")).Enum() == 0 {
		user.Set(field(user, "status"), protoreflect.ValueOfEnum(1))
	}
	now := time.Now()
	ts := user.Mutable(field(user, "create_time")).Message()
	ts.Set(field(ts, "seconds"), protoreflect.ValueOfInt64(now.Unix()))
	ts.Set(field(ts, "nanos"), protoreflect.ValueOfInt32(int32(now.Nanosecond())))

	s.users[str(user, "id")] = user
	s.notify(user)
	return s.withUser("user.v1.CreateUserResponse", user), nil
}

func (s *server) getUser(_ context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[str(req, "id")]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user %q not found", str(req, "id")))
	}
	return s.withUser("user.v1.GetUserResponse", user), nil
}

func (s *server) listUsers(_ context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.users))
	for id := range s.users {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	res := newResponse("user.v1.ListUsersResponse", s.schema)
	list := res.Mutable(field(res, "users")).List()
	status := req.Get(field(req, "status")).Enum()
	limit := int(req.Get(field(req, "page_size")).Int())
	for _, id := range ids {
		u := s.users[id]
		if status != 0 && u.Get(field(u, "status")).Enum() != status {
			continue
		}
		if limit > 0 && list.Len() >= limit {
			break
		}
		list.Append(protoreflect.ValueOfMessage(proto.Clone(u).(*dynamicpb.Message)))
	}
	return res, nil
}

func (s *server) updateUser(_ context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[str(req, "id")]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user %q not found", str(req, "id")))
	}
	copyCommonFields(req, user)
	s.notify(user)
	return s.withUser("user.v1.UpdateUserResponse", user), nil
}

func (s *server) deleteUser(_ context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := str(req, "id")
	if _, ok := s.users[id]; !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user %q not found", id))
	}
	delete(s.users, id)
	return newResponse("user.v1.DeleteUserResponse", s.schema), nil
}

func (s *server) banUser(_ context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[str(req, "id")]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user %q not found", str(req, "id")))
	}
	user.Set(field(user, "status"), protoreflect.ValueOfEnum(2))
	labels := user.Mutable(field(user, "labels")).Map()
	labels.Set(protoreflect.ValueOfString("ban_reason").MapKey(), protoreflect.ValueOfString(str(req, "reason")))
	return s.withUser("admin.v1.BanUserResponse", user), nil
}

func copyCommonFields(src, dst *dynamicpb.Message) {
	src.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		target := dst.Descriptor().Fields().ByName(fd.Name())
		if target == nil || target.Name() == "id" {
			return true
		}
		if target.Kind() != fd.Kind() || target.Cardinality() != fd.Cardinality() || target.IsMap() != fd.IsMap() {
			return true
		}
		switch {
		case fd.IsList():
			l := dst.Mutable(target).List()
			l.Truncate(0)
			for i := 0; i < v.List().Len(); i++ {
				l.Append(v.List().Get(i))
			}
		case fd.IsMap():
			m := dst.Mutable(target).Map()
			v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
				m.Set(k, mv)
				return true
			})
		case fd.Message() != nil:
			b, err := proto.Marshal(v.Message().Interface())
			if err == nil {
				_ = proto.Unmarshal(b, dst.Mutable(target).Message().Interface())
			}
		default:
			dst.Set(target, v)
		}
		return true
	})
}
