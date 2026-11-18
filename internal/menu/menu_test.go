package menu_test

import (
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/align"

	"github.com/lantervale/light/internal/menu"
)

type shown struct {
	t      *testing.T
	doc    *ui.Document
	menu   *menu.Menu
	in     *input.State
	frame  func()
	chosen []string
}

const width, height = 640, 360

func show(t *testing.T) *shown {
	t.Helper()
	win := platform.NewWindow(platform.Size(width, height))
	s := &shown{t: t, doc: ui.NewDocument(ui.CSS("html, body { height: 100%; } body { margin: 0; }"),
		ui.CSS(menu.Sheet))}
	s.menu = menu.New(s.doc)
	s.doc.Body().Append(ui.Button(ui.ID("elsewhere"), ui.Text("Elsewhere")), s.menu.Node())
	win.Add(s.doc)
	w := ember.New().AddEntities(win)
	s.in = win.Input()
	s.frame = func() { w.RunFrames(1) }
	s.frame()
	return s
}

func (s *shown) entries() []menu.Entry {
	choose := func(name string) func() { return func() { s.chosen = append(s.chosen, name) } }
	return []menu.Entry{
		{Label: "Back", Do: choose("back"), Disabled: true},
		{Label: "Reload", Do: choose("reload")},
		menu.Separator(),
		{Label: "View page source", Do: choose("source")},
	}
}

func (s *shown) open(at geom.Vec2) {
	s.menu.Open(at, geom.Rect{W: width, H: height}, s.entries())
	s.frame()
	s.frame()
}

func (s *shown) press(k input.Key) {
	s.in.SetKey(k, true)
	s.frame()
	s.in.SetKey(k, false)
	s.frame()
	s.frame()
}

func (s *shown) clickAt(at geom.Vec2) {
	s.in.SetMouse(at)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

func (s *shown) item(i int) *ui.Node { return s.menu.Node().Children()[i] }

func TestAMenuOpensAtThePointWithItsEntries(t *testing.T) {
	s := show(t)
	if s.menu.IsOpen() {
		t.Fatal("a new menu is open")
	}
	if _, shown := s.doc.BoxOf(s.menu.Node()); shown {
		t.Fatal("a closed menu is laid out")
	}
	s.open(geom.V2(100, 50))
	box := s.menu.Node().Box()
	if !s.menu.IsOpen() || box.X != 100 || box.Y != 50 {
		t.Fatalf("open %v at %v", s.menu.IsOpen(), box)
	}
	if got := s.item(1).Text(); got != "Reload" || len(s.menu.Node().Children()) != 4 {
		t.Fatalf("the second entry reads %q of %d", got, len(s.menu.Node().Children()))
	}
	if faults := align.Check(s.doc); len(faults) > 0 {
		t.Errorf("layout faults: %v", faults)
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		t.Errorf("CSS warnings: %v", warns)
	}
}

func TestAMenuOpenedNearAnEdgeStaysInside(t *testing.T) {
	s := show(t)
	s.open(geom.V2(width-10, height-10))
	box := s.menu.Node().Box()
	if box.X+box.W > width || box.Y+box.H > height || box.X < 0 || box.Y < 0 {
		t.Fatalf("the menu is at %v in a window of %v by %v", box, width, height)
	}
}

func TestAClickChoosesAnEntryAndClosesTheMenu(t *testing.T) {
	s := show(t)
	s.open(geom.V2(100, 50))
	s.clickAt(s.item(3).Box().Center())
	if len(s.chosen) != 1 || s.chosen[0] != "source" || s.menu.IsOpen() {
		t.Fatalf("chosen %v, open %v", s.chosen, s.menu.IsOpen())
	}
}

func TestADisabledEntryIsDimmedAndDoesNothing(t *testing.T) {
	s := show(t)
	s.open(geom.V2(100, 50))
	if v, _ := s.item(0).Attr("aria-disabled"); v != "true" {
		t.Fatal("a disabled entry isn't marked")
	}
	s.clickAt(s.item(0).Box().Center())
	if len(s.chosen) != 0 || !s.menu.IsOpen() {
		t.Fatalf("chosen %v, open %v", s.chosen, s.menu.IsOpen())
	}
}

func TestTheArrowsAndEnterChooseAndEscapeCloses(t *testing.T) {
	s := show(t)
	s.open(geom.V2(100, 50))
	s.press(input.KeyDown)
	s.press(input.KeyEnter)
	if len(s.chosen) != 1 || s.chosen[0] != "source" || s.menu.IsOpen() {
		t.Fatalf("Down then Enter chose %v, open %v: Back is disabled, so Reload is first", s.chosen, s.menu.IsOpen())
	}
	s.open(geom.V2(100, 50))
	s.press(input.KeyUp)
	s.press(input.KeyEnter)
	if len(s.chosen) != 2 || s.chosen[1] != "source" {
		t.Fatalf("Up wraps to the last: chosen %v", s.chosen)
	}
	s.open(geom.V2(100, 50))
	s.press(input.KeyEscape)
	if s.menu.IsOpen() || len(s.chosen) != 2 {
		t.Fatalf("after Escape: open %v, chosen %v", s.menu.IsOpen(), s.chosen)
	}
}

func TestAPressElsewhereClosesTheMenu(t *testing.T) {
	s := show(t)
	s.open(geom.V2(100, 50))
	s.clickAt(geom.V2(500, 300))
	if s.menu.IsOpen() {
		t.Fatal("the menu stays open after a press outside it")
	}
	s.open(geom.V2(200, 100))
	if box := s.menu.Node().Box(); box.X != 200 || !s.menu.IsOpen() {
		t.Fatalf("opened again, it is at %v", box)
	}
	s.menu.Close()
	if s.menu.IsOpen() {
		t.Fatal("Close leaves it open")
	}
}
