package commandline

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/atsuya-m/pluto/internal/adapter/tui/style"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

const maxVisibleSuggestions = 8

type SubmitMsg struct {
	Input string
}

type KeyMap struct {
	Submit      key.Binding
	Complete    key.Binding
	CompletePrv key.Binding
	Next        key.Binding
	Prev        key.Binding
	Dismiss     key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Submit:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run")),
		Complete:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "complete")),
		CompletePrv: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous")),
		Next:        key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓/ctrl+n", "next")),
		Prev:        key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/ctrl+p", "previous")),
		Dismiss:     key.NewBinding(key.WithKeys("esc", "ctrl+g"), key.WithHelp("esc", "dismiss")),
	}
}

type Model struct {
	input textinput.Model
	keys  KeyMap

	history []string
	cursor  int

	completer   Completer
	headerKeys  []string
	suggestions []Suggestion
	selected    int
	base        string
	baseWord    string
	dismissed   bool
	width       int
}

func New() Model {
	ti := textinput.New()
	ti.Prompt = style.Prompt.Render("pluto> ")
	ti.Focus()
	return Model{input: ti, keys: DefaultKeyMap(), selected: -1, width: 80}
}

func (m *Model) SetRPCs(rpcs []usecase.RPCSummary) {
	m.completer = NewCompleter(rpcs).WithHeaderKeys(m.headerKeys)
	m.refresh()
}

func (m *Model) SetHeaderKeys(keys []string) {
	m.headerKeys = keys
	m.completer = m.completer.WithHeaderKeys(keys)
}

func (m *Model) Focus() tea.Cmd {
	return m.input.Focus()
}

func (m *Model) SetWidth(w int) {
	m.width = w
	m.input.Width = w - 8
}

func (m Model) Value() string {
	return m.input.Value()
}

func (m Model) Selecting() bool {
	return m.selected >= 0
}

func (m *Model) refresh() {
	m.selected = -1
	if m.input.Position() != len([]rune(m.input.Value())) {
		m.suggestions = nil
		return
	}
	_, m.suggestions = m.completer.Complete(m.input.Value())
}

func (m *Model) setValue(v string) {
	m.input.SetValue(v)
	m.input.CursorEnd()
}

func (m *Model) startCycle() {
	m.base = m.input.Value()
	m.baseWord, m.suggestions = m.completer.Complete(m.base)
	m.dismissed = false
}

func (m *Model) apply() {
	s := m.suggestions[m.selected]
	m.setValue(strings.TrimSuffix(m.base, m.baseWord) + s.Text)
}

func (m *Model) move(delta int) {
	if len(m.suggestions) == 0 {
		return
	}
	if m.selected < 0 {
		m.startCycle()
		if len(m.suggestions) == 0 {
			return
		}
		if len(m.suggestions) == 1 {
			m.setValue(strings.TrimSuffix(m.base, m.baseWord) + m.suggestions[0].Text + " ")
			m.refresh()
			return
		}
		if delta > 0 {
			m.selected = 0
		} else {
			m.selected = len(m.suggestions) - 1
		}
	} else {
		m.selected = (m.selected + delta + len(m.suggestions)) % len(m.suggestions)
	}
	m.apply()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(k, m.keys.Submit):
		v := m.input.Value()
		if v != "" && (len(m.history) == 0 || m.history[len(m.history)-1] != v) {
			m.history = append(m.history, v)
		}
		m.cursor = len(m.history)
		m.input.Reset()
		m.dismissed = false
		m.refresh()
		return m, func() tea.Msg { return SubmitMsg{Input: v} }
	case key.Matches(k, m.keys.Complete):
		m.move(1)
		return m, nil
	case key.Matches(k, m.keys.CompletePrv):
		m.move(-1)
		return m, nil
	case key.Matches(k, m.keys.Dismiss):
		if m.selected >= 0 {
			m.setValue(m.base)
		}
		m.selected = -1
		m.dismissed = true
		return m, nil
	case key.Matches(k, m.keys.Next):
		if m.selected >= 0 {
			m.move(1)
		} else {
			m.historyNext()
		}
		return m, nil
	case key.Matches(k, m.keys.Prev):
		if m.selected >= 0 {
			m.move(-1)
		} else {
			m.historyPrev()
		}
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	if m.input.Value() != before {
		m.dismissed = false
	}
	m.refresh()
	return m, cmd
}

func (m *Model) historyPrev() {
	if m.cursor > 0 {
		m.cursor--
		m.setValue(m.history[m.cursor])
		m.dismissed = true
		m.refresh()
	}
}

func (m *Model) historyNext() {
	if m.cursor < len(m.history)-1 {
		m.cursor++
		m.setValue(m.history[m.cursor])
	} else {
		m.cursor = len(m.history)
		m.input.Reset()
	}
	m.dismissed = true
	m.refresh()
}

var (
	suggestionStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236"))
	suggestionSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("13")).Bold(true)
	suggestionDescStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("238"))
	suggestionDescSelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("219"))
)

func (m Model) View() string {
	view := m.input.View()
	if m.dismissed || len(m.suggestions) == 0 {
		return view
	}

	start := 0
	if m.selected >= maxVisibleSuggestions {
		start = m.selected - maxVisibleSuggestions + 1
	}
	end := min(start+maxVisibleSuggestions, len(m.suggestions))

	textW, descW := 0, 0
	for _, s := range m.suggestions[start:end] {
		textW = max(textW, lipgloss.Width(s.Text))
		descW = max(descW, lipgloss.Width(s.Description))
	}
	descW = min(descW, max(m.width-textW-12, 10))

	var b strings.Builder
	b.WriteString(view)
	for i := start; i < end; i++ {
		s := m.suggestions[i]
		text := fmt.Sprintf(" %-*s ", textW, s.Text)
		desc := fmt.Sprintf(" %-*s ", descW, truncate(s.Description, descW))
		if i == m.selected {
			text, desc = suggestionSelectedStyle.Render(text), suggestionDescSelStyle.Render(desc)
		} else {
			text, desc = suggestionStyle.Render(text), suggestionDescStyle.Render(desc)
		}
		b.WriteString("\n       " + text + desc)
	}
	if rest := len(m.suggestions) - end; rest > 0 {
		b.WriteString("\n       " + style.Subtle.Render(fmt.Sprintf("… %d more", rest)))
	}
	return b.String()
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}
