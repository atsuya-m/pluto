package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type DynamicMessageBuilder struct {
	message *dynamicpb.Message
}

func NewDynamicMessageBuilder(desc protoreflect.MessageDescriptor) *DynamicMessageBuilder {
	return &DynamicMessageBuilder{message: dynamicpb.NewMessage(desc)}
}

func NewDynamicMessageBuilderFrom(msg proto.Message) *DynamicMessageBuilder {
	desc := msg.ProtoReflect().Descriptor()
	b := NewDynamicMessageBuilder(desc)
	proto.Merge(b.message, msg)
	return b
}

func (b *DynamicMessageBuilder) Descriptor() protoreflect.MessageDescriptor {
	return b.message.Descriptor()
}

func (b *DynamicMessageBuilder) Message() proto.Message {
	return proto.Clone(b.message)
}

type target struct {
	parent protoreflect.Message
	field  protoreflect.FieldDescriptor
	step   Step
}

func (t target) list() protoreflect.List {
	return t.parent.Mutable(t.field).List()
}

func (t target) mapValue() protoreflect.Map {
	return t.parent.Mutable(t.field).Map()
}

func (b *DynamicMessageBuilder) Field(path FieldPath) (protoreflect.FieldDescriptor, error) {
	fd, err := b.containerField(path)
	if err != nil {
		return nil, err
	}
	if path.Last().IsKey() && fd.IsMap() {
		return fd.MapValue(), nil
	}
	return fd, nil
}

func (b *DynamicMessageBuilder) containerField(path FieldPath) (protoreflect.FieldDescriptor, error) {
	if len(path) == 0 {
		return nil, ErrEmptyPath
	}
	parent, err := b.descriptorAt(path.Parent())
	if err != nil {
		return nil, err
	}
	return fieldOf(parent, path.Last())
}

func (b *DynamicMessageBuilder) Set(path FieldPath, value Value) error {
	t, err := b.lookup(path, true)
	if err != nil {
		return err
	}
	switch {
	case t.step.IsIndex():
		l := t.list()
		if t.step.Index < 0 || t.step.Index >= l.Len() {
			return &IndexOutOfRangeError{Field: string(t.field.Name()), Index: t.step.Index, Len: l.Len()}
		}
		l.Set(t.step.Index, value.Proto())
	case t.step.IsKey():
		k, err := ParseMapKey(t.field, t.step.Key)
		if err != nil {
			return err
		}
		t.mapValue().Set(k, value.Proto())
	default:
		t.parent.Set(t.field, value.Proto())
	}
	return nil
}

func (b *DynamicMessageBuilder) Clear(path FieldPath) error {
	t, err := b.lookup(path, false)
	if err != nil || t.parent == nil {
		return err
	}
	switch {
	case t.step.IsIndex():
		return b.remove(t)
	case t.step.IsKey():
		k, err := ParseMapKey(t.field, t.step.Key)
		if err != nil {
			return err
		}
		if t.parent.Has(t.field) {
			t.mapValue().Clear(k)
		}
	default:
		t.parent.Clear(t.field)
	}
	return nil
}

func (b *DynamicMessageBuilder) Get(path FieldPath) (protoreflect.Value, bool) {
	t, err := b.lookup(path, false)
	if err != nil || t.parent == nil {
		return protoreflect.Value{}, false
	}
	switch {
	case t.step.IsIndex():
		l := t.parent.Get(t.field).List()
		if t.step.Index < 0 || t.step.Index >= l.Len() {
			return protoreflect.Value{}, false
		}
		return l.Get(t.step.Index), true
	case t.step.IsKey():
		k, err := ParseMapKey(t.field, t.step.Key)
		if err != nil {
			return protoreflect.Value{}, false
		}
		m := t.parent.Get(t.field).Map()
		return m.Get(k), m.Has(k)
	default:
		return t.parent.Get(t.field), t.parent.Has(t.field)
	}
}

func (b *DynamicMessageBuilder) Has(path FieldPath) bool {
	_, has := b.Get(path)
	return has
}

func (b *DynamicMessageBuilder) Len(path FieldPath) int {
	t, err := b.lookup(path.Container(), false)
	if err != nil || t.parent == nil || !t.parent.Has(t.field) {
		return 0
	}
	switch {
	case t.field.IsList():
		return t.parent.Get(t.field).List().Len()
	case t.field.IsMap():
		return t.parent.Get(t.field).Map().Len()
	default:
		return 0
	}
}

func (b *DynamicMessageBuilder) Append(path FieldPath, value Value) (int, error) {
	t, err := b.lookupList(path)
	if err != nil {
		return 0, err
	}
	l := t.list()
	l.Append(value.Proto())
	return l.Len() - 1, nil
}

func (b *DynamicMessageBuilder) AppendMessage(path FieldPath) (int, error) {
	t, err := b.lookupList(path)
	if err != nil {
		return 0, err
	}
	if t.field.Message() == nil {
		return 0, fmt.Errorf("%s is not a repeated message field", t.field.Name())
	}
	l := t.list()
	l.AppendMutable()
	return l.Len() - 1, nil
}

func (b *DynamicMessageBuilder) Move(path FieldPath, from, to int) error {
	t, err := b.lookupList(path)
	if err != nil {
		return err
	}
	l := t.list()
	if from < 0 || from >= l.Len() || to < 0 || to >= l.Len() {
		return &IndexOutOfRangeError{Field: string(t.field.Name()), Index: to, Len: l.Len()}
	}
	a, c := l.Get(from), l.Get(to)
	l.Set(from, c)
	l.Set(to, a)
	return nil
}

func (b *DynamicMessageBuilder) EnsureMapMessage(path FieldPath) error {
	t, err := b.lookup(path, true)
	if err != nil {
		return err
	}
	if !t.step.IsKey() || t.field.MapValue().Message() == nil {
		return fmt.Errorf("%s is not a map entry with a message value", path)
	}
	k, err := ParseMapKey(t.field, t.step.Key)
	if err != nil {
		return err
	}
	t.mapValue().Mutable(k)
	return nil
}

func (b *DynamicMessageBuilder) MapKeys(path FieldPath) []string {
	t, err := b.lookup(path.Container(), false)
	if err != nil || t.parent == nil || !t.field.IsMap() || !t.parent.Has(t.field) {
		return nil
	}
	var keys []protoreflect.MapKey
	t.parent.Get(t.field).Map().Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		keys = append(keys, k)
		return true
	})
	sort.Slice(keys, func(i, j int) bool { return lessMapKey(keys[i], keys[j]) })
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, FormatScalar(t.field.MapKey(), k.Value()))
	}
	return out
}

func (b *DynamicMessageBuilder) WhichOneof(path FieldPath, oneof protoreflect.OneofDescriptor) protoreflect.FieldDescriptor {
	msg, err := b.messageAt(path, false)
	if err != nil || msg == nil {
		return nil
	}
	return msg.WhichOneof(oneof)
}

func (b *DynamicMessageBuilder) SetJSON(path FieldPath, input string) error {
	if path.Last().IsIndex() || path.Last().IsKey() {
		return fmt.Errorf("JSON editing is only supported for whole fields, not %s", path)
	}
	fd, err := b.Field(path)
	if err != nil {
		return err
	}
	parentDesc := fd.ContainingMessage()
	raw := fmt.Sprintf("{%q: %s}", fd.JSONName(), input)
	tmp := dynamicpb.NewMessage(parentDesc)
	if err := protojson.Unmarshal([]byte(raw), tmp); err != nil {
		return &InvalidValueError{Field: string(fd.Name()), Input: input, Err: err}
	}
	t, err := b.lookup(path, tmp.Has(fd))
	if err != nil || t.parent == nil {
		return err
	}
	if !tmp.Has(fd) {
		t.parent.Clear(fd)
		return nil
	}
	t.parent.Set(fd, tmp.Get(fd))
	return nil
}

func (b *DynamicMessageBuilder) FieldJSON(path FieldPath) (string, error) {
	t, err := b.lookup(path, false)
	if err != nil {
		return "", err
	}
	if t.parent == nil {
		return "", nil
	}
	v, has := b.Get(path)
	if !has {
		return "", nil
	}

	fd := t.field
	var holder *dynamicpb.Message
	switch {
	case t.step.IsIndex():
		holder = dynamicpb.NewMessage(t.parent.Descriptor())
		holder.Mutable(fd).List().Append(v)
	case t.step.IsKey():
		k, _ := ParseMapKey(fd, t.step.Key)
		holder = dynamicpb.NewMessage(t.parent.Descriptor())
		holder.Mutable(fd).Map().Set(k, v)
	default:
		holder = dynamicpb.NewMessage(t.parent.Descriptor())
		holder.Set(fd, v)
	}
	out, err := protojson.Marshal(holder)
	if err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out, &fields); err != nil {
		return "", err
	}
	raw := fields[fd.JSONName()]
	switch {
	case t.step.IsIndex():
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 {
			return "", err
		}
		raw = items[0]
	case t.step.IsKey():
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil || len(entries) != 1 {
			return "", err
		}
		for _, e := range entries {
			raw = e
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return compact.String(), nil
}

func (b *DynamicMessageBuilder) remove(t target) error {
	if !t.parent.Has(t.field) {
		return nil
	}
	l := t.list()
	i := t.step.Index
	if i < 0 || i >= l.Len() {
		return &IndexOutOfRangeError{Field: string(t.field.Name()), Index: i, Len: l.Len()}
	}
	for j := i; j < l.Len()-1; j++ {
		l.Set(j, l.Get(j+1))
	}
	l.Truncate(l.Len() - 1)
	if l.Len() == 0 {
		t.parent.Clear(t.field)
	}
	return nil
}

func (b *DynamicMessageBuilder) lookupList(path FieldPath) (target, error) {
	t, err := b.lookup(path.Container(), true)
	if err != nil {
		return target{}, err
	}
	if !t.field.IsList() {
		return target{}, fmt.Errorf("%s is not a repeated field", t.field.Name())
	}
	return t, nil
}

func (b *DynamicMessageBuilder) lookup(path FieldPath, create bool) (target, error) {
	fd, err := b.containerField(path)
	if err != nil {
		return target{}, err
	}
	last := path.Last()
	if last.IsIndex() && !fd.IsList() || last.IsKey() && !fd.IsMap() {
		return target{}, &UnknownFieldError{Message: string(fd.ContainingMessage().FullName()), Field: last.String()}
	}
	parent, err := b.messageAt(path.Parent(), create)
	if err != nil {
		return target{}, err
	}
	return target{parent: parent, field: fd, step: last}, nil
}

func (b *DynamicMessageBuilder) descriptorAt(path FieldPath) (protoreflect.MessageDescriptor, error) {
	desc := b.message.Descriptor()
	for _, step := range path {
		fd, err := fieldOf(desc, step)
		if err != nil {
			return nil, err
		}
		md, err := stepMessage(fd, step)
		if err != nil {
			return nil, err
		}
		desc = md
	}
	return desc, nil
}

func (b *DynamicMessageBuilder) messageAt(path FieldPath, create bool) (protoreflect.Message, error) {
	var msg protoreflect.Message = b.message
	for _, step := range path {
		fd, err := fieldOf(msg.Descriptor(), step)
		if err != nil {
			return nil, err
		}
		if _, err := stepMessage(fd, step); err != nil {
			return nil, err
		}
		switch {
		case step.IsIndex():
			if !msg.Has(fd) {
				return nil, &IndexOutOfRangeError{Field: string(fd.Name()), Index: step.Index, Len: 0}
			}
			l := msg.Get(fd).List()
			if step.Index < 0 || step.Index >= l.Len() {
				return nil, &IndexOutOfRangeError{Field: string(fd.Name()), Index: step.Index, Len: l.Len()}
			}
			msg = l.Get(step.Index).Message()
		case step.IsKey():
			k, err := ParseMapKey(fd, step.Key)
			if err != nil {
				return nil, err
			}
			if create {
				msg = msg.Mutable(fd).Map().Mutable(k).Message()
				continue
			}
			if !msg.Has(fd) || !msg.Get(fd).Map().Has(k) {
				return nil, nil
			}
			msg = msg.Get(fd).Map().Get(k).Message()
		default:
			if create {
				msg = msg.Mutable(fd).Message()
				continue
			}
			if !msg.Has(fd) {
				return nil, nil
			}
			msg = msg.Get(fd).Message()
		}
	}
	return msg, nil
}

func fieldOf(desc protoreflect.MessageDescriptor, step Step) (protoreflect.FieldDescriptor, error) {
	fd := desc.Fields().ByName(protoreflect.Name(step.Field))
	if fd == nil {
		return nil, &UnknownFieldError{Message: string(desc.FullName()), Field: step.Field}
	}
	return fd, nil
}

func stepMessage(fd protoreflect.FieldDescriptor, step Step) (protoreflect.MessageDescriptor, error) {
	switch {
	case step.IsIndex() && fd.IsList() && fd.Message() != nil:
		return fd.Message(), nil
	case step.IsKey() && fd.IsMap() && fd.MapValue().Message() != nil:
		return fd.MapValue().Message(), nil
	case !step.IsIndex() && !step.IsKey() && !fd.IsList() && !fd.IsMap() && fd.Message() != nil:
		return fd.Message(), nil
	default:
		return nil, ErrNotSingularMsg
	}
}

func lessMapKey(a, b protoreflect.MapKey) bool {
	switch av := a.Interface().(type) {
	case string:
		return av < b.Interface().(string)
	case bool:
		return !av && b.Interface().(bool)
	case int32:
		return av < b.Interface().(int32)
	case int64:
		return av < b.Interface().(int64)
	case uint32:
		return av < b.Interface().(uint32)
	case uint64:
		return av < b.Interface().(uint64)
	default:
		return fmt.Sprint(av) < fmt.Sprint(b.Interface())
	}
}
