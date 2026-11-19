package app

import (
	"strings"

	"github.com/lantervale/ember/ui"
)

// answerTokens is the most tokens an answer from a page has.
const answerTokens = 256

// answerCard is the card an answer from the page comes in, hanging
// from the end of the address field.
type answerCard struct {
	root *ui.Node
	// question is what was asked, of the page at address.
	question, address string
	asked             *inquiry
	// off is whether the robot was turned off: the words are then for
	// the search engine. source is the sentence of the page the answer
	// is from, found whether there is one, and sought whether it was
	// looked for.
	off           bool
	source        passage
	found, sought bool
}

func (s *Screen) buildAnswer() *ui.Node {
	s.replying.root = ui.Div(ui.ID("answer"), ui.Class("sheet"), ui.Attr("role", "dialog"),
		ui.Attr("aria-label", "Answer from this page"), ui.OnBack(func(e ui.Event) {
			s.closeAnswer(true)
			e.StopPropagation()
		}))
	return s.replying.root
}

func (s *Screen) answerOpen() bool { return s.replying.root.HasClass("shown") }

// submitted takes what was typed in the address field at Enter: a
// question, which starts with a question mark, is answered from the
// shown page when the model helps with that; anything else is opened,
// words by the search engine.
func (s *Screen) submitted(typed string) {
	question, asks := strings.CutPrefix(strings.TrimSpace(typed), "?")
	if question = strings.TrimSpace(question); !asks || question == "" || !s.helps(answersFeature) ||
		s.readable() == nil {
		s.Open(s.Resolve(typed))
		return
	}
	s.answerFrom(question)
}

// answerFrom asks the model the question of the shown page, and opens
// the card its answer comes in.
func (s *Screen) answerFrom(question string) {
	page := s.readable()
	s.menu.Close()
	s.closePanel(false)
	s.closeCard(false)
	s.closeChoices()
	s.closeSheets(s.replying.root)
	c := &s.replying
	c.asked.stop()
	c.question, c.address, c.off, c.found, c.sought = question, s.browser.Tab().Current(), false, false, false
	c.asked = s.inquire("You answer a reader's question about a web page. The page's text"+untrusted+
		"Answer from it alone. First line: the answer in a few words. Second line: one or two sentences that say "+
		"more. If the page does not say, say that it does not.",
		"Question: "+question+"\nThe page:\n", pageText(page.Doc), answerTokens)
	c.root.AddClass("shown")
	s.fillAnswer()
	s.hang(c.root, s.star, panelNotch)
	s.doc.Focus(s.doc.ByID("answer-stop"))
}

// closeAnswer closes the card and ends an answer that was coming;
// refocus gives the address field the focus.
func (s *Screen) closeAnswer(refocus bool) {
	if !s.answerOpen() {
		return
	}
	c := &s.replying
	c.asked.stop()
	c.root.RemoveClass("shown")
	empty(c.root)
	s.showNavigation()
	if refocus {
		s.doc.Focus(s.address)
	}
}

// followAnswer shows what came of the answer since the last frame, the
// sentence it is from once it is whole, and closes the card when it is
// left or its page is.
func (s *Screen) followAnswer() {
	if !s.answerOpen() {
		return
	}
	c := &s.replying
	if s.browser.Tab().Current() != c.address || s.leftBy(c.root, s.field) {
		s.closeAnswer(false)
		return
	}
	s.hang(c.root, s.star, panelNotch)
	if asked := "? " + c.question; !s.editing && s.doc.Value(s.address) != asked {
		s.doc.SetValue(s.address, asked)
	}
	if !c.asked.take() {
		return
	}
	if c.asked.done && c.asked.err == nil && !c.asked.stopped && !c.sought {
		c.sought = true
		if page := s.readable(); page != nil {
			c.source, c.found = sourceOf(page.Doc, c.asked.text)
		}
		s.showSource()
	}
	s.fillAnswer()
}

// showSource selects the sentence the answer is from in the page and
// brings it into view while the robot is on, and selects nothing while
// it is off. The selection's card is not opened for it.
func (s *Screen) showSource() {
	c := &s.replying
	page := s.readable()
	if page == nil || !c.found {
		return
	}
	if c.off {
		page.Doc.ClearSelection()
		return
	}
	page.Doc.Select(c.source.from, c.source.to)
	page.Doc.ScrollIntoView(c.source.from.Node.Parent())
}

func (s *Screen) fillAnswer() {
	c := &s.replying
	s.refill(c.root, func() {
		engine := s.engine().Name
		search := ui.Button(ui.ID("answer-search"), ui.OnClick(func() {
			question := c.question
			s.closeAnswer(false)
			s.Open(s.search(question))
		}))
		robot := robotToggle("answer-robot", "Answered from this page, on this device.", !c.off, func() {
			c.off = !c.off
			s.showSource()
			s.fillAnswer()
		})
		if c.off {
			search.SetText("Search")
			c.root.Append(ui.Span(ui.Class("notch")),
				sheetHead(s.siteTile(c.address, "big"), "answer", c.question,
					"Not sent anywhere. Search sends these words to "+engine+".", robot),
				sheetFoot(nil, search))
			return
		}
		title, detail := headline(c.asked.text)
		if c.asked.err != nil {
			title, detail = "", c.asked.err.Error()
		}
		if title == "" {
			title = c.question
		}
		c.root.Append(ui.Span(ui.Class("notch")),
			sheetHead(s.siteTile(c.address, "big"), "answer", title, s.answerStands(), robot))
		if detail != "" {
			c.root.Append(ui.P(ui.ID("answer-text"), ui.Class("inset"), ui.Text(detail)))
		}
		search.SetText("Search " + engine)
		last := ui.Button(ui.ID("answer-done"), ui.Text("Done"), ui.OnClick(func() { s.closeAnswer(true) }))
		if !c.asked.done {
			last = ui.Button(ui.ID("answer-stop"), ui.Text("Stop"), ui.OnClick(c.asked.stop))
		}
		c.root.Append(sheetFoot([]*ui.Node{search}, last))
	}, "answer-done", "answer-search")
}

// answerStands says where the answer stands: being read, stopped,
// failed, or from the page, with whether its sentence was found.
func (s *Screen) answerStands() string {
	a := s.replying.asked
	switch {
	case a.err != nil:
		return "The model could not answer"
	case a.stopped && a.done:
		return "Stopped"
	case !a.done:
		return "Reading this page, on this device"
	}
	from := "From this page"
	if a.capped {
		from = "From the start of this page, which is long"
	}
	if s.replying.found {
		return from + " · source highlighted"
	}
	return from + " · no matching sentence was found"
}

// sheetHead is a card's first row: a tile, a title and what stands
// under it, named after the card, then what is at its right end.
func sheetHead(tile *ui.Node, name, title, sub string, end *ui.Node) *ui.Node {
	head := ui.Div(ui.Class("head"), tile, ui.Div(ui.Class("named"),
		ui.Strong(ui.ID(name+"-title"), ui.Text(title)), ui.Span(ui.ID(name+"-sub"), ui.Text(sub))))
	if end != nil {
		head.Append(end)
	}
	return head
}

// askAbout readies the address field for a question about the page.
func (s *Screen) askAbout() {
	s.doc.Focus(s.address)
	s.editing = true
	s.locate()
	s.doc.SetValue(s.address, "? ")
	s.doc.SetSelectionRange(s.address, 2, 2)
}
