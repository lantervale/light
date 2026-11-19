package app //nolint:testpackage // the screen is driven by what it holds

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/localnet"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

// heldSite serves pages a test holds: /first at once, with a link to
// /quick, and /slow, /again and /frame only once open is closed or
// sent to; left counts the held requests their asker gave up.
type heldSite struct {
	*httptest.Server
	open chan struct{}
	sync.Mutex
	asked map[string]int
	left  int
}

func heldServer(t *testing.T) *heldSite {
	t.Helper()
	site := &heldSite{open: make(chan struct{}), asked: map[string]int{}}
	page := func(title, colour, body string) string {
		return fmt.Sprintf(`<title>%s</title><body style="margin:0;background:%s"><h1 id="name">%s</h1>%s`+
			`<div id="tall" style="height:3000px"></div><p id="foot">foot</p>`, title, colour, title, body)
	}
	site.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site.Lock()
		site.asked[r.URL.Path]++
		nth := site.asked[r.URL.Path]
		site.Unlock()
		held := r.URL.Path == "/slow" || r.URL.Path == "/frame" || (r.URL.Path == "/again" && nth > 1)
		if held {
			select {
			case <-site.open:
			case <-r.Context().Done():
				site.Lock()
				site.left++
				site.Unlock()
				return
			}
		}
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/first":
			_, _ = w.Write([]byte(page("First", "#ffeecc", `<a id="quick" href="/quick">quick</a>`)))
		case "/again":
			_, _ = w.Write([]byte(page(fmt.Sprintf("Again %d", nth), "#ffffff", "")))
		case "/framed":
			_, _ = w.Write([]byte(page("Framed", "#ffffff", `<iframe id="inner" name="inner" src="/inside"></iframe>`+
				`<a id="aim" href="/frame" target="inner">aim</a>`)))
		case "/inside":
			_, _ = w.Write([]byte(`<p id="inside">inside</p>`))
		case "/frame":
			_, _ = w.Write([]byte(`<p id="arrived">arrived</p>`))
		default:
			_, _ = w.Write([]byte(page(strings.TrimPrefix(r.URL.Path, "/"), "#102030", "")))
		}
	}))
	t.Cleanup(func() {
		site.release()
		site.Close()
	})
	return site
}

func (site *heldSite) release() {
	select {
	case <-site.open:
	default:
		close(site.open)
	}
}

func (site *heldSite) gaveUp() int {
	site.Lock()
	defer site.Unlock()
	return site.left
}

// drawn reports whether the page area shows something: a page of
// light's with elements, or a page of the web whose Document has some.
func (s *shown) drawn() bool {
	if s.doc.ByID("loading") != nil || len(s.view.Children()) != 1 {
		return false
	}
	page := s.view.Children()[0]
	if s.host == nil {
		return len(page.Children()) > 0
	}
	doc := s.doc.Frame(s.host)
	return doc != nil && len(doc.Body().Children()) > 0
}

// showing is the name the shown page of the web gives itself.
func (s *shown) showing() string {
	if s.host == nil || s.doc.Frame(s.host) == nil || s.doc.Frame(s.host).ByID("name") == nil {
		return ""
	}
	return s.doc.Frame(s.host).ByID("name").Text()
}

func (s *shown) lockIcon() string {
	src, _ := s.lock.Attr("src")
	return src
}

func (s *shown) tabName() string { return s.strip[s.browser.Active].name.Text() }

// committed runs frames until the shown tab stands on address, and
// fails if a frame on the way shows an empty page area.
func (s *shown) committed(address string) {
	s.t.Helper()
	for range 600 {
		s.frame()
		if !s.drawn() {
			s.t.Fatalf("a frame of the way to %s shows an empty page", address)
		}
		if s.browser.Tab().Current() == address && s.session().pending == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	s.t.Fatalf("%s never took the tab", address)
}

func TestThePageShownStaysUntilTheNextOneIsReady(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	first, slow := site.URL+"/first", site.URL+"/slow"
	old := s.visit(first)
	s.enter(slow)
	for range 20 {
		s.frame()
		if !s.drawn() || s.showing() != "First" || s.doc.Frame(s.host) != old {
			t.Fatalf("the page area shows %q while the next page loads, want the page it showed", s.showing())
		}
	}
	tab := s.browser.Tab()
	if tab.Current() != first || tab.Len() != 2 || tab.CanForward() || s.tabName() != "First" {
		t.Errorf("the tab stands on %s, named %q, before the next page is ready", tab.Current(), s.tabName())
	}
	if s.Profile.visits.Len() != 1 {
		t.Errorf("the history has %v before the next page is ready", s.Profile.visits.Len())
	}
	if !s.loading() || s.session().pending == nil || !s.strip[0].tab.HasClass("loading") {
		t.Error("nothing says the tab loads")
	}
	if label, _ := s.reload.Attr("aria-label"); label != "Stop" {
		t.Errorf("the reload button is %q while the next page loads", label)
	}
	if !s.doc.ByID("loadline").HasClass("shown") || !strings.Contains(s.status.Text(), "Waiting for") {
		t.Errorf("no progress line, or the bubble says %q", s.status.Text())
	}
	s.at(slow)
	if !s.field.HasClass("pending") || s.lockIcon() != "open.svg" {
		t.Errorf("the address bar shows the next address as the one of the page: icon %s", s.lockIcon())
	}
	foot := old.ByID("foot")
	old.ScrollIntoView(foot)
	s.frame()
	s.frame()
	if r, ok := old.BoxOf(foot); !ok || r.Y > s.size.Y {
		t.Errorf("the page held does not scroll: its foot is at %v", r)
	}
	site.release()
	s.committed(slow)
	s.frame()
	if s.showing() != "slow" || s.tabName() != "slow" || s.loading() || s.field.HasClass("pending") {
		t.Errorf("once ready the tab shows %q, named %q", s.showing(), s.tabName())
	}
	if tab.Len() != 3 || s.Profile.visits.Len() != 2 || s.doc.ByID("loadline").HasClass("shown") ||
		s.strip[0].tab.HasClass("loading") {
		t.Errorf("once ready: back %v, history %v", tab.CanBack(), s.Profile.visits.Len())
	}
	if bg, _ := s.doc.Frame(s.host).ComputedValue(s.doc.Frame(s.host).Body(), "background-color"); bg != "#102030" {
		t.Errorf("the page comes with the background %s", bg)
	}
}

func TestAClickOnThePageHeldStartsAnotherMoveAndEndsTheFirst(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	old := s.visit(site.URL + "/first")
	s.Open(site.URL + "/slow")
	s.frame()
	s.clickAt(s.inPage("quick"))
	s.committed(site.URL + "/quick")
	if s.showing() != "quick" || old == s.doc.Frame(s.host) {
		t.Errorf("the tab shows %q", s.showing())
	}
	s.until("the first move is given up", func() bool { return site.gaveUp() == 1 })
	if len(s.session().loads) != 0 {
		t.Errorf("%d loads are left", len(s.session().loads))
	}
	s.at(strings.TrimPrefix(site.URL+"/quick", "https://"))
}

func TestStoppingAMoveStaysOnThePageShown(t *testing.T) {
	site := heldServer(t)
	first := site.URL + "/first"
	for _, how := range []string{"the Stop button", "Escape"} {
		s := show(t)
		s.visit(first)
		routines := runtime.NumGoroutine()
		s.Open(site.URL + "/slow")
		s.frame()
		if how == "Escape" {
			s.press(input.KeyEscape)
		} else {
			s.click("reload")
		}
		s.frame()
		if s.loading() || s.session().pending != nil || len(s.session().loads) != 0 {
			t.Fatalf("%s: the tab still loads", how)
		}
		if s.showing() != "First" || s.browser.Tab().Current() != first || s.browser.Tab().Len() != 2 {
			t.Errorf("%s: the tab shows %q at %s", how, s.showing(), s.browser.Tab().Current())
		}
		s.at(first)
		if s.field.HasClass("pending") || s.doc.ByID("loadline").HasClass("shown") || s.view.HasClass("waiting") {
			t.Errorf("%s: the window still says a page is on its way", how)
		}
		s.until("the goroutines of the load end", func() bool { return runtime.NumGoroutine() <= routines+2 })
	}
}

func TestAMoveThatFailsShowsWhyAtItsAddress(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	s.visit(site.URL + "/first")
	gone := "http://127.0.0.1:1/nothing"
	s.Open(gone)
	s.committed(gone)
	if s.doc.ByID("page") == nil || !s.doc.ByID("page").HasClass("unreachable") || s.host != nil {
		t.Fatal("no page says why")
	}
	s.at(gone)
	if !s.browser.Tab().CanBack() || s.field.HasClass("pending") || s.loading() {
		t.Error("the failed move is not a step of the tab, or still loads")
	}
}

func TestAMoveAPageAskedForKeepsTheAddressOfThePageShown(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	first, slow := site.URL+"/first", site.URL+"/slow"
	s.visit(first)
	s.linked(slow)
	s.frame()
	s.at(first)
	if s.field.HasClass("pending") || !s.loading() || s.showing() != "First" {
		t.Errorf("a page's own move shows in the address bar before its page: %q", s.doc.Value(s.address))
	}
	site.release()
	s.committed(slow)
	s.at(slow)
}

func TestReloadAndTheHistoryHoldThePageShown(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	again := site.URL + "/again"
	s.visit(again)
	if s.showing() != "Again 1" {
		t.Fatalf("the tab shows %q", s.showing())
	}
	s.reloadOrStop()
	for range 10 {
		s.frame()
		if s.showing() != "Again 1" || !s.loading() {
			t.Fatalf("a reload shows %q while it loads", s.showing())
		}
	}
	site.release()
	s.until("the reload lands", func() bool { return s.showing() == "Again 2" && !s.loading() })
	if s.browser.Tab().Len() != 2 {
		t.Error("a reload is a step of the tab")
	}
	s.visit(site.URL + "/quick")
	delete(s.session().pages, again)
	s.move((*tabs.Tab).Back)
	for range 5 {
		s.frame()
		if !s.drawn() {
			t.Fatal("a step back shows an empty page")
		}
	}
	s.committed(again)
	if !s.browser.Tab().CanForward() || s.browser.Tab().Len() != 3 || s.showing() != "Again 3" {
		t.Errorf("back left the tab on %q", s.showing())
	}
}

func TestAPageLongOnItsWayDimsThePageHeld(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	s.dimAfter = 30 * time.Millisecond
	s.visit(site.URL + "/first")
	s.Open(site.URL + "/slow")
	s.frame()
	if s.view.HasClass("waiting") {
		t.Fatal("the page held is dimmed at once")
	}
	s.until("the page held is dimmed", func() bool { return s.view.HasClass("waiting") })
	if !s.drawn() || s.showing() != "First" {
		t.Error("the page held is gone")
	}
	s.clickAt(s.inPage("quick"))
	s.committed(site.URL + "/quick")
	if s.view.HasClass("waiting") {
		t.Error("the page that came is dimmed")
	}
}

func TestATabWithNoPageWaitsOnLightsOwnSurface(t *testing.T) {
	site := heldServer(t)
	for _, dark := range []bool{false, true} {
		s := show(t)
		s.configure(func(c *config.Config) { c.Appearance = appearance(dark) })
		s.OpenInNewTab(site.URL + "/slow")
		s.frame()
		if s.doc.ByID("loading") == nil || s.dark() != dark {
			t.Fatalf("dark %v: the new tab waits on something else than light's surface", dark)
		}
		want := map[bool]string{false: "#ffffff", true: "#000000"}[dark]
		if bg, _ := s.doc.ComputedValue(s.view, "background-color"); bg != want {
			t.Errorf("dark %v: the surface is %s", dark, bg)
		}
		if !s.strip[1].tab.HasClass("loading") || !s.doc.ByID("loadline").HasClass("shown") {
			t.Errorf("dark %v: nothing says the tab loads", dark)
		}
		site.release()
		s.until("the page comes", func() bool { return s.showing() == "slow" })
		if bg, _ := s.doc.Frame(s.host).ComputedValue(s.doc.Frame(s.host).Body(), "background-color"); bg != "#102030" {
			t.Errorf("dark %v: the page comes with the background %s", dark, bg)
		}
	}
}

func TestALoadKeepsWhenEachOfItsStepsEnded(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	s.visit(site.URL + "/first")
	s.frame()
	s.frame()
	timing := s.session().pages[site.URL+"/first"].Timing
	order := []web.Mark{web.Started, web.Asked, web.Sent, web.Answered, web.Received, web.Parsed, web.Gathered,
		web.Built, web.Shown, web.Drawn}
	for i, mark := range order {
		if timing.At(mark).IsZero() {
			t.Fatalf("%s is not marked", mark)
		}
		if i > 0 && timing.At(mark).Before(timing.At(order[i-1])) {
			t.Errorf("%s is marked before %s", mark, order[i-1])
		}
	}
	if line := s.timingLine(site.URL + "/first"); !strings.Contains(line, "drawn ") || !strings.Contains(line, "/first") {
		t.Errorf("the timing reads %q", line)
	}
}

func TestAFrameKeepsItsDocumentUntilTheNextOneIsReady(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	doc := s.visit(site.URL + "/framed")
	inner := doc.ByID("inner")
	s.until("the frame shows its document", func() bool {
		return doc.Frame(inner) != nil && doc.Frame(inner).ByID("inside") != nil
	})
	s.clickAt(s.inPage("aim"))
	for range 10 {
		s.frame()
		if doc.Frame(inner) == nil || doc.Frame(inner).ByID("inside") == nil {
			t.Fatal("the frame is empty while its next document loads")
		}
	}
	site.release()
	s.until("the frame shows the next document", func() bool { return doc.Frame(inner).ByID("arrived") != nil })
}

func TestTheNetworkPanelSaysWhenEachStepOfThePagesLoadEnded(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	s.visit(site.URL + "/first")
	s.frame()
	steps := s.timingOf(site.URL + "/first")
	if len(steps) < 9 || steps[0][0] != "asked" || steps[len(steps)-1][0] != "drawn" {
		t.Fatalf("the steps are %v", steps)
	}
	if s.timingOf(site.URL+"/other") != nil {
		t.Error("an address the tab has no page of has a timing")
	}
	s.toggleTools()
	s.frame()
	for _, row := range s.session().log.Rows() {
		if row.Address == site.URL+"/first" {
			s.click(fmt.Sprintf("dt-row-%d", row.ID))
		}
	}
	if s.doc.ByID("dt-timing") == nil || !strings.Contains(s.doc.ByID("dt-detail").Text(), "built") {
		t.Errorf("the details of the page's request read %q", s.doc.ByID("dt-detail").Text())
	}
	s.visit(site.URL + "/quick")
	var again netlog.Row
	s.until("a request of the page went through a connection used before", func() bool {
		for _, row := range s.session().log.Rows() {
			if row.Reused && row.Done && row.Initiator != "navigation" {
				again = row
			}
		}
		return again.ID != 0
	})
	s.click(fmt.Sprintf("dt-row-%d", again.ID))
	detail := s.doc.ByID("dt-detail").Text()
	for _, want := range []string{"Connection", "used again", "HTTP/1.1", "Waiting", "Receiving", "ms after the page"} {
		if s.doc.ByID("dt-connection") == nil || !strings.Contains(detail, want) {
			t.Errorf("the details of a request on a connection used again say nothing of %q: %q", want, detail)
		}
	}
}

func TestTheLineFillsAsTheLoadGoesAndNeverMovesThePage(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	s.visit(site.URL + "/first")
	before, _ := s.doc.BoxOf(s.view)
	s.Open(site.URL + "/slow")
	s.frame()
	first := s.lineAt
	for range 30 {
		s.frame()
	}
	if s.lineAt <= first || s.lineAt >= filled(web.Sent+1) {
		t.Errorf("the line went from %v to %v while the request waits for its answer", first, s.lineAt)
	}
	if during, _ := s.doc.BoxOf(s.view); during != before {
		t.Errorf("the line moved the page from %v to %v", before, during)
	}
	if line, _ := s.doc.BoxOf(s.line); line.Y != before.Y || line.H != 3 {
		t.Errorf("the line is at %v", line)
	}
	if src, _ := s.strip[0].icon.Attr("src"); src != spinner || s.strip[0].icon.Style() == "" {
		t.Errorf("the tab shows %q while it loads", src)
	}
	site.release()
	s.committed(site.URL + "/slow")
	s.frame()
	if src, _ := s.strip[0].icon.Attr("src"); src == spinner || s.strip[0].icon.Style() != "" || s.lineAt != 0 {
		t.Errorf("once ready the tab shows %q", src)
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		t.Errorf("CSS warnings: %v", warns)
	}
}

func TestEachRequestOfAPageIsToldWithItsConnectionAndItsSteps(t *testing.T) {
	site := heldServer(t)
	s := show(t)
	told := func(address string) string {
		s.visit(address)
		s.frame()
		for _, row := range s.session().log.Rows() {
			if row.Address == address {
				return requestLine(row, s.session().pages[address].Timing)
			}
		}
		return ""
	}
	first, quick := told(site.URL+"/first"), told(site.URL+"/quick")
	for _, want := range []string{"new", "HTTP/1.1", "connect ", "wait ", "receive ", "document by navigation"} {
		if !strings.Contains(first, want) {
			t.Errorf("the first request is told as %q, without %q", first, want)
		}
	}
	for _, want := range []string{"used again", "before parsed"} {
		if !strings.Contains(quick, want) || strings.Contains(quick, "connect ") {
			t.Errorf("the request of the next page is told as %q, without %q or with a connect", quick, want)
		}
	}
}

func TestWhatAPageAsksForComesThroughTheConnectionThePageCameThrough(t *testing.T) {
	l := newLAN(t)
	for _, space := range []localnet.Space{localnet.Public, localnet.Private, localnet.Loopback} {
		site := l.serve(space)
		l.show(site+"/", `<title>Site</title><link rel="stylesheet" href="/look.css"><p>text</p>`)
		s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})
		s.visit(site + "/")
		// The sheet and the site's icon are asked for together: one of
		// them takes the page's connection, the other opens its own.
		var page netlog.Row
		again := 0
		s.until("the page's sheet and its icon were answered", func() bool {
			rows := s.session().log.Rows()
			return len(rows) == 3 && !slices.ContainsFunc(rows, func(r netlog.Row) bool { return !r.Done })
		})
		for _, row := range s.session().log.Rows() {
			switch {
			case row.Address == site+"/":
				page = row
			case row.Reused:
				again++
			}
		}
		if page.Reused || again == 0 {
			t.Errorf("a page of the %s network: its connection was used before (%t), and %d of what it asked for used it",
				space, page.Reused, again)
		}
	}
}
