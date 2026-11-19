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
		if got := tabs.Resolve(typed, nil); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", typed, got, want)
		}
	}
}

func TestWhatIsTypedIsAnAddressOrWordsForTheSearchItIsGiven(t *testing.T) {
	engine := func(query string) string { return "search:" + query }
	for typed, want := range map[string]string{
		"light://search?q=tile+maps":       "light://search?q=tile+maps",
		"light://config":                   "light://config",
		"https://example.com/a b":          "https://example.com/a b",
		"HTTP://example.com":               "HTTP://example.com",
		"ftp://example.test":               "ftp://example.test",
		"about:blank":                      "about:blank",
		"example.com/path?q=1#top":         "https://example.com/path?q=1#top",
		"example.com:8080":                 "https://example.com:8080",
		"example.com.":                     "https://example.com.",
		"example.com./path":                "https://example.com./path",
		"example.com/?to=http://other.com": "https://example.com/?to=http://other.com",
		"localhost":                        "http://localhost",
		"LocalHost:8080":                   "http://LocalHost:8080",
		"localhost/path":                   "http://localhost/path",
		"app.localhost:3000":               "http://app.localhost:3000",
		"127.0.0.1":                        "http://127.0.0.1",
		"192.168.1.1:8080/admin":           "http://192.168.1.1:8080/admin",
		"[::1]":                            "http://[::1]",
		"[::1]:8080/x":                     "http://[::1]:8080/x",
		"foo.bar baz":                      "search:foo.bar baz",
		"guide":                            "search:guide",
		"localhost rocks":                  "search:localhost rocks",
		"what does :// mean":               "search:what does :// mean",
		"go: the language":                 "search:go: the language",
		"about":                            "search:about",
		".hidden":                          "search:.hidden",
		"...":                              "search:...",
		"a..b":                             "search:a..b",
		"wait.\twhat":                      "search:wait.\twhat",
		"[::1":                             "search:[::1",
		"view-source:localhost:8080":       "view-source:http://localhost:8080",
		"view-source:tile maps":            "view-source:search:tile maps",
	} {
		if got := tabs.Resolve(typed, engine); got != want {
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
		if got := tabs.Resolve(typed, nil); got != want {
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

// travels is a tab's history played from a script: "visit a", "push
// a#x", "frame 0 b", "pushframe 0 b#x", "replaceframe 0 c", "go -1",
// "gone 0" (the page has no frame there any more), "here 0",
// "redirect a2", "renameframe 0 b#y", then what the tab is at: its page and its
// frames.
func travels(t *testing.T, script string) (*tabs.Tab, string) {
	t.Helper()
	h := tabs.NewTab("start")
	gone := map[string]bool{}
	has := func(path string) bool { return !gone[path] }
	var changes []string
	for _, line := range strings.Split(script, ";") {
		word := strings.Fields(line)
		switch word[0] {
		case "visit":
			h.Visit(word[1])
		case "push":
			h.Push(word[1])
		case "frame":
			h.VisitFrame(word[1], word[2])
		case "pushframe":
			h.PushFrame(word[1], word[2])
		case "replaceframe":
			h.ReplaceFrame(word[1], word[2])
		case "redirect":
			h.Redirect(h.Current(), word[1])
		case "renameframe":
			h.RenameFrame(word[1], word[2])
		case "gone":
			gone[word[1]] = true
		case "here":
			delete(gone, word[1])
		case "go":
			delta := 1
			if word[1] == "-1" {
				delta = -1
			} else if word[1] != "1" {
				delta = map[string]int{"-2": -2, "2": 2, "-3": -3, "3": 3, "-9": -9}[word[1]]
			}
			c := h.Travel(delta, has)
			change := strings.Join(c.Frames, ",")
			if c.Top {
				change = "top"
			}
			changes = append(changes, change)
		default:
			t.Fatalf("no step %q", line)
		}
	}
	at := h.Current()
	for _, path := range []string{"0", "0/0", "1", "named"} {
		if to := h.Frame(path); to != "" {
			at += " " + path + "=" + to
		}
	}
	return h, at + " | " + strings.Join(changes, " ")
}

func TestATabsHistoryGoesThroughItsFrames(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{"a frame's step is one of the tab's", "visit a;frame 0 b;go -1", "a | 0"},
		{"and forward again", "visit a;frame 0 b;go -1;go 1", "a 0=b | 0 0"},
		{"two frames in the order they went", "visit a;frame 0 b;frame 1 c;frame 0 d;go -1;go -1",
			"a 0=b | 0 1"},
		{"two frames, all the way back, then the page", "visit a;frame 0 b;frame 1 c;go -1;go -1;go -1",
			"start | 1 0 top"},
		{"several steps at once", "visit a;frame 0 b;frame 1 c;frame 0 d;go -3;go 2", "a 0=b 1=c | 0,1 0,1"},
		{"past either end nothing moves", "visit a;frame 0 b;go -9;go 2", "a 0=b |  "},
		{"a step after going back drops what was ahead", "visit a;frame 0 b;frame 0 c;go -1;frame 1 d;go 1;go -1",
			"a 0=b | 0  1"},
		{"a frame in a frame", "visit a;frame 0 b;frame 0/0 c;go -1;go 1", "a 0=b 0/0=c | 0/0 0/0"},
		{"a frame that moves starts those in it again", "visit a;frame 0 b;frame 0/0 c;frame 0 d", "a 0=d | "},
		{"back over it brings them back, the outer one told", "visit a;frame 0 b;frame 0/0 c;frame 0 d;go -1",
			"a 0=b 0/0=c | 0"},
		{"a frame that is no more is passed over", "visit a;frame 0 b;frame 1 c;gone 1;go -1", "a | 0"},
		{"and the step before it is taken when only it is behind", "visit a;frame 1 c;gone 1;go -1", "start | top"},
		{"a frame gone at the first step leaves the tab there", "frame 1 c;gone 1;go -1", "start 1=c | "},
		{"a frame that came back is stepped through again", "visit a;frame 1 c;gone 1;here 1;go -1", "a | 1"},
		{"going forward passes over it too", "visit a;frame 1 c;frame 0 b;go -2;gone 1;go 1", "a 0=b 1=c | 0,1 0"},
		{"replacing adds no step", "visit a;frame 0 b;replaceframe 0 c;go -1;go 1", "a 0=c | 0 0"},
		{"replacing the first document of a frame adds none", "visit a;replaceframe 0 c;go -1", "start | top"},
		{"a named frame", "visit a;frame named b;go -1;go 1", "a named=b | named named"},
		{"an address a frame's document gave itself keeps the frames in it",
			"visit a;frame 0 b;frame 0/0 c;pushframe 0 b#x;go -1", "a 0=b 0/0=c | 0"},
		{"an address the page gave itself keeps its frames", "visit a;frame 0 b;push a#x;go -1", "a 0=b | top"},
		{"another page starts with its frames where their elements ask", "visit a;frame 0 b;visit c", "c | "},
		{"back to a page brings its frames back", "visit a;frame 0 b;visit c;go -1", "a 0=b | top"},
		{"a redirect keeps the frames", "visit a;frame 0 b;redirect a2", "a2 0=b | "},
		{"an address a frame's document took in place of its own adds no step",
			"visit a;frame 0 b;frame 0/0 c;renameframe 0 b#y;go -1;go 1", "a 0=b#y 0/0=c | 0 0"},
		{"the same address twice is one step", "visit a;frame 0 b;frame 0 b;go -1;go -1", "start | 0 top"},
	} {
		if _, got := travels(t, c.script); got != c.want {
			t.Errorf("%s (%s): the tab is at %q, want %q", c.name, c.script, got, c.want)
		}
	}
}

func TestATabCountsItsFramesSteps(t *testing.T) {
	h, _ := travels(t, "visit a;frame 0 b;pushframe 0 b#x;replaceframe 0 c;frame 1 d")
	if h.Len() != 5 || h.At() != 4 || !h.CanBack() || h.CanForward() {
		t.Errorf("a tab of five steps counts %d, at %d", h.Len(), h.At())
	}
	ahead := h.Ahead(-2, nil)
	if h.At() != 4 || !ahead.Moved() || strings.Join(ahead.Frames, ",") != "0,1" {
		t.Errorf("looking ahead moved the tab to %d, or told %v", h.At(), ahead)
	}
	h.Back()
	h.Take(ahead)
	if h.At() != 3 {
		t.Errorf("a move told of from another step was made: the tab is at %d", h.At())
	}
	if since := h.Since(4, nil); since.Top || strings.Join(since.Frames, ",") != "1" {
		t.Errorf("since the last step %v changed", since)
	}
}

func TestADuplicateKeepsItsOwnFrames(t *testing.T) {
	b := tabs.NewBrowser("start")
	b.Tab().VisitFrame("0", "b")
	b.Duplicate(0)
	b.Tabs[1].ReplaceFrame("0", "c")
	b.Tabs[1].Back()
	b.Tabs[1].Forward()
	if b.Tabs[0].Frame("0") != "b" || b.Tabs[1].Frame("0") != "c" || b.Tabs[1].Len() != 2 {
		t.Errorf("the tab's frame is at %q and its copy's at %q", b.Tabs[0].Frame("0"), b.Tabs[1].Frame("0"))
	}
}

func TestAFramesPathIsItsKeysFromThePageDown(t *testing.T) {
	if got := tabs.FramePath("0", "a/b", "é 1"); got != "0/a%2Fb/%C3%A9%201" {
		t.Errorf("the path is %q", got)
	}
}
