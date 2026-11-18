package tabs_test

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/tabs"
)

func TestVisitingAfterBackDropsTheForwardPages(t *testing.T) {
	h := tabs.NewTab("a")
	h.Visit("b")
	h.Visit("c")
	h.Back()
	h.Back()
	h.Back()
	if h.Current() != "a" || h.CanBack() || !h.CanForward() {
		t.Fatalf("at %q, back %v, forward %v", h.Current(), h.CanBack(), h.CanForward())
	}
	h.Forward()
	h.Visit("d")
	if h.Current() != "d" || h.CanForward() {
		t.Fatalf("at %q, forward %v", h.Current(), h.CanForward())
	}
	h.Back()
	if h.Current() != "b" {
		t.Fatalf("back from d is %q", h.Current())
	}
}

func TestATabCountsThePagesItCanGoThrough(t *testing.T) {
	h := tabs.NewTab("a")
	h.Visit("b")
	h.Visit("c")
	h.Back()
	if h.Len() != 3 {
		t.Fatalf("a tab of three pages counts %d", h.Len())
	}
	h.Visit("d")
	if h.Len() != 3 {
		t.Fatalf("after a visit from the middle the tab counts %d", h.Len())
	}
}

func TestVisitingTheSamePageAddsNothing(t *testing.T) {
	h := tabs.NewTab("a")
	h.Visit("a")
	if h.CanBack() {
		t.Fatal("a reload became a history entry")
	}
}

func TestResolve(t *testing.T) {
	for typed, want := range map[string]string{
		"":                 tabs.Home,
		"  light://guide ": "light://guide",
		"example.com":      "https://example.com",
		"tile maps":        "light://search?q=tile+maps",
		"guide":            "light://search?q=guide",
		"what is 1.5 zoom": "light://search?q=what+is+1.5+zoom",
	} {
		if got := tabs.Resolve(typed); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", typed, got, want)
		}
	}
}

func TestClosingTabsKeepsTheShownOne(t *testing.T) {
	b := tabs.NewBrowser(tabs.Home)
	b.Open("b")
	b.Open("c")
	b.Select(1)
	b.Close(0)
	if b.Tab().Current() != "b" {
		t.Fatalf("closing a tab before the shown one shows %q", b.Tab().Current())
	}
	b.Close(1)
	if b.Tab().Current() != "b" || len(b.Tabs) != 1 {
		t.Fatalf("shows %q of %d tabs", b.Tab().Current(), len(b.Tabs))
	}
	b.Close(0)
	if len(b.Tabs) != 1 || b.Tab().Current() != tabs.Home {
		t.Fatalf("closing the last tab leaves %q of %d tabs", b.Tab().Current(), len(b.Tabs))
	}
}

func TestARedirectedPageTakesItsFinalAddress(t *testing.T) {
	h := tabs.NewTab("a")
	h.Visit("http://b")
	h.Redirect("http://b", "https://b/")
	if h.Current() != "https://b/" || h.CanForward() {
		t.Fatalf("at %q, forward %v", h.Current(), h.CanForward())
	}
	h.Back()
	h.Redirect("http://b", "https://c/")
	if h.Current() != "a" {
		t.Fatalf("a redirect of a page left moved the tab to %q", h.Current())
	}
	h.Forward()
	if h.Current() != "https://b/" {
		t.Fatalf("forward is %q", h.Current())
	}
}

func TestAPageWithNothingToShowLeavesTheTabWhereItWas(t *testing.T) {
	h := tabs.NewTab("a")
	h.Visit("b")
	h.Visit("empty")
	h.Drop()
	if h.Current() != "b" || h.CanForward() {
		t.Fatalf("at %q, forward %v", h.Current(), h.CanForward())
	}
	h = tabs.NewTab("empty")
	h.Drop()
	if h.Current() != "empty" {
		t.Fatalf("a tab's only page was dropped: at %q", h.Current())
	}
}

func TestADuplicateKeepsTheTabsWayBackAndIsShown(t *testing.T) {
	b := tabs.NewBrowser(tabs.Home)
	b.Tab().Visit("b")
	b.Open("c")
	b.Duplicate(0)
	if len(b.Tabs) != 3 || b.Active != 1 || b.Tab().Current() != "b" || !b.Tab().CanBack() {
		t.Fatalf("%d tabs, shown %d at %q", len(b.Tabs), b.Active, b.Tab().Current())
	}
	b.Tab().Back()
	if b.Tabs[0].Current() != "b" {
		t.Fatal("the duplicate moves its original")
	}
}

func TestClosingTheOtherTabsLeavesOne(t *testing.T) {
	b := tabs.NewBrowser(tabs.Home)
	b.Open("b")
	b.Open("c")
	b.CloseOthers(1)
	if len(b.Tabs) != 1 || b.Active != 0 || b.Tab().Current() != "b" {
		t.Fatalf("%d tabs, shown %q", len(b.Tabs), b.Tab().Current())
	}
}

func TestResolveKeepsAViewSourcePrefix(t *testing.T) {
	for typed, want := range map[string]string{
		"view-source:example.com":         "view-source:https://example.com",
		"view-source:https://example.com": "view-source:https://example.com",
	} {
		if got := tabs.Resolve(typed); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", typed, got, want)
		}
	}
	if of, ok := tabs.Source("view-source:https://example.com"); !ok || of != "https://example.com" {
		t.Errorf("Source gives %q, %v", of, ok)
	}
	if got := tabs.SourceOf("https://example.com"); got != "view-source:https://example.com" {
		t.Errorf("SourceOf gives %q", got)
	}
	if _, ok := tabs.Source("https://example.com"); ok {
		t.Error("a plain address is a source view")
	}
}

func TestLightsOwnPagesAreAtItsScheme(t *testing.T) {
	if tabs.Home != "light://home" || tabs.Page("history") != "light://history" {
		t.Fatalf("home is %s, the history %s", tabs.Home, tabs.Page("history"))
	}
	if got := tabs.Search("tile maps"); got != "light://search?q=tile+maps" {
		t.Fatalf("a search is at %s", got)
	}
	for address, want := range map[string][2]string{
		"light://history":            {"history", ""},
		"light://search?q=tile+maps": {"search", "tile maps"},
		"light://":                   {"", ""},
	} {
		name, query, own := tabs.Own(address)
		if !own || name != want[0] || query != want[1] {
			t.Errorf("%s is the page %q asked %q, own %v", address, name, query, own)
		}
	}
	for _, address := range []string{"ember://home", "https://light/", "view-source:light://home", "history", ":"} {
		if name, _, own := tabs.Own(address); own {
			t.Errorf("%s is taken for light's page %q", address, name)
		}
	}
}

func TestABrowserStartsAndStartsAgainOnItsStartPage(t *testing.T) {
	b := tabs.NewBrowser("https://example.com")
	if got := b.Tab().Current(); got != "https://example.com" {
		t.Fatalf("the first tab is at %s", got)
	}
	b.Start = "https://example.org"
	b.Tab().Visit("b")
	b.Close(0)
	if got := b.Tab().Current(); len(b.Tabs) != 1 || got != "https://example.org" || b.Tab().CanBack() {
		t.Fatalf("closing the last tab leaves one at %s", got)
	}
}

func addresses(b *tabs.Browser) string {
	var out []string
	for _, tab := range b.Tabs {
		out = append(out, tab.Current())
	}
	return strings.Join(out, " ")
}

func TestATabOpenedBesideTheShownOneFollowsThoseItOpened(t *testing.T) {
	b := tabs.NewBrowser("a")
	b.Open("z")
	b.Select(0)
	first := b.OpenBeside("b", false)
	b.OpenBeside("c", false)
	if got := addresses(b); got != "a b c z" || b.Active != 0 || first.Current() != "b" {
		t.Fatalf("the tabs are %q, tab %d shown", got, b.Active)
	}
	b.OpenBeside("d", true)
	if got := addresses(b); got != "a b c d z" || b.Tab().Current() != "d" {
		t.Fatalf("the tabs are %q, %q shown", got, b.Tab().Current())
	}
	b.OpenBeside("e", false)
	if got := addresses(b); got != "a b c d e z" || b.Tab().Current() != "d" {
		t.Fatalf("the tabs are %q, %q shown", got, b.Tab().Current())
	}
}
