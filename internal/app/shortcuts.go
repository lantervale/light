package app

import (
	"runtime"
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// What reloading, the history, the downloads and the bookmarks are
// called, in the menus, the buttons and the pages.
const (
	reloads    = "Reload"
	visited    = "History"
	saves      = "Downloads"
	bookmarked = "Bookmarks"
)

// shortcuts is light's shortcuts as a menu never shown, which ui chooses
// from by its items' keys, a text field focused or not: Command on macOS
// and Control elsewhere with T (a new tab), W (close the tab), L (the
// address), R (reload or stop), D (bookmark the page) and [ or ] (back,
// forward), and as Chrome
// has them on each system, the history, the downloads, the page's source,
// the developer tools (F12 too, on every system), the settings (,) and the tab's zoom (+, - and 0).
// Off macOS, where they move no caret, Alt with the left or right arrow
// goes back and forward too. A focused text field keeps the keys it
// edits with: on macOS, Command with Y is its redo.
func (s *Screen) shortcuts() *ui.Node {
	menu := ui.Div(ui.ID("keys"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Shortcuts"))
	keys := chordsOf(runtime.GOOS)
	for _, item := range []struct {
		name, keys string
		do         func()
	}{
		{"New tab", newTabKeys, s.newTab},
		{"New window", newWindowKeys, func() { s.newWindow(s.config.StartPage) }},
		{"Close tab", "Cmd+W", func() { s.closeTab(s.browser.Active) }},
		{"Address", "Cmd+L", func() { s.doc.Focus(s.address) }},
		{reloads, "Cmd+R", s.reloadOrStop},
		{"Back", keys.back, func() { s.move((*tabs.Tab).Back) }},
		{"Forward", keys.forward, func() { s.move((*tabs.Tab).Forward) }},
		{"Bookmark this page", bookmarkKeys, s.bookmark},
		{findName, findKeys, s.openFind},
		{"Find next", findNextKeys, func() { s.stepFind(1) }},
		{"Find previous", findPreviousKeys, func() { s.stepFind(-1) }},
		{visited, keys.visited, func() { s.Open(historyPage) }},
		{saves, keys.saves, func() { s.Open(downloadsPage) }},
		{"View page source", keys.source, s.viewSource},
		{"Developer tools", keys.tools, s.toggleTools},
		{settings, settingsKeys, func() { s.Open(configPage) }},
		{"Zoom in", zoomInKeys, func() { s.zoom(1) }},
		{"Zoom out", zoomOutKeys, func() { s.zoom(-1) }},
		{"Actual size", "Cmd+0", func() { s.zoom(0) }},
		{"Quit", quitKeys, func() { s.quitting = true }},
	} {
		menu.Append(ui.Div(ui.Attr("role", "menuitem"), ui.Attr("aria-keyshortcuts", item.keys), ui.Text(item.name),
			ui.OnClick(item.do)))
	}
	return menu
}

// The shortcuts every system has, as ui reads them: Cmd is Command on
// macOS and Control elsewhere.
const (
	newTabKeys    = "Cmd+T"
	newWindowKeys = "Cmd+N"
	bookmarkKeys  = "Cmd+D"
	settingsKeys  = "Cmd+,"
	zoomInKeys    = "Cmd+Plus"
	zoomOutKeys   = "Cmd+-"
	quitKeys      = "Cmd+Q"
)

// macOS is the system whose shortcuts are Command's, as runtime.GOOS
// names it.
const macOS = "darwin"

// chords are the shortcuts that differ from a system to another.
type chords struct {
	back, forward, visited, saves, source, tools string
}

// chordsOf is the shortcuts of system, as runtime.GOOS names it.
func chordsOf(system string) chords {
	if system == macOS {
		return chords{back: "Cmd+[", forward: "Cmd+]", visited: "Cmd+Y", saves: "Cmd+Shift+J", source: "Cmd+Alt+U",
			tools: "Cmd+Alt+I F12"}
	}
	return chords{back: "Cmd+[ Alt+ArrowLeft", forward: "Cmd+] Alt+ArrowRight", visited: "Ctrl+H", saves: "Ctrl+J",
		source: "Ctrl+U", tools: "Ctrl+Shift+I F12"}
}

// keysShown is the first of the shortcuts keys names as a menu shows it
// on system, in words, the faces light draws with having no sign for
// Command, Shift or Option: Cmd and Option on macOS, Ctrl and Alt
// elsewhere.
func keysShown(keys, system string) string {
	first, _, _ := strings.Cut(keys, " ")
	parts := strings.Split(first, "+")
	for i, part := range parts {
		switch {
		case part == "Cmd" && system != macOS:
			parts[i] = "Ctrl"
		case part == "Alt" && system == macOS:
			parts[i] = "Option"
		case part == "Plus":
			parts[i] = "+"
		}
	}
	return strings.Join(parts, "+")
}

// viewSource shows the source of the shown tab's page in a new tab,
// which takes the page as it was fetched.
func (s *Screen) viewSource() {
	address := shownBy(s.browser.Tab().Current())
	page := s.session().pages[address]
	s.browser.Open(tabs.SourceOf(address))
	if page != nil {
		s.session().pages[address] = page
	}
	s.show()
}

func (s *Screen) toggleTools() {
	tools := &s.session().tools
	tools.Open = !tools.Open
	s.show()
}
