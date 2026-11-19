package app

import (
	"github.com/lantervale/light/internal/web"
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
	tab.Push(address)
	session.pages[address] = page
	web.Reveal(page.Doc, address)
	s.showTabs()
	s.showNavigation()
	s.say()
	return true
}

// restore scrolls the page the shown tab came back to through its
// history, from another place of it at from: to the place its address
// names, the top for one that names none.
func (s *Screen) restore(session *session, from string) {
	at := s.browser.Tab().Current()
	page := session.pages[at]
	if page == nil || page.Doc == nil || page != session.pages[from] || web.Fragment(from) == web.Fragment(at) {
		return
	}
	web.Reveal(page.Doc, at)
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
