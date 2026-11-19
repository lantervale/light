package app

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/internal/menu"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

// target is what a right click is on: at most one of a tab of the strip,
// a request of the developer tools and a text field, else a link, a
// picture and selected text in any combination, else the page.
type target struct {
	tab       int
	request   *netlog.Row
	field     *ui.Node
	link      string
	picture   string
	selection string
	// archive is whether the click is on what saves the requests of the
	// developer tools: its button, or their table.
	archive bool
	// node is the element of the page under the click, nil for what is
	// not of the page.
	node *ui.Node
}

// contextMenu opens the menu of what a right click is on, at the pointer.
func (s *Screen) contextMenu(e ui.Event) {
	size := s.win.Size()
	s.menu.Open(s.win.Input().Mouse(), geom.Rect{W: size.X, H: size.Y}, s.entries(s.targetOf(e.Node)))
	e.StopPropagation()
}

// closeMenuWhenMoved closes the menu when what it was opened over moves
// under it: a turn of the wheel, a resize of the window.
func (s *Screen) closeMenuWhenMoved() {
	size := s.win.Size()
	if s.win.Input().Wheel() != (geom.Vec2{}) || size != s.size {
		s.menu.Close()
	}
	s.size = size
}

// targetOf is what n, the element a right click is on, is part of. On
// the frame of a page of the web, it is what that page has under the
// pointer, in the innermost of its own frames: the Inspector is shown
// the page's iframe for what is in one.
func (s *Screen) targetOf(n *ui.Node) target {
	t := target{tab: -1, selection: s.doc.Selection().Text}
	base := s.browser.Tab().Current()
	var frame *ui.Node
	if page := s.session().pages[base]; n == s.host && page != nil && page.Doc != nil {
		at, _ := s.doc.FramePoint(s.host, s.win.Input().Mouse())
		var doc *ui.Document
		n, doc, base, frame = s.innermost(page, at)
		if t.selection = doc.Selection().Text; t.selection == "" {
			t.selection = page.Doc.Selection().Text
		}
	}
	if t.node = s.ofPage(n); t.node == nil && frame != nil {
		t.node = frame
	}
	for ; n != nil; n = n.Parent() {
		if row, ok := s.tools.RowAt(n); ok && t.request == nil {
			t.request = &row
		}
		t.archive = t.archive || n.ID() == devtools.ExportID || n.ID() == "dt-rows"
		if number, ok := strings.CutPrefix(n.ID(), "tab-"); ok && n.HasClass("tab") {
			t.tab, _ = strconv.Atoi(number)
		}
		if n.Tag().String() == "input" && t.field == nil {
			t.field = n
		}
		if t.link == "" {
			t.link = addressIn(n, base, "href", linked)
		}
		if t.picture == "" {
			t.picture = addressIn(n, base, "src", pictured)
		}
	}
	return t
}

// addressIn is the address n names in one of its attributes, read from
// base; "" for none, and for one light does not open.
func addressIn(n *ui.Node, base string, attributes ...string) string {
	from, err := url.Parse(base)
	if err != nil {
		return ""
	}
	for _, name := range attributes {
		ref, ok := n.Attr(name)
		if !ok || strings.HasPrefix(ref, "#") {
			continue
		}
		if to, bad := from.Parse(strings.TrimSpace(ref)); bad == nil && (onTheWeb(to.String()) ||
			to.Scheme == tabs.Scheme) {
			return to.String()
		}
	}
	return ""
}

// entries is the menu of a target, its groups apart.
func (s *Screen) entries(t target) []menu.Entry {
	switch {
	case t.tab >= 0:
		return s.tabEntries(t.tab)
	case t.request != nil:
		return append(s.addressEntries("Open in new tab", "Copy address", t.request.Address),
			s.archiveEntries(t.request.ID)...)
	case t.archive:
		return s.archiveEntries(0)[1:]
	case t.field != nil:
		return s.fieldEntries(t.field)
	}
	var groups [][]menu.Entry
	if t.link != "" {
		groups = append(groups, s.linkEntries(t.link))
	}
	if t.picture != "" {
		groups = append(groups,
			s.savedEntries("Open image in new tab", "Save image as…", "Copy image address", t.picture))
	}
	if t.selection != "" {
		groups = append(groups, []menu.Entry{
			{Label: "Copy", Do: func() { s.win.SetClipboard(t.selection) }},
			{Label: searchLabel(s.engine().Name, t.selection),
				Do: func() { s.openBeside(s.search(words(t.selection)), ahead) }},
		})
	}
	if len(groups) == 0 {
		groups = s.pageEntries()
	}
	if inspect := s.inspectEntries(t.node); inspect != nil {
		groups = append(groups, inspect)
	}
	var entries []menu.Entry
	for i, group := range groups {
		if i > 0 {
			entries = append(entries, menu.Separator())
		}
		entries = append(entries, group...)
	}
	return entries
}

// words is text on one line, a space between its words.
func words(text string) string { return strings.Join(strings.Fields(text), " ") }

// searchLabel names the entry that asks engine for the selected text,
// which it shows the beginning of.
func searchLabel(engine, selection string) string {
	const shown = 30
	query := []rune(words(selection))
	if len(query) > shown {
		query = append(slices.Clone(query[:shown-1]), '…')
	}
	return "Search " + engine + " for “" + string(query) + "”"
}

// linkEntries open a link as the keys held at a click on it do: in a
// tab behind the shown one, or in a window.
func (s *Screen) linkEntries(link string) []menu.Entry {
	return []menu.Entry{
		{Label: "Open link in new tab", Do: func() { s.openBeside(link, behind) }},
		{Label: "Open link in new window", Do: func() { s.openWindow(link) }},
		{Label: "Save link as…", Do: func() { s.download(link, "") }, Disabled: !onTheWeb(link)},
		{Label: "Copy link address", Do: func() { s.win.SetClipboard(link) }},
	}
}

func (s *Screen) addressEntries(opens, copies, address string) []menu.Entry {
	return []menu.Entry{
		{Label: opens, Do: func() { s.openBehind(address) }},
		{Label: copies, Do: func() { s.win.SetClipboard(address) }},
	}
}

// savedEntries is addressEntries with an entry between them that
// downloads address.
func (s *Screen) savedEntries(opens, saves, copies, address string) []menu.Entry {
	entries := s.addressEntries(opens, copies, address)
	return slices.Insert(entries, 1, menu.Entry{Label: saves, Do: func() { s.download(address, "") },
		Disabled: !onTheWeb(address)})
}

func (s *Screen) pageEntries() [][]menu.Entry {
	tab := s.browser.Tab()
	return [][]menu.Entry{
		{
			{Label: "Back", Do: func() { s.move((*tabs.Tab).Back) }, Disabled: !tab.CanBack()},
			{Label: "Forward", Do: func() { s.move((*tabs.Tab).Forward) }, Disabled: !tab.CanForward()},
			{Label: reloads, Do: s.reloadOrStop},
		},
		{
			{Label: "Save page as…", Do: s.savePage, Disabled: s.sourceShown() == nil},
			{Label: "View page source", Do: s.viewSource},
			{Label: visited, Do: func() { s.Open(historyPage) }},
			{Label: saves, Do: func() { s.Open(downloadsPage) }},
			{Label: "Developer tools", Do: s.toggleTools},
			{Label: settings, Do: func() { s.Open(configPage) }},
		},
	}
}

func (s *Screen) tabEntries(i int) []menu.Entry {
	return []menu.Entry{
		{Label: "New tab", Do: s.newTab},
		{Label: reloads, Do: func() {
			s.browser.Select(i)
			s.reloadOrStop()
		}},
		{Label: "Duplicate", Do: func() { s.duplicate(i) }},
		menu.Separator(),
		{Label: "Close tab", Do: func() { s.closeTab(i) }},
		{Label: "Close other tabs", Do: func() {
			s.browser.CloseOthers(i)
			s.show()
		}, Disabled: len(s.browser.Tabs) == 1},
	}
}

// duplicate opens a copy of tab i, which shows the pages its original
// fetched without fetching them again.
func (s *Screen) duplicate(i int) {
	pages := s.sessionOf(s.browser.Tabs[i]).pages
	s.browser.Duplicate(i)
	for address, page := range pages {
		s.session().pages[address] = page
	}
	s.show()
}

// fieldEntries edits a text field as its keys do, on the selection it
// had when the menu opened: the menu takes the focus from it.
func (s *Screen) fieldEntries(field *ui.Node) []menu.Entry {
	from, to := s.doc.SelectionRange(field)
	text := []rune(s.doc.Value(field))
	selected := string(text[from:to])
	edit := func(insert string, selectFrom, selectTo int) {
		s.editing = field == s.address
		s.doc.SetValue(field, string(text[:from])+insert+string(text[to:]))
		s.doc.SetSelectionRange(field, selectFrom, selectTo)
		s.doc.Focus(field)
	}
	return []menu.Entry{
		{Label: "Cut", Disabled: selected == "", Do: func() {
			s.win.SetClipboard(selected)
			edit("", from, from)
		}},
		{Label: "Copy", Disabled: selected == "", Do: func() {
			s.win.SetClipboard(selected)
			edit(selected, from, to)
		}},
		{Label: "Paste", Disabled: s.win.Clipboard() == "", Do: func() {
			pasted := s.win.Clipboard()
			end := from + len([]rune(pasted))
			edit(pasted, end, end)
		}},
		menu.Separator(),
		{Label: "Select all", Disabled: len(text) == 0, Do: func() { edit(selected, 0, len(text)) }},
	}
}
