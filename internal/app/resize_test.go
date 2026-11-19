package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"testing"

	"github.com/lantervale/ember/geom"
)

func TestTheScreenAndThePageAreLaidOutAgainWhenTheWindowIsResized(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	page := s.visit(srv.URL + "/")
	if words := box(t, page, "words"); words.W != 1024 {
		t.Fatalf("in a window 1024 wide the page's paragraph is %v", words)
	}
	for _, size := range []geom.Vec2{geom.V2(760, 900), geom.V2(1600, 480)} {
		s.win.SetSize(size)
		s.frame()
		s.frame()
		if tabs := s.box("tabs"); tabs.X+tabs.W > size.X {
			t.Errorf("in a window of %v the tab strip is %v", size, tabs)
		}
		host, _ := s.doc.BoxOf(s.host)
		if host.X != 0 || host.W != size.X || host.Y+host.H != size.Y {
			t.Errorf("in a window of %v the page is drawn in %v", size, host)
		}
		if words := box(t, page, "words"); words.W != size.X {
			t.Errorf("in a window of %v the page's paragraph is %v", size, words)
		}
		if w, h := s.viewport(); w != host.W || h != host.H {
			t.Errorf("in a window of %v the pages are told a view of %v by %v, not %v", size, w, h, host)
		}
	}
}

func TestTheToolsStayInAWindowMadeSmaller(t *testing.T) {
	s := show(t)
	s.toggleTools()
	s.frame()
	s.tools.SetHeight(500)
	s.frame()
	if tools := s.box("devtools"); tools.H != 500 {
		t.Fatalf("the tools given 500 are %v high", tools.H)
	}
	s.win.SetSize(geom.V2(1024, 480))
	s.frame()
	s.frame()
	s.frame()
	tools, stage := s.box("devtools"), s.box("stage")
	if tools.Y+tools.H != 480 || stage.H <= 0 || tools.Y != stage.Y+stage.H {
		t.Fatalf("in a window 480 high the tools are at %v under a page at %v", tools, stage)
	}
}
