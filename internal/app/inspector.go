package app

import (
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/internal/devtools/inspector"
	"github.com/lantervale/light/internal/menu"
	"github.com/lantervale/light/internal/web"
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
		shown.Frame = s.inspectedFrame(s.session(), page)
		return shown
	}
	if len(s.view.Children()) == 0 {
		return inspector.Page{}
	}
	return inspector.Page{Doc: s.doc, Root: s.view.Children()[0],
		Area: func() (geom.Rect, bool) { return s.doc.BoxOf(s.view) }}
}

// inspectedFrame is the page an iframe of top, or of a frame of it,
// shows, as the Inspector reads it. The developer tools are the
// user's: they read every frame, whatever its origin, and nothing of
// what they read reaches a page's scripts.
func (s *Screen) inspectedFrame(session *session, top *web.Page) func(n *ui.Node) (inspector.Page, bool) {
	var frame func(n *ui.Node) (inspector.Page, bool)
	frame = func(n *ui.Node) (inspector.Page, bool) {
		f := session.frames[top].frameOf(n)
		if f == nil || f.gone || f.page == nil || f.page.Doc == nil || f.in.Doc.Frame(n) != f.page.Doc {
			return inspector.Page{}, false
		}
		shown := inspector.Page{Doc: f.page.Doc, Root: f.page.Doc.HTML(), Frame: frame}
		if f.page.Tree != nil {
			shown.Head, shown.Tag = f.page.Tree.Head(), f.page.Tree.Tag
		}
		return shown, true
	}
	return frame
}

// documentOf is the frame whose document n is of, nil for a node of
// the page itself; false for a node of neither.
func (s *Screen) documentOf(n *ui.Node) (*frame, bool) {
	tab, session := s.browser.Tab(), s.session()
	page := session.pages[tab.Current()]
	if n == nil || page == nil || page.Doc == nil {
		return nil, false
	}
	root := rootOf(n)
	if root == rootOf(page.Doc.HTML()) || page.Tree != nil && root == rootOf(page.Tree.Head()) {
		return nil, true
	}
	for _, f := range session.frames[page].frameList() {
		if f.gone || f.page == nil || f.page.Doc == nil {
			continue
		}
		if root == rootOf(f.page.Doc.HTML()) || f.page.Tree != nil && root == rootOf(f.page.Tree.Head()) {
			return f, true
		}
	}
	return nil, false
}

// name tells the scripts of the shown page and of its frames which
// node the Inspector has selected: the console's $0 in the document
// it is of, nothing in the others.
func (s *Screen) name() {
	if s.host == nil {
		return
	}
	selected := s.inspect.Selected()
	in, _ := s.documentOf(selected)
	set := s.session().frames[s.session().pages[s.browser.Tab().Current()]]
	for _, live := range s.scripted(set) {
		if live == set.scriptsIn(s.session(), in) {
			live.rt.Inspect(selected)
		} else {
			live.rt.Inspect(nil)
		}
	}
}

// contexts are the documents of the shown page that run scripts: what
// the console's line may be evaluated in.
func (s *Screen) contexts() []devtools.Context {
	tab, session := s.browser.Tab(), s.session()
	page := session.pages[tab.Current()]
	if s.host == nil || page == nil || page.Err != nil {
		return nil
	}
	set := session.frames[page]
	all := []devtools.Context{{Name: "top", Origin: originOf(page.Address)}}
	for _, f := range set.frameList() {
		if f.run != nil && !f.gone && f.windowed() {
			all = append(all, devtools.Context{ID: f.id, Name: f.label(), Origin: f.origin})
		}
	}
	return all
}

// evaluateIn runs what the console's line holds in the document of
// the frame whose window is context.
func (s *Screen) evaluateIn(context int, source string) (string, error) {
	set := s.session().frames[s.session().pages[s.browser.Tab().Current()]]
	for _, f := range set.frameList() {
		if f.id == context && f.run != nil && !f.gone {
			return s.evaluated(f.run, source)
		}
	}
	return "", errNoScripts
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

// ofPage is n when it is a node of the shown page or of a frame of
// it, nil otherwise.
func (s *Screen) ofPage(n *ui.Node) *ui.Node {
	root := s.inspected().Root
	for at := n; at != nil && root != nil; at = at.Parent() {
		if at == root {
			return n
		}
	}
	if in, framed := s.documentOf(n); framed && in != nil && s.host != nil {
		return n
	}
	return nil
}
