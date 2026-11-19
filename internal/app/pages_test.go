package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/ui/align"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

func TestEveryPageOfLightsOwnIsOpenedListedAndFound(t *testing.T) {
	s := show(t)
	if len(pages()) < 6 || pages()[0].name != tabs.HomeName {
		t.Fatalf("the pages are %v, the start page not first", pages())
	}
	for _, page := range pages() {
		address := tabs.Page(page.name)
		s.Open(address)
		s.frame()
		if got := s.title(address); got != page.title || s.node("page").HasClass("unreachable") {
			t.Errorf("%s is named %q, want %q", address, got, page.title)
		}
		if faults := align.Check(s.doc); len(faults) > 0 {
			t.Errorf("%s: layout faults: %v", address, faults)
		}
		s.Open(tabs.Search(page.title))
		s.frame()
		if text := s.node("page").Text(); !strings.Contains(text, address) {
			t.Errorf("a search for %q does not find %s: %q", page.title, address, text)
		}
		s.Open(tabs.Page(aboutName))
		s.frame()
		if text := s.node("own-pages").Text(); !strings.Contains(text, address) {
			t.Errorf("the About page does not list %s: %q", address, text)
		}
		s.Open(tabs.Home)
		s.frame()
		if s.doc.ByID("go-"+page.name) != nil || s.doc.ByID("own-links") != nil || s.doc.ByID("home-note") != nil {
			t.Errorf("the start page has a footer, or a link to %s in it", address)
		}
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		t.Fatalf("CSS warnings: %v", warns)
	}
}

func TestAnUnknownPageOfLightsListsThoseThatExist(t *testing.T) {
	s := show(t)
	s.enter("light://nowhere")
	s.at("light://nowhere")
	text := s.node("page").Text()
	if !s.node("page").HasClass("unreachable") || !strings.Contains(text, "light has no page nowhere") {
		t.Fatalf("the page reads %q", text)
	}
	for _, page := range pages() {
		if !strings.Contains(s.node("own-pages").Text(), tabs.Page(page.name)) {
			t.Errorf("%s is not listed: %q", page.name, text)
		}
	}
	if faults := align.Check(s.doc); len(faults) > 0 {
		t.Fatalf("layout faults: %v", faults)
	}
}

func TestTheSchemeLightHadIsNoAddressOfItsPages(t *testing.T) {
	s := show(t)
	s.enter("ember://home")
	s.at("ember://home")
	if text := s.node("page").Text(); !s.node("page").HasClass("unreachable") ||
		!strings.Contains(text, "light opens light://, http:// and https:// addresses") {
		t.Fatalf("the page reads %q", text)
	}
	s.configure(func(c *config.Config) { c.SearchEngine = search.Own })
	s.enter("history")
	s.at(tabs.Search("history"))
	if text := s.node("page").Text(); !strings.Contains(text, historyPage) {
		t.Fatalf("a page's bare name is a search that finds it: %q", text)
	}
}
