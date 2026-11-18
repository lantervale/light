package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

var errStopped = errors.New("its loading was stopped")

// load is a page on its way: cancel stops what of it can be, the page
// and its sheets.
type load struct{ cancel context.CancelFunc }

// session is what a tab holds beside its addresses: the pages of the web
// it fetched, by the address asked and by the one that answered, those on
// their way, its requests, what its page's scripts said, those scripts
// while they run, what sites keep for as long as the tab, and its
// developer tools.
type session struct {
	pages map[string]*web.Page
	loads map[string]*load
	// posts are the forms on their way to an address with a POST, each
	// sent by the load of its address that follows, once.
	posts   map[string]web.Submission
	log     *netlog.Log
	said    *console.Lines
	scripts *running
	kept    map[string]dom.Storage
	// frames are the frames of the pages the tab holds, by their page.
	frames map[*web.Page]*framed
	tools  devtools.State
	// zoom is the tab's own zoom in percent, 0 for the settings'.
	zoom int
	// note is what the status bubble says of the tab's last move when it
	// showed nothing new.
	note string
}

// session is the shown tab's.
func (s *Screen) session() *session { return s.sessionOf(s.browser.Tab()) }

func (s *Screen) sessionOf(tab *tabs.Tab) *session {
	if s.sessions[tab] == nil {
		s.sessions[tab] = &session{
			pages: map[string]*web.Page{}, loads: map[string]*load{}, posts: map[string]web.Submission{},
			log:  netlog.New(s.config.Requests),
			said: console.New(consoleLines), kept: map[string]dom.Storage{}, frames: map[*web.Page]*framed{},
			tools: devtools.State{Preserve: s.config.PreserveLog},
		}
		s.keepBodies(s.sessions[tab])
	}
	return s.sessions[tab]
}

// forget stops and drops what closed tabs held.
func (s *Screen) forget() {
	open := map[*tabs.Tab]bool{}
	for _, tab := range s.browser.Tabs {
		open[tab] = true
	}
	for tab, session := range s.sessions {
		if open[tab] {
			continue
		}
		for _, l := range session.loads {
			l.cancel()
		}
		s.end(session)
		for _, set := range session.frames {
			set.release()
		}
		delete(s.sessions, tab)
	}
}

// fetched is the page the shown tab has for an address of the web, nil
// until it landed: it starts its load the first time it is asked for.
func (s *Screen) fetched(address string) *web.Page { return s.fetch(s.browser.Tab(), address) }

// fetch is fetched for a tab, shown or not.
func (s *Screen) fetch(tab *tabs.Tab, address string) *web.Page {
	session := s.sessionOf(tab)
	page := session.pages[address]
	if page != nil || session.loads[address] != nil {
		return page
	}
	if !session.tools.Preserve {
		session.log.Clear()
		session.said.Clear()
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	l := &load{cancel}
	session.loads[address] = l
	links := web.Links{Open: s.followed, Over: s.point, Save: s.download, Unsent: session.unsent,
		Aimed: func(address, target string) bool { return s.aimedFromTop(tab, address, target, nil) },
		Post: func(sent web.Submission) {
			if !s.aimedFromTop(tab, sent.Address, sent.Target, &sent) {
				s.posted(sent)
			}
		}}
	loader, ctx := s.current(), netlog.With(s.began(ctx), session.log, "navigation", netlog.Document)
	sent, posting := session.posts[address]
	delete(session.posts, address)
	go func() {
		defer cancel()
		var loaded *web.Page
		if posting {
			loaded = loader.Post(ctx, sent, links)
		} else {
			loaded = loader.Load(ctx, address, links)
		}
		s.landed <- func() { s.land(tab, address, l, loaded) }
	}()
	return nil
}

// land takes the page a load ended with, unless it was stopped meanwhile:
// the tab is at the address that answered. An answer with nothing to
// show, and one that is downloaded, leave the tab where it was.
func (s *Screen) land(tab *tabs.Tab, address string, l *load, page *web.Page) {
	session := s.sessions[tab]
	if session == nil || session.loads[address] != l {
		return
	}
	delete(session.loads, address)
	if page.Empty && tab.Current() == address && tab.CanBack() {
		tab.Drop()
		session.note = fmt.Sprintf("%s answered %d %s: nothing to show", hostOf(address), page.Status,
			http.StatusText(page.Status))
		s.show()
		return
	}
	if page.Kind == content.Download && page.Err == nil {
		s.save(page.Address, "", "")
		if tab.Current() == address && tab.CanBack() {
			tab.Drop()
			s.show()
			return
		}
	}
	session.pages[address], session.pages[page.Address] = page, page
	s.zoomPages(session)
	tab.Redirect(address, page.Address)
	if page.Icon != nil {
		s.pictures.Store(faviconOf(page.Address), page.Icon)
	}
	if page.Kind == content.Picture && page.Err == nil {
		s.pictures.Store(pictureOf(page.Address), page.Body)
	}
	if page.Err == nil && tab.Current() == page.Address {
		s.record(page.Address, page.Title)
	}
	if tab != s.browser.Tab() {
		s.showTabs()
		return
	}
	s.show()
}

func (s *Screen) loading() bool {
	return s.session().loads[shownBy(s.browser.Tab().Current())] != nil
}

// reloadOrStop is the one button: it stops the shown page while it loads,
// and fetches it again once it has.
func (s *Screen) reloadOrStop() {
	address, session := shownBy(s.browser.Tab().Current()), s.session()
	if l := session.loads[address]; l != nil {
		l.cancel()
		delete(session.loads, address)
		session.pages[address] = &web.Page{Address: address, Err: errStopped}
	} else {
		delete(session.pages, address)
	}
	s.show()
}

// shownBy is the address whose page address shows: its own, or the one a
// source view is of.
func shownBy(address string) string {
	if of, ok := tabs.Source(address); ok {
		return of
	}
	return address
}

func hostOf(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return address
	}
	return u.Hostname()
}

// The names the Document's img elements ask a site's icon and a picture
// shown alone by.
func faviconOf(address string) string { return "favicon:" + address }
func pictureOf(address string) string { return "picture:" + address }
