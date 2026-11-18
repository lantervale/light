package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"runtime"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"
)

func (s *shown) chord(mod, k input.Key) {
	s.in.SetKey(mod, true)
	s.frame()
	s.in.SetKey(k, true)
	s.frame()
	s.in.SetKey(k, false)
	s.in.SetKey(mod, false)
	s.frame()
}

func TestAPagesTextIsSelectedAndCopiedAndTheAddressBarKeepsItsOwn(t *testing.T) {
	srv := serve(t)
	s := show(t)
	page := s.visit(srv.URL + "/vaults")
	var text *ui.Node
	page.Body().Walk(func(n *ui.Node) bool {
		if n.IsText() && text == nil {
			text = n
		}
		return true
	})
	if text == nil {
		t.Fatal("the page has no text")
	}

	page.Select(ui.Position{Node: text, Offset: 4}, ui.Position{Node: text, Offset: 16})
	rects := page.SelectionRects()
	page.ClearSelection()
	if len(rects) != 1 {
		t.Fatalf("selected rects %v", rects)
	}
	host, _ := s.doc.BoxOf(s.host)
	y := host.Y + rects[0].Y + rects[0].H/2
	s.in.SetMouse(geom.V2(host.X+rects[0].X+0.5, y))
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouse(geom.V2(host.X+rects[0].X+rects[0].W+0.5, y))
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	if got := page.Selection().Text; got != "lower vaults" {
		t.Fatalf("dragged over %q, want %q", got, "lower vaults")
	}
	if !s.doc.Selection().Empty() {
		t.Fatal("light's own screen selected something")
	}

	mod := input.KeyLeftControl
	if runtime.GOOS == "darwin" {
		mod = input.KeyLeftSuper
	}
	s.chord(mod, input.KeyC)
	if got := s.win.Clipboard(); got != "lower vaults" {
		t.Fatalf("copied %q", got)
	}
	s.chord(mod, input.KeyA)
	if got := page.Selection().Text; got != "The lower vaults stay wet." {
		t.Fatalf("select all took %q", got)
	}

	s.click("address")
	s.chord(mod, input.KeyA)
	s.chord(mod, input.KeyC)
	if got := s.win.Clipboard(); got != srv.URL+"/vaults" {
		t.Fatalf("the address bar copied %q", got)
	}
	if got := page.Selection().Text; got != "The lower vaults stay wet." {
		t.Fatalf("the page's selection became %q under the address bar's keys", got)
	}
}
