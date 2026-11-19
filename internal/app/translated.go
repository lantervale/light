package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
)

const (
	// translationNotch is how far from the right edge of the card of a
	// page's translation its notch is.
	translationNotch = 62
	// wordingTokens are the fewest and the most tokens of the
	// translation of one passage.
	wordingFewest, wordingMost = 64, 1024
)

// rendered is what a window shows of the pages translated in place: the
// label of the address field, the card that hangs from it, the veil
// over what is not translated yet, and the translation of each page
// its tabs hold.
type rendered struct {
	label, card, veil *ui.Node
	pages             map[*web.Page]*rendition
	// helped is whether the model translated at the last frame, drawn
	// what the label shows, changes the count of what changed of the
	// shown page's translation and filled the one the card shows.
	helped          bool
	drawn           string
	changes, filled uint64
	// veiled is where what the veil covers stood when it was drawn.
	veiled []geom.Rect
}

// rendition is a page translated in place: its passages, and what the
// model made of each in each language it was asked for.
type rendition struct {
	// from is the page's language and into the one its text shows in.
	from     string
	into     translation
	wordings []*wording
	versions map[string]*version
	// on is whether the translation shows, stopped whether its reader
	// stopped it, failed why the model did, and asked the passage at
	// that is being translated.
	on, stopped bool
	failed      error
	asked       *inquiry
	at          int
}

// version is a page's translation into one language: what each passage
// reads, settled whether it was translated or left as written, a
// passage the model does not hold whole being left.
type version struct {
	said    []string
	settled []bool
	left    int
}

func (s *Screen) buildTranslation() {
	r := &s.rendered
	r.pages = map[*web.Page]*rendition{}
	s.doc.AddSheet(translateSheet)
	r.label = ui.Button(ui.ID("translation-label"), ui.Attr("aria-haspopup", "dialog"), ui.OnClick(func() {
		if s.translationOpen() {
			s.closeTranslation(false)
			return
		}
		s.openTranslation()
	}))
	s.field.Remove(s.sorting.label)
	s.field.Remove(s.star)
	s.field.Append(r.label, s.sorting.label, s.star)
	r.veil = ui.Div(ui.ID("translation-veil"), ui.Attr("aria-hidden", "true"))
	r.card = ui.Div(ui.ID("translation"), ui.Class("sheet"), ui.Attr("role", "dialog"),
		ui.Attr("aria-label", "Translation of this page"), ui.OnBack(func(e ui.Event) {
			s.closeTranslation(true)
			e.StopPropagation()
		}))
	s.view.Parent().Append(r.veil, r.card)
}

// foreign is the language a page is written in and the one it is
// translated into, when the model translates and the page is in a
// language its reader does not read: the one its html element names,
// nothing being offered for a page that names none, for a language
// never translated, or for one the translation would be in.
func (s *Screen) foreign(page *web.Page) (string, translation, bool) {
	if page == nil || !s.helps(translateFeature) {
		return "", translation{}, false
	}
	named, _ := page.Doc.HTML().Attr("lang")
	from := config.Language(named)
	into := s.translatesInto()
	if from == "" || from == config.Language(into.tag) || s.config.PageRule(from) == config.NeverTranslate ||
		slices.ContainsFunc(s.languages(), func(tag string) bool { return config.Language(tag) == from }) {
		return "", translation{}, false
	}
	return from, into, true
}

// shownIn reports whether a tab of the window shows page.
func (s *Screen) shownIn(page *web.Page) bool {
	for _, tab := range s.browser.Tabs {
		if session := s.sessions[tab]; session != nil && session.pages[tab.Current()] == page {
			return true
		}
	}
	return false
}

// watchTranslation shows the label of a page in a language its reader
// does not read, translates the shown page a passage at a time while
// its translation is on, holds the translation of a page a tab behind
// shows, and puts the original back in a page that was left, reloaded
// or is no more translated: nothing stays half translated.
func (s *Screen) watchTranslation() {
	r := &s.rendered
	if helps := s.helps(translateFeature); helps != r.helped {
		r.helped = helps
		if s.relink != nil {
			s.relink()
		}
		for _, refresh := range s.fields {
			refresh()
		}
	}
	shown := s.readable()
	from, into, offered := s.foreign(shown)
	for page, t := range r.pages {
		if _, _, still := s.foreign(page); !still || !s.shownIn(page) {
			t.end()
			delete(r.pages, page)
			r.changes++
		} else if page != shown {
			t.hold()
		}
	}
	t := r.pages[shown]
	if offered && t == nil && s.config.PageRule(from) == config.AlwaysTranslate {
		t = s.translate(shown, from, into)
	}
	if t != nil {
		if t.into != into {
			t.turn(into)
			r.changes++
		}
		if s.advance(t) {
			r.changes++
		}
	}
	s.showTranslation(from, into, offered, t)
	s.followTranslation(t)
	s.veilUntranslated(shown, t)
}

// translate begins the translation of a page in place.
func (s *Screen) translate(page *web.Page, from string, into translation) *rendition {
	t := &rendition{from: from, into: into, wordings: wordingsOf(page.Doc), versions: map[string]*version{}, on: true}
	s.rendered.pages[page] = t
	s.rendered.changes++
	return t
}

// version is the page's translation into the language it shows in.
func (t *rendition) version() *version {
	v := t.versions[t.into.tag]
	if v == nil {
		v = &version{said: make([]string, len(t.wordings)), settled: make([]bool, len(t.wordings))}
		t.versions[t.into.tag] = v
	}
	return v
}

// done is how many passages are translated or left as written.
func (v *version) done() int {
	done := 0
	for _, settled := range v.settled {
		if settled {
			done++
		}
	}
	return done
}

// running reports whether the page is being translated still.
func (t *rendition) running() bool {
	return t.on && !t.stopped && t.failed == nil && t.version().done() < len(t.wordings)
}

// hold stops what is asked of the model for a page that is not shown;
// it goes on once it is.
func (t *rendition) hold() {
	t.asked.stop()
	t.asked = nil
}

// end stops the translation and puts the page's own text back.
func (t *rendition) end() {
	t.hold()
	for _, w := range t.wordings {
		w.restore()
	}
}

// show has the page read as its translation, as far as it came, or as
// it was written.
func (t *rendition) show(on bool) {
	t.end()
	t.on, t.stopped, t.failed = on, false, nil
	if !on {
		return
	}
	for i, said := range t.version().said {
		if said != "" {
			t.wordings[i].write(said)
		}
	}
}

// turn has the page translated into another language: what was
// translated into it before shows at once.
func (t *rendition) turn(into translation) {
	t.end()
	t.into = into
	t.show(t.on)
}

// advance takes the translation of the passage the model was asked
// for, written in the page when it is whole, and asks for the next;
// it reports whether anything changed.
func (s *Screen) advance(t *rendition) bool {
	v := t.version()
	if a := t.asked; a != nil {
		if !a.take() || !a.done {
			return false
		}
		t.asked = nil
		said := strings.TrimSpace(strings.NewReplacer(opens, "", ends, "").Replace(a.text))
		switch {
		case a.err != nil:
			t.failed = a.err
			return true
		case a.capped || said == "":
			v.left++
		default:
			v.said[t.at] = said
			t.wordings[t.at].write(said)
		}
		v.settled[t.at] = true
		return true
	}
	if !t.running() {
		return false
	}
	t.at = slices.Index(v.settled, false)
	given := t.wordings[t.at].markdown()
	t.asked = s.inquire("You translate a web page, a passage at a time. The passage"+untrusted+onlyText+
		"Translate it from "+tongue(t.from)+" into "+t.into.name+". Give the translation only, on one line, and "+
		"keep its marks where they are: what stands between ** and **, between * and *, between [ and ], and "+
		"between ` and `, which stays as it is.", "The passage, in Markdown:\n", given,
		min(max(len(given)/2, wordingFewest), wordingMost))
	return false
}

// stopTranslation stops the translation where it is: what is
// translated stays.
func (s *Screen) stopTranslation(t *rendition) {
	t.hold()
	t.stopped = true
	s.rendered.changes++
}

// showTranslation shows the label of the shown page: its language and
// the one it is translated into, pressed while its translation shows.
func (s *Screen) showTranslation(from string, into translation, offered bool, t *rendition) {
	r := &s.rendered
	on := t != nil && t.on
	key := fmt.Sprint(offered, from, into.tag, on)
	if key == r.drawn {
		return
	}
	r.drawn = key
	empty(r.label)
	toggle(r.label, "shown", offered)
	if !offered {
		s.closeTranslation(false)
		return
	}
	said := "This page is in " + tongue(from) + ": translate it"
	if on {
		said = "This page is translated from " + tongue(from)
	}
	r.label.SetAttr("aria-label", said)
	r.label.SetAttr("aria-pressed", strconv.FormatBool(on))
	r.label.Append(ui.Span(ui.Class("pill"), glyph("translate"),
		ui.Span(ui.Text(strings.ToUpper(from)+" → "+strings.ToUpper(config.Language(into.tag))))))
}

func (s *Screen) translationOpen() bool { return s.rendered.card.HasClass("shown") }

// openTranslation opens the card of the shown page's translation, and
// begins the translation of a page that has none.
func (s *Screen) openTranslation() {
	page := s.readable()
	from, into, offered := s.foreign(page)
	if !offered {
		return
	}
	s.menu.Close()
	s.closePanel(false)
	s.closeCard(false)
	s.closeChoices()
	s.closeSheets(s.rendered.card)
	s.list.RemoveClass("shown")
	s.recent.RemoveClass("shown")
	r := &s.rendered
	t := r.pages[page]
	if t == nil {
		t = s.translate(page, from, into)
	}
	r.card.AddClass("shown")
	s.fillTranslation(t)
	s.hang(r.card, r.label, translationNotch)
	s.doc.Focus(s.doc.ByID("translation-end"))
}

// closeTranslation closes the card; refocus gives the label the focus.
// The translation goes on.
func (s *Screen) closeTranslation(refocus bool) {
	r := &s.rendered
	if !s.translationOpen() {
		return
	}
	r.card.RemoveClass("shown")
	empty(r.card)
	if refocus && r.label.HasClass("shown") {
		s.doc.Focus(r.label)
	}
}

// followTranslation keeps the open card under the label, shows in it
// what changed of the translation, and closes it when it is left or
// its page is.
func (s *Screen) followTranslation(t *rendition) {
	r := &s.rendered
	if !s.translationOpen() {
		return
	}
	if t == nil || s.choosing() || s.cardOpen() || s.panelOpen() || s.leftBy(r.card, r.label) {
		s.closeTranslation(false)
		return
	}
	s.hang(r.card, r.label, translationNotch)
	if r.filled != r.changes {
		s.fillTranslation(t)
	}
}

// stands says where a page's translation stands: its title and what
// the card's one filled button does about it.
func (t *rendition) stands() (string, string) {
	switch {
	case !t.on:
		return "This page is in " + tongue(t.from), translateTitle
	case t.failed != nil:
		return "The model could not translate this page", "Try again"
	case t.stopped:
		return "Translation stopped", translateTitle
	case t.running():
		return "Translating this page", stopLabel
	}
	return "This page is translated", doneLabel
}

// progress says how far the translation is, in passages.
func (t *rendition) progress() string {
	v := t.version()
	said := fmt.Sprintf("%d of %d paragraphs", v.done()-v.left, len(t.wordings))
	if len(t.wordings) == 1 {
		said = fmt.Sprintf("%d of 1 paragraph", v.done()-v.left)
	}
	if v.left > 0 {
		said += fmt.Sprintf(" · %d too long, left as written", v.left)
	}
	if t.running() {
		said += " · what is not translated yet is grey"
	}
	return said
}

// fillTranslation writes the card: where the translation stands, the
// robot that puts the page's own text back, how far it is, the
// languages, the rule for the pages of this language, and what stops
// it or begins it again.
func (s *Screen) fillTranslation(t *rendition) {
	r := &s.rendered
	r.filled = r.changes
	address := s.browser.Tab().Current()
	title, does := t.stands()
	sub := tongue(t.from) + " → " + t.into.label + " · on this device, nothing sent"
	switch {
	case !t.on:
		sub = "Shown as it was written"
	case t.failed != nil:
		sub = t.failed.Error()
	}
	s.refill(r.card, func() {
		r.card.Append(ui.Span(ui.Class("notch")), sheetHead(s.siteTile(address, "big"), "translation", title, sub,
			robotToggle("translation-robot", "Translated for you on this device. A click shows the page as it was "+
				"written.", t.on, func() { s.showTranslated(t, !t.on) })))
		if t.on {
			v := t.version()
			share := 100
			if len(t.wordings) > 0 {
				share = v.done() * 100 / len(t.wordings)
			}
			r.card.Append(ui.Div(ui.Class("part"),
				ui.Div(ui.Class("track"), ui.Div(ui.Class("share"), ui.Style("width: "+strconv.Itoa(share)+"%"))),
				ui.Span(ui.ID("translation-progress"), ui.Text(t.progress()))))
		}
		r.card.Append(s.translationInto(t), s.translationRule(t))
		quiet := []*ui.Node{ui.Button(ui.ID("translation-open"), ui.Text("Open in Translate"),
			ui.OnClick(func() { s.openInTranslate(t) }))}
		if t.on {
			quiet = append([]*ui.Node{ui.Button(ui.ID("translation-original"), ui.Text("Show the original"),
				ui.OnClick(func() { s.showTranslated(t, false) }))}, quiet...)
		}
		r.card.Append(sheetFoot(quiet, ui.Button(ui.ID("translation-end"), ui.Text(does), ui.OnClick(func() {
			switch does {
			case stopLabel:
				s.stopTranslation(t)
			case doneLabel:
				s.closeTranslation(true)
			default:
				s.showTranslated(t, true)
			}
		}))))
	}, "translation-end")
}

// showTranslated has the page read as its translation or as it was
// written, as the card's robot is turned.
func (s *Screen) showTranslated(t *rendition, on bool) {
	t.show(on)
	s.rendered.changes++
}

// translationInto are the languages the page is translated into: the
// choice is the selection's and the translation page's.
func (s *Screen) translationInto(t *rendition) *ui.Node {
	into := ui.Div(ui.ID("translation-into"))
	for _, offered := range translations() {
		chip := s.pill("translation-into-"+offered.tag, offered.label, "Translate into "+offered.name,
			offered == t.into, func() {
				s.configure(func(c *config.Config) { c.TranslateInto = offered.tag })
				t.stopped, t.failed = false, nil
				s.rendered.changes++
			})
		enable(chip, config.Language(offered.tag) != t.from)
		into.Append(chip)
	}
	return ui.Div(ui.Class("part"), ui.Span(ui.Text("Into")), into)
}

// translationRule is what light does with the pages of the page's
// language: ask each time, always translate, never.
func (s *Screen) translationRule(t *rendition) *ui.Node {
	rules := ui.Div(ui.ID("translation-rule"))
	held := s.config.PageRule(t.from)
	for _, rule := range []struct {
		id, label string
		rule      config.PageRule
	}{{"ask", "Ask each time", config.AskToTranslate}, {"always", "Always translate", config.AlwaysTranslate},
		{"never", "Never", config.NeverTranslate}} {
		rules.Append(s.pill("translation-rule-"+rule.id, rule.label, rule.label+" for pages in "+tongue(t.from),
			rule.rule == held, func() {
				s.configure(func(c *config.Config) { *c = c.WithPageRule(t.from, rule.rule) })
				s.rendered.changes++
			}))
	}
	return ui.Div(ui.Class("part"), ui.Span(ui.Text("For pages in "+tongue(t.from))), rules)
}

// openInTranslate opens the translation page in a tab of its own with
// the page's text as it was written.
func (s *Screen) openInTranslate(t *rendition) {
	lines := make([]string, len(t.wordings))
	for i, w := range t.wordings {
		lines[i] = strings.Join(strings.Fields(w.text(piece{from: 0, to: len(w.nodes)})), " ")
	}
	s.desk.brought, s.desk.broughtIn = strings.Join(lines, "\n\n"), t.from
	s.closeTranslation(false)
	s.OpenInNewTab(translatePage)
}

// veilUntranslated draws a veil over the blocks of the shown page that
// are not translated yet, while it is being translated: light changes
// nothing of the page's own look.
func (s *Screen) veilUntranslated(page *web.Page, t *rendition) {
	r := &s.rendered
	if t == nil || !t.running() || s.host == nil {
		if r.veil.HasClass("shown") {
			r.veil.RemoveClass("shown")
			empty(r.veil)
		}
		r.veiled = r.veiled[:0]
		return
	}
	host, _ := s.doc.BoxOf(s.host)
	placed := []geom.Rect{host}
	settled := t.version().settled
	var last *ui.Node
	for i, w := range t.wordings {
		if box, shown := page.Doc.BoxOf(w.block); !settled[i] && w.block != last && shown {
			placed, last = append(placed, box), w.block
		}
	}
	if slices.Equal(placed, r.veiled) {
		return
	}
	r.veiled = placed
	stage, _ := s.doc.BoxOf(r.veil.Parent())
	r.veil.AddClass("shown")
	r.veil.SetStyle(fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx", host.X-stage.X, host.Y-stage.Y,
		host.W, host.H))
	empty(r.veil)
	for _, b := range placed[1:] {
		if b.X+b.W < 0 || b.Y+b.H < 0 || b.X > host.W || b.Y > host.H {
			continue
		}
		r.veil.Append(ui.Div(ui.Style(fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx", b.X, b.Y, b.W,
			b.H))))
	}
}

// translationSettings is the settings' part of the pages translated in
// place, there while the model translates: the languages whose pages
// are translated as they show, and those whose pages never are.
func (s *Screen) translationSettings() *ui.Node {
	on := ui.Div(ui.H2(ui.Text("Translation")),
		s.textSetting("set-translate-always", "Languages always translated", "Their pages are translated as they "+
			"show, as fr, de.", func(c *config.Config) *string { return &c.TranslateAlways }, same),
		s.textSetting("set-translate-never", "Languages never translated", "Their pages are offered no translation.",
			func(c *config.Config) *string { return &c.TranslateNever }, same))
	held := ui.Div(ui.ID("translation-settings"))
	show := func() {
		switch helps := s.helps(translateFeature); {
		case !helps:
			empty(held)
		case len(held.Children()) == 0:
			held.Append(on)
		}
	}
	s.fields = append(s.fields, show)
	show()
	return held
}
