package app

import (
	"context"
	"net/url"
	"runtime"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
)

// agent is the User-Agent light says it is: the settings', else its
// own.
func (s *Screen) agent() string {
	if s.config.UserAgent != "" {
		return s.config.UserAgent
	}
	return identity.Default(s.opts.Version)
}

// languages are the languages a site is asked for and a page's scripts
// are told, as browsers list them: the settings', else the system's.
func (s *Screen) languages() []string {
	if set := identity.Tags(s.config.Languages); len(set) > 0 {
		return identity.Languages(set)
	}
	return identity.Languages(s.opts.Languages)
}

// navigator is what a page's scripts read of light and of its machine,
// the same as its requests say.
func (s *Screen) navigator() dom.Navigator {
	return dom.Navigator{
		Agent: s.agent(), Platform: identity.Platform(runtime.GOOS, runtime.GOARCH), Languages: s.languages(),
		Cores: runtime.NumCPU(), Driven: s.opts.Driven,
	}
}

// around is the window around a page, for its scripts: its size and its
// place in points, and how many pixels a pixel of the page is drawn
// with, at its zoom.
func (s *Screen) around(zoom float32) dom.Window {
	size, at := s.win.Size(), s.win.CurrentPosition()
	return dom.Window{Width: size.X, Height: size.Y, X: at.X, Y: at.Y, Ratio: s.win.ContentScale() * zoom}
}

// by is how a navigation the page's scripts ask for now is begun: as
// the page's, an act of the player's while they hear a click of his,
// their own doing otherwise.
func (r *running) by(s *Screen) func(do func()) {
	if r.acting {
		return s.byPage
	}
	return s.byScript
}

// asking is ctx for what a script of the running page asked for,
// described as what it is: a script element's source, a link's sheet,
// or a fetch.
func asking(ctx context.Context, live *running, q dom.Request) context.Context {
	dest := identity.Empty
	switch {
	case q.Script:
		dest = identity.Script
	case q.Sheet:
		dest = identity.Style
	}
	return fetch.Asking(fetch.By(localnet.From(ctx, live.page.Space), live.address()),
		identity.Request{Destination: dest})
}

// saving is ctx for a download: one the shown page of the web led to
// says so, one the player asked for on a page of light's does not.
func (s *Screen) saving(ctx context.Context, from string) context.Context {
	q := identity.Request{Destination: identity.Empty, Mode: identity.NoCORS}
	if at, err := url.Parse(from); err == nil && onTheWeb(from) {
		q.Initiator = at
	}
	return fetch.Asking(ctx, q)
}
