package inspector

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"
)

const (
	// indent is how far a level of the tree moves its rows, and hang how
	// far the lines a long row wraps to stand right of its tag.
	indent = 12
	hang   = 14
	// short is the longest text an element shows on its own row, long
	// the longest a text node shows, and wide the longest value of an
	// attribute, in characters.
	short = 80
	long  = 300
	wide  = 160
)

// device is the attribute ui keeps on a Document's root for its own
// sheets, which is none of the page's.
const device = "data-input-device"

// item is one node of the page as the tree shows it: its row, and under
// it its children and its closing tag while it is open.
type item struct {
	n      *ui.Node
	parent *item
	depth  int
	// doctype is the line the tree begins with, which is no node.
	doctype bool
	// expandable is whether it has children to show under it, open
	// whether it shows them, kids those shown and children the nodes
	// they were made from.
	expandable, open bool
	kids             []*item
	children         []*ui.Node
	// said is what the row was last made from, and unstyled whether
	// the page had no style for it yet then.
	said                    string
	unstyled                bool
	node, row, under, close *ui.Node
}

// tag is the name of the element n.
func (in *Inspector) tag(n *ui.Node) string {
	switch page := in.pageOf(n); {
	case n == page.Head:
		return "head"
	case page.Tag != nil:
		return page.Tag(n)
	}
	return n.Tag().String()
}

// childrenOf are the nodes the tree shows under n: its children but
// the text that is only white space, after the page's head under its
// root, and under an iframe the root of the page it shows.
func (in *Inspector) childrenOf(n *ui.Node) []*ui.Node {
	var out []*ui.Node
	if shown := in.shows(n); shown != nil {
		return []*ui.Node{shown.Root}
	}
	if page := in.pageOf(n); n == page.Root && page.Head != nil {
		out = append(out, page.Head)
	}
	for _, c := range n.Children() {
		if c.IsText() && strings.TrimSpace(c.Text()) == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// parentOf is the node n is shown under.
func (in *Inspector) parentOf(n *ui.Node) *ui.Node {
	switch page := in.pageOf(n); {
	case n == page.Head:
		return page.Root
	case n == page.Root && in.roots[n] != nil:
		return in.roots[n].frame
	}
	return n.Parent()
}

// each calls do with every node of the tree from n down, as the tree
// lists them.
func (in *Inspector) each(n *ui.Node, do func(n *ui.Node)) {
	do(n)
	if n.IsText() {
		return
	}
	for _, c := range in.childrenOf(n) {
		in.each(c, do)
	}
}

// rebuild makes the tree again for the page.
func (in *Inspector) rebuild() {
	clear(in.items)
	clear(in.rows)
	in.top = nil
	if in.page.Root == nil {
		fill(in.tree, in.none)
		return
	}
	fill(in.tree)
	if in.page.Doctype {
		doctype := &item{doctype: true}
		in.make(doctype)
		in.top = append(in.top, doctype)
		in.tree.Append(doctype.node)
	}
	root := in.build(in.page.Root, nil)
	in.top = append(in.top, root)
	in.tree.Append(root.node)
	if selected := in.state.selected; selected != nil {
		if it := in.reveal(selected); it != nil {
			it.row.AddClass("selected")
		} else {
			in.state.selected = nil
		}
	}
}

// build is the item of n under parent, opened as the state has it: the
// first two levels unless closed by hand, the head closed.
func (in *Inspector) build(n *ui.Node, parent *item) *item {
	it := &item{n: n, parent: parent}
	if parent != nil {
		it.depth = parent.depth + 1
	}
	in.items[n] = it
	in.make(it)
	open, set := in.state.toggled[n]
	if !set {
		open = it.depth < 2 && !in.isHead(n) && in.of(n) == nil && in.inner[n] == nil
	}
	if open && it.expandable {
		in.open(it)
	}
	return it
}

// make makes the elements of an item and fills its row.
func (in *Inspector) make(it *item) {
	it.row = ui.Div(ui.Class("dt-insp-row"),
		ui.Style("padding-left: "+strconv.Itoa(6+it.depth*indent+2*hang)+"px"))
	it.node = ui.Div(ui.Class("dt-insp-node"), it.row)
	in.rows[it.row] = it
	in.say(it)
}

// said is what the row of an item is made from: when it changes, the
// row is made again.
func (in *Inspector) said(it *item) string {
	if it.doctype {
		return "<!DOCTYPE html>"
	}
	n := it.n
	if n.IsText() {
		return n.Text()
	}
	var b strings.Builder
	b.WriteString(in.tag(n))
	b.WriteString(" #" + n.ID() + " ." + strings.Join(n.Classes(), " ") + " ;" + n.Style())
	for _, a := range n.Attrs() {
		b.WriteString(" " + a.Name + "=" + a.Value)
	}
	children := in.childrenOf(n)
	fmt.Fprintf(&b, " %d %t", len(children), it.open)
	if text, ok := alone(children); ok {
		b.WriteString(" " + text)
	}
	return b.String()
}

// alone is the text of an element that holds nothing but a short one,
// which its row shows.
func alone(children []*ui.Node) (string, bool) {
	if len(children) != 1 || !children[0].IsText() {
		return "", false
	}
	text := spaced(children[0].Text())
	return text, utf8.RuneCountInString(text) <= short
}

// spaced is text with each run of white space as one space, and none at
// its ends.
func spaced(text string) string { return strings.Join(strings.Fields(text), " ") }

// cut is text with no more than n characters, the rest as an ellipsis.
func cut(text string, n int) string {
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	return string([]rune(text)[:n]) + "…"
}

func span(class, text string) *ui.Node { return ui.Span(ui.Class(class), ui.Text(text)) }

// say fills the row of an item: a doctype, a text, or an element's
// opening tag with its attributes, then what it holds when that is a
// short text, a pill and its closing tag while it is closed, and its
// badges.
func (in *Inspector) say(it *item) {
	it.said = in.said(it)
	twist := ui.Span(ui.Class("dt-insp-twist"))
	switch {
	case it.doctype:
		it.expandable = false
		fill(it.row, twist, span("dt-insp-doctype", "<!DOCTYPE html>"))
		return
	case it.n.IsText():
		it.expandable = false
		fill(it.row, twist, span("dt-insp-text", cut(spaced(it.n.Text()), long)))
		return
	}
	n, name := it.n, in.tag(it.n)
	children := in.childrenOf(n)
	text, inline := alone(children)
	it.expandable = len(children) > 0 && !inline
	if !it.expandable && it.open {
		in.shut(it)
	}
	if it.expandable {
		twist.AddClass("dt-insp-closed")
		if it.open {
			twist.AddClass("dt-insp-open")
		}
	}
	parts := []*ui.Node{twist, span("dt-insp-p", "<"), span("dt-insp-tag", name)}
	attribute := func(name, value string) {
		parts = append(parts, span("dt-insp-an", " "+name), span("dt-insp-p", `="`),
			span("dt-insp-av", cut(value, wide)), span("dt-insp-p", `"`))
	}
	if n.ID() != "" {
		attribute("id", n.ID())
	}
	if len(n.Classes()) > 0 {
		attribute("class", strings.Join(n.Classes(), " "))
	}
	for _, a := range n.Attrs() {
		if a.Name != device {
			attribute(a.Name, a.Value)
		}
	}
	if n.Style() != "" {
		attribute("style", n.Style())
	}
	parts = append(parts, span("dt-insp-p", ">"))
	switch {
	case inline:
		parts = append(parts, span("dt-insp-text", text), closing(name))
	case it.expandable && !it.open:
		parts = append(parts, span("dt-insp-pill", "…"), closing(name))
	case !it.expandable && !void[name]:
		parts = append(parts, closing(name))
	}
	it.unstyled = false
	if _, styled := in.docOf(n).ComputedValue(n, "display"); !styled && !in.isHead(n) {
		it.unstyled = true
	}
	for _, badge := range in.badges(n) {
		parts = append(parts, span("dt-insp-badge", badge))
	}
	fill(it.row, parts...)
}

// void are the elements that have no closing tag.
//
//nolint:gochecknoglobals // a table
var void = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}

func closing(name string) *ui.Node {
	return ui.Span(ui.Class("dt-insp-end"), span("dt-insp-p", "</"), span("dt-insp-tag", name), span("dt-insp-p", ">"))
}

// badges name what an element is to its children: a flex or a grid
// container, and a box that scrolls.
func (in *Inspector) badges(n *ui.Node) []string {
	if in.page.Doc == nil || in.isHead(n) {
		return nil
	}
	var out []string
	doc := in.docOf(n)
	switch display, _ := doc.ComputedValue(n, "display"); display {
	case "flex", "inline-flex":
		out = append(out, "flex")
	case "grid", "inline-grid":
		out = append(out, "grid")
	}
	for _, axis := range []string{"overflow-x", "overflow-y"} {
		if v, _ := doc.ComputedValue(n, axis); v == "auto" || v == "scroll" {
			out = append(out, "scroll")
			break
		}
	}
	return out
}

// toggle opens a closed item and closes an open one, and remembers it.
func (in *Inspector) toggle(it *item) {
	if in.state.toggled == nil {
		in.state.toggled = map[*ui.Node]bool{}
	}
	in.state.toggled[it.n] = !it.open
	if it.open {
		in.shut(it)
		if selected := in.items[in.state.selected]; selected == nil && in.state.selected != nil {
			in.selectItem(it)
		}
		in.say(it)
		return
	}
	in.open(it)
}

// open shows the children of an item under its row, and its closing
// tag under them.
func (in *Inspector) open(it *item) {
	it.open = true
	it.under = ui.Div(ui.Class("dt-insp-kids"))
	it.close = ui.Div(ui.Class("dt-insp-row"), ui.Class("dt-insp-close"),
		ui.Style("padding-left: "+strconv.Itoa(6+it.depth*indent+2*hang)+"px"),
		ui.Span(ui.Class("dt-insp-twist")), closing(in.tag(it.n)))
	in.rows[it.close] = it
	it.node.Append(it.under, it.close)
	in.list(it)
	in.say(it)
}

// shut takes the children of an item out of the tree.
func (in *Inspector) shut(it *item) {
	it.open = false
	for _, kid := range it.kids {
		in.drop(kid)
	}
	it.kids, it.children = nil, nil
	delete(in.rows, it.close)
	it.node.Remove(it.under)
	it.node.Remove(it.close)
	it.under, it.close = nil, nil
}

// drop forgets an item and those under it.
func (in *Inspector) drop(it *item) {
	for _, kid := range it.kids {
		in.drop(kid)
	}
	if it.n == in.state.selected && it.n != nil {
		in.orphan = it.parent
	}
	delete(in.items, it.n)
	delete(in.rows, it.row)
	delete(in.rows, it.close)
}

// window is which of its children an item shows: those from one place
// up to another.
type window struct{ from, to int }

// windowOf is which of its count children an item shows: the first few
// unless more were asked for.
func (in *Inspector) windowOf(it *item, count int) window {
	w, set := in.state.listed[it.n]
	if !set {
		w = window{0, few}
	}
	w.from = max(min(w.from, count-1), 0)
	w.to = min(max(w.to, w.from), count)
	return w
}

func (in *Inspector) setWindow(it *item, w window) {
	if in.state.listed == nil {
		in.state.listed = map[*ui.Node]window{}
	}
	in.state.listed[it.n] = w
}

// list fills an open item with its children as the page has them now:
// the items of those it showed already stay as they were, and a line
// before and after stands for those it does not show.
func (in *Inspector) list(it *item) {
	children := in.childrenOf(it.n)
	shown := in.windowOf(it, len(children))
	before := map[*ui.Node]*item{}
	for _, kid := range it.kids {
		before[kid.n] = kid
	}
	for _, old := range it.under.Children() {
		if in.rows[old] == it {
			delete(in.rows, old)
		}
	}
	rest := func(class string, count int) *ui.Node {
		line := ui.Div(ui.Class("dt-insp-row"), ui.Class("dt-insp-rest"), ui.Class(class),
			ui.Style("padding-left: "+strconv.Itoa(6+(it.depth+1)*indent+2*hang)+"px"),
			ui.Text(fmt.Sprintf("… %d more nodes", count)))
		in.rows[line] = it
		return line
	}
	kids := make([]*item, 0, shown.to-shown.from)
	nodes := make([]*ui.Node, 0, shown.to-shown.from+2)
	if shown.from > 0 {
		nodes = append(nodes, rest("dt-insp-earlier", shown.from))
	}
	for _, c := range children[shown.from:shown.to] {
		kid := before[c]
		if kid == nil {
			kid = in.build(c, it)
		}
		delete(before, c)
		kids = append(kids, kid)
		nodes = append(nodes, kid.node)
	}
	for _, gone := range before {
		in.drop(gone)
	}
	if shown.to < len(children) {
		nodes = append(nodes, rest("dt-insp-more", len(children)-shown.to))
	}
	it.kids, it.children = kids, children
	fill(it.under, nodes...)
}

// more shows a few more of the children of an item: those after the
// ones it shows, or those before.
func (in *Inspector) more(it *item, after bool) {
	shown := in.windowOf(it, len(in.childrenOf(it.n)))
	if after {
		shown.to += few
	} else {
		shown.from = max(shown.from-few, 0)
	}
	in.setWindow(it, shown)
	in.list(it)
}

// rowAt is the item whose row n is part of, and for a line that stands
// for children not shown, its class: those before or those after.
func (in *Inspector) rowAt(n *ui.Node) (*item, string) {
	part := ""
	for ; n != nil && n != in.tree; n = n.Parent() {
		for _, class := range []string{"dt-insp-earlier", "dt-insp-more"} {
			if n.HasClass(class) {
				part = class
			}
		}
		if it := in.rows[n]; it != nil {
			return it, part
		}
	}
	return nil, ""
}

// visible are the items whose rows show, from the top.
func (in *Inspector) visible() []*item {
	var out []*item
	var add func(it *item)
	add = func(it *item) {
		out = append(out, it)
		for _, kid := range it.kids {
			add(kid)
		}
	}
	for _, it := range in.top {
		add(it)
	}
	return out
}

// reveal opens what stands over n in the tree and gives its item, nil
// for a node that is not of the page. A text shown on the row of its
// element gives that element's.
func (in *Inspector) reveal(n *ui.Node) *item {
	if in.page.Root == nil {
		return nil
	}
	var path []*ui.Node
	for at := n; ; at = in.parentOf(at) {
		if at == nil {
			return nil
		}
		path = append(path, at)
		if at == in.page.Root {
			break
		}
	}
	slices.Reverse(path)
	it := in.items[in.page.Root]
	for _, next := range path[1:] {
		if !it.expandable {
			return it
		}
		children := in.childrenOf(it.n)
		at := slices.Index(children, next)
		if at < 0 {
			return nil
		}
		if shown := in.windowOf(it, len(children)); at < shown.from || at >= shown.to {
			in.setWindow(it, window{max(at-few/2, 0), at + few/2})
			if it.open {
				in.list(it)
			}
		}
		if !it.open {
			in.toggle(it)
		}
		if it = in.items[next]; it == nil {
			return nil
		}
	}
	return it
}

// refresh shows what changed in the page since the tree was last made:
// the rows whose node changed are made again, and the children of the
// open ones listed again. A selected node that left the page gives the
// selection to what it was under.
func (in *Inspector) refresh() {
	if in.page.Root == nil {
		return
	}
	for _, it := range in.visible() {
		if in.items[it.n] != it && !it.doctype {
			continue
		}
		if it.unstyled || it.said != in.said(it) {
			in.say(it)
		}
		if it.open && !slices.Equal(in.childrenOf(it.n), it.children) {
			in.list(it)
		}
	}
	if selected := in.state.selected; selected != nil && in.items[selected] == nil {
		under := in.orphan
		for under != nil && in.items[under.n] != under {
			under = under.parent
		}
		in.selectItem(under)
	}
	in.orphan = nil
	in.restyle()
}

// named reports whether the search for query finds n: an element by
// what its opening tag reads, by #id or by .class, a text by what it
// holds.
func (in *Inspector) named(n *ui.Node, query string) bool {
	switch {
	case n.IsText():
		return strings.Contains(strings.ToLower(n.Text()), query)
	case strings.HasPrefix(query, "#"):
		return strings.EqualFold(n.ID(), query[1:])
	case strings.HasPrefix(query, "."):
		return slices.ContainsFunc(n.Classes(), func(c string) bool { return strings.EqualFold(c, query[1:]) })
	}
	var b strings.Builder
	b.WriteString("<" + in.tag(n))
	if n.ID() != "" {
		b.WriteString(` id="` + n.ID() + `"`)
	}
	if len(n.Classes()) > 0 {
		b.WriteString(` class="` + strings.Join(n.Classes(), " ") + `"`)
	}
	for _, a := range n.Attrs() {
		b.WriteString(" " + a.Name + `="` + a.Value + `"`)
	}
	b.WriteString(">")
	return strings.Contains(strings.ToLower(b.String()), query)
}

func plural(at, of int) string {
	switch {
	case at > 0:
		return fmt.Sprintf("%d of %d", at, of)
	case of == 1:
		return "1 match"
	}
	return fmt.Sprintf("%d matches", of)
}
