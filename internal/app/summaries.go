package app

import (
	"slices"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/model"
)

const (
	summariesFeature = "summaries"
	summaryTokens    = 64
)

// followSummaries sums up the page the shown tab showed once it shows
// another or is closed, and keeps the summaries the model wrote since
// the last frame.
func (s *Screen) followSummaries() {
	if page := s.readable(); page != s.read {
		s.digest(s.read)
		s.read = page
	}
	s.digested()
}

// digest asks the model, when nobody waits for it, what a page that
// was left says in a line, unless its visit says so already.
func (s *Screen) digest(page *web.Page) {
	if page == nil || s.ended || !s.helps(summariesFeature) || s.digests[page.Address] != nil {
		return
	}
	if v, visited := s.visits.Last(page.Address); !visited || v.Summary != "" {
		return
	}
	text := pageText(page.Doc)
	if text == "" {
		return
	}
	if s.digests == nil {
		s.digests = map[string]*consulted{}
	}
	s.digests[page.Address] = s.consult(model.Request{
		System: "You sum up web pages. A page's title and text" + untrusted +
			"Say what the page says in one sentence of at most twenty words. Give that sentence only.",
		MaxTokens: summaryTokens, Priority: model.Background,
	}, "The page:\n", page.Title+"\n"+text)
}

// digested keeps each summary that came with the last visit of its
// page.
func (p *Profile) digested() {
	for address, asked := range p.digests {
		answer, came := asked.take()
		if !came {
			continue
		}
		delete(p.digests, address)
		if summary := line(answer.text, summaryLength); answer.err == nil && summary != "" &&
			p.visits.Summarise(address, summary) {
			p.rewrite()
		}
	}
}

// summarising reports whether the start page shows the summaries.
func (s *Screen) summarising() bool { return s.helps(summariesFeature) && !s.config.HideSummaries }

// summed is the card of a visit with what its page says under what it
// held, when the summaries show and it has one.
func (s *Screen) summed(v history.Visit, card *ui.Node) *ui.Node {
	if v.Summary == "" || !s.summarising() {
		return card
	}
	top := ui.Span(ui.Class("top"))
	for _, part := range slices.Clone(card.Children()) {
		card.Remove(part)
		top.Append(part)
	}
	card.AddClass("summed")
	card.Append(top, ui.Span(ui.Class("summary"), ui.Text(v.Summary)))
	return card
}

// summing is the heading of the last pages visited with the robot that
// shows and hides their summaries, while the model helps with them;
// cards that say what their pages say are one under the other.
func (s *Screen) summing(head *ui.Node) *ui.Node {
	if !s.helps(summariesFeature) {
		return head
	}
	robot := robotToggle("continue-robot", "One-line summaries, written on this device when a page is closed.",
		s.summarising(), func() {
			s.configure(func(c *config.Config) { c.HideSummaries = !c.HideSummaries })
			if s.relist != nil {
				s.relist()
				s.doc.Focus(s.doc.ByID("continue-robot"))
			}
		})
	ink := glyph("robot-tile")
	ink.AddClass("ink")
	robot.Append(ink)
	head.Append(robot)
	for _, v := range s.lastVisits(homeCards) {
		toggle(head, "summed", head.HasClass("summed") || v.Summary != "" && s.summarising())
	}
	return head
}
