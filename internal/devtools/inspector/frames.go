package inspector

import (
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
)

// framed is a page an iframe shows: the iframe, and the page it is
// in, nil for the one the Inspector shows.
type framed struct {
	Page
	frame *ui.Node
	outer *framed
}

// of is the page n is of when a frame shows it, nil for a node of the
// page the Inspector shows.
func (in *Inspector) of(n *ui.Node) *framed {
	for at := n; at != nil; at = at.Parent() {
		if page := in.roots[at]; page != nil {
			return page
		}
	}
	return nil
}

// pageOf is the page n is of.
func (in *Inspector) pageOf(n *ui.Node) Page {
	if page := in.of(n); page != nil {
		return page.Page
	}
	return in.page
}

// docOf is the Document that styles and lays n out.
func (in *Inspector) docOf(n *ui.Node) *ui.Document { return in.pageOf(n).Doc }

// isHead reports whether n is the head of its page.
func (in *Inspector) isHead(n *ui.Node) bool { return n != nil && n == in.pageOf(n).Head }

// shows is the page the iframe n shows now, nil for none and for what
// is no iframe.
func (in *Inspector) shows(n *ui.Node) *framed {
	ask := in.pageOf(n).Frame
	if ask == nil || n.IsText() {
		return nil
	}
	page, shown := ask(n)
	was := in.inner[n]
	if !shown || page.Doc == nil || page.Root == nil {
		in.forget(n)
		return nil
	}
	if was != nil && was.Root == page.Root && was.Head == page.Head {
		was.Page = page
		return was
	}
	in.forget(n)
	now := &framed{Page: page, frame: n, outer: in.of(n)}
	in.inner[n], in.roots[page.Root] = now, now
	if page.Head != nil {
		in.roots[page.Head] = now
	}
	return now
}

// forget lets go of the page the iframe n showed.
func (in *Inspector) forget(n *ui.Node) {
	if was := in.inner[n]; was != nil {
		delete(in.inner, n)
		delete(in.roots, was.Root)
		delete(in.roots, was.Head)
	}
}

// through is r, a box of the Document of n, in that of the page the
// Inspector shows: through each iframe from n's up, at its zoom. It is
// false while one of them has no box.
func (in *Inspector) through(n *ui.Node, r geom.Rect) (geom.Rect, bool) {
	for page := in.of(n); page != nil; page = page.outer {
		outer := in.docOf(page.frame)
		box, laid := outer.BoxOf(page.frame)
		if !laid {
			return geom.Rect{}, false
		}
		zoom := outer.Zoom()
		r = geom.Rect{X: box.X + r.X*zoom, Y: box.Y + r.Y*zoom, W: r.W * zoom, H: r.H * zoom}
	}
	return r, true
}

// into is the element under at, a point of doc, through the iframes
// there: the innermost one of the innermost document that has one.
func (in *Inspector) into(doc *ui.Document, at geom.Vec2) *ui.Node {
	n := doc.ElementAt(at)
	for n != nil {
		page := in.shows(n)
		if page == nil {
			break
		}
		inside, over := doc.FramePoint(n, at)
		if !over {
			break
		}
		under := page.Doc.ElementAt(inside)
		if under == nil {
			break
		}
		n, doc, at = under, page.Doc, inside
	}
	return n
}

// discover finds the pages the frames of the page show, those of
// their own frames too, when n is of none the tree met yet: an element
// of a frame never opened.
func (in *Inspector) discover(n *ui.Node) {
	top := n
	for top.Parent() != nil {
		top = top.Parent()
	}
	if in.of(n) != nil || in.page.Root == nil || top == in.page.Root || top == in.page.Head {
		return
	}
	var walk func(n *ui.Node)
	walk = func(n *ui.Node) {
		if n.IsText() {
			return
		}
		if shown := in.shows(n); shown != nil {
			walk(shown.Root)
			return
		}
		for _, c := range n.Children() {
			walk(c)
		}
	}
	walk(in.page.Root)
}
