package schema_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/atsuya-m/pluto/internal/domain/schema"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

func TestSchema_IndexesAreSorted(t *testing.T) {
	s := fixture.Schema(t)

	var services []string
	for _, svc := range s.Services() {
		services = append(services, svc.FullName())
	}
	if want := []string{"admin.v1.AdminService", "user.v1.UserService"}; !slices.Equal(services, want) {
		t.Errorf("services = %v, want %v", services, want)
	}

	var rpcs []string
	for _, r := range s.RPCs() {
		rpcs = append(rpcs, r.FullName())
	}
	if !slices.IsSorted(rpcs) {
		t.Errorf("rpcs are not sorted: %v", rpcs)
	}
	if len(rpcs) != 10 {
		t.Errorf("len(rpcs) = %d, want 10", len(rpcs))
	}
}

func TestSchema_ResolveRPC(t *testing.T) {
	s := fixture.Schema(t)
	tests := []struct {
		query string
		want  string
	}{
		{"CreateUser", "user.v1.UserService.CreateUser"},
		{"UserService.CreateUser", "user.v1.UserService.CreateUser"},
		{"user.v1.UserService.CreateUser", "user.v1.UserService.CreateUser"},
		{"user.v1.UserService/CreateUser", "user.v1.UserService.CreateUser"},
		{"/user.v1.UserService/CreateUser", "user.v1.UserService.CreateUser"},
		{"AdminService.GetUser", "admin.v1.AdminService.GetUser"},
		{"BanUser", "admin.v1.AdminService.BanUser"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r, err := s.ResolveRPC(tt.query)
			if err != nil {
				t.Fatalf("ResolveRPC(%q) error = %v", tt.query, err)
			}
			if r.FullName() != tt.want {
				t.Errorf("ResolveRPC(%q) = %s, want %s", tt.query, r.FullName(), tt.want)
			}
		})
	}
}

func TestSchema_ResolveRPC_Ambiguous(t *testing.T) {
	_, err := fixture.Schema(t).ResolveRPC("GetUser")

	var ambiguous *schema.AmbiguousSymbolError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want AmbiguousSymbolError", err)
	}
	want := []string{"admin.v1.AdminService.GetUser", "user.v1.UserService.GetUser"}
	if !slices.Equal(ambiguous.Candidates, want) {
		t.Errorf("candidates = %v, want %v", ambiguous.Candidates, want)
	}
}

func TestSchema_ResolveNotFound(t *testing.T) {
	s := fixture.Schema(t)
	for _, q := range []string{"", "Nope", "serService.CreateUser", "reateUser"} {
		if _, err := s.ResolveRPC(q); !errors.Is(err, schema.ErrSymbolNotFound) {
			t.Errorf("ResolveRPC(%q) error = %v, want ErrSymbolNotFound", q, err)
		}
	}
	if _, err := s.ResolveMessage("Nope"); !errors.Is(err, schema.ErrSymbolNotFound) {
		t.Errorf("ResolveMessage error = %v, want ErrSymbolNotFound", err)
	}
}

func TestSchema_ResolveMessageAndEnum(t *testing.T) {
	s := fixture.Schema(t)

	m, err := s.ResolveMessage("Profile")
	if err != nil || m.FullName() != "user.v1.Profile" {
		t.Errorf("ResolveMessage(Profile) = %v, %v", m.FullName(), err)
	}

	e, err := s.ResolveEnum("UserStatus")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, v := range e.Values() {
		names = append(names, v.Name)
	}
	if want := []string{"USER_STATUS_UNSPECIFIED", "USER_STATUS_ACTIVE", "USER_STATUS_DISABLED"}; !slices.Equal(names, want) {
		t.Errorf("enum values = %v, want %v", names, want)
	}
}

func TestRPC(t *testing.T) {
	create := fixture.RPC(t, "CreateUser")
	if create.ServiceName() != "user.v1.UserService" || create.Procedure() != "/user.v1.UserService/CreateUser" {
		t.Errorf("service = %s, procedure = %s", create.ServiceName(), create.Procedure())
	}
	if create.Input().FullName() != "user.v1.CreateUserRequest" || create.Output().FullName() != "user.v1.CreateUserResponse" {
		t.Errorf("input = %s, output = %s", create.Input().FullName(), create.Output().FullName())
	}
	if !create.Unary() {
		t.Error("CreateUser should be unary")
	}

	watch := fixture.RPC(t, "WatchUsers")
	if watch.Unary() || !watch.ServerStreaming() || watch.ClientStreaming() {
		t.Errorf("WatchUsers streaming flags: unary=%v server=%v client=%v", watch.Unary(), watch.ServerStreaming(), watch.ClientStreaming())
	}
}

func TestMessage_Fields(t *testing.T) {
	m, err := fixture.Schema(t).ResolveMessage("user.v1.CreateUserRequest")
	if err != nil {
		t.Fatal(err)
	}
	fields := m.Fields()

	var numbers []int
	byName := map[string]schema.Field{}
	for _, f := range fields {
		numbers = append(numbers, f.Number())
		byName[f.Name()] = f
	}
	if !slices.IsSorted(numbers) {
		t.Errorf("fields are not ordered by number: %v", numbers)
	}

	tests := []struct {
		name        string
		typeName    string
		label       string
		optional    bool
		hasPresence bool
		oneof       string
	}{
		{"name", "string", "", false, false, ""},
		{"nickname", "string", "optional", true, true, ""},
		{"status", "user.v1.UserStatus", "", false, false, ""},
		{"tags", "string", "repeated", false, false, ""},
		{"profile", "user.v1.Profile", "", false, true, ""},
		{"labels", "map<string, string>", "", false, false, ""},
		{"email", "string", "", false, true, "contact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := byName[tt.name]
			if !ok {
				t.Fatalf("field %s not found", tt.name)
			}
			if f.TypeName() != tt.typeName {
				t.Errorf("TypeName = %q, want %q", f.TypeName(), tt.typeName)
			}
			if f.Label() != tt.label {
				t.Errorf("Label = %q, want %q", f.Label(), tt.label)
			}
			if f.IsOptional() != tt.optional {
				t.Errorf("IsOptional = %v, want %v", f.IsOptional(), tt.optional)
			}
			if f.HasPresence() != tt.hasPresence {
				t.Errorf("HasPresence = %v, want %v", f.HasPresence(), tt.hasPresence)
			}
			if f.OneofName() != tt.oneof || f.IsOneof() != (tt.oneof != "") {
				t.Errorf("OneofName = %q, IsOneof = %v, want %q", f.OneofName(), f.IsOneof(), tt.oneof)
			}
		})
	}

	if _, ok := byName["labels"].Message(); ok {
		t.Error("map field should not expose its entry message")
	}
	if sub, ok := byName["profile"].Message(); !ok || sub.FullName() != "user.v1.Profile" {
		t.Errorf("profile.Message() = %v, %v", sub.FullName(), ok)
	}
}

func TestNew_OnlyServices(t *testing.T) {
	s, err := schema.New(fixture.Registry(t), schema.OnlyServices("admin.v1.AdminService"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Services()) != 1 || len(s.RPCs()) != 2 {
		t.Errorf("services = %d, rpcs = %d", len(s.Services()), len(s.RPCs()))
	}
	if _, err := s.ResolveRPC("GetUser"); err != nil {
		t.Errorf("GetUser should no longer be ambiguous: %v", err)
	}
	if _, err := s.ResolveMessage("user.v1.User"); err != nil {
		t.Errorf("messages of filtered files should remain resolvable: %v", err)
	}
}
