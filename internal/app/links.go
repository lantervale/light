package app

import (
	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/internal/dom"
)

// opening is where a click on a link opens it: in the shown tab, or in a
// tab of its own beside it, behind it or shown ahead of it.
type opening int

const (
	inTab opening = iota
	behind
	ahead
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
// tab behind the shown one, Shift with either that tab shown.
func openingBy(system string, in *input.State, middle bool) opening {
	switch {
	case !middle && !in.Down(command(system)):
		return inTab
	case in.Down(input.KeyShift):
		return ahead
	}
	return behind
}

// activate opens the address of a link of light's that was clicked,
// where the keys held say.
func (s *Screen) activate(address string) {
	if where := openingBy(s.system, s.win.Input(), false); where != inTab {
		s.openBeside(address, where)
		return
	}
	s.Open(address)
}

// followed is activate for a link of the shown tab's page: in that tab,
// the page asked for the address; in a tab of its own, the player did.
func (s *Screen) followed(address string) {
	if where := openingBy(s.system, s.win.Input(), false); where != inTab {
		s.openBeside(address, where)
		return
	}
	s.linked(address)
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
		s.aimed = link
	case link != "" && link == s.aimed:
		s.openBeside(link, openingBy(s.system, in, true))
	}
}

// held are the keys held with a click, as its event tells them to a
// page's scripts.
func (s *Screen) held() dom.Keys {
	in := s.win.Input()
	return dom.Keys{Ctrl: in.Down(input.KeyControl), Shift: in.Down(input.KeyShift), Alt: in.Down(input.KeyAlt),
		Meta: in.Down(input.KeySuper)}
}
