package request_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

func field(md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	return md.Fields().ByName(protoreflect.Name(name))
}

func TestParseScalar(t *testing.T) {
	createUser := fixture.Message(t, "user.v1.CreateUserRequest")
	fieldOpts := (&descriptorpb.FieldOptions{}).ProtoReflect().Descriptor()
	wrappers := map[string]protoreflect.MessageDescriptor{
		"int64":  (&wrapperspb.Int64Value{}).ProtoReflect().Descriptor(),
		"uint32": (&wrapperspb.UInt32Value{}).ProtoReflect().Descriptor(),
		"uint64": (&wrapperspb.UInt64Value{}).ProtoReflect().Descriptor(),
		"float":  (&wrapperspb.FloatValue{}).ProtoReflect().Descriptor(),
		"double": (&wrapperspb.DoubleValue{}).ProtoReflect().Descriptor(),
		"bytes":  (&wrapperspb.BytesValue{}).ProtoReflect().Descriptor(),
	}

	tests := []struct {
		name  string
		fd    protoreflect.FieldDescriptor
		input string
		want  any
	}{
		{"string keeps spaces", field(createUser, "name"), " Taro ", " Taro "},
		{"int32", field(createUser, "age"), "20", int32(20)},
		{"int32 hex", field(createUser, "age"), "0x10", int32(16)},
		{"int32 trims", field(createUser, "age"), " 7 ", int32(7)},
		{"bool", field(createUser, "admin"), "true", true},
		{"enum by name", field(createUser, "status"), "USER_STATUS_ACTIVE", protoreflect.EnumNumber(1)},
		{"enum by number", field(createUser, "status"), "2", protoreflect.EnumNumber(2)},
		{"int64", field(wrappers["int64"], "value"), "-9000000000", int64(-9000000000)},
		{"uint32", field(wrappers["uint32"], "value"), "42", uint32(42)},
		{"uint64", field(wrappers["uint64"], "value"), "18446744073709551615", uint64(18446744073709551615)},
		{"float", field(wrappers["float"], "value"), "1.5", float32(1.5)},
		{"double", field(wrappers["double"], "value"), "2.25", 2.25},
		{"bytes base64", field(wrappers["bytes"], "value"), "aGk=", []byte("hi")},
		{"bytes raw fallback", field(wrappers["bytes"], "value"), "not base64!", []byte("not base64!")},
		{"bool option", field(fieldOpts, "deprecated"), "false", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := request.ParseScalar(tt.fd, tt.input)
			if err != nil {
				t.Fatalf("ParseScalar error = %v", err)
			}
			got := v.Proto().Interface()
			if b, ok := tt.want.([]byte); ok {
				if string(got.([]byte)) != string(b) {
					t.Errorf("got %q, want %q", got, b)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseScalar_Errors(t *testing.T) {
	createUser := fixture.Message(t, "user.v1.CreateUserRequest")
	tests := []struct {
		name  string
		fd    protoreflect.FieldDescriptor
		input string
	}{
		{"int32 text", field(createUser, "age"), "abc"},
		{"int32 overflow", field(createUser, "age"), "3000000000"},
		{"bool text", field(createUser, "admin"), "yes please"},
		{"unknown enum", field(createUser, "status"), "USER_STATUS_NOPE"},
		{"message kind", field(createUser, "profile"), "{}"},
		{"struct", field((&structpb.Struct{}).ProtoReflect().Descriptor(), "fields"), "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := request.ParseScalar(tt.fd, tt.input)
			var invalid *request.InvalidValueError
			if !errors.As(err, &invalid) {
				t.Errorf("error = %v, want InvalidValueError", err)
			}
		})
	}
}

func TestFormatScalar(t *testing.T) {
	createUser := fixture.Message(t, "user.v1.CreateUserRequest")
	bytesField := field((&wrapperspb.BytesValue{}).ProtoReflect().Descriptor(), "value")

	tests := []struct {
		name string
		fd   protoreflect.FieldDescriptor
		v    protoreflect.Value
		want string
	}{
		{"string", field(createUser, "name"), protoreflect.ValueOfString("Taro"), "Taro"},
		{"int32", field(createUser, "age"), protoreflect.ValueOfInt32(20), "20"},
		{"enum known", field(createUser, "status"), protoreflect.ValueOfEnum(1), "USER_STATUS_ACTIVE"},
		{"enum unknown", field(createUser, "status"), protoreflect.ValueOfEnum(99), "99"},
		{"bytes", bytesField, protoreflect.ValueOfBytes([]byte("hi")), "aGk="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := request.FormatScalar(tt.fd, tt.v); got != tt.want {
				t.Errorf("FormatScalar = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFieldPath(t *testing.T) {
	p := request.Path("profile")
	child := p.Append("address")
	grandchild := child.Append("city")

	if grandchild.String() != "profile.address.city" || grandchild.Last().Field != "city" {
		t.Errorf("grandchild = %s, last = %s", grandchild, grandchild.Last())
	}
	if grandchild.Parent().String() != "profile.address" {
		t.Errorf("parent = %s", grandchild.Parent())
	}
	if len(p) != 1 || len(child) != 2 {
		t.Error("Append must not mutate the receiver")
	}
	var empty request.FieldPath
	if empty.Parent() != nil || empty.Last() != (request.Step{}) {
		t.Error("empty path helpers should return zero values")
	}
}
