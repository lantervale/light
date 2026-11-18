package dom

import (
	"errors"
	"fmt"

	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"
)

// The names of elements, attributes and events light treats apart.
const (
	input   = "input"
	button  = "button"
	selects = "select"
	style   = "style"
	checked = "checked"
	submit  = "submit"
)

// handles are the numbers scripts know the nodes of the page's tree
// by: a text node's is negative, any other's positive, a CDATA
// section's among them.
type handles struct {
	nodes map[int]*html.Node
	ids   map[*html.Node]int
	last  int
	made  int
	// docs are the documents the page's scripts made, and owners the
	// document of each node that is not the page's (documents.go).
	docs   map[*html.Node]*made
	owners map[*html.Node]*html.Node
	// prefixes are the prefixes scripts gave the elements they made.
	prefixes map[*html.Node]string
}

func newHandles() handles {
	return handles{nodes: map[int]*html.Node{}, ids: map[*html.Node]int{}, docs: map[*html.Node]*made{},
		owners: map[*html.Node]*html.Node{}, prefixes: map[*html.Node]string{}}
}

// ref is the number of n, 0 for none: a new one the first time.
func (h *handles) ref(n *html.Node) int {
	if n == nil {
		return 0
	}
	if id, known := h.ids[n]; known {
		return id
	}
	h.last++
	id := h.last
	if n.Type == html.TextNode && n.Namespace == "" {
		id = -id
	}
	h.nodes[id], h.ids[n] = n, id
	return id
}

func (h *handles) refs(nodes []*html.Node) []int {
	out := make([]int, len(nodes))
	for i, n := range nodes {
		out[i] = h.ref(n)
	}
	return out
}

// release forgets the number of a node no script holds any more.
func (h *handles) release(id int) {
	if n, known := h.nodes[id]; known {
		delete(h.nodes, id)
		delete(h.ids, n)
	}
}

var errNoNode = errors.New("light: a script named a node the page does not have")

// arguments are what a script gave an operation.
type arguments []any

func (a arguments) text(i int) string {
	if i >= len(a) {
		return ""
	}
	switch v := a[i].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func (a arguments) number(i int) float64 {
	if i >= len(a) {
		return 0
	}
	n, _ := a[i].(float64)
	return n
}

func (a arguments) flag(i int) bool {
	if i >= len(a) {
		return false
	}
	on, _ := a[i].(bool)
	return on
}

// node is the node a script named by its number.
func (r *Runtime) node(a arguments, i int) (*html.Node, error) {
	n := r.nodes[int(a.number(i))]
	if n == nil {
		return nil, errNoNode
	}
	return n, nil
}

// op is one operation scripts ask of light.
type op func(a arguments) (any, error)

// of is an operation on the node of the tree its first argument names.
func (r *Runtime) of(do func(n *html.Node, a arguments) (any, error)) op {
	return func(a arguments) (any, error) {
		n, err := r.node(a, 0)
		if err != nil {
			return nil, err
		}
		return do(n, a)
	}
}

// on is an operation on what ui has of the node its first argument
// names: its element, and for a node ui shows nothing of one that is
// nowhere, which has no box, no value and no state.
func (r *Runtime) on(do func(n *ui.Node, a arguments) (any, error)) op {
	return r.of(func(n *html.Node, a arguments) (any, error) { return do(r.shown(n), a) })
}

// shown is what ui has of n, an element that is nowhere for none, as
// for every node of a document a script made: ui makes nothing of
// those, so nothing of them is fetched.
func (r *Runtime) shown(n *html.Node) *ui.Node {
	if r.home(n) != nil {
		return ui.Div()
	}
	if u := r.tree.Make(n); u != nil {
		return u
	}
	if u := r.tree.Node(n); u != nil {
		return u
	}
	return ui.Div()
}

// nodeOf is the node of the tree u shows, its nearest ancestor's that
// shows one for an element ui made itself, nil for none.
func (r *Runtime) nodeOf(u *ui.Node) *html.Node {
	for ; u != nil; u = u.Parent() {
		if n := r.tree.Of(u); n != nil {
			return n
		}
	}
	return nil
}

// refOf is the number of the node u shows, 0 for none.
func (r *Runtime) refOf(u *ui.Node) int { return r.ref(r.nodeOf(u)) }

// nothing is what an operation with nothing to say answers: null.
func nothing() (any, error) {
	return nil, nil //nolint:nilnil // null is an answer
}
