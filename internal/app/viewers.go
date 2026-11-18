package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/content"
)

// maxShownText is how much of a text is laid out; the rest is said, not
// shown.
const maxShownText = 512 << 10

// alone is a picture shown alone: its element, its own size, whether it
// is shown at it, and the size it was last drawn at.
type alone struct {
	img         *ui.Node
	size, drawn geom.Vec2
	natural     bool
}

// picturePage is a picture alone on a dark page, fitted to the view; a
// click shows it at its own size, and another fits it again.
func (s *Screen) picturePage(page *web.Page) shownPage {
	img := ui.Img(ui.ID("shown-picture"), ui.Attr("src", pictureOf(page.Address)), ui.Attr("alt", page.FileName),
		ui.Attr(pictured, page.Address))
	shown := &alone{img: img, size: geom.V2(float32(page.Size.X), float32(page.Size.Y))}
	frame := ui.Div(ui.ID("picture"), ui.TabIndex(0), ui.Attr("aria-label", "Fit or show at its own size"), img)
	ui.OnClick(func() {
		shown.natural = !shown.natural
		toggle(frame, "natural", shown.natural)
	}).Apply(frame)
	s.alone = shown
	return shownPage{class: "picture", wide: true, body: []*ui.Node{frame}}
}

// fit is the size a picture of size is drawn at in a view: its own, or
// the largest of its shape the view holds.
func fit(size, view geom.Vec2) geom.Vec2 {
	return size.Scale(min(1, view.X/size.X, view.Y/size.Y))
}

// sizePicture keeps the picture shown alone at its own size or fitted to
// the view, as the view changes. One with no size of its own, an SVG, is
// left to the sheet.
func (s *Screen) sizePicture() {
	view, laid := s.doc.BoxOf(s.view)
	if s.alone == nil || s.alone.size.X == 0 || s.alone.size.Y == 0 || !laid {
		return
	}
	size := s.alone.size
	if !s.alone.natural {
		size = fit(size, geom.V2(view.W, view.H))
	}
	if size != s.alone.drawn {
		s.alone.drawn = size
		s.alone.img.SetStyle(fmt.Sprintf("width: %gpx; height: %gpx; max-width: none; max-height: none", size.X, size.Y))
	}
}

// pictured is the attribute a picture of light's own pages keeps its
// address in, for the context menu.
const pictured = "data-src"

// textPage is text as it was sent: in one face, selectable, its lines
// never wrapped.
func textPage(text string) shownPage {
	shown, rest := cutText(text)
	return shownPage{class: "text", wide: true, body: append([]*ui.Node{ui.Pre(ui.ID("text"), ui.Text(shown))},
		rest...)}
}

// cutText is the part of text that is laid out, and a line saying so
// when it is not all of it.
func cutText(text string) (string, []*ui.Node) {
	if len(text) <= maxShownText {
		return content.Spaced(text), nil
	}
	shown := content.Spaced(strings.ToValidUTF8(text[:maxShownText], ""))
	return shown, []*ui.Node{ui.P(ui.ID("cut"), ui.Text(fmt.Sprintf(
		"The first %d kB of %d are shown.", maxShownText>>10, len(text)>>10)))}
}

// sourcePage is the text a page's server sent, its lines numbered in a
// gutter no selection takes.
func (s *Screen) sourcePage(of string) shownPage {
	if !onTheWeb(of) {
		return shownPage{body: []*ui.Node{
			ui.H1(ui.Text("This page has no source")),
			ui.P(ui.Code(ui.Text(of)), ui.Text(" is not a page of the web: light builds its own pages in Go, "+
				"and no server sent a text for it.")),
		}}
	}
	page := s.fetched(of)
	switch {
	case page == nil:
		return shownPage{body: []*ui.Node{ui.P(ui.ID("loading"))}}
	case page.Err != nil:
		return s.unreachable(of, page.Err.Error())
	case page.Kind != content.Page && page.Kind != content.Text:
		return shownPage{body: []*ui.Node{
			ui.H1(ui.Text("This page has no source")),
			ui.P(ui.Code(ui.Text(of)), ui.Text(" is "+page.Media+", not text.")),
		}}
	}
	shown, rest := cutText(page.Source)
	return shownPage{class: "source", wide: true, body: append([]*ui.Node{
		ui.Pre(ui.ID("gutter"), ui.Attr("aria-hidden", "true"), ui.Text(numbers(shown))),
		ui.Pre(ui.ID("source"), ui.Text(shown)),
	}, rest...)}
}

// numbers is the numbers of text's lines, one a line.
func numbers(text string) string {
	lines := strings.Count(text, "\n") + 1
	if strings.HasSuffix(text, "\n") {
		lines--
	}
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}

// savedPage is shown by a tab opened on an answer that is saved, not
// shown: it says so, and leads to the downloads.
func (s *Screen) savedPage(page *web.Page) shownPage {
	return shownPage{class: "download", body: []*ui.Node{
		ui.H1(ui.Text(page.FileName)),
		ui.P(ui.Class("kind"), ui.Text(page.Media)),
		ui.P(ui.Text("light does not show this kind of file: it is downloaded instead. See the "),
			s.link("downloads", downloadsPage), ui.Text(".")),
	}}
}

func bytesOf(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d bytes", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f kB", float64(n)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1000*1000))
}
