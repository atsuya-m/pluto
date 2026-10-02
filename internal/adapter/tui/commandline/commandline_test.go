package commandline

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/atsuya-m/pluto/internal/application/usecase"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input string
		want  Command
	}{
		{"", Command{}},
		{"   ", Command{}},
		{"rpcs", Command{Name: "rpcs", Args: []string{}}},
		{"call CreateUser", Command{Name: "call", Args: []string{"CreateUser"}}},
		{"  desc   rpc  CreateUser ", Command{Name: "desc", Args: []string{"rpc", "CreateUser"}}},
		{`call "Create User" 'a b'`, Command{Name: "call", Args: []string{"Create User", "a b"}}},
		{`call ""`, Command{Name: "call", Args: []string{""}}},
		{"CreateUser", Command{Name: "CreateUser", Args: []string{}}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseCommand(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.want.Name || !slices.Equal(got.Args, tt.want.Args) {
				t.Errorf("ParseCommand(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}

	if _, err := ParseCommand(`call "open`); !errors.Is(err, ErrUnterminatedQuote) {
		t.Errorf("unterminated quote error = %v", err)
	}
}

func testRPCs() []usecase.RPCSummary {
	return []usecase.RPCSummary{
		{Name: "CreateUser", FullName: "user.v1.UserService.CreateUser", Service: "user.v1.UserService", RequestType: "user.v1.CreateUserRequest", ResponseType: "user.v1.CreateUserResponse"},
		{Name: "GetUser", FullName: "user.v1.UserService.GetUser", Service: "user.v1.UserService"},
		{Name: "GetUser", FullName: "admin.v1.AdminService.GetUser", Service: "admin.v1.AdminService"},
		{Name: "WatchUsers", FullName: "user.v1.UserService.WatchUsers", Service: "user.v1.UserService", ServerStreaming: true},
	}
}

func texts(s []Suggestion) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		out = append(out, x.Text)
	}
	return out
}

func TestCompleter(t *testing.T) {
	c := NewCompleter(testRPCs())
	tests := []struct {
		input    string
		wantWord string
		want     []string
	}{
		{"", "", nil},
		{"c", "c", []string{"call", "clear", "CreateUser"}},
		{"call ", "", []string{"CreateUser", "UserService.GetUser", "AdminService.GetUser", "WatchUsers"}},
		{"call get", "get", []string{"UserService.GetUser", "AdminService.GetUser"}},
		{"call admin", "admin", []string{"AdminService.GetUser"}},
		{"desc ", "", []string{"rpc", "message", "CreateUser", "UserService.GetUser", "AdminService.GetUser", "WatchUsers"}},
		{"desc rpc Cre", "Cre", []string{"CreateUser"}},
		{"rpcs ", "", []string{"AdminService", "UserService"}},
		{"user", "user", []string{"CreateUser", "UserService.GetUser", "WatchUsers", "AdminService.GetUser"}},
		{"call CreateUser ", "", nil},
		{"services ", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			word, got := c.Complete(tt.input)
			if word != tt.wantWord || !slices.Equal(texts(got), tt.want) {
				t.Errorf("Complete(%q) = %q %v, want %q %v", tt.input, word, texts(got), tt.wantWord, tt.want)
			}
		})
	}
}

func TestCompleter_Headers(t *testing.T) {
	c := NewCompleter(testRPCs()).WithHeaderKeys([]string{"Authorization", "X-Trace"})
	tests := []struct {
		input string
		want  []string
	}{
		{"header ", []string{"set", "add", "rm", "clear"}},
		{"header rm ", []string{"Authorization", "X-Trace"}},
		{"header set x", []string{"X-Trace"}},
		{"load ", nil},
		{"view ", []string{"headers"}},
		{"he", []string{"header", "help"}},
	}
	for _, tt := range tests {
		if _, got := c.Complete(tt.input); !slices.Equal(texts(got), tt.want) {
			t.Errorf("Complete(%q) = %v, want %v", tt.input, texts(got), tt.want)
		}
	}
}

func TestCompleter_Descriptions(t *testing.T) {
	_, got := NewCompleter(testRPCs()).Complete("call Watch")
	if len(got) != 1 || !strings.HasSuffix(got[0].Description, "[server stream]") {
		t.Errorf("suggestions = %+v", got)
	}
	_, got = NewCompleter(testRPCs()).Complete("call Create")
	if got[0].Description != "UserService · CreateUserRequest → CreateUserResponse" {
		t.Errorf("description = %q", got[0].Description)
	}
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func press(m Model, k tea.KeyType) (Model, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: k})
}

func TestModel_TabCyclesSuggestions(t *testing.T) {
	m := New()
	m.SetRPCs(testRPCs())
	m = typeText(m, "call get")

	m, _ = press(m, tea.KeyTab)
	if m.Value() != "call UserService.GetUser" || !m.Selecting() {
		t.Fatalf("after tab: %q selecting=%v", m.Value(), m.Selecting())
	}
	m, _ = press(m, tea.KeyTab)
	if m.Value() != "call AdminService.GetUser" {
		t.Errorf("after 2nd tab: %q", m.Value())
	}
	m, _ = press(m, tea.KeyTab)
	if m.Value() != "call UserService.GetUser" {
		t.Errorf("tab should wrap around: %q", m.Value())
	}
	m, _ = press(m, tea.KeyShiftTab)
	if m.Value() != "call AdminService.GetUser" {
		t.Errorf("after shift+tab: %q", m.Value())
	}
	m, _ = press(m, tea.KeyEsc)
	if m.Value() != "call get" || m.Selecting() {
		t.Errorf("esc should restore the typed text: %q selecting=%v", m.Value(), m.Selecting())
	}
}

func TestModel_TabWithSingleCandidateAccepts(t *testing.T) {
	m := New()
	m.SetRPCs(testRPCs())
	m = typeText(m, "call Cre")

	m, _ = press(m, tea.KeyTab)
	if m.Value() != "call CreateUser " || m.Selecting() {
		t.Errorf("value = %q selecting=%v", m.Value(), m.Selecting())
	}
}

func TestModel_ArrowKeysMoveSelectionOnlyWhileSelecting(t *testing.T) {
	m := New()
	m.SetRPCs(testRPCs())
	m = typeText(m, "call get")
	m, _ = press(m, tea.KeyTab)
	m, _ = press(m, tea.KeyCtrlN)
	if m.Value() != "call AdminService.GetUser" {
		t.Errorf("ctrl+n while selecting: %q", m.Value())
	}
	m, _ = press(m, tea.KeyUp)
	if m.Value() != "call UserService.GetUser" {
		t.Errorf("up while selecting: %q", m.Value())
	}
}

func TestModel_SubmitAndHistory(t *testing.T) {
	m := New()
	m = typeText(m, "rpcs")
	m, cmd := press(m, tea.KeyEnter)
	if msg, ok := cmd().(SubmitMsg); !ok || msg.Input != "rpcs" {
		t.Fatalf("submit msg = %#v", cmd())
	}
	if m.Value() != "" {
		t.Errorf("input should be cleared after submit: %q", m.Value())
	}
	m = typeText(m, "services")
	m, _ = press(m, tea.KeyEnter)

	m, _ = press(m, tea.KeyCtrlP)
	if m.Value() != "services" {
		t.Errorf("ctrl+p: %q", m.Value())
	}
	m, _ = press(m, tea.KeyUp)
	if m.Value() != "rpcs" {
		t.Errorf("up: %q", m.Value())
	}
	m, _ = press(m, tea.KeyUp)
	if m.Value() != "rpcs" {
		t.Errorf("up at oldest entry should stay: %q", m.Value())
	}
	m, _ = press(m, tea.KeyCtrlN)
	if m.Value() != "services" {
		t.Errorf("ctrl+n: %q", m.Value())
	}
	m, _ = press(m, tea.KeyDown)
	if m.Value() != "" {
		t.Errorf("down past newest should clear: %q", m.Value())
	}
}

func TestModel_EmacsEditing(t *testing.T) {
	m := New()
	m = typeText(m, "call Foo")
	m, _ = press(m, tea.KeyCtrlA)
	m = typeText(m, "x")
	m, _ = press(m, tea.KeyCtrlE)
	m = typeText(m, "y")
	if m.Value() != "xcall Fooy" {
		t.Errorf("ctrl+a/ctrl+e: %q", m.Value())
	}
	m, _ = press(m, tea.KeyCtrlW)
	if m.Value() != "xcall " {
		t.Errorf("ctrl+w: %q", m.Value())
	}
	m, _ = press(m, tea.KeyCtrlU)
	if m.Value() != "" {
		t.Errorf("ctrl+u: %q", m.Value())
	}
}

func TestModel_ViewShowsPopupWhileTyping(t *testing.T) {
	m := New()
	m.SetRPCs(testRPCs())
	m = typeText(m, "call Cre")
	if v := m.View(); !strings.Contains(v, "CreateUser") || !strings.Contains(v, "CreateUserRequest → CreateUserResponse") {
		t.Errorf("view does not show the suggestion popup:\n%s", v)
	}
	m, _ = press(m, tea.KeyEsc)
	if v := m.View(); strings.Contains(v, "CreateUserRequest") {
		t.Errorf("popup should be dismissed:\n%s", v)
	}
}
