package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/tabs"
)

func onTheWeb(address string) bool {
	return strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://")
}

// title is what a tab at address is named.
func (s *Screen) title(address string) string {
	name, query, _ := tabs.Own(address)
	if name == tabs.SearchName {
		return query + " - Search"
	}
	if page, ok := s.pageNamed(name); ok {
		return page.title
	}
	if _, source := tabs.Source(address); source {
		return address
	}
	for _, session := range s.sessions {
		if page := session.pages[address]; page != nil && page.Title != "" {
			return page.Title
		}
	}
	return shownAs(address)
}

// language is the language of the page at address, which its title is
// in: its html element's lang, else its Content-Language; "" for a page
// light holds none of, so the machine's faces are chosen by script.
func (s *Screen) language(address string) string {
	for _, session := range s.sessions {
		if page := session.pages[address]; page != nil && page.Doc != nil {
			if lang, ok := page.Doc.HTML().Attr("lang"); ok {
				return lang
			}
			return page.Doc.Language()
		}
	}
	return ""
}

// shownPage is one page as the view shows it: its elements, the class
// that styles them, and whether it scrolls sideways too.
type shownPage struct {
	class string
	wide  bool
	body  []*ui.Node
}

// page is the page of address: one of light's own, the source of one, a
// page of the web, or why there is none.
func (s *Screen) page(address string) *ui.Node {
	shown := s.pageOf(address)
	toggle(s.view, "wide", shown.wide)
	s.shade()
	page := ui.Article(ui.ID("page"), ui.Class(shown.class))
	page.Append(shown.body...)
	return page
}

func (s *Screen) pageOf(address string) shownPage {
	name, query, lights := tabs.Own(address)
	of, source := tabs.Source(address)
	built, known := s.pageNamed(name)
	switch {
	case source:
		return s.sourcePage(of)
	case name == tabs.SearchName:
		return shownPage{class: name, body: s.results(query)}
	case name == scriptsName:
		return s.scriptsBody()
	case known:
		return shownPage{class: name, body: built.body(s)}
	case lights:
		return s.unknown(address, name)
	case onTheWeb(address):
		return s.webPage(address)
	case address == blankAddress:
		return s.blankBody()
	}
	return s.unreachable(address, "light opens "+own+", http:// and https:// addresses")
}

// webPage is the page at an address on the web: fetched off the frame
// the first time it is shown, an empty page until Update has it, then
// shown by its kind.
func (s *Screen) webPage(address string) shownPage {
	page := s.fetched(address)
	switch {
	case page == nil:
		return shownPage{body: []*ui.Node{ui.P(ui.ID("loading"))}}
	case errors.Is(page.Err, context.DeadlineExceeded):
		return s.unreachable(address, s.late(address))
	case page.Err != nil:
		return s.unreachable(address, page.Err.Error())
	case page.Empty:
		return shownPage{class: "blank"}
	}
	switch page.Kind {
	case content.Page:
		return s.hosted(page)
	case content.Picture:
		return s.picturePage(page)
	case content.Text:
		return textPage(page.Source)
	case content.Download:
		return s.savedPage(page)
	}
	return s.unreachable(address, "light does not know how to show it")
}

// hosted is a page of the web built as a Document of its own, shown in
// a frame, so its sheets style nothing of light's, and a panic of it
// stops there (broken).
func (s *Screen) hosted(page *web.Page) shownPage {
	s.host = ui.Iframe(page.Doc, ui.ID("web"), ui.Guarded())
	return shownPage{class: "hosted", body: []*ui.Node{s.host}}
}

// onPage is where the pointer is on the shown page of the web, in the
// space its Document's ElementAt takes; false while it is over
// something else, what light shows over the page included.
func (s *Screen) onPage() (geom.Vec2, bool) {
	at := s.win.Input().Mouse()
	if s.host == nil || s.doc.ElementAt(at) != s.host {
		return geom.Vec2{}, false
	}
	return s.doc.FramePoint(s.host, at)
}

// late says a page was not sent within the time the settings give it.
func (s *Screen) late(address string) string {
	given := fmt.Sprintf("%d seconds", s.config.Timeout)
	if s.config.Timeout == 1 {
		given = "1 second"
	}
	return hostOf(address) + " did not send it within the " + given +
		" a page is given (Request timeout, in the settings)"
}

// unknown says light has no page of that name, and lists those it has.
func (s *Screen) unknown(address, name string) shownPage {
	page := s.unreachable(address, "light has no page "+name+". Its pages are")
	page.body = append(page.body, s.ownPages())
	return page
}

func (s *Screen) unreachable(address, why string) shownPage {
	return shownPage{class: "unreachable", body: []*ui.Node{
		ui.H1(ui.Text("This page can't be reached")),
		ui.P(ui.Code(ui.Text(address)), ui.Text(": "+why+".")),
		ui.P(s.link("Go to the start page", tabs.Home)),
	}}
}

// link is an anchor to address reading text.
func (s *Screen) link(text, address string) *ui.Node {
	return s.anchor(address, ui.Text(text))
}

// linked is the attribute an anchor of light's own pages keeps its
// address in, for the context menu.
const linked = "data-href"

// anchor opens address where the keys held at its click say, and names
// it in the status bubble while the pointer is over it.
func (s *Screen) anchor(address string, args ...ui.Arg) *ui.Node {
	a := ui.Span(ui.Class("link"), ui.TabIndex(0), ui.Attr(linked, address),
		ui.OnClick(func() { s.activate(address) }),
		ui.OnPointerEnter(func() { s.point(address) }),
		ui.OnPointerLeave(func() { s.point("") }))
	for _, arg := range args {
		arg.Apply(a)
	}
	return a
}
