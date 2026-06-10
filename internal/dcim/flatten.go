package dcim

import "strings"

// FlatLocation is a tree node with its depth, for indented select options
// and similar flattened renderings.
type FlatLocation struct {
	Node  *LocationNode
	Depth int
}

// Indent returns a textual indent matching the node's depth.
func (f FlatLocation) Indent() string {
	return strings.Repeat("  ", f.Depth) // non-breaking spaces survive HTML
}

// FlattenTree lists the tree depth-first, parents before children.
func FlattenTree(nodes []*LocationNode) []FlatLocation {
	var out []FlatLocation
	var walk func(ns []*LocationNode, depth int)
	walk = func(ns []*LocationNode, depth int) {
		for _, n := range ns {
			out = append(out, FlatLocation{Node: n, Depth: depth})
			walk(n.Children, depth+1)
		}
	}
	walk(nodes, 0)
	return out
}
