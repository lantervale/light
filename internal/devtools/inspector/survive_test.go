package inspector_test

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools/inspector"
)

// sweep presses the pointer every few points of the whole window, the
// tools, the page and what is beside them.
func (s *shown) sweep() {
	for y := float32(-10); y < 720; y += 9 {
		s.input.SetMouse(geom.Vec2{X: 30 + float32(int(y)%7)*40, Y: y})
		s.frames(1)
		s.input.SetMouseButton(input.MouseLeft, true)
		s.frames(1)
		s.input.SetMouseButton(input.MouseLeft, false)
		s.frames(1)
	}
}

func TestAPressAnywhereIsSurvived(t *testing.T) {
	for _, own := range []bool{false, true} {
		s := show(t, own)
		s.click(s.twist(s.card))
		s.sweep()
		s.click(s.node("dt-insp-pick"))
		s.sweep()
		if failed := s.in.Failed(); failed != "" {
			t.Fatalf("the Inspector failed: %s", failed)
		}
	}
}

func TestAPageHalfThereShowsNoPage(t *testing.T) {
	s := show(t, false)
	s.click(s.row(s.words))
	s.input.SetMouse(s.box(s.row(s.card)).Center())
	s.frames(2)
	whole := s.page
	for name, page := range map[string]inspector.Page{
		"no Document": {Root: whole.Root, Area: whole.Area},
		"no root":     {Doc: whole.Doc, Area: whole.Area},
		"no area":     {Doc: whole.Doc, Root: whole.Root},
		"a root out of its Document": {Doc: whole.Doc, Root: ui.Div(ui.P(ui.Text("alone"))), Area: whole.Area,
			Doctype: true},
	} {
		s.page = page
		s.frames(2)
		s.in.SetPicking(true)
		s.sweep()
		s.in.Select(s.words)
		s.in.Select(nil)
		s.in.Select(ui.Div())
		s.frames(20)
		if failed := s.in.Failed(); failed != "" {
			t.Fatalf("%s: the Inspector failed: %s", name, failed)
		}
		s.page = whole
		s.frames(2)
	}
}

func TestAPageSwappedUnderThePointerIsFollowed(t *testing.T) {
	s := show(t, false)
	s.click(s.node("dt-insp-pick"))
	s.input.SetMouse(s.onPage(s.words))
	s.frames(2)
	if _, ok := s.in.Highlighted(); !ok {
		t.Fatal("picking outlines nothing")
	}
	next := ui.NewDocument(ui.BrowserSheets())
	next.Body().Append(ui.H1(ui.Text("Vaults")))
	s.page = inspector.Page{Doc: next, Root: next.HTML(), Doctype: true, Area: s.page.Area}
	s.input.SetMouseButton(input.MouseLeft, true)
	s.frames(1)
	s.input.SetMouseButton(input.MouseLeft, false)
	s.frames(20)

	s.click(s.in.RowOf(next.Body()))
	next.HTML().Remove(next.Body())
	s.input.SetMouse(s.box(s.node("dt-insp-tree")).Center())
	s.frames(40)
	s.sweep()
	if failed := s.in.Failed(); failed != "" {
		t.Fatalf("the Inspector failed: %s", failed)
	}
	if s.in.Selected() == next.Body() {
		t.Fatal("a node taken out of its page stays selected")
	}
}

func TestAFailureOfTheInspectorStaysInItsTab(t *testing.T) {
	s := show(t, false)
	whole := s.page
	s.page.Area = func() (geom.Rect, bool) { panic("the page is gone") }
	s.frames(3)
	if failed := s.in.Failed(); !strings.Contains(failed, "the page is gone") {
		t.Fatalf("the failure is told as %q", failed)
	}
	if got := s.node("dt-insp-tree").Text(); !strings.HasPrefix(got,
		"The Inspector failed and looks at the page again: ") {
		t.Fatalf("the tree reads %q", got)
	}
	if s.in.Picking() || s.in.Selected() != nil {
		t.Fatal("a failed Inspector keeps hold of the page")
	}
	s.in.Select(s.words)
	s.in.SetPicking(true)
	s.in.Show(s.state)
	s.page = whole
	s.frames(40)
	if s.in.Failed() != "" || s.in.RowOf(s.words) == nil {
		t.Fatalf("the page whole again, the Inspector still says %q", s.in.Failed())
	}
}
