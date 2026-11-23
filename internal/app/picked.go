package app

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
)

//go:embed picked.css
var pickedSheet string

const (
	pickedTokens = 640
	// pickedWidth and pickedHeight are the room the selection's card is
	// given under the pointer, and pickedGap how far under.
	pickedWidth, pickedHeight, pickedGap = 440, 300, 18
)

// action is something the model does with a selection.
type action struct {
	id, label, title, did string
	// system is what the model is told, into naming the language a
	// translation is made in.
	system func(into string) string
}

const translating = "translate"

func actions() []action {
	passage := "The passage a reader selected on a web page" + untrusted
	return []action{
		{"explain", "Explain", "In plain words", "Explained", func(string) string {
			return passage + "Explain it in plain words, in two or three short sentences."
		}},
		{"summarise", "Summarise", "In short", "Summarised", func(string) string {
			return passage + "Say what it says in one or two sentences."
		}},
		{translating, "Translate", "Translated", "Translated", func(into string) string {
			return translatorTold("", into, keepMarkdown)
		}},
	}
}

// translation is a language a selection is translated into: its tag,
// its name as its readers write it, and as the model is told it.
type translation struct{ tag, label, name string }

func translations() []translation {
	return []translation{
		{"en-GB", "English (UK)", "British English"},
		{"en-US", "English (US)", "American English"},
		{"fr", "Français", "French"},
	}
}

// translatesInto is the language a selection is translated into: the
// one chosen last, else the first of the reader's that is offered,
// else American English.
func (s *Screen) translatesInto() translation {
	offered := translations()
	for _, t := range offered {
		if t.tag == s.config.TranslateInto {
			return t
		}
	}
	for _, tag := range s.languages() {
		for _, t := range offered {
			if base, _, _ := strings.Cut(tag, "-"); t.tag == tag || t.tag == base {
				return t
			}
		}
	}
	return offered[1]
}

// pickedCard is the card of the page's selection: what the model does
// with it, what light does by itself.
type pickedCard struct {
	root *ui.Node
	// dragged is the body of the Document whose text a drag of the right
	// button selected since the last frame, text the selection the card
	// is open for and written what the model is given of it, of the page
	// at address.
	dragged                *ui.Node
	text, written, address string
	// at is where the pointer stood on the stage when the card opened.
	at geom.Vec2
	// did is the action asked of the model, asked its answer, and off
	// whether the robot was turned off.
	did   *action
	asked *inquiry
	off   bool
}

func (s *Screen) buildPicked() *ui.Node {
	s.doc.AddSheet(pickedSheet)
	s.excerpt.root = ui.Div(ui.ID("picked"), ui.Class("sheet"), ui.Attr("role", "dialog"),
		ui.Attr("aria-label", "Selected text"))
	return s.excerpt.root
}

func (s *Screen) pickedOpen() bool { return s.excerpt.root.HasClass("shown") }

func (s *Screen) closePicked() {
	p := &s.excerpt
	p.asked.stop()
	p.asked, p.did = nil, nil
	if s.pickedOpen() {
		p.root.RemoveClass("shown")
		empty(p.root)
	}
}

// followPicked opens the card for the text a drag of the right button
// selected in the page, under the pointer, closes it when that
// selection goes or the card is left, and shows what came of what the
// model was asked. A selection made any other way opens nothing.
func (s *Screen) followPicked() {
	p := &s.excerpt
	dragged := p.dragged
	p.dragged = nil
	page := s.readable()
	if page == nil || !s.helps(pickedFeature) {
		s.closePicked()
		return
	}
	selected := page.Doc.Selection().Text
	if s.pickedOpen() && (selected != p.text || s.leftBy(p.root)) {
		s.closePicked()
	}
	if dragged == page.Doc.Body() && strings.TrimSpace(selected) != "" {
		s.closePicked()
		s.openPicked(selected, markdownOf(page.Doc, page.Doc.Selection()))
	}
	if s.pickedOpen() {
		s.placePicked()
	}
	if p.asked != nil && p.asked.take() {
		s.fillPicked()
	}
}

func (s *Screen) openPicked(text, written string) {
	p := &s.excerpt
	s.closeSorting(false)
	if written == "" {
		written = strings.Join(strings.Fields(text), " ")
	}
	p.text, p.written, p.address, p.off = text, written, s.browser.Tab().Current(), false
	stage, _ := s.doc.BoxOf(s.status.Parent())
	p.at = s.win.Input().Mouse().Sub(geom.V2(stage.X, stage.Y))
	p.root.AddClass("shown")
	s.fillPicked()
	s.placePicked()
}

// placePicked puts the card under where the pointer stood, above it
// when it has no room under, and against the stage's foot when it has
// room at neither: the card grows with what the model writes.
func (s *Screen) placePicked() {
	p := &s.excerpt
	stage, _ := s.doc.BoxOf(s.status.Parent())
	height := float32(pickedHeight)
	if card, ok := s.doc.BoxOf(p.root); ok && card.H > 0 {
		height = card.H
	}
	left := min(max(p.at.X-pickedGap, 8), max(stage.W-pickedWidth-8, 8))
	top := p.at.Y + pickedGap
	if top+height > stage.H-8 {
		top = p.at.Y - pickedGap - height
	}
	if top < 8 {
		top = max(stage.H-height-8, 8)
	}
	if style := fmt.Sprintf("left: %gpx; top: %gpx", left, top); p.root.Style() != style {
		p.root.SetStyle(style)
	}
}

// do asks the model to do an action with the selection.
func (s *Screen) do(a action) {
	p := &s.excerpt
	p.asked.stop()
	p.did, p.off = &a, false
	into, then := s.translatesInto().name, ""
	if a.id == translating {
		then = rendering(into)
	}
	p.asked = s.inquireThen(a.system(into), "The passage, in Markdown:\n", p.written, then, pickedTokens)
	s.fillPicked()
}

func (s *Screen) fillPicked() {
	p := &s.excerpt
	s.refill(p.root, func() {
		title, sub := "Selected text", siteOf(p.address)
		var robot *ui.Node
		shows := p.asked != nil && !p.off
		if p.asked != nil {
			robot = robotToggle("picked-robot", p.did.did+" for you on this device.", !p.off, func() {
				p.off = !p.off
				s.fillPicked()
			})
		}
		if shows {
			title, sub = p.did.title, s.pickedStands()
		}
		p.root.Append(sheetHead(s.siteTile(p.address, "big"), "picked", title, sub, robot))
		if text := s.pickedText(); shows && text != "" {
			said := ui.Div(ui.ID("picked-text"), ui.Class("inset"), ui.Attr("dir", "auto"))
			if p.asked.err != nil {
				said.Append(ui.P(ui.Text(text)))
			} else {
				said.Append(markdown(text)...)
			}
			p.root.Append(said)
		}
		if !p.off {
			chips := ui.Div(ui.ID("picked-chips"))
			for _, a := range actions() {
				chips.Append(s.pill("picked-"+a.id, a.label, a.label+" the selection",
					p.did != nil && p.did.id == a.id, func() { s.do(a) }))
			}
			p.root.Append(ui.Div(ui.Class("part"), ui.Span(ui.Text("With the selection")), chips))
			if p.did != nil && p.did.id == translating {
				into, chosen := ui.Div(ui.ID("picked-into")), s.translatesInto()
				for _, t := range translations() {
					into.Append(s.pill("picked-into-"+t.tag, t.label, "Translate into "+t.name, t == chosen, func() {
						s.configure(func(c *config.Config) { c.TranslateInto = t.tag })
						s.do(*p.did)
					}))
				}
				p.root.Append(ui.Div(ui.Class("part"), ui.Span(ui.Text("Into")), into))
			}
		}
		var stop *ui.Node
		if shows && !p.asked.done {
			stop = ui.Button(ui.ID("picked-stop"), ui.Text("Stop"), ui.OnClick(p.asked.stop))
		}
		p.root.Append(sheetFoot([]*ui.Node{
			ui.Button(ui.ID("picked-copy"), ui.Text("Copy"), ui.OnClick(func() { s.win.SetClipboard(p.text) })),
			ui.Button(ui.ID("picked-search"), ui.Text("Search"), ui.OnClick(func() {
				text := p.text
				s.closePicked()
				s.Open(s.search(text))
			})),
		}, stop))
	})
}

func (s *Screen) pickedText() string {
	if a := s.excerpt.asked; a != nil && a.err != nil {
		return a.err.Error()
	} else if a != nil {
		return strings.TrimSpace(a.text)
	}
	return ""
}

// pickedStands says where what the model does with the selection
// stands.
func (s *Screen) pickedStands() string {
	a := s.excerpt.asked
	switch {
	case a.err != nil:
		return "The model could not do it"
	case a.stopped && a.done:
		return "Stopped"
	case a.capped:
		return "The start of your selection, which is long · on this device, nothing sent"
	}
	return "Your selection · on this device, nothing sent"
}
