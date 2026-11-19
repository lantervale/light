package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/tabs"
)

// morning is a Thursday the start page's tests are held on.
func morning() time.Time { return time.Date(2026, time.November, 19, 10, 0, 0, 0, time.UTC) }

// texts are the texts of the children of class under the element id,
// each child's parts apart by " | ".
func (s *shown) texts(id, class string) []string {
	s.t.Helper()
	var out []string
	s.node(id).Walk(func(n *ui.Node) bool {
		if n.IsText() || !n.HasClass(class) {
			return true
		}
		var parts []string
		n.Walk(func(leaf *ui.Node) bool {
			if leaf.IsText() && strings.TrimSpace(leaf.Text()) != "" {
				parts = append(parts, leaf.Text())
			}
			return true
		})
		out = append(out, strings.Join(parts, " | "))
		return false
	})
	return out
}

// seeded is a light whose history and bookmarks hold a few pages of
// sites it never asked, and whose requests are counted.
func seeded(t *testing.T) (*shown, *atomic.Int32) {
	t.Helper()
	asked := &atomic.Int32{}
	s := showWith(t, Options{Downloads: t.TempDir(), Now: morning,
		Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			asked.Add(1)
			return nil, errors.New("no network in tests")
		})})
	for _, v := range []struct {
		address, title string
		ago            time.Duration
	}{
		{"https://old.test/", "Older than the cards", 400 * 24 * time.Hour},
		{"https://harbourferry.test/tickets", "Ticket conditions", 6 * 24 * time.Hour},
		{"https://marine-notes.test/charging", "", 3 * 24 * time.Hour},
		{"https://gazette.test/ferry", "An older title", 2 * 24 * time.Hour},
		{"light://guide", "The ui guide", 26 * time.Hour},
		{"https://gazette.test/ferry", "Council votes to retire the diesel ferry", 20 * time.Minute},
	} {
		s.Profile.visits.Add(v.address, v.title, morning().Add(-v.ago))
	}
	s.rewrite()
	s.marks.Add("https://gazette.test/", "Port Ellery Gazette", "", morning())
	s.marks.Add("https://www.harbourferry.test/", "Harbour Ferry", "", morning())
	s.marks.Add("https://go.test/doc", "Go documentation", "Reading list", morning())
	s.marks.Add("https://go.test/blog", "The Go blog", "Reading list", morning())
	s.marks.AddFolder("Empty")
	s.remark()
	s.frame()
	s.frame()
	return s, asked
}

func TestTheStartPageOfANewLightHasItsLogoItsFieldAndItsOwnPages(t *testing.T) {
	s := show(t)
	for _, id := range []string{"home-bookmarks", "continue", "home-side", "tiles", "recently-closed"} {
		if s.doc.ByID(id) != nil {
			t.Errorf("with nothing kept the start page has #%s", id)
		}
	}
	logo, word, field := s.box("logo"), s.box("wordmark"), s.box("search")
	if logo.Y+logo.H > word.Y || word.Y+word.H > field.Y || !s.doc.Focused(s.node("search")) {
		t.Fatalf("the logo %v, the wordmark %v and the field %v are not one under the other, the field focused", logo,
			word, field)
	}
	if note := s.node("home-note").Text(); note != "Made from what is on this device. Nothing was fetched to show "+
		"this page." {
		t.Fatalf("the note reads %q", note)
	}
	want := "light's own pages: | The ui guide | Examples | About | Bookmarks | History | Downloads | Settings | " +
		"Scripts | CSS Zen Garden"
	if got := strings.Join(s.texts("page", "link"), " | "); "light's own pages: | "+got != want {
		t.Fatalf("the links are %q, want %q", got, want)
	}
	links, note := s.box("own-links"), s.box("home-note")
	if links.Y < note.Y+note.H || links.H > 20 {
		t.Fatalf("the links %v are not one row under the note %v", links, note)
	}
	if text := s.node("page").Text(); strings.Contains(strings.ToLower(text), "model") ||
		strings.Contains(strings.ToLower(text), "summar") {
		t.Fatalf("the start page speaks of a model: %q", text)
	}
	s.clean()
	s.click("go-about")
	s.at(tabs.Page(aboutName))
}

func TestTheStartPageShowsTheBookmarksAsChipsAndAFolderAsOne(t *testing.T) {
	s, _ := seeded(t)
	want := "G | Port Ellery Gazette, H | Harbour Ferry, Reading list"
	if got := strings.Join(s.texts("home-bookmarks", "chip"), ", "); got != want {
		t.Fatalf("the chips are %q, want %q", got, want)
	}
	chips := s.node("home-bookmarks").Children()[1].Children()
	if first, last := chips[0].Box(), chips[2].Box(); first.Y != last.Y || first.H != 36 {
		t.Fatalf("the chips are at %v and %v", first, last)
	}
	head, all := s.box("home-bookmarks"), s.box("all-bookmarks")
	if all.X+all.W != head.X+head.W || all.Y > chips[0].Box().Y {
		t.Fatalf("“All bookmarks” at %v is not at the right end of the section's head %v", all, head)
	}
	s.clean()
	s.clickAt(chips[2].Box().Center())
	s.at(bookmarksPage)
	s.click("home")
	s.click("all-bookmarks")
	s.at(bookmarksPage)
	s.click("home")
	s.clickAt(s.node("home-bookmarks").Children()[1].Children()[1].Box().Center())
	s.at("www.harbourferry.test/")
}

func TestTheStartPageOffersTheLastPagesVisitedEachOnce(t *testing.T) {
	s, _ := seeded(t)
	want := []string{
		"G | Council votes to retire the diesel ferry | gazette.test · 20 min ago",
		"The ui guide | light · yesterday",
		"M | marine-notes.test/charging | marine-notes.test · Monday",
		"H | Ticket conditions | harbourferry.test · Friday",
	}
	if got := s.texts("continue", "card"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the cards are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	cards := s.node("continue").Children()[1].Children()
	if src(cards[1].Children()[0].Children()[0]) != logo {
		t.Fatal("the card of a page of light's own does not show its logo")
	}
	first, second, third := cards[0].Box(), cards[1].Box(), cards[2].Box()
	if first.W < 560 || second.X != first.X || second.W != first.W || second.Y != first.Y+first.H+10 ||
		third.X != first.X || third.Y != second.Y+second.H+10 {
		t.Fatalf("in a window 1024 wide the cards are at %v, %v and %v", first, second, third)
	}
	if side := s.node("home-side"); len(side.Children()) != 0 || side.Box().X != first.X+first.W+24 {
		t.Fatalf("with nothing beside the cards %v the side holds %d sections at %v", first, len(side.Children()),
			side.Box())
	}
	s.clean()
	s.clickAt(third.Center())
	s.at("marine-notes.test/charging")
}

func TestTheStartPageFetchesNothing(t *testing.T) {
	s, asked := seeded(t)
	s.newTab()
	for range 30 {
		s.frame()
	}
	time.Sleep(50 * time.Millisecond)
	s.frame()
	if n := asked.Load(); n != 0 {
		t.Fatalf("showing the start page made %d requests", n)
	}
	s.node("page").Walk(func(n *ui.Node) bool {
		if !n.IsText() && n.Tag().String() == "img" && !strings.HasSuffix(src(n), ".svg") {
			t.Errorf("the start page shows the picture %q, which light does not hold", src(n))
		}
		return true
	})
}

func TestTheStartPageShowsASitesIconOnceLightHoldsIt(t *testing.T) {
	srv, _ := crypt(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Now: morning})
	s.visit(srv.URL + "/")
	s.marks.Add(srv.URL+"/vaults", "Vaults", "", morning())
	s.remark()
	s.click("home")
	chip := s.node("home-bookmarks").Children()[1].Children()[0]
	if icon := chip.Children()[0].Children()[0]; src(icon) != faviconOf(srv.URL+"/") {
		t.Fatalf("the chip of a site whose icon light holds shows %q", src(icon))
	}
	card := s.node("continue").Children()[1].Children()[0]
	if icon := card.Children()[0].Children()[0]; src(icon) != faviconOf(srv.URL+"/") {
		t.Fatalf("the card of a page whose icon light holds shows %q", src(icon))
	}
	s.clean()
}

func TestTheStartPageFollowsTheBookmarksAndTheHistoryOfAnotherWindow(t *testing.T) {
	s := showWith(t, Options{Downloads: t.TempDir(), Now: morning})
	s.newWindow(tabs.Page(guideName))
	s.frame()
	s.frame()
	if got := s.texts("continue", "card"); len(got) != 1 || !strings.Contains(got[0], "The ui guide") {
		t.Fatalf("after a visit in another window the cards are %v", got)
	}
	s.screens[1].marks.Add("https://a.test/", "A", "", morning())
	s.screens[1].remark()
	s.frame()
	if got := s.texts("home-bookmarks", "chip"); len(got) != 1 || got[0] != "A | A" {
		t.Fatalf("after a bookmark in another window the chips are %v", got)
	}
	s.marks.Remove("https://a.test/")
	s.remark()
	s.clearHistory()
	s.frame()
	if s.doc.ByID("home-bookmarks") != nil || s.doc.ByID("continue") != nil {
		t.Fatal("with nothing kept any more the start page still has its sections")
	}
	s.clean()
}

func TestTheStartPageStacksInANarrowWindowAndIsShadedByTheDarkMode(t *testing.T) {
	s, _ := seeded(t)
	for i := range 12 {
		s.marks.Add("https://site.test/"+string(rune('a'+i)), "A bookmark with a long name "+string(rune('a'+i)), "",
			morning())
	}
	s.remark()
	s.win.SetSize(geom.V2(760, 600))
	s.frame()
	s.frame()
	s.frame()
	cards := s.node("continue").Children()[1].Children()
	first, second := cards[0].Box(), cards[1].Box()
	if second.X != first.X || second.Y < first.Y+first.H || first.W != second.W {
		t.Fatalf("in a window 760 wide the cards are at %v and %v", first, second)
	}
	s.newTab()
	s.Open(tabs.Page(guideName))
	s.closeTab(1)
	s.frame()
	s.frame()
	if side, pages := s.box("home-side"), s.box("continue"); side.X != pages.X || side.W != pages.W ||
		side.Y != pages.Y+pages.H+24 {
		t.Fatalf("in a window 760 wide the side %v is not under the pages %v", side, pages)
	}
	chips := s.node("home-bookmarks").Children()[1].Children()
	page, last := s.box("page"), chips[len(chips)-1].Box()
	if len(chips) != homeChips || last.Y <= chips[0].Box().Y || last.X+last.W > page.X+page.W {
		t.Fatalf("%d chips, the first at %v, the last at %v, in a page %v", len(chips), chips[0].Box(), last, page)
	}
	s.clean()
	s.configure(func(c *config.Config) { c.DarkMode = true })
	s.frame()
	if !s.dark() || src(s.node("logo")) != darkLogo || s.doc.ByID("continue") == nil {
		t.Fatalf("the dark start page is dark %v, its logo %q", s.dark(), src(s.node("logo")))
	}
	s.clean()
}

func TestAVisitIsSaidToBeAsLongAgoAsItWas(t *testing.T) {
	now := time.Date(2026, time.November, 19, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{20 * time.Second, "just now"},
		{90 * time.Second, "1 min ago"},
		{59 * time.Minute, "59 min ago"},
		{3 * time.Hour, "3 h ago"},
		{11 * time.Hour, "yesterday"},
		{30 * time.Hour, "yesterday"},
		{3 * 24 * time.Hour, "Monday"},
		{6 * 24 * time.Hour, "Friday"},
		{7 * 24 * time.Hour, "12 November"},
		{400 * 24 * time.Hour, "15 October 2025"},
	} {
		if got := ago(now.Add(-c.ago), now); got != c.want {
			t.Errorf("%v ago is %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestTheStartPageIsAColumnInTheMiddleOfAWideWindow(t *testing.T) {
	s, _ := seeded(t)
	s.win.SetSize(geom.V2(1600, 900))
	s.frame()
	s.frame()
	s.frame()
	view, page, held := s.box("view"), s.box("page"), s.box("held")
	left, right := page.X-view.X, view.X+view.W-page.X-page.W
	if page.W != 1024 || held.W != 960 || left < 280 || left-right > 16 || right-left > 16 {
		t.Fatalf("in a window 1600 wide the page is at %v, what it holds at %v, in a view %v", page, held, view)
	}
	logo := s.box("logo")
	if centre := logo.X + logo.W/2; centre < page.X+page.W/2-1 || centre > page.X+page.W/2+1 {
		t.Fatalf("the logo %v is not in the middle of the page %v", logo, page)
	}
	s.clean()
}

func TestTheStartPageHasTheSizesOfItsMockup(t *testing.T) {
	s, _ := seeded(t)
	s.win.SetSize(geom.V2(1200, 920))
	s.frame()
	s.frame()
	s.frame()
	view, logo, word, field := s.box("view"), s.box("logo"), s.box("wordmark"), s.box("search")
	if logo.Y != view.Y+28 || logo.W != 72 || word.Y != logo.Y+logo.H+3 || word.H != 54 || field.Y != word.Y+word.H+18 ||
		field.W != 560 || field.H != 44 {
		t.Fatalf("in a view %v the logo is at %v, the wordmark %v, the field %v", view, logo, word, field)
	}
	held, marks, pages, note := s.box("held"), s.box("home-bookmarks"), s.box("continue"), s.box("home-note")
	if held.W != 960 || held.Y != field.Y+field.H+28 || pages.Y != marks.Y+marks.H+28 || pages.X != held.X ||
		pages.W != 580 || note.Y != held.Y+held.H+28 {
		t.Fatalf("what the page holds is at %v: the bookmarks %v, the pages %v, the note %v", held, marks, pages, note)
	}
	head, cards := s.node("continue").Children()[0].Box(), s.node("continue").Children()[1].Children()
	if card := cards[0].Box(); head.H != 30 || card.Y != head.Y+head.H+10 || card.H != 66 {
		t.Fatalf("the head of the pages is at %v, their first card %v", head, card)
	}
	for _, c := range []struct {
		dark             bool
		ink, card, field string
	}{{false, "#1b1b1f", "#ffffff", "#ffffff"}, {true, "#e8e6ef", "#1c1b22", "#1c1b22"}} {
		s.configure(func(cfg *config.Config) { cfg.DarkMode = c.dark })
		s.frame()
		card := s.node("continue").Children()[1].Children()[0]
		ink, _ := s.doc.ComputedValue(card.Children()[1].Children()[0], "color")
		ground, _ := s.doc.ComputedValue(card, "background-color")
		under, _ := s.doc.ComputedValue(s.node("search"), "background-color")
		if ink != c.ink || ground != c.card || under != c.field {
			t.Errorf("dark %v: a card's title is %s on %s, the field %s", c.dark, ink, ground, under)
		}
	}
	s.clean()
}

func TestATabClosedIsOfferedAgainBesideTheLastPages(t *testing.T) {
	s, _ := seeded(t)
	s.win.SetSize(geom.V2(1200, 920))
	s.newTab()
	s.closeTab(1)
	s.frame()
	s.frame()
	if s.doc.ByID("recently-closed") != nil {
		t.Fatal("a tab closed on the start page is offered again")
	}
	for _, name := range []string{guideName, aboutName, examplesName, guideName, historyName} {
		s.newTab()
		s.Open(tabs.Page(name))
		s.closeTab(1)
	}
	s.frame()
	s.frame()
	want := "History | just now, The ui guide | just now, Examples | just now"
	if got := strings.Join(s.texts("recently-closed", "shut"), ", "); got != want {
		t.Fatalf("the tabs closed are %q, want %q", got, want)
	}
	if head := s.node("recently-closed").Children()[0].Text(); head != "Recently closed" {
		t.Fatalf("the tabs closed are headed %q", head)
	}
	pages, side, rows := s.box("continue"), s.box("home-side"), s.node("recently-closed").Children()[1].Children()
	if side.X != pages.X+pages.W+24 || side.Y != pages.Y || side.W != 356 || rows[0].Box().H != 36 ||
		rows[1].Box().Y != rows[0].Box().Y+36 {
		t.Fatalf("the side is at %v beside the pages %v, its rows at %v and %v", side, pages, rows[0].Box(),
			rows[1].Box())
	}
	s.clean()
	s.clickAt(rows[1].Box().Center())
	s.at(tabs.Page(guideName))
	s.click("home")
	if got := strings.Join(s.texts("recently-closed", "shut"), ", "); got != "History | just now, Examples | just now" {
		t.Fatalf("after one was opened again the tabs closed are %q", got)
	}
	s.newWindow(tabs.Home)
	s.frame()
	s.frame()
	other := s.screens[1]
	s.newTab()
	s.Open(tabs.Page(aboutName))
	s.closeTab(1)
	s.frame()
	s.frame()
	if got := other.doc.ByID("recently-closed"); got == nil || !strings.Contains(got.Text(), "About") {
		t.Fatal("another window's start page does not follow the tabs closed")
	}
	s.clearHistory()
	s.frame()
	if s.doc.ByID("recently-closed") != nil {
		t.Fatal("the history cleared, the tabs closed are still offered")
	}
	s.configure(func(c *config.Config) { c.KeepHistory = false })
	s.newTab()
	s.Open(tabs.Page(guideName))
	s.closeTab(1)
	s.frame()
	if s.doc.ByID("recently-closed") != nil {
		t.Fatal("a tab closed while no history is kept is offered again")
	}
	s.clean()
}
