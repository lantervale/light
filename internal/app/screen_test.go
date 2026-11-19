package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui/align"
	"github.com/lantervale/ember/world"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

type shown struct {
	*Screen
	t     *testing.T
	win   *platform.Window
	in    *input.State
	frame func()
	world *world.World
}

// show is a window showing light with no network, its history in memory
// and its downloads in a folder of the test's.
func show(t *testing.T) *shown {
	t.Helper()
	return showWith(t, Options{Downloads: t.TempDir()})
}

func showWith(t *testing.T, opts Options) *shown {
	t.Helper()
	win := Window(nil)
	screen := New(win, opts)
	win.Add(screen.Document())
	w := ember.New().AddEntities(win).AddSystems(screen)
	t.Cleanup(func() { _ = screen.Close() })
	s := &shown{Screen: screen, t: t, win: win, in: win.Input(), frame: func() { w.RunFrames(1) }, world: w}
	s.frame()
	return s
}

func (s *shown) click(id string) {
	s.t.Helper()
	n := s.doc.ByID(id)
	if n == nil {
		s.t.Fatalf("no element %q", id)
	}
	s.doc.ScrollIntoView(n)
	s.frame()
	r, _ := s.doc.BoxOf(n)
	s.in.SetMouse(r.Center())
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

func (s *shown) enter(address string) {
	s.t.Helper()
	s.click("address")
	s.doc.SetValue(s.address, address)
	s.in.SetKey(input.KeyEnter, true)
	s.frame()
	s.in.SetKey(input.KeyEnter, false)
	s.frame()
	s.frame()
}

func (s *shown) at(address string) {
	s.t.Helper()
	if got := s.doc.Value(s.address); got != address {
		s.t.Fatalf("the address bar shows %q, want %q", got, address)
	}
}

func (s *shown) disabled(id string) bool {
	_, off := s.doc.ByID(id).Attr("disabled")
	return off
}

func TestEveryPageIsClean(t *testing.T) {
	s := show(t)
	addresses := []string{tabs.Search("tabs"), "ftp://example.test", tabs.SourceOf(tabs.Page(aboutName))}
	for _, page := range pages() {
		addresses = append(addresses, tabs.Page(page.name))
	}
	for _, address := range addresses {
		s.Open(address)
		s.frame()
		if faults := align.Check(s.doc); len(faults) > 0 {
			t.Errorf("%s: layout faults: %v", address, faults)
		}
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		t.Fatalf("CSS warnings: %v", warns)
	}
}

func TestTheAboutPageListsEveryAdaptation(t *testing.T) {
	s := show(t)
	s.Open("light://about")
	s.frame()
	rows := s.doc.ByID("adaptations").Children()
	if len(rows) != len(web.Adaptations()) || len(rows) == 0 {
		t.Fatalf("%d rows for %d adaptations", len(rows), len(web.Adaptations()))
	}
	for i, a := range web.Adaptations() {
		if got := rows[i].Text(); got != a.Name+": "+a.What {
			t.Errorf("row %d reads %q, want the adaptation %q", i, got, a.Name)
		}
	}
}

func TestATileOpensItsPageAndBackReturns(t *testing.T) {
	s := show(t)
	if !s.disabled("back") || !s.disabled("forward") {
		t.Fatal("a new tab can already go back or forward")
	}
	s.click("go-guide")
	s.at("light://guide")
	if s.disabled("back") {
		t.Fatal("back stays disabled after a visit")
	}
	s.click("back")
	s.at(tabs.Home)
	s.click("forward")
	s.at("light://guide")
}

func TestATypedWordSearchesThePages(t *testing.T) {
	s := show(t)
	s.configure(func(c *config.Config) { c.SearchEngine = search.Own })
	s.enter("tile maps")
	s.at("light://search?q=tile+maps")
	if text := s.doc.ByID("page").Text(); !strings.Contains(text, "Examples") || s.doc.ByID("none") != nil {
		t.Fatalf("the results are %q", text)
	}
}

func TestATypedHostNameIsAnHTTPSAddress(t *testing.T) {
	s := show(t)
	s.session().pages["https://example.test"] = &web.Page{Err: errors.New("no such host")}
	s.enter("example.test")
	s.at("https://example.test")
	if text := s.doc.ByID("page").Text(); !strings.Contains(text, "no such host") {
		t.Fatalf("the page is %q", text)
	}
}

func TestAnAddressOfAnotherSchemeHasNoPage(t *testing.T) {
	s := show(t)
	s.enter("ftp://example.test")
	if text := s.doc.ByID("page").Text(); !strings.Contains(text, "can't be reached") {
		t.Fatalf("the page is %q", text)
	}
}

func TestTabsKeepTheirOwnPage(t *testing.T) {
	s := show(t)
	s.click("go-about")
	s.click("new-tab")
	s.at(tabs.Home)
	s.click("tab-0")
	s.at("light://about")
	s.click("close-0")
	s.at(tabs.Home)
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("%d tabs after closing one of two", len(s.browser.Tabs))
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTheStartPagesDemoTileOpensCSSZenGarden(t *testing.T) {
	s := showWith(t, Options{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no network in tests")
	})})
	s.click("go-demo")
	s.until("the demo's answer landed", func() bool { return !s.loading() })
	if got := s.browser.Tab().Current(); got != "https://csszengarden.com" {
		t.Fatalf("the demo tile opened %q", got)
	}
}
