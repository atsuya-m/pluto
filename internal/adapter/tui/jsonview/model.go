package jsonview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/atsuya-m/pluto/internal/adapter/tui/style"
)

type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Top         key.Binding
	Bottom      key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Toggle      key.Binding
	Expand      key.Binding
	Collapse    key.Binding
	ExpandAll   key.Binding
	CollapseAll key.Binding
	Search      key.Binding
	Next        key.Binding
	Prev        key.Binding
	Commit      key.Binding
	Cancel      key.Binding
	CopyValue   key.Binding
	CopyPath    key.Binding
}

type CopyMsg struct {
	Text  string
	Label string
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("↓/j", "down")),
		Top:         key.NewBinding(key.WithKeys("home", "g", "alt+<"), key.WithHelp("g", "top")),
		Bottom:      key.NewBinding(key.WithKeys("end", "G", "alt+>"), key.WithHelp("G", "bottom")),
		PageUp:      key.NewBinding(key.WithKeys("pgup", "alt+v", "ctrl+u"), key.WithHelp("M-v", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("pgdown", "ctrl+v", "ctrl+d"), key.WithHelp("C-v", "page down")),
		Toggle:      key.NewBinding(key.WithKeys("enter", " ", "tab"), key.WithHelp("enter", "fold")),
		Expand:      key.NewBinding(key.WithKeys("right", "l", "ctrl+f"), key.WithHelp("l", "open")),
		Collapse:    key.NewBinding(key.WithKeys("left", "h", "ctrl+b"), key.WithHelp("h", "close/parent")),
		ExpandAll:   key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "open all")),
		CollapseAll: key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "close all")),
		Search:      key.NewBinding(key.WithKeys("/", "ctrl+s"), key.WithHelp("/", "search")),
		Next:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next/prev match")),
		Prev:        key.NewBinding(key.WithKeys("N")),
		Commit:      key.NewBinding(key.WithKeys("enter")),
		Cancel:      key.NewBinding(key.WithKeys("esc", "ctrl+g")),
		CopyValue:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy value")),
		CopyPath:    key.NewBinding(key.WithKeys("Y"), key.WithHelp("Y", "copy path")),
	}
}

type Model struct {
	root   *Node
	lines  []*Node
	cursor int
	offset int
	width  int
	height int
	keys   KeyMap

	searching bool
	input     textinput.Model
	query     string
	matches   []*Node
	match     int
}

func New(data []byte, width, height int) (Model, error) {
	root, err := Parse(data)
	if err != nil {
		return Model{}, err
	}
	ti := textinput.New()
	ti.Prompt = style.Prompt.Render("/")
	m := Model{root: root, keys: DefaultKeyMap(), input: ti, width: width, height: height}
	AutoExpand(root, m.rows())
	m.refresh()
	return m, nil
}

func (m Model) Searching() bool {
	return m.searching
}

func (m Model) Cursor() int {
	return m.cursor
}

func (m Model) Lines() int {
	return len(m.lines)
}

func (m Model) Current() *Node {
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		return nil
	}
	return m.lines[m.cursor]
}

func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.scroll()
}

func (m Model) rows() int {
	return max(m.height-3, 3)
}

func (m *Model) refresh() {
	current := m.Current()
	m.lines = Visible(m.root)
	if current != nil {
		m.focus(current)
	}
	m.cursor = max(min(m.cursor, len(m.lines)-1), 0)
	m.scroll()
}

func (m *Model) focus(n *Node) {
	for i, l := range m.lines {
		if l == n {
			m.cursor = i
			return
		}
	}
	for p := n.Parent; p != nil; p = p.Parent {
		for i, l := range m.lines {
			if l == p {
				m.cursor = i
				return
			}
		}
	}
}

func (m *Model) scroll() {
	rows := m.rows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(min(m.offset, len(m.lines)-rows), 0)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		if m.searching {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.searching {
		return m.updateSearch(k)
	}
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		for _, r := range k.Runes {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt})
		}
		return m, nil
	}

	cur := m.Current()
	switch {
	case key.Matches(k, m.keys.Up):
		m.cursor = max(m.cursor-1, 0)
	case key.Matches(k, m.keys.Down):
		m.cursor = min(m.cursor+1, len(m.lines)-1)
	case key.Matches(k, m.keys.Top):
		m.cursor = 0
	case key.Matches(k, m.keys.Bottom):
		m.cursor = len(m.lines) - 1
	case key.Matches(k, m.keys.PageUp):
		m.cursor = max(m.cursor-m.rows(), 0)
	case key.Matches(k, m.keys.PageDown):
		m.cursor = min(m.cursor+m.rows(), len(m.lines)-1)
	case key.Matches(k, m.keys.Toggle):
		if cur != nil && cur.IsContainer() {
			cur.Expanded = !cur.Expanded
			m.refresh()
		}
	case key.Matches(k, m.keys.Expand):
		if cur != nil && cur.IsContainer() {
			if cur.Expanded && len(cur.Children) > 0 {
				m.cursor++
			} else {
				cur.Expanded = true
				m.refresh()
			}
		}
	case key.Matches(k, m.keys.Collapse):
		if cur == nil {
			break
		}
		if cur.IsContainer() && cur.Expanded {
			cur.Expanded = false
			m.refresh()
		} else if cur.Parent != nil && cur.Parent.Parent != nil {
			m.focus(cur.Parent)
		}
	case key.Matches(k, m.keys.ExpandAll):
		target := m.root
		if cur != nil && cur.IsContainer() {
			target = cur
		}
		ExpandAll(target)
		m.refresh()
	case key.Matches(k, m.keys.CollapseAll):
		CollapseAll(m.root)
		if cur != nil {
			top := cur
			for top.Parent != nil && top.Parent.Parent != nil {
				top = top.Parent
			}
			m.lines = Visible(m.root)
			m.focus(top)
		}
		m.refresh()
	case key.Matches(k, m.keys.Search):
		m.searching = true
		m.input.SetValue(m.query)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case key.Matches(k, m.keys.CopyValue):
		if cur != nil {
			msg := CopyMsg{Text: cur.CopyText(), Label: cur.Path()}
			return m, func() tea.Msg { return msg }
		}
	case key.Matches(k, m.keys.CopyPath):
		if cur != nil {
			msg := CopyMsg{Text: cur.Path(), Label: "path " + cur.Path()}
			return m, func() tea.Msg { return msg }
		}
	case key.Matches(k, m.keys.Next):
		m.jump(1)
	case key.Matches(k, m.keys.Prev):
		m.jump(-1)
	case k.Type == tea.KeyRunes && len(k.Runes) == 1 && k.Runes[0] >= '1' && k.Runes[0] <= '9':
		ExpandToDepth(m.root, int(k.Runes[0]-'0'))
		m.refresh()
	}
	m.scroll()
	return m, nil
}

func (m Model) updateSearch(k tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.Commit):
		m.searching = false
		m.input.Blur()
		m.query = m.input.Value()
		m.matches = Search(m.root, m.query)
		m.match = -1
		m.jumpFromCursor()
		return m, nil
	case key.Matches(k, m.keys.Cancel):
		m.searching = false
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m *Model) jumpFromCursor() {
	if len(m.matches) == 0 {
		return
	}
	cur := m.Current()
	order := map[*Node]int{}
	i := 0
	Walk(m.root, func(n *Node) { order[n] = i; i++ })
	start := 0
	if cur != nil {
		for j, n := range m.matches {
			if order[n] >= order[cur] {
				start = j
				break
			}
			start = 0
		}
	}
	m.match = start
	m.show(m.matches[m.match])
}

func (m *Model) jump(delta int) {
	if len(m.matches) == 0 {
		return
	}
	m.match = (m.match + delta + len(m.matches)) % len(m.matches)
	m.show(m.matches[m.match])
}

func (m *Model) show(n *Node) {
	Reveal(n)
	m.lines = Visible(m.root)
	m.focus(n)
	m.scroll()
}

var (
	keyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	stringStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	numberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	literalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	summaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	matchStyle    = lipgloss.NewStyle().Underline(true)
)

func (m Model) View() string {
	var b strings.Builder
	end := min(m.offset+m.rows(), len(m.lines))
	matched := map[*Node]bool{}
	for _, n := range m.matches {
		matched[n] = true
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderLine(m.lines[i], i == m.cursor, matched[m.lines[i]]))
		b.WriteString("\n")
	}
	if len(m.lines) == 0 {
		b.WriteString(summaryStyle.Render("  (empty)") + "\n")
	}

	status := fmt.Sprintf("%d/%d", m.cursor+1, len(m.lines))
	if cur := m.Current(); cur != nil {
		status = cur.Path() + "  " + summaryStyle.Render(status)
	}
	if m.query != "" {
		if len(m.matches) == 0 {
			status += summaryStyle.Render(fmt.Sprintf("  /%s: no match", m.query))
		} else {
			status += summaryStyle.Render(fmt.Sprintf("  /%s: %d/%d", m.query, m.match+1, len(m.matches)))
		}
	}
	b.WriteString(status)
	if m.searching {
		b.WriteString("\n" + m.input.View())
	}
	return b.String()
}

func (m Model) renderLine(n *Node, selected, matched bool) string {
	indent := strings.Repeat("  ", n.Depth)
	marker := "  "
	if n.IsContainer() {
		if n.Expanded {
			marker = "▾ "
		} else {
			marker = "▸ "
		}
	}
	label := keyStyle.Render(n.Label())
	if matched {
		label = matchStyle.Render(label)
	}

	var value string
	switch {
	case n.IsContainer() && n.Expanded:
		value = summaryStyle.Render(n.Summary())
	case n.IsContainer():
		value = summaryStyle.Render(n.Summary())
	default:
		value = scalarStyle(n).Render(truncate(n.Value, max(m.width-len(indent)-len(n.Label())-8, 16)))
	}
	line := fmt.Sprintf("%s%s%s: %s", indent, marker, label, value)
	if selected {
		return selectedStyle.Render(style.Cursor.Render("❯") + line)
	}
	return " " + line
}

func scalarStyle(n *Node) lipgloss.Style {
	switch n.Kind {
	case KindString:
		return stringStyle
	case KindNumber:
		return numberStyle
	default:
		return literalStyle
	}
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}

func (m Model) Help() string {
	return strings.Join([]string{"j/k move", "enter fold", "h/l", "L/H all", "/ search", "y/Y copy value/path"}, " • ")
}
