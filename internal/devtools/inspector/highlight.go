package inspector

import (
	"fmt"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
)

// The parts of the highlight, from the outside in.
const (
	edgeMargin = iota
	edgeBorder
	edgePadding
	edgeContent
)

// overlays makes what outlines an element over the page: a part for
// each of its boxes, a label, and the outline of the selected one, all
// cut to the page's area.
func (in *Inspector) overlays() {
	in.clip = ui.Div(ui.ID("dt-insp-clip"))
	for i, name := range []string{"margin", "border", "padding", "content"} {
		in.edges[i] = ui.Div(ui.ID("dt-insp-hl-"+name), ui.Class("dt-insp-hl"))
		in.clip.Append(in.edges[i])
	}
	in.outline = ui.Div(ui.ID("dt-insp-outline"), ui.Class("dt-insp-hl"))
	in.label = ui.Div(ui.ID("dt-insp-label"), ui.Class("dt-insp-hl"))
	in.clip.Append(in.outline)
	in.overlay = ui.Div(ui.ID("dt-insp-overlay"), ui.Attr("aria-hidden", "true"), in.clip, in.label)
}

// origin is where the page's own points start in the Inspector's
// Document.
func (in *Inspector) origin(area geom.Rect) geom.Vec2 {
	if in.page.Doc == in.doc {
		return geom.Vec2{}
	}
	return geom.V2(area.X, area.Y)
}

// boxesOf are the boxes of n, an element of the page, in the
// Inspector's Document.
func (in *Inspector) boxesOf(n *ui.Node, area geom.Rect) (ui.Boxes, bool) {
	if n == nil || n.IsText() || in.page.Doc == nil {
		return ui.Boxes{}, false
	}
	boxes, ok := in.page.Doc.BoxesOf(n)
	if !ok {
		return ui.Boxes{}, false
	}
	by := in.origin(area)
	move := func(r geom.Rect) geom.Rect { return geom.Rect{X: r.X + by.X, Y: r.Y + by.Y, W: r.W, H: r.H} }
	return ui.Boxes{Margin: move(boxes.Margin), Border: move(boxes.Border), Padding: move(boxes.Padding),
		Content: move(boxes.Content)}, true
}

// Highlighted is the border box the highlight shows, in the points of
// the Inspector's Document; false while it shows none.
func (in *Inspector) Highlighted() (geom.Rect, bool) {
	defer in.survive()
	area, ok := in.area()
	if !ok || !in.shown {
		return geom.Rect{}, false
	}
	boxes, ok := in.boxesOf(in.hovered, area)
	return boxes.Border, ok
}

func (in *Inspector) area() (geom.Rect, bool) {
	if in.page.Root == nil || in.page.Doc == nil || in.page.Area == nil {
		return geom.Rect{}, false
	}
	return in.page.Area()
}

// under is the element of the page under at, a point of the
// Inspector's Document; nil outside the page's area.
func (in *Inspector) under(at geom.Vec2) *ui.Node {
	area, ok := in.area()
	if !ok || at.X < area.X || at.Y < area.Y || at.X >= area.X+area.W || at.Y >= area.Y+area.H {
		return nil
	}
	if in.page.Doc != in.doc {
		n := in.page.Doc.ElementAt(at.Sub(in.origin(area)))
		for at := n; at != nil; at = at.Parent() {
			if at == in.page.Root {
				return n
			}
		}
		return nil
	}
	return in.deepest(in.page.Root, at)
}

// deepest is the last element from n down whose border box holds at:
// what a page of the Inspector's own Document has there, whatever is
// over it.
func (in *Inspector) deepest(n *ui.Node, at geom.Vec2) *ui.Node {
	kids := n.Children()
	for i := len(kids) - 1; i >= 0; i-- {
		if kids[i].IsText() {
			continue
		}
		if found := in.deepest(kids[i], at); found != nil {
			return found
		}
	}
	if boxes, ok := in.page.Doc.BoxesOf(n); ok && at.X >= boxes.Border.X && at.Y >= boxes.Border.Y &&
		at.X < boxes.Border.X+boxes.Border.W && at.Y < boxes.Border.Y+boxes.Border.H {
		return n
	}
	return nil
}

// place puts the highlight over the element pointed at and the outline
// over the selected one, and takes both away while the Inspector is not
// shown or the page has no area.
func (in *Inspector) place() {
	area, ok := in.area()
	stage, laid := in.doc.BoxOf(in.overlay)
	shown := in.shown && ok
	toggle(in.overlay, "shown", shown)
	if !shown || !laid {
		return
	}
	restyle(in.clip, placed(geom.Rect{X: area.X - stage.X, Y: area.Y - stage.Y, W: area.W, H: area.H}))
	within := func(r geom.Rect) geom.Rect { return geom.Rect{X: r.X - area.X, Y: r.Y - area.Y, W: r.W, H: r.H} }

	boxes, pointed := in.boxesOf(in.hovered, area)
	levels := []geom.Rect{boxes.Margin, boxes.Border, boxes.Padding, boxes.Content, boxes.Content}
	for i, edge := range in.edges {
		toggle(edge, "shown", pointed)
		if !pointed {
			continue
		}
		outer, inner := levels[i], levels[i+1]
		style := placed(within(outer))
		if i != edgeContent {
			style += fmt.Sprintf("; border-width: %.1fpx %.1fpx %.1fpx %.1fpx", max(inner.Y-outer.Y, 0),
				max(outer.X+outer.W-inner.X-inner.W, 0), max(outer.Y+outer.H-inner.Y-inner.H, 0), max(inner.X-outer.X, 0))
		}
		restyle(edge, style)
	}
	toggle(in.label, "shown", pointed)
	if pointed {
		if text := in.labelled(in.hovered, boxes.Border); in.label.Text() != text {
			in.label.SetText(text)
		}
		y := boxes.Border.Y - 22
		if y < area.Y {
			y = min(boxes.Border.Y+boxes.Border.H+4, area.Y+area.H-22)
		}
		x := min(max(boxes.Border.X, area.X), area.X+area.W-40)
		restyle(in.label, fmt.Sprintf("left: %.1fpx; top: %.1fpx", x-stage.X, y-stage.Y))
	}

	selected, outlined := in.boxesOf(in.element(), area)
	toggle(in.outline, "shown", outlined)
	if outlined {
		restyle(in.outline, placed(within(selected.Border)))
	}
}

// labelled is what the label says of n: its tag, id and classes, and the
// size of its border box.
func (in *Inspector) labelled(n *ui.Node, border geom.Rect) string {
	var b strings.Builder
	b.WriteString(in.tag(n))
	if n.ID() != "" {
		b.WriteString("#" + n.ID())
	}
	for _, class := range n.Classes() {
		b.WriteString("." + class)
	}
	return cut(b.String(), 60) + "  " + number(border.W) + " × " + number(border.H)
}

func placed(r geom.Rect) string {
	return fmt.Sprintf("left: %.1fpx; top: %.1fpx; width: %.1fpx; height: %.1fpx", r.X, r.Y, max(r.W, 0), max(r.H, 0))
}

func restyle(n *ui.Node, style string) {
	if n.Style() != style {
		n.SetStyle(style)
	}
}
