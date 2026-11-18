package app

import (
	"runtime"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// What reloading and the history are called, in the menus, the buttons
// and the pages.
const (
	reloads = "Reload"
	visited = "History"
	saves   = "Downloads"
)

// shortcuts is light's shortcuts as a menu never shown, which ui chooses
// from by its items' keys, a text field focused or not: Command on macOS
// and Control elsewhere with T (a new tab), W (close the tab), L (the
// address), R (reload or stop) and [ or ] (back, forward), and as Chrome
// has them on each system, the history, the downloads, the page's source,
// the developer tools (F12 too, on every system), the settings (,) and the tab's zoom (+, - and 0).
// Off macOS, where they move no caret, Alt with the left or right arrow
// goes back and forward too. A focused text field keeps the keys it
// edits with: on macOS, Command with Y is its redo.
func (s *Screen) shortcuts() *ui.Node {
	menu := ui.Div(ui.ID("keys"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Shortcuts"))
	back, forward, visitedKeys, savesKeys := "Cmd+[", "Cmd+]", "Cmd+Y", "Cmd+Shift+J"
	source, tools := "Cmd+Alt+U", "Cmd+Alt+I F12"
	if runtime.GOOS != "darwin" {
		back, forward = back+" Alt+ArrowLeft", forward+" Alt+ArrowRight"
		visitedKeys, savesKeys = "Ctrl+H", "Ctrl+J"
		source, tools = "Ctrl+U", "Ctrl+Shift+I F12"
	}
	for _, item := range []struct {
		name, keys string
		do         func()
	}{
		{"New tab", "Cmd+T", s.newTab},
		{"New window", "Cmd+N", func() { s.newWindow(s.config.StartPage) }},
		{"Close tab", "Cmd+W", func() { s.closeTab(s.browser.Active) }},
		{"Address", "Cmd+L", func() { s.doc.Focus(s.address) }},
		{reloads, "Cmd+R", s.reloadOrStop},
		{"Back", back, func() { s.move((*tabs.Tab).Back) }},
		{"Forward", forward, func() { s.move((*tabs.Tab).Forward) }},
		{visited, visitedKeys, func() { s.Open(historyPage) }},
		{saves, savesKeys, func() { s.Open(downloadsPage) }},
		{"View page source", source, s.viewSource},
		{"Developer tools", tools, s.toggleTools},
		{settings, "Cmd+,", func() { s.Open(configPage) }},
		{"Zoom in", "Cmd+Plus", func() { s.zoom(1) }},
		{"Zoom out", "Cmd+-", func() { s.zoom(-1) }},
		{"Actual size", "Cmd+0", func() { s.zoom(0) }},
		{"Quit", "Cmd+Q", func() { s.quitting = true }},
	} {
		menu.Append(ui.Div(ui.Attr("role", "menuitem"), ui.Attr("aria-keyshortcuts", item.keys), ui.Text(item.name),
			ui.OnClick(item.do)))
	}
	return menu
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
