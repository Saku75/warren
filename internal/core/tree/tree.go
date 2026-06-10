// Package tree assembles flat, parent-linked rows into slug trees. Every
// parent-scoped type (locations, tenant groups, site groups) renders and
// addresses the same way, so they share this one implementation.
package tree

import "github.com/saku75/warren/internal/core/id"

// Node wraps an item with its slug path and children.
type Node[T any] struct {
	Item T
	// Path is the slug path from the tree's root, including any prefix
	// given to Build (e.g. the owning site's slug).
	Path     string
	Children []*Node[T]
}

// Build assembles nodes from rows. Sibling order follows the input order,
// so callers pass rows sorted as they want them displayed. prefix is
// prepended to every path ("" for global trees, "site-slug/" for
// site-anchored ones). Rows whose parent is absent from items are dropped;
// that cannot happen for full-table queries.
func Build[T any](items []T, itemID func(T) id.ID, parentID func(T) *id.ID, slug func(T) string, prefix string) []*Node[T] {
	nodes := make(map[id.ID]*Node[T], len(items))
	for _, it := range items {
		nodes[itemID(it)] = &Node[T]{Item: it}
	}

	var roots []*Node[T]
	for _, it := range items {
		n := nodes[itemID(it)]
		if p := parentID(it); p == nil {
			n.Path = prefix + slug(it)
			roots = append(roots, n)
		} else if parent, ok := nodes[*p]; ok {
			parent.Children = append(parent.Children, n)
		}
	}

	var fill func(n *Node[T])
	fill = func(n *Node[T]) {
		for _, c := range n.Children {
			c.Path = n.Path + "/" + slug(c.Item)
			fill(c)
		}
	}
	for _, r := range roots {
		fill(r)
	}
	return roots
}

// Flat is a node with its depth, for indented select options.
type Flat[T any] struct {
	Node  *Node[T]
	Depth int
}

// Indent returns a non-breaking-space indent matching the depth.
func (f Flat[T]) Indent() string {
	out := ""
	for range f.Depth {
		out += "   "
	}
	return out
}

// Flatten lists nodes depth-first, parents before children.
func Flatten[T any](roots []*Node[T]) []Flat[T] {
	var out []Flat[T]
	var walk func(ns []*Node[T], depth int)
	walk = func(ns []*Node[T], depth int) {
		for _, n := range ns {
			out = append(out, Flat[T]{Node: n, Depth: depth})
			walk(n.Children, depth+1)
		}
	}
	walk(roots, 0)
	return out
}
