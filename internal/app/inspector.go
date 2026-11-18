package app

import (
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/internal/devtools/inspector"
	"github.com/lantervale/light/internal/menu"
	"github.com/lantervale/light/pkg/tabs"
)

// inspectorHost is what the Inspector of the developer tools asks of
// the screen.
func (s *Screen) inspectorHost() inspector.Host {
	return inspector.Host{
		Page:     s.inspected,
		Input:    func() *input.State { return s.win.Input() },
		Selected: func(*ui.Node) { s.name() },
		Source:   func(address string, _ int) { s.openBehind(tabs.SourceOf(address)) },
	}
}

// inspected is the page the shown tab shows, as the Inspector reads
// it: a page of the web from its Document's root, with what it keeps
// out of sight as its head, a page of light's own from its element in
// the screen's Document.
func (s *Screen) inspected() inspector.Page {
	if host := s.host; host != nil {
		page := s.session().pages[s.browser.Tab().Current()]
		if page == nil || page.Doc == nil {
			return inspector.Page{}
		}
		shown := inspector.Page{Doc: page.Doc, Root: page.Doc.HTML(), Doctype: true,
			Area: func() (geom.Rect, bool) { return s.doc.BoxOf(host) }}
		if page.Tree != nil {
			shown.Head, shown.Tag = page.Tree.Head(), page.Tree.Tag
		}
		return shown
	}
	if len(s.view.Children()) == 0 {
		return inspector.Page{}
	}
	return inspector.Page{Doc: s.doc, Root: s.view.Children()[0],
		Area: func() (geom.Rect, bool) { return s.doc.BoxOf(s.view) }}
}

// name tells the shown page's scripts, when it runs some, which node
// the Inspector has selected: the console's $0.
func (s *Screen) name() {
	if live := s.session().scripts; live != nil && s.host != nil {
		live.rt.Inspect(s.inspect.Selected())
	}
}

// inspectElement shows n, a node of the shown page, selected in the
// Inspector, opening the developer tools on it.
func (s *Screen) inspectElement(n *ui.Node) {
	tools := &s.session().tools
	tools.Open, tools.Tab = true, devtools.InspectorTab
	s.tools.Refresh()
	s.inspect.Select(n)
}

// inspectEntries is the group of the menu that inspects what a right
// click is on, none for what is not of the page.
func (s *Screen) inspectEntries(n *ui.Node) []menu.Entry {
	if n == nil {
		return nil
	}
	return []menu.Entry{{Label: "Inspect element", Do: func() { s.inspectElement(n) }}}
}

// ofPage is n when it is a node of the shown page, nil otherwise.
func (s *Screen) ofPage(n *ui.Node) *ui.Node {
	root := s.inspected().Root
	for at := n; at != nil && root != nil; at = at.Parent() {
		if at == root {
			return n
		}
	}
	return nil
}
