package main

import (
	"net/url"
	"strings"
)

const home = "ember://home"

// demo is the page of the web the start page offers.
const demo = "https://csszengarden.com"

// History is one tab's visited addresses and where it stands among them.
type History struct {
	urls []string
	at   int
}

func NewHistory(address string) *History { return &History{urls: []string{address}} }

func (h *History) Current() string  { return h.urls[h.at] }
func (h *History) CanBack() bool    { return h.at > 0 }
func (h *History) CanForward() bool { return h.at < len(h.urls)-1 }

// Visit goes to address and drops what Forward would have returned to.
func (h *History) Visit(address string) {
	if address == h.Current() {
		return
	}
	h.urls = append(h.urls[:h.at+1], address)
	h.at++
}

func (h *History) Back() {
	if h.CanBack() {
		h.at--
	}
}

func (h *History) Forward() {
	if h.CanForward() {
		h.at++
	}
}

// Browser is the open tabs and the one shown.
type Browser struct {
	Tabs   []*History
	Active int
}

func NewBrowser() *Browser { return &Browser{Tabs: []*History{NewHistory(home)}} }

func (b *Browser) Tab() *History { return b.Tabs[b.Active] }

func (b *Browser) Open(address string) {
	b.Tabs = append(b.Tabs, NewHistory(address))
	b.Active = len(b.Tabs) - 1
}

func (b *Browser) Select(i int) {
	if i >= 0 && i < len(b.Tabs) {
		b.Active = i
	}
}

// Close closes tab i; closing the last one leaves a fresh tab on home.
func (b *Browser) Close(i int) {
	if i < 0 || i >= len(b.Tabs) {
		return
	}
	if len(b.Tabs) == 1 {
		b.Tabs[0] = NewHistory(home)
		return
	}
	b.Tabs = append(b.Tabs[:i], b.Tabs[i+1:]...)
	if b.Active > i || b.Active == len(b.Tabs) {
		b.Active--
	}
}

// Resolve turns what was typed in the address bar into an address: an
// address as it is, a host name as https, anything else as a search.
func Resolve(typed string) string {
	typed = strings.TrimSpace(typed)
	switch {
	case typed == "":
		return home
	case strings.Contains(typed, "://"):
		return typed
	case strings.Contains(typed, ".") && !strings.Contains(typed, " "):
		return "https://" + typed
	}
	return "ember://search?q=" + url.QueryEscape(typed)
}
