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
type load struct {
	cancel context.CancelFunc
	// timing is when each of its steps ended.
	timing *web.Timing
}

// session is what a tab holds beside its addresses: the pages of the web
// it fetched, by the address asked and by the one that answered, those on
// their way, its requests, what its page's scripts said, those scripts
// while they run, what sites keep for as long as the tab, and its
// developer tools.
type session struct {
	pages map[string]*web.Page
	loads map[string]*load
	// pending is the move the tab waits for on the page it shows, nil
	// for none (loading.go).
	pending *pending
	// posts are the forms on their way to an address with a POST, each
	// sent by the load of its address that follows, once.
	posts map[string]web.Submission
	log   *netlog.Log
	// told is the last request LIGHT_TIMING was told of.
	told    int
	said    *console.Lines
	scripts *running
	kept    map[string]dom.Storage
	// frames are the frames of the pages the tab holds, by their page,
	// and returned whether the tab's history moved since they were last
	// brought where its step has them (framehistory.go).
	frames   map[*web.Page]*framed
	returned bool
	tools    devtools.State
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
		session.pending = nil
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
	s.begin(tab, address)
	return nil
}

// begin starts the load of the page a tab is to show at address.
func (s *Screen) begin(tab *tabs.Tab, address string) {
	session := s.sessionOf(tab)
	if session.loads[address] != nil {
		return
	}
	if !session.tools.Preserve {
		session.log.Clear()
		session.said.Clear()
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	l := &load{cancel: cancel, timing: &web.Timing{}}
	l.timing.Mark(web.Started)
	ctx = web.Timed(ctx, l.timing)
	session.loads[address] = l
	links := web.Links{Open: s.followed, Over: s.point, Save: s.download, Unsent: session.unsent,
		Aimed: func(address, target string) bool { return s.targeted(tab, address, target, nil) },
		Post: func(sent web.Submission) {
			if !s.targeted(tab, sent.Address, sent.Target, &sent) {
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
}

// land takes the page a load ended with, unless it was stopped meanwhile:
// the tab is at the address that answered, which it moves to now when
// it waited for the page on the one it showed. An answer with nothing
// to show, and one that is downloaded, leave the tab where it was.
func (s *Screen) land(tab *tabs.Tab, address string, l *load, page *web.Page) {
	session := s.sessions[tab]
	if session == nil || session.loads[address] != l {
		return
	}
	delete(session.loads, address)
	waited := session.pending
	if waited != nil && waited.address != address {
		waited = nil
	}
	stays := func() bool {
		switch {
		case waited != nil:
			session.pending = nil
		case tab.Current() == address && tab.CanBack():
			tab.Drop()
		default:
			return false
		}
		s.show()
		return true
	}
	if page.Empty && stays() {
		session.note = fmt.Sprintf("%s answered %d %s: nothing to show", hostOf(address), page.Status,
			http.StatusText(page.Status))
		s.say()
		return
	}
	if page.Kind == content.Download && page.Err == nil {
		if s.save(page.Address, "", ""); stays() {
			return
		}
	}
	session.pages[address], session.pages[page.Address] = page, page
	s.zoomPages(session)
	if waited != nil {
		session.pending = nil
		waited.arrive()
	}
	page.Timing.Mark(web.Shown)
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
	if page.Doc != nil && web.Fragment(page.Address) != "" {
		web.Reveal(page.Doc, page.Address)
	}
	if tab != s.browser.Tab() {
		s.showTabs()
		return
	}
	s.show()
	s.dismiss(tab)
}

func (s *Screen) loading() bool { return s.waited(s.browser.Tab()) != "" }

// reloadOrStop is the one button: it stops what the shown tab waits
// for, the move to another page, which leaves it on the one it shows,
// or the load of its first, and fetches the page again once it has it.
func (s *Screen) reloadOrStop() {
	address, session := shownBy(s.browser.Tab().Current()), s.session()
	if s.stop() {
		return
	}
	if l := session.loads[address]; l != nil {
		l.cancel()
		delete(session.loads, address)
		session.pages[address] = &web.Page{Address: address, Err: errStopped}
		s.show()
		return
	}
	s.depart(s.browser.Tab().Current(), true, func() {})
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
