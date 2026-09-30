package request_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

func newCreateUserBuilder(t *testing.T) *request.DynamicMessageBuilder {
	t.Helper()
	return request.NewDynamicMessageBuilder(fixture.Message(t, "user.v1.CreateUserRequest"))
}

func mustSet(t *testing.T, b *request.DynamicMessageBuilder, path request.FieldPath, input string) {
	t.Helper()
	fd, err := b.Field(path)
	if err != nil {
		t.Fatalf("Field(%s): %v", path, err)
	}
	v, err := request.ParseScalar(fd, input)
	if err != nil {
		t.Fatalf("ParseScalar(%s, %q): %v", path, input, err)
	}
	if err := b.Set(path, v); err != nil {
		t.Fatalf("Set(%s): %v", path, err)
	}
}

func toJSON(t *testing.T, m proto.Message) string {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestBuilder_OptionalPresence(t *testing.T) {
	path := request.Path("nickname")

	t.Run("unset", func(t *testing.T) {
		b := newCreateUserBuilder(t)
		if b.Has(path) {
			t.Error("Has = true, want false")
		}
	})

	t.Run("empty", func(t *testing.T) {
		b := newCreateUserBuilder(t)
		mustSet(t, b, path, "")
		v, has := b.Get(path)
		if !has || v.String() != "" {
			t.Errorf("Get = %q, %v, want \"\", true", v.String(), has)
		}
	})

	t.Run("value", func(t *testing.T) {
		b := newCreateUserBuilder(t)
		mustSet(t, b, path, "foo")
		v, has := b.Get(path)
		if !has || v.String() != "foo" {
			t.Errorf("Get = %q, %v, want \"foo\", true", v.String(), has)
		}
	})

	t.Run("clear after value", func(t *testing.T) {
		b := newCreateUserBuilder(t)
		mustSet(t, b, path, "foo")
		if err := b.Clear(path); err != nil {
			t.Fatal(err)
		}
		if b.Has(path) {
			t.Error("Has = true after Clear")
		}
	})
}

func TestBuilder_ImplicitPresenceZeroIsAbsent(t *testing.T) {
	b := newCreateUserBuilder(t)
	mustSet(t, b, request.Path("name"), "")
	if b.Has(request.Path("name")) {
		t.Error("proto3 implicit field with zero value should not be present")
	}
}

func TestBuilder_Oneof(t *testing.T) {
	b := newCreateUserBuilder(t)
	email, phone := request.Path("email"), request.Path("phone")

	mustSet(t, b, email, "a@example.com")
	if !b.Has(email) || b.Has(phone) {
		t.Errorf("after email: email=%v phone=%v", b.Has(email), b.Has(phone))
	}

	mustSet(t, b, phone, "090")
	if b.Has(email) || !b.Has(phone) {
		t.Errorf("after phone: email=%v phone=%v", b.Has(email), b.Has(phone))
	}

	oneof := b.Descriptor().Oneofs().ByName("contact")
	if got := b.WhichOneof(nil, oneof); got == nil || got.Name() != "phone" {
		t.Errorf("WhichOneof = %v, want phone", got)
	}
}

func TestBuilder_NestedPath(t *testing.T) {
	b := newCreateUserBuilder(t)
	city := request.Path("profile", "address", "city")

	if b.Has(request.Path("profile")) {
		t.Fatal("profile should start unset")
	}
	mustSet(t, b, city, "Tokyo")
	if !b.Has(request.Path("profile")) || !b.Has(request.Path("profile", "address")) {
		t.Error("setting a nested field should make its parents present")
	}
	if got := toJSON(t, b.Message()); got != `{"profile":{"address":{"city":"Tokyo"}}}` {
		t.Errorf("message = %s", got)
	}

	if err := b.Clear(request.Path("profile")); err != nil {
		t.Fatal(err)
	}
	if b.Has(city) || b.Has(request.Path("profile")) {
		t.Error("clearing a parent should remove nested values")
	}
	if err := b.Clear(city); err != nil {
		t.Errorf("clearing a field under an unset parent should be a no-op: %v", err)
	}
}

func TestBuilder_Repeated(t *testing.T) {
	b := newCreateUserBuilder(t)
	tags := request.Path("tags")

	steps := []struct {
		name  string
		input string
		want  string
	}{
		{"add", `["go"]`, `["go"]`},
		{"add more", `["go","grpc"]`, `["go","grpc"]`},
		{"edit", `["go","connect"]`, `["go","connect"]`},
		{"delete", `["connect"]`, `["connect"]`},
	}
	for _, s := range steps {
		if err := b.SetJSON(tags, s.input); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		got, err := b.FieldJSON(tags)
		if err != nil || got != s.want {
			t.Errorf("%s: FieldJSON = %s, %v, want %s", s.name, got, err, s.want)
		}
	}

	if err := b.SetJSON(tags, `[]`); err != nil {
		t.Fatal(err)
	}
	if b.Has(tags) {
		t.Error("empty list should not be present")
	}
}

func TestBuilder_Map(t *testing.T) {
	b := newCreateUserBuilder(t)
	labels := request.Path("labels")

	steps := []struct {
		name  string
		input string
		want  string
	}{
		{"add", `{"env":"dev"}`, `{"env":"dev"}`},
		{"update", `{"env":"prod"}`, `{"env":"prod"}`},
		{"add second", `{"env":"prod","team":"backend"}`, `{"env":"prod","team":"backend"}`},
		{"delete", `{"team":"backend"}`, `{"team":"backend"}`},
	}
	for _, s := range steps {
		if err := b.SetJSON(labels, s.input); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		got, err := b.FieldJSON(labels)
		if err != nil || got != s.want {
			t.Errorf("%s: FieldJSON = %s, %v, want %s", s.name, got, err, s.want)
		}
	}
}

func TestBuilder_SetJSONInvalid(t *testing.T) {
	b := newCreateUserBuilder(t)
	err := b.SetJSON(request.Path("tags"), `["unterminated`)

	var invalid *request.InvalidValueError
	if !errors.As(err, &invalid) {
		t.Errorf("error = %v, want InvalidValueError", err)
	}
}

func TestBuilder_UnknownField(t *testing.T) {
	b := newCreateUserBuilder(t)
	err := b.Clear(request.Path("nope"))

	var unknown *request.UnknownFieldError
	if !errors.As(err, &unknown) {
		t.Errorf("error = %v, want UnknownFieldError", err)
	}
	if err := b.Clear(nil); !errors.Is(err, request.ErrEmptyPath) {
		t.Errorf("Clear(nil) error = %v, want ErrEmptyPath", err)
	}
	if err := b.SetJSON(request.Path("tags", "x"), `1`); !errors.Is(err, request.ErrNotSingularMsg) {
		t.Errorf("path through a list error = %v, want ErrNotSingularMsg", err)
	}
}

func TestBuilder_MessageIsACopy(t *testing.T) {
	b := newCreateUserBuilder(t)
	mustSet(t, b, request.Path("name"), "Taro")
	snapshot := b.Message()
	mustSet(t, b, request.Path("name"), "Jiro")

	if got := toJSON(t, snapshot); got != `{"name":"Taro"}` {
		t.Errorf("snapshot changed after further edits: %s", got)
	}
}

func TestNewDynamicMessageBuilderFrom(t *testing.T) {
	src := newCreateUserBuilder(t)
	mustSet(t, src, request.Path("name"), "Taro")
	b := request.NewDynamicMessageBuilderFrom(src.Message())

	if got := toJSON(t, b.Message()); got != `{"name":"Taro"}` {
		t.Errorf("message = %s", got)
	}
}

func TestBuilder_ListElements(t *testing.T) {
	b := newCreateUserBuilder(t)
	tags := request.Path("tags")

	for _, s := range []string{"a", "b", "c"} {
		v, _ := request.ParseScalar(mustField(t, b, tags), s)
		if _, err := b.Append(tags, v); err != nil {
			t.Fatal(err)
		}
	}
	if b.Len(tags) != 3 {
		t.Fatalf("Len = %d", b.Len(tags))
	}

	mustSet(t, b, tags.AtIndex(1), "B")
	if v, ok := b.Get(tags.AtIndex(1)); !ok || v.String() != "B" {
		t.Errorf("tags[1] = %q, %v", v.String(), ok)
	}
	if err := b.Move(tags, 0, 2); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.FieldJSON(tags); got != `["c","B","a"]` {
		t.Errorf("after move: %s", got)
	}
	if err := b.Clear(tags.AtIndex(1)); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.FieldJSON(tags); got != `["c","a"]` {
		t.Errorf("after delete: %s", got)
	}
	if got, _ := b.FieldJSON(tags.AtIndex(0)); got != `"c"` {
		t.Errorf("element json = %s", got)
	}

	var outOfRange *request.IndexOutOfRangeError
	if err := b.Set(tags.AtIndex(5), request.ValueOf(protoreflect.ValueOfString("x"))); !errors.As(err, &outOfRange) {
		t.Errorf("Set out of range error = %v", err)
	}
	if b.Has(tags.AtIndex(5)) {
		t.Error("Has out of range should be false")
	}

	_ = b.Clear(tags.AtIndex(0))
	_ = b.Clear(tags.AtIndex(0))
	if b.Has(tags) || b.Len(tags) != 0 {
		t.Error("removing the last element should unset the field")
	}
}

func TestBuilder_RepeatedMessages(t *testing.T) {
	b := newCreateUserBuilder(t)
	addresses := request.Path("addresses")

	i, err := b.AppendMessage(addresses)
	if err != nil || i != 0 {
		t.Fatalf("AppendMessage = %d, %v", i, err)
	}
	mustSet(t, b, addresses.AtIndex(0).Append("city"), "Tokyo")
	j, _ := b.AppendMessage(addresses)
	mustSet(t, b, addresses.AtIndex(j).Append("city"), "Osaka")

	if got := toJSON(t, b.Message()); got != `{"addresses":[{"city":"Tokyo"},{"city":"Osaka"}]}` {
		t.Errorf("message = %s", got)
	}
	if got := addresses.AtIndex(1).Append("city").String(); got != "addresses[1].city" {
		t.Errorf("path string = %s", got)
	}
	if _, err := b.AppendMessage(request.Path("tags")); err == nil {
		t.Error("AppendMessage on a scalar list should fail")
	}
}

func TestBuilder_MapEntries(t *testing.T) {
	b := newCreateUserBuilder(t)
	scores := request.Path("scores")

	mustSet(t, b, scores.AtKey("10"), "ten")
	mustSet(t, b, scores.AtKey("2"), "two")
	mustSet(t, b, scores.AtKey("-1"), "minus")
	if got := b.MapKeys(scores); strings.Join(got, ",") != "-1,2,10" {
		t.Errorf("keys should be sorted numerically: %v", got)
	}
	if v, ok := b.Get(scores.AtKey("2")); !ok || v.String() != "two" {
		t.Errorf("scores[2] = %q, %v", v.String(), ok)
	}
	mustSet(t, b, scores.AtKey("2"), "TWO")
	if err := b.Clear(scores.AtKey("10")); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.FieldJSON(scores); got != `{"-1":"minus","2":"TWO"}` {
		t.Errorf("scores = %s", got)
	}
	if b.Len(scores) != 2 {
		t.Errorf("Len = %d", b.Len(scores))
	}

	var invalid *request.InvalidValueError
	if err := b.Set(scores.AtKey("abc"), request.ValueOf(protoreflect.ValueOfString("x"))); !errors.As(err, &invalid) {
		t.Errorf("invalid key error = %v", err)
	}
}

func TestBuilder_MapMessageValues(t *testing.T) {
	b := newCreateUserBuilder(t)
	places := request.Path("places")

	if err := b.EnsureMapMessage(places.AtKey("home")); err != nil {
		t.Fatal(err)
	}
	if !b.Has(places.AtKey("home")) {
		t.Fatal("EnsureMapMessage should create the entry")
	}
	mustSet(t, b, places.AtKey("home").Append("city"), "Tokyo")
	if got := toJSON(t, b.Message()); got != `{"places":{"home":{"city":"Tokyo"}}}` {
		t.Errorf("message = %s", got)
	}
	if got, _ := b.FieldJSON(places.AtKey("home")); got != `{"city":"Tokyo"}` {
		t.Errorf("entry json = %s", got)
	}
	if err := b.EnsureMapMessage(request.Path("scores").AtKey("1")); err == nil {
		t.Error("EnsureMapMessage on scalar map should fail")
	}
}

func TestBuilder_RepeatedEnum(t *testing.T) {
	b := newCreateUserBuilder(t)
	history := request.Path("history")
	v, err := request.ParseScalar(mustField(t, b, history), "USER_STATUS_ACTIVE")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Append(history, v); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.FieldJSON(history); got != `["USER_STATUS_ACTIVE"]` {
		t.Errorf("history = %s", got)
	}
}

func TestBuilder_InvalidStepKinds(t *testing.T) {
	b := newCreateUserBuilder(t)
	if err := b.Clear(request.Path("name").AtIndex(0)); err == nil {
		t.Error("index on a singular field should fail")
	}
	if err := b.Clear(request.Path("tags").AtKey("x")); err == nil {
		t.Error("key on a list should fail")
	}
	if err := b.SetJSON(request.Path("tags").AtIndex(0), `"x"`); err == nil {
		t.Error("SetJSON on an element should fail")
	}
}

func mustField(t *testing.T, b *request.DynamicMessageBuilder, path request.FieldPath) protoreflect.FieldDescriptor {
	t.Helper()
	fd, err := b.Field(path)
	if err != nil {
		t.Fatal(err)
	}
	return fd
}
