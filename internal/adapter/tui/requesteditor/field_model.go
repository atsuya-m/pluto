package requesteditor

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type FieldModel struct {
	Name    string
	Type    string
	Display string
	Unset   bool
	Default bool
	Action  bool
}

type rowKind int

const (
	rowField rowKind = iota
	rowOneof
	rowElement
	rowEntry
	rowAdd
)

type row struct {
	kind    rowKind
	field   schema.Field
	oneof   protoreflect.OneofDescriptor
	members []schema.Field
	path    request.FieldPath
	key     string
	index   int
}

func messageRows(msg schema.Message, parent request.FieldPath) []row {
	var rows []row
	seen := map[protoreflect.FullName]bool{}
	for _, f := range msg.Fields() {
		if !f.IsOneof() {
			rows = append(rows, row{kind: rowField, field: f, path: parent.Append(f.Name())})
			continue
		}
		oneof := f.Descriptor().ContainingOneof()
		if seen[oneof.FullName()] {
			continue
		}
		seen[oneof.FullName()] = true
		var members []schema.Field
		for _, m := range msg.Fields() {
			if m.Descriptor().ContainingOneof() == oneof {
				members = append(members, m)
			}
		}
		rows = append(rows, row{kind: rowOneof, oneof: oneof, members: members, path: parent})
	}
	return rows
}

func containerRows(b *request.DynamicMessageBuilder, container schema.Field, path request.FieldPath) []row {
	var rows []row
	if container.IsMap() {
		for _, k := range b.MapKeys(path) {
			rows = append(rows, row{kind: rowEntry, field: container, path: path.AtKey(k), key: k})
		}
	} else {
		for i := range b.Len(path) {
			rows = append(rows, row{kind: rowElement, field: container, path: path.AtIndex(i), index: i})
		}
	}
	return append(rows, row{kind: rowAdd, field: container, path: path})
}

func buildRowModel(b *request.DynamicMessageBuilder, r row) FieldModel {
	switch r.kind {
	case rowOneof:
		fm := FieldModel{Name: string(r.oneof.Name()), Type: "oneof"}
		active := b.WhichOneof(r.path, r.oneof)
		if active == nil {
			fm.Display, fm.Unset = "<unset>", true
			return fm
		}
		for _, m := range r.members {
			if m.Descriptor() == active {
				inner := buildFieldModel(b, r.path.Append(m.Name()), m)
				fm.Display = m.Name() + ": " + inner.Display
			}
		}
		return fm
	case rowElement:
		fm := FieldModel{Name: fmt.Sprintf("[%d]", r.index), Type: shortType(r.field.Descriptor())}
		fm.Display = valueDisplay(b, r.path, r.field.Descriptor())
		return fm
	case rowEntry:
		value := r.field.Descriptor().MapValue()
		fm := FieldModel{Name: entryKey(r.field.Descriptor(), r.key), Type: shortType(value)}
		fm.Display = valueDisplay(b, r.path, value)
		return fm
	case rowAdd:
		return FieldModel{Name: "+ Add", Action: true}
	default:
		return buildFieldModel(b, r.path, r.field)
	}
}

func entryKey(fd protoreflect.FieldDescriptor, key string) string {
	if fd.MapKey().Kind() == protoreflect.StringKind {
		return strconv.Quote(key)
	}
	return key
}

func valueDisplay(b *request.DynamicMessageBuilder, path request.FieldPath, fd protoreflect.FieldDescriptor) string {
	v, has := b.Get(path)
	if !has {
		return "<unset>"
	}
	if fd.Message() != nil {
		return jsonPreview(b, path)
	}
	return scalarDisplay(fd, v)
}

func buildFieldModel(b *request.DynamicMessageBuilder, path request.FieldPath, f schema.Field) FieldModel {
	fm := FieldModel{Name: f.Name(), Type: typeLabel(f)}
	v, has := b.Get(path)
	fd := f.Descriptor()

	switch {
	case f.IsMap():
		if !has {
			fm.Display, fm.Default = "{}", true
			return fm
		}
		fm.Display = fmt.Sprintf("{%d entries} %s", v.Map().Len(), jsonPreview(b, path))
	case f.IsList():
		if !has {
			fm.Display, fm.Default = "[]", true
			return fm
		}
		fm.Display = fmt.Sprintf("[%d items] %s", v.List().Len(), jsonPreview(b, path))
	case fd.Message() != nil:
		switch {
		case !has:
			fm.Display, fm.Unset = "<unset>", true
		case isWellKnown(fd.Message()):
			fm.Display = jsonPreview(b, path)
		default:
			fm.Display = "{...}"
		}
	default:
		switch {
		case has:
			fm.Display = scalarDisplay(fd, v)
		case f.HasPresence():
			fm.Display, fm.Unset = "<unset>", true
		default:
			fm.Display, fm.Default = scalarDisplay(fd, fd.Default()), true
		}
	}
	return fm
}

func scalarDisplay(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	s := request.FormatScalar(fd, v)
	switch fd.Kind() {
	case protoreflect.StringKind, protoreflect.BytesKind:
		return strconv.Quote(s)
	default:
		return s
	}
}

func jsonPreview(b *request.DynamicMessageBuilder, path request.FieldPath) string {
	s, err := b.FieldJSON(path)
	if err != nil {
		return "<error>"
	}
	const limit = 48
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

func typeLabel(f schema.Field) string {
	t := shortType(f.Descriptor())
	if f.IsMap() {
		t = fmt.Sprintf("map<%s,%s>", shortType(f.Descriptor().MapKey()), shortType(f.Descriptor().MapValue()))
	}
	if l := f.Label(); l != "" {
		return l + " " + t
	}
	return t
}

func shortType(fd protoreflect.FieldDescriptor) string {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageTypeName(fd.Message())
	case protoreflect.EnumKind:
		return string(fd.Enum().Name())
	default:
		return fd.Kind().String()
	}
}

func messageTypeName(md protoreflect.MessageDescriptor) string {
	if isWellKnown(md) {
		return string(md.FullName())
	}
	return string(md.Name())
}

func isWellKnown(md protoreflect.MessageDescriptor) bool {
	return strings.HasPrefix(string(md.FullName()), "google.protobuf.")
}
