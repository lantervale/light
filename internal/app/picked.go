package app

import (
	"fmt"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/identity"
)

const (
	pickedTokens = 320
	// pickedWidth and pickedHeight are the room the selection's card is
	// given under the pointer, and pickedGap how far under.
	pickedWidth, pickedHeight, pickedGap = 440, 300, 18
)

// action is something the model does with a selection.
type action struct {
	id, label, title, did string
	// system is what the model is told, which language names the
	// language of its reader in.
	system func(language string) string
}

func actions() []action {
	passage := "The passage a reader selected on a web page" + untrusted
	return []action{
		{"explain", "Explain", "In plain words", "Explained", func(string) string {
			return passage + "Explain it in plain words, in two or three short sentences."
		}},
		{"summarise", "Summarise", "In short", "Summarised", func(string) string {
			return passage + "Say what it says in one or two sentences."
		}},
		{"translate", "Translate", "Translated", "Translated", func(language string) string {
			return passage + "Translate it into the language whose tag is " + language + ". Give the translation only."
		}},
	}
}

// pickedCard is the card of the page's selection: what the model does
// with it, what light does by itself.
type pickedCard struct {
	root *ui.Node
	// seen is the selection the card was last opened or left closed
	// for, text the one it is open for, of the page at address.
	seen, text, address string
	// did is the action asked of the model, asked its answer, and off
	// whether the robot was turned off.
	did   *action
	asked *inquiry
	off   bool
}

func (s *Screen) buildPicked() *ui.Node {
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

// followPicked opens the card for a selection of the page once the
// button that made it is up, under the pointer, closes it when the
// selection goes or the card is left, and shows what came of what the
// model was asked.
func (s *Screen) followPicked() {
	p := &s.excerpt
	page := s.readable()
	if page == nil || !s.helps(pickedFeature) {
		p.seen = ""
		s.closePicked()
		return
	}
	if s.pickedOpen() && s.leftBy(p.root) {
		s.closePicked()
	}
	in := s.win.Input()
	if selected := page.Doc.Selection().Text; selected != p.seen && !in.Down(input.MouseLeft) {
		p.seen = selected
		s.closePicked()
		if strings.TrimSpace(selected) != "" {
			s.openPicked(selected)
		}
	}
	if p.asked != nil && p.asked.take() {
		s.fillPicked()
	}
}

func (s *Screen) openPicked(text string) {
	p := &s.excerpt
	s.closeSorting(false)
	p.text, p.address, p.off = text, s.browser.Tab().Current(), false
	stage, _ := s.doc.BoxOf(s.status.Parent())
	at := s.win.Input().Mouse().Sub(geom.V2(stage.X, stage.Y))
	left := min(max(at.X-pickedGap, 8), max(stage.W-pickedWidth-8, 8))
	top := at.Y + pickedGap
	if top+pickedHeight > stage.H {
		top = max(at.Y-pickedGap-pickedHeight, 8)
	}
	p.root.SetStyle(fmt.Sprintf("left: %gpx; top: %gpx", left, top))
	p.root.AddClass("shown")
	s.fillPicked()
}

// do asks the model to do an action with the selection.
func (s *Screen) do(a action) {
	p := &s.excerpt
	p.asked.stop()
	language := identity.Languages(s.opts.Languages)[0]
	p.did, p.off = &a, false
	p.asked = s.inquire(a.system(language), "The passage:\n", strings.Join(strings.Fields(p.text), " "), pickedTokens)
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
			p.root.Append(ui.P(ui.ID("picked-text"), ui.Class("inset"), ui.Text(text)))
		}
		if !p.off {
			chips := ui.Div(ui.ID("picked-chips"))
			for _, a := range actions() {
				chips.Append(s.pill("picked-"+a.id, a.label, a.label+" the selection",
					p.did != nil && p.did.id == a.id, func() { s.do(a) }))
			}
			p.root.Append(ui.Div(ui.Class("part"), ui.Span(ui.Text("With the selection")), chips))
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
