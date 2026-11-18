package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/tabs"
)

// clickHolding clicks at with keys held.
func (s *shown) clickHolding(at geom.Vec2, keys ...input.Key) {
	for _, k := range keys {
		s.in.SetKey(k, true)
	}
	s.frame()
	s.clickAt(at)
	for _, k := range keys {
		s.in.SetKey(k, false)
	}
	s.frame()
}

func (s *shown) middleClickAt(at geom.Vec2) {
	s.in.SetMouse(at)
	s.frame()
	s.in.SetMouseButton(input.MouseMiddle, true)
	s.frame()
	s.in.SetMouseButton(input.MouseMiddle, false)
	s.frame()
	s.frame()
}

// middle is the middle of the screen's element id.
func (s *shown) middle(id string) geom.Vec2 {
	s.t.Helper()
	r, ok := s.doc.BoxOf(s.node(id))
	if !ok {
		s.t.Fatalf("%q is not shown", id)
	}
	return r.Center()
}

// opened are the addresses of the open tabs, the shown one in brackets.
func (s *shown) opened() string {
	var out []string
	for i, tab := range s.browser.Tabs {
		address := tab.Current()
		if i == s.browser.Active {
			address = "[" + address + "]"
		}
		out = append(out, address)
	}
	return strings.Join(out, " ")
}

func (s *shown) tabNames() string {
	var out []string
	for i := range s.browser.Tabs {
		out = append(out, s.node(tabID(i)).Text())
	}
	return strings.Join(out, " | ")
}

func TestTheKeysHeldAtAClickSayWhereALinkOpens(t *testing.T) {
	for _, c := range []struct {
		system string
		held   []input.Key
		middle bool
		want   opening
	}{
		{system: "darwin", want: inTab},
		{system: "darwin", held: []input.Key{input.KeyLeftSuper}, want: behind},
		{system: "darwin", held: []input.Key{input.KeyRightSuper}, want: behind},
		{system: "darwin", held: []input.Key{input.KeyLeftSuper, input.KeyLeftShift}, want: ahead},
		{system: "darwin", held: []input.Key{input.KeyLeftControl}, want: inTab},
		{system: "darwin", held: []input.Key{input.KeyLeftShift}, want: inTab},
		{system: "linux", held: []input.Key{input.KeyLeftControl}, want: behind},
		{system: "linux", held: []input.Key{input.KeyRightControl, input.KeyRightShift}, want: ahead},
		{system: "linux", held: []input.Key{input.KeyLeftSuper}, want: inTab},
		{system: "windows", held: []input.Key{input.KeyLeftControl}, want: behind},
		{system: "darwin", middle: true, want: behind},
		{system: "linux", middle: true, held: []input.Key{input.KeyLeftShift}, want: ahead},
	} {
		s := show(t)
		s.Open("light://about")
		s.frame()
		for _, k := range c.held {
			s.in.SetKey(k, true)
		}
		s.frame()
		if got := openingBy(c.system, s.in, c.middle); got != c.want {
			t.Errorf("on %s with %v held, middle %v: %v, want %v", c.system, c.held, c.middle, got, c.want)
		}
	}
}

func TestACommandClickOpensALinkInATabBehindTheShownOne(t *testing.T) {
	srv, seen := crypt(t)
	s := showWith(t, Options{Now: clock()})
	s.system = "darwin"
	s.visit(srv.URL + "/")
	s.clickHolding(s.inPage("vault"), input.KeyLeftSuper)
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/vaults"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.at(srv.URL + "/")
	if s.host == nil || s.browser.Tab().Len() != 2 {
		t.Fatal("the shown tab left its page")
	}
	behind := s.browser.Tabs[1]
	s.until("the tab behind loaded without being shown", func() bool {
		return s.sessions[behind] != nil && s.sessions[behind].pages[srv.URL+"/vaults"] != nil
	})
	if got, want := s.tabNames(), "A crypt | Vaults"; got != want {
		t.Errorf("the tabs are named %q, want %q", got, want)
	}
	if s.host == nil {
		t.Fatal("the shown page went when the tab behind landed")
	}
	s.click("tab-1")
	s.at(srv.URL + "/vaults")
	if seen.count("/vaults") != 1 {
		t.Errorf("the page was fetched %d times", seen.count("/vaults"))
	}
	s.Open(historyPage)
	s.frame()
	if got := strings.Join(s.visits(), "\n"); !strings.Contains(got, "Vaults | "+srv.URL+"/vaults") {
		t.Errorf("the history lists %q", got)
	}
	s.clean()
}

func TestACommandShiftClickShowsTheTabItOpens(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.system = "darwin"
	s.visit(srv.URL + "/")
	s.clickHolding(s.inPage("vault"), input.KeyLeftSuper, input.KeyLeftShift)
	if got, want := s.opened(), srv.URL+"/ ["+srv.URL+"/vaults]"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.until("the tab loaded", func() bool { return s.session().pages[srv.URL+"/vaults"] != nil })
	s.at(srv.URL + "/vaults")
	if s.browser.Tab().CanBack() {
		t.Fatal("the new tab can go back")
	}
}

func TestOffMacOSControlIsTheKey(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.system = "linux"
	s.visit(srv.URL + "/")
	s.clickHolding(s.inPage("vault"), input.KeyLeftControl)
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/vaults"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.clickHolding(s.inPage("vault"), input.KeyLeftSuper)
	s.until("the link was followed", func() bool { return s.session().pages[srv.URL+"/vaults"] != nil })
	if got, want := s.opened(), "["+srv.URL+"/vaults] "+srv.URL+"/vaults"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
}

func TestAMiddleClickOpensALinkInATabBehind(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.middleClickAt(s.inPage("vault"))
	s.middleClickAt(s.inPage("door"))
	s.middleClickAt(s.inPage("words"))
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/vaults "+srv.URL+"/vaults"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.middleClickAt(s.middle("tab-1"))
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/vaults"; got != want {
		t.Fatalf("after a middle click on a tab the tabs are %s, want %s", got, want)
	}
	s.in.SetKey(input.KeyLeftShift, true)
	s.middleClickAt(s.inPage("vault"))
	s.in.SetKey(input.KeyLeftShift, false)
	s.frame()
	if got, want := s.opened(), srv.URL+"/ "+srv.URL+"/vaults ["+srv.URL+"/vaults]"; got != want {
		t.Fatalf("with Shift the tabs are %s, want %s", got, want)
	}
}

func TestAMiddleButtonReleasedOffTheLinkItWentDownOnOpensNothing(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.in.SetMouse(s.inPage("vault"))
	s.frame()
	s.in.SetMouseButton(input.MouseMiddle, true)
	s.frame()
	s.in.SetMouse(s.inPage("words"))
	s.frame()
	s.in.SetMouseButton(input.MouseMiddle, false)
	s.frame()
	s.frame()
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("the tabs are %s", s.opened())
	}
}

func TestALinkThatGoesNowhereOpensNoTab(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Places</title><style>a { display: block; height: 24px }</style>
			<a id="top" href="#top">the top</a><a id="code" href="javascript:void(0)">some code</a>
			<a id="mail" href="mailto:a@example.test">a letter</a><a id="bare">no address</a>`),
	})
	s := show(t)
	s.system = "darwin"
	s.visit(srv.URL + "/")
	for _, id := range []string{"top", "code", "mail", "bare"} {
		s.clickHolding(s.inPage(id), input.KeyLeftSuper)
		s.middleClickAt(s.inPage(id))
		if got, want := s.opened(), "["+srv.URL+"/]"; got != want {
			t.Fatalf("after #%s the tabs are %s, want %s", id, got, want)
		}
	}
}

func TestLinksOfLightsOwnPagesOpenInTabsToo(t *testing.T) {
	s := showWith(t, Options{Now: clock()})
	s.system = "darwin"
	s.clickHolding(s.middle("go-guide"), input.KeyLeftSuper)
	s.clickHolding(s.middle("go-about"), input.KeyLeftSuper)
	s.middleClickAt(s.middle("go-examples"))
	if got, want := s.opened(), "[light://home] light://guide light://about light://examples"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	if got, want := s.tabNames(), "New Tab | The ui guide | About | Examples"; got != want {
		t.Errorf("the tabs are named %q, want %q", got, want)
	}
	s.Open(historyPage)
	s.frame()
	filter := s.node("history-filter")
	s.doc.SetValue(filter, "guide")
	s.listVisits(s.node("history-list"), "guide")
	s.frame()
	var row geom.Vec2
	for _, n := range s.node("history-list").Children() {
		if n.HasClass("visit") {
			r, _ := s.doc.BoxOf(n.Children()[2])
			row = r.Center()
		}
	}
	s.clickHolding(row, input.KeyLeftSuper)
	if got, want := s.opened(),
		"[light://history] light://guide light://about light://examples light://guide"; got != want {
		t.Fatalf("from the history the tabs are %s, want %s", got, want)
	}
	if got := s.doc.Value(filter); got != "guide" || s.node("history-filter") != filter {
		t.Fatalf("the history page was built again under the click: its filter reads %q", got)
	}
	s.clean()
}

func TestACommandClickWhileAFieldHasTheKeyboard(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.system = "darwin"
	s.visit(srv.URL + "/")
	s.click("address")
	s.clickHolding(s.inPage("vault"), input.KeyLeftSuper)
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/vaults"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.Open(tabs.Home)
	s.frame()
	s.frame()
	s.clickHolding(s.middle("go-guide"), input.KeyLeftSuper)
	if got, want := s.opened(), "[light://home] "+srv.URL+"/vaults light://guide"; got != want {
		t.Fatalf("from the start page, its search focused, the tabs are %s, want %s", got, want)
	}
}

func TestACommandClickWhileAFieldOfThePageHasTheKeyboard(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>A form</title><style>a, input { display: block; height: 24px }</style>
			<input id="name"><a id="on" href="/on">on</a><p id="words">Some words.</p>`),
		"/on": typed("text/html", `<title>On</title><p>There.</p>`),
	})
	s := show(t)
	s.system = "darwin"
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("name"))
	s.in.AddText('a')
	s.frame()
	s.clickHolding(s.inPage("on"), input.KeyLeftSuper)
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/on"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
}
