package jsonview

import (
	"strings"
	"testing"
)

const sample = `{
  "user": {
    "id": "u-1",
    "profile": {"bio": "hello", "tags": ["go", "grpc"]},
    "age": 20,
    "admin": false,
    "note": null
  },
  "items": [{"name": "a"}, {"name": "b"}],
  "weird key": 1
}`

func labels(nodes []*Node) string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Label())
	}
	return strings.Join(out, ",")
}

func TestParseKeepsOrderAndKinds(t *testing.T) {
	root, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if labels(root.Children) != "user,items,weird key" {
		t.Errorf("top level = %s", labels(root.Children))
	}
	user := root.Children[0]
	if labels(user.Children) != "id,profile,age,admin,note" {
		t.Errorf("user = %s", labels(user.Children))
	}
	kinds := []Kind{KindString, KindObject, KindNumber, KindBool, KindNull}
	for i, c := range user.Children {
		if c.Kind != kinds[i] {
			t.Errorf("%s kind = %v, want %v", c.Key, c.Kind, kinds[i])
		}
	}
	if user.Children[0].Value != `"u-1"` || user.Children[2].Value != "20" || user.Children[4].Value != "null" {
		t.Errorf("values = %q %q %q", user.Children[0].Value, user.Children[2].Value, user.Children[4].Value)
	}
	if user.Depth != 0 || user.Children[1].Children[0].Depth != 2 {
		t.Errorf("depths = %d %d", user.Depth, user.Children[1].Children[0].Depth)
	}

	for _, bad := range []string{`{`, `{"a":1} {}`, `[1,]`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
	if scalar, err := Parse([]byte(`"x"`)); err != nil || len(Visible(scalar)) != 1 {
		t.Errorf("scalar root: %v", err)
	}
}

func TestPathAndSummary(t *testing.T) {
	root, _ := Parse([]byte(sample))
	tags := root.Children[0].Children[1].Children[1]
	if tags.Path() != ".user.profile.tags" || tags.Children[1].Path() != ".user.profile.tags[1]" {
		t.Errorf("paths = %s %s", tags.Path(), tags.Children[1].Path())
	}
	if root.Children[2].Path() != `["weird key"]` {
		t.Errorf("quoted path = %s", root.Children[2].Path())
	}
	if root.Path() != "." {
		t.Errorf("root path = %s", root.Path())
	}
	if tags.Summary() != "[2 items]" || root.Children[0].Summary() != "{5 keys}" || root.Children[0].Children[1].Children[0].Summary() != `"hello"` {
		t.Errorf("summaries = %s %s", tags.Summary(), root.Children[0].Summary())
	}
	one, _ := Parse([]byte(`{"a":[1],"b":{"c":1}}`))
	if one.Children[0].Summary() != "[1 item]" || one.Children[1].Summary() != "{1 key}" {
		t.Error("singular summaries")
	}
}

func TestVisibleAndExpansion(t *testing.T) {
	root, _ := Parse([]byte(sample))
	ExpandToDepth(root, 0)
	if labels(Visible(root)) != "user,items,weird key" {
		t.Errorf("depth 0 = %s", labels(Visible(root)))
	}
	ExpandToDepth(root, 1)
	if labels(Visible(root)) != "user,id,profile,age,admin,note,items,[0],[1],weird key" {
		t.Errorf("depth 1 = %s", labels(Visible(root)))
	}
	ExpandAll(root)
	if len(Visible(root)) != Count(root) {
		t.Errorf("expand all = %d, count = %d", len(Visible(root)), Count(root))
	}
	CollapseAll(root)
	if len(Visible(root)) != 3 {
		t.Errorf("collapse all = %d", len(Visible(root)))
	}
}

func TestAutoExpand(t *testing.T) {
	root, _ := Parse([]byte(sample))
	AutoExpand(root, 100)
	if len(Visible(root)) != Count(root) {
		t.Errorf("small document should be fully expanded: %d/%d", len(Visible(root)), Count(root))
	}
	AutoExpand(root, 5)
	if got := len(Visible(root)); got != 3 {
		t.Errorf("tight budget should keep top level only, got %d lines", got)
	}
	AutoExpand(root, 10)
	if got := len(Visible(root)); got > 10 || got < 3 {
		t.Errorf("visible lines = %d, budget 10", got)
	}
}

func TestAutoExpandOpensSingleWrapper(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"users":[`)
	for i := range 100 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":1}`)
	}
	b.WriteString(`]}`)
	root, _ := Parse([]byte(b.String()))
	AutoExpand(root, 10)
	if got := len(Visible(root)); got != 101 {
		t.Errorf("a single top-level field should be opened even if it overflows: %d lines", got)
	}
	if root.Children[0].Children[0].Expanded {
		t.Error("only the wrapper should be opened")
	}
}

func TestSearchAndReveal(t *testing.T) {
	root, _ := Parse([]byte(sample))
	CollapseAll(root)
	matches := Search(root, "GRPC")
	if len(matches) != 1 || matches[0].Path() != ".user.profile.tags[1]" {
		t.Fatalf("matches = %v", matches)
	}
	Reveal(matches[0])
	found := false
	for _, n := range Visible(root) {
		if n == matches[0] {
			found = true
		}
	}
	if !found {
		t.Error("Reveal should make the match visible")
	}
	if got := Search(root, "name"); len(got) != 2 {
		t.Errorf("key search = %d", len(got))
	}
	if Search(root, "  ") != nil {
		t.Error("blank query should match nothing")
	}
}

func TestCopyText(t *testing.T) {
	root, _ := Parse([]byte(`{"s":"a \"q\" b","n":1.5,"b":true,"z":null,"o":{"x":[1,{"y":"z"}],"e":{},"a":[]}}`))
	byKey := map[string]*Node{}
	for _, c := range root.Children {
		byKey[c.Key] = c
	}
	tests := map[string]string{
		"s": `a "q" b`,
		"n": "1.5",
		"b": "true",
		"z": "null",
		"o": "{\n  \"x\": [\n    1,\n    {\n      \"y\": \"z\"\n    }\n  ],\n  \"e\": {},\n  \"a\": []\n}",
	}
	for k, want := range tests {
		if got := byKey[k].CopyText(); got != want {
			t.Errorf("%s: CopyText = %q, want %q", k, got, want)
		}
	}
}
