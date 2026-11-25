package dom

import "golang.org/x/net/html"

// The page is read whole before its scripts run, but each of its
// scripts that is no defer, async or module script sees it as far as
// a browser's parser had read it when it ran: what comes after the
// script is taken out of the tree before the first runs, and given back
// node by node, at the end of its parent as a parser adds it, up to
// each script before it runs and all of it once the last has.

// unread is what the parser had not read of the page yet.
type unread struct {
	// nodes are those after the first script that runs as it is read,
	// in the order they are read, each with the parent it was read in.
	nodes   []*html.Node
	parents []*html.Node
	// ends is where each script's text ends among nodes, and next the
	// first not given back yet.
	ends map[*html.Node]int
	next int
}

// holdBack takes out of the tree what comes after the first of the
// scripts that run as they are read.
func (r *Runtime) holdBack(first *html.Node) *unread {
	held := &unread{ends: map[*html.Node]int{}}
	if first == nil {
		return held
	}
	var order []*html.Node
	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		order = append(order, n)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
		if isScript(n) {
			held.ends[n] = len(order)
		}
	}
	visit(r.tree.Root())
	from := held.ends[first]
	if from == 0 {
		return held
	}
	held.nodes = order[from:]
	held.parents = make([]*html.Node, len(held.nodes))
	for i, n := range held.nodes {
		held.parents[i] = n.Parent
	}
	for n, end := range held.ends {
		held.ends[n] = max(end-from, 0)
	}
	for i := len(held.nodes) - 1; i >= 0; i-- {
		r.tree.Remove(held.nodes[i])
	}
	return held
}

// reach gives back what the parser read up to the end of script, all
// of what is left for none.
func (r *Runtime) reach(held *unread, script *html.Node) {
	end := len(held.nodes)
	if script != nil {
		if at, known := held.ends[script]; known {
			end = at
		}
	}
	for ; held.next < end; held.next++ {
		r.tree.Insert(held.parents[held.next], held.nodes[held.next], nil)
	}
}
