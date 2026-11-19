package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"fmt"
	"slices"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

// tools is how the page's menu ends, whatever the page.
const tools = "View page source History Downloads Developer tools Settings - Inspect element"

func labels(entries []string) string { return fmt.Sprint(entries) }

func TestARightClickOnThePageOffersItsMoves(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.rightClick("page")
	if got, want := labels(s.entries()),
		"[Back (off) Forward (off) Reload - Save page as… (off) "+tools+"]"; got != want {
		t.Fatalf("a new tab's menu is %s, want %s", got, want)
	}
	s.visit(srv.URL + "/")
	at := s.inPage("space")
	s.rightClickAt(at)
	if got, want := labels(s.entries()),
		"[Back Forward (off) Reload - Save page as… "+tools+"]"; got != want {
		t.Fatalf("the page's menu is %s, want %s", got, want)
	}
	if box := s.menu.Node().Box(); box.X != at.X || box.Y != at.Y {
		t.Errorf("the menu is at %v, the pointer at %v", box, at)
	}
	s.clean()
	s.choose("History")
	s.at(historyPage)
	if s.menu.IsOpen() {
		t.Fatal("the menu stays open after a choice")
	}
	s.rightClickAt(s.node("page").Children()[0].Box().Center())
	if got, want := labels(s.entries()),
		"[Back Forward (off) Reload - Save page as… (off) "+tools+"]"; got != want {
		t.Fatalf("a page of light's own offers %s, want %s", got, want)
	}
	s.choose("Back")
	s.at(srv.URL + "/")
}

func TestARightClickOnALinkOpensItInANewTabOrCopiesIt(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.rightClickAt(s.inPage("vault"))
	if got, want := labels(s.entries()),
		"[Open link in new tab Open link in new window Save link as… Copy link address - Inspect element]"; got != want {
		t.Fatalf("a link's menu is %s, want %s", got, want)
	}
	s.choose("Copy link address")
	if got := s.win.Clipboard(); got != srv.URL+"/vaults" {
		t.Fatalf("copied %q", got)
	}
	s.at(srv.URL + "/")
	s.rightClickAt(s.inPage("vault"))
	s.choose("Open link in new tab")
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/vaults"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.at(srv.URL + "/")
	s.rightClickAt(s.inPage("vault"))
	s.choose("Open link in new window")
	s.frame()
	if len(s.screens) != 2 || s.window(1).opened() != "["+srv.URL+"/vaults]" || len(s.browser.Tabs) != 2 {
		t.Fatalf("%d windows, the first's tabs are %s", len(s.screens), s.opened())
	}
}

func TestARightClickOnAPictureOpensCopiesOrSavesIt(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.until("the map is drawn", func() bool { return box(t, doc, "map").W == 30 })
	s.rightClickAt(s.inPage("map"))
	if got, want := labels(s.entries()),
		"[Open image in new tab Save image as… Copy image address - Inspect element]"; got != want {
		t.Fatalf("a picture's menu is %s, want %s", got, want)
	}
	s.choose("Copy image address")
	if got := s.win.Clipboard(); got != srv.URL+"/map.png" {
		t.Fatalf("copied %q", got)
	}
	s.rightClickAt(s.inPage("map"))
	s.choose("Save image as…")
	s.until("the picture is saved", func() bool { return s.downloaded("map.png") != "" })
	s.until("the bubble says so", func() bool { return s.status.Text() == "Downloaded map.png" })

	s.until("the door is drawn", func() bool { return box(t, doc, "door").W == 30 })
	s.rightClickAt(s.inPage("door"))
	if got, want := labels(s.entries()),
		"[Open link in new tab Open link in new window Save link as… Copy link address - Open image in new tab "+
			"Save image as… Copy image address - Inspect element]"; got != want {
		t.Fatalf("a picture in a link offers %s, want %s", got, want)
	}
	s.choose("Open image in new tab")
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/map.png"; got != want {
		t.Fatalf("after opening the picture the tabs are %s, want %s", got, want)
	}
	s.click("tab-1")
	s.at(srv.URL + "/map.png")
}

func TestARightClickOnSelectedTextCopiesIt(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	words := doc.ByID("words").Children()[0]
	doc.Select(ui.Position{Node: words, Offset: 4}, ui.Position{Node: words, Offset: 16})
	s.rightClickAt(s.inPage("words"))
	if got := labels(s.entries()); got != "[Copy Search DuckDuckGo for “lower vaults” - Inspect element]" {
		t.Fatalf("selected text offers %s", got)
	}
	s.choose("Copy")
	if got := s.win.Clipboard(); got != "lower vaults" {
		t.Fatalf("copied %q", got)
	}
	s.rightClickAt(s.inPage("vault"))
	if got, want := labels(s.entries()),
		"[Open link in new tab Open link in new window Save link as… Copy link address - Copy "+
			"Search DuckDuckGo for “lower vaults” - Inspect element]"; got != want {
		t.Fatalf("a link with text selected offers %s, want %s", got, want)
	}
}

func TestARightClickOnAFieldEditsIt(t *testing.T) {
	s := show(t)
	s.Open("light://guide")
	s.frame()
	s.click("address")
	s.doc.SetSelectionRange(s.address, 8, 13)
	s.win.SetClipboard("")
	s.rightClick("address")
	if got, want := labels(s.entries()), "[Cut Copy Paste (off) - Select all]"; got != want {
		t.Fatalf("a field's menu is %s, want %s", got, want)
	}
	s.choose("Copy")
	if got := s.win.Clipboard(); got != "guide" {
		t.Fatalf("copied %q", got)
	}
	if got := s.doc.Value(s.address); got != "light://guide" || !s.doc.Focused(s.address) {
		t.Fatalf("after Copy the field reads %q, focused %v", got, s.doc.Focused(s.address))
	}
	s.rightClick("address")
	s.choose("Cut")
	if got := s.doc.Value(s.address); got != "light://" {
		t.Fatalf("after Cut the field reads %q", got)
	}
	s.win.SetClipboard("about")
	s.rightClick("address")
	s.choose("Paste")
	if got := s.doc.Value(s.address); got != "light://about" {
		t.Fatalf("after Paste the field reads %q", got)
	}
	s.rightClick("address")
	s.choose("Select all")
	if from, to := s.doc.SelectionRange(s.address); from != 0 || to != len("light://about") {
		t.Fatalf("Select all selects %d to %d", from, to)
	}
	s.rightClick("address")
	if got, want := labels(s.entries()), "[Cut Copy Paste - Select all]"; got != want {
		t.Fatalf("with all selected the menu is %s, want %s", got, want)
	}
	s.press(input.KeyEscape)
	if s.menu.IsOpen() {
		t.Fatal("Escape leaves the menu open")
	}

	s.Open(tabs.Home)
	s.frame()
	s.rightClick("search")
	if got, want := labels(s.entries()), "[Cut (off) Copy (off) Paste - Select all (off)]"; got != want {
		t.Fatalf("the empty search field offers %s, want %s", got, want)
	}
}

func TestARightClickOnATabOffersWhatTabsDo(t *testing.T) {
	srv, seen := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/vaults")
	s.rightClick("tab-0")
	if got, want := labels(s.entries()), "[New tab Reload Duplicate - Close tab Close other tabs (off)]"; got != want {
		t.Fatalf("a tab's menu is %s, want %s", got, want)
	}
	s.choose("Duplicate")
	if len(s.browser.Tabs) != 2 || s.browser.Active != 1 {
		t.Fatalf("%d tabs, tab %d shown", len(s.browser.Tabs), s.browser.Active)
	}
	s.at(srv.URL + "/vaults")
	if s.host == nil || seen.count("/vaults") != 1 {
		t.Fatalf("the duplicate shows no page, or fetched it again: %d fetches", seen.count("/vaults"))
	}
	s.rightClick("tab-0")
	s.choose("New tab")
	s.at(tabs.Home)
	s.rightClick("tab-1")
	s.choose("Reload")
	s.visit(srv.URL + "/vaults")
	if s.browser.Active != 1 || seen.count("/vaults") != 2 {
		t.Fatalf("Reload on tab 1: tab %d shown, %d fetches", s.browser.Active, seen.count("/vaults"))
	}
	s.rightClick("tab-2")
	s.choose("Close other tabs")
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("%d tabs after closing the others", len(s.browser.Tabs))
	}
	s.at(tabs.Home)
}

func TestALinkOfLightsOwnPagesHasALinksMenu(t *testing.T) {
	s := show(t)
	s.Open("light://examples")
	s.frame()
	link := s.node("page").Children()[3].Children()[0]
	r, _ := s.doc.BoxOf(link)
	s.rightClickAt(r.Center())
	if got, want := labels(s.entries()),
		"[Open link in new tab Open link in new window Save link as… (off) Copy link address - "+
			"Inspect element]"; got != want {
		t.Fatalf("the menu is %s, want %s", got, want)
	}
	s.choose("Copy link address")
	if got := s.win.Clipboard(); got != "light://guide" {
		t.Fatalf("copied %q", got)
	}
}

func TestTheMenuStaysInTheWindowAndClosesWhenThingsMove(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	size := s.win.Size()
	s.rightClickAt(geom.V2(size.X-4, size.Y-4))
	if box := s.menu.Node().Box(); box.X+box.W > size.X || box.Y+box.H > size.Y {
		t.Fatalf("the menu is at %v in a window of %v", box, size)
	}
	s.in.AddWheel(geom.V2(0, 1))
	s.frame()
	if s.menu.IsOpen() {
		t.Fatal("a scroll leaves the menu open")
	}
	s.rightClickAt(s.inPage("space"))
	s.win.SetSize(geom.V2(900, 600))
	s.frame()
	s.frame()
	if s.menu.IsOpen() {
		t.Fatal("a resize leaves the menu open")
	}
	s.rightClickAt(s.inPage("space"))
	s.command(input.KeyT)
	if s.menu.IsOpen() {
		t.Fatal("a new tab leaves the menu open")
	}
}

func TestAChoiceInTheMenuClicksNothingOfThePageUnderIt(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	link := s.inPage("vault")
	s.rightClickAt(link.Sub(geom.V2(20, 40)))
	if !s.menu.Node().Box().Contains(link) {
		t.Fatalf("the menu at %v does not cover the link at %v", s.menu.Node().Box(), link)
	}
	s.clickAt(link)
	s.at(srv.URL + "/")
}

func TestARightClickOnARequestCopiesOrOpensItsAddress(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.toggleTools()
	s.visit(srv.URL + "/")
	s.until("every request of the page ended", func() bool {
		rows := s.session().log.Rows()
		return len(rows) >= 3 && !slices.ContainsFunc(rows, func(r netlog.Row) bool { return !r.Done })
	})
	s.frame()
	s.rightClick(s.requestRow("/site.css"))
	if got, want := labels(s.entries()), "[Open in new tab Copy address - Copy as HAR (sanitized) "+
		"Save as HAR (sanitized) Save all as HAR (sanitized) - Copy as HAR (with sensitive data) "+
		"Save as HAR (with sensitive data) Save all as HAR (with sensitive data)]"; got != want {
		t.Fatalf("a request's menu is %s, want %s", got, want)
	}
	s.choose("Copy address")
	if got := s.win.Clipboard(); got != srv.URL+"/site.css" {
		t.Fatalf("copied %q", got)
	}
	s.rightClick(s.requestRow("/site.css"))
	s.newTab()
	s.click("tab-0")
	s.rightClick(s.requestRow("/site.css"))
	s.choose("Open in new tab")
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/site.css "+tabs.Home; got != want {
		t.Fatalf("after opening the request the tabs are %s, want %s", got, want)
	}
	s.click("tab-1")
	s.at(srv.URL + "/site.css")
}
