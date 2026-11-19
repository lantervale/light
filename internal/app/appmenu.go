package app

import (
	"runtime"

	"github.com/lantervale/ember/ui"
)

// menuButton makes the navigation row's last button and the menu it
// opens, a card hanging from it: what light does, each with its glyph
// and its keys. ui opens it at a click or by Enter on the button, walks
// it by the arrows, chooses by Enter or a click, and closes it by Escape,
// which gives the button the focus back, or a press elsewhere.
func (s *Screen) menuButton() *ui.Node {
	keys := chordsOf(runtime.GOOS)
	s.zoomed = ui.Span(ui.ID("menu-zoom"))
	s.keeps = s.choice("save", "Save page as…", "", s.savePage)
	s.choices = ui.Div(ui.ID("app-menu"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Menu"),
		ui.Span(ui.Class("notch")),
		s.choice("tab", "New tab", newTabKeys, s.newTab),
		s.choice("window", "New window", newWindowKeys, func() { s.newWindow(s.config.StartPage) }),
		ui.Div(ui.Attr("role", "separator")),
		s.choice("star", bookmarked, "", func() { s.Open(bookmarksPage) }),
		s.choice("history", visited, keys.visited, func() { s.Open(historyPage) }),
		s.choice("downloads", saves, keys.saves, func() { s.Open(downloadsPage) }),
		ui.Div(ui.Attr("role", "separator")),
		ui.Div(ui.Class("zoom"), glyph("row-zoom"), ui.Span(ui.Class("label"), ui.Text("Zoom")),
			ui.Span(ui.Class("steps"), s.step("zoom-out", "Zoom out", "minus", -1), s.zoomed,
				s.step("zoom-in", "Zoom in", "plus", 1)),
			ui.Div(ui.ID("menu-full"), ui.Attr("role", "menuitem"), ui.Attr("aria-label", "Full screen"),
				glyph("row-full"), ui.OnClick(func() { s.win.SetFullscreen(!s.win.Fullscreen()) }))),
		ui.Div(ui.Attr("role", "separator")),
		s.keeps,
		ui.Div(ui.Attr("role", "separator")),
		s.choice("code", "Developer tools", keys.tools, s.toggleTools),
		s.choice("settings", settings, settingsKeys, func() { s.Open(configPage) }),
		ui.Div(ui.Attr("role", "separator")),
		s.choice("quit", "Quit light", quitKeys, func() { s.quitting = true }))
	s.more = ui.Button(ui.ID("menu"), ui.Attr("aria-label", "Menu"), ui.Attr("aria-haspopup", "menu"),
		ui.Attr("aria-controls", "app-menu"), glyph("menu"))
	return s.more
}

// choice is a row of the menu named after its glyph: the glyph, what it
// reads and the keys that do the same, none for "".
func (s *Screen) choice(name, label, keys string, do func()) *ui.Node {
	row := ui.Div(ui.ID("menu-"+name), ui.Attr("role", "menuitem"), ui.Attr("aria-label", label), glyph("row-"+name),
		ui.Span(ui.Class("label"), ui.Text(label)), ui.OnClick(do))
	if keys != "" {
		row.Append(ui.Span(ui.Class("keys"), ui.Text(keysShown(keys, runtime.GOOS))))
	}
	return row
}

// step is an end of the menu's zoom: it moves the shown tab's zoom, and
// the menu stays open on it.
func (s *Screen) step(name, label, icon string, by int) *ui.Node {
	var item *ui.Node
	item = ui.Div(ui.ID("menu-"+name), ui.Attr("role", "menuitem"), ui.Attr("aria-label", label), glyph("row-"+icon),
		ui.OnClick(func() {
			s.zoom(by)
			s.again = item
		}))
	return item
}

// choosing reports whether the menu is open.
func (s *Screen) choosing() bool {
	_, open := s.choices.Attr("open")
	return open
}

func (s *Screen) closeChoices() {
	s.choices.RemoveAttr("open")
	s.more.SetAttr("aria-expanded", "false")
}

// followChoices opens the menu again on the entry that keeps it open,
// which ui closed it at, and while it is open keeps it under its button,
// alone over the page, its zoom the shown tab's and its entry that saves
// the page off where there is nothing to save.
func (s *Screen) followChoices() {
	if s.again != nil {
		s.choices.SetAttr("open", "")
		s.more.SetAttr("aria-expanded", "true")
		s.doc.Focus(s.again)
		s.again = nil
	}
	if !s.choosing() {
		return
	}
	s.hang(s.choices, s.more, menuNotch)
	s.list.RemoveClass("shown")
	s.recent.RemoveClass("shown")
	if zoom := percent(s.zoomOf(s.session()), ""); s.zoomed.Text() != zoom {
		s.zoomed.SetText(zoom)
	}
	if _, off := s.keeps.Attr("aria-disabled"); off != (s.sourceShown() == nil) {
		if off {
			s.keeps.RemoveAttr("aria-disabled")
		} else {
			s.keeps.SetAttr("aria-disabled", "true")
		}
	}
}
