// Package tabs is light's rules for its tabs, in plain Go: the
// pages a tab went through, the open tabs, and what a typed address means.
package tabs

import (
	"net/url"
	"slices"
	"strings"
)

const (
	// Scheme is the scheme of the addresses of light's own pages.
	Scheme = "light"
	// HomeName and SearchName are the names of the start page and of
	// the page that lists what a search finds.
	HomeName   = "home"
	SearchName = "search"
	// Home is the start page, what a new tab shows unless told.
	Home = Scheme + "://" + HomeName
)

// Page is the address of light's own page name.
func Page(name string) string { return Scheme + "://" + name }

// Search is the address of the page listing what query finds.
func Search(query string) string { return Page(SearchName) + "?q=" + url.QueryEscape(query) }

// Own splits the address of one of light's own pages into the page's
// name and what it is asked; it is false for any other address.
func Own(address string) (string, string, bool) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != Scheme {
		return "", "", false
	}
	return u.Host, u.Query().Get("q"), true
}

// Tab is one tab: its visited addresses and where it stands among them.
type Tab struct {
	urls []string
	at   int
}

func NewTab(address string) *Tab { return &Tab{urls: []string{address}} }

func (h *Tab) Current() string  { return h.urls[h.at] }
func (h *Tab) CanBack() bool    { return h.at > 0 }
func (h *Tab) CanForward() bool { return h.at < len(h.urls)-1 }

// Len is how many pages Back and Forward go through.
func (h *Tab) Len() int { return len(h.urls) }

// Visit goes to address and drops what Forward would have returned to.
func (h *Tab) Visit(address string) {
	if address == h.Current() {
		return
	}
	h.urls = append(h.urls[:h.at+1], address)
	h.at++
}

func (h *Tab) Back() {
	if h.CanBack() {
		h.at--
	}
}

func (h *Tab) Forward() {
	if h.CanForward() {
		h.at++
	}
}

// Redirect names the page the tab stands on by the address its server
// sent it to, when it stands on from.
func (h *Tab) Redirect(from, to string) {
	if h.Current() == from {
		h.urls[h.at] = to
	}
}

// Drop leaves the page the tab stands on for the one before, as if it was
// never visited; a tab's only page stays.
func (h *Tab) Drop() {
	if !h.CanBack() {
		return
	}
	h.urls = h.urls[:h.at]
	h.at--
}

// Browser is the open tabs, the one shown, and the address a tab starts
// at.
type Browser struct {
	Tabs   []*Tab
	Active int
	Start  string
}

// NewBrowser is a browser of one tab, at start.
func NewBrowser(start string) *Browser { return &Browser{Tabs: []*Tab{NewTab(start)}, Start: start} }

func (b *Browser) Tab() *Tab { return b.Tabs[b.Active] }

func (b *Browser) Open(address string) {
	b.Tabs = append(b.Tabs, NewTab(address))
	b.Active = len(b.Tabs) - 1
}

func (b *Browser) Select(i int) {
	if i >= 0 && i < len(b.Tabs) {
		b.Active = i
	}
}

// Close closes tab i; closing the last one leaves a fresh tab at Start.
func (b *Browser) Close(i int) {
	if i < 0 || i >= len(b.Tabs) {
		return
	}
	if len(b.Tabs) == 1 {
		b.Tabs[0] = NewTab(b.Start)
		return
	}
	b.Tabs = append(b.Tabs[:i], b.Tabs[i+1:]...)
	if b.Active > i || b.Active == len(b.Tabs) {
		b.Active--
	}
}

// Duplicate opens a copy of tab i after it, with its way back and forward,
// and shows it.
func (b *Browser) Duplicate(i int) {
	if i < 0 || i >= len(b.Tabs) {
		return
	}
	twin := &Tab{urls: slices.Clone(b.Tabs[i].urls), at: b.Tabs[i].at}
	b.Tabs = slices.Insert(b.Tabs, i+1, twin)
	b.Active = i + 1
}

// CloseOthers closes every tab but i.
func (b *Browser) CloseOthers(i int) {
	if i < 0 || i >= len(b.Tabs) {
		return
	}
	b.Tabs = []*Tab{b.Tabs[i]}
	b.Active = 0
}

// sourcePrefix is what an address begins with to show a page's source.
const sourcePrefix = "view-source:"

// Source is the address whose source address shows, false for an address
// that shows a page.
func Source(address string) (string, bool) {
	return strings.CutPrefix(address, sourcePrefix)
}

// SourceOf is the address that shows the source of address's page.
func SourceOf(address string) string { return sourcePrefix + address }

// Resolve turns what was typed in the address bar into an address: an
// address as it is, a host name as https, anything else as a search, and
// the same after view-source: for the source of a page.
func Resolve(typed string) string {
	typed = strings.TrimSpace(typed)
	if of, ok := Source(typed); ok {
		return SourceOf(Resolve(of))
	}
	switch {
	case typed == "":
		return Home
	case strings.Contains(typed, "://"):
		return typed
	case strings.Contains(typed, ".") && !strings.Contains(typed, " "):
		return "https://" + typed
	}
	return Search(typed)
}
