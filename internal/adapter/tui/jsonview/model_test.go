package jsonview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func press(m Model, k tea.KeyType) Model {
	m, _ = m.Update(tea.KeyMsg{Type: k})
	return m
}

func typeRunes(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func newModel(t *testing.T, height int) Model {
	t.Helper()
	m, err := New([]byte(sample), 100, height)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestModel_NavigationAndFolding(t *testing.T) {
	m := newModel(t, 40)
	if m.Lines() != Count(m.root) {
		t.Fatalf("small document should open fully: %d lines", m.Lines())
	}

	m = press(m, tea.KeyEnter)
	if m.Lines() != 7 || m.Current().Path() != ".user" {
		t.Errorf("collapse user: lines = %d, at %s", m.Lines(), m.Current().Path())
	}
	m = typeRunes(m, "l")
	m = typeRunes(m, "l")
	if m.Current().Path() != ".user.id" {
		t.Errorf("l twice should enter the object: %s", m.Current().Path())
	}
	m = typeRunes(m, "jl")
	if m.Current().Path() != ".user.profile.bio" {
		t.Errorf("at %s", m.Current().Path())
	}
	m = typeRunes(m, "h")
	if m.Current().Path() != ".user.profile" {
		t.Errorf("h on a scalar should go to the parent: %s", m.Current().Path())
	}
	m = typeRunes(m, "h")
	if m.Current().Expanded {
		t.Error("h on an open container should close it")
	}

	m = typeRunes(m, "H")
	if m.Lines() != 3 || m.Current().Path() != ".user" {
		t.Errorf("H: lines = %d at %s", m.Lines(), m.Current().Path())
	}
	m = typeRunes(m, "L")
	if m.Lines() != 1+len(collectUnder(m.root.Children[0]))+2 {
		t.Errorf("L on user: lines = %d", m.Lines())
	}
	m = typeRunes(m, "1")
	if m.Lines() != 10 {
		t.Errorf("1: lines = %d", m.Lines())
	}

	m = typeRunes(m, "G")
	if m.Current().Path() != `["weird key"]` {
		t.Errorf("G: %s", m.Current().Path())
	}
	m = typeRunes(m, "g")
	if m.Cursor() != 0 {
		t.Errorf("g: %d", m.Cursor())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jjj")})
	if m.Cursor() != 3 {
		t.Errorf("batched runes should move three rows: %d", m.Cursor())
	}
}

func collectUnder(n *Node) []*Node {
	var out []*Node
	for _, c := range n.Children {
		out = append(out, c)
		out = append(out, collectUnder(c)...)
	}
	return out
}

func TestModel_Search(t *testing.T) {
	m := newModel(t, 40)
	m = typeRunes(m, "H")
	m = typeRunes(m, "/")
	if !m.Searching() {
		t.Fatal("/ should start searching")
	}
	m = typeRunes(m, "name")
	m = press(m, tea.KeyEnter)
	if m.Searching() || m.Current().Path() != ".items[0].name" {
		t.Fatalf("first match: %s", m.Current().Path())
	}
	if !strings.Contains(m.View(), "/name: 1/2") {
		t.Errorf("status should show match position:\n%s", m.View())
	}
	m = typeRunes(m, "n")
	if m.Current().Path() != ".items[1].name" {
		t.Errorf("n: %s", m.Current().Path())
	}
	m = typeRunes(m, "n")
	if m.Current().Path() != ".items[0].name" {
		t.Errorf("n should wrap: %s", m.Current().Path())
	}
	m = typeRunes(m, "N")
	if m.Current().Path() != ".items[1].name" {
		t.Errorf("N: %s", m.Current().Path())
	}

	m = typeRunes(m, "/")
	m = press(m, tea.KeyCtrlU)
	m = typeRunes(m, "zzz")
	m = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "no match") {
		t.Errorf("view:\n%s", m.View())
	}
	m = typeRunes(m, "/")
	m = press(m, tea.KeyEsc)
	if m.Searching() {
		t.Error("esc should cancel search")
	}
}

func TestModel_ScrollAndView(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"list":[`)
	for i := range 200 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"i":1}`)
	}
	b.WriteString(`]}`)
	m, err := New([]byte(b.String()), 80, 12)
	if err != nil {
		t.Fatal(err)
	}
	if m.Lines() != 201 || strings.Count(m.View(), "\n") > 12 {
		t.Errorf("the single wrapper should open while the view stays within the height: %d lines", m.Lines())
	}
	m = typeRunes(m, "L")
	m = typeRunes(m, "G")
	view := m.View()
	if strings.Count(view, "\n") > 12 || !strings.Contains(view, ".list[199].i") {
		t.Errorf("view should be limited to the height and show the path:\n%s", view)
	}
	if !strings.Contains(view, "❯") {
		t.Error("cursor marker missing")
	}
}
