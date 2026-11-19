package app

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

const (
	// bookmarksPage is where the bookmarks are listed.
	bookmarksPage = own + bookmarksName
	// bookmarksFile is the bookmarks' file in the folder light is given.
	bookmarksFile = "bookmarks.json"
	// panelNotch and menuNotch are how far from the right edge of the
	// bookmark's panel and of the menu the middle of its notch is, in
	// points.
	panelNotch = 26
	menuNotch  = 15
)

// remark has the bookmarks that changed written, and shown again in the
// windows that show them.
func (p *Profile) remark() {
	p.marked++
	p.markFile.touch()
}

// watchBookmarks shows the bookmarks again once they changed, in this
// window or another: the star, the panel and the page that lists them.
func (s *Screen) watchBookmarks() {
	if s.marksSeen == s.marked {
		return
	}
	s.marksSeen = s.marked
	s.showStar()
	if _, kept := s.marks.Lookup(s.panel.address); !kept {
		s.closePanel(false)
	}
	if s.relistMarks != nil {
		s.relistMarks()
	}
}

// starButton makes the star at the end of the address field, which
// keeps the shown page and opens its bookmark's panel.
func (s *Screen) starButton() *ui.Node {
	s.star = s.button("star", "Bookmark this page", "star", func() {
		if s.panelOpen() {
			s.closePanel(false)
			return
		}
		s.bookmark()
	})
	return s.star
}

// showStar shows the star of a page of the web, filled once the page is
// kept; a page of light's own has none.
func (s *Screen) showStar() {
	address := s.browser.Tab().Current()
	if !onTheWeb(address) {
		s.star.SetAttr("hidden", "")
		return
	}
	s.star.RemoveAttr("hidden")
	_, kept := s.marks.Lookup(address)
	label, icon := "Bookmark this page", "star.svg"
	if kept {
		label, icon = "Edit this bookmark", "star-on.svg"
	}
	toggle(s.star, "on", kept)
	s.star.SetAttr("aria-label", label)
	s.star.Children()[0].SetAttr("src", icon)
}

// bookmark keeps the shown page of the web, under its title and in the
// usual folder, and opens its bookmark's panel.
func (s *Screen) bookmark() {
	address := s.browser.Tab().Current()
	if !onTheWeb(address) {
		return
	}
	if _, kept := s.marks.Lookup(address); !kept {
		s.marks.Add(address, s.title(address), s.marks.Usual(), s.opts.Now())
		s.remark()
		s.showStar()
	}
	s.openPanel(address)
}

// bookmarkPanel is the card that hangs from the star: the bookmark of
// address, named and filed as it is edited.
type bookmarkPanel struct {
	root, notch, tile, name, site, folders *ui.Node
	address                                string
	// naming is whether the field of a new folder stands for "+ New",
	// and typing whether a field of the panel had the focus at the last
	// frame.
	naming, typing bool
}

// buildPanel makes the panel, closed.
func (s *Screen) buildPanel() *ui.Node {
	p := &s.panel
	p.notch = ui.Span(ui.Class("notch"))
	p.tile = ui.Span(ui.ID("bookmark-tile"))
	p.name = ui.Input(ui.ID("bookmark-name"), ui.Attr("aria-label", "Name"),
		ui.OnInput(func() {
			if s.marks.Rename(p.address, s.doc.Value(p.name)) {
				s.remark()
			}
		}),
		ui.OnSubmit(func() { s.closePanel(true) }))
	p.site = ui.Span(ui.ID("bookmark-site"))
	p.folders = ui.Div(ui.ID("bookmark-folders"))
	p.root = ui.Div(ui.ID("bookmark"), ui.Attr("role", "dialog"), ui.Attr("aria-label", "Bookmark"), p.notch,
		ui.Div(ui.Class("head"), p.tile, ui.Div(ui.Class("named"), p.name, p.site)),
		ui.Div(ui.Class("filed"), ui.Span(ui.Text("Folder")), p.folders),
		ui.Div(ui.Class("foot"),
			ui.Button(ui.ID("bookmark-remove"), ui.Text("Remove bookmark"), ui.OnClick(func() {
				s.marks.Remove(p.address)
				s.remark()
				s.closePanel(true)
				s.showStar()
			})),
			ui.Button(ui.ID("bookmark-done"), ui.Text("Done"), ui.OnClick(func() { s.closePanel(true) }))),
		ui.OnBack(func(e ui.Event) {
			s.leavePanel()
			e.StopPropagation()
		}))
	return p.root
}

func (s *Screen) panelOpen() bool { return s.panel.root.HasClass("shown") }

// openPanel shows the panel for the bookmark of address, its name ready
// to be typed over.
func (s *Screen) openPanel(address string) {
	p := &s.panel
	mark, kept := s.marks.Lookup(address)
	if !kept {
		return
	}
	s.menu.Close()
	p.address, p.naming = address, false
	empty(p.tile)
	p.tile.Append(s.siteTile(address, "big"))
	s.doc.SetValue(p.name, mark.Name)
	p.site.SetText(siteOf(address))
	s.showFolders()
	p.root.AddClass("shown")
	s.hang(p.root, s.star, panelNotch)
	s.list.RemoveClass("shown")
	s.recent.RemoveClass("shown")
	s.doc.Focus(p.name)
}

// closePanel closes the panel; refocus gives the star the focus it had.
func (s *Screen) closePanel(refocus bool) {
	if !s.panelOpen() {
		return
	}
	s.panel.root.RemoveClass("shown")
	s.panel.address, s.panel.typing = "", false
	if refocus {
		s.doc.Focus(s.star)
	}
}

// showFolders lists the folders as chips, the bookmark's own marked by a
// check, then what makes a new one. A click on a folder files the
// bookmark there, and one on its own takes it out.
func (s *Screen) showFolders() {
	p := &s.panel
	empty(p.folders)
	mark, _ := s.marks.Lookup(p.address)
	for _, folder := range s.marks.Folders() {
		chosen := folder == mark.Folder
		chip := ui.Button(ui.Class("folder"), ui.Attr("aria-pressed", strconv.FormatBool(chosen)), ui.OnClick(func() {
			to := folder
			if chosen {
				to = ""
			}
			s.file(to)
		}))
		if chosen {
			chip.Append(glyph("check"))
		}
		chip.Append(ui.Span(ui.Text(folder)))
		p.folders.Append(chip)
	}
	if !p.naming {
		p.folders.Append(ui.Button(ui.ID("bookmark-new"), ui.Class("folder"), ui.Text("+ New"), ui.OnClick(func() {
			s.nameFolder(true)
		})))
		return
	}
	field := ui.Input(ui.ID("bookmark-folder"), ui.Placeholder("Folder name"), ui.Attr("aria-label", "New folder"))
	ui.OnSubmit(func() { s.file(s.doc.Value(field)) }).Apply(field)
	p.folders.Append(field)
	s.doc.Focus(field)
}

// nameFolder shows the field of a new folder, or what opens it again.
func (s *Screen) nameFolder(on bool) {
	s.panel.naming, s.panel.typing = on, false
	s.showFolders()
	if !on {
		s.doc.Focus(s.panel.name)
	}
}

// file moves the panel's bookmark to folder, made if need be.
func (s *Screen) file(folder string) {
	s.panel.naming, s.panel.typing = false, false
	s.marks.Move(s.panel.address, folder)
	s.remark()
	s.showFolders()
	s.doc.Focus(s.panel.name)
}

// leavePanel is Escape in the panel: out of the field of a new folder,
// else out of the panel.
func (s *Screen) leavePanel() {
	if s.panel.naming {
		s.nameFolder(false)
		return
	}
	s.closePanel(true)
}

// followPanel keeps the open panel under the star, and closes it at a
// press on anything else and at Escape. The panel hears Back from its
// buttons only: with nothing focused Escape is read here, and from a
// text field ui takes the focus at Escape, so a field of the panel that
// lost the focus to nothing, without a press, is Escape too.
func (s *Screen) followPanel() {
	if !s.panelOpen() {
		return
	}
	p := &s.panel
	s.hang(p.root, s.star, panelNotch)
	in := s.win.Input()
	pointing := in.Down(input.MouseLeft) || in.Released(input.MouseLeft)
	typing := s.doc.Focused(p.name) || s.doc.Focused(s.doc.ByID("bookmark-folder"))
	if in.Pressed(input.KeyEscape) || p.typing && !typing && !pointing && !s.focusedIn(s.doc.Body()) {
		s.leavePanel()
		return
	}
	if p.typing = typing; !in.Pressed(input.MouseLeft) {
		return
	}
	for _, kept := range []*ui.Node{s.panel.root, s.star} {
		if r, ok := s.doc.BoxOf(kept); ok && r.Contains(in.Mouse()) {
			return
		}
	}
	s.closePanel(false)
}

// focusedIn reports whether n or an element in it has the focus.
func (s *Screen) focusedIn(n *ui.Node) bool {
	found := false
	n.Walk(func(n *ui.Node) bool {
		found = found || s.doc.Focused(n)
		return !found
	})
	return found
}

// hang places card, a card of the stage whose notch is notch from its
// right edge, so the notch points at the middle of from, an element of
// the navigation row.
func (s *Screen) hang(card, from *ui.Node, notch float32) {
	stage, _ := s.doc.BoxOf(s.status.Parent())
	anchor, ok := s.doc.BoxOf(from)
	if !ok {
		return
	}
	style := fmt.Sprintf("right: %gpx", max(stage.X+stage.W-anchor.Center().X-notch, 0))
	if card.Style() != style {
		card.SetStyle(style)
	}
}

// siteOf names the site of an address: its host without a leading www.,
// and light for a page of its own.
func siteOf(address string) string {
	u, err := url.Parse(address)
	switch {
	case err != nil:
		return address
	case u.Scheme == tabs.Scheme:
		return tabs.Scheme
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// tints are the colours of the tiles of the sites light holds no icon of.
//
//nolint:gochecknoglobals // a read-only table
var tints = [...]string{"#5aa9e6", "#e6885a", "#5ad18a", "#b48cf2", "#f0b354"}

// siteIcon names the picture that stands for the site of address: the
// icon of that page, else of another page of its site, when light holds
// one, and its logo for a page of its own. Nothing is asked for it.
func (s *Screen) siteIcon(address string) string {
	if name := s.icon(address); name != "" {
		return name
	}
	site, found := siteOf(address), ""
	s.pictures.Range(func(key, _ any) bool {
		name, _ := key.(string)
		if of, ok := strings.CutPrefix(name, faviconOf("")); ok && siteOf(of) == site {
			found = name
		}
		return found == ""
	})
	return found
}

// siteTile is the square that stands for the site of address: its icon,
// else the first letter of its name on a colour of its own.
func (s *Screen) siteTile(address string, classes ...string) *ui.Node {
	tile := ui.Span(ui.Class("site"), ui.Class(classes...), ui.Attr("aria-hidden", "true"))
	if name := s.siteIcon(address); name != "" {
		tile.AddClass("pictured")
		tile.Append(ui.Img(ui.Attr("src", name), ui.Attr("alt", "")))
		return tile
	}
	site := siteOf(address)
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(site))
	tile.SetStyle("background: " + tints[hash.Sum32()%uint32(len(tints))])
	letter := "?"
	for _, r := range site {
		letter = string(unicode.ToUpper(r))
		break
	}
	tile.Append(ui.Span(ui.Text(letter)))
	return tile
}

// bookmarksBody is the bookmarks' page: every bookmark by folder, those
// in none first.
func bookmarksBody(s *Screen) []*ui.Node {
	list := ui.Div(ui.ID("bookmarks-list"))
	body := []*ui.Node{ui.H1(ui.Text(bookmarked))}
	if s.unkept != nil {
		body = append(body, ui.P(ui.ID("bookmarks-problem"), ui.Text("The bookmarks' file is not in use: "+
			s.unkept.Error()+".")))
	}
	s.relistMarks = func() { s.listMarks(list) }
	s.listMarks(list)
	return append(body, list)
}

// listMarks fills list with the bookmarks: a heading a folder, then each
// bookmark's tile, name and address, and a button that removes it.
func (s *Screen) listMarks(list *ui.Node) {
	empty(list)
	if s.marks.Len() == 0 {
		list.Append(ui.P(ui.ID("bookmarks-none"), ui.Text("No page bookmarked. The star in the address field keeps "+
			"the page it shows.")))
	}
	for _, group := range s.marks.Groups() {
		if group.Folder != "" {
			list.Append(ui.H2(ui.Text(group.Folder)))
		}
		for _, mark := range group.Bookmarks {
			list.Append(ui.Div(ui.Class("visit"), s.siteTile(mark.Address), s.link(mark.Name, mark.Address),
				ui.Span(ui.Class("address"), ui.Text(shownAs(mark.Address))),
				ui.Button(ui.Class("remove"), ui.Attr("aria-label", "Remove the bookmark"), ui.Text("×"),
					ui.OnClick(func() {
						s.marks.Remove(mark.Address)
						s.remark()
					}))))
		}
	}
}
