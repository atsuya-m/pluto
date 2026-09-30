package requesteditor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/atsuya-m/pluto/internal/adapter/tui/style"
	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

type PreviewMsg struct{}

type CancelMsg struct{}

type EditorMode int

const (
	ModeBrowse EditorMode = iota
	ModeText
	ModeChoice
)

type FrameKind int

const (
	FrameMessage FrameKind = iota
	FrameList
	FrameMap
)

type EditorFrame struct {
	Kind    FrameKind
	Message schema.Message
	Field   schema.Field
	Path    request.FieldPath
	Title   string
	Cursor  int
	Offset  int
}

type editAction int

const (
	actSet editAction = iota
	actAppend
	actMapKey
	actJSON
)

type editTarget struct {
	path      request.FieldPath
	fd        protoreflect.FieldDescriptor
	container protoreflect.FieldDescriptor
	label     string
	typeLabel string
	presence  bool
	action    editAction
	adding    bool
	oneof     protoreflect.OneofDescriptor
}

type choiceKind int

const (
	choiceValue choiceKind = iota
	choiceUnset
	choiceMember
)

type choice struct {
	label  string
	kind   choiceKind
	value  protoreflect.Value
	member schema.Field
}

type Model struct {
	rpc     usecase.RPCSummary
	builder *request.DynamicMessageBuilder

	stack []EditorFrame
	mode  EditorMode

	input  textinput.Model
	target editTarget

	choices      []choice
	choiceCursor int

	err  string
	keys KeyMap

	width  int
	height int
}

func New(rpc usecase.RPCSummary, input schema.Message, builder *request.DynamicMessageBuilder) Model {
	ti := textinput.New()
	ti.Prompt = style.Prompt.Render("> ")
	return Model{
		rpc:     rpc,
		builder: builder,
		stack:   []EditorFrame{{Kind: FrameMessage, Message: input, Title: input.Name()}},
		input:   ti,
		keys:    DefaultKeyMap(),
		width:   80,
		height:  24,
	}
}

func (m Model) RPC() usecase.RPCSummary {
	return m.rpc
}

func (m Model) Builder() *request.DynamicMessageBuilder {
	return m.builder
}

func (m Model) Depth() int {
	return len(m.stack) - 1
}

func (m Model) Cursor() int {
	return m.frame().Cursor
}

func (m Model) Mode() EditorMode {
	return m.mode
}

func (m Model) FrameKind() FrameKind {
	return m.frame().Kind
}

func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.input.Width = w - 4
}

func (m Model) frame() EditorFrame {
	return m.stack[len(m.stack)-1]
}

func (m *Model) setFrame(f EditorFrame) {
	m.stack[len(m.stack)-1] = f
}

func (m *Model) push(f EditorFrame) {
	m.stack = append(m.stack, f)
}

func (m Model) rows() []row {
	f := m.frame()
	if f.Kind == FrameMessage {
		return messageRows(f.Message, f.Path)
	}
	return containerRows(m.builder, f.Field, f.Path)
}

func (m Model) currentRow() (row, bool) {
	rows := m.rows()
	c := m.frame().Cursor
	if c < 0 || c >= len(rows) {
		return row{}, false
	}
	return rows[c], true
}

func (m *Model) clampCursor() {
	f := m.frame()
	n := len(m.rows())
	f.Cursor = max(min(f.Cursor, n-1), 0)
	m.setFrame(f)
	m.scroll()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		if m.mode == ModeText {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.mode == ModeText {
		return m.updateText(k)
	}
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			var cmd tea.Cmd
			m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt})
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	if m.mode == ModeChoice {
		return m.updateChoice(k)
	}
	return m.updateBrowse(k)
}

func (m Model) updateBrowse(k tea.KeyMsg) (Model, tea.Cmd) {
	m.err = ""
	f := m.frame()
	n := len(m.rows())

	switch {
	case key.Matches(k, m.keys.Up):
		f.Cursor = max(f.Cursor-1, 0)
	case key.Matches(k, m.keys.Down):
		f.Cursor = max(min(f.Cursor+1, n-1), 0)
	case key.Matches(k, m.keys.Top):
		f.Cursor = 0
	case key.Matches(k, m.keys.Bottom):
		f.Cursor = max(n-1, 0)
	case key.Matches(k, m.keys.PageUp):
		f.Cursor = max(f.Cursor-m.visibleRows(), 0)
	case key.Matches(k, m.keys.PageDown):
		f.Cursor = max(min(f.Cursor+m.visibleRows(), n-1), 0)
	case key.Matches(k, m.keys.Send):
		return m, func() tea.Msg { return PreviewMsg{} }
	case key.Matches(k, m.keys.Back):
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
			m.clampCursor()
			return m, nil
		}
		if s := k.String(); s == "esc" || s == "ctrl+g" {
			return m, func() tea.Msg { return CancelMsg{} }
		}
		return m, nil
	case key.Matches(k, m.keys.MoveUp), key.Matches(k, m.keys.MoveDown):
		return m.moveElement(key.Matches(k, m.keys.MoveUp)), nil
	case key.Matches(k, m.keys.Clear):
		if r, ok := m.currentRow(); ok {
			m.clearRow(r)
		}
		return m, nil
	case key.Matches(k, m.keys.Add):
		return m.add()
	case key.Matches(k, m.keys.EditJSON):
		return m.editJSON()
	case key.Matches(k, m.keys.Edit):
		if r, ok := m.currentRow(); ok {
			return m.editRow(r)
		}
		return m, nil
	}
	m.setFrame(f)
	m.scroll()
	return m, nil
}

func (m *Model) clearRow(r row) {
	switch r.kind {
	case rowAdd:
		return
	case rowOneof:
		if active := m.builder.WhichOneof(r.path, r.oneof); active != nil {
			m.setError(m.builder.Clear(r.path.Append(string(active.Name()))))
		}
	default:
		m.setError(m.builder.Clear(r.path))
	}
	m.clampCursor()
}

func (m Model) moveElement(up bool) Model {
	r, ok := m.currentRow()
	if !ok || r.kind != rowElement {
		return m
	}
	to := r.index + 1
	if up {
		to = r.index - 1
	}
	if to < 0 || to >= m.builder.Len(r.path) {
		return m
	}
	if err := m.builder.Move(r.path, r.index, to); err != nil {
		m.setError(err)
		return m
	}
	f := m.frame()
	f.Cursor = to
	m.setFrame(f)
	m.scroll()
	return m
}

func (m Model) editRow(r row) (Model, tea.Cmd) {
	switch r.kind {
	case rowOneof:
		return m.startOneofChoice(r), nil
	case rowElement:
		return m.editValue(editTarget{
			path:      r.path,
			fd:        r.field.Descriptor(),
			label:     r.path.Last().String(),
			typeLabel: shortType(r.field.Descriptor()),
		})
	case rowEntry:
		value := r.field.Descriptor().MapValue()
		return m.editValue(editTarget{
			path:      r.path,
			fd:        value,
			label:     r.path.Last().String(),
			typeLabel: shortType(value),
		})
	case rowAdd:
		return m.startAdd(r.field, r.path)
	default:
		return m.editField(r.field, r.path)
	}
}

func (m Model) editField(f schema.Field, path request.FieldPath) (Model, tea.Cmd) {
	fd := f.Descriptor()
	switch {
	case f.IsList():
		m.push(EditorFrame{Kind: FrameList, Field: f, Path: path, Title: fmt.Sprintf("%s (%s)", f.Name(), typeLabel(f))})
		return m, nil
	case f.IsMap():
		m.push(EditorFrame{Kind: FrameMap, Field: f, Path: path, Title: fmt.Sprintf("%s (%s)", f.Name(), typeLabel(f))})
		return m, nil
	case fd.Message() != nil && isWellKnown(fd.Message()):
		return m.startText(editTarget{path: path, fd: fd, label: f.Name(), typeLabel: typeLabel(f), action: actJSON})
	default:
		return m.editValue(editTarget{path: path, fd: fd, label: f.Name(), typeLabel: typeLabel(f), presence: f.HasPresence()})
	}
}

func (m Model) editValue(t editTarget) (Model, tea.Cmd) {
	fd := t.fd
	switch {
	case fd.Message() != nil:
		md := schema.NewMessage(fd.Message())
		m.push(EditorFrame{Kind: FrameMessage, Message: md, Path: t.path, Title: fmt.Sprintf("%s (%s)", t.path.Last(), md.Name())})
		return m, nil
	case fd.Kind() == protoreflect.BoolKind:
		m.choices = []choice{
			{label: "true", value: protoreflect.ValueOfBool(true)},
			{label: "false", value: protoreflect.ValueOfBool(false)},
		}
		return m.startChoice(t), nil
	case fd.Kind() == protoreflect.EnumKind:
		m.choices = nil
		values := fd.Enum().Values()
		for i := 0; i < values.Len(); i++ {
			vd := values.Get(i)
			m.choices = append(m.choices, choice{label: string(vd.Name()), value: protoreflect.ValueOfEnum(vd.Number())})
		}
		return m.startChoice(t), nil
	default:
		return m.startText(t)
	}
}

func (m Model) add() (Model, tea.Cmd) {
	f := m.frame()
	if f.Kind != FrameMessage {
		return m.startAdd(f.Field, f.Path)
	}
	r, ok := m.currentRow()
	if !ok || r.kind != rowField || (!r.field.IsList() && !r.field.IsMap()) {
		return m, nil
	}
	m, _ = m.editField(r.field, r.path)
	return m.startAdd(r.field, r.path)
}

func (m Model) startAdd(container schema.Field, path request.FieldPath) (Model, tea.Cmd) {
	fd := container.Descriptor()
	if fd.IsMap() {
		return m.startText(editTarget{
			path:      path,
			fd:        fd.MapKey(),
			container: fd,
			label:     container.Name() + " key",
			typeLabel: shortType(fd.MapKey()),
			action:    actMapKey,
		})
	}
	if fd.Message() != nil {
		i, err := m.builder.AppendMessage(path)
		if err != nil {
			m.setError(err)
			return m, nil
		}
		m.focusAddRow()
		return m.editValue(editTarget{path: path.AtIndex(i), fd: fd})
	}
	return m.editValue(editTarget{
		path:      path,
		fd:        fd,
		label:     container.Name() + " (new)",
		typeLabel: shortType(fd),
		action:    actAppend,
	})
}

func (m *Model) focusAddRow() {
	f := m.frame()
	if f.Kind == FrameMessage {
		return
	}
	f.Cursor = len(m.rows()) - 1
	m.setFrame(f)
	m.scroll()
}

func (m Model) editJSON() (Model, tea.Cmd) {
	f := m.frame()
	if f.Kind != FrameMessage {
		return m.startText(editTarget{path: f.Path, fd: f.Field.Descriptor(), label: f.Field.Name(), typeLabel: typeLabel(f.Field), action: actJSON})
	}
	r, ok := m.currentRow()
	if !ok {
		return m, nil
	}
	switch r.kind {
	case rowField:
		return m.startText(editTarget{path: r.path, fd: r.field.Descriptor(), label: r.field.Name(), typeLabel: typeLabel(r.field), action: actJSON})
	case rowOneof:
		if active := m.builder.WhichOneof(r.path, r.oneof); active != nil {
			for _, mem := range r.members {
				if mem.Descriptor() == active {
					return m.startText(editTarget{path: r.path.Append(mem.Name()), fd: active, label: mem.Name(), typeLabel: typeLabel(mem), action: actJSON})
				}
			}
		}
	}
	return m, nil
}

func (m Model) startOneofChoice(r row) Model {
	m.choices = nil
	active := m.builder.WhichOneof(r.path, r.oneof)
	m.choiceCursor = 0
	for i, mem := range r.members {
		m.choices = append(m.choices, choice{label: mem.Name() + "  " + style.Subtle.Render(typeLabel(mem)), kind: choiceMember, member: mem})
		if mem.Descriptor() == active {
			m.choiceCursor = i
		}
	}
	m.choices = append(m.choices, choice{label: "<unset>", kind: choiceUnset})
	m.target = editTarget{path: r.path, label: string(r.oneof.Name()), typeLabel: "oneof", oneof: r.oneof}
	m.mode = ModeChoice
	return m
}

func (m Model) startChoice(t editTarget) Model {
	if t.presence {
		m.choices = append(m.choices, choice{label: "<unset>", kind: choiceUnset})
	}
	m.target = t
	m.mode = ModeChoice
	m.choiceCursor = 0
	if t.action == actAppend {
		return m
	}
	if v, has := m.builder.Get(t.path); has {
		for i, c := range m.choices {
			if c.kind == choiceValue && c.value.Equal(v) {
				m.choiceCursor = i
			}
		}
	} else if t.presence {
		m.choiceCursor = len(m.choices) - 1
	}
	return m
}

func (m Model) startText(t editTarget) (Model, tea.Cmd) {
	m.target = t
	m.mode = ModeText

	value := ""
	switch t.action {
	case actJSON:
		value, _ = m.builder.FieldJSON(t.path)
	case actSet:
		if v, has := m.builder.Get(t.path); has {
			value = request.FormatScalar(t.fd, v)
		}
	}
	m.input.SetValue(value)
	m.input.CursorEnd()
	switch t.action {
	case actJSON:
		m.input.Placeholder = "JSON value (empty to unset)"
	case actMapKey:
		m.input.Placeholder = "key (" + t.typeLabel + ")"
	default:
		m.input.Placeholder = t.typeLabel
	}
	return m, m.input.Focus()
}

func (m Model) updateText(k tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.Commit):
		return m.commitText(m.input.Value())
	case key.Matches(k, m.keys.Cancel):
		m.err = ""
		m.mode = ModeBrowse
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m Model) commitText(value string) (Model, tea.Cmd) {
	t := m.target
	var err error
	switch t.action {
	case actJSON:
		if strings.TrimSpace(value) == "" {
			err = m.builder.Clear(t.path)
		} else {
			err = m.builder.SetJSON(t.path, value)
		}
	case actMapKey:
		if _, err = request.ParseMapKey(t.container, value); err == nil {
			m.mode = ModeBrowse
			m.input.Blur()
			return m.startMapValue(t, value)
		}
	case actAppend:
		var v request.Value
		if v, err = request.ParseScalar(t.fd, value); err == nil {
			_, err = m.builder.Append(t.path, v)
		}
	default:
		var v request.Value
		if v, err = request.ParseScalar(t.fd, value); err == nil {
			err = m.builder.Set(t.path, v)
		}
	}
	if err != nil {
		m.setError(err)
		return m, nil
	}
	m.err = ""
	m.mode = ModeBrowse
	m.input.Blur()
	if t.adding || t.action == actAppend {
		m.focusAddRow()
	}
	return m, nil
}

func (m Model) startMapValue(keyTarget editTarget, k string) (Model, tea.Cmd) {
	value := keyTarget.container.MapValue()
	path := keyTarget.path.AtKey(k)
	if value.Message() != nil {
		if err := m.builder.EnsureMapMessage(path); err != nil {
			m.setError(err)
			return m, nil
		}
	}
	return m.editValue(editTarget{path: path, fd: value, label: path.Last().String(), typeLabel: shortType(value), adding: true})
}

func (m Model) updateChoice(k tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.Up):
		m.choiceCursor = max(m.choiceCursor-1, 0)
	case key.Matches(k, m.keys.Down):
		m.choiceCursor = min(m.choiceCursor+1, len(m.choices)-1)
	case key.Matches(k, m.keys.Commit):
		c := m.choices[m.choiceCursor]
		t := m.target
		m.mode = ModeBrowse
		switch c.kind {
		case choiceMember:
			return m.editField(c.member, t.path.Append(c.member.Name()))
		case choiceUnset:
			if t.oneof != nil {
				m.clearOneof(t.path, t.oneof)
			} else {
				m.setError(m.builder.Clear(t.path))
			}
		default:
			if t.action == actAppend {
				_, err := m.builder.Append(t.path, request.ValueOf(c.value))
				m.setError(err)
			} else {
				m.setError(m.builder.Set(t.path, request.ValueOf(c.value)))
			}
			if t.adding || t.action == actAppend {
				m.focusAddRow()
			}
		}
	case key.Matches(k, m.keys.Cancel):
		m.mode = ModeBrowse
	}
	return m, nil
}

func (m *Model) clearOneof(path request.FieldPath, oneof protoreflect.OneofDescriptor) {
	if active := m.builder.WhichOneof(path, oneof); active != nil {
		m.setError(m.builder.Clear(path.Append(string(active.Name()))))
	}
}

func (m *Model) setError(err error) {
	if err == nil {
		m.err = ""
		return
	}
	var invalid *request.InvalidValueError
	if errors.As(err, &invalid) {
		m.err = invalid.Err.Error()
		return
	}
	m.err = err.Error()
}

func (m Model) visibleRows() int {
	reserved := 7
	switch m.mode {
	case ModeText:
		reserved += 3
	case ModeChoice:
		reserved += len(m.choices) + 2
	}
	return max(m.height-reserved, 3)
}

func (m *Model) scroll() {
	f := m.frame()
	rows := m.visibleRows()
	if f.Cursor < f.Offset {
		f.Offset = f.Cursor
	}
	if f.Cursor >= f.Offset+rows {
		f.Offset = f.Cursor - rows + 1
	}
	m.setFrame(f)
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(style.Title.Render(m.rpc.Name))
	for _, fr := range m.stack {
		b.WriteString(style.Subtle.Render("  ›  ") + fr.Title)
	}
	b.WriteString("\n\n")

	rows := m.rows()
	if len(rows) == 0 {
		b.WriteString(style.Subtle.Render("  (no fields)") + "\n")
	}
	models := make([]FieldModel, len(rows))
	nameW, typeW := 0, 0
	for i, r := range rows {
		models[i] = buildRowModel(m.builder, r)
		if !models[i].Action {
			nameW = max(nameW, len(models[i].Name))
			typeW = max(typeW, len(models[i].Type))
		}
	}

	fr := m.frame()
	end := min(fr.Offset+m.visibleRows(), len(models))
	if fr.Offset > 0 {
		b.WriteString(style.Subtle.Render("  ↑ more") + "\n")
	}
	for i := fr.Offset; i < end; i++ {
		b.WriteString(m.renderRow(models[i], i == fr.Cursor, nameW, typeW) + "\n")
	}
	if end < len(models) {
		b.WriteString(style.Subtle.Render("  ↓ more") + "\n")
	}

	switch m.mode {
	case ModeText:
		kind := m.target.typeLabel
		if m.target.action == actJSON {
			kind += ", json"
		}
		fmt.Fprintf(&b, "\n%s %s\n%s\n", style.Selected.Render(m.target.label), style.Type.Render("["+kind+"]"), m.input.View())
	case ModeChoice:
		fmt.Fprintf(&b, "\n%s %s\n", style.Selected.Render(m.target.label), style.Type.Render("["+m.target.typeLabel+"]"))
		for i, c := range m.choices {
			if i == m.choiceCursor {
				b.WriteString(style.Cursor.Render("❯ ") + style.Selected.Render(c.label) + "\n")
			} else {
				b.WriteString("  " + c.label + "\n")
			}
		}
	}

	if m.err != "" {
		b.WriteString("\n" + style.Error.Render("✘ "+m.err) + "\n")
	}
	b.WriteString("\n" + style.Help.Render(m.help()))
	return b.String()
}

func (m Model) renderRow(fm FieldModel, selected bool, nameW, typeW int) string {
	cursor := "  "
	if selected {
		cursor = style.Cursor.Render("❯ ")
	}
	if fm.Action {
		label := style.Subtle.Render(fm.Name)
		if selected {
			label = style.Selected.Render(fm.Name)
		}
		return cursor + label
	}
	name := fmt.Sprintf("%-*s", nameW, fm.Name)
	if selected {
		name = style.Selected.Render(name)
	}
	display := style.Value.Render(fm.Display)
	switch {
	case fm.Unset:
		display = style.Unset.Render(fm.Display)
	case fm.Default:
		display = style.Default.Render(fm.Display)
	}
	return fmt.Sprintf("%s%s  %s  %s", cursor, name, style.Type.Render(fmt.Sprintf("%-*s", typeW, fm.Type)), display)
}

func (m Model) help() string {
	switch m.mode {
	case ModeText:
		return helpLine(m.keys.Commit, m.keys.Cancel)
	case ModeChoice:
		return helpLine(m.keys.Up, m.keys.Down, m.keys.Commit, m.keys.Cancel)
	}
	switch m.frame().Kind {
	case FrameList:
		return helpLine(m.keys.Up, m.keys.Down, m.keys.Edit, m.keys.Add, m.keys.Clear, m.keys.MoveUp, m.keys.MoveDown, m.keys.EditJSON, m.keys.Back)
	case FrameMap:
		return helpLine(m.keys.Up, m.keys.Down, m.keys.Edit, m.keys.Add, m.keys.Clear, m.keys.EditJSON, m.keys.Back)
	default:
		return helpLine(m.keys.Up, m.keys.Down, m.keys.Edit, m.keys.Clear, m.keys.EditJSON, m.keys.Send, m.keys.Back)
	}
}
