package rpcselector

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/atsuya-m/pluto/internal/application/usecase"
)

type SelectedMsg struct {
	RPC usecase.RPCSummary
}

type BackMsg struct{}

type item struct {
	rpc usecase.RPCSummary
}

func (i item) Title() string {
	return shortService(i.rpc.Service) + "." + i.rpc.Name
}

func (i item) Description() string {
	desc := i.rpc.RequestType + " → " + i.rpc.ResponseType
	switch {
	case i.rpc.ClientStreaming && i.rpc.ServerStreaming:
		desc += "  [bidi stream]"
	case i.rpc.ClientStreaming:
		desc += "  [client stream]"
	case i.rpc.ServerStreaming:
		desc += "  [server stream]"
	}
	return desc
}

func (i item) FilterValue() string {
	return i.rpc.FullName
}

func shortService(full string) string {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '.' {
			return full[i+1:]
		}
	}
	return full
}

type KeyMap struct {
	Select key.Binding
	Back   key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Back:   key.NewBinding(key.WithKeys("esc", "ctrl+g"), key.WithHelp("esc/C-g", "back")),
	}
}

type Model struct {
	list list.Model
	keys KeyMap
}

func New() Model {
	d := list.NewDefaultDelegate()
	l := list.New(nil, d, 80, 20)
	l.Title = "Select RPC"
	l.SetShowStatusBar(true)
	l.DisableQuitKeybindings()
	addEmacsKeys(&l.KeyMap)
	keys := DefaultKeyMap()
	l.AdditionalShortHelpKeys = func() []key.Binding { return []key.Binding{keys.Select, keys.Back} }
	return Model{list: l, keys: keys}
}

func (m *Model) SetRPCs(rpcs []usecase.RPCSummary) {
	items := make([]list.Item, 0, len(rpcs))
	for _, r := range rpcs {
		items = append(items, item{rpc: r})
	}
	m.list.SetItems(items)
	m.list.Title = fmt.Sprintf("Select RPC (%d)", len(rpcs))
}

func (m *Model) SetSize(w, h int) {
	m.list.SetSize(w, h)
}

func (m *Model) ResetFilter() {
	m.list.ResetFilter()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && m.list.FilterState() != list.Filtering && k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			var cmd tea.Cmd
			m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt})
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	if k, ok := msg.(tea.KeyMsg); ok && m.list.FilterState() != list.Filtering {
		switch {
		case key.Matches(k, m.keys.Select):
			if it, ok := m.list.SelectedItem().(item); ok {
				return m, func() tea.Msg { return SelectedMsg{RPC: it.rpc} }
			}
			return m, nil
		case key.Matches(k, m.keys.Back) && m.list.FilterState() == list.Unfiltered:
			return m, func() tea.Msg { return BackMsg{} }
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return m.list.View()
}

func addEmacsKeys(km *list.KeyMap) {
	extend := func(b *key.Binding, keys ...string) {
		b.SetKeys(append(b.Keys(), keys...)...)
	}
	extend(&km.CursorUp, "ctrl+p")
	extend(&km.CursorDown, "ctrl+n")
	extend(&km.PrevPage, "alt+v")
	extend(&km.NextPage, "ctrl+v")
	extend(&km.GoToStart, "alt+<")
	extend(&km.GoToEnd, "alt+>")
	extend(&km.CancelWhileFiltering, "ctrl+g")
	extend(&km.ClearFilter, "ctrl+g")
}
