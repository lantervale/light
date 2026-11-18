package app

import (
	"slices"
	"strconv"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/tabs"
)

// stepping is what the tab's history keeps of a frame going somewhere.
// The history of a tab is one, its page's and its frames' (tabs.Tab):
// what a frame is sent to by a link, a form or a script is a step of
// it, as in a browser; what its element first asks for is none, nor is
// what takes the place of its document.
type stepping int

const (
	// aStep is a step of the tab's history.
	aStep stepping = iota
	// byElement is what the frame's element asks for: no step, and the
	// frame is where the history has no word of it.
	byElement
	// inPlace takes the place of the step the frame is at.
	inPlace
	// broughtBack is the tab's history bringing the frame back: no step.
	broughtBack
)

// place is the key of f among the frames of its document: its number
// among those the document ever had, in the order they came, which a
// frame taken away leaves to no other.
func (f *frame) place() string { return strconv.Itoa(f.ord) }

// path is the place of f in its tab's page, as the tab's history
// names it.
func (f *frame) path() string {
	var keys []string
	for at := f; at != nil; at = at.parent {
		keys = append(keys, at.place())
	}
	slices.Reverse(keys)
	return tabs.FramePath(keys...)
}

// byPath is the frame at that place of the page, nil for none.
func (set *framed) byPath(path string) *frame {
	for _, f := range set.all {
		if f.live && !f.gone && f.path() == path {
			return f
		}
	}
	return nil
}

// framed reports whether the shown page has a frame at that place.
func (s *Screen) framed(path string) bool {
	tab := s.browser.Tab()
	set := s.sessionOf(tab).frames[s.sessionOf(tab).pages[tab.Current()]]
	return set != nil && set.byPath(path) != nil
}

// stepped tells the tab's history where q sends f, at address.
func (s *Screen) stepped(f *frame, q wanted, address string) {
	tab := f.set.tab
	if q.form != nil {
		address = q.form.Address
	}
	switch {
	case q.how == broughtBack:
	case q.how == byElement || address == "":
		tab.ReplaceFrame(f.path(), "")
		f.at = ""
	case q.how == inPlace:
		tab.ReplaceFrame(f.path(), address)
		f.at = address
	default:
		tab.VisitFrame(f.path(), address)
		f.at = address
		s.later = append(s.later, s.showNavigation)
	}
}

// follow1 has f show what its element asks for, key, or what the
// tab's history has it at when it comes into its page: a page shown
// again, by a reload or a step back, has its frames where they were. A
// src a script changes later is a step of the history.
func (s *Screen) follow1(f *frame, key string) {
	came := !f.live
	f.live, f.key = true, key
	src, _ := f.node.Attr("src")
	inline, has := f.node.Attr("srcdoc")
	changed := !came && !has && src != f.asked
	f.asked = src
	if came {
		f.began = wanted{from: f.in, ref: src, inline: inline, isInline: has, how: broughtBack}
	}
	if to := f.set.tab.Frame(f.path()); came && to != "" {
		f.at = ""
		s.bring(f, to)
		return
	}
	how := byElement
	if changed {
		how = aStep
	}
	s.navigate(f, wanted{from: f.in, ref: src, inline: inline, isInline: has, how: how})
}

// bring has f where the tab's history has it, to: its document told
// it is back at an address it gave itself, or another loaded, the one
// its element asks for when to is none.
func (s *Screen) bring(f *frame, to string) {
	switch {
	case to == f.at:
	case f.run != nil && f.page != nil && f.load == nil && (to == f.home || f.pushed[to]):
		f.at = to
		if to == "" || to == f.home {
			to = f.first
		}
		f.run.at = to
		f.run.rt.Arrived(to)
	case to == "":
		f.at = ""
		s.navigate(f, f.began)
	default:
		f.at = to
		s.navigate(f, wanted{from: f.in, ref: to, how: broughtBack})
	}
}

// renamed is the document of f giving itself another address, in
// place of its own for "replace", as a step of the tab's history
// otherwise.
func (s *Screen) renamed(f *frame, how, address string) {
	if f.gone || !f.live {
		return
	}
	if f.pushed == nil {
		f.pushed = map[string]bool{}
	}
	f.pushed[address] = true
	if how == "replace" {
		f.set.tab.RenameFrame(f.path(), address)
	} else {
		f.set.tab.PushFrame(f.path(), address)
	}
	f.at = address
}

// arrive shows where a move through the tab's history left it: its
// page when it is another, and its frames where the step has them.
func (s *Screen) arrive(change tabs.Change) {
	s.session().note = ""
	s.session().returned = true
	if change.Top || !change.Moved() {
		s.show()
		return
	}
	s.showNavigation()
}

// travel goes delta steps through the shown tab's history for the
// scripts of the document f shows, the page's for nil. A document in a
// sandbox moves only itself and the frames in it, and its tab's page
// as its sandbox lets it be sent elsewhere.
func (s *Screen) travel(f *frame, user bool, delta int) {
	tab := s.browser.Tab()
	change := tab.Ahead(delta, s.framed)
	if f != nil && f.held.On {
		may := !change.Top || frames.Navigation{Top: true, Activated: user, Sandbox: f.held}.Allowed()
		for _, path := range change.Frames {
			if moved := f.set.byPath(path); moved == nil || !moved.within(f) {
				may = false
			}
		}
		if !may {
			f.set.session.said.Add(console.Line{Level: console.Error, Frame: f.label(), Text: "Unsafe attempt to go " +
				"through the history of the tab from a sandboxed frame: it may move only itself and the frames in it."})
			return
		}
	}
	tab.Take(change)
	s.arrive(change)
}
