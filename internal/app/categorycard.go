package app

import (
	"strconv"
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/categories"
)

// sortingNotch is how far from the right edge of the categories' card
// its notch is.
const sortingNotch = 26

// tag is the glyph of the card's tile.
const tag = `<path d="M3 3h8.5l9.5 9.5-8.5 8.5L3 11.5z" fill="none" stroke="currentColor" stroke-width="2" ` +
	`stroke-linejoin="round"/><circle cx="8" cy="8" r="1.4" fill="currentColor"/>`

func (s *Screen) sortingOpen() bool { return s.sorting.card.HasClass("shown") }

// sheets is how many cards are open over the stage.
func (s *Screen) sheets() int {
	open := 0
	for _, n := range s.view.Parent().Children() {
		if n.HasClass("sheet") && n.HasClass("shown") {
			open++
		}
	}
	return open
}

// openSorting shows the card of the shown page's category, or the
// settings' categories when the shown page is not one that is sorted.
func (s *Screen) openSorting() {
	if !s.helps(sortingFeature) {
		return
	}
	page := s.sortedPage(s.browser.Tab())
	if page == nil {
		s.closeChoices()
		s.Open(configPage)
		if s.sorting.held != nil {
			s.doc.ScrollIntoView(s.sorting.held)
		}
		return
	}
	s.menu.Close()
	s.closePanel(false)
	s.closeCard(false)
	s.closeChoices()
	s.closeSheets(s.sorting.card)
	s.list.RemoveClass("shown")
	s.recent.RemoveClass("shown")
	g := &s.sorting
	g.address = page.Address
	g.card.AddClass("shown")
	g.others = s.sheets()
	s.fillSorting()
	s.hangSorting()
	s.doc.Focus(s.doc.ByID("category-done"))
}

// closeSorting closes the card; refocus gives the label the focus.
func (s *Screen) closeSorting(refocus bool) {
	if !s.sortingOpen() {
		return
	}
	g := &s.sorting
	g.card.RemoveClass("shown")
	empty(g.card)
	if refocus && g.label.HasClass("shown") {
		s.doc.Focus(g.label)
	}
}

// sortingAnchor is what the card hangs from: the page's label, and the
// address field's star for a page that has none.
func (s *Screen) sortingAnchor() *ui.Node {
	if s.sorting.label.HasClass("shown") {
		return s.sorting.label
	}
	return s.star
}

// hangSorting places the card so its notch points at the middle of
// what it hangs from.
func (s *Screen) hangSorting() { s.hang(s.sorting.card, s.sortingAnchor(), sortingNotch) }

// followSorting keeps the open card under what it hangs from, shows in
// it what changed of the page's judgment, and closes it when it is
// left, when its page is, or when another card opens.
func (s *Screen) followSorting() {
	if !s.sortingOpen() {
		return
	}
	g := &s.sorting
	page := s.sortedPage(s.browser.Tab())
	if page == nil || page.Address != g.address || s.choosing() || s.cardOpen() || s.panelOpen() ||
		s.sheets() > g.others || s.leftBy(g.card, s.sortingAnchor()) {
		s.closeSorting(false)
		return
	}
	g.others = s.sheets()
	s.hangSorting()
	if g.filled != g.changes {
		s.fillSorting()
	}
}

// fillSorting writes the card: the page's category and its site, the
// robot that takes the model's judgment off and puts it back, why, the
// categories with the page's own chosen, what adds one, and what keeps
// the category for the whole site.
func (s *Screen) fillSorting() {
	g := &s.sorting
	g.filled = g.changes
	page := s.sortedPage(s.browser.Tab())
	j := g.judged[page]
	if j == nil {
		return
	}
	list := s.sorted.list
	v, judged := s.verdictOf(page)
	s.refill(g.card, func() {
		title, why, tile := "not judged", "The model's judgment was taken off this page.", categories.Default().Mark
		switch {
		case judged:
			title, why, tile = v.Name, whyOf(v), v.Mark
		case j.pending:
			title, why = "being read", "The model on this device is reading the page."
		case !j.off:
			why = "The model on this device could not judge this page."
		}
		head := ui.Div(ui.Class("head"),
			ui.Span(ui.Class("site", "big"), ui.Style("background: "+tile.Colour),
				ui.Svg(tag, ui.Attr("viewBox", "0 0 24 24"), ui.Attr("aria-hidden", "true"))),
			ui.Div(ui.Class("named"), ui.Strong(ui.ID("category-title"), ui.Text("This page: "+title)),
				ui.Span(ui.ID("category-site"), ui.Text(siteOf(j.address)))))
		if j.off || judged && v.By == categories.ByModel {
			head.Append(robotToggle("category-robot", judgedHere, !j.off, func() {
				j.off = !j.off
				g.changes++
				s.fillSorting()
			}))
		}
		rows := ui.Div(ui.ID("category-list"), ui.Attr("role", "group"), ui.Attr("aria-label", "Your categories"))
		for i, c := range list.All() {
			rows.Append(s.sortingRow(i, c, judged && c.Name == v.Name))
		}
		field := ui.Input(ui.ID("category-new"), ui.Placeholder("recipes without a life story first"),
			ui.Attr("aria-label", "Describe the pages that belong in a new category"))
		add := func() {
			if _, added := list.Add(s.doc.Value(field)); added {
				s.resorted()
				g.changes++
				s.fillSorting()
				s.doc.Focus(s.doc.ByID("category-new"))
			}
		}
		ui.OnSubmit(add).Apply(field)
		_, always := list.Site(j.address)
		site := ui.Button(ui.ID("category-site-always"), ui.Attr("aria-pressed", strconv.FormatBool(always)),
			ui.Text("Always for this site"), ui.OnClick(func() { s.keepForSite(j.address, v.Name, always) }))
		if always {
			site.Append(glyph("check"))
		}
		enable(site, judged)
		g.card.Append(ui.Span(ui.Class("notch")), head,
			ui.P(ui.ID("category-reason"), ui.Class("inset"), ui.Text(why)),
			ui.Div(ui.Class("part", "listing"), ui.Span(ui.Text("Your categories · first match wins")), rows),
			ui.Div(ui.Class("part"), ui.Span(ui.Text("Add a category: describe the pages that belong in it")),
				ui.Div(ui.Class("adding"), field, ui.Button(ui.ID("category-add"), ui.Text("+ Add"), ui.OnClick(add)))),
			sheetFoot([]*ui.Node{site},
				ui.Button(ui.ID("category-done"), ui.Text("Done"), ui.OnClick(func() { s.closeSorting(true) }))))
	}, "category-done")
}

// sortingRow is a category of the card: its mark, its name, what
// belongs in it and what light does with its pages. A click has the
// page be of it, and one on the page's own takes that correction back.
func (s *Screen) sortingRow(i int, c categories.Category, matched bool) *ui.Node {
	name := ui.Span(ui.Class("name"), ui.Span(ui.Text(c.Name)))
	if matched {
		name.Append(glyph("check"))
	}
	return ui.Button(ui.ID("category-row-"+strconv.Itoa(i)), ui.Class("category"),
		ui.Attr("aria-pressed", strconv.FormatBool(matched)),
		ui.Attr("aria-label", c.Name+": "+c.Description+". "+c.Action.String()),
		markOf(c.Mark), ui.Span(ui.Class("text"), name, ui.Span(ui.Class("what"), ui.Text(c.Description))),
		ui.Span(ui.Class("does"), ui.Text(c.Action.String())),
		ui.OnClick(func() { s.correct(c.Name, matched) }))
}

// correct has the card's page be of the category name, whatever the
// model made of it; a click on the category it is of already takes a
// correction of its reader's back.
func (s *Screen) correct(name string, matched bool) {
	g := &s.sorting
	list := s.sorted.list
	chosen, corrected := list.Chosen(g.address)
	corrected = corrected && chosen.By == categories.ByPage
	switch {
	case matched && corrected:
		list.Forget(categories.Choice{Address: pageKey(g.address)})
	case matched:
		return
	default:
		list.ChoosePage(g.address, name)
	}
	s.sortedAgain()
}

// pageKey is an address as a correction is kept by.
func pageKey(address string) string {
	page, _, _ := strings.Cut(address, "#")
	return page
}

// keepForSite keeps the category name for every page of the site of
// address, or lets go of the one kept when always.
func (s *Screen) keepForSite(address, name string, always bool) {
	list := s.sorted.list
	if always {
		list.ForgetSite(address)
	} else if list.ChooseSite(address, name) {
		list.Forget(categories.Choice{Address: pageKey(address)})
	}
	s.sortedAgain()
}

// sortedAgain has a choice of the reader's written, the pages it
// changes counted under their new category, and shown in the card.
func (s *Screen) sortedAgain() {
	g := &s.sorting
	s.resorted()
	g.changes++
	for page, j := range g.judged {
		j.passed = false
		if v, ok := s.verdictOf(page); ok {
			s.count(j.address, v.Name)
		}
	}
	if s.sortingOpen() {
		s.fillSorting()
	}
}
