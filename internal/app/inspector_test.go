package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/tabs"
)

// markup are the rows the Inspector's tree shows, each as its text,
// but the head's, which only a page loaded for its scripts keeps.
func (s *shown) markup() []string {
	var out []string
	s.node("dt-insp-tree").Walk(func(n *ui.Node) bool {
		if n.HasClass("dt-insp-row") {
			if _, ok := s.doc.BoxOf(n); ok && n.Text() != "<head>…</head>" {
				out = append(out, n.Text())
			}
			return false
		}
		return true
	})
	return out
}

// inspect opens the developer tools on the Inspector.
func (s *shown) inspector() {
	s.t.Helper()
	s.toggleTools()
	s.frame()
	s.click("dt-tab-" + devtools.InspectorTab)
	for range 20 {
		s.frame()
	}
}

func TestTheInspectorShowsTheMarkupOfAPageOfTheWeb(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.inspector()
	rows := s.markup()
	want := []string{"<!DOCTYPE html>", "<html>", "<body>", `<p id="words">The lower vaults stay wet.</p>`,
		`<a id="vault" href="/vaults">the vault page</a>`, "<br>"}
	if got := strings.Join(rows, "\n"); !strings.HasPrefix(got, strings.Join(want, "\n")) {
		t.Fatalf("the tree shows\n%s", got)
	}
	if rows[len(rows)-1] != "</html>" || rows[len(rows)-2] != "</body>" {
		t.Fatalf("the tree ends with %q", rows[len(rows)-2:])
	}

	words := doc.ByID("words")
	row := s.inspect.RowOf(words)
	s.in.SetMouse(row.Box().Center())
	s.frame()
	s.frame()
	host, _ := s.doc.BoxOf(s.host)
	boxes, _ := doc.BoxesOf(words)
	outlined, ok := s.inspect.Highlighted()
	if want := (geom.Rect{X: boxes.Border.X + host.X, Y: boxes.Border.Y + host.Y, W: boxes.Border.W,
		H: boxes.Border.H}); !ok || outlined != want {
		t.Fatalf("the paragraph is outlined at %v (%t), the page has it at %v", outlined, ok, want)
	}
	if shown := s.node("dt-insp-hl-border").Box(); shown != outlined {
		t.Fatalf("the outline is drawn at %v, not %v", shown, outlined)
	}
	s.clickAt(row.Box().Center())
	if s.inspect.Selected() != words {
		t.Fatalf("a press on the row selected %v", s.inspect.Selected())
	}
	s.inspect.Select(doc.ByID("map"))
	s.frame()
	if got := s.node("dt-insp-rules").Text(); !strings.HasPrefix(got, "#map {site.css:2display: block;}") {
		t.Fatalf("the rules of the map read %q", got)
	}
	s.clean()
	if failed := s.inspect.Failed(); failed != "" {
		t.Fatalf("the Inspector failed: %s", failed)
	}

	s.toggleTools()
	s.frame()
	s.frame()
	if _, shown := s.doc.BoxOf(s.inspect.Overlay()); shown {
		t.Fatal("the outline stays over the page once the tools are closed")
	}
	s.clean()
}

func TestPickingAnElementKeepsThePressFromThePage(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.inspector()
	s.click("dt-insp-pick")
	if !s.inspect.Picking() {
		t.Fatal("the button did not start picking")
	}
	s.clickAt(s.inPage("vault"))
	s.frame()
	s.at(srv.URL + "/")
	if s.inspect.Selected() != doc.ByID("vault") || s.inspect.Picking() {
		t.Fatalf("picking the link selected %v", s.inspect.Selected())
	}
	if !s.inspect.RowOf(doc.ByID("vault")).HasClass("selected") {
		t.Fatal("the row of the picked link is not the selected one")
	}
	s.click("dt-insp-pick")
	s.press(input.KeyEscape)
	if s.inspect.Picking() {
		t.Fatal("Escape did not end picking")
	}
	s.clean()
	s.clickAt(s.inPage("vault"))
	s.until("the link is followed once picking ended", func() bool {
		return s.browser.Tab().Current() == srv.URL+"/vaults"
	})
}

func TestInspectElementOfTheMenuSelectsWhatIsUnderIt(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.rightClickAt(s.inPage("door"))
	s.choose("Inspect element")
	for range 3 {
		s.frame()
	}
	if !s.toolsShown() || s.session().tools.Shown() != devtools.InspectorTab {
		t.Fatalf("the tools show %q", s.session().tools.Shown())
	}
	door := doc.ByID("door")
	if s.inspect.Selected() != door || !s.inspect.RowOf(door).HasClass("selected") {
		t.Fatalf("the menu selected %v", s.inspect.Selected())
	}
	if row := s.inspect.RowOf(door).Text(); !strings.HasPrefix(row, `<img id="door" src="/map.png" alt="A door">`) {
		t.Fatalf("the picture's row reads %q", row)
	}
	if got := s.node("dt-insp-box-content").Text(); got != "30 × 20" {
		t.Fatalf("the picture's box is %q", got)
	}
	s.clean()
}

func TestTheInspectorShowsAPageOfLightsOwn(t *testing.T) {
	s := show(t)
	s.Open(historyPage)
	s.frame()
	s.inspector()
	rows := s.markup()
	if len(rows) < 3 || !strings.HasPrefix(rows[0], `<article id="page" class="history">`) ||
		!strings.HasPrefix(rows[1], "<h1>") {
		t.Fatalf("the tree shows %q", rows)
	}
	title := s.node("page").Children()[0]
	s.rightClickAt(title.Box().Center())
	s.choose("Inspect element")
	s.frame()
	s.frame()
	if s.inspect.Selected() != title {
		t.Fatalf("the menu selected %v", s.inspect.Selected())
	}
	outline := s.node("dt-insp-outline").Box()
	if boxes, _ := s.doc.BoxesOf(title); outline != boxes.Border {
		t.Fatalf("the title is outlined at %v, it is at %v", outline, boxes.Border)
	}
	s.clean()
}

func TestTheSheetOfARuleOpensItsSourceInATabBehind(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.newTab()
	s.click("tab-0")
	s.inspector()
	s.inspect.Select(doc.ByID("map"))
	s.frame()
	var from *ui.Node
	s.node("dt-insp-rules").Walk(func(n *ui.Node) bool {
		if from == nil && n.HasClass("dt-insp-link") {
			from = n
		}
		return true
	})
	if from == nil {
		t.Fatal("the rule of the map names no sheet to open")
	}
	s.clickAt(from.Box().Center())
	if got, want := s.opened(), "["+srv.URL+"/] "+tabs.SourceOf(srv.URL+"/site.css")+" "+tabs.Home; got != want {
		t.Fatalf("after a click on the rule's sheet the tabs are %s, want %s", got, want)
	}
}
