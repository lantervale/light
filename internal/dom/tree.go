package dom

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/css"

	"github.com/lantervale/light/internal/web"
)

// The names scripts know the nodes that are no element by.
const (
	fragmentTag = "#document-fragment"
	textTag     = "#text"
	commentTag  = "#comment"
	doctypeTag  = "#doctype"
	documentTag = "#document"
	cdataTag    = "#cdata-section"
)

// Scripts read the page's tree as its HTML was read (web.Tree): every
// element, text and comment, shown or not. What ui shows of a node is
// asked of the tree, which keeps it like the node.

// element is the first child of n that is an element of that tag, nil
// for none.
func element(n *html.Node, tag atom.Atom) *html.Node {
	if n == nil {
		return nil
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (tag == 0 || c.DataAtom == tag) && c.Namespace == "" {
			return c
		}
	}
	return nil
}

// top is the page's html element, nil for a page left without one.
func (r *Runtime) top() *html.Node { return element(r.tree.Root(), 0) }

func (r *Runtime) head() *html.Node { return element(r.top(), atom.Head) }

func (r *Runtime) body() *html.Node { return element(r.top(), atom.Body) }

// kids are the children of n, its elements only when asked.
func kids(n *html.Node, elements bool) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if !elements || c.Type == html.ElementNode {
			out = append(out, c)
		}
	}
	return out
}

// walk visits the nodes under root in document order, root left out,
// until visit says to stop.
func walk(root *html.Node, visit func(*html.Node) bool) bool {
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if !visit(c) || !walk(c, visit) {
			return false
		}
	}
	return true
}

// within reports whether n is root or under it.
func within(root, n *html.Node) bool {
	for ; n != nil; n = n.Parent {
		if n == root {
			return true
		}
	}
	return false
}

// inside reports whether u is root or under it.
func inside(root, u *ui.Node) bool {
	for ; u != nil; u = u.Parent() {
		if u == root {
			return true
		}
	}
	return false
}

// connected reports whether what u shows is in the page.
func (r *Runtime) connected(u *ui.Node) bool {
	n := r.tree.Of(u)
	return n != nil && r.tree.Connected(n)
}

// tagOf is the name the page gave u's element.
func (r *Runtime) tagOf(u *ui.Node) string { return r.tree.Tag(u) }

// nameOf is the name scripts know n by: an element's tag, after its
// namespace when it is not HTML's ("svg:path"), in braces when it is
// neither SVG's nor MathML's ("{urn:x}item", "{}item" for none), and
// after a colon for one of HTML whose own name has one (":a:b"). A
// processing instruction's is its target after a question mark, and a
// document a script made says whether createDocument made it, which
// makes it an XMLDocument.
func (r *Runtime) nameOf(n *html.Node) string {
	switch n.Type { //nolint:exhaustive // the tree has no other
	case html.TextNode:
		if n.Namespace == web.CDATASection {
			return cdataTag
		}
		return textTag
	case html.CommentNode:
		if n.Namespace == web.Instruction {
			return "?" + web.InstructionTarget(n)
		}
		return commentTag
	case html.DoctypeNode:
		return doctypeTag
	case html.DocumentNode:
		switch d := r.docs[n]; {
		case n == r.tree.Root():
			return documentTag
		case d != nil && d.created:
			return documentTag + ":xml"
		case d != nil:
			return documentTag + ":html"
		}
		return fragmentTag
	}
	switch n.Namespace {
	case "":
		if strings.Contains(n.Data, ":") {
			return ":" + n.Data
		}
		return n.Data
	case "svg", "math":
		return n.Namespace + ":" + n.Data
	case web.NoNamespace:
		return "{}" + n.Data
	}
	return "{" + n.Namespace + "}" + n.Data
}

func (r *Runtime) document(arguments) (any, error) {
	return []int{r.ref(r.top()), r.ref(r.head()), r.ref(r.body())}, nil
}

// top level are the children of the document: the doctype, the html
// element and the comments around them.
func (r *Runtime) topLevel(arguments) (any, error) { return r.refs(kids(r.tree.Root(), false)), nil }

func (r *Runtime) tag(n *html.Node, _ arguments) (any, error) { return r.nameOf(n), nil }

// parent is the number of n's parent, 0 for none, and "document" for a
// child of the document.
func (r *Runtime) parent(n *html.Node, _ arguments) (any, error) {
	if n.Parent != nil && n.Parent == r.tree.Root() {
		return r.pageOr(nil, "document"), nil
	}
	return r.ref(n.Parent), nil
}

func (r *Runtime) children(n *html.Node, a arguments) (any, error) {
	return r.refs(kids(n, a.flag(1))), nil
}

// child is the first child of n, or its last when asked for -1.
func (r *Runtime) child(n *html.Node, a arguments) (any, error) {
	c, step := n.FirstChild, func(c *html.Node) *html.Node { return c.NextSibling }
	if a.number(1) < 0 {
		c, step = n.LastChild, func(c *html.Node) *html.Node { return c.PrevSibling }
	}
	for ; c != nil; c = step(c) {
		if !a.flag(2) || c.Type == html.ElementNode {
			return r.ref(c), nil
		}
	}
	return 0, nil
}

// sibling is the node after n among its parent's, before it for -1.
func (r *Runtime) sibling(n *html.Node, a arguments) (any, error) {
	step := func(c *html.Node) *html.Node { return c.NextSibling }
	if a.number(1) < 0 {
		step = func(c *html.Node) *html.Node { return c.PrevSibling }
	}
	for c := step(n); c != nil; c = step(c) {
		if !a.flag(2) || c.Type == html.ElementNode {
			return r.ref(c), nil
		}
	}
	return 0, nil
}

// isConnected reports whether n is in a document: the page, or one a
// script made.
func (r *Runtime) isConnected(n *html.Node, _ arguments) (any, error) {
	return r.tree.Connected(n) || r.docs[lineage(n)[0]] != nil, nil
}

func (r *Runtime) contains(n *html.Node, a arguments) (any, error) {
	other, err := r.node(a, 1)
	if err != nil {
		return false, nil //nolint:nilerr // a node light does not know is in nothing
	}
	return within(n, other), nil
}

// The bits compareDocumentPosition answers with.
const (
	disconnected = 1
	preceding    = 2
	following    = 4
	containing   = 8
	containedBy  = 16
	// apart is what two nodes of different trees are: which is first
	// is the engine's to say, the same way each time.
	apart = disconnected | 32
)

// compare says where the second node is from the first.
func (r *Runtime) compare(n *html.Node, a arguments) (any, error) {
	other, err := r.node(a, 1)
	if err != nil {
		return apart | preceding, nil //nolint:nilerr // a node light does not know is nowhere
	}
	switch {
	case within(other, n):
		return containing | preceding, nil
	case within(n, other):
		return containedBy | following, nil
	}
	mine, theirs := lineage(n), lineage(other)
	if mine[0] != theirs[0] {
		if r.ids[n] < r.ids[other] {
			return apart | following, nil
		}
		return apart | preceding, nil
	}
	at := 0
	for mine[at] == theirs[at] {
		at++
	}
	for c := mine[at].NextSibling; c != nil; c = c.NextSibling {
		if c == theirs[at] {
			return following, nil
		}
	}
	return preceding, nil
}

// lineage is n and what it is in, from the outermost.
func lineage(n *html.Node) []*html.Node {
	var up []*html.Node
	for ; n != nil; n = n.Parent {
		up = append(up, n)
	}
	for i, j := 0, len(up)-1; i < j; i, j = i+1, j-1 {
		up[i], up[j] = up[j], up[i]
	}
	return up
}

var errTooManyNodes = fmt.Errorf("light: the page's scripts made more than %d nodes", maxNodes)

// create is a new element of a tag, in a namespace when one is named,
// or a fragment.
func (r *Runtime) create(a arguments) (any, error) {
	if r.made++; r.made > maxNodes {
		return nil, errTooManyNodes
	}
	tag := a.text(0)
	if tag == fragmentTag {
		return r.own(&html.Node{Type: html.DocumentNode}, a, 2), nil
	}
	n := r.tree.Element(tag, a.text(1))
	if prefix := a.text(3); prefix != "" {
		r.prefixes[n] = prefix
	}
	return r.own(n, a, 2), nil
}

func (r *Runtime) createText(a arguments) (any, error) {
	if r.made++; r.made > maxNodes {
		return nil, errTooManyNodes
	}
	return r.own(&html.Node{Type: html.TextNode, Data: a.text(0)}, a, 1), nil
}

func (r *Runtime) createComment(a arguments) (any, error) {
	if r.made++; r.made > maxNodes {
		return nil, errTooManyNodes
	}
	return r.own(&html.Node{Type: html.CommentNode, Data: a.text(0)}, a, 1), nil
}

// insert puts the second node in the first before the third, at its
// end for none: a node of another document becomes one of the first's.
// A script that enters the page with it runs.
func (r *Runtime) insert(parent *html.Node, a arguments) (any, error) {
	child, err := r.node(a, 1)
	if err != nil {
		return nil, err
	}
	before := r.nodes[int(a.number(2))]
	switch {
	case before != nil && before.Parent != parent:
		//nolint:staticcheck // a DOMException's message, as Chrome words it
		return nil, errors.New("NotFoundError: The node before which the new node is to be inserted is not a child " +
			"of this node.")
	case within(child, parent):
		//nolint:revive,staticcheck // a DOMException's message, as Chrome words it
		return nil, errors.New("HierarchyRequestError: The new child element contains the parent.")
	}
	r.receive(child)
	if to := r.home(parent); to != r.home(child) {
		r.adopt(child, to)
	}
	r.settle(r.tree.Insert(parent, child, before))
	return nothing()
}

// settle runs the scripts among what entered the page, those that
// never ran.
func (r *Runtime) settle(entered []*html.Node) {
	for _, n := range entered {
		if n.DataAtom == atom.Script && !r.ran[n] {
			r.ran[n] = true
			r.added(n)
		}
	}
}

func (r *Runtime) remove(n *html.Node, _ arguments) (any, error) {
	r.tree.Remove(n)
	return nothing()
}

func (r *Runtime) clone(n *html.Node, a arguments) (any, error) {
	if r.made++; r.made > maxNodes {
		return nil, errTooManyNodes
	}
	c := r.copyOf(n, a.flag(1))
	if d := r.home(n); d != nil {
		r.adopt(c, d)
	}
	return r.ref(c), nil
}

// copyOf is a copy of n, of what it holds too when deep.
func (r *Runtime) copyOf(n *html.Node, deep bool) *html.Node {
	c := &html.Node{Type: n.Type, Data: n.Data, DataAtom: n.DataAtom, Namespace: n.Namespace,
		Attr: append([]html.Attribute(nil), n.Attr...)}
	if n.Type == html.ElementNode && n.DataAtom == atom.Template && n.Namespace == "" {
		c = r.tree.Element(n.Data, "")
		c.Attr = append([]html.Attribute(nil), n.Attr...)
	}
	if prefix := r.prefixOf(n); prefix != "" {
		r.prefixes[c] = prefix
	}
	if r.ran[n] {
		r.ran[c] = true
	}
	if !deep {
		return c
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.AppendChild(r.copyOf(child, true))
	}
	if from, to := r.tree.Content(n), r.tree.Content(c); from != nil && to != nil {
		for child := from.FirstChild; child != nil; child = child.NextSibling {
			to.AppendChild(r.copyOf(child, true))
		}
	}
	return c
}

// content is what a template holds, as a fragment.
func (r *Runtime) content(n *html.Node, _ arguments) (any, error) {
	return r.ref(r.tree.Content(n)), nil
}

// textOf is the text of n: its own, or that of the text nodes under it.
func textOf(n *html.Node) string {
	switch {
	case n.Type == html.TextNode || n.Type == html.CommentNode || n.Type == html.DoctypeNode:
		return n.Data
	case n.FirstChild != nil && n.FirstChild == n.LastChild && n.FirstChild.Type == html.TextNode:
		return n.FirstChild.Data
	}
	var text strings.Builder
	walk(n, func(c *html.Node) bool {
		if c.Type == html.TextNode {
			text.WriteString(c.Data)
		}
		return true
	})
	return text.String()
}

func (r *Runtime) text(n *html.Node, _ arguments) (any, error) { return textOf(n), nil }

func (r *Runtime) setText(n *html.Node, a arguments) (any, error) {
	text := a.text(1)
	switch {
	case n.Type == html.TextNode || n.Type == html.CommentNode:
		r.tree.SetData(n, text)
	case text != "" && n.FirstChild != nil && n.FirstChild == n.LastChild && n.FirstChild.Type == html.TextNode &&
		r.ids[n.FirstChild] == 0:
		r.tree.SetData(n.FirstChild, text)
	default:
		r.empty(n)
		if text != "" {
			r.tree.Insert(n, &html.Node{Type: html.TextNode, Data: text}, nil)
		}
	}
	return nothing()
}

// empty takes out what n holds, and lets go of what no script holds.
func (r *Runtime) empty(n *html.Node) {
	for c := n.LastChild; c != nil; c = n.LastChild {
		r.tree.Remove(c)
		r.forget(c)
	}
}

func (r *Runtime) setHTML(n *html.Node, a arguments) (any, error) {
	into := n
	if content := r.tree.Content(n); content != nil {
		into = content
	}
	made, err := r.markupIn(a.text(1), n)
	if err != nil {
		return nil, err
	}
	r.empty(into)
	for _, c := range made {
		r.tree.Insert(into, c, nil)
	}
	return nothing()
}

// fragment is markup read as the children of an element of a tag, or
// as what the node a script names holds.
func (r *Runtime) fragment(a arguments) (any, error) {
	if within := r.nodes[int(a.number(2))]; within != nil {
		made, err := r.markupIn(a.text(0), within)
		return r.refs(made), err
	}
	return r.refs(r.markup(a.text(0), a.text(1))), nil
}

// markup is markup read as the children of an element of the tag
// within. Its scripts never run, as a browser has it.
func (r *Runtime) markup(text, within string) []*html.Node {
	made := r.tree.Fragment(text, within)
	never := func(c *html.Node) bool {
		if c.Type == html.ElementNode && c.DataAtom == atom.Script {
			r.ran[c] = true
		}
		return true
	}
	for _, n := range made {
		never(n)
		walk(n, never)
	}
	return made
}

// read is a selector read once.
func (r *Runtime) read(selector string) (selectors, error) {
	if parsed, known := r.selectors[selector]; known {
		return parsed, nil
	}
	parsed, err := parseSelectors(selector)
	if err != nil {
		return nil, errors.New("SyntaxError: " + err.Error())
	}
	if len(r.selectors) >= maxSelectors {
		clear(r.selectors)
	}
	r.selectors[selector] = parsed
	return parsed, nil
}

// maxSelectors is how many selectors are kept read.
const maxSelectors = 512

func (r *Runtime) matcher(scope *html.Node) matcher {
	return matcher{
		checked: func(n *html.Node) bool {
			if u := r.tree.Node(n); u != nil {
				return r.doc.Checked(u)
			}
			_, on := r.tree.Attr(n, checked)
			return on
		},
		focused: func(n *html.Node) bool {
			u := r.tree.Node(n)
			return u != nil && r.doc.Focused(u)
		},
		value: func(n *html.Node) string {
			if u := r.tree.Node(n); u != nil && r.takesText(u) {
				return r.doc.Value(u)
			}
			value, _ := r.tree.Attr(n, "value")
			return value
		},
		said:     r.validity,
		target:   r.targeted,
		document: func(n *html.Node) bool { return n == r.tree.Root() || r.docs[n] != nil },
		scope:    scope,
		shown: func(n *html.Node, pseudo string) bool {
			u := r.tree.Node(n)
			return u != nil && shownSelectors()[pseudo].Matches(u)
		},
	}
}

// shownSelectors are ember's selectors of the pseudo-classes it keeps,
// by name.
//
//nolint:gochecknoglobals // read once
var shownSelectors = sync.OnceValue(func() map[string]css.Selector {
	all := map[string]css.Selector{}
	for _, pseudo := range append([]string{opened, "user-valid", "user-invalid"}, shownStates...) {
		parsed, err := css.ParseSelectors(":" + pseudo)
		if err != nil || len(parsed) != 1 {
			panic("ember reads no :" + pseudo)
		}
		all[pseudo] = parsed[0]
	}
	return all
})

// query finds the elements under a node that match a selector, in the
// whole page for no node: the first, or all of them.
func (r *Runtime) query(a arguments) (any, error) {
	root := firstElement(r.tree.Root())
	whole := a.number(0) == 0
	if !whole {
		var err error
		if root, err = r.node(a, 0); err != nil {
			return nil, err
		}
		if kept := r.keeper(root); kept != r {
			return kept.askedByKin(r.asked, a)
		}
	}
	parsed, err := r.read(a.text(1))
	if err != nil {
		return nil, err
	}
	all := a.flag(2)
	var found []*html.Node
	if root == nil {
		return r.found(found, all), nil
	}
	m := r.matcher(root)
	if whole && parsed.matches(root, m) {
		found = append(found, root)
	}
	if all || len(found) == 0 {
		walk(root, func(n *html.Node) bool {
			if n.Type == html.ElementNode && parsed.matches(n, m) {
				found = append(found, n)
			}
			return all || len(found) == 0
		})
	}
	return r.found(found, all), nil
}

// found is what a query found: all of it, or the first, 0 for none.
func (r *Runtime) found(found []*html.Node, all bool) any {
	switch {
	case all:
		return r.refs(found)
	case len(found) == 0:
		return 0
	}
	return r.ref(found[0])
}

func (r *Runtime) matches(n *html.Node, a arguments) (any, error) {
	parsed, err := r.read(a.text(1))
	if err != nil {
		return nil, err
	}
	return parsed.matches(n, r.matcher(n)), nil
}

func (r *Runtime) closest(n *html.Node, a arguments) (any, error) {
	parsed, err := r.read(a.text(1))
	if err != nil {
		return nil, err
	}
	m := r.matcher(n)
	for at := n; at != nil; at = at.Parent {
		if parsed.matches(at, m) {
			return r.ref(at), nil
		}
	}
	return 0, nil
}

func (r *Runtime) byID(a arguments) (any, error) {
	id := a.text(0)
	var found *html.Node
	if id != "" {
		walk(r.tree.Root(), func(n *html.Node) bool {
			if n.Type == html.ElementNode && idOf(n) == id {
				found = n
			}
			return found == nil
		})
	}
	return r.ref(found), nil
}

// held are the numbers of the nodes that left the page, and of those
// in it.
func (r *Runtime) held(arguments) (any, error) {
	detached, attached := []int{}, []int{}
	for id, n := range r.nodes {
		if r.keeper(n) != r {
			continue
		}
		if r.tree.Connected(n) {
			attached = append(attached, id)
		} else {
			detached = append(detached, id)
		}
	}
	return []any{detached, attached}, nil
}

// let forgets the nodes no script holds any more, and with them the
// trees out of the page that no script holds a node of.
func (r *Runtime) let(a arguments) (any, error) {
	ids, _ := a[0].([]any)
	tops := map[*html.Node]*Runtime{}
	for _, id := range ids {
		number, ok := id.(float64)
		n := r.nodes[int(number)]
		if !ok || n == nil {
			continue
		}
		kept := r.keeper(n)
		r.release(int(number))
		if top := lineage(n)[0]; top != kept.tree.Root() {
			tops[top] = kept
		}
	}
	for top, kept := range tops {
		kept.forget(top)
	}
	return nothing()
}

// forget lets go of a tree that is out of the page, unless a script
// holds a node of it: what ui has of its nodes, and whether its
// scripts ran.
func (r *Runtime) forget(top *html.Node) {
	free := func(n *html.Node) bool { return r.ids[n] == 0 }
	if !free(top) || !walk(top, free) {
		return
	}
	drop := func(n *html.Node) bool {
		delete(r.ran, n)
		delete(r.owners, n)
		delete(r.docs, n)
		delete(r.prefixes, n)
		if content := r.tree.Content(n); content != nil {
			r.forget(content)
		}
		r.tree.Forget(n)
		return true
	}
	drop(top)
	walk(top, drop)
}
