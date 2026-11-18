package app

import (
	_ "embed"
	"net/url"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/tabs"
)

//go:embed asking.css
var askingSheet string

const (
	// askedMost is how much of a question's message is shown.
	askedMost = 2000
	// stoppable is how many questions a page asks before the next
	// offers to stop it asking.
	stoppable = 1

	answerOK = "ok"
)

// modal is the questions of a tab's page, the scripts' alert, confirm
// and prompt: one at a time, since the scripts of the tab wait for its
// answer.
type modal struct {
	s   *Screen
	tab *tabs.Tab
	// open is the question that waits for its answer, shown while its
	// tab is.
	open *question
	// page is the page that asked last, asked how many questions it
	// asked, and stopped whether the user had it ask no more.
	page    *web.Page
	asked   int
	stopped bool
	// hold keeps the scripts of the tab's other documents from running
	// while one waits.
	hold dom.Hold
}

// question is one question: what is asked, by the scripts of which
// document, and the dialog that shows it while its tab is shown.
type question struct {
	dom.Question
	says string
	live *running
	// text is what a prompt's field holds, kept while it is not shown.
	text string
	// node is the dialog, field its text field and stop its box to ask
	// no more.
	node, field, stop *ui.Node
	// answer is what was answered, handed to the scripts at the next
	// frame.
	answer *dom.Answer
}

// modals are the questions of every tab, and the windows whose sheet
// styles their dialog.
type modals struct {
	of     map[*tabs.Tab]*modal
	styled map[*ui.Document]bool
}

// modalOf is the questions of a tab.
func (s *Screen) modalOf(tab *tabs.Tab) *modal {
	if s.modals.of == nil {
		s.modals.of, s.modals.styled = map[*tabs.Tab]*modal{}, map[*ui.Document]bool{}
	}
	if s.modals.of[tab] == nil {
		s.modals.of[tab] = &modal{s: s, tab: tab}
	}
	return s.modals.of[tab]
}

// says is who asks, as the dialog names it: the host of a document.
func says(address string, framed bool) string {
	host := ""
	if u, err := url.Parse(address); err == nil {
		host = u.Host
	}
	switch {
	case host == "":
		return "This page says"
	case framed:
		return "An embedded page at " + host + " says"
	}
	return host + " says"
}

// asker is what shows the questions of a document of a tab: those of
// live, at address. refused is why its questions are never shown, ""
// for a document that may ask, and say tells the console.
func (s *Screen) asker(tab *tabs.Tab, live *running, who func() string, refused string, say func(string)) func(
	dom.Question) bool {
	return func(q dom.Question) bool {
		a := s.modalOf(tab)
		if top := s.sessionOf(tab).pages[tab.Current()]; top != a.page {
			a.page, a.asked, a.stopped = top, 0, false
		}
		why := refused
		switch {
		case why != "":
		case a.stopped:
			why = "The page was stopped from asking again."
		case a.open != nil:
			why = "Another document of the tab waits for an answer."
		}
		if why != "" {
			say("Ignored call to '" + q.Kind + "()'. " + why)
			return false
		}
		a.asked++
		if len([]rune(q.Message)) > askedMost {
			q.Message = string([]rune(q.Message)[:askedMost]) + "…"
		}
		a.open = &question{Question: q, says: who(), live: live, text: q.Default}
		return true
	}
}

// pageDialogs shows the questions of a tab's page.
func (s *Screen) pageDialogs(tab *tabs.Tab, session *session, live *running) func(dom.Question) bool {
	return s.asker(tab, live, func() string { return says(live.page.Address, false) }, "", func(text string) {
		session.said.Add(console.Line{Level: console.Error, Text: text, Source: live.page.Address})
	})
}

// frameDialogs shows the questions of a frame's document, when it is of
// its tab's page's origin: a document of another origin asks nothing,
// as the HTML standard has it.
func (s *Screen) frameDialogs(f *frame, live *running) func(dom.Question) bool {
	refused := ""
	if f.held.Opaque() || f.origin != originOf(f.set.top.Address) {
		refused = "A document of another origin than its tab's page shows no dialog."
	}
	label, session := f.label(), f.set.session
	return s.asker(f.set.tab, live, func() string { return says(f.origin, true) }, refused, func(text string) {
		session.said.Add(console.Line{Level: console.Error, Text: text, Source: live.page.Address, Frame: label})
	})
}

// asked shows the question of the shown tab, hands the scripts the
// answer given since the last frame, and reports whether the tab still
// waits for one: its scripts then run nothing. A question whose page
// was left is forgotten, and one whose tab is not shown waits, its tab
// marked.
func (s *Screen) asked() bool {
	for tab, a := range s.modals.of {
		if a.s != s {
			continue
		}
		if a.open != nil && a.open.live.rt.Closed() {
			s.unshow(a.open)
			a.open = nil
		}
		if _, held := s.sessions[tab]; !held && a.open == nil {
			delete(s.modals.of, tab)
		}
	}
	shown := s.browser.Tab()
	for i, tab := range s.browser.Tabs {
		a := s.modals.of[tab]
		if at := s.doc.ByID(tabID(i)); at != nil {
			toggle(at, "asking", a != nil && a.open != nil)
		}
		if a != nil && a.open != nil && tab != shown {
			s.unshow(a.open)
		}
	}
	a := s.modals.of[shown]
	if a == nil || a.open == nil {
		return false
	}
	if a.open.answer != nil {
		s.answer(a, *a.open.answer)
	}
	if a.open != nil && a.open.node == nil {
		s.ask(a)
	}
	return a.open != nil
}

// ask shows the open question of a as a dialog over the window: the
// page and the header take neither the pointer nor the keys until it
// is answered. Enter says yes, Escape no.
func (s *Screen) ask(a *modal) {
	q := a.open
	if !s.modals.styled[s.doc] {
		s.modals.styled[s.doc] = true
		s.doc.AddSheet(askingSheet)
	}
	closed := ui.OnClose(func(e ui.Event) {
		if q.node == nil || e.Node != q.node {
			return
		}
		given := dom.Answer{OK: e.Value == answerOK, Text: s.typed(q)}
		a.stopped = a.stopped || (q.stop != nil && s.doc.Checked(q.stop))
		q.answer = &given
	})
	parts := []ui.Arg{ui.ID("asking"), closed, ui.Attr("role", "alertdialog"), ui.Attr("aria-label", q.says),
		ui.H2(ui.Text(q.says)), ui.Div(ui.ID("asking-message"), ui.Text(q.Message))}
	ok := ui.Button(ui.ID("asking-ok"), ui.Text("OK"), ui.Value(answerOK), ui.Attr("formmethod", "dialog"))
	buttons := ui.Div(ui.ID("asking-buttons"))
	if q.Kind == dom.Prompt {
		q.field = ui.Input(ui.ID("asking-answer"), ui.Value(q.text), ui.Attr("aria-label", "Answer"),
			ui.Attr("autofocus", ""), ui.OnSubmit(func() { s.doc.Close(q.node, answerOK) }))
		parts = append(parts, q.field)
	} else {
		ok.SetAttr("autofocus", "")
	}
	if a.asked > stoppable {
		q.stop = ui.Input(ui.ID("asking-stop"), ui.Type("checkbox"))
		parts = append(parts, ui.Label(q.stop, ui.Text("Stop this page from asking again")))
	}
	if q.Kind != dom.Alert {
		buttons.Append(ui.Button(ui.ID("asking-cancel"), ui.Text("Cancel"), ui.Value("cancel"),
			ui.Attr("formmethod", "dialog")))
	}
	buttons.Append(ok)
	node := ui.Dialog(append(parts, buttons)...)
	toggle(node, "dark", s.config.DarkMode)
	q.node = node
	s.doc.Body().Append(node)
	s.doc.ShowModal(node)
}

// typed is what the field of a prompt holds.
func (s *Screen) typed(q *question) string {
	if q.field != nil {
		q.text = s.doc.Value(q.field)
	}
	return q.text
}

// unshow takes the dialog of a question off the window: the question
// waits on.
func (s *Screen) unshow(q *question) {
	node := q.node
	if node == nil {
		return
	}
	s.typed(q)
	q.node, q.field, q.stop = nil, nil, nil
	s.doc.Close(node, "")
	s.doc.Body().Remove(node)
}

// answer hands the scripts that asked the answer to the open question
// of a: they go on, and may ask again.
func (s *Screen) answer(a *modal, given dom.Answer) {
	q := a.open
	if q == nil {
		return
	}
	s.unshow(q)
	a.open = nil
	if q.Kind != dom.Prompt || !given.OK {
		given.Text = ""
	}
	q.live.rt.Answer(given)
}

// questionOf is the question a tab waits the answer of, in whichever
// window holds it.
func (p *Profile) questionOf(tab *tabs.Tab) (*modal, *question) {
	if a := p.modals.of[tab]; a != nil && a.open != nil && !a.open.live.rt.Closed() {
		return a, a.open
	}
	return nil, nil
}
