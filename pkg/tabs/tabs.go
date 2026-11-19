// Package tabs is light's rules for its tabs, in plain Go: the
// pages a tab went through, the open tabs, and what a typed address means.
package tabs

import (
	"maps"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
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

// Tab is one tab: the pages it went through, where it stands among
// them, and the tab it was opened beside. Its pages and what their
// frames went through are one history, as browsers keep it: each step
// is a page with the address each of its frames that moved is at.
type Tab struct {
	steps []step
	at    int
	from  *Tab
}

// step is one step of a tab's history: the address of its page, and
// those of its frames that went elsewhere than their element asks, by
// their place in the page (FramePath).
type step struct {
	address string
	frames  map[string]string
}

func NewTab(address string) *Tab { return &Tab{steps: []step{{address: address}}} }

func (h *Tab) Current() string  { return h.steps[h.at].address }
func (h *Tab) CanBack() bool    { return h.at > 0 }
func (h *Tab) CanForward() bool { return h.at < len(h.steps)-1 }

// Len is how many steps Back and Forward go through, those of the
// page's frames among them.
func (h *Tab) Len() int { return len(h.steps) }

// Visit goes to address and drops what Forward would have returned to.
func (h *Tab) Visit(address string) {
	if address == h.Current() {
		return
	}
	h.add(step{address: address})
}

// Push is Visit for an address the page gave itself: its frames stay
// where they are.
func (h *Tab) Push(address string) {
	if address == h.Current() {
		return
	}
	h.add(step{address: address, frames: maps.Clone(h.steps[h.at].frames)})
}

func (h *Tab) add(next step) {
	h.steps = append(h.steps[:h.at+1], next)
	h.at++
}

// FramePath is the place of a frame in its page: the key of each
// frame it is in, from the page down, then its own: whatever names a
// frame among those of its document.
func FramePath(keys ...string) string {
	escaped := make([]string, len(keys))
	for i, key := range keys {
		escaped[i] = url.PathEscape(key)
	}
	return strings.Join(escaped, "/")
}

// Frame is the address the frame at path is at, "" for what its
// element asks.
func (h *Tab) Frame(path string) string { return h.steps[h.at].frames[path] }

// VisitFrame is a step of the frame at path to address: the page
// stays, the frames in that one start again, and what Forward would
// have returned to is dropped.
func (h *Tab) VisitFrame(path, address string) {
	if address == h.Frame(path) {
		return
	}
	h.add(step{address: h.Current(), frames: h.framesWith(path, address)})
}

// ReplaceFrame names the address the frame at path is at, without a
// step: "" for what its element asks.
func (h *Tab) ReplaceFrame(path, address string) {
	h.steps[h.at].frames = h.framesWith(path, address)
}

// PushFrame is VisitFrame for an address the frame's document gave
// itself: the frames in it stay where they are.
func (h *Tab) PushFrame(path, address string) {
	if address == h.Frame(path) {
		return
	}
	frames := maps.Clone(h.steps[h.at].frames)
	if frames == nil {
		frames = map[string]string{}
	}
	frames[path] = address
	h.add(step{address: h.Current(), frames: frames})
}

// framesWith are the frames of the step the tab stands on with the one
// at path at address, and none in it.
func (h *Tab) framesWith(path, address string) map[string]string {
	frames := map[string]string{}
	for at, to := range h.steps[h.at].frames {
		if at != path && !strings.HasPrefix(at, path+"/") {
			frames[at] = to
		}
	}
	if address != "" {
		frames[path] = address
	}
	return frames
}

// RenameFrame is ReplaceFrame for an address the frame's document gave
// itself: the frames in it stay where they are.
func (h *Tab) RenameFrame(path, address string) {
	if h.steps[h.at].frames == nil {
		h.steps[h.at].frames = map[string]string{}
	}
	h.steps[h.at].frames[path] = address
}

// Change is what a move through a tab's history changes: the page, or
// the frames of it at those paths, the outer ones only.
type Change struct {
	Top    bool
	Frames []string
	from   int
	to     int
}

// Moved reports whether the move changes anything.
func (c Change) Moved() bool { return c.Top || len(c.Frames) > 0 }

// At is the step the tab stands on, the first being 0.
func (h *Tab) At() int { return h.at }

// Since is what differs between the step numbered from and the one
// the tab stands on. has reports whether the page still has a frame at
// a path; nil when it has them all.
func (h *Tab) Since(from int, has func(path string) bool) Change { return h.between(from, h.at, has) }

func (h *Tab) between(from, to int, has func(path string) bool) Change {
	c := Change{from: from, to: to}
	a, b := h.steps[from], h.steps[to]
	if a.address != b.address {
		c.Top = true
		return c
	}
	var paths []string
	for path := range a.frames {
		if b.frames[path] != a.frames[path] {
			paths = append(paths, path)
		}
	}
	for path := range b.frames {
		if _, known := a.frames[path]; !known {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		inner := slices.ContainsFunc(c.Frames, func(outer string) bool { return strings.HasPrefix(path, outer+"/") })
		if !inner && (has == nil || has(path)) {
			c.Frames = append(c.Frames, path)
		}
	}
	return c
}

// Ahead is what going delta steps would change, without going: back
// for a negative delta. A step that would change only frames the page
// has no more is passed over, and a move past either end changes
// nothing.
func (h *Tab) Ahead(delta int, has func(path string) bool) Change {
	at, by := h.at, 1
	if delta < 0 {
		by, delta = -1, -delta
	}
	for ; delta > 0; delta-- {
		next := at + by
		for next >= 0 && next < len(h.steps) && !h.between(at, next, has).Moved() {
			next += by
		}
		if next < 0 || next >= len(h.steps) {
			return Change{from: h.at, to: h.at}
		}
		at = next
	}
	return h.between(h.at, at, has)
}

// Take makes the move Ahead told of.
func (h *Tab) Take(c Change) {
	if c.from == h.at && c.to >= 0 && c.to < len(h.steps) {
		h.at = c.to
	}
}

// Travel goes delta steps as Ahead tells, and says what changed.
func (h *Tab) Travel(delta int, has func(path string) bool) Change {
	c := h.Ahead(delta, has)
	h.Take(c)
	return c
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
		h.steps[h.at].address = to
	}
}

// Drop leaves the page the tab stands on for the one before, as if it was
// never visited; a tab's only page stays.
func (h *Tab) Drop() {
	if !h.CanBack() {
		return
	}
	h.steps = h.steps[:h.at]
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

// OpenBeside opens a tab at address after the shown one and those opened
// beside it already, and shows it when shown says so.
func (b *Browser) OpenBeside(address string, shown bool) *Tab {
	tab := NewTab(address)
	tab.from = b.Tab()
	i := b.Active + 1
	for i < len(b.Tabs) && b.Tabs[i].from == tab.from {
		i++
	}
	b.Tabs = slices.Insert(b.Tabs, i, tab)
	if shown {
		b.Active = i
	}
	return tab
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
	twin := &Tab{steps: slices.Clone(b.Tabs[i].steps), at: b.Tabs[i].at}
	for at := range twin.steps {
		twin.steps[at].frames = maps.Clone(twin.steps[at].frames)
	}
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
// address as it is, a host name as https, this machine and a numbered
// host as http, anything else as what search makes of the words (Search
// when nil), and the same after view-source: for the source of a page.
func Resolve(typed string, search func(query string) string) string {
	typed = strings.TrimSpace(typed)
	if of, ok := Source(typed); ok {
		return SourceOf(Resolve(of, search))
	}
	switch {
	case typed == "":
		return Home
	case schemed(typed):
		return typed
	case strings.ContainsFunc(typed, unicode.IsSpace):
	case local(hostOf(typed)):
		return "http://" + typed
	case named(hostOf(typed)):
		return "https://" + typed
	}
	if search == nil {
		search = Search
	}
	return search(typed)
}

// schemed reports whether typed begins as an address does: a scheme
// and ://, or about:.
func schemed(typed string) bool {
	scheme, _, ok := strings.Cut(typed, ":")
	if !ok || scheme == "" || !strings.HasPrefix(typed[len(scheme):], "://") && scheme != "about" {
		return false
	}
	return !strings.ContainsFunc(scheme, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("+-.", r)
	})
}

// hostOf is the host what was typed without a scheme begins with: up to
// its path, without its port.
func hostOf(typed string) string {
	host := typed
	if end := strings.IndexAny(host, "/?#"); end >= 0 {
		host = host[:end]
	}
	if at := strings.LastIndex(host, ":"); at >= 0 && !strings.HasSuffix(host, "]") {
		if _, err := strconv.ParseUint(host[at+1:], 10, 16); err == nil {
			host = host[:at]
		}
	}
	return host
}

// local reports whether host is this machine by its name, or a host by
// its number: what is asked over http.
func local(host string) bool {
	if inner, ok := strings.CutPrefix(host, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		addr, err := netip.ParseAddr(inner)
		return ok && err == nil && addr.Is6()
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Is4()
	}
	const name = "localhost"
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == name || strings.HasSuffix(host, "."+name)
}

// named reports whether host reads as a name of the Internet: labels
// between dots, none empty but after a last dot.
func named(host string) bool {
	host = strings.TrimSuffix(host, ".")
	return strings.Contains(host, ".") && !slices.Contains(strings.Split(host, "."), "")
}
