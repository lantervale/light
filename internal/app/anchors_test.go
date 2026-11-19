package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
)

const glossaryLook = `<style>body { margin: 0 } a, h2, p { display: block; height: 20px; margin: 0 }
	.gap { height: 1500px }</style>`

// glossary is a small site of long pages with places in them: a page
// whose links lead to its own places and to one of another page, and a
// page that frames the first.
func glossary(t *testing.T) (*httptest.Server, *asked) {
	t.Helper()
	return answers(t, map[string]answer{
		"/": typed("text/html", `<title>Glossary</title>`+glossaryLook+`
			<a id="to-low" href="#low">low</a><a id="to-named" href="#named">named</a>
			<a id="to-spaced" href="#two%20words">spaced</a><a id="to-missing" href="#missing">missing</a>
			<a id="to-other" href="/other#deep">other</a>
			<div class="gap"></div>
			<h2 id="low">Low</h2>
			<a id="up" href="#top">up</a><a id="bare" href="#">bare</a><a id="lower" href="#named">lower</a>
			<div class="gap"></div><a id="by-name" name="named">Named</a>
			<div class="gap"></div><p id="two words">Two words</p><div class="gap"></div>`),
		"/other": typed("text/html", `<title>Other</title>`+glossaryLook+`<p id="first">First</p>
			<div class="gap"></div><p id="deep">Deep</p><div class="gap"></div>`),
		"/framing": typed("text/html", `<title>Framing</title><body style="margin:0">
			<iframe id="inner" src="/" width="300" height="200"></iframe>
			<div style="height:1500px"></div><p id="below">below</p>`),
	})
}

// inView reports whether the element id of doc, shown in a view that
// high, is in it.
func inView(t *testing.T, doc *ui.Document, id string, height float32) bool {
	t.Helper()
	r := box(t, doc, id)
	return r.Y >= 0 && r.Y+r.H <= height
}

// shows reports, a few frames on, whether the shown page of the web has
// its element id in view.
func (s *shown) shows(id string) bool {
	s.t.Helper()
	for range 4 {
		s.frame()
	}
	page := s.session().pages[s.browser.Tab().Current()]
	if page == nil || page.Doc == nil || s.host == nil {
		s.t.Fatal("no page of the web is shown")
	}
	host, _ := s.doc.BoxOf(s.host)
	return inView(s.t, page.Doc, id, host.H)
}

// topmost reports, a few frames on, whether the shown page of the web has
// its element id at the top of its view.
func (s *shown) topmost(id string) bool {
	s.t.Helper()
	s.shows(id)
	y := box(s.t, s.session().pages[s.browser.Tab().Current()].Doc, id).Y
	return y > -0.5 && y < 0.5
}

// scrolled is how far down the shown page of the web is scrolled, a few
// frames on, and scroll scrolls it there.
func (s *shown) scrolled() float32 {
	s.t.Helper()
	for range 4 {
		s.frame()
	}
	doc := s.session().pages[s.browser.Tab().Current()].Doc
	return doc.Scroll(doc.HTML()).Y
}

func (s *shown) scroll(y float32) {
	s.t.Helper()
	doc := s.session().pages[s.browser.Tab().Current()].Doc
	doc.SetScroll(doc.HTML(), geom.V2(0, y))
	s.frame()
}

func TestAnAddressWithAFragmentShowsTheElementItNames(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	if s.visit(srv.URL+"/#low") == nil {
		t.Fatal("the page did not load")
	}
	s.at(srv.URL + "/#low")
	if !s.topmost("low") || s.shows("to-low") {
		t.Fatalf("opened at #low the page has it at its top %v, and shows its own %v", s.topmost("low"),
			s.shows("to-low"))
	}
	if got := seen.count("/"); got != 1 {
		t.Fatalf("the page was asked for %d times", got)
	}
}

func TestAFragmentLinkScrollsThePageWithoutFetchingIt(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/")
	if s.shows("low") {
		t.Fatal("the place is in view before its link is clicked")
	}
	s.in.SetMouse(s.inPage("to-low"))
	s.frame()
	s.frame()
	if got := s.status.Text(); got != srv.URL+"/#low" {
		t.Fatalf("under the pointer the link names %q to open", got)
	}
	s.clickAt(s.inPage("to-low"))
	s.at(srv.URL + "/#low")
	if !s.topmost("low") {
		t.Fatal("the link did not bring its place to the top of the view")
	}
	if got := seen.count("/"); got != 1 {
		t.Fatalf("the page was asked for %d times", got)
	}
	if s.disabled("back") || s.browser.Tab().Len() != 3 {
		t.Fatalf("back is disabled %v after the link, the history has %d steps", s.disabled("back"),
			s.browser.Tab().Len())
	}
}

func TestBackAndForwardCrossThePlacesOfAPageWithoutFetchingIt(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("to-low"))
	if !s.shows("low") {
		t.Fatal("the link did not scroll to its place")
	}
	s.clickAt(s.inPage("lower"))
	s.at(srv.URL + "/#named")
	if !s.shows("by-name") {
		t.Fatal("the second link did not scroll to its place")
	}
	s.click("back")
	s.at(srv.URL + "/#low")
	if !s.shows("low") {
		t.Fatal("back did not return to the place before")
	}
	s.click("back")
	s.at(srv.URL + "/")
	if !s.shows("to-low") {
		t.Fatal("back did not return to the top of the page")
	}
	s.click("forward")
	s.at(srv.URL + "/#low")
	if !s.shows("low") || s.disabled("forward") {
		t.Fatalf("forward shows the place %v, forward disabled %v", s.shows("low"), s.disabled("forward"))
	}
	if got := seen.count("/"); got != 1 {
		t.Fatalf("the page was asked for %d times", got)
	}
}

func TestBackAndForwardReturnToWhereAPageWasLeft(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.scroll(60)
	s.Open(srv.URL + "/#low")
	if !s.topmost("low") {
		t.Fatal("the address did not scroll to its place")
	}
	low := s.scrolled()
	s.scroll(low + 200)
	s.Open(srv.URL + "/#named")
	if !s.topmost("by-name") {
		t.Fatal("the second address did not scroll to its place")
	}
	s.click("back")
	s.at(srv.URL + "/#low")
	if got := s.scrolled(); got != low+200 {
		t.Fatalf("back at #low the page is scrolled %v, want the %v it was left at", got, low+200)
	}
	s.click("back")
	s.at(srv.URL + "/")
	if got := s.scrolled(); got != 60 {
		t.Fatalf("back at its first address the page is scrolled %v, want the 60 it was left at", got)
	}
	s.click("forward")
	s.at(srv.URL + "/#low")
	if got := s.scrolled(); got != low+200 {
		t.Fatalf("forward at #low the page is scrolled %v, want %v", got, low+200)
	}
	s.visit(srv.URL + "/other")
	s.scroll(700)
	s.click("back")
	s.until("the first page showed again", func() bool { return s.session().pending == nil })
	s.at(srv.URL + "/#low")
	if got := s.scrolled(); got != low+200 {
		t.Fatalf("back from another page the page is scrolled %v, want %v", got, low+200)
	}
	s.click("forward")
	s.until("the other page showed again", func() bool { return s.session().pending == nil })
	s.at(srv.URL + "/other")
	if got := s.scrolled(); got != 700 || seen.count("/") != 1 {
		t.Fatalf("forward the other page is scrolled %v, want 700; the first was asked for %d times", got,
			seen.count("/"))
	}
}

func TestANewStepAtAnAddressLeftBeforeGoesToItsPlace(t *testing.T) {
	srv, _ := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.Open(srv.URL + "/#low")
	s.scroll(s.scrolled() + 300)
	s.click("back")
	s.Open(srv.URL + "/#low")
	if !s.topmost("low") {
		t.Fatal("the address did not scroll to its place")
	}
	s.Open(srv.URL + "/#named")
	s.click("back")
	s.at(srv.URL + "/#low")
	if !s.topmost("low") {
		t.Fatal("a new step at an address was scrolled where an older one was left")
	}
}

func TestReloadKeepsWhereThePageIsScrolled(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/other#deep")
	if !s.topmost("deep") {
		t.Fatal("the page did not show the place its address names")
	}
	deep := s.scrolled()
	s.scroll(deep + 100)
	s.click("reload")
	s.until("the page was fetched again", func() bool {
		return seen.count("/other") == 2 && s.session().pending == nil
	})
	if got := s.scrolled(); got != deep+100 {
		t.Fatalf("fetched again the page is scrolled %v, want the %v it was at", got, deep+100)
	}
}

func TestAFragmentNamesTheTopAnAnchorOrNothing(t *testing.T) {
	srv, _ := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("to-missing"))
	s.at(srv.URL + "/#missing")
	if !s.shows("to-low") {
		t.Fatal("a fragment that names nothing scrolled the page")
	}
	s.Open(srv.URL + "/#low")
	if !s.shows("low") {
		t.Fatal("the address did not scroll to its place")
	}
	s.clickAt(s.inPage("up"))
	s.at(srv.URL + "/#top")
	if !s.shows("to-low") {
		t.Fatal("#top did not scroll to the top")
	}
	s.Open(srv.URL + "/#low")
	if !s.shows("low") {
		t.Fatal("the address did not scroll to its place again")
	}
	s.clickAt(s.inPage("bare"))
	s.at(srv.URL + "/#")
	if !s.shows("to-low") {
		t.Fatal("# did not scroll to the top")
	}
	s.clickAt(s.inPage("to-named"))
	if !s.shows("by-name") {
		t.Fatal("an anchor was not found by its name")
	}
	s.click("back")
	s.clickAt(s.inPage("to-spaced"))
	s.at(srv.URL + "/#two%20words")
	if !s.shows("two words") {
		t.Fatal("an escaped fragment did not find its element")
	}
}

func TestALinkToAPlaceOfAnotherPageLoadsItThere(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("to-other"))
	s.until("the other page showed", func() bool {
		return s.session().pages[srv.URL+"/other#deep"] != nil && s.session().pending == nil
	})
	s.at(srv.URL + "/other#deep")
	if !s.topmost("deep") {
		t.Fatal("the page did not show the place its address names")
	}
	s.click("reload")
	s.until("the page was fetched again", func() bool {
		return seen.count("/other") == 2 && s.session().pending == nil
	})
	s.at(srv.URL + "/other#deep")
	if !s.shows("deep") {
		t.Fatal("the page fetched again did not show the place its address names")
	}
}

func TestAFragmentLinkInAFrameScrollsThatFrameAlone(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	doc := s.visit(srv.URL + "/framing")
	s.framedText("inner")
	s.quiet()
	f, stood := s.frameAt("inner"), box(t, doc, "inner")
	if inView(t, f.page.Doc, "low", 200) {
		t.Fatal("the place is in view before its link is clicked")
	}
	s.clickAt(s.inFrame(f, "to-low"))
	for range 4 {
		s.frame()
	}
	if at := box(t, f.page.Doc, "low").Y; at < -0.5 || at > 0.5 {
		t.Fatalf("the link left its place %v from the top of its frame", at)
	}
	if now := box(t, doc, "inner"); now != stood || s.browser.Tab().Current() != srv.URL+"/framing" {
		t.Fatalf("the page moved with its frame: the frame is at %v, the tab at %s", now, s.browser.Tab().Current())
	}
	if got := seen.count("/"); got != 1 {
		t.Fatalf("the frame's document was asked for %d times", got)
	}
	if s.disabled("back") {
		t.Fatal("the place of a frame is no step of the history")
	}
	s.click("back")
	for range 4 {
		s.frame()
	}
	if got := seen.count("/"); got != 1 || !inView(t, f.page.Doc, "to-low", 200) {
		t.Fatalf("back asked for the frame's document %d times, its top shown %v", got,
			inView(t, f.page.Doc, "to-low", 200))
	}
}

func TestAFragmentLinkOpensInATabOfItsOwnByTheKeys(t *testing.T) {
	srv, seen := glossary(t)
	s := show(t)
	s.system = macOS
	s.visit(srv.URL + "/")
	s.clickHolding(s.inPage("to-low"), input.KeyLeftSuper)
	s.middleClickAt(s.inPage("to-named"))
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/#low "+srv.URL+"/#named"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.until("the tabs behind loaded", func() bool { return seen.count("/") == 3 })
	if s.shows("low") {
		t.Fatal("a link opened in another tab scrolled the page it is in")
	}
	s.click(tabID(1))
	s.at(srv.URL + "/#low")
	if !s.shows("low") {
		t.Fatal("the tab opened behind does not show the place its address names")
	}
}

func TestAZoomedPageScrollsToItsFragment(t *testing.T) {
	srv, _ := glossary(t)
	s := show(t)
	s.configure(func(c *config.Config) { c.Zoom = 200 })
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("to-low"))
	if !s.topmost("low") {
		t.Fatal("the link did not bring its place to the top of the zoomed page")
	}
}

func TestAFragmentIsNeverSentToAServer(t *testing.T) {
	var (
		mu   sync.Mutex
		sent []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sent = append(sent, r.RequestURI+" "+r.Referer())
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Kept</title><p id="secret">kept</p><a id="on" href="/next#second">on</a>
			<img src="/pixel.png#third" alt="">`))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	s.visit(srv.URL + "/#secret")
	s.clickAt(s.inPage("on"))
	s.until("the next page showed", func() bool {
		return s.session().pages[srv.URL+"/next#second"] != nil && s.session().pending == nil
	})
	mu.Lock()
	defer mu.Unlock()
	if all := strings.Join(sent, "\n"); len(sent) < 2 || strings.ContainsAny(all, "#") ||
		strings.Contains(all, "secret") || strings.Contains(all, "second") || strings.Contains(all, "third") {
		t.Fatalf("the server was sent %q", sent)
	}
}
