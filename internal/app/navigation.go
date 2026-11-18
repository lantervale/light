package app

import (
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// navigation makes the row under the tabs: back, forward, reload or
// stop, home, the address field and the downloads.
func (s *Screen) navigation() *ui.Node {
	s.back = s.button("back", "Back", "back", func() { s.move((*tabs.Tab).Back) })
	s.forward = s.button("forward", "Forward", "forward", func() { s.move((*tabs.Tab).Forward) })
	s.reload = s.button("reload", "Reload", "reload", s.reloadOrStop)
	s.lock = ui.Img(ui.ID("lock"), ui.Attr("alt", ""))
	s.address = ui.Input(ui.ID("address"), ui.Attr("aria-label", "Address"),
		ui.OnSubmit(func() { s.Open(tabs.Resolve(s.doc.Value(s.address))) }))
	s.field = ui.Div(ui.ID("field"), s.lock, s.address)
	return ui.Header(ui.ID("bar"), s.back, s.forward, s.reload,
		s.button("home", "Home", "home", func() { s.Open(s.config.StartPage) }), s.field, s.downloadsButton())
}

func (s *Screen) button(id, label, icon string, click func()) *ui.Node {
	return ui.Button(ui.ID(id), ui.Attr("aria-label", label), glyph(icon), ui.OnClick(click))
}

// showNavigation shows the row for the shown tab: its address, where it
// can go, and reload or stop.
func (s *Screen) showNavigation() {
	tab := s.browser.Tab()
	s.locate()
	enable(s.back, tab.CanBack())
	enable(s.forward, tab.CanForward())
	label, icon := reloads, "reload.svg"
	if s.loading() {
		label, icon = "Stop", "stop.svg"
	}
	s.reload.SetAttr("aria-label", label)
	s.reload.Children()[0].SetAttr("src", icon)
}

// locate fills the address field: the whole address while it is edited,
// else as shown, with a lock beside one fetched over https.
func (s *Screen) locate() {
	address := s.browser.Tab().Current()
	icon := "open.svg"
	if strings.HasPrefix(address, "https://") {
		icon = "lock.svg"
	}
	s.lock.SetAttr("src", icon)
	toggle(s.field, "editing", s.editing)
	if !s.editing {
		address = shownAs(address)
	}
	s.doc.SetValue(s.address, address)
}

// shownAs is an address as the address field and the status bubble show
// it: an https one without its scheme, any other whole.
func shownAs(address string) string { return strings.TrimPrefix(address, "https://") }
