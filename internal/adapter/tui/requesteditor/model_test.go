package requesteditor

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/domain/request"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

const (
	idxName = iota
	idxNickname
	idxAge
	idxStatus
	idxTags
	idxProfile
	idxLabels
	idxContact
	idxAdmin
	idxAddresses
	idxScores
	idxPlaces
	idxHistory
)

func newEditor(t *testing.T) Model {
	t.Helper()
	rpc := fixture.RPC(t, "CreateUser")
	b := request.NewDynamicMessageBuilder(rpc.Input().Descriptor())
	m := New(usecase.RPCSummary{Name: rpc.Name(), FullName: rpc.FullName()}, rpc.Input(), b)
	m.SetSize(100, 40)
	return m
}

func press(m Model, k tea.KeyType) (Model, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: k})
}

func runes(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func moveTo(t *testing.T, m Model, idx int) Model {
	t.Helper()
	m, _ = press(m, tea.KeyHome)
	for range idx {
		m, _ = press(m, tea.KeyDown)
	}
	if m.Cursor() != idx {
		t.Fatalf("cursor = %d, want %d", m.Cursor(), idx)
	}
	return m
}

func has(m Model, path ...string) bool {
	return m.Builder().Has(request.Path(path...))
}

func get(m Model, path ...string) string {
	v, _ := m.Builder().Get(request.Path(path...))
	return v.String()
}

func TestRequestEditor_Navigation(t *testing.T) {
	m := newEditor(t)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyUp)
	if m.Cursor() != 1 {
		t.Errorf("down, down, up: cursor = %d, want 1", m.Cursor())
	}

	m = runes(m, "jjk")
	if m.Cursor() != 2 {
		t.Errorf("j j k: cursor = %d, want 2", m.Cursor())
	}

	m, _ = press(m, tea.KeyCtrlN)
	m, _ = press(m, tea.KeyCtrlN)
	m, _ = press(m, tea.KeyCtrlP)
	if m.Cursor() != 3 {
		t.Errorf("ctrl+n, ctrl+n, ctrl+p: cursor = %d, want 3", m.Cursor())
	}

	m, _ = press(m, tea.KeyEnd)
	if m.Cursor() != idxHistory {
		t.Errorf("end: cursor = %d", m.Cursor())
	}
	m, _ = press(m, tea.KeyDown)
	if m.Cursor() != idxHistory {
		t.Errorf("down at bottom should stay: cursor = %d", m.Cursor())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'<'}, Alt: true})
	if m.Cursor() != 0 {
		t.Errorf("alt+<: cursor = %d", m.Cursor())
	}
	m, _ = press(m, tea.KeyUp)
	if m.Cursor() != 0 {
		t.Errorf("up at top should stay: cursor = %d", m.Cursor())
	}
}

func TestRequestEditor_BatchedRunesAreSplit(t *testing.T) {
	m := newEditor(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jjj")})
	if m.Cursor() != 3 {
		t.Errorf("batched jjj should move three rows, cursor = %d", m.Cursor())
	}
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyEsc)
	m = moveTo(t, m, idxName)
	m, _ = press(m, tea.KeyEnter)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jjj")})
	m, _ = press(m, tea.KeyEnter)
	if get(m, "name") != "jjj" {
		t.Errorf("batched runes in text mode should be typed as-is, name = %q", get(m, "name"))
	}
}

func TestRequestEditor_UnsetOptionalField(t *testing.T) {
	m := moveTo(t, newEditor(t), idxNickname)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "Taro")
	m, _ = press(m, tea.KeyEnter)
	if get(m, "nickname") != "Taro" {
		t.Fatalf("nickname = %q", get(m, "nickname"))
	}

	m = runes(m, "x")
	if has(m, "nickname") {
		t.Error("x should unset nickname")
	}

	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "Jiro")
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyCtrlD)
	if has(m, "nickname") {
		t.Error("ctrl+d should unset nickname")
	}
}

func TestRequestEditor_ScalarEditEmptyString(t *testing.T) {
	m := moveTo(t, newEditor(t), idxNickname)
	m, _ = press(m, tea.KeyEnter)
	if m.Mode() != ModeText {
		t.Fatalf("mode = %v, want ModeText", m.Mode())
	}
	m, _ = press(m, tea.KeyEnter)

	if m.Mode() != ModeBrowse {
		t.Errorf("mode = %v, want ModeBrowse", m.Mode())
	}
	if !has(m, "nickname") || get(m, "nickname") != "" {
		t.Errorf("nickname present=%v value=%q, want present empty string", has(m, "nickname"), get(m, "nickname"))
	}
	if !strings.Contains(norm(m.View()), `nickname optional string ""`) {
		t.Errorf("view should show the empty string:\n%s", m.View())
	}
}

func TestRequestEditor_ReEditPrefillsCurrentValue(t *testing.T) {
	m := moveTo(t, newEditor(t), idxAge)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "20")
	m, _ = press(m, tea.KeyEnter)

	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "1")
	m, _ = press(m, tea.KeyEnter)
	if v, _ := m.Builder().Get(request.Path("age")); v.Int() != 201 {
		t.Errorf("age = %d, want 201", v.Int())
	}
}

func TestRequestEditor_InvalidInputKeepsTextMode(t *testing.T) {
	m := moveTo(t, newEditor(t), idxAge)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "abc")
	m, _ = press(m, tea.KeyEnter)

	if m.Mode() != ModeText {
		t.Errorf("mode = %v, want ModeText after invalid input", m.Mode())
	}
	if !strings.Contains(m.View(), "✘") {
		t.Errorf("view should show the error:\n%s", m.View())
	}
	if has(m, "age") {
		t.Error("invalid input must not be stored")
	}

	m, _ = press(m, tea.KeyEsc)
	if m.Mode() != ModeBrowse || has(m, "age") {
		t.Errorf("esc should cancel: mode=%v has=%v", m.Mode(), has(m, "age"))
	}
}

func TestRequestEditor_CtrlGCancelsTextInput(t *testing.T) {
	m := moveTo(t, newEditor(t), idxName)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "Taro")
	m, _ = press(m, tea.KeyCtrlG)
	if m.Mode() != ModeBrowse || has(m, "name") {
		t.Errorf("ctrl+g should cancel: mode=%v has=%v", m.Mode(), has(m, "name"))
	}
}

func TestRequestEditor_EnumSelection(t *testing.T) {
	m := moveTo(t, newEditor(t), idxStatus)
	m, _ = press(m, tea.KeyEnter)
	if m.Mode() != ModeChoice {
		t.Fatalf("mode = %v, want ModeChoice", m.Mode())
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)

	if v, _ := m.Builder().Get(request.Path("status")); v.Enum() != 1 {
		t.Errorf("status = %d, want 1 (ACTIVE)", v.Enum())
	}
	if strings.Contains(m.View(), "<unset>\n") && m.Mode() == ModeChoice {
		t.Error("implicit presence enum should not offer <unset>")
	}
}

func TestRequestEditor_OptionalBoolOffersUnset(t *testing.T) {
	m := moveTo(t, newEditor(t), idxAdmin)
	m, _ = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "<unset>") {
		t.Fatalf("optional bool should offer <unset>:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyHome)
	m, _ = press(m, tea.KeyUp)
	m, _ = press(m, tea.KeyUp)
	m, _ = press(m, tea.KeyEnter)
	if v, ok := m.Builder().Get(request.Path("admin")); !ok || !v.Bool() {
		t.Errorf("admin = %v present=%v, want true", v.Bool(), ok)
	}

	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if has(m, "admin") {
		t.Error("selecting <unset> should clear admin")
	}
}

func TestRequestEditor_OneofSwitching(t *testing.T) {
	m := moveTo(t, newEditor(t), idxContact)
	m, _ = press(m, tea.KeyEnter)
	if m.Mode() != ModeChoice || !strings.Contains(m.View(), "email") || !strings.Contains(m.View(), "phone") {
		t.Fatalf("oneof should offer its members:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyHome)
	m, _ = press(m, tea.KeyUp)
	m, _ = press(m, tea.KeyUp)
	m, _ = press(m, tea.KeyEnter)
	if m.Mode() != ModeText {
		t.Fatalf("choosing email should open its editor, mode = %v", m.Mode())
	}
	m = runes(m, "a@example.com")
	m, _ = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), `email: "a@example.com"`) {
		t.Errorf("oneof row should show the active member:\n%s", m.View())
	}

	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "090")
	m, _ = press(m, tea.KeyEnter)
	if has(m, "email") || !has(m, "phone") {
		t.Errorf("email=%v phone=%v, want only phone", has(m, "email"), has(m, "phone"))
	}

	m = runes(m, "x")
	if has(m, "phone") || has(m, "email") {
		t.Error("x on the oneof row should clear the active member")
	}

	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "b@example.com")
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if has(m, "email") {
		t.Error("<unset> should clear the oneof")
	}
}

func TestRequestEditor_NestedMessageStack(t *testing.T) {
	m := moveTo(t, newEditor(t), idxProfile)
	m, _ = press(m, tea.KeyEnter)
	if m.Depth() != 1 {
		t.Fatalf("depth = %d, want 1", m.Depth())
	}
	if !strings.Contains(m.View(), "profile (Profile)") {
		t.Errorf("breadcrumb missing:\n%s", m.View())
	}
	if has(m, "profile") {
		t.Error("entering a nested message must not make it present")
	}

	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "hello")
	m, _ = press(m, tea.KeyEnter)
	if get(m, "profile", "bio") != "hello" {
		t.Errorf("profile.bio = %q", get(m, "profile", "bio"))
	}

	m, _ = press(m, tea.KeyEsc)
	if m.Depth() != 0 || m.Cursor() != idxProfile {
		t.Errorf("after esc: depth=%d cursor=%d, want 0 %d", m.Depth(), m.Cursor(), idxProfile)
	}
	if !strings.Contains(m.View(), "{...}") {
		t.Errorf("profile should be shown as present:\n%s", m.View())
	}
}

func TestRequestEditor_RepeatedScalarList(t *testing.T) {
	m := moveTo(t, newEditor(t), idxTags)
	m, _ = press(m, tea.KeyEnter)
	if m.FrameKind() != FrameList || !strings.Contains(m.View(), "+ Add") {
		t.Fatalf("enter on a repeated field should open the list editor:\n%s", m.View())
	}

	for _, tag := range []string{"go", "grpc", "tui"} {
		m, _ = press(m, tea.KeyEnter)
		m = runes(m, tag)
		m, _ = press(m, tea.KeyEnter)
	}
	if got, _ := m.Builder().FieldJSON(request.Path("tags")); got != `["go","grpc","tui"]` {
		t.Fatalf("tags = %s", got)
	}
	if m.Cursor() != 3 {
		t.Errorf("cursor should stay on + Add, got %d", m.Cursor())
	}

	m, _ = press(m, tea.KeyHome)
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyCtrlU)
	m = runes(m, "connect")
	m, _ = press(m, tea.KeyEnter)

	m = runes(m, "K")
	if m.Cursor() != 0 {
		t.Errorf("K should follow the moved element, cursor = %d", m.Cursor())
	}
	m = runes(m, "J")
	m = runes(m, "J")
	if got, _ := m.Builder().FieldJSON(request.Path("tags")); got != `["go","tui","connect"]` {
		t.Errorf("after edit/move: %s", got)
	}

	m = runes(m, "d")
	if got, _ := m.Builder().FieldJSON(request.Path("tags")); got != `["go","tui"]` {
		t.Errorf("after delete: %s", got)
	}
	if m.Cursor() != 2 {
		t.Errorf("cursor should be clamped after delete, got %d", m.Cursor())
	}

	m, _ = press(m, tea.KeyEsc)
	if m.FrameKind() != FrameMessage || !strings.Contains(m.View(), `[2 items] ["go","tui"]`) {
		t.Errorf("after esc:\n%s", m.View())
	}
}

func TestRequestEditor_AddFromMessageFrame(t *testing.T) {
	m := moveTo(t, newEditor(t), idxTags)
	m = runes(m, "a")
	if m.FrameKind() != FrameList || m.Mode() != ModeText {
		t.Fatalf("a on a repeated field should open the list and start adding: frame=%v mode=%v", m.FrameKind(), m.Mode())
	}
	m = runes(m, "go")
	m, _ = press(m, tea.KeyEnter)
	if got, _ := m.Builder().FieldJSON(request.Path("tags")); got != `["go"]` {
		t.Errorf("tags = %s", got)
	}
}

func TestRequestEditor_RepeatedEnumList(t *testing.T) {
	m := moveTo(t, newEditor(t), idxHistory)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "a")
	if m.Mode() != ModeChoice {
		t.Fatalf("adding to a repeated enum should use the choice UI, mode = %v", m.Mode())
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if got, _ := m.Builder().FieldJSON(request.Path("history")); got != `["USER_STATUS_ACTIVE"]` {
		t.Errorf("history = %s", got)
	}
}

func TestRequestEditor_RepeatedMessageList(t *testing.T) {
	m := moveTo(t, newEditor(t), idxAddresses)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "a")
	if m.FrameKind() != FrameMessage || m.Depth() != 2 || !strings.Contains(m.View(), "addresses[0] (Address)") {
		t.Fatalf("adding a message element should open it:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "Tokyo")
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyEsc)

	if m.FrameKind() != FrameList || !strings.Contains(m.View(), `{"city":"Tokyo"}`) {
		t.Errorf("list should show the element:\n%s", m.View())
	}
	if got := toJSON(t, m); got != `{"addresses":[{"city":"Tokyo"}]}` {
		t.Errorf("message = %s", got)
	}
}

func TestRequestEditor_MapScalarEntries(t *testing.T) {
	m := moveTo(t, newEditor(t), idxLabels)
	m, _ = press(m, tea.KeyEnter)
	if m.FrameKind() != FrameMap {
		t.Fatalf("frame = %v, want map", m.FrameKind())
	}

	add := func(k, v string) {
		m = runes(m, "a")
		m = runes(m, k)
		m, _ = press(m, tea.KeyEnter)
		m = runes(m, v)
		m, _ = press(m, tea.KeyEnter)
	}
	add("team", "backend")
	add("env", "dev")
	if got, _ := m.Builder().FieldJSON(request.Path("labels")); got != `{"env":"dev","team":"backend"}` {
		t.Fatalf("labels = %s", got)
	}
	if v := m.View(); strings.Index(v, `"env"`) > strings.Index(v, `"team"`) {
		t.Errorf("entries should be sorted by key:\n%s", v)
	}

	m, _ = press(m, tea.KeyHome)
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyCtrlU)
	m = runes(m, "prod")
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "x")
	if got, _ := m.Builder().FieldJSON(request.Path("labels")); got != `{"team":"backend"}` {
		t.Errorf("after update+delete: %s", got)
	}
}

func TestRequestEditor_MapIntKeyValidation(t *testing.T) {
	m := moveTo(t, newEditor(t), idxScores)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "a")
	m = runes(m, "abc")
	m, _ = press(m, tea.KeyEnter)
	if m.Mode() != ModeText || !strings.Contains(m.View(), "✘") {
		t.Fatalf("invalid int32 key should be rejected:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyCtrlU)
	m = runes(m, "7")
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "seven")
	m, _ = press(m, tea.KeyEnter)
	if got, _ := m.Builder().FieldJSON(request.Path("scores")); got != `{"7":"seven"}` {
		t.Errorf("scores = %s", got)
	}
}

func TestRequestEditor_MapMessageValues(t *testing.T) {
	m := moveTo(t, newEditor(t), idxPlaces)
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "a")
	m = runes(m, "home")
	m, _ = press(m, tea.KeyEnter)
	if m.FrameKind() != FrameMessage || !strings.Contains(m.View(), `places["home"] (Address)`) {
		t.Fatalf("message map value should open its editor:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "JP")
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyEsc)
	if got := toJSON(t, m); got != `{"places":{"home":{"country":"JP"}}}` {
		t.Errorf("message = %s", got)
	}
}

func TestRequestEditor_EditListAsJSON(t *testing.T) {
	m := moveTo(t, newEditor(t), idxTags)
	m = runes(m, "e")
	m = runes(m, `["go","grpc"]`)
	m, _ = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), `[2 items] ["go","grpc"]`) {
		t.Errorf("tags display:\n%s", m.View())
	}

	m, _ = press(m, tea.KeyEnter)
	m = runes(m, "e")
	m, _ = press(m, tea.KeyCtrlU)
	m, _ = press(m, tea.KeyEnter)
	if has(m, "tags") {
		t.Error("empty JSON input should unset tags")
	}
}

func toJSON(t *testing.T, m Model) string {
	t.Helper()
	b, err := protojson.Marshal(m.Builder().Message())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestRequestEditor_EditAsJSON(t *testing.T) {
	m := moveTo(t, newEditor(t), idxProfile)
	m = runes(m, "e")
	m = runes(m, `{"bio":"json"}`)
	m, _ = press(m, tea.KeyEnter)
	if get(m, "profile", "bio") != "json" {
		t.Errorf("profile.bio = %q", get(m, "profile", "bio"))
	}
}

func TestRequestEditor_PreviewAndCancelMsgs(t *testing.T) {
	m := newEditor(t)
	_, cmd := press(m, tea.KeyCtrlS)
	if _, ok := cmd().(PreviewMsg); !ok {
		t.Errorf("ctrl+s msg = %#v, want PreviewMsg", cmd())
	}
	_, cmd = press(m, tea.KeyEsc)
	if _, ok := cmd().(CancelMsg); !ok {
		t.Errorf("esc at root msg = %#v, want CancelMsg", cmd())
	}
	_, cmd = press(m, tea.KeyCtrlG)
	if _, ok := cmd().(CancelMsg); !ok {
		t.Errorf("ctrl+g at root msg = %#v, want CancelMsg", cmd())
	}
	_, cmd = press(m, tea.KeyLeft)
	if cmd != nil {
		t.Error("left at root should not cancel")
	}
}

func TestRequestEditor_ViewShowsUnsetAndDefaults(t *testing.T) {
	v := norm(newEditor(t).View())
	for _, want := range []string{
		`name      string              ""`,
		`nickname  optional string     <unset>`,
		`status    UserStatus          USER_STATUS_UNSPECIFIED`,
		`labels    map<string,string>  {}`,
		`contact    oneof               <unset>`,
	} {
		if !strings.Contains(v, norm(want)) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

var spaces = regexp.MustCompile(` +`)

func norm(s string) string {
	return spaces.ReplaceAllString(s, " ")
}
