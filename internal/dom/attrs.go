package dom

import (
	"golang.org/x/net/html"

	"github.com/lantervale/light/internal/web"
)

// prefixOf is the prefix n was written with, or made with by a script.
func (r *Runtime) prefixOf(n *html.Node) string {
	if prefix, made := r.prefixes[n]; made {
		return prefix
	}
	return r.tree.Prefixes()[n]
}

// written is the name of an element as HTML's markup writes it: its
// local name for HTML's, SVG's and MathML's, with its prefix for any
// other.
func (r *Runtime) written(n *html.Node) string {
	switch n.Namespace {
	case "", "svg", "math":
		return n.Data
	}
	if prefix := r.prefixOf(n); prefix != "" {
		return prefix + ":" + n.Data
	}
	return n.Data
}

// stated has what ui keeps apart from n's attributes follow one that
// was set: a field's value, a box's check.
func (r *Runtime) stated(n *html.Node, name, value string) {
	u := r.tree.Node(n)
	switch {
	case u == nil:
	case name == "value":
		r.doc.SetValue(u, value)
	case name == checked:
		r.doc.SetChecked(u, true)
	}
}

// attribute answers for the attributes of n as the DOM names them, by
// namespace ("" for none) and local name: "list" is each one's name as
// written, value, namespace, prefix and local name, in the order they
// were set, "own" is the value of one, "set" sets one, which keeps the prefix it had, "replace"
// sets one under the prefix given, and "remove" takes one off.
func (r *Runtime) attribute(n *html.Node, a arguments) (any, error) {
	switch a.text(1) {
	case "list":
		out := make([][5]string, len(n.Attr))
		for i, at := range n.Attr {
			space, prefix, local := web.AttrName(at)
			out[i] = [5]string{web.QualifiedAttr(at), at.Val, space, prefix, local}
		}
		return out, nil
	case "own":
		if value, ok := r.tree.AttrNS(n, a.text(2), a.text(3)); ok {
			return value, nil
		}
	case "set":
		space, local, value := a.text(2), a.text(4), a.text(5)
		_, had := r.tree.AttrNS(n, space, local)
		r.tree.SetAttrNS(n, space, a.text(3), local, value)
		if space == "" {
			r.stated(n, local, value)
			r.scriptAttr(n, local, had)
		}
	case "replace":
		space, local, value := a.text(2), a.text(4), a.text(5)
		_, had := r.tree.AttrNS(n, space, local)
		r.tree.ReplaceAttrNS(n, space, a.text(3), local, value)
		if space == "" {
			r.stated(n, local, value)
			r.scriptAttr(n, local, had)
		}
	case opRemove:
		space, local := a.text(2), a.text(3)
		if u := r.tree.Node(n); u != nil && space == "" && local == checked {
			r.doc.SetChecked(u, false)
		}
		r.tree.RemoveAttrNS(n, space, local)
	}
	return nothing()
}
