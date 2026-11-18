// Package frames is the rules of the documents a page shows in its
// iframes, in plain Go: how many a tab holds and how deep, what an
// iframe's attributes ask it to show, what its sandbox allows, whether
// a document lets itself be framed (X-Frame-Options, the
// frame-ancestors of a Content-Security-Policy), where a link's target
// leads, and which cookies a frame's requests may carry.
package frames

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

const (
	// MaxDepth is how many frames deep a tab's pages go, and MaxFrames
	// how many frames a tab holds: one over either stays empty.
	MaxDepth  = 8
	MaxFrames = 64
)

const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// Why a frame stays empty.
var (
	ErrTooDeep = fmt.Errorf("it is more than %d frames deep", MaxDepth)
	ErrTooMany = fmt.Errorf("its tab already holds %d frames", MaxFrames)
	// ErrRecursive is a frame asked to show the page of one of the
	// frames it is in.
	ErrRecursive = errors.New("it would show a page it is itself framed in")
	// ErrScheme is an address light shows in no frame.
	ErrScheme = errors.New("only http and https addresses, about:blank and a srcdoc are shown in a frame")
)

// Admit is why a tab that holds count frames takes no other at depth,
// the depth of a page's own frames being 1; nil when it does.
func Admit(depth, count int) error {
	switch {
	case depth > MaxDepth:
		return ErrTooDeep
	case count >= MaxFrames:
		return ErrTooMany
	}
	return nil
}

// Kind is what an iframe shows.
type Kind int

const (
	// Blank is nothing: no src, an empty one, or about:blank.
	Blank Kind = iota
	// Inline is the markup of its srcdoc.
	Inline
	// Remote is the document at its src.
	Remote
)

// Shown is what an iframe with those attributes shows: its srcdoc when
// it has one, else its src.
func Shown(src string, srcdoc bool) Kind {
	src = strings.TrimSpace(src)
	switch {
	case srcdoc:
		return Inline
	case src == "" || blank(src):
		return Blank
	}
	return Remote
}

func blank(ref string) bool {
	ref = strings.ToLower(ref)
	rest, about := strings.CutPrefix(ref, "about:blank")
	return about && (rest == "" || rest[0] == '?' || rest[0] == '#')
}

// Resolve is the address ref names from base, the address of the
// document that holds the frame. Anything but an http or https address
// is ErrScheme: a javascript: address runs nothing.
func Resolve(base *url.URL, ref string) (*url.URL, error) {
	to, err := base.Parse(strings.TrimSpace(ref))
	if err != nil || !web(to) || to.Hostname() == "" {
		return nil, ErrScheme
	}
	return to, nil
}

// Recursive reports whether to, its fragment aside, is the address of
// one of the documents a frame is in.
func Recursive(to *url.URL, ancestors []*url.URL) bool {
	for _, a := range ancestors {
		if a != nil && bare(a) == bare(to) {
			return true
		}
	}
	return false
}

func bare(u *url.URL) string {
	c := *u
	c.Fragment, c.RawFragment = "", ""
	return c.String()
}

func web(u *url.URL) bool { return u != nil && (u.Scheme == schemeHTTP || u.Scheme == schemeHTTPS) }

// Origin is the origin of an address as browsers write it: its scheme,
// its host and its port when it is not the scheme's own; "null" for
// what has none.
func Origin(u *url.URL) string {
	if !web(u) || u.Hostname() == "" {
		return "null"
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" && p != defaultPort(u.Scheme) {
		return u.Scheme + "://" + host + ":" + p
	}
	return u.Scheme + "://" + host
}

func defaultPort(scheme string) string {
	if scheme == schemeHTTPS {
		return "443"
	}
	return "80"
}

func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return defaultPort(u.Scheme)
}

// Site is the site of an address: its scheme and its registrable
// domain, its host where it has none; "" for what is not of the web.
func Site(u *url.URL) string {
	if !web(u) {
		return ""
	}
	return u.Scheme + "://" + registrable(u.Hostname())
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

// Partition is the top-level site the cookies of a frame are kept
// under: that of top when the frame's document, or one of those it is
// in (chain, a nil for one with no origin), is of another site; "" for
// a frame whose every document is of top's site, whose cookies are the
// site's own.
func Partition(top *url.URL, chain []*url.URL) string {
	site := Site(top)
	for _, at := range chain {
		if at == nil || Site(at) != site {
			return site
		}
	}
	return ""
}

// Mixed reports whether to is an address in the clear that a frame
// under those documents may not show: one of them came over https, and
// to is an http address that is not this machine's.
func Mixed(ancestors []*url.URL, to *url.URL) bool {
	if to == nil || to.Scheme != schemeHTTP || local(to.Hostname()) {
		return false
	}
	for _, a := range ancestors {
		if a != nil && a.Scheme == schemeHTTPS {
			return true
		}
	}
	return false
}

func local(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Target is where a link or a form with a target leads.
type Target int

const (
	// Self is the frame the link is in: no target, _self, and, as light
	// opens no window for a page, _blank and a name no frame has.
	Self Target = iota
	// Parent is the frame around it, the tab's page for one that is in
	// none.
	Parent
	// Top is the tab's page.
	Top
	// Named is the frame of the tab that has the target as its name.
	Named
)

// TargetOf reads a target attribute: a keyword whatever its case, else
// a name.
func TargetOf(target string) Target {
	switch strings.ToLower(target) {
	case "", "_self", "_blank":
		return Self
	case "_parent":
		return Parent
	case "_top":
		return Top
	}
	return Named
}
