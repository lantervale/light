package app

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/complete"
	"github.com/lantervale/light/pkg/tabs"
)

//go:embed complete.css
var completeSheet string

// offeredPages is the most pages of this device the list offers.
const offeredPages = 4

// offer is a row of the list: a question for the shown page when it
// has no address, else the address it opens.
type offer struct {
	id string
	// lead is what stands before the row, label what it does, words
	// what with, and where the address of a page of this device.
	lead                *ui.Node
	label, words, where string
	address             string
}

// completion is the list under the address field while it is typed in:
// what Enter does, a question for the shown page when the model answers
// those, and the pages of this device that match. It is made of what
// this device holds alone, and asks nothing of anybody.
type completion struct {
	root, keys *ui.Node
	rows       []offer
	// typed is whether the field was typed in since it took the focus,
	// and shut whether Escape closed the list since it last was.
	typed, shut bool
	// text is what the rows were made for and from what they were made
	// of; at is the highlighted row and armed the one the pointer went
	// down on, -1 for none.
	text, from string
	at, armed  int
	// value is what the field held at the last frame and carets where
	// its selection was at the last three, the latest first; step is
	// the rows the keys asked the highlight to move by since.
	value  string
	carets [3][2]int
	step   int
}

func (s *Screen) completeOpen() bool {
	return s.completing.root != nil && s.completing.root.HasClass("shown")
}

// retyped notes that the address field was typed in: the list shows
// from then, till the field is left.
func (s *Screen) retyped() { s.completing.typed, s.completing.shut = true, false }

// decided is Enter in the address field: the highlighted row is taken,
// and with none what was typed is.
func (s *Screen) decided() {
	c := &s.completing
	if s.completeOpen() && c.at >= 0 {
		s.takeOffer(c.at)
		return
	}
	s.closeComplete()
	c.typed = false
	s.submitted(s.doc.Value(s.address))
}

// takeOffer does what the row at says: a question is asked as one typed
// after a question mark is, an address is opened.
func (s *Screen) takeOffer(at int) {
	c := &s.completing
	row := c.rows[at]
	s.closeComplete()
	c.typed = false
	if row.address == "" {
		s.submitted("? " + row.words)
		return
	}
	s.Open(row.address)
}

func (s *Screen) closeComplete() {
	c := &s.completing
	c.armed = -1
	if !s.completeOpen() {
		return
	}
	c.root.RemoveClass("shown")
	empty(c.root)
	c.rows, c.text, c.from, c.at, c.step = nil, "", "", -1, 0
	s.address.RemoveAttr("aria-activedescendant")
	s.address.SetAttr("aria-expanded", "false")
	for _, key := range c.keys.Children() {
		key.SetAttr("aria-disabled", "true")
	}
}

// followComplete shows the list while the address field has the focus
// and was typed in, takes the row clicked, and closes it on Escape,
// when the field is left, and when another card opens. ui takes the
// focus off a text field at Escape and tells nobody: an open list whose
// field lost the focus to nothing, without a press, is Escape, and the
// field has the focus and what was typed again.
func (s *Screen) followComplete() {
	c := &s.completing
	in := s.win.Input()
	if s.pointedOffer(in) {
		return
	}
	if !s.doc.Focused(s.address) {
		pointing := in.Down(input.MouseLeft) || in.Released(input.MouseLeft)
		switch {
		case c.armed >= 0:
		case s.completeOpen() && !pointing && !s.focusedIn(s.doc.Body()):
			s.closeComplete()
			c.shut = true
			s.doc.Focus(s.address)
			s.editing = true
			s.locate()
			s.doc.SetValue(s.address, c.value)
			s.doc.SetSelectionRange(s.address, c.carets[0][0], c.carets[0][1])
		default:
			c.typed, c.shut = false, false
			s.closeComplete()
		}
		return
	}
	step, held := c.step, c.value == s.doc.Value(s.address)
	c.value, c.step = s.doc.Value(s.address), 0
	from, to := s.doc.SelectionRange(s.address)
	c.carets = [3][2]int{{from, to}, c.carets[0], c.carets[1]}
	typed := strings.TrimSpace(c.value)
	if !c.typed || c.shut || typed == "" || s.completeOpen() && s.covers() {
		s.closeComplete()
		return
	}
	s.fillComplete(typed)
	if step != 0 && held {
		s.stepOffer(step)
	}
	stage, field := s.view.Parent().Box(), s.field.Box()
	if style := fmt.Sprintf("left: %gpx; width: %gpx", field.X-stage.X, field.W); c.root.Style() != style {
		c.root.SetStyle(style)
	}
}

// covers reports whether a card, a panel or a menu is open that the
// list would stand over.
func (s *Screen) covers() bool {
	return s.panelOpen() || s.cardOpen() || s.choosing() || s.menu.IsOpen() || s.sheets() > 1
}

// pointedOffer takes the row the pointer went down on and came up
// from, and reports whether it took one: the list is a listbox, whose
// options ui selects at a click and tells nothing of.
func (s *Screen) pointedOffer(in *input.State) bool {
	c := &s.completing
	if !s.completeOpen() {
		return false
	}
	if in.Pressed(input.MouseLeft) {
		c.armed = s.offerAt(in.Mouse())
	}
	if c.armed < 0 || in.Down(input.MouseLeft) {
		return false
	}
	at := c.armed
	c.armed = -1
	if s.offerAt(in.Mouse()) != at {
		return false
	}
	s.takeOffer(at)
	return true
}

// offerAt is the row under at, -1 for none.
func (s *Screen) offerAt(at geom.Vec2) int {
	for i, row := range s.completing.root.Children() {
		if row.Box().Contains(at) {
			return i
		}
	}
	return -1
}

// stepOffer moves the highlight by a row, past the last to none, which
// is what was typed, and from none to the first or the last. The keys
// that ask for it moved the field's caret two frames ago, as the arrows
// do: it goes back where it was.
func (s *Screen) stepOffer(by int) {
	c := &s.completing
	n := len(c.rows) + 1
	s.highlight((c.at+1+by+n)%n - 1)
	c.carets[0] = c.carets[2]
	s.doc.SetSelectionRange(s.address, c.carets[0][0], c.carets[0][1])
}

// highlight marks the row at as the one Enter takes, -1 for none, and
// names it to the field, which keeps the focus.
func (s *Screen) highlight(at int) {
	c := &s.completing
	c.at = at
	for i, row := range c.root.Children() {
		if on, _ := row.Attr("aria-selected"); on != onOff(i == at) {
			row.SetAttr("aria-selected", onOff(i == at))
		}
	}
	if at < 0 {
		s.address.RemoveAttr("aria-activedescendant")
		return
	}
	s.address.SetAttr("aria-activedescendant", c.rows[at].id)
}

// buildComplete makes the list, closed, and the keys that move through
// it as a menu never shown: Alt with the down or up arrow. The arrows
// alone are the field's own, which a ui text field tells nobody of.
func (s *Screen) buildComplete() {
	c := &s.completing
	s.doc.AddSheet(completeSheet)
	c.root = ui.Div(ui.ID("complete"), ui.Class("sheet"), ui.Attr("role", "listbox"),
		ui.Attr("aria-label", "Suggestions"))
	c.keys = ui.Div(ui.ID("complete-keys"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Suggestions' keys"))
	for _, item := range []struct {
		name, keys string
		by         int
	}{{"Next suggestion", "Alt+ArrowDown", 1}, {"Previous suggestion", "Alt+ArrowUp", -1}} {
		c.keys.Append(ui.Div(ui.Attr("role", "menuitem"), ui.Attr("aria-keyshortcuts", item.keys),
			ui.Attr("aria-disabled", "true"), ui.Text(item.name), ui.OnClick(func() { c.step = item.by })))
	}
	s.view.Parent().Append(c.root, c.keys)
	s.address.SetAttr("aria-controls", "complete")
	s.address.SetAttr("aria-autocomplete", "list")
}

// fillComplete writes the list for what is typed, when that or what it
// is made of changed, and opens it over the other cards, which close.
func (s *Screen) fillComplete(typed string) {
	c := &s.completing
	if c.root == nil {
		s.buildComplete()
	}
	from := fmt.Sprint(s.answers(), s.engine().ID, s.mark(), s.browser.Tab().Current(), s.recorded, s.marked)
	if s.completeOpen() && typed == c.text && from == c.from {
		return
	}
	if !s.completeOpen() {
		s.menu.Close()
		s.closePanel(false)
		s.closeCard(false)
		s.closeChoices()
		s.closeSheets(c.root)
		c.root.AddClass("shown")
		s.address.SetAttr("aria-expanded", "true")
		for _, key := range c.keys.Children() {
			key.RemoveAttr("aria-disabled")
		}
	}
	at := c.at
	if typed != c.text {
		at = -1
	}
	c.rows, c.text, c.from = s.offers(typed), typed, from
	empty(c.root)
	for _, row := range c.rows {
		said := strings.TrimSpace(row.label + " " + row.words + " " + row.where)
		node := ui.Div(ui.ID(row.id), ui.Class("offer"), ui.Attr("role", "option"), ui.Attr("aria-label", said),
			row.lead)
		if row.label != "" {
			node.Append(ui.Span(ui.Class("label"), ui.Text(row.label)))
		}
		node.Append(ui.Span(ui.Class("words"), ui.Text(row.words)))
		if row.where != "" {
			node.Append(ui.Span(ui.Class("where"), ui.Text(row.where)))
		}
		c.root.Append(node)
	}
	s.highlight(min(at, len(c.rows)-1))
}

// answers reports whether the model answers questions about the shown
// page.
func (s *Screen) answers() bool { return s.helps(answersFeature) && s.readable() != nil }

// offers are the rows for what is typed: a question for the shown page
// when the model answers those and no address is typed, what Enter
// opens, read as the address field reads it, and the pages visited and
// kept that hold its words.
func (s *Screen) offers(typed string) []offer {
	words, query := typed, ""
	address := tabs.Resolve(typed, func(sought string) string {
		query = sought
		return s.search(sought)
	})
	var rows []offer
	if question, asks := strings.CutPrefix(typed, "?"); s.answers() && query != "" {
		if question = strings.TrimSpace(question); asks && question != "" {
			words, query, address = question, question, s.search(question)
		}
		rows = append(rows, offer{id: "complete-ask", lead: lead(glyph("row-model")), label: "Ask this page:",
			words: words})
	}
	enter := offer{id: "complete-enter", lead: lead(glyph("open")), label: "Go to", words: shownAs(address),
		address: address}
	if query != "" && address == s.search(query) {
		enter.lead, enter.label, enter.words = lead(ui.Img(ui.Attr("src", s.mark()), ui.Attr("alt", ""))),
			"Search "+s.engine().Name+":", query
	}
	rows = append(rows, enter)
	for i, page := range complete.Match(words, s.local(address), offeredPages) {
		row := offer{id: "complete-page-" + strconv.Itoa(i), lead: s.siteTile(page.Address),
			words: page.Title, where: shownAs(page.Address), address: page.Address}
		if row.words == "" {
			row.words, row.where = row.where, ""
		}
		rows = append(rows, row)
	}
	return rows
}

// lead is the place of a row's mark, as wide as a site's tile.
func lead(mark *ui.Node) *ui.Node {
	return ui.Span(ui.Class("lead"), ui.Attr("aria-hidden", "true"), mark)
}

// local are the pages this device holds but the one at address: the
// visits of its history and its bookmarks.
func (s *Screen) local(address string) []complete.Page {
	var pages []complete.Page
	for _, day := range s.visits.Days("") {
		for _, v := range day.Visits {
			if v.Address != address {
				pages = append(pages, complete.Page{Address: v.Address, Title: v.Title, At: v.At})
			}
		}
	}
	for _, mark := range s.marks.All() {
		if mark.Address != address {
			pages = append(pages, complete.Page{Address: mark.Address, Title: mark.Name, At: mark.Added, Kept: true})
		}
	}
	return pages
}
