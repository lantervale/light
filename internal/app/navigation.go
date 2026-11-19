package app

import (
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

// navigation makes the row under the tabs: back, forward, reload or
// stop, home, the address field with its star, the downloads and the
// menu.
func (s *Screen) navigation() *ui.Node {
	s.back = s.button("back", "Back", "back", func() { s.move((*tabs.Tab).Back) })
	s.forward = s.button("forward", "Forward", "forward", func() { s.move((*tabs.Tab).Forward) })
	s.reload = s.button("reload", "Reload", "reload", s.reloadOrStop)
	s.lock = ui.Img(ui.ID("lock"), ui.Attr("alt", ""))
	s.address = ui.Input(ui.ID("address"), ui.Attr("aria-label", "Address"),
		ui.OnSubmit(func() { s.Open(s.Resolve(s.doc.Value(s.address))) }))
	s.sought = ui.Img(ui.ID("sought"), ui.Attr("alt", ""))
	s.field = ui.Div(ui.ID("field"), s.lock, s.sought, s.address, s.starButton())
	return ui.Header(ui.ID("bar"), s.back, s.forward, s.reload,
		s.button("home", "Home", "home", func() { s.Open(s.config.StartPage) }), s.field, s.downloadsButton(),
		s.modelMark(), s.menuButton())
}

// engine is the search engine the settings name.
func (s *Screen) engine() search.Engine { return search.Named(s.config.SearchEngine) }

// search is the address of what the settings' engine finds for query:
// its page that needs no script when light runs none.
func (s *Screen) search(query string) string { return s.engine().Address(query, s.scripting()) }

// mark names the picture that stands for the settings' engine: light's
// logo for its own pages, else the icon of the engine's site once a
// page of it was shown, and a magnifier until then. Nothing is asked
// of the engine for it.
func (s *Screen) mark() string {
	e := s.engine()
	if e.ID == search.Own {
		return logo
	}
	icon := searchMark
	s.pictures.Range(func(key, _ any) bool {
		name, _ := key.(string)
		if address, ok := strings.CutPrefix(name, faviconOf("")); ok && e.Serves(address) {
			icon = name
		}
		return icon == searchMark
	})
	return icon
}

// searching reports whether what is being typed in the address field
// is words for the engine, not an address.
func (s *Screen) searching() bool {
	words := false
	if s.editing {
		tabs.Resolve(s.doc.Value(s.address), func(string) string {
			words = true
			return ""
		})
	}
	return words
}

// seek shows the engine's mark in place of the lock while what is typed
// is words for it.
func (s *Screen) seek() {
	searching := s.searching()
	if searching {
		if mark, _ := s.sought.Attr("src"); mark != s.mark() {
			s.sought.SetAttr("src", s.mark())
		}
	}
	if searching != s.field.HasClass("searching") {
		toggle(s.field, "searching", searching)
	}
}

// Resolve is the address of what was typed, as the address bar reads
// it: an address, or the words the settings' engine is asked for.
func (s *Screen) Resolve(typed string) string { return tabs.Resolve(typed, s.search) }

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
	s.showStar()
}

// locate fills the address field: the whole address while it is edited,
// else as shown, with a lock beside one fetched over https. A move the
// tab waits for that its user asked for is named there at once, marked
// as waited for and without the lock: the page shown is not its page.
// One a page asked for is named when its page shows.
func (s *Screen) locate() {
	address := s.browser.Tab().Current()
	icon := "open.svg"
	if strings.HasPrefix(address, "https://") {
		icon = "lock.svg"
	}
	p := s.session().pending
	if p != nil && !p.named {
		p = nil
	}
	if p != nil {
		address, icon = p.address, "open.svg"
	}
	toggle(s.field, "pending", p != nil)
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
