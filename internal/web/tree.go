package web

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/identity"
)

// Script is one script element of a page.
type Script struct {
	// Source is its text: its own, or what its src gave.
	Source string
	// Address is where its text came from: its src after its redirects,
	// else the page.
	Address string
	// Node is its element in the Tree.
	Node *html.Node
	// Deferred is a script that runs after those the page has in line
	// (defer, async, a module).
	Deferred bool
	// Module is a module script: its text is its own, or what Address
	// names, which is left for the page's scripts to fetch as a module
	// is. Async is one that runs as soon as it and what it imports came.
	Module, Async bool
	// ImportMap is an import map, which Source holds.
	ImportMap bool
	// Skipped is why it does not run, "" for one that does.
	Skipped string
}

// Tree is a page as its scripts read and change it: the tree its HTML
// was read into, every element, text and comment of it where the page
// wrote it, and what ui shows of it. The tree is the page; the
// Document holds only the nodes ui has an element for, each made from
// its node of the tree and kept like it, so what a script changes is
// changed in the tree and shows from there.
type Tree struct {
	b   *builder
	doc *ui.Document
	// root is the document node of the tree.
	root *html.Node
	// shown is what ui has of a node, and of the node of what ui has.
	shown map[*html.Node]*ui.Node
	of    map[*ui.Node]*html.Node
	// stale are the svg elements to draw again (Redraw).
	stale []*html.Node
	// head holds, out of the Document, an element for each of those the
	// developer tools list and ui shows nothing of: the page's title,
	// metas, links, styles and scripts.
	head *ui.Node
	// contents is what each template holds.
	contents map[*html.Node]*html.Node
	// Activate, when set, is asked to follow a link in place of the
	// link itself: follow opens or saves what it names.
	Activate func(a *ui.Node, follow func())
	// Heard, when set, is told what ui tells a field's handlers: "input",
	// "change" or "submit", as it happens.
	Heard func(field *ui.Node, event string)
	// Submit, when set, is asked to send a form whose submitter was
	// activated in place of the form itself, and Enter in a field of a
	// form is left to Heard: Send sends it.
	Submit func(form, submitter *ui.Node)
	// handlers is whether an element of the page has a script's handler.
	handlers bool
}

// Scripted reports whether the page has anything for a script engine to
// run: a script, or a handler on an element.
func (p *Page) Scripted() bool {
	return p.Tree != nil && (len(p.Scripts) > 0 || p.Tree.handlers)
}

// kept are the elements ui shows nothing of that the developer tools
// list under the page's head.
//
//nolint:gochecknoglobals,exhaustive // the elements kept, not every element there is
var kept = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Link: true, atom.Meta: true, atom.Title: true, atom.Base: true,
}

// Root is the document node of the tree: its children are the page's
// doctype, its html element and the comments around them.
func (t *Tree) Root() *html.Node { return t.root }

// Head is the element holding what is listed out of sight.
func (t *Tree) Head() *ui.Node { return t.head }

// Node is what ui has of n, nil for a node it has nothing of yet.
func (t *Tree) Node(n *html.Node) *ui.Node { return t.shown[n] }

// Of is the node of the tree that u is ui's element or text of, nil
// for one that is none's.
func (t *Tree) Of(u *ui.Node) *html.Node { return t.of[u] }

// Tag is the name the page gave u's element, a text node's "".
func (t *Tree) Tag(u *ui.Node) string {
	switch n := t.of[u]; {
	case u.IsText():
		return ""
	case n != nil:
		return n.Data
	}
	return u.Tag().String()
}

// Content is what a template element holds, nil for another element.
func (t *Tree) Content(n *html.Node) *html.Node { return t.contents[n] }

// Element is a new element of that tag, in a namespace ("svg", "math")
// or in HTML's for "".
func (t *Tree) Element(tag, namespace string) *html.Node {
	n := &html.Node{Type: html.ElementNode, Data: tag, Namespace: namespace}
	if namespace == "" {
		n.DataAtom = atom.Lookup([]byte(tag))
	}
	if n.DataAtom == atom.Template {
		t.contents[n] = &html.Node{Type: html.DocumentNode}
	}
	return n
}

// Fragment is markup read as the children of an element of the tag
// within. Its scripts are not the page's.
func (t *Tree) Fragment(markup, within string) []*html.Node {
	context := &html.Node{Type: html.ElementNode, Data: within, DataAtom: atom.Lookup([]byte(within))}
	nodes, err := html.ParseFragmentWithOptions(strings.NewReader(cut(markup)), context,
		html.ParseOptionEnableScripting(true))
	if err != nil {
		return nil
	}
	order := writtenIn(markup)
	for _, n := range nodes {
		order.restore(n)
		t.apart(n)
	}
	return nodes
}

// formatting are the elements whose attributes html's parser sorts by
// name as it reads them.
//
//nolint:gochecknoglobals,exhaustive // the elements sorted, not every element there is
var formatting = map[atom.Atom]bool{
	atom.A: true, atom.B: true, atom.Big: true, atom.Code: true, atom.Em: true, atom.Font: true, atom.I: true,
	atom.Nobr: true, atom.S: true, atom.Small: true, atom.Strike: true, atom.Strong: true, atom.Tt: true,
	atom.U: true,
}

// attributeOrder is the order a page wrote the attributes of its formatting
// elements in, by what the parser makes of each.
type attributeOrder map[string][]html.Attribute

// signature is a tag and its attributes, whatever their order.
func signature(tag string, attributes []html.Attribute) string {
	parts := make([]string, len(attributes))
	for i, a := range attributes {
		parts[i] = a.Key + "\x00" + a.Val
	}
	slices.Sort(parts)
	return tag + "\x01" + strings.Join(parts, "\x01")
}

// writtenIn reads the start tags of markup for the order of their
// attributes.
func writtenIn(markup string) attributeOrder {
	if !strings.Contains(markup, "<") {
		return nil
	}
	order := attributeOrder{}
	for z := html.NewTokenizer(strings.NewReader(markup)); ; {
		kind := z.Next()
		if kind == html.ErrorToken {
			return order
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		name, more := z.TagName()
		if !more || !formatting[atom.Lookup(name)] {
			continue
		}
		token := html.Token{Data: string(name)}
		for more {
			var key, value []byte
			key, value, more = z.TagAttr()
			token.Attr = append(token.Attr, html.Attribute{Key: string(key), Val: string(value)})
		}
		if len(token.Attr) < 2 {
			continue
		}
		if at := signature(token.Data, token.Attr); order[at] == nil {
			order[at] = token.Attr
		}
	}
}

// restore gives the formatting elements under n their attributes in
// the order the page wrote them.
func (w attributeOrder) restore(n *html.Node) {
	if len(w) == 0 {
		return
	}
	if n.Type == html.ElementNode && formatting[n.DataAtom] && n.Namespace == "" && len(n.Attr) > 1 {
		if order := w[signature(n.Data, n.Attr)]; order != nil {
			n.Attr = slices.Clone(order)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.restore(c)
	}
}

// apart sets what the templates under n hold apart from them, as
// scripts find it: in their content.
func (t *Tree) apart(n *html.Node) {
	if n.Type == html.ElementNode && n.DataAtom == atom.Template && n.Namespace == "" && t.contents[n] == nil {
		content := &html.Node{Type: html.DocumentNode}
		for c := n.FirstChild; c != nil; c = n.FirstChild {
			n.RemoveChild(c)
			content.AppendChild(c)
		}
		t.contents[n] = content
		t.apart(content)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		t.apart(c)
	}
}

// bind makes root the tree of doc, whose elements are built from it.
func (t *Tree) bind(root *html.Node, doc *ui.Document) {
	t.root, t.doc = root, doc
	t.apart(root)
	var top, head, body *html.Node
	for c := root.FirstChild; c != nil && top == nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			top = c
		}
	}
	if top == nil {
		return
	}
	for c := top.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type != html.ElementNode:
		case c.DataAtom == atom.Head && head == nil:
			head = c
		case c.DataAtom == atom.Body && body == nil:
			body = c
		}
	}
	t.keep(top, doc.HTML())
	t.b.attributes(top, doc.HTML())
	if head != nil {
		t.keep(head, t.head)
	}
	if body != nil {
		t.keep(body, doc.Body())
		t.b.attributes(body, doc.Body())
		t.b.children(body, doc.Body())
	}
	t.enter(top, nil)
}

func (t *Tree) keep(n *html.Node, u *ui.Node) *ui.Node {
	t.shown[n], t.of[u] = u, n
	return u
}

// drawn reports whether ui has an element or a text for n among those
// of its parent's.
func (t *Tree) drawn(n *html.Node) bool {
	switch n.Type { //nolint:exhaustive // a comment, a doctype and a fragment show nothing
	case html.TextNode:
		return true
	case html.ElementNode:
		return !hidden(n) && n.DataAtom != atom.Html && n.DataAtom != atom.Body && n.DataAtom != atom.Head
	}
	return false
}

// hidden reports whether n is an element ui builds nothing for once
// scripts run: those left out and what is drawn in them, those listed
// out of sight, a noscript.
func hidden(n *html.Node) bool {
	return dropped[n.DataAtom] || kept[n.DataAtom] || n.DataAtom == atom.Noscript ||
		n.Namespace == svgNamespace && !pictured(n)
}

// holds reports whether ui shows the children of n under what it has
// of n.
func (t *Tree) holds(n *html.Node) bool {
	return n.Type == html.ElementNode && !pictured(n) && (t.drawn(n) || t.shown[n] == t.doc.Body())
}

// Make is what ui has of n, made with what n holds the first time: nil
// for a node ui has nothing for.
func (t *Tree) Make(n *html.Node) *ui.Node {
	t.b.count = 0
	return t.made(n)
}

// made is what ui has of n, made within the elements a page is built
// up to.
func (t *Tree) made(n *html.Node) *ui.Node {
	if u := t.shown[n]; u != nil || !t.drawn(n) {
		return u
	}
	if n.Type == html.TextNode {
		return t.keep(n, newText(n.Data))
	}
	return t.b.element(n)
}

// Forget lets go of what ui has of n, a node that left the page for
// good.
func (t *Tree) Forget(n *html.Node) {
	if u := t.shown[n]; u != nil {
		delete(t.of, u)
		delete(t.shown, n)
	}
	delete(t.contents, n)
}

func newText(text string) *ui.Node {
	holder := ui.Div(ui.Text(text))
	node := holder.Children()[0]
	holder.Remove(node)
	return node
}

// listed is the element the developer tools list for n, one of those
// kept: its attributes, and its text.
func (t *Tree) listed(n *html.Node) *ui.Node {
	u := t.shown[n]
	if u == nil {
		u = t.keep(n, ui.Div())
		t.b.attributes(n, u)
		t.retext(n)
	}
	return u
}

// retext gives the element listed for n the text n holds now.
func (t *Tree) retext(n *html.Node) {
	u := t.shown[n]
	if u == nil || n.Type != html.ElementNode || !kept[n.DataAtom] {
		return
	}
	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			text.WriteString(c.Data)
		}
	}
	if text.Len() > 0 || len(u.Children()) > 0 {
		u.SetText(text.String())
	}
}

// Connected reports whether n is in the page.
func (t *Tree) Connected(n *html.Node) bool {
	for ; n != nil; n = n.Parent {
		if n == t.root {
			return true
		}
	}
	return false
}

// enter lists what entered the page with n among what is kept out of
// sight, and adds it to entered.
func (t *Tree) enter(n *html.Node, entered []*html.Node) []*html.Node {
	if n.Type != html.ElementNode {
		return entered
	}
	if kept[n.DataAtom] && n.Namespace == "" {
		if u := t.listed(n); u.Parent() != t.head {
			t.head.Append(u)
		}
		return append(entered, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		entered = t.enter(c, entered)
	}
	return entered
}

// leave takes what left the page with n off what is listed.
func (t *Tree) leave(n *html.Node) {
	if n.Type != html.ElementNode {
		return
	}
	if kept[n.DataAtom] && n.Namespace == "" {
		if u := t.shown[n]; u != nil && u.Parent() == t.head {
			t.head.Remove(u)
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		t.leave(c)
	}
}

// Insert puts child in parent before one of its children, at its end
// for nil, and shows it there when ui shows parent's children. It
// answers the elements kept out of sight that entered the page with
// it: its scripts among them.
func (t *Tree) Insert(parent, child, before *html.Node) []*html.Node {
	if before == child {
		before = child.NextSibling
	}
	t.Remove(child)
	parent.InsertBefore(child, before)
	t.redraw(parent)
	if into := t.shown[parent]; into != nil && t.holds(parent) {
		if u := t.Make(child); u != nil {
			place(into, u, t.next(into, child.NextSibling))
		}
	}
	t.retext(parent)
	if !t.Connected(parent) {
		return nil
	}
	return t.enter(child, nil)
}

// next is what ui has, under into, of the first of n and the nodes
// after it that it has anything of there.
func (t *Tree) next(into *ui.Node, n *html.Node) *ui.Node {
	for ; n != nil; n = n.NextSibling {
		if u := t.shown[n]; u != nil && u.Parent() == into {
			return u
		}
	}
	return nil
}

// place puts child in parent before one of its children, at its end
// for none.
func place(parent, child, before *ui.Node) {
	if before == nil {
		parent.Append(child)
		return
	}
	after := slices.Clone(parent.Children()[before.Index():])
	after = slices.DeleteFunc(after, func(n *ui.Node) bool { return n == child })
	parent.Append(child)
	parent.Append(after...)
}

// Remove takes child out of its parent, and out of what ui shows.
func (t *Tree) Remove(child *html.Node) {
	parent := child.Parent
	if parent == nil {
		return
	}
	if u := t.shown[child]; u != nil && u.Parent() != nil && u.Parent() != t.head {
		u.Parent().Remove(u)
	}
	if t.Connected(parent) {
		t.leave(child)
	}
	parent.RemoveChild(child)
	t.redraw(parent)
	t.retext(parent)
}

// SetData gives a text node or a comment its text.
func (t *Tree) SetData(n *html.Node, data string) {
	n.Data = data
	t.redraw(n.Parent)
	if u := t.shown[n]; u != nil && n.Type == html.TextNode {
		u.SetText(data)
	}
	if n.Parent != nil {
		t.retext(n.Parent)
	}
}

// called reports whether a is the attribute scripts know by name: its
// key, after its namespace when it has one (xlink:href).
func called(a html.Attribute, name string) bool {
	if a.Namespace == "" {
		return a.Key == name
	}
	return len(name) == len(a.Namespace)+1+len(a.Key) && strings.HasPrefix(name, a.Namespace+":") &&
		strings.HasSuffix(name, a.Key)
}

// Attr is n's attribute name, and whether it has one. What ui sets
// itself as the page is used, a details' open, is read from it.
func (t *Tree) Attr(n *html.Node, name string) (string, bool) {
	if name == "open" {
		if u := t.shown[n]; u != nil {
			return u.Attr(name)
		}
	}
	for _, a := range n.Attr {
		if called(a, name) {
			return a.Val, true
		}
	}
	return "", false
}

// SetAttr sets an attribute of n, and of what ui has of it.
func (t *Tree) SetAttr(n *html.Node, name, value string) {
	at := slices.IndexFunc(n.Attr, func(a html.Attribute) bool { return called(a, name) })
	if at < 0 {
		n.Attr = append(n.Attr, html.Attribute{Key: name, Val: value})
	} else {
		n.Attr[at].Val = value
	}
	t.redraw(n)
	if u := t.shown[n]; u != nil {
		t.b.attribute(n, u, name, value)
	}
}

// RemoveAttr takes an attribute off n, and off what ui has of it.
func (t *Tree) RemoveAttr(n *html.Node, name string) {
	n.Attr = slices.DeleteFunc(n.Attr, func(a html.Attribute) bool { return called(a, name) })
	t.redraw(n)
	u := t.shown[n]
	switch {
	case u == nil:
	case n.DataAtom == atom.Img && (name == "src" || name == attrSrcset || name == attrSizes):
		t.b.attribute(n, u, name, "")
	case name == "id" || name == "class" || name == "style":
		u.SetAttr(name, "")
	default:
		u.RemoveAttr(name)
	}
}

// make is the element of a tag: ui's, a div for one ui has none for,
// and an a that follows its href as it is when clicked.
func (b *builder) make(tag string) *ui.Node {
	e := build(tag)
	if tag == tagInput || tag == tagButton {
		b.submits(e, tag)
	}
	if b.tree == nil {
		return e
	}
	switch tag {
	case "a":
		b.live(e)
	case atom.Input.String(), atom.Textarea.String(), atom.Select.String():
		b.heard(e)
	}
	return e
}

// heard has what ui tells field's handlers told to the page's scripts.
func (b *builder) heard(field *ui.Node) {
	for _, told := range []struct {
		event ui.EventType
		name  string
	}{{ui.EventInput, atom.Input.String()}, {ui.EventChange, "change"}, {ui.EventSubmit, "submit"}} {
		ui.On(field, told.event, func() {
			if b.tree.Heard != nil {
				b.tree.Heard(field, told.name)
			}
		})
	}
}

// live makes a, an a element, a link to whatever its href is when it is
// clicked or pointed at.
func (b *builder) live(a *ui.Node) {
	address := func() string {
		href, _ := a.Attr("href")
		if href == "" || strings.HasPrefix(href, "#") {
			return ""
		}
		return b.resolve(href)
	}
	follow := func() {
		to := address()
		if to == "" {
			return
		}
		if name, saved := a.Attr("download"); saved {
			b.links.Save(to, name)
			return
		}
		b.links.Open(to)
	}
	ui.OnClick(func() {
		if b.tree.Activate != nil {
			b.tree.Activate(a, follow)
			return
		}
		follow()
	}).Apply(a)
	ui.OnPointerEnter(func() { b.links.Over(address()) }).Apply(a)
	ui.OnPointerLeave(func() { b.links.Over("") }).Apply(a)
}

// The kinds of script elements that are scripts.
const (
	classicScript = "classic"
	moduleScript  = "module"
	importMap     = "importmap"
)

// scriptKind is the kind of script a script element is, "" for one
// that is none: one of another type holds data, and one marked nomodule
// is for the browsers that read no module.
func scriptKind(n *html.Node) string {
	switch kind := strings.ToLower(strings.TrimSpace(attr(n, "type"))); kind {
	case "", "text/javascript", "application/javascript", "text/ecmascript", "application/ecmascript",
		"application/x-javascript", "text/jscript", "text/livescript":
		if _, fallback := lookup(n, "nomodule"); fallback {
			return ""
		}
		return classicScript
	case moduleScript, importMap:
		return kind
	}
	return ""
}

// Fetched reports whether address is one light fetches: of http or of
// https.
func Fetched(address string) bool { return opened(address) }

// Script gets the script at address for a page: its text, where it
// came from, and why it is not run when it could not be read.
func (l *Loader) Script(ctx context.Context, address string) (string, string, string) {
	answer, err := l.Client.Get(asking(ctx, identity.Script), address, identity.Accept(identity.Script))
	switch {
	case err != nil:
		return "", address, err.Error()
	case !answer.OK():
		return "", answer.Address, fmt.Sprintf("the server answered %d %s", answer.Status,
			http.StatusText(answer.Status))
	}
	return cut(answer.Code()), answer.Address, ""
}
