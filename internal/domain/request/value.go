package request

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type Value struct {
	v protoreflect.Value
}

func ValueOf(v protoreflect.Value) Value {
	return Value{v: v}
}

func (v Value) Proto() protoreflect.Value {
	return v.v
}

func ParseScalar(fd protoreflect.FieldDescriptor, input string) (Value, error) {
	v, err := parseScalar(fd, input)
	if err != nil {
		return Value{}, &InvalidValueError{Field: string(fd.Name()), Input: input, Err: err}
	}
	return Value{v: v}, nil
}

func parseScalar(fd protoreflect.FieldDescriptor, input string) (protoreflect.Value, error) {
	s := strings.TrimSpace(input)
	switch fd.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(input), nil
	case protoreflect.BytesKind:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return protoreflect.ValueOfBytes([]byte(input)), nil
		}
		return protoreflect.ValueOfBytes(b), nil
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBool(b), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfInt32(int32(n)), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfInt64(n), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := strconv.ParseUint(s, 0, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfUint32(uint32(n)), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := strconv.ParseUint(s, 0, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfUint64(n), nil
	case protoreflect.FloatKind:
		f, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat32(float32(f)), nil
	case protoreflect.DoubleKind:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat64(f), nil
	case protoreflect.EnumKind:
		return parseEnum(fd.Enum(), s)
	default:
		return protoreflect.Value{}, ErrUnsupportedKind
	}
}

func parseEnum(ed protoreflect.EnumDescriptor, s string) (protoreflect.Value, error) {
	if vd := ed.Values().ByName(protoreflect.Name(s)); vd != nil {
		return protoreflect.ValueOfEnum(vd.Number()), nil
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
		return protoreflect.Value{}, fmt.Errorf("unknown enum value for %s", ed.FullName())
	}
	return protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)), nil
}

func FormatScalar(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		return base64.StdEncoding.EncodeToString(v.Bytes())
	case protoreflect.EnumKind:
		if vd := fd.Enum().Values().ByNumber(v.Enum()); vd != nil {
			return string(vd.Name())
		}
		return strconv.Itoa(int(v.Enum()))
	default:
		return fmt.Sprint(v.Interface())
	}
}

func ParseMapKey(fd protoreflect.FieldDescriptor, input string) (protoreflect.MapKey, error) {
	if !fd.IsMap() {
		return protoreflect.MapKey{}, fmt.Errorf("%s is not a map field", fd.Name())
	}
	v, err := ParseScalar(fd.MapKey(), input)
	if err != nil {
		return protoreflect.MapKey{}, err
	}
	return v.Proto().MapKey(), nil
}
