package requesteditor

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Edit     key.Binding
	EditJSON key.Binding
	Add      key.Binding
	MoveUp   key.Binding
	MoveDown key.Binding
	Clear    key.Binding
	Send     key.Binding
	Back     key.Binding
	Commit   key.Binding
	Cancel   key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("↑/k/C-p", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("↓/j/C-n", "down")),
		Top:      key.NewBinding(key.WithKeys("home", "g", "alt+<"), key.WithHelp("g/M-<", "top")),
		Bottom:   key.NewBinding(key.WithKeys("end", "G", "alt+>"), key.WithHelp("G/M->", "bottom")),
		PageUp:   key.NewBinding(key.WithKeys("pgup", "alt+v"), key.WithHelp("M-v", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+v"), key.WithHelp("C-v", "page down")),
		Edit:     key.NewBinding(key.WithKeys("enter", "l", "right", "ctrl+f"), key.WithHelp("enter", "edit/open")),
		EditJSON: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit as json")),
		Add:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		MoveUp:   key.NewBinding(key.WithKeys("K", "alt+up"), key.WithHelp("K", "move up")),
		MoveDown: key.NewBinding(key.WithKeys("J", "alt+down"), key.WithHelp("J", "move down")),
		Clear:    key.NewBinding(key.WithKeys("x", "d", "backspace", "delete", "ctrl+d"), key.WithHelp("x/d", "unset/delete")),
		Send:     key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("C-s", "preview/send")),
		Back:     key.NewBinding(key.WithKeys("esc", "h", "left", "ctrl+b", "ctrl+g"), key.WithHelp("esc/C-g", "back")),
		Commit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "commit")),
		Cancel:   key.NewBinding(key.WithKeys("esc", "ctrl+g"), key.WithHelp("esc/C-g", "cancel")),
	}
}

func helpLine(bindings ...key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return strings.Join(parts, " • ")
}
