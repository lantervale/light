package app //nolint:testpackage // the screen is driven by what it holds: its tabs, its sessions, its frames

import (
	"testing"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/tabs"
)

// blankSite is a page whose links and forms ask for a window of their
// own, and a frame of another site that does too.
func blankSite(t *testing.T) (*framing, *shown) {
	t.Helper()
	const asking = `<body style="margin:0"><a id="new" href="/next" target="_blank">new</a>
		<a id="named" href="/next" target="elsewhere">named</a> <a id="shouting" href="/next" target="_BLANK">loud</a>
		<form action="/found" target="_blank"><input name="q" value="a b"><button id="find">find</button></form>
		<form action="/sent" method="post" target="_blank"><input name="p" value="1"><button id="send">send</button>
		</form>`
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(asking + `<iframe id="one" src="https://other.test/links" width="500"></iframe>
			<iframe id="closed" sandbox src="https://other.test/links" width="500"></iframe>`),
		"site.test/next":   html(`<title>Next</title><p>the next page</p>`),
		"site.test/found":  html(`<title>Found</title><p>found it</p>`),
		"site.test/sent":   html(`<title>Sent</title><p>it was sent</p>`),
		"other.test/links": html(asking),
		"other.test/next":  html(`<title>Next</title><p>the next page</p>`),
		"other.test/found": html(`<title>Found</title><p>found it</p>`),
		"other.test/sent":  html(`<title>Sent</title><p>it was sent</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.system = "darwin"
	s.visit("https://site.test/")
	s.framedText("one")
	s.framedText("closed")
	s.quiet()
	s.OpenInNewTab(tabs.Home)
	s.click("tab-0")
	return f, s
}

// landed waits for the shown tab to be the second, with the page at
// address, and closes it.
func (s *shown) landed(address string) {
	s.t.Helper()
	const top = "https://site.test/"
	if got, want := s.opened(), top+" ["+address+"] "+tabs.Home; got != want {
		s.t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.until(address+" loaded in its tab", func() bool { return s.session().pages[address] != nil })
	if s.browser.Tab().CanBack() {
		s.t.Errorf("the tab opened on %s can go back", address)
	}
	s.click("close-1")
	s.click("tab-0")
	if got, want := s.opened(), "["+top+"] "+tabs.Home; got != want {
		s.t.Fatalf("after closing the tab the tabs are %s, want %s", got, want)
	}
	s.until("the page shows again", func() bool { return s.host != nil })
	s.frame()
}

func TestALinkToANewWindowOpensAShownTabBesideItsOwn(t *testing.T) {
	f, s := blankSite(t)
	for _, id := range []string{"new", "named", "shouting"} {
		s.clickAt(s.inPage(id))
		s.landed("https://site.test/next")
	}
	for name, want := range map[string]string{"Referer": "https://site.test/", "Sec-Fetch-Site": "same-origin",
		"Sec-Fetch-Dest": "document", "Sec-Fetch-User": "?1"} {
		if got := f.header("site.test/next", name); got != want {
			t.Errorf("the new tab's navigation says %s: %q, want %q", name, got, want)
		}
	}
	s.clean()
}

func TestAFormToANewWindowIsSentInAShownTabBesideItsOwn(t *testing.T) {
	f, s := blankSite(t)
	s.clickAt(s.inPage("find"))
	s.landed("https://site.test/found?q=a+b")
	s.clickAt(s.inPage("send"))
	s.landed("https://site.test/sent")
	for name, want := range map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"Origin": "https://site.test", "Referer": "https://site.test/"} {
		if got := f.header("site.test/sent", name); got != want {
			t.Errorf("the form sent in a new tab says %s: %q, want %q", name, got, want)
		}
	}
	if len(s.session().posts) != 0 {
		t.Errorf("the page's own tab still holds %d forms to send", len(s.session().posts))
	}
}

func TestTheKeysHeldWinOverALinksNewWindow(t *testing.T) {
	_, s := blankSite(t)
	const top, next = "https://site.test/", "https://site.test/next"
	s.clickHolding(s.inPage("new"), input.KeyLeftSuper)
	if got, want := s.opened(), "["+top+"] "+next+" "+tabs.Home; got != want {
		t.Fatalf("after a Command click the tabs are %s, want %s", got, want)
	}
	s.middleClickAt(s.inPage("new"))
	if got, want := s.opened(), "["+top+"] "+next+" "+next+" "+tabs.Home; got != want {
		t.Fatalf("after a middle click the tabs are %s, want %s", got, want)
	}
	s.clickHolding(s.inPage("new"), input.KeyLeftShift)
	if len(s.screens) != 2 || len(s.browser.Tabs) != 4 {
		t.Fatalf("after a Shift click light has %d windows, the first %d tabs", len(s.screens), len(s.browser.Tabs))
	}
	if got, want := s.window(1).opened(), "["+next+"]"; got != want {
		t.Fatalf("the new window's tabs are %s, want %s", got, want)
	}
}

func TestALinkToANewWindowInAFrameOpensAShownTabOfTheWindow(t *testing.T) {
	f, s := blankSite(t)
	one := s.frameAt("one")
	for _, id := range []string{"new", "named"} {
		s.clickAt(s.inFrame(one, id))
		s.landed("https://other.test/next")
		one = s.frameAt("one")
		if one.page == nil || one.page.Address != "https://other.test/links" {
			t.Fatalf("after a click on %q the frame left its document", id)
		}
	}
	if got := f.header("other.test/next", "Referer"); got != "https://other.test/links" {
		t.Errorf("the new tab's navigation says it comes from %q, want the frame's document", got)
	}
	s.clickAt(s.inFrame(one, "find"))
	s.landed("https://other.test/found?q=a+b")
	s.clickAt(s.inFrame(s.frameAt("one"), "send"))
	s.landed("https://other.test/sent")
	if got := f.header("other.test/sent", "Origin"); got != "https://other.test" {
		t.Errorf("the frame's form sent in a new tab says it is of %q", got)
	}
	s.clickHolding(s.inFrame(s.frameAt("one"), "new"), input.KeyLeftSuper)
	if got, want := s.opened(), "[https://site.test/] https://other.test/next "+tabs.Home; got != want {
		t.Fatalf("after a Command click in the frame the tabs are %s, want %s", got, want)
	}
}

func TestASandboxedFramesLinkToANewWindowStillGoesNowhere(t *testing.T) {
	_, s := blankSite(t)
	closed := s.frameAt("closed")
	s.clickAt(s.inFrame(closed, "new"))
	for range 10 {
		s.frame()
	}
	if got, want := s.opened(), "[https://site.test/] "+tabs.Home; got != want || len(s.screens) != 1 {
		t.Fatalf("the tabs are %s, want %s, in %d windows", got, want, len(s.screens))
	}
	if at := s.frameAt("closed").page.Address; at != "https://other.test/links" {
		t.Errorf("the sandboxed frame went to %s", at)
	}
	if !s.consoleSays(console.Error, "'allow-popups' permission is not set") {
		t.Errorf("the console does not say why: %v", s.session().said.Lines())
	}
}

func TestASandboxedFrameWithoutPopupsOpensNoWindowAndGoesNowhere(t *testing.T) {
	const links = `<body style="margin:0"><a id="new" href="/next" target="_blank">new</a>
		<a id="named" href="/next" target="elsewhere">named</a>
		<form action="/found" target="_blank"><button id="find">find</button></form>`
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<body style="margin:0">
			<iframe id="leaving" sandbox="allow-top-navigation allow-forms" src="https://other.test/links" width="500"></iframe>
			<iframe id="closed" sandbox="allow-forms" src="https://other.test/links" width="500"></iframe>`),
		"other.test/links": html(links),
		"other.test/next":  html(`<p>the next page</p>`),
		"other.test/found": html(`<p>found it</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	for _, frame := range []string{"leaving", "closed"} {
		s.framedText(frame)
		for _, id := range []string{"new", "named", "find"} {
			s.session().said.Clear()
			s.clickAt(s.inFrame(s.frameAt(frame), id))
			s.quiet()
			if got := s.opened(); got != "[https://site.test/]" {
				t.Fatalf("%s in the frame %s left the tabs as %s", id, frame, got)
			}
			if got := s.framedText(frame); got != "new named find" {
				t.Errorf("%s in the frame %s sent the frame to %q", id, frame, got)
			}
			if !s.consoleSays(console.Error, "'allow-popups' permission is not set") {
				t.Errorf("%s in the frame %s was kept without a word in the console", id, frame)
			}
		}
	}
	if f.came("other.test/next") || f.came("other.test/found") {
		t.Error("a window a sandbox kept closed was asked for")
	}
}

// escaping is a page with two sandboxed frames allowed popups, one of
// them allowed to open them out of its sandbox; more is what else both
// sandboxes allow, and body what their document holds after its links
// and forms.
func escaping(t *testing.T, more, body string) (*framing, *shown) {
	t.Helper()
	const links = `<body style="margin:0"><a id="new" href="/next" target="_blank">new</a>
		<a id="named" href="/next" target="elsewhere">named</a>
		<form action="/found" target="_blank"><button id="find">find</button></form>
		<form id="post" action="/sent" method="post" target="_blank"><button id="send">send</button></form>`
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<body style="margin:0">
			<iframe id="held" sandbox="allow-popups allow-forms ` + more + `" src="https://other.test/links" width="500">
			</iframe><iframe id="free" sandbox="allow-popups allow-popups-to-escape-sandbox allow-forms ` + more + `"
			src="https://other.test/links" width="500"></iframe>`),
		"other.test/links": html(links + body),
		"other.test/next":  html(`<title>Next</title><p>the next page</p>`),
		"other.test/found": html(`<title>Found</title><p>found it</p>`),
		"other.test/sent":  html(`<title>Sent</title><p>it was sent</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.framedText("held")
	s.framedText("free")
	s.quiet()
	return f, s
}

// refused checks that a click on id in the frame held opened nothing
// and that the console names the token the frame lacks.
func (s *shown) refused(f *framing, id string) {
	s.t.Helper()
	s.session().said.Clear()
	before := s.framedText("held")
	s.clickAt(s.inFrame(s.frameAt("held"), id))
	s.quiet()
	if got := s.opened(); got != "[https://site.test/]" || len(s.screens) != 1 {
		s.t.Fatalf("%s in a frame that may not leave its sandbox left the tabs as %s, in %d windows", id, got,
			len(s.screens))
	}
	if got := s.framedText("held"); got != before || s.frameAt("held").page.Address != "https://other.test/links" {
		s.t.Errorf("%s sent the frame to %q", id, got)
	}
	if !s.consoleSays(console.Error, "'allow-popups-to-escape-sandbox' permission is not set") {
		s.t.Errorf("%s was refused without the console naming the token: %v", id, s.session().said.Lines())
	}
	if lines := s.session().said.Lines(); len(lines) != 1 || s.consoleSays(console.Error, "'allow-popups' permission") {
		s.t.Errorf("%s was refused in %d lines, want the one that names the token the frame lacks: %v", id, len(lines),
			lines)
	}
	for _, to := range []string{"other.test/next", "other.test/found", "other.test/sent"} {
		if f.came(to) {
			s.t.Fatalf("%s asked for %s", id, to)
		}
	}
}

func TestASandboxedFrameOpensNoWindowItsSandboxWouldNotFollowInto(t *testing.T) {
	f, s := escaping(t, "", "")
	for _, id := range []string{"new", "named", "find", "send"} {
		s.refused(f, id)
	}
}

func TestASandboxedFrameAllowedToEscapeOpensAnUnsandboxedTab(t *testing.T) {
	f, s := escaping(t, "", "")
	for id, address := range map[string]string{"new": "https://other.test/next", "named": "https://other.test/next",
		"find": "https://other.test/found", "send": "https://other.test/sent"} {
		s.session().said.Clear()
		s.clickAt(s.inFrame(s.frameAt("free"), id))
		if got, want := s.opened(), "https://site.test/ ["+address+"]"; got != want {
			t.Fatalf("%s in a frame allowed to escape left the tabs as %s, want %s", id, got, want)
		}
		s.until(address+" loaded in its tab", func() bool { return s.session().pages[address] != nil })
		if set := s.session().frames[s.session().pages[address]]; set != nil && len(set.all) != 0 {
			t.Errorf("the tab opened by %s holds %d frames", id, len(set.all))
		}
		s.click("close-1")
		s.until("the page shows again", func() bool { return s.host != nil })
		s.framedText("free")
		s.quiet()
	}
	if got := f.header("other.test/sent", "Origin"); got != "null" {
		t.Errorf("the sandboxed frame's form says it is of %q", got)
	}
}
