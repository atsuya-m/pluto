package jsonview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func snapshot(t *testing.T, data string, width, maxLines int) (string, bool) {
	t.Helper()
	out, truncated, err := Snapshot([]byte(data), width, maxLines)
	if err != nil {
		t.Fatal(err)
	}
	return out, truncated
}

func TestSnapshot_ExpandsEverythingThatFits(t *testing.T) {
	got, truncated := snapshot(t, `{"user":{"id":"u-1","tags":["a","b"],"meta":{}}}`, 80, 10)
	want := strings.Join([]string{
		`user: {3 keys}`,
		`  id: "u-1"`,
		`  tags: [2 items]`,
		`    [0]: "a"`,
		`    [1]: "b"`,
		`  meta: {}`,
	}, "\n")
	if got != want || truncated {
		t.Errorf("truncated = %v, got:\n%s\nwant:\n%s", truncated, got, want)
	}
}

func TestSnapshot_ElidesArraysEvenWhenTheyFit(t *testing.T) {
	got, truncated := snapshot(t, `{"ids":[1,2,3,4,5]}`, 80, 100)
	want := strings.Join([]string{
		`ids: [5 items]`,
		`  [0]: 1`,
		`  [1]: 2`,
		`  [2]: 3`,
		`  … 2 more items`,
	}, "\n")
	if got != want || !truncated {
		t.Errorf("truncated = %v, got:\n%s\nwant:\n%s", truncated, got, want)
	}
}

func TestSnapshot_ElidesLongArraysAndPreviewsCollapsedNodes(t *testing.T) {
	var users []string
	for i := range 40 {
		users = append(users, fmt.Sprintf(`{"id":"u-%02d","name":"user%02d","tags":["x"]}`, i, i))
	}
	got, truncated := snapshot(t, `{"users":[`+strings.Join(users, ",")+`],"nextPageToken":"p2"}`, 80, 10)
	want := strings.Join([]string{
		`users: [40 items]`,
		`  [0]: {id: "u-00", name: "user00", tags: [1 item]}`,
		`  [1]: {id: "u-01", name: "user01", tags: [1 item]}`,
		`  [2]: {id: "u-02", name: "user02", tags: [1 item]}`,
		`  … 37 more items`,
		`nextPageToken: "p2"`,
	}, "\n")
	if got != want || !truncated {
		t.Errorf("truncated = %v, got:\n%s\nwant:\n%s", truncated, got, want)
	}
}

func TestSnapshot_PreviewIsCutAtWidth(t *testing.T) {
	data := `{"a":{"b":{"c":1}},"items":[` + strings.Repeat(`{"x":1},`, 5) + `{"x":1}],"s":"` + strings.Repeat("long text ", 6) + `"}`
	got, truncated := snapshot(t, data, 40, 3)
	want := strings.Join([]string{
		`a: {b: {1 key}}`,
		`items: [{1 key}, {1 key}, {1 key}, …]`,
		`s: "long text long text long text long …`,
	}, "\n")
	if got != want || !truncated {
		t.Errorf("truncated = %v, got:\n%s\nwant:\n%s", truncated, got, want)
	}
	for _, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line is %d columns wide: %q", w, line)
		}
	}
}

func TestSnapshot_CutsTopLevelWhenItDoesNotFit(t *testing.T) {
	var fields []string
	for i := range 30 {
		fields = append(fields, fmt.Sprintf(`"f%02d":%d`, i, i))
	}
	got, truncated := snapshot(t, `{`+strings.Join(fields, ",")+`}`, 80, 5)
	want := strings.Join([]string{`f00: 0`, `f01: 1`, `f02: 2`, `f03: 3`, `… 26 more`}, "\n")
	if got != want || !truncated {
		t.Errorf("truncated = %v, got:\n%s\nwant:\n%s", truncated, got, want)
	}
}

func TestSnapshot_ScalarAndEmpty(t *testing.T) {
	if got, _ := snapshot(t, `{}`, 80, 10); got != "{}" {
		t.Errorf("empty object = %q", got)
	}
	if got, _ := snapshot(t, `"hi"`, 80, 10); got != `"hi"` {
		t.Errorf("scalar = %q", got)
	}
}
