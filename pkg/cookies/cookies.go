// Package cookies is the cookies a browser keeps for the sites it
// visits: one Jar for every request, the rules that say which cookie a
// response may set and which a request carries (domain, path, Secure,
// HttpOnly, SameSite, the name prefixes, the public suffixes, third
// parties), what a page's scripts see of them, and the file they are
// kept in between runs.
package cookies

import (
	"cmp"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

// The limits a jar keeps.
const (
	// MaxSize is the most bytes a cookie's name and value hold together,
	// and MaxAttribute the most an attribute's value does.
	MaxSize      = 4096
	MaxAttribute = 1024
	// MaxPerSite is how many cookies one site keeps, and MaxTotal how
	// many the jar does: the least lately sent go first.
	MaxPerSite = 180
	MaxTotal   = 3000
	// MaxAge is the longest a cookie is kept, whatever it asks for.
	MaxAge = 400 * 24 * time.Hour
)

// SameSite is which requests from other sites a cookie goes with.
type SameSite string

const (
	// Lax goes with a navigation from another site, not with what its
	// pages ask for: what a cookie is when it does not say.
	Lax SameSite = "lax"
	// Strict goes with no request from another site.
	Strict SameSite = "strict"
	// None goes with every request, and must be Secure.
	None SameSite = "none"
)

// Cookie is one cookie as the jar keeps it.
type Cookie struct {
	Name, Value string
	// Domain is the host the cookie is of, and HostOnly whether it goes
	// to that host alone, not to those under it.
	Domain   string
	HostOnly bool
	Path     string
	// Secure goes over https only; HTTPOnly is kept from scripts.
	Secure, HTTPOnly bool
	SameSite         SameSite
	// Expires is when the cookie ends, zero for one that ends with the
	// run.
	Expires time.Time
	// Created is when it was first set, Used when it was last sent.
	Created, Used time.Time
}

// Persistent reports whether the cookie outlives the run.
func (c Cookie) Persistent() bool { return !c.Expires.IsZero() }

// Request is a request as the jar sees it: where it goes and who asks.
type Request struct {
	URL *url.URL
	// Top is the page the request is made for, a sheet's, a picture's or
	// a script's; nil for a navigation, which is its own.
	Top *url.URL
	// From is the page a navigation began on, a link's or a script's;
	// nil for one the player asked for.
	From *url.URL
}

// navigation reports whether the request shows a page of its own.
func (q Request) navigation() bool { return q.Top == nil }

// sameSite reports whether the request stays in the site that asks.
func (q Request) sameSite() bool {
	from := q.Top
	if q.navigation() {
		if from = q.From; from == nil || !web(from) {
			return true
		}
	}
	return siteOf(from) == siteOf(q.URL)
}

// Jar is every site's cookies, safe to use from any goroutine.
type Jar struct {
	mu sync.Mutex
	// sites are the cookies by the site their domain is of.
	sites   map[string][]*Cookie
	off     bool
	third   bool
	changed bool
	now     func() time.Time
}

// Option sets a Jar up.
type Option func(*Jar)

// Clock tells the jar the time, time.Now unless set.
func Clock(now func() time.Time) Option { return func(j *Jar) { j.now = now } }

// New is an empty jar that keeps cookies, and none of a third party.
func New(opts ...Option) *Jar {
	j := &Jar{sites: map[string][]*Cookie{}, now: time.Now}
	for _, opt := range opts {
		opt(j)
	}
	return j
}

// Enabled reports whether the jar takes and gives cookies.
func (j *Jar) Enabled() bool {
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return !j.off
}

// SetEnabled has the jar take and give cookies, or neither: those it
// holds are kept meanwhile.
func (j *Jar) SetEnabled(on bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.off = !on
}

// ThirdParty reports whether a request to another site than its page's
// carries and sets cookies.
func (j *Jar) ThirdParty() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.third
}

func (j *Jar) SetThirdParty(on bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.third = on
}

// Changed reports whether the jar holds what its file does not.
func (j *Jar) Changed() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.changed
}

// Len is how many cookies the jar holds.
func (j *Jar) Len() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.count()
}

func (j *Jar) count() int {
	n := 0
	for _, site := range j.sites {
		n += len(site)
	}
	return n
}

// Clear forgets every cookie.
func (j *Jar) Clear() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.sites) > 0 {
		clear(j.sites)
		j.changed = true
	}
}

// Header is the Cookie header q is sent with, "" for none.
func (j *Jar) Header(q Request) string {
	if j == nil {
		return ""
	}
	return header(j.matching(q, false, true))
}

// Script is what document.cookie reads in the page at address: its
// cookies but those kept from scripts.
func (j *Jar) Script(page *url.URL) string {
	if j == nil {
		return ""
	}
	return header(j.matching(Request{URL: page, Top: page}, true, true))
}

// For are the cookies a request of the page at address to itself
// carries, those kept from scripts among them.
func (j *Jar) For(page *url.URL) []Cookie {
	if j == nil {
		return nil
	}
	return j.matching(Request{URL: page, Top: page}, false, false)
}

func header(cookies []Cookie) string {
	pairs := make([]string, len(cookies))
	for i, c := range cookies {
		pairs[i] = c.Name + "=" + c.Value
	}
	return strings.Join(pairs, "; ")
}

// matching are the cookies q carries, the longest paths first, then the
// oldest; scripted leaves out those kept from scripts, and sent notes
// they were used.
func (j *Jar) matching(q Request, scripted, sent bool) []Cookie {
	if q.URL == nil || !web(q.URL) {
		return nil
	}
	same, navigation := q.sameSite(), q.navigation()
	host, path, secure := hostOf(q.URL), pathOf(q.URL), q.URL.Scheme == schemeHTTPS
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.off || (!navigation && !same && !j.third) {
		return nil
	}
	now := j.now()
	var out []Cookie
	for _, c := range j.sites[registrable(host)] {
		switch {
		case !domainMatches(host, c.Domain, c.HostOnly), !pathMatches(path, c.Path), c.expired(now),
			c.Secure && !secure, c.HTTPOnly && scripted,
			!same && c.SameSite != None && (!navigation || c.SameSite != Lax):
			continue
		}
		if sent {
			c.Used = now
		}
		out = append(out, *c)
	}
	slices.SortStableFunc(out, func(a, b Cookie) int {
		return cmp.Or(cmp.Compare(len(b.Path), len(a.Path)), a.Created.Compare(b.Created))
	})
	return out
}

// Store takes the cookies the answer to q sets, its Set-Cookie headers:
// each one alone, those the rules refuse left out.
func (j *Jar) Store(q Request, lines []string) {
	if j == nil {
		return
	}
	for _, line := range lines {
		j.set(q, line, false)
	}
}

// SetScript takes the cookie a script of the page at address gives
// document.cookie.
func (j *Jar) SetScript(page *url.URL, line string) {
	if j == nil {
		return
	}
	j.set(Request{URL: page, Top: page}, line, true)
}

func (j *Jar) set(q Request, line string, scripted bool) {
	if q.URL == nil || !web(q.URL) {
		return
	}
	same, navigation := q.sameSite(), q.navigation()
	host, secure := hostOf(q.URL), q.URL.Scheme == schemeHTTPS
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.off || (!navigation && !same && !j.third) {
		return
	}
	now := j.now()
	c, ok := parse(line, now)
	if !ok || !c.allowed(host, pathOf(q.URL), secure, scripted) || (!navigation && !same && c.SameSite != None) {
		return
	}
	key := registrable(c.Domain)
	site := j.sites[key]
	at := slices.IndexFunc(site, func(kept *Cookie) bool {
		return kept.Name == c.Name && kept.Domain == c.Domain && kept.Path == c.Path
	})
	if !secure && slices.ContainsFunc(site, func(kept *Cookie) bool { return kept.shadows(c, now) }) {
		return
	}
	if at >= 0 {
		kept := site[at]
		if kept.HTTPOnly && scripted && !kept.expired(now) {
			return
		}
		c.Created = kept.Created
		site = slices.Delete(site, at, at+1)
		j.changed = j.changed || kept.Persistent()
	}
	if !c.expired(now) {
		site = append(site, c)
		j.changed = j.changed || c.Persistent()
	}
	j.sites[key] = site
	j.trim(key, now)
}

// trim drops the cookies that ended, then those least lately sent over
// what a site and the jar may keep.
func (j *Jar) trim(key string, now time.Time) {
	j.sites[key] = j.fit(j.sites[key], MaxPerSite, now)
	if len(j.sites[key]) == 0 {
		delete(j.sites, key)
	}
	if j.count() <= MaxTotal {
		return
	}
	var all []*Cookie
	for _, site := range j.sites {
		all = append(all, site...)
	}
	kept := map[*Cookie]bool{}
	for _, c := range j.fit(all, MaxTotal, now) {
		kept[c] = true
	}
	for key, site := range j.sites {
		if site = slices.DeleteFunc(site, func(c *Cookie) bool { return !kept[c] }); len(site) == 0 {
			delete(j.sites, key)
		} else {
			j.sites[key] = site
		}
	}
}

func (j *Jar) fit(cookies []*Cookie, most int, now time.Time) []*Cookie {
	drop := func(c *Cookie) {
		j.changed = j.changed || c.Persistent()
	}
	cookies = slices.DeleteFunc(cookies, func(c *Cookie) bool {
		if c.expired(now) {
			drop(c)
			return true
		}
		return false
	})
	if over := len(cookies) - most; over > 0 {
		slices.SortStableFunc(cookies, func(a, b *Cookie) int { return a.Used.Compare(b.Used) })
		for _, c := range cookies[:over] {
			drop(c)
		}
		cookies = slices.Delete(cookies, 0, over)
		slices.SortStableFunc(cookies, func(a, b *Cookie) int { return a.Created.Compare(b.Created) })
	}
	return cookies
}

func (c *Cookie) expired(now time.Time) bool { return c.Persistent() && !c.Expires.After(now) }

// allowed reports whether a response from host at path may set the
// cookie, over https when secure, or a script of such a page when
// scripted. It settles the cookie's domain and path on the way.
func (c *Cookie) allowed(host, path string, secure, scripted bool) bool {
	domained := c.Domain != ""
	switch {
	case !domained:
		c.Domain, c.HostOnly = host, true
	case isSuffix(c.Domain) || net.ParseIP(c.Domain) != nil:
		if c.Domain != host {
			return false
		}
		c.HostOnly = true
	case !domainMatches(host, c.Domain, false):
		return false
	}
	named := c.Path != ""
	if !named {
		c.Path = defaultPath(path)
	}
	prefix := strings.ToLower(c.Name)
	switch {
	case c.Secure && !secure,
		c.HTTPOnly && scripted,
		c.SameSite == None && !c.Secure,
		strings.HasPrefix(prefix, "__secure-") && !c.Secure,
		strings.HasPrefix(prefix, "__host-") && (!c.Secure || domained || !named || c.Path != "/"):
		return false
	}
	return true
}

// shadows reports whether c, a Secure cookie, is one that set, from a
// page not over https, would be sent in place of.
func (c *Cookie) shadows(set *Cookie, now time.Time) bool {
	return c.Secure && !c.expired(now) && c.Name == set.Name && pathMatches(set.Path, c.Path) &&
		(domainMatches(c.Domain, set.Domain, false) || domainMatches(set.Domain, c.Domain, false))
}

// schemeHTTPS is the scheme a Secure cookie goes over.
const schemeHTTPS = "https"

// web reports whether u is an address cookies go to.
func web(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == schemeHTTPS) && u.Hostname() != ""
}

func hostOf(u *url.URL) string { return strings.TrimSuffix(strings.ToLower(u.Hostname()), ".") }

func pathOf(u *url.URL) string {
	if path := u.EscapedPath(); strings.HasPrefix(path, "/") {
		return path
	}
	return "/"
}

// registrable is the domain a site registered, the one its hosts share
// cookies under: host itself for an address in numbers and for a name
// that is a public suffix.
func registrable(host string) string {
	if net.ParseIP(host) != nil {
		return host
	}
	if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return domain
	}
	return host
}

// isSuffix reports whether domain is one sites register under, "com" or
// "co.uk", which no cookie may be of.
func isSuffix(domain string) bool {
	suffix, _ := publicsuffix.PublicSuffix(domain)
	return suffix == domain
}

// siteOf is the site an address is of: its scheme and its registrable
// domain, whatever its port.
func siteOf(u *url.URL) string { return u.Scheme + "://" + registrable(hostOf(u)) }

func domainMatches(host, domain string, hostOnly bool) bool {
	if host == domain {
		return true
	}
	return !hostOnly && strings.HasSuffix(host, "."+domain) && net.ParseIP(host) == nil
}

func pathMatches(path, cookie string) bool {
	if path == cookie {
		return true
	}
	return strings.HasPrefix(path, cookie) && (strings.HasSuffix(cookie, "/") || path[len(cookie)] == '/')
}

// defaultPath is the path of a cookie that names none: the folder of
// the address that set it.
func defaultPath(path string) string {
	if last := strings.LastIndexByte(path, '/'); last > 0 {
		return path[:last]
	}
	return "/"
}
