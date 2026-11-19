package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"runtime"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// rows are the menu's rows as they read: a row's label and its keys, "-"
// for a line between groups, and "(off)" after a row that cannot be
// chosen.
func (s *shown) rows() []string {
	s.t.Helper()
	var out []string
	var listed []*ui.Node
	for _, row := range s.choices.Children() {
		if row.HasClass("group") {
			listed = append(listed, row.Children()...)
			continue
		}
		listed = append(listed, row)
	}
	for _, row := range listed {
		role, _ := row.Attr("role")
		switch {
		case row.HasClass("notch"):
			continue
		case role == "separator":
			out = append(out, "-")
			continue
		case row.HasClass("zoom"):
			var parts []string
			row.Walk(func(n *ui.Node) bool {
				if label, ok := n.Attr("aria-label"); ok {
					parts = append(parts, label)
				}
				return true
			})
			out = append(out, row.Text()+" ("+strings.Join(parts, ", ")+")")
			continue
		}
		var cells []string
		for _, cell := range row.Children() {
			if text := cell.Text(); text != "" {
				cells = append(cells, text)
			}
		}
		if off, _ := row.Attr("aria-disabled"); off == "true" {
			cells = append(cells, "(off)")
		}
		if len(row.Children()) == 0 || row.Children()[0].Tag().String() != "img" {
			s.t.Errorf("the row %v has no glyph", cells)
		}
		out = append(out, strings.Join(cells, " "))
	}
	return out
}

// pick opens the menu and clicks its row id.
func (s *shown) pick(id string) {
	s.t.Helper()
	if !s.choosing() {
		s.click("menu")
	}
	if !s.choosing() {
		s.t.Fatal("a click on its button does not open the menu")
	}
	s.clickAt(s.box(id).Center())
}

// focused is the id of the menu's row that has the focus, "" for none.
func (s *shown) focused() string {
	for _, id := range []string{"menu", "menu-tab", "menu-window", "menu-star", "menu-history", "menu-downloads",
		"menu-zoom-out", "menu-zoom-in", "menu-full", "menu-model-on", "menu-model", "menu-save", "menu-code",
		"menu-settings", "menu-quit"} {
		if n := s.doc.ByID(id); n != nil && s.doc.Focused(n) {
			return id
		}
	}
	return ""
}

func TestTheMenuHangsFromItsButtonAndListsWhatLightDoes(t *testing.T) {
	s := show(t)
	bar, button, saves := s.box("bar"), s.box("menu"), s.box("downloads")
	if button.X < saves.X+saves.W || button.X+button.W > bar.X+bar.W || bar.X+bar.W-button.X-button.W > 16 {
		t.Fatalf("the menu's button at %v is not the last of the row %v, after the downloads' at %v", button, bar, saves)
	}
	if s.choosing() {
		t.Fatal("the menu starts open")
	}
	s.click("menu")
	keys := chordsOf(runtime.GOOS)
	shows := func(keys string) string { return keysShown(keys, runtime.GOOS) }
	want := []string{
		"New tab " + shows(newTabKeys),
		"New window " + shows(newWindowKeys),
		"-",
		"Bookmarks",
		"History " + shows(keys.visited),
		"Downloads " + shows(keys.saves),
		"-",
		"Zoom100 % (Zoom out, Zoom in, Full screen)",
		"-",
		"Save page as… (off)",
		"-",
		"Developer tools " + shows(keys.tools),
		"Settings " + shows(settingsKeys),
		"-",
		"Quit light " + shows(quitKeys),
	}
	if got := s.rows(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the menu lists\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	card, notch := s.box("app-menu"), s.choices.Children()[0].Box()
	if centre := notch.Center().X; centre < button.X || centre > button.X+button.W || card.Y < button.Y+button.H ||
		card.X+card.W > bar.X+bar.W {
		t.Fatalf("the menu %v, its notch %v, does not hang from its button %v", card, notch, button)
	}
	if expanded, _ := s.more.Attr("aria-expanded"); expanded != "true" {
		t.Fatalf("the open menu's button says aria-expanded=%q", expanded)
	}
	s.clean()
	s.clickAt(geom.V2(900, 620))
	if s.choosing() {
		t.Fatal("a press elsewhere leaves the menu open")
	}
	s.at(tabs.Home)
	s.clean()
}

func TestTheMenuShowsTheKeysOfTheSystem(t *testing.T) {
	for _, c := range []struct{ keys, mac, other string }{
		{newTabKeys, "Cmd+T", "Ctrl+T"},
		{chordsOf("darwin").saves, "Cmd+Shift+J", "Ctrl+Shift+J"},
		{chordsOf("linux").saves, "Ctrl+J", "Ctrl+J"},
		{chordsOf("darwin").tools, "Cmd+Option+I", "Ctrl+Alt+I"},
		{chordsOf("linux").tools, "Ctrl+Shift+I", "Ctrl+Shift+I"},
		{settingsKeys, "Cmd+,", "Ctrl+,"},
		{zoomInKeys, "Cmd++", "Ctrl++"},
		{"F12", "F12", "F12"},
	} {
		if mac, other := keysShown(c.keys, "darwin"), keysShown(c.keys, "linux"); mac != c.mac || other != c.other {
			t.Errorf("%q shows as %q on macOS and %q elsewhere, want %q and %q", c.keys, mac, other, c.mac, c.other)
		}
	}
}

func TestTheMenuIsWalkedByTheArrowsChosenByEnterAndClosedByEscape(t *testing.T) {
	s := show(t)
	s.doc.Focus(s.more)
	s.frame()
	s.press(input.KeyEnter)
	if !s.choosing() || s.focused() != "menu-tab" {
		t.Fatalf("Enter on the button: the menu open %v, the focus on %q", s.choosing(), s.focused())
	}
	s.press(input.KeyDown)
	s.press(input.KeyDown)
	if s.focused() != "menu-star" {
		t.Fatalf("two arrows down the focus is on %q", s.focused())
	}
	s.press(input.KeyUp)
	if s.focused() != "menu-window" {
		t.Fatalf("an arrow up the focus is on %q", s.focused())
	}
	s.press(input.KeyEscape)
	if s.choosing() || s.focused() != "menu" {
		t.Fatalf("after Escape the menu is open %v, the focus on %q", s.choosing(), s.focused())
	}
	s.press(input.KeyEnter)
	for range 3 {
		s.press(input.KeyDown)
	}
	if s.focused() != "menu-history" {
		t.Fatalf("three arrows down the focus is on %q", s.focused())
	}
	s.press(input.KeyEnter)
	if s.at(historyPage); s.choosing() {
		t.Fatal("the menu stays open after a choice")
	}
	s.clean()
}

func TestTheMenusPointerAndItsArrowsHighlightTheSameOneRow(t *testing.T) {
	s := show(t)
	s.click("menu")
	s.press(input.KeyDown)
	s.in.SetMouse(s.box("menu-settings").Center())
	s.frame()
	s.frame()
	if s.focused() != "menu-settings" {
		t.Fatalf("under the pointer the focus is on %q", s.focused())
	}
	s.in.SetMouse(s.box("address").Center())
	s.frame()
	s.press(input.KeyDown)
	if s.focused() != "menu-quit" {
		t.Fatalf("an arrow down from the row the pointer left, the focus is on %q", s.focused())
	}
}

func TestEachRowOfTheMenuDoesWhatItReads(t *testing.T) {
	s := show(t)
	s.pick("menu-tab")
	if len(s.browser.Tabs) != 2 || s.choosing() {
		t.Fatalf("New tab left %d tabs, the menu open %v", len(s.browser.Tabs), s.choosing())
	}
	for id, address := range map[string]string{"menu-star": bookmarksPage, "menu-history": historyPage,
		"menu-downloads": downloadsPage, "menu-settings": configPage} {
		s.Open(tabs.Page(guideName))
		s.frame()
		s.pick(id)
		s.at(address)
	}
	s.pick("menu-code")
	if !s.toolsShown() {
		t.Fatal("Developer tools did not open them")
	}
	s.pick("menu-window")
	s.frame()
	if len(s.screens) != 2 {
		t.Fatalf("New window left light with %d windows", len(s.screens))
	}
	s.pick("menu-quit")
	if !s.quitting {
		t.Fatal("Quit light did not end it")
	}
}

func TestTheMenuZoomsTheTabStayingOpenAndFillsTheScreen(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	page := s.visit(srv.URL + "/")
	s.pick("menu-zoom-in")
	s.frame()
	if !s.choosing() || s.node("menu-zoom").Text() != "110 %" || page.Zoom() != 1.1 || s.focused() != "menu-zoom-in" {
		t.Fatalf("after a step in the menu is open %v and reads %q, the page at %v, the focus on %q", s.choosing(),
			s.node("menu-zoom").Text(), page.Zoom(), s.focused())
	}
	s.pick("menu-zoom-out")
	s.frame()
	s.press(input.KeyEnter)
	if !s.choosing() || s.node("menu-zoom").Text() != "90 %" || page.Zoom() != 0.9 {
		t.Fatalf("after two steps out the menu is open %v and reads %q, the page at %v", s.choosing(),
			s.node("menu-zoom").Text(), page.Zoom())
	}
	s.clean()
	s.pick("menu-full")
	if !s.win.Fullscreen() || s.choosing() {
		t.Fatalf("Full screen: the window fills it %v, the menu open %v", s.win.Fullscreen(), s.choosing())
	}
	s.pick("menu-full")
	if s.win.Fullscreen() {
		t.Fatal("Full screen again leaves the window filling the screen")
	}
}

func TestTheMenuSavesAPageOfTheWebAndNotOneOfLightsOwn(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.pick("menu-save")
	if !s.choosing() || len(s.saves.Items()) != 0 {
		t.Fatalf("on the start page the row that saves closes the menu %v, and saved %d", !s.choosing(),
			len(s.saves.Items()))
	}
	s.visit(srv.URL + "/")
	s.click("menu")
	s.frame()
	if off, _ := s.node("menu-save").Attr("aria-disabled"); off == "true" {
		t.Fatal("on a page of the web the row that saves is off")
	}
	s.pick("menu-save")
	s.settled()
	if got := s.downloaded("A crypt.html"); !strings.HasPrefix(got, "<title>A crypt</title>") {
		t.Fatalf("the page's file holds %q", got)
	}
}
