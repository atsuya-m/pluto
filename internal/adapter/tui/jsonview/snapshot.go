package jsonview

import (
	"fmt"
	"strings"
)

const snapshotArrayItems = 3

func Snapshot(data []byte, width, maxLines int) (string, bool, error) {
	root, err := Parse(data)
	if err != nil {
		return "", false, err
	}
	if !root.IsContainer() {
		return scalarStyle(root).Render(truncate(root.Value, max(width, 16))), false, nil
	}
	if len(root.Children) == 0 {
		return summaryStyle.Render(emptyContainer(root)), false, nil
	}
	lines := snapshotLines(root, width, 0)
	for depth := 1; depth < 64; depth++ {
		next := snapshotLines(root, width, depth)
		if len(next) > maxLines || len(next) == len(lines) {
			break
		}
		lines = next
	}
	truncated := len(lines) != Count(root)
	if len(lines) > maxLines {
		rest := len(lines) - (maxLines - 1)
		lines = append(lines[:maxLines-1], summaryStyle.Render(fmt.Sprintf("… %d more", rest)))
		truncated = true
	}
	return strings.Join(lines, "\n"), truncated, nil
}

func snapshotLines(root *Node, width, depth int) []string {
	var lines []string
	var walk func(n *Node)
	walk = func(n *Node) {
		indent := strings.Repeat("  ", n.Depth)
		label := keyStyle.Render(n.Label()) + ": "
		room := width - len(indent) - len(n.Label()) - 2
		switch {
		case !n.IsContainer():
			lines = append(lines, indent+label+scalarStyle(n).Render(truncate(n.Value, max(room, 16))))
			return
		case len(n.Children) == 0:
			lines = append(lines, indent+label+summaryStyle.Render(emptyContainer(n)))
			return
		case n.Depth >= depth:
			lines = append(lines, indent+label+preview(n, max(room, 16)))
			return
		}
		lines = append(lines, indent+label+summaryStyle.Render(n.Summary()))
		children := n.Children
		elided := 0
		if n.Kind == KindArray && len(children) > snapshotArrayItems {
			elided = len(children) - snapshotArrayItems
			children = children[:snapshotArrayItems]
		}
		for _, c := range children {
			walk(c)
		}
		if elided > 0 {
			lines = append(lines, indent+"  "+summaryStyle.Render(fmt.Sprintf("… %d more items", elided)))
		}
	}
	for _, c := range root.Children {
		walk(c)
	}
	return lines
}

func preview(n *Node, room int) string {
	open, closing := "{", "}"
	if n.Kind == KindArray {
		open, closing = "[", "]"
	}
	plainLen := len(open)
	styled := []string{summaryStyle.Render(open)}
	for i, c := range n.Children {
		item, itemLen := previewItem(n, c)
		sep := ""
		if i > 0 {
			sep = ", "
		}
		tail := len(closing)
		if i < len(n.Children)-1 {
			tail = len(", …") + len(closing)
		}
		if plainLen+len(sep)+itemLen+tail > room {
			if i == 0 {
				return summaryStyle.Render(n.Summary())
			}
			styled = append(styled, summaryStyle.Render(", …"))
			break
		}
		plainLen += len(sep) + itemLen
		styled = append(styled, summaryStyle.Render(sep), item)
	}
	return strings.Join(styled, "") + summaryStyle.Render(closing)
}

func previewItem(parent, c *Node) (string, int) {
	value, plain := "", ""
	switch {
	case !c.IsContainer():
		value, plain = scalarStyle(c).Render(c.Value), c.Value
	case len(c.Children) == 0:
		plain = emptyContainer(c)
		value = summaryStyle.Render(plain)
	default:
		plain = c.Summary()
		value = summaryStyle.Render(plain)
	}
	if parent.Kind == KindArray {
		return value, len([]rune(plain))
	}
	return keyStyle.Render(c.Key) + summaryStyle.Render(": ") + value, len([]rune(c.Key)) + 2 + len([]rune(plain))
}

func emptyContainer(n *Node) string {
	if n.Kind == KindArray {
		return "[]"
	}
	return "{}"
}
