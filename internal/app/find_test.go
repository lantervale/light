package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
	"github.com/lantervale/light/pkg/tabs"
)

// crossings is a site whose page says when a ticket is paid back without
// the words "money back", with a last line far under the window.
func crossings(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Tickets</title><style>body { margin: 20px; font: 16px/24px sans-serif; }
			#gone { display: none; }</style>
			<h1>Ticket conditions</h1>
			<h2>Changing a crossing</h2>
			<p id="moved">A ticket can be moved to another sailing on the same day at no charge.</p>
			<h2>Cancelled sailings</h2>
			<p id="cancelled">When a sailing is cancelled, we return the full fare to the card you paid with.
				You do not need to ask.</p>
			<h2>If you cannot travel</h2>
			<p id="travel">Tell us before departure and you get the price of the <b>ticket</b> back, less a fee.
				<a id="terms" href="/terms">After departure, tickets are not reimbursed.</a></p>
			<h2>Season passes</h2>
			<p id="season">Unused months are credited to your account if you move away.</p>
			<p id="gone">A ticket nobody sees.</p>
			<div style="height: 2000px"></div>
			<p id="far">A last ticket, far below.</p>`),
		"/terms": typed("text/html", `<title>Terms</title><p>The terms of a ticket.</p>`),
	})
	return srv
}

// kept is a light whose requests stay on this machine, and keptOn one
// whose model is on and ready too.
func kept(t *testing.T) *shown {
	t.Helper()
	return showWith(t, Options{Downloads: t.TempDir(), Transport: &fenced{}})
}

func keptOn(t *testing.T) (*shown, *model.Fake) {
	t.Helper()
	s, fake := modelled(t, Options{Transport: &fenced{}})
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	return s, fake
}

func (s *shown) findShown() bool { return s.find.root.HasClass("shown") }

// finds opens the find by its keys and types words in it.
func (s *shown) finds(words string) {
	s.t.Helper()
	s.command(input.KeyF)
	if !s.findShown() || !s.doc.Focused(s.find.query) {
		s.t.Fatalf("the keys opened the find %v, its words focused %v", s.findShown(), s.doc.Focused(s.find.query))
	}
	s.types(words)
}

// stands is what the find says under its words.
func (s *shown) stands() string { return s.find.sub.Text() }

// findMarks are the marks drawn over the page, in the window, and which
// of them are the place the find stands at.
func (s *shown) findMarks() ([]geom.Rect, []geom.Rect) {
	var all, current []geom.Rect
	if !s.find.marks.HasClass("shown") {
		return nil, nil
	}
	for _, m := range s.find.marks.Children() {
		r, ok := s.doc.BoxOf(m)
		if !ok {
			s.t.Fatalf("a mark has no box")
		}
		all = append(all, r)
		if m.HasClass("current") {
			current = append(current, r)
		}
	}
	return all, current
}

// under is the id of the page's element the middle of a mark is over.
func (s *shown) under(page *ui.Document, mark geom.Rect) string {
	s.t.Helper()
	at, ok := s.doc.FramePoint(s.host, mark.Center())
	if !ok {
		s.t.Fatalf("the mark %v is not over the page", mark)
	}
	for n := page.ElementAt(at); n != nil; n = n.Parent() {
		if n.ID() != "" {
			return n.ID()
		}
	}
	return ""
}

// refinds types other words over those of the open find.
func (s *shown) refinds(words string) {
	s.t.Helper()
	s.doc.Focus(s.find.query)
	s.frame()
	s.command(input.KeyA)
	s.types(words)
}

// shifted presses a key while Shift is held.
func (s *shown) shifted(k input.Key) {
	s.in.SetKey(input.KeyLeftShift, true)
	s.frame()
	s.press(k)
	s.in.SetKey(input.KeyLeftShift, false)
	s.frame()
}

func markup(doc *ui.Document) string {
	var out strings.Builder
	doc.HTML().Walk(func(n *ui.Node) bool {
		if n.IsText() {
			out.WriteString(n.Text())
			return true
		}
		out.WriteString("<" + n.Tag().String() + " " + n.ID() + " " + strings.Join(n.Classes(), ".") + " " + n.Style() + ">")
		return true
	})
	return out.String()
}

func TestTheFindOpensAtTheTopRightOfThePageAndMarksEveryPlaceThatHasTheWords(t *testing.T) {
	srv := crossings(t)
	s := kept(t)
	page := s.visit(srv.URL + "/")
	before := markup(page)
	if s.findShown() {
		t.Fatal("the find starts open")
	}
	s.finds("TICKET")
	if got := s.stands(); got != "1 of 5" {
		t.Fatalf("five places the page shows have the word, whatever its case; the find says %q", got)
	}
	card, host := s.box("find"), s.box("web")
	if card.X+card.W > host.X+host.W || host.X+host.W-card.X-card.W > 40 || card.Y < host.Y || card.Y-host.Y > 40 {
		t.Fatalf("the card %v is not at the top right of the page %v", card, host)
	}
	all, current := s.findMarks()
	if len(all) != 4 || len(current) != 1 || current[0] != all[0] {
		t.Fatalf("the window shows four of them, the first the current one: %v, %v", all, current)
	}
	for i, want := range []string{"", "moved", "travel", "terms"} {
		if got := s.under(page, all[i]); got != want {
			t.Errorf("the mark %d is over %q, want %q", i, got, want)
		}
	}
	if word := all[1]; word.W < 30 || word.W > 60 || word.H < 14 || word.H > 30 {
		t.Errorf("the mark %v is not the size of a word", word)
	}
	if !page.Selection().Empty() || markup(page) != before {
		t.Fatal("finding changed the page's selection or its elements")
	}
	s.clean()

	s.refinds("the  ticket BACK")
	if got := s.stands(); got != "1 of 1" {
		t.Fatalf("words across an element, their white space single, are found %q", got)
	}
	if all, _ = s.findMarks(); len(all) < 2 {
		t.Fatalf("a place across elements is marked in each: %v", all)
	}
	s.refinds("refund")
	all, _ = s.findMarks()
	if got := s.stands(); got != "Not on this page" || len(all) != 0 || s.doc.ByID("find-robot") != nil {
		t.Fatalf("words the page has not: %q, marks %v", got, all)
	}
	if !s.disabled("find-next") || !s.disabled("find-previous") {
		t.Fatal("with nothing found there is a next place")
	}
	s.clean()
}

func TestEnterGoesToTheNextPlaceShiftEnterToThePreviousAndThePlaceComesIntoView(t *testing.T) {
	srv := crossings(t)
	s := kept(t)
	page := s.visit(srv.URL + "/")
	s.finds("ticket")
	s.press(input.KeyEnter)
	if got := s.stands(); got != "2 of 5" {
		t.Fatalf("after Enter the find says %q", got)
	}
	if _, current := s.findMarks(); len(current) != 1 || s.under(page, current[0]) != "moved" {
		t.Fatalf("the current mark is %v", current)
	}
	s.shifted(input.KeyF3)
	s.shifted(input.KeyF3)
	if got := s.stands(); got != "5 of 5" {
		t.Fatalf("Shift and F3 from the first place go round to the last: %q", got)
	}
	s.frame()
	host := s.box("web")
	_, current := s.findMarks()
	if len(current) != 1 || current[0].Y < host.Y || current[0].Y+current[0].H > host.Y+host.H ||
		s.under(page, current[0]) != "far" {
		t.Fatalf("the last place, far under the window, is not in view: %v in %v", current, host)
	}
	s.command(input.KeyG)
	if got := s.stands(); got != "1 of 5" {
		t.Fatalf("Next from the last place goes round to the first: %q", got)
	}
	s.frame()
	if _, current = s.findMarks(); len(current) != 1 || s.under(page, current[0]) != "" {
		t.Fatalf("back at the first place the current mark is %v", current)
	}
	s.click("find-previous")
	s.click("find-previous")
	s.press(input.KeyF3)
	s.click("find-previous")
	if got := s.stands(); got != "4 of 5" {
		t.Fatalf("Previous twice, F3 and Previous: %q", got)
	}
	s.clean()

	s.frame()
	_, current = s.findMarks()
	if len(current) != 1 || s.under(page, current[0]) != "terms" {
		t.Fatalf("the fourth place is %v", current)
	}
	s.clickAt(current[0].Center())
	s.loaded(srv.URL + "/terms")
	if s.findShown() {
		t.Fatal("the find outlived its page")
	}
}

func TestEscapeClosesTheFindAndClearsIt(t *testing.T) {
	srv := crossings(t)
	s := kept(t)
	page := s.visit(srv.URL + "/")
	s.finds("ticket")
	s.press(input.KeyEscape)
	all, _ := s.findMarks()
	if s.findShown() || len(all) != 0 || !page.Selection().Empty() {
		t.Fatalf("after Escape the find is open %v, the marks %v", s.findShown(), all)
	}
	s.types("x")
	s.command(input.KeyF)
	if got := s.doc.Value(s.find.query); got != "" || s.stands() != "Type the words to find" {
		t.Fatalf("opened again the find holds %q and says %q", got, s.stands())
	}
	s.types("fare")
	s.click("find-close")
	if all, _ = s.findMarks(); s.findShown() || len(all) != 0 {
		t.Fatalf("after Close the find is open %v, the marks %v", s.findShown(), all)
	}
	s.pick("menu-find")
	if !s.findShown() || !s.doc.Focused(s.find.query) {
		t.Fatal("the menu's row does not open the find")
	}
	s.clean()
}

func TestTheFindFollowsThePageAsItScrollsZoomsAndInTheDark(t *testing.T) {
	srv := crossings(t)
	s := kept(t)
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	page := s.visit(srv.URL + "/")
	s.finds("fare")
	_, before := s.findMarks()
	if len(before) != 1 || s.under(page, before[0]) != "cancelled" {
		t.Fatalf("the place is marked at %v", before)
	}
	s.clean()
	s.in.SetMouse(s.inPage("moved"))
	s.in.AddWheel(geom.V2(0, 2))
	for range glide {
		s.frame()
	}
	_, after := s.findMarks()
	if len(after) != 1 || after[0].Y >= before[0].Y || s.under(page, after[0]) != "cancelled" {
		t.Fatalf("once the page scrolled the mark went from %v to %v", before, after)
	}
	s.zoom(2)
	s.frame()
	s.frame()
	s.frame()
	_, zoomed := s.findMarks()
	if len(zoomed) != 1 || zoomed[0].W <= after[0].W || s.under(page, zoomed[0]) != "cancelled" {
		t.Fatalf("once the page zoomed the mark went from %v to %v", after, zoomed)
	}
	s.clean()
}

func TestOnlyAPageOfTheWebIsSearched(t *testing.T) {
	s := kept(t)
	s.Open(tabs.Page(guideName))
	s.frame()
	s.command(input.KeyF)
	if s.findShown() {
		t.Fatal("the keys opened the find over a page of light's own")
	}
	s.click("menu")
	if off, _ := s.node("menu-find").Attr("aria-disabled"); off != "true" {
		t.Fatal("the menu's row is not off over a page of light's own")
	}
	if !slices.Contains(s.rows(), findName+" "+keysShown(findKeys, runtime.GOOS)+" (off)") {
		t.Fatalf("the menu lists %v", s.rows())
	}
}

func TestFoldKeepsWhereEachLetterWas(t *testing.T) {
	folded, at := fold("  The\n\tÉTÉ  is ")
	if folded != " the été is " {
		t.Fatalf("folded to %q", folded)
	}
	if want := []int{0, 2, 3, 4, 5, 7, 7, 9, 10, 10, 12, 14, 15, 16, 17}; !slices.Equal(at, want) {
		t.Fatalf("its bytes were at %v, want %v", at, want)
	}
}
