package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui/align"
)

type shown struct {
	*explorer
	t     *testing.T
	win   *platform.Window
	in    *input.State
	frame func()
}

func show(t *testing.T) *shown {
	t.Helper()
	win := platform.NewWindow(platform.Size(1024, 680))
	e := newExplorer(win)
	win.Add(e.doc)
	w := ember.New().AddEntities(win).AddSystems(e)
	s := &shown{explorer: e, t: t, win: win, in: win.Input(), frame: func() { w.RunFrames(1) }}
	s.frame()
	return s
}

func (s *shown) click(id string) {
	s.t.Helper()
	n := s.doc.ByID(id)
	if n == nil {
		s.t.Fatalf("no element %q", id)
	}
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
	for _, address := range []string{home, "ember://guide", "ember://examples", "ember://about",
		"ember://search?q=tabs", "ftp://example.test"} {
		s.open(address)
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
	s.open("ember://about")
	s.frame()
	rows := s.doc.ByID("list").Children()
	if len(rows) != len(adaptations) {
		t.Fatalf("%d rows for %d adaptations", len(rows), len(adaptations))
	}
	for i, a := range adaptations {
		if got := rows[i].Text(); got != a.name+": "+a.what {
			t.Errorf("row %d reads %q, want the adaptation %q", i, got, a.name)
		}
	}
}

func TestATileOpensItsPageAndBackReturns(t *testing.T) {
	s := show(t)
	if !s.disabled("back") || !s.disabled("forward") {
		t.Fatal("a new tab can already go back or forward")
	}
	s.click("go-guide")
	s.at("ember://guide")
	if s.disabled("back") {
		t.Fatal("back stays disabled after a visit")
	}
	s.click("back")
	s.at(home)
	s.click("forward")
	s.at("ember://guide")
}

func TestATypedWordSearchesThePages(t *testing.T) {
	s := show(t)
	s.enter("tile maps")
	s.at("ember://search?q=tile+maps")
	if text := s.doc.ByID("page").Text(); !strings.Contains(text, "Examples") || s.doc.ByID("none") != nil {
		t.Fatalf("the results are %q", text)
	}
}

func TestATypedHostNameIsAnHTTPSAddress(t *testing.T) {
	s := show(t)
	s.remotes["https://example.test"] = &remote{err: errors.New("no such host")}
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
	s.at(home)
	s.click("tab-0")
	s.at("ember://about")
	s.click("close-0")
	s.at(home)
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("%d tabs after closing one of two", len(s.browser.Tabs))
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTheStartPagesDemoTileOpensCSSZenGarden(t *testing.T) {
	old := client.Transport
	client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no network in tests")
	})
	t.Cleanup(func() { client.Transport = old })
	s := show(t)
	s.click("go-demo")
	if got := s.browser.Tab().Current(); got != "https://csszengarden.com" {
		t.Fatalf("the demo tile opened %q", got)
	}
}
