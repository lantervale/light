package app

import (
	"fmt"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/bookmarks"
	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/tabs"
)

const (
	// demo is the page of the web the start page offers.
	demo = "https://csszengarden.com"
	// homeChips is how many bookmarks and folders the start page shows,
	// and homeCards how many of the last pages visited.
	homeChips = 8
	homeCards = 4
)

// homePage is the start page: the logo, the search field, then what
// light holds of its user's, the bookmarks and the last pages visited,
// and its own pages in a row of links. Nothing is fetched to show it: a
// site's tile is its icon only when light has it already.
func homePage(s *Screen) []*ui.Node {
	search := ui.Input(ui.ID("search"), ui.Type("search"), ui.Placeholder(s.searches()), ui.Attr("autofocus", ""))
	ui.On(search, ui.EventSubmit, func() { s.Open(s.Resolve(s.doc.Value(search))) })
	mark := ui.Img(ui.ID("search-mark"), ui.Attr("src", s.mark()), ui.Attr("alt", ""))
	s.fields = append(s.fields, func() {
		search.SetAttr("placeholder", s.searches())
		mark.SetAttr("src", s.mark())
	})
	held := ui.Div(ui.ID("held"))
	fill := func() {
		s.visitsSeen = s.recorded
		empty(held)
		held.Append(s.homeBookmarks()...)
		held.Append(s.homeVisits()...)
		held.Append(s.homeSorted()...)
	}
	s.relist, s.relistMarks = fill, fill
	fill()
	links := ui.Nav(ui.ID("own-links"))
	link := func() {
		empty(links)
		links.Append(ui.Span(ui.Text("light's own pages:")))
		for _, page := range s.lists()[1:] {
			links.Append(s.anchor(tabs.Page(page.name), ui.ID("go-"+page.name), ui.Text(page.title)))
		}
		links.Append(s.anchor(demo, ui.ID("go-demo"), ui.Text("CSS Zen Garden")))
	}
	s.relink = link
	link()
	return []*ui.Node{s.switcher(), ui.Div(ui.ID("wordmark"), ui.Text("light")),
		ui.Div(ui.ID("search-box"), mark, search), held,
		ui.P(ui.ID("home-note"), ui.Text("Made from what is on this device. Nothing was fetched to show this page.")),
		links}
}

// homeBookmarks is the start page's bookmarks: those in no folder as
// chips, then a chip a folder that holds some, and a link to them all;
// nothing when none is kept.
func (s *Screen) homeBookmarks() []*ui.Node {
	if s.marks.Len() == 0 {
		return nil
	}
	chips := ui.Div(ui.Class("chips"))
	for _, group := range s.marks.Groups() {
		for _, chip := range s.chipsOf(group) {
			if len(chips.Children()) < homeChips {
				chips.Append(chip)
			}
		}
	}
	return []*ui.Node{ui.Section(ui.ID("home-bookmarks"),
		ui.Div(ui.Class("head"), ui.H2(ui.Text(bookmarked)),
			s.anchor(bookmarksPage, ui.ID("all-bookmarks"), ui.Text("All bookmarks"))),
		chips)}
}

// chipsOf is a chip a bookmark of those in no folder, and one chip for
// a folder that holds some, which opens the page that lists them.
func (s *Screen) chipsOf(group bookmarks.Group) []*ui.Node {
	if group.Folder != "" {
		if len(group.Bookmarks) == 0 {
			return nil
		}
		return []*ui.Node{s.chip(bookmarksPage, group.Folder,
			ui.Span(ui.Class("site", "shelf"), ui.Attr("aria-hidden", "true"), glyph("folder")))}
	}
	chips := make([]*ui.Node, len(group.Bookmarks))
	for i, mark := range group.Bookmarks {
		chips[i] = s.chip(mark.Address, mark.Name, s.siteTile(mark.Address))
	}
	return chips
}

// chip is a link of the start page to address: a tile, then its name.
func (s *Screen) chip(address, name string, tile *ui.Node) *ui.Node {
	return s.pressed(address, ui.Class("chip"), tile, ui.Span(ui.Class("name"), ui.Text(name)))
}

// pressed is a button that opens address as a link of light's does:
// where the keys held at its click say, named in the status bubble
// while the pointer is over it.
func (s *Screen) pressed(address string, args ...ui.Arg) *ui.Node {
	return ui.Button(append([]ui.Arg{ui.Attr(linked, address),
		ui.OnClick(func() { s.activate(address) }),
		ui.OnPointerEnter(func() { s.point(address) }),
		ui.OnPointerLeave(func() { s.point("") })}, args...)...)
}

// homeVisits is the start page's last pages visited, each address once,
// the newest first: a card each, with its site's tile, its title, its
// site and when it was visited; nothing for an empty history.
func (s *Screen) homeVisits() []*ui.Node {
	visits := s.lastVisits(homeCards)
	if len(visits) == 0 {
		return nil
	}
	now := s.opts.Now()
	cards := ui.Div(ui.Class("cards"))
	for _, v := range visits {
		title := v.Title
		if title == "" {
			title = shownAs(v.Address)
		}
		cards.Append(s.summed(v, s.pressed(v.Address, ui.Class("card"), s.siteTile(v.Address, "big"),
			ui.Span(ui.Class("text"), ui.Strong(ui.Text(title)),
				ui.Span(ui.Class("where"), ui.Text(siteOf(v.Address)+" · "+ago(v.At, now)))))))
	}
	head := s.summing(ui.Div(ui.Class("head"), ui.H2(ui.Text("Continue"))))
	return []*ui.Node{ui.Section(ui.ID("continue"), head, cards)}
}

// lastVisits is the last n pages of the history, each address once.
func (s *Screen) lastVisits(n int) []history.Visit {
	var last []history.Visit
	seen := map[string]bool{}
	for _, day := range s.visits.Days("") {
		for _, v := range day.Visits {
			if seen[v.Address] {
				continue
			}
			seen[v.Address] = true
			if last = append(last, v); len(last) == n {
				return last
			}
		}
	}
	return last
}

// ago says when at was, seen from now: how long ago within the day,
// then yesterday, the day of the week within the week, and the date.
func ago(at, now time.Time) string {
	at = at.In(now.Location())
	passed := now.Sub(at)
	days := daysBetween(at, now)
	switch {
	case passed < time.Minute:
		return "just now"
	case passed < time.Hour:
		return fmt.Sprintf("%d min ago", int(passed.Minutes()))
	case days == 0:
		return fmt.Sprintf("%d h ago", int(passed.Hours()))
	case days == 1:
		return "yesterday"
	case days < 7:
		return at.Weekday().String()
	case at.Year() == now.Year():
		return at.Format("2 January")
	}
	return at.Format("2 January 2006")
}

// daysBetween is how many midnights passed from at to now.
func daysBetween(at, now time.Time) int {
	day := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	return int(day(now).Sub(day(at)).Hours() / 24)
}
