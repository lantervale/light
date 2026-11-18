package inspector_test

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools/inspector"
)

const framedSheet = `body { margin: 0; }
#inner { box-sizing: border-box; margin: 4px 6px; padding: 3px; width: 120px; height: 30px; color: purple; }
#nested { display: block; border: 0; width: 200px; height: 60px; margin-top: 8px; }`

// framing has the page of s show a document in an iframe, with a
// document in an iframe of its own, and gives the three: the iframe,
// the element of the first document and that of the second.
func framing(s *shown) (*ui.Node, *ui.Node, *ui.Node) {
	deep := ui.NewDocument(ui.BrowserSheets(),
		ui.CSS(`body { margin: 0; } #leaf { margin: 5px; height: 20px; width: 50px; }`))
	leaf := ui.P(ui.ID("leaf"), ui.Text("leaf"))
	deep.Body().Append(leaf)
	child := ui.NewDocument(ui.BrowserSheets(), ui.CSS(framedSheet, ui.From("https://framed.test/frame.css")))
	inner, nested := ui.Div(ui.ID("inner"), ui.Text("inner")), ui.Iframe(deep, ui.ID("nested"))
	child.Body().Append(inner, nested)
	frame := ui.Iframe(child, ui.ID("frame"),
		ui.Style("position: absolute; left: 30px; top: 10px; border: 0; width: 400px; height: 150px"))
	s.page.Doc.Body().Append(frame)
	head := ui.Div(ui.Div(ui.Attr("name", "framed")))
	pages := map[*ui.Node]inspector.Page{
		frame: {Doc: child, Root: child.HTML(), Head: head, Tag: func(n *ui.Node) string {
			if n.Parent() == head {
				return "meta"
			}
			return n.Tag().String()
		}},
		nested: {Doc: deep, Root: deep.HTML()},
	}
	var of func(n *ui.Node) (inspector.Page, bool)
	of = func(n *ui.Node) (inspector.Page, bool) {
		page, shows := pages[n]
		page.Frame = of
		return page, shows
	}
	s.page.Frame = of
	s.frames(40)
	return frame, inner, leaf
}

func TestAnIframeOpensOnTheDocumentItShows(t *testing.T) {
	s := show(t, false)
	frame, inner, leaf := framing(s)
	got := strings.Join(s.rows(), "\n")
	if !strings.Contains(got, `<iframe id="frame"`) || strings.Contains(got, "inner") {
		t.Fatalf("the tree shows\n%s", got)
	}
	s.click(s.twist(frame))
	rows := strings.Join(s.rows(), "\n")
	if !strings.Contains(rows, "<html>…</html>\n</iframe>") {
		t.Errorf("an open iframe does not hold its document:\n%s", rows)
	}
	s.in.Select(leaf)
	s.frames(2)
	if s.in.Selected() != leaf || !s.row(leaf).HasClass("selected") {
		t.Fatalf("an element two frames down selected %v", s.in.Selected())
	}
	rows = strings.Join(s.rows(), "\n")
	for _, want := range []string{`<div id="inner">inner</div>`, `<iframe id="nested">`, `<p id="leaf">leaf</p>`,
		`<meta name="framed">`} {
		if strings.Contains(want, "meta") == strings.Contains(rows, want) {
			t.Errorf("revealing the leaf: %q in\n%s", want, rows)
		}
	}
	s.in.Select(inner)
	s.frames(2)
	rules := strings.Join(s.texts("dt-insp-rules", "dt-insp-rule"), "\n")
	if !strings.Contains(rules, "#inner") || !strings.Contains(rules, "purple") || !strings.Contains(rules, "frame.css") {
		t.Errorf("the rules of an element of a frame are\n%s", rules)
	}
	if box := s.node("dt-insp-box").Text(); !strings.Contains(box, "120 × 30") && !strings.Contains(box, "114 × 24") {
		t.Errorf("the box of an element of a frame reads %q", box)
	}
	if len(s.selected) < 2 || s.selected[len(s.selected)-1] != inner {
		t.Errorf("the screen was told %v selected", s.selected)
	}
}

func TestAnElementOfAFrameIsOutlinedWhereItShows(t *testing.T) {
	s := show(t, false)
	frame, inner, leaf := framing(s)
	s.in.Select(leaf)
	s.frames(2)
	area := s.box(s.web)
	s.input.SetMouse(s.box(s.row(inner)).Center())
	s.frames(3)
	got, ok := s.in.Highlighted()
	want := geom.Rect{X: area.X + 30 + 6, Y: area.Y + 10 + 4, W: 120, H: 30}
	if !ok || got != want {
		t.Errorf("the element of the frame is outlined at %v, %t, want %v", got, ok, want)
	}
	s.input.SetMouse(s.box(s.row(leaf)).Center())
	s.frames(3)
	got, ok = s.in.Highlighted()
	want = geom.Rect{X: area.X + 30 + 5, Y: area.Y + 10 + 4 + 30 + 8 + 5, W: 50, H: 20}
	if !ok || got != want {
		t.Errorf("the element two frames down is outlined at %v, %t, want %v", got, ok, want)
	}
	_ = frame
}

func TestPickingGoesIntoFrames(t *testing.T) {
	s := show(t, false)
	_, inner, leaf := framing(s)
	area := s.box(s.web)
	s.click(s.node("dt-insp-pick"))
	at := geom.V2(area.X+30+6+60, area.Y+10+4+15)
	s.input.SetMouse(at)
	s.frames(3)
	if got, ok := s.in.Highlighted(); !ok || got.Center() != at {
		t.Fatalf("picking over a frame outlines %v, %t, want one around %v", got, ok, at)
	}
	s.clickAt(at)
	if s.in.Selected() != inner || s.in.Picking() || !s.row(inner).HasClass("selected") {
		t.Fatalf("a press in a frame selected %v", s.in.Selected())
	}
	s.click(s.node("dt-insp-pick"))
	s.clickAt(geom.V2(area.X+30+5+25, area.Y+10+4+30+8+5+10))
	if s.in.Selected() != leaf {
		t.Fatalf("a press two frames down selected %v", s.in.Selected())
	}
}

func TestTheTreeFollowsWhatAFrameShows(t *testing.T) {
	s := show(t, false)
	frame, inner, _ := framing(s)
	s.in.Select(inner)
	s.frames(2)
	other := ui.NewDocument(ui.BrowserSheets())
	other.Body().Append(ui.P(ui.ID("fresh"), ui.Text("fresh")))
	s.page.Doc.SetFrame(frame, other)
	was := s.page.Frame
	s.page.Frame = func(n *ui.Node) (inspector.Page, bool) {
		if n == frame {
			return inspector.Page{Doc: other, Root: other.HTML()}, true
		}
		return was(n)
	}
	s.frames(40)
	rows := strings.Join(s.rows(), "\n")
	if strings.Contains(rows, "inner") || s.in.Selected() == inner {
		t.Errorf("the tree keeps the document the frame showed:\n%s", rows)
	}
	s.in.Select(other.ByID("fresh"))
	s.frames(2)
	if rows = strings.Join(s.rows(), "\n"); !strings.Contains(rows, `<p id="fresh">fresh</p>`) {
		t.Errorf("the tree does not show the frame's new document:\n%s", rows)
	}
	s.page.Frame = func(*ui.Node) (inspector.Page, bool) { return inspector.Page{}, false }
	s.frames(40)
	rows = strings.Join(s.rows(), "\n")
	if strings.Contains(rows, "fresh") || !strings.Contains(rows, `<iframe id="frame"`) {
		t.Errorf("an iframe that shows nothing still holds a document:\n%s", rows)
	}
}
