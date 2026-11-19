package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
)

const (
	findName = "Find in page"
	findKeys = "Cmd+F"
	// A text field keeps Enter with Shift to itself and tells nothing of
	// the keys held with it, so the previous place has the keys Chrome
	// gives it beside Shift with Enter.
	findNextKeys     = "Cmd+G F3"
	findPreviousKeys = "Cmd+Shift+G Shift+F3"
	// maxFound is the most places of a page a find counts and marks.
	maxFound = 500
)

// findCard is the card that finds words in the shown page of the web,
// at the top right of the page, and what it marks in the page.
type findCard struct {
	root, query, sub, end, body, previous, next, marks, row *ui.Node
	// page is the page searched, found the places it has the words at,
	// shown what is marked in it, these or the passages that mean the
	// words, and at the one the find stands at.
	page         *web.Page
	found, shown []passage
	at           int
	// placed is where what is marked stood when the marks were drawn,
	// stale whether they are to be drawn again whatever moved, and
	// typing whether the words had the focus at the last frame.
	placed        []geom.Rect
	stale, typing bool
	meaning       meaning
}

// findRow is the menu's row that opens the find.
func (s *Screen) findRow() *ui.Node {
	s.find.row = s.choice("find", findName, findKeys, s.openFind)
	return s.find.row
}

// buildFind makes the card, closed, and the layer its marks are drawn
// in over the page.
func (s *Screen) buildFind() *ui.Node {
	f := &s.find
	f.meaning.read = make(chan understood, 1)
	f.query = ui.Input(ui.ID("find-query"), ui.Placeholder(findName), ui.Attr("aria-label", findName),
		ui.OnInput(s.seekWords), ui.OnSubmit(func() { s.stepFind(1) }))
	f.sub = ui.Span(ui.ID("find-sub"))
	f.end = ui.Div(ui.ID("find-end"))
	f.body = ui.Div(ui.ID("find-body"))
	f.previous = ui.Button(ui.ID("find-previous"), ui.Text("Previous"), ui.OnClick(func() { s.stepFind(-1) }))
	f.next = ui.Button(ui.ID("find-next"), ui.Class("agree"), ui.Text("Next"), ui.OnClick(func() { s.stepFind(1) }))
	f.root = ui.Div(ui.ID("find"), ui.Class("sheet"), ui.Attr("role", "dialog"), ui.Attr("aria-label", findName),
		ui.Div(ui.Class("head"), ui.Span(ui.Class("site", "big", "find"), glyph("find")),
			ui.Div(ui.Class("named"), f.query, f.sub), f.end),
		f.body,
		ui.Div(ui.Class("foot"),
			ui.Div(ui.Class("quiet"), ui.Button(ui.ID("find-close"), ui.Text("Close"), ui.OnClick(s.closeFind))),
			ui.Div(ui.Class("quiet"), f.previous, f.next)),
		ui.OnBack(func(e ui.Event) {
			s.closeFind()
			e.StopPropagation()
		}))
	f.marks = ui.Div(ui.ID("find-marks"), ui.Attr("aria-hidden", "true"))
	return ui.Div(f.marks, f.root)
}

func (s *Screen) findOpen() bool { return s.find.root.HasClass("shown") }

// openFind shows the card over a page of the web, its words ready to
// be typed over.
func (s *Screen) openFind() {
	if s.readable() == nil {
		return
	}
	s.menu.Close()
	s.closePanel(false)
	s.closeChoices()
	s.closeCard(false)
	s.closeSheets(s.find.root)
	f := &s.find
	f.root.AddClass("shown")
	s.doc.Focus(f.query)
	s.doc.SetSelectionRange(f.query, 0, utf8.RuneCountInString(s.doc.Value(f.query)))
	s.seekWords()
}

// closeFind closes the card and forgets its words: nothing stays marked.
func (s *Screen) closeFind() {
	f := &s.find
	f.root.RemoveClass("shown")
	s.doc.SetValue(f.query, "")
	f.found, f.shown, f.at, f.typing = nil, nil, 0, false
	s.unmean()
	empty(f.body)
	empty(f.end)
	s.markFound()
}

// seekWords finds the words of the card in the shown page, and stands at
// the first place that has them.
func (s *Screen) seekWords() {
	f := &s.find
	if f.page = s.readable(); f.page == nil {
		s.closeFind()
		return
	}
	f.found, f.at = drawnIn(f.page.Doc, occurrences(f.page.Doc, s.doc.Value(f.query))), 0
	s.unmean()
	if len(f.found) == 0 && strings.TrimSpace(s.doc.Value(f.query)) != "" && s.means() {
		f.meaning.wait = meaningPause
	}
	s.stand()
}

// stepFind moves the find by places, round from the last to the first.
func (s *Screen) stepFind(by int) {
	f := &s.find
	if len(f.shown) == 0 {
		return
	}
	f.at = (f.at + by + len(f.shown)) % len(f.shown)
	s.stand()
}

// stand shows where the find stands: how many places, which one, the
// passages when they are what is marked, and that place in view.
func (s *Screen) stand() {
	f := &s.find
	f.shown = f.found
	if len(f.found) == 0 {
		f.shown = s.meant()
	}
	f.at = min(f.at, max(len(f.shown)-1, 0))
	f.sub.SetText(s.findStands())
	enable(f.previous, len(f.shown) > 0)
	enable(f.next, len(f.shown) > 0)
	s.showMeaning()
	f.stale = true
	if len(f.shown) > 0 {
		f.page.Doc.ScrollIntoView(f.shown[f.at].from.Node.Parent())
	}
}

// findStands says where the find stands under its words.
func (s *Screen) findStands() string {
	f := &s.find
	switch {
	case strings.TrimSpace(s.doc.Value(f.query)) == "":
		return "Type the words to find"
	case len(f.found) > 0:
		return strconv.Itoa(f.at+1) + " of " + strconv.Itoa(len(f.found))
	case s.canMean():
		return s.meaningStands()
	}
	return "Not on this page"
}

// followFind closes the find when its page goes and at Escape, keeps
// the menu's row off where there is no page to search, and draws the
// marks where the page has them now. Escape is read as the bookmark's
// panel reads it (followPanel).
func (s *Screen) followFind() {
	f := &s.find
	page := s.readable()
	if s.choosing() {
		if _, off := f.row.Attr("aria-disabled"); off != (page == nil) {
			if off {
				f.row.RemoveAttr("aria-disabled")
			} else {
				f.row.SetAttr("aria-disabled", "true")
			}
		}
	}
	if f.page != nil && f.page != page {
		s.forgetMeaning()
		f.page = nil
		s.closeFind()
	}
	if !s.findOpen() {
		return
	}
	in := s.win.Input()
	typing, pointing := s.doc.Focused(f.query), in.Down(input.MouseLeft) || in.Released(input.MouseLeft)
	if in.Pressed(input.KeyEscape) || f.typing && !typing && !pointing && !s.focusedIn(s.doc.Body()) {
		s.closeFind()
		return
	}
	f.typing = typing
	s.understand()
	s.markFound()
}

// markFound draws what the find marks over the page, the place it stands at
// stronger: again once the page or what holds a marked text moved.
// ui draws nothing of light's under a page's text, so the marks are
// drawn over it, clear enough to read through.
func (s *Screen) markFound() {
	f := &s.find
	if len(f.shown) == 0 || s.host == nil {
		if f.marks.HasClass("shown") {
			f.marks.RemoveClass("shown")
			empty(f.marks)
		}
		f.placed = f.placed[:0]
		return
	}
	doc := f.page.Doc
	host, _ := s.doc.BoxOf(s.host)
	placed := make([]geom.Rect, 0, len(f.shown)+1)
	placed = append(placed, host)
	for _, p := range f.shown {
		r, _ := doc.BoxOf(p.from.Node.Parent())
		placed = append(placed, r)
	}
	if !f.stale && slices.Equal(placed, f.placed) {
		return
	}
	f.placed, f.stale = placed, false
	stage, _ := s.doc.BoxOf(f.marks.Parent().Parent())
	f.marks.AddClass("shown")
	f.marks.SetStyle(fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx", host.X-stage.X, host.Y-stage.Y,
		host.W, host.H))
	empty(f.marks)
	zoom := doc.Zoom()
	for i, rects := range rectsOf(doc, f.shown) {
		for _, r := range rects {
			r = geom.Rect{X: r.X * zoom, Y: r.Y * zoom, W: r.W * zoom, H: r.H * zoom}
			if r.X+r.W < 0 || r.Y+r.H < 0 || r.X > host.W || r.Y > host.H {
				continue
			}
			m := ui.Div(ui.Style(fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx", r.X, r.Y, r.W, r.H)))
			toggle(m, "current", i == f.at)
			f.marks.Append(m)
		}
	}
}

// rectsOf are the rectangles each passage of a page is drawn in, in
// the page's space. ui gives the rectangles of the selection alone, so
// each passage is selected in turn and the page's own selection put
// back: its scripts read nothing of it, the page's elements being left
// as they are.
func rectsOf(doc *ui.Document, passages []passage) [][]geom.Rect {
	held := doc.Selection()
	out := make([][]geom.Rect, len(passages))
	for i, p := range passages {
		doc.Select(p.from, p.to)
		out[i] = doc.SelectionRects()
	}
	doc.Select(held.Anchor, held.Focus)
	return out
}

// drawnIn are the passages the page draws: not those of an element it
// does not show.
func drawnIn(doc *ui.Document, passages []passage) []passage {
	rects := rectsOf(doc, passages)
	out := passages[:0]
	for i, p := range passages {
		if len(rects[i]) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// fold is a text in lower case with its white space single, and where
// each of its bytes, and its end, is in the text.
func fold(text string) (string, []int) {
	var folded strings.Builder
	at := make([]int, 0, len(text)+1)
	spaced := false
	for i, r := range text {
		if unicode.IsSpace(r) {
			if spaced {
				continue
			}
			r, spaced = ' ', true
		} else {
			r, spaced = unicode.ToLower(r), false
		}
		for range utf8.RuneLen(r) {
			at = append(at, i)
		}
		folded.WriteRune(r)
	}
	return folded.String(), append(at, len(text))
}

// occurrences are the places a page's text has the words of query at,
// whatever their case and the white space between them, in the page's
// order.
func occurrences(doc *ui.Document, query string) []passage {
	needle, _ := fold(query)
	if needle = strings.TrimSpace(needle); needle == "" {
		return nil
	}
	var out []passage
	texts, runs := blocks(doc)
	for i, text := range texts {
		hay, at := fold(text)
		for from := 0; len(out) < maxFound; {
			found := strings.Index(hay[from:], needle)
			if found < 0 {
				break
			}
			start := from + found
			from = start + len(needle)
			end := at[from-1] + utf8.RuneLen(firstRune(text[at[from-1]:]))
			out = append(out, passage{place(runs[i], at[start], false), place(runs[i], end, true), text[at[start]:end]})
		}
	}
	return out
}

func firstRune(text string) rune {
	r, _ := utf8.DecodeRuneInString(text)
	return r
}
