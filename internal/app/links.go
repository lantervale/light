package app

import (
	"strings"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/tabs"
)

// opening is where a click on a link opens it: in the shown tab, in a
// tab of its own beside it, behind it or shown ahead of it, or in a
// window of its own, apart.
type opening int

const (
	inTab opening = iota
	behind
	ahead
	apart
)

// command is the key shortcuts are made with: Command on macOS, Control
// elsewhere.
func command(system string) input.Key {
	if system == "darwin" {
		return input.KeySuper
	}
	return input.KeyControl
}

// openingBy is where a click opens a link by the keys held with it, as
// Chrome and Firefox have them: the command key or the middle button a
// tab behind the shown one, Shift with either that tab shown, and Shift
// alone a window.
func openingBy(system string, in *input.State, middle bool) opening {
	shift := in.Down(input.KeyShift)
	switch {
	case (middle || in.Down(command(system))) && shift:
		return ahead
	case middle || in.Down(command(system)):
		return behind
	case shift:
		return apart
	}
	return inTab
}

// activate opens the address of a link of light's that was clicked,
// where the keys held say.
func (s *Screen) activate(address string) {
	if !s.elsewhere(address, openingBy(s.system, s.win.Input(), false)) {
		s.Open(address)
	}
}

// followed is activate for a link of the shown tab's page: in that tab,
// the page asked for the address; anywhere else, the player did.
func (s *Screen) followed(address string) {
	if !s.elsewhere(address, openingBy(s.system, s.win.Input(), false)) {
		s.linked(address)
	}
}

// elsewhere opens address where it is asked to but in the shown tab,
// and reports whether it did.
func (s *Screen) elsewhere(address string, where opening) bool {
	switch where {
	case inTab:
		return false
	case apart:
		s.openWindow(address)
	case behind, ahead:
		s.openBeside(address, where)
	}
	return true
}

// openWindow opens address in a window of its own.
func (s *Screen) openWindow(address string) {
	s.visit(address)
	s.newWindow(address)
}

// newWindow asks for a window on address, opened at the next frame as
// this one is, a little down and right of it.
func (s *Screen) newWindow(address string) {
	s.windows = append(s.windows, ask{address: address, from: s})
}

// openBeside opens address in a tab of its own beside the shown one. A
// tab behind loads a page of the web at once; any other page is made
// when the tab is shown.
func (s *Screen) openBeside(address string, where opening) {
	tab := s.browser.OpenBeside(address, where == ahead)
	s.visit(address)
	if where == ahead {
		s.show()
		return
	}
	if onTheWeb(address) {
		s.fetch(tab, address)
	}
	s.showTabs()
}

// openBehind opens address in a tab of its own behind the shown one, as
// the menus and the developer tools ask.
func (s *Screen) openBehind(address string) { s.openBeside(address, behind) }

// openTab opens address in a tab of its own, shown beside the one that
// asks, with the form sent there when it is one's answer.
func (s *Screen) openTab(address string, form *web.Submission) {
	tab := s.browser.OpenBeside(address, true)
	if form != nil {
		s.sessionOf(tab).posts[address] = *form
	}
	s.visit(address)
	s.show()
}

// asksWindow reports whether a target asks for a window of its own,
// which light opens as a tab: _blank, or a name, for one no frame has.
func asksWindow(target string) bool {
	return strings.EqualFold(target, "_blank") || frames.TargetOf(target) == frames.Named
}

// targeted opens address where a link or a form of tab's page with that
// target says, in a frame of the page or in a tab of its own, and
// reports whether it did: the tab itself is left, and so is a link the
// keys held open elsewhere.
func (s *Screen) targeted(tab *tabs.Tab, address, target string, form *web.Submission) bool {
	switch {
	case s.aimedFromTop(tab, address, target, form):
	case !asksWindow(target), form == nil && openingBy(s.system, s.win.Input(), false) != inTab:
		return false
	default:
		s.byPage(func() { s.openTab(address, form) })
	}
	return true
}

// pops reports whether a link or a form of f with that target opens a
// tab of its own: its sandbox may keep it from it. A tab is never
// sandboxed, so a sandbox opens one only when it lets its windows out
// of it, and the console is told of the one that does not.
func (s *Screen) pops(f *frame, target string) bool {
	if !asksWindow(target) || s.reach(f, target) != nil || f.sandbox.On && !f.sandbox.Popups {
		return false
	}
	if f.sandbox.On && !f.sandbox.PopupsEscape {
		f.set.session.said.Add(console.Line{Level: console.Error, Text: "Blocked opening a new window from a " +
			"sandboxed frame: light opens no sandboxed tab, and the frame's 'allow-popups-to-escape-sandbox' " +
			"permission is not set."})
		return false
	}
	return true
}

// followUnderMiddle opens the link the middle button went down and came
// up on in a tab of its own. The page's scripts hear no click of it.
func (s *Screen) followUnderMiddle() {
	in := s.win.Input()
	pressed, released := in.Pressed(input.MouseMiddle), in.Released(input.MouseMiddle)
	if !pressed && !released {
		return
	}
	link := ""
	if !s.overlaid() && s.tabAt(in.Mouse()) < 0 {
		link = s.targetOf(s.doc.ElementAt(in.Mouse())).link
	}
	switch {
	case pressed:
		if s.aimed = link; link != "" {
			s.blur()
		}
	case link != "" && link == s.aimed:
		s.openBeside(link, openingBy(s.system, in, true))
	}
}

// blur takes the keyboard from the field that has it, as a press on
// what is no field does: the keys held are hidden while one has it.
func (s *Screen) blur() {
	s.doc.Focus(nil)
	if page := s.session().pages[s.browser.Tab().Current()]; page != nil && page.Doc != nil {
		page.Doc.Focus(nil)
	}
}

// held are the keys held with a click, as its event tells them to a
// page's scripts.
func (s *Screen) held() dom.Keys {
	in := s.win.Input()
	return dom.Keys{Ctrl: in.Down(input.KeyControl), Shift: in.Down(input.KeyShift), Alt: in.Down(input.KeyAlt),
		Meta: in.Down(input.KeySuper)}
}
