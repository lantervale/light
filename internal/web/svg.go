package web

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// svgNamespace is the namespace html gives an svg element and what is
// in it.
const svgNamespace = "svg"

// pictured reports whether n is an svg element of the page itself, which
// ui shows as one picture: not one inside another.
func pictured(n *html.Node) bool {
	return n.Type == html.ElementNode && n.DataAtom == atom.Svg && n.Namespace == svgNamespace &&
		(n.Parent == nil || n.Parent.Namespace != svgNamespace)
}

// drawing is what an svg element holds, as SVG.
func drawing(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}

// redraw has the svg element n is in, when it is in one, drawn again
// at the next Redraw.
func (t *Tree) redraw(n *html.Node) {
	for ; n != nil && n.Namespace == svgNamespace; n = n.Parent {
		if pictured(n) {
			if t.shown[n] != nil {
				t.stale = append(t.stale, n)
			}
			return
		}
	}
}

// Redraw gives ui what the svg elements changed since the last one hold
// now: once a frame, however many changes a script made.
func (t *Tree) Redraw() {
	for _, n := range t.stale {
		if u := t.shown[n]; u != nil {
			u.SetMarkup(drawing(n))
		}
	}
	t.stale = t.stale[:0]
}
