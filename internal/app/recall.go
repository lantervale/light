package app

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/model"
	"github.com/lantervale/light/pkg/recall"
)

//go:embed recall.css
var recallSheet string

const (
	// recallFeature is the feature that remembers what pages said, off
	// until its user turns it on, and recallLabel what it is called.
	recallFeature = "recall"
	recallLabel   = "Remember what pages said"
	// recallFile is the file of what pages said in the folder light is
	// given.
	recallFile = "recall.bin"
	// recallSettle is how long a page is shown, loaded, before what it
	// says is read: to be tuned with recall's own constants.
	recallSettle = 3 * time.Second
	// unremembered says what is never remembered.
	unremembered = "Light's own pages, local files and sites you exclude are never remembered."
)

// memory is what the pages visited said, for the history asked by
// meaning: the most private thing a Profile keeps. Nothing of it is
// logged, sent, or shown to a page.
type memory struct {
	pages *recall.Store
	file  *store
	// problem is why the file could not be read or written.
	problem error
	// seen is the history's changes and excluded the sites never
	// remembered the pages were last matched with; changed counts the
	// changes of the pages, for the windows that show them.
	seen     uint64
	excluded string
	changed  uint64
	// landing brings the passages a model embedded back to the frame,
	// reading the pages being embedded, and epoch what was forgotten
	// meanwhile.
	landing chan embedded
	reading map[string]bool
	epoch   uint64
	ctx     context.Context //nolint:containedctx // ended with the Profile, which the embeddings outlive otherwise
	end     context.CancelFunc
	settle  time.Duration
	// offering is whether forgetting is offered, the feature just
	// turned off.
	offering bool
}

// embedded is what a page said when it was visited, as a model embedded it.
type embedded struct {
	epoch          uint64
	address, title string
	at             time.Time
	texts          []string
	vectors        [][]float32
	err            error
}

// settling is the page a window shows, since when, and whether what it
// says was read; styled is whether the window's Document has the sheet
// of what pages said, and recount what shows again how much is kept on
// the settings' page.
type settling struct {
	page    *web.Page
	since   time.Time
	read    bool
	styled  bool
	recount func()
}

// recollect reads what pages said from the folder light was given.
func (p *Profile) recollect() {
	p.assists.Ask(recallFeature)
	m := &memory{pages: recall.New(recall.MaxBytes), landing: make(chan embedded, 16), reading: map[string]bool{},
		seen: ^uint64(0), settle: recallSettle}
	m.ctx, m.end = context.WithCancel(context.Background())
	m.file = &store{name: recallFile, write: func(path string) error { return recall.Save(path, m.pages) }}
	if p.opts.Dir != "" {
		m.pages, m.problem = recall.Load(m.file.path(p.opts.Dir), recall.MaxBytes)
	}
	m.excluded = p.config.RecallExcluded
	p.memory = m
}

// remembers reports whether what pages say is remembered and asked:
// the model is on and its user turned the feature on.
func (p *Profile) remembers() bool { return p.helps(recallFeature) && p.embeds() }

// lastVisit is the last visit to address the history holds.
func lastVisit(log *history.Log, address string) (history.Visit, bool) {
	for _, day := range log.Days("") {
		for _, v := range day.Visits {
			if v.Address == address {
				return v, true
			}
		}
	}
	return history.Visit{}, false
}

// recalled has what the pages said that changed written, and shown
// again in the windows that show it.
func (p *Profile) recalled() {
	p.memory.changed++
	p.memory.file.touch()
}

// watchRecall keeps what pages said in step with the history and the
// settings, takes what a model embedded, reads the shown page once it
// settled, and follows the history page's question.
func (s *Screen) watchRecall() {
	if !s.settled.styled {
		s.settled.styled = true
		s.doc.AddSheet(recallSheet)
	}
	s.matchRecall()
	s.readSettled()
	s.followAsk()
	if s.settled.recount != nil {
		s.settled.recount()
	}
}

// matchRecall forgets the pages the history no longer holds and those
// of sites excluded since, and writes the file that changed.
func (p *Profile) matchRecall() {
	m := p.memory
	for landed := false; !landed; {
		select {
		case page := <-m.landing:
			p.keepSaid(page)
		default:
			landed = true
		}
	}
	if m.seen != p.recorded || m.excluded != p.config.RecallExcluded {
		m.seen, m.excluded = p.recorded, p.config.RecallExcluded
		visited := map[string]bool{}
		for _, day := range p.visits.Days("") {
			for _, v := range day.Visits {
				visited[v.Address] = true
			}
		}
		if m.pages.Keep(func(address string) bool { return visited[address] && !p.excludes(address) }) > 0 {
			m.epoch++
			p.recalled()
		}
	}
	if err := m.file.keep(p.opts.Dir); err != nil {
		m.problem = err
	}
}

func (p *Profile) excludes(address string) bool { return p.config.Excludes(recall.Site(address)) }

// readSettled reads what the shown page says once it has been there, loaded,
// for a while.
func (s *Screen) readSettled() {
	page, at := s.readable(), &s.settled
	if page != at.page {
		at.page, at.since, at.read = page, time.Now(), false
	}
	if page == nil || at.read || !s.remembers() || s.loading() || time.Since(at.since) < s.memory.settle {
		return
	}
	at.read = true
	s.remember(page)
}

// remember has the model embed the passages of a page of the web the
// history holds, off the frame, unless the run is driven, its site is
// excluded or this visit of it is remembered already.
func (p *Profile) remember(page *web.Page) {
	m := p.memory
	visit, visited := lastVisit(p.visits, page.Address)
	if p.opts.Driven || !onTheWeb(page.Address) || p.excludes(page.Address) || !visited || m.reading[page.Address] {
		return
	}
	if at, kept := m.pages.At(page.Address); kept && at.Equal(visit.At) {
		return
	}
	texts := recall.Passages(pageText(page.Doc), recall.PassageSize, recall.PassagesPerPage)
	if len(texts) == 0 {
		return
	}
	m.reading[page.Address] = true
	read := embedded{epoch: m.epoch, address: page.Address, title: page.Title, at: visit.At, texts: texts}
	embedder := p.opts.Model
	go func() {
		read.vectors, read.err = embedder.Embed(m.ctx, model.Texts{Texts: texts, Kind: model.Document,
			Priority: model.Background})
		select {
		case m.landing <- read:
		case <-m.ctx.Done():
		}
	}()
}

// keepSaid keeps what a model embedded of a page, unless it was
// forgotten, excluded or turned off meanwhile.
func (p *Profile) keepSaid(page embedded) {
	m := p.memory
	delete(m.reading, page.address)
	if _, visited := lastVisit(p.visits, page.address); page.err != nil || page.epoch != m.epoch || !visited ||
		!p.remembers() || p.excludes(page.address) {
		return
	}
	if m.pages.Add(page.address, page.title, page.at, page.texts, page.vectors) == nil {
		p.recalled()
	}
}

// forgetSaid forgets what every page said, and removes its file.
func (p *Profile) forgetSaid() {
	m := p.memory
	m.pages.Clear()
	m.epoch++
	m.offering = false
	p.recalled()
	m.problem = m.file.flush(p.opts.Dir)
	p.refresh()
}

// refresh shows each setting again on the pages that show them.
func (p *Profile) refresh() {
	for _, s := range p.screens {
		for _, show := range s.fields {
			show()
		}
	}
}

// assistChanged is told a feature the model helps with was turned on or
// off: turning this one off offers to forget what was remembered.
func (p *Profile) assistChanged(id string) {
	if id != recallFeature {
		return
	}
	p.memory.offering = !p.assists.On(id) && p.memory.pages.Len() > 0
	p.refresh()
}

// setRemembering turns the feature on or off.
func (p *Profile) setRemembering(on bool) {
	if p.assists.Set(recallFeature, on) {
		p.assisted.touch()
		p.redrawModel()
		p.assistChanged(recallFeature)
	}
}

// endRecall ends the embeddings that run and writes what is not yet
// written.
func (p *Profile) endRecall() error {
	p.memory.end()
	return p.memory.file.flush(p.opts.Dir)
}

// keptOf says how many pages are remembered.
func keptOf(pages int) string {
	if pages == 1 {
		return "what 1 page said"
	}
	return fmt.Sprintf("what %d pages said", pages)
}

// forgetOffer offers to forget what was remembered, the feature just
// turned off: nil when nothing is offered. Its elements are named
// after name.
func (s *Screen) forgetOffer(name string) *ui.Node {
	m := s.memory
	if !m.offering || m.pages.Len() == 0 {
		return nil
	}
	done := func(forget bool) func() {
		return func() {
			if m.offering = false; forget {
				s.forgetSaid()
			}
			s.refresh()
			if s.cardOpen() {
				s.fillCard()
			}
		}
	}
	return ui.Div(ui.ID(name+"-offer"), ui.Class("offer"),
		ui.P(ui.Text("Light still keeps "+keptOf(m.pages.Len())+".")),
		ui.Button(ui.ID(name+"-keep"), ui.Text("Keep"), ui.OnClick(done(false))),
		ui.Button(ui.ID(name+"-forget"), ui.Class("agree"), ui.Text("Forget"), ui.OnClick(done(true))))
}

// recallSettings is the settings' part of what pages said, there while
// the model is on: what turns remembering on, the sites never
// remembered, and what forgets.
func (s *Screen) recallSettings() *ui.Node {
	remembering := ui.Input(ui.ID("set-recall"), ui.Type("checkbox"))
	ui.On(remembering, ui.EventChange, func() { s.setRemembering(s.doc.Checked(remembering)) })
	kept := ui.Span(ui.ID("recall-kept"), ui.Class("hint"))
	forget := ui.Button(ui.ID("set-recall-clear"), ui.Text("Forget what pages said"), ui.OnClick(s.forgetSaid))
	offer := ui.Div(ui.ID("recall-offered"))
	on := ui.Div(
		ui.H2(ui.Text(recallLabel)),
		ui.Div(ui.Class("setting"), ui.Label(ui.Class("check"), remembering,
			ui.Text("Remember what the pages you visit say, to search the history by meaning"))),
		offer,
		ui.P(ui.Class("hint"), ui.Text("Off until you switch it on, even with the model on. Once on, light keeps "+
			"passages of the pages you visit, and what the model made of them, in its folder on this device: "+
			"nothing of it is sent anywhere, and no page reads it. Removing a visit or clearing the history forgets "+
			"what its page said. "+unremembered)),
		s.textSetting("set-recall-excluded", "Sites never remembered", "Their hosts, as bank.example, mail.example. "+
			"What their pages said is forgotten.", func(c *config.Config) *string { return &c.RecallExcluded }, same),
		ui.Div(ui.Class("setting"), forget, kept))
	held := ui.Div(ui.ID("recall-settings"))
	seen := ^uint64(0)
	show := func() {
		if !s.modelOn() {
			empty(held)
			return
		}
		if len(held.Children()) == 0 {
			held.Append(on)
		}
		s.doc.SetChecked(remembering, s.assists.On(recallFeature))
		pages := s.memory.pages.Len()
		kept.SetText("Nothing is remembered.")
		if pages > 0 {
			kept.SetText("Light keeps " + keptOf(pages) + ".")
		}
		enable(forget, pages > 0 || s.memory.problem != nil)
		empty(offer)
		if offered := s.forgetOffer("set-recall"); offered != nil {
			offer.Append(offered)
		}
		seen = s.memory.changed
	}
	s.fields = append(s.fields, show)
	s.settled.recount = func() {
		if seen != s.memory.changed && s.inView(held) {
			show()
		}
	}
	show()
	return held
}
