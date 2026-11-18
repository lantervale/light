package app

import (
	"context"
	"net/url"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/cookies"
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

// linked opens address as the shown tab's page asked for it.
func (s *Screen) linked(address string) { s.byPage(func() { s.Open(address) }) }

// began is ctx for a navigation, from the page that asked for it when
// one did.
func (s *Screen) began(ctx context.Context) context.Context {
	if from, err := url.Parse(s.from); s.from != "" && err == nil {
		return cookies.From(ctx, from)
	}
	return ctx
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
