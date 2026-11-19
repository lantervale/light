package app

import (
	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/tabs"
)

// place moves the shown tab to address when it is a place in the page
// it shows, as browsers go to a fragment: a step of the tab's history
// with nothing fetched, the page scrolled to what the address names,
// and its scripts told at their next frame. It reports whether it did.
func (s *Screen) place(address string) bool {
	tab, session := s.browser.Tab(), s.session()
	at := tab.Current()
	page := session.pages[at]
	if !onTheWeb(at) || !web.SamePage(at, address) || page == nil || page.Doc == nil || page.Err != nil {
		return false
	}
	s.giveUp(session)
	s.keep(tab)
	tab.Push(address)
	session.pages[address] = page
	web.Reveal(page.Doc, address)
	s.showTabs()
	s.showNavigation()
	s.say()
	return true
}

// left is where the page of a step of a tab's history was scrolled when
// the tab left the step, at that address.
type left struct {
	address string
	scroll  geom.Vec2
}

// keep notes where the page tab stands on is scrolled, for the step it
// is about to leave: restore puts the page back there when the tab's
// history returns to the step.
func (s *Screen) keep(tab *tabs.Tab) {
	session := s.sessionOf(tab)
	if session.left == nil {
		session.left = map[int]left{}
	}
	at := tab.Current()
	if page := session.pages[at]; page != nil && page.Doc != nil {
		session.left[tab.At()] = left{address: at, scroll: page.Doc.Scroll(page.Doc.HTML())}
	}
}

// restore scrolls the page tab came to through its history, or fetched
// again, from the address from: where the step was left, a page fetched
// again too; else, for another place of the page it was on, to the place
// its address names, the top for one that names none.
func (s *Screen) restore(tab *tabs.Tab, from string) {
	session := s.sessionOf(tab)
	at := tab.Current()
	page := session.pages[at]
	if page == nil || page.Doc == nil {
		return
	}
	if was, kept := session.left[tab.At()]; kept && was.address == at {
		page.Doc.SetScroll(page.Doc.HTML(), was.scroll)
		session.put = page
		return
	}
	if page == session.pages[from] && web.Fragment(from) != web.Fragment(at) {
		web.Reveal(page.Doc, at)
	}
}

// address is the address the document f shows is at: the one it gave
// itself last, else the one it came from.
func (f *frame) address() string {
	switch {
	case f.run != nil:
		return f.run.at
	case f.pushed[f.at]:
		return f.at
	case f.page.Address == inlineAddress && f.page.Base != nil:
		return f.page.Base.String()
	}
	return f.page.Address
}

// placeFrame is place for the document f shows: a step of the tab's
// history that scrolls that document alone.
func (s *Screen) placeFrame(f *frame, address string) bool {
	if f.gone || f.page == nil || f.page.Doc == nil || f.load != nil || !web.SamePage(f.address(), address) {
		return false
	}
	s.renamed(f, "push", address)
	web.Reveal(f.page.Doc, address)
	if f.run != nil {
		f.run.at = address
		f.run.rt.Arrived(address)
	}
	s.showNavigation()
	return true
}
