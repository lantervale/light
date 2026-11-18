package identity

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Destination is what a request is for, as Sec-Fetch-Dest names it.
type Destination string

const (
	Document Destination = "document"
	Style    Destination = "style"
	Script   Destination = "script"
	Image    Destination = "image"
	Font     Destination = "font"
	// Empty is a script's fetch or XMLHttpRequest, and a download.
	Empty Destination = "empty"
)

// Mode is how a request is made, as Sec-Fetch-Mode names it.
type Mode string

const (
	Navigate   Mode = "navigate"
	CORS       Mode = "cors"
	NoCORS     Mode = "no-cors"
	SameOrigin Mode = "same-origin"
)

// Request is what light knows of a request before it is sent.
type Request struct {
	Destination Destination
	// Mode is the destination's own unless it is said: a document is
	// navigated to, what a script fetches is read across origins (cors),
	// the rest is not (no-cors). A font is among the rest: light asks
	// for it as for a picture, with no Origin and no check of who may
	// read it.
	Mode Mode
	// Initiator is the address of the page that asks, nil for what the
	// user asked for: an address typed, a bookmark, a page of light's.
	Initiator *url.URL
	// Referrer is the address the request says it comes from when it is
	// not the page's: a sheet's, for its fonts.
	Referrer *url.URL
	// User is whether an act of the user began a navigation: a click, an
	// address typed, a form submitted.
	User bool
}

func (q Request) mode() Mode {
	switch {
	case q.Mode != "":
		return q.Mode
	case q.Destination == Document:
		return Navigate
	case q.Destination == Empty:
		return CORS
	}
	return NoCORS
}

// Accept is what a request for dest says it accepts: the types light
// reads, as browsers list theirs.
func Accept(dest Destination) string {
	switch dest {
	case Document:
		return "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"
	case Image:
		return "image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"
	case Style:
		return "text/css,*/*;q=0.1"
	case Script, Font, Empty:
	}
	return "*/*"
}

const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

func web(u *url.URL) bool { return u != nil && (u.Scheme == schemeHTTP || u.Scheme == schemeHTTPS) }

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func port(u *url.URL) string {
	switch p := u.Port(); {
	case p != "":
		return p
	case u.Scheme == schemeHTTPS:
		return "443"
	}
	return "80"
}

// sameSite reports whether two addresses are of one site: one scheme,
// and one registrable domain, or one host where there is none.
func sameSite(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && registrable(a.Hostname()) == registrable(b.Hostname())
}

func registrable(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if net.ParseIP(host) != nil {
		return host
	}
	if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return domain
	}
	return host
}

// The values of Sec-Fetch-Site.
const (
	siteNone       = "none"
	siteSameOrigin = "same-origin"
	siteSameSite   = "same-site"
	siteCrossSite  = "cross-site"
)

// Site is what Sec-Fetch-Site says of a request the page at initiator
// began, by every address it went through, its redirects included:
// none for what no page of the web asked for.
func Site(initiator *url.URL, trail []*url.URL) string {
	if !web(initiator) {
		return siteNone
	}
	site := siteSameOrigin
	for _, at := range trail {
		switch {
		case sameOrigin(initiator, at):
		case sameSite(initiator, at):
			site = siteSameSite
		default:
			return siteCrossSite
		}
	}
	return site
}

// Trustworthy reports whether an address is one fetch metadata is sent
// to: an https one, or one of this machine.
func Trustworthy(u *url.URL) bool {
	if u == nil || u.Scheme == schemeHTTPS {
		return u != nil
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Referrer is what a request for to says of from, the address it comes
// from, by the policy browsers have unless told
// (strict-origin-when-cross-origin): the whole address without its
// fragment and its user within an origin, the origin alone to another,
// and nothing from https to http or from an address that is not of the
// web.
func Referrer(from, to *url.URL) string {
	switch {
	case !web(from) || to == nil:
		return ""
	case from.Scheme == schemeHTTPS && to.Scheme != schemeHTTPS:
		return ""
	case !sameOrigin(from, to):
		return from.Scheme + "://" + from.Host + "/"
	}
	said := *from
	said.User, said.Fragment, said.RawFragment = nil, "", ""
	return said.String()
}

// Identity is what every request says of light and of its user.
type Identity struct {
	// Agent is the User-Agent, and Languages the Accept-Language.
	Agent, Languages string
}

// Apply gives a request's headers what light says by default. Any
// request names light, and the user's languages unless it names its
// own. One that is described, q, says more, for the address it now
// goes to, the last of trail, after those it was redirected from: what
// it accepts, unless a script's fetch said, where it comes from, and
// to an address that is Trustworthy what it is for and from what site.
func (i Identity) Apply(h http.Header, q *Request, trail []*url.URL) {
	if i.Agent != "" {
		h.Set("User-Agent", i.Agent)
	}
	if i.Languages != "" && h.Get("Accept-Language") == "" {
		h.Set("Accept-Language", i.Languages)
	}
	if q == nil || len(trail) == 0 {
		return
	}
	to, mode := trail[len(trail)-1], q.mode()
	if q.Destination != Empty || h.Get("Accept") == "" {
		h.Set("Accept", Accept(q.Destination))
	}
	if mode == Navigate {
		h.Set("Upgrade-Insecure-Requests", "1")
	}
	from := q.Referrer
	if from == nil {
		from = q.Initiator
	}
	set(h, "Referer", Referrer(from, to))
	if !Trustworthy(to) {
		for _, name := range []string{"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site", "Sec-Fetch-User"} {
			h.Del(name)
		}
		return
	}
	h.Set("Sec-Fetch-Dest", string(q.Destination))
	h.Set("Sec-Fetch-Mode", string(mode))
	h.Set("Sec-Fetch-Site", Site(q.Initiator, trail))
	user := ""
	if mode == Navigate && q.User {
		user = "?1"
	}
	set(h, "Sec-Fetch-User", user)
}

func set(h http.Header, name, value string) {
	if value == "" {
		h.Del(name)
		return
	}
	h.Set(name, value)
}
