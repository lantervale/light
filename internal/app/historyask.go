package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
	"github.com/lantervale/light/pkg/recall"
)

const (
	// excerptSize is the most bytes of a passage a match shows, and
	// markedWord the least letters of a word of the question that is
	// marked in it: both to be tuned with recall's constants.
	excerptSize = 280
	markedWord  = 4
	// lastWeek is how many days back "Last week" keeps.
	lastWeek = 7
	// clockHands is the drawing of the tile of a question asked of the
	// history.
	clockHands = `<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>`
)

// historyAsk is the history page asked by meaning: the question its
// field was given, in a card over the pages that said something like
// it, and what forgets them under the list.
type historyAsk struct {
	filter, list, card, foot *ui.Node
	// plain lists the visits as the page does without a model.
	plain func()
	// on is whether the page shows any of this.
	on bool
	// question is what was asked, vector what the model made of it,
	// pending whether it is still at it and failed why it could not;
	// asked counts the questions, for the answer of an older one.
	question string
	vector   []float32
	pending  bool
	failed   error
	asked    uint64
	// meaning is whether the robot is on, week whether only the last
	// week's visits are kept, and confirming whether the card asks
	// before it forgets everything.
	meaning, week, confirming bool
	// seen is the changes of what pages said the page shows.
	seen uint64
}

// askable has the history page take a question: Enter in its field asks
// what the pages said, while the model remembers it. With the model or
// the feature off the page is as it is without one.
func (s *Screen) askable(body []*ui.Node, filter, list *ui.Node) []*ui.Node {
	a := &historyAsk{filter: filter, list: list, plain: s.relist, on: s.remembers(), seen: s.memory.changed}
	a.card = ui.Div(ui.ID("history-ask"))
	a.foot = ui.Div(ui.ID("history-remembers"))
	s.asking = a
	ui.On(filter, ui.EventSubmit, func() { s.askHistory(strings.TrimSpace(s.doc.Value(filter))) })
	ui.On(filter, ui.EventInput, func() { s.dropQuestion() })
	s.relist = func() {
		if s.asking == a && a.question != "" {
			s.fillAsk()
			return
		}
		a.plain()
	}
	if a.on {
		s.fillRemembers()
		body = append(body, a.foot)
	}
	return body
}

// inView reports whether n is in the shown page.
func (s *Screen) inView(n *ui.Node) bool {
	for ; n != nil; n = n.Parent() {
		if n == s.view {
			return true
		}
	}
	return false
}

// followAsk shows or takes away what the history page has of the
// feature as it is turned on and off, and shows the matches again once
// what pages said changed.
func (s *Screen) followAsk() {
	a := s.asking
	switch {
	case a == nil:
		return
	case !s.inView(a.list):
		s.asking = nil
		return
	}
	if on := s.remembers(); on != a.on {
		if a.on = on; !on {
			a.question, a.vector, a.pending, a.confirming = "", nil, false, false
			a.plain()
		}
		s.fillAsk()
	}
	if a.on && a.seen != s.memory.changed {
		s.fillAsk()
	}
}

// askHistory asks the pages visited a question: the model embeds it off
// the frame, the visits whose title or address hold its words listed
// meanwhile.
func (s *Screen) askHistory(question string) {
	a := s.asking
	if a == nil || !a.on {
		return
	}
	if question == "" {
		s.dropQuestion()
		return
	}
	a.question, a.meaning, a.confirming = question, true, false
	a.vector, a.failed, a.pending = nil, nil, true
	a.asked++
	asked, embedder, ctx := a.asked, s.opts.Model, s.memory.ctx
	go func() {
		vectors, err := embedder.Embed(ctx, model.Texts{Texts: []string{question}, Kind: model.Query,
			Priority: model.Interactive})
		s.landed <- func() {
			if s.asking != a || a.asked != asked {
				return
			}
			if a.pending, a.failed = false, err; err == nil && len(vectors) == 1 {
				a.vector = vectors[0]
			}
			s.fillAsk()
		}
	}()
	s.fillAsk()
}

// dropQuestion leaves the question: the field filters the visits again.
func (s *Screen) dropQuestion() {
	a := s.asking
	if a == nil || a.question == "" {
		return
	}
	a.question, a.vector, a.pending, a.failed = "", nil, false, nil
	a.asked++
	a.plain()
	s.fillAsk()
}

// byMeaning reports whether the list shows what the pages said.
func (a *historyAsk) byMeaning() bool { return a.question != "" && a.meaning && a.vector != nil }

// fillAsk shows the page as its question stands: the card when there is
// one or forgetting is to be confirmed, the matches or the visits, and
// the line that says what is never remembered.
func (s *Screen) fillAsk() {
	a := s.asking
	a.seen = s.memory.changed
	page := a.list.Parent()
	for _, n := range []*ui.Node{a.card, a.list, a.foot} {
		if n.Parent() == page {
			page.Remove(n)
		}
	}
	if a.on && (a.question != "" || a.confirming) {
		s.refill(a.card, s.fillCardOfAsk)
		page.Append(a.card)
	}
	page.Append(a.list)
	if a.on {
		s.fillRemembers()
		page.Append(a.foot)
	}
	switch {
	case a.byMeaning():
		s.listSaid()
	case a.question != "":
		s.listVisits(a.list, a.question)
	}
}

// fillCardOfAsk writes the card: the question and how it was matched,
// the robot that turns meaning off and on, the days kept, and what
// confirms forgetting.
func (s *Screen) fillCardOfAsk() {
	a := s.asking
	title, sub := a.question, "Matched by meaning against pages you visited, on this device."
	switch {
	case a.question == "":
		title, sub = "What pages said", "Kept on this device, for the history to be asked by meaning."
	case a.pending:
		sub = "Searching what the pages said…"
	case a.failed != nil:
		sub = "The model could not search by meaning: their titles and addresses were searched."
	case !a.meaning:
		sub = "Matched against the titles and addresses of pages you visited."
	}
	var robot *ui.Node
	if a.question != "" && a.failed == nil {
		robot = robotToggle("history-robot", "Searched what the pages said, not only their titles.", a.meaning,
			func() {
				a.meaning = !a.meaning
				s.fillAsk()
			})
	}
	tile := ui.Span(ui.Class("site", "big", "clock"), ui.Attr("aria-hidden", "true"), ui.Svg(clockHands,
		ui.Attr("viewBox", "0 0 24 24"), ui.Attr("width", "18"), ui.Attr("height", "18"), ui.Attr("fill", "none"),
		ui.Attr("stroke", "currentColor"), ui.Attr("stroke-width", "2.2"), ui.Attr("stroke-linecap", "round"),
		ui.Attr("stroke-linejoin", "round")))
	a.card.Append(sheetHead(tile, "history-ask", title, sub, robot))
	if a.byMeaning() {
		when := func(id, label string, week bool) *ui.Node {
			return s.pill(id, label, label, a.week == week, func() {
				a.week = week
				s.fillAsk()
			})
		}
		a.card.Append(ui.Div(ui.Class("part"), ui.Span(ui.Text("When")), ui.Div(ui.ID("history-when"),
			when("history-when-week", "Last week", true), when("history-when-any", "Any time", false))))
	}
	if a.confirming {
		sure := "Forget " + keptOf(s.memory.pages.Len()) + "? Their titles and addresses stay in the history."
		if s.memory.pages.Len() == 0 {
			sure = "Remove the file that could not be read?"
		}
		a.card.Append(ui.Div(ui.ID("history-forget-sure"), ui.Attr("role", "alertdialog"),
			ui.Attr("aria-label", "Forget what pages said"), ui.P(ui.Text(sure)),
			ui.Button(ui.ID("history-forget-keep"), ui.Text("Keep"), ui.OnClick(func() {
				a.confirming = false
				s.fillAsk()
			})),
			ui.Button(ui.ID("history-forget-all"), ui.Class("agree"), ui.Text("Forget"), ui.OnClick(func() {
				a.confirming = false
				s.forgetSaid()
				s.fillAsk()
			}))))
	}
}

// fillRemembers writes the line under the list: what is never
// remembered, why the file is not in use, and what forgets everything.
func (s *Screen) fillRemembers() {
	a, m := s.asking, s.memory
	empty(a.foot)
	note := ui.P(ui.ID("history-unremembered"), ui.Text(unremembered))
	if m.problem != nil {
		note = ui.P(ui.ID("history-unremembered"), ui.Text(unremembered+" What pages said is not in use: "+
			m.problem.Error()+"."))
	}
	forget := ui.Button(ui.ID("history-forget"), ui.Text("Forget what pages said"), ui.OnClick(func() {
		a.confirming = true
		s.fillAsk()
		s.doc.Focus(s.doc.ByID("history-forget-keep"))
	}))
	enable(forget, m.pages.Len() > 0 || m.problem != nil)
	a.foot.Append(note, forget)
}

// listSaid fills the list with the pages that said something like the
// question, the closest first: each with its tile, its title, its site,
// the day it was visited and the passage that matched, in the page's
// own words.
func (s *Screen) listSaid() {
	a := s.asking
	s.visitsSeen = s.recorded
	empty(a.list)
	query := recall.Query{Threshold: recall.Threshold, Most: recall.MostMatches}
	if a.week {
		query.Since = s.opts.Now().AddDate(0, 0, -lastWeek)
	}
	matches := s.memory.pages.Search(a.vector, query)
	if len(matches) == 0 {
		a.list.Append(ui.P(ui.ID("history-unsaid"), ui.Text("No page remembered says something like it.")))
	}
	wanted := terms(a.question)
	for _, m := range matches {
		a.list.Append(s.saidRow(m, wanted))
	}
}

func (s *Screen) saidRow(m recall.Match, wanted map[string]bool) *ui.Node {
	title, site := m.Title, recall.Site(m.Address)
	if title == "" {
		title = shownAs(m.Address)
	}
	passage := []ui.Arg{ui.Class("said")}
	for _, run := range marked(excerpt(m.Passage, wanted, excerptSize), wanted) {
		if run.marked {
			passage = append(passage, ui.Strong(ui.Text(run.text)))
			continue
		}
		passage = append(passage, ui.Text(run.text))
	}
	return ui.Div(ui.Class("recalled"), s.siteTile(m.Address, "big"), ui.Div(ui.Class("text"),
		ui.Div(ui.Class("line"), s.link(title, m.Address),
			ui.Span(ui.Class("when"), ui.Text(m.At.Format("Mon 2 Jan")))),
		ui.Div(ui.Class("line"), ui.Span(ui.Class("where"), ui.Text(site)),
			ui.Button(ui.Class("forget"), ui.Text("Forget"), ui.Attr("aria-label", "Forget what this page said"),
				ui.OnClick(func() {
					if s.memory.pages.Forget(m.Address) {
						s.recalled()
					}
				})),
			ui.Button(ui.Class("exclude"), ui.Text("Never remember this site"), ui.OnClick(func() {
				s.configure(func(c *config.Config) { *c = c.Excluding(site) })
			}))),
		ui.P(passage...)))
}

// stretch is a run of a passage, marked when it is a word of the question.
type stretch struct {
	text   string
	marked bool
}

// worded reports whether r is part of a word.
func worded(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// eachWord calls each for every word of text, with where it begins and
// ends, until it says to stop.
func eachWord(text string, each func(from, to int) bool) {
	start := -1
	for at, r := range text {
		switch {
		case worded(r) && start < 0:
			start = at
		case !worded(r) && start >= 0:
			if !each(start, at) {
				return
			}
			start = -1
		}
	}
	if start >= 0 {
		each(start, len(text))
	}
}

// asksFor reports whether word is one of the question's own, long
// enough to be worth a mark.
func asksFor(word string, wanted map[string]bool) bool {
	return utf8.RuneCountInString(word) >= markedWord && wanted[strings.ToLower(word)]
}

// marked cuts a passage into runs, the words that are literally the
// question's marked: nothing else is, and nothing is written for it.
func marked(passage string, wanted map[string]bool) []stretch {
	var out []stretch
	done := 0
	eachWord(passage, func(from, to int) bool {
		if asksFor(passage[from:to], wanted) {
			if from > done {
				out = append(out, stretch{text: passage[done:from]})
			}
			out, done = append(out, stretch{passage[from:to], true}), to
		}
		return true
	})
	if done < len(passage) {
		out = append(out, stretch{text: passage[done:]})
	}
	return out
}

// excerpt is at most size bytes of a passage, round the first word of
// it that is the question's, cut between words, with an ellipsis where
// it was cut.
func excerpt(passage string, wanted map[string]bool, size int) string {
	if len(passage) <= size {
		return passage
	}
	first := 0
	eachWord(passage, func(from, to int) bool {
		if asksFor(passage[from:to], wanted) {
			first = from
			return false
		}
		return true
	})
	start := 0
	if first > size*2/3 {
		start = strings.LastIndexByte(passage[:first-size/3], ' ') + 1
	}
	end := min(start+size, len(passage))
	if end < len(passage) {
		if space := strings.LastIndexByte(passage[start:end], ' '); space > 0 {
			end = start + space
		}
		for !utf8.RuneStart(passage[end]) {
			end--
		}
	}
	out := passage[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(passage) {
		out += "…"
	}
	return out
}
