package app

import (
	"context"
	"net/url"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
)

// cookiesFile is the cookies' file in the folder light is given.
const cookiesFile = "cookies.json"

// byPage runs what the shown tab's page asked for, a link followed or
// a script's move: a navigation begun meanwhile is known to come from
// that page.
func (s *Screen) byPage(do func()) {
	s.from = s.browser.Tab().Current()
	defer func() { s.from = "" }()
	do()
}

// fromPage is byPage for what the document at address, one of a frame
// of the shown tab's page, asked for.
func (s *Screen) fromPage(address string, do func()) {
	s.from = address
	defer func() { s.from = "" }()
	do()
}

// linked opens address as the shown tab's page asked for it.
func (s *Screen) linked(address string) { s.byPage(func() { s.Open(address) }) }

// byScript is byPage for what a script of the page did unasked: the
// navigation is not the player's act.
func (s *Screen) byScript(do func()) {
	s.unasked = true
	defer func() { s.unasked = false }()
	s.byPage(do)
}

// began is ctx for a navigation, from the page that asked for it when
// one did, described as what it is: the player's own, a link or a form
// of a page, or a script's.
func (s *Screen) began(ctx context.Context) context.Context {
	q := identity.Request{Destination: identity.Document, User: !s.unasked}
	if from, err := url.Parse(s.from); s.from != "" && err == nil {
		q.Initiator = from
		ctx = cookies.From(ctx, from)
		if s.unasked && onTheWeb(s.from) {
			ctx = localnet.From(ctx, s.spaceOf(s.from))
		}
	}
	return fetch.Asking(ctx, q)
}

// spaceOf is the space the shown tab's page at address came from, the
// Internet for one that is not known.
func (s *Screen) spaceOf(address string) localnet.Space {
	if page := s.session().pages[address]; page != nil {
		return page.Space
	}
	return localnet.Public
}

// address is where the scripts' page is now, its own moves followed.
func (r *running) address() *url.URL {
	at := r.at
	if at == "" {
		at = r.page.Address
	}
	u, err := url.Parse(at)
	if err != nil {
		return nil
	}
	return u
}

func (s *Screen) clearCookies() { s.jar.Clear() }

// clearCookiesSetting is the button that forgets every cookie.
func (s *Screen) clearCookiesSetting() *ui.Node {
	return ui.Div(ui.Class("setting"), ui.Button(ui.ID("set-clear-cookies"), ui.Text("Clear cookies"),
		ui.OnClick(s.clearCookies)))
}

// storedBy is what the site of the shown tab's page keeps, for the
// developer tools: its cookies and its localStorage.
func (s *Screen) storedBy() devtools.Stored {
	address := shownBy(s.browser.Tab().Current())
	at, err := url.Parse(address)
	origin := originOf(address)
	if err != nil || origin == "" || (at.Scheme != "http" && at.Scheme != "https") {
		return devtools.Stored{}
	}
	kept := devtools.Stored{Site: origin, Cookies: s.jar.For(at)}
	area := s.sites.Area(origin)
	for _, key := range area.Keys() {
		value, _ := area.Get(key)
		kept.Local = append(kept.Local, [2]string{key, value})
	}
	return kept
}
