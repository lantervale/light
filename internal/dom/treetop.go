package dom

import "golang.org/x/net/html"

// treeTop is the root of the tree a node is in: the page's document, a
// document a script made, a fragment, or the node itself when nothing
// holds it.
func (r *Runtime) treeTop(n *html.Node, _ arguments) (any, error) {
	for n.Parent != nil {
		n = n.Parent
	}
	if n == r.tree.Root() {
		return r.pageOr(nil, "document"), nil
	}
	return r.ref(n), nil
}
