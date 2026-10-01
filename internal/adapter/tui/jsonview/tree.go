package jsonview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Kind int

const (
	KindObject Kind = iota
	KindArray
	KindString
	KindNumber
	KindBool
	KindNull
)

type Node struct {
	Key      string
	Index    int
	Kind     Kind
	Value    string
	Children []*Node
	Parent   *Node
	Depth    int
	Expanded bool
}

func (n *Node) IsContainer() bool {
	return n.Kind == KindObject || n.Kind == KindArray
}

func Parse(data []byte) (*Node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	root, err := parseValue(dec, nil, "", -1, -1)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("unexpected data after JSON value")
	}
	root.Expanded = true
	return root, nil
}

func parseValue(dec *json.Decoder, parent *Node, key string, index, depth int) (*Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	n := &Node{Key: key, Index: index, Parent: parent, Depth: depth}
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			n.Kind = KindObject
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("invalid object key %v", kt)
				}
				child, err := parseValue(dec, n, k, -1, depth+1)
				if err != nil {
					return nil, err
				}
				n.Children = append(n.Children, child)
			}
		case '[':
			n.Kind = KindArray
			for i := 0; dec.More(); i++ {
				child, err := parseValue(dec, n, "", i, depth+1)
				if err != nil {
					return nil, err
				}
				n.Children = append(n.Children, child)
			}
		default:
			return nil, fmt.Errorf("unexpected delimiter %v", v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
	case string:
		n.Kind = KindString
		n.Value = strconv.Quote(v)
	case json.Number:
		n.Kind = KindNumber
		n.Value = v.String()
	case bool:
		n.Kind = KindBool
		n.Value = strconv.FormatBool(v)
	case nil:
		n.Kind = KindNull
		n.Value = "null"
	default:
		return nil, fmt.Errorf("unexpected token %v", tok)
	}
	return n, nil
}

func (n *Node) Label() string {
	if n.Index >= 0 {
		return "[" + strconv.Itoa(n.Index) + "]"
	}
	return n.Key
}

func (n *Node) Summary() string {
	switch n.Kind {
	case KindObject:
		if len(n.Children) == 1 {
			return "{1 key}"
		}
		return fmt.Sprintf("{%d keys}", len(n.Children))
	case KindArray:
		if len(n.Children) == 1 {
			return "[1 item]"
		}
		return fmt.Sprintf("[%d items]", len(n.Children))
	default:
		return n.Value
	}
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (n *Node) Path() string {
	if n.Parent == nil {
		return "."
	}
	var parts []string
	for c := n; c.Parent != nil; c = c.Parent {
		switch {
		case c.Index >= 0:
			parts = append(parts, "["+strconv.Itoa(c.Index)+"]")
		case identifier.MatchString(c.Key):
			parts = append(parts, "."+c.Key)
		default:
			parts = append(parts, "["+strconv.Quote(c.Key)+"]")
		}
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(parts[i])
	}
	return b.String()
}

func Visible(root *Node) []*Node {
	var out []*Node
	var walk func(*Node)
	walk = func(n *Node) {
		out = append(out, n)
		if n.IsContainer() && n.Expanded {
			for _, c := range n.Children {
				walk(c)
			}
		}
	}
	if !root.IsContainer() {
		return []*Node{root}
	}
	for _, c := range root.Children {
		walk(c)
	}
	return out
}

func Walk(root *Node, fn func(*Node)) {
	fn(root)
	for _, c := range root.Children {
		Walk(c, fn)
	}
}

func Count(root *Node) int {
	n := 0
	Walk(root, func(*Node) { n++ })
	return n - 1
}

func ExpandAll(n *Node) {
	Walk(n, func(c *Node) { c.Expanded = true })
}

func CollapseAll(n *Node) {
	Walk(n, func(c *Node) {
		if c.Parent != nil {
			c.Expanded = false
		}
	})
}

func ExpandToDepth(root *Node, depth int) {
	Walk(root, func(c *Node) {
		c.Expanded = c.Parent == nil || c.Depth < depth
	})
}

func AutoExpand(root *Node, maxLines int) {
	defer func() {
		if len(root.Children) == 1 {
			root.Children[0].Expanded = true
		}
	}()
	ExpandToDepth(root, 0)
	for depth := 1; depth < 64; depth++ {
		ExpandToDepth(root, depth)
		if len(Visible(root)) > maxLines {
			ExpandToDepth(root, depth-1)
			return
		}
		deeper := false
		Walk(root, func(c *Node) {
			if c.IsContainer() && !c.Expanded && len(c.Children) > 0 {
				deeper = true
			}
		})
		if !deeper {
			return
		}
	}
}

func Reveal(n *Node) {
	for p := n.Parent; p != nil; p = p.Parent {
		p.Expanded = true
	}
}

func Search(root *Node, query string) []*Node {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var out []*Node
	Walk(root, func(n *Node) {
		if n.Parent == nil {
			return
		}
		if strings.Contains(strings.ToLower(n.Key), q) || (!n.IsContainer() && strings.Contains(strings.ToLower(n.Value), q)) {
			out = append(out, n)
		}
	})
	return out
}

func (n *Node) CopyText() string {
	switch n.Kind {
	case KindString:
		if v, err := strconv.Unquote(n.Value); err == nil {
			return v
		}
		return n.Value
	case KindObject, KindArray:
		var b strings.Builder
		writeJSON(&b, n, 0)
		return b.String()
	default:
		return n.Value
	}
}

func writeJSON(b *strings.Builder, n *Node, indent int) {
	pad := strings.Repeat("  ", indent+1)
	switch n.Kind {
	case KindObject:
		if len(n.Children) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, c := range n.Children {
			b.WriteString(pad + strconv.Quote(c.Key) + ": ")
			writeJSON(b, c, indent+1)
			if i < len(n.Children)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", indent) + "}")
	case KindArray:
		if len(n.Children) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, c := range n.Children {
			b.WriteString(pad)
			writeJSON(b, c, indent+1)
			if i < len(n.Children)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", indent) + "]")
	default:
		b.WriteString(n.Value)
	}
}
