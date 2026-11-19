package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// hosts is one server that answers for every host name, and the Cookie
// header each request came with, by its host and path.
type hosts struct {
	sync.Mutex
	got       map[string]string
	transport http.RoundTripper
}

func names(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *hosts {
	t.Helper()
	n := &hosts{got: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Lock()
		n.got[r.Host+r.URL.Path] = r.Header.Get("Cookie")
		n.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}}
	t.Cleanup(transport.CloseIdleConnections)
	n.transport = transport
	return n
}

// came reports whether a request for that host and path came.
func (n *hosts) came(to string) bool {
	n.Lock()
	defer n.Unlock()
	_, came := n.got[to]
	return came
}

// sent is how many cookies the last request for that host and path
// came with, -1 for none that came.
func (n *hosts) sent(to string) int {
	n.Lock()
	defer n.Unlock()
	header, came := n.got[to]
	switch {
	case !came:
		return -1
	case header == "":
		return 0
	}
	return strings.Count(header, ";") + 1
}

// keptValue is the value of the cookie that outlives a run: no file
// but the cookies' may hold it.
const keptValue = "kept-7f3a9c"

// cookied is a site that sets a cookie that lasts and one of the run
// on its front page, which asks for a sheet, a picture and a third
// party's picture.
func cookied(t *testing.T) *hosts {
	t.Helper()
	pixel := string(picture(t, 2, 2))
	return names(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Host + r.URL.Path {
		case "site.test/":
			w.Header().Add("Set-Cookie", "kept="+keptValue+"; Max-Age=3600; Path=/; HttpOnly")
			w.Header().Add("Set-Cookie", "run=1; Path=/")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Cookied</title><link rel="stylesheet" href="/site.css">
				<img src="/map.png" alt="a map"><img src="http://tracker.test/pixel.png" alt="a pixel">
				<a id="next" href="/next">next</a>`))
		case "site.test/site.css":
			w.Header().Set("Content-Type", "text/css")
		case "site.test/map.png", "tracker.test/pixel.png":
			w.Header().Add("Set-Cookie", "id=1; Path=/")
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(pixel))
		case "site.test/file.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("a file"))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Next</title><p>next</p>`))
		}
	})
}

func (s *shown) pictured(n *hosts) {
	s.t.Helper()
	s.until("the page's pictures were asked for", func() bool {
		return n.came("site.test/map.png") && n.came("tracker.test/pixel.png") && n.came("site.test/site.css")
	})
}

func TestACookieAPageSetsIsSentAfterItAndKeptForTheNextRun(t *testing.T) {
	n := cookied(t)
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Transport: n.transport})
	s.visit("http://site.test/")
	s.pictured(n)
	if got := n.sent("site.test/"); got != 0 {
		t.Fatalf("the first request came with %d cookies", got)
	}
	for _, to := range []string{"site.test/site.css", "site.test/map.png"} {
		if got := n.sent(to); got != 2 {
			t.Errorf("the page's own %s came with %d cookies, want the 2 its page set", to, got)
		}
	}
	if got := n.sent("tracker.test/pixel.png"); got != 0 {
		t.Errorf("a third party's picture came with %d cookies", got)
	}
	if got := s.jar.Len(); got != 3 {
		t.Errorf("the jar holds %d cookies, want the page's 2 and its picture's, none of a third party", got)
	}
	s.visit("http://site.test/next")
	if got := n.sent("site.test/next"); got != 3 {
		t.Errorf("the next navigation came with %d cookies, want 3", got)
	}
	s.download("http://site.test/file.bin", "")
	s.until("the download was asked for", func() bool { return n.came("site.test/file.bin") })
	if got := n.sent("site.test/file.bin"); got != 3 {
		t.Errorf("a download came with %d cookies, want 3", got)
	}
	s.settled()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, _ := os.ReadFile(filepath.Join(dir, file.Name()))
		if holds := strings.Contains(string(data), keptValue); holds != (file.Name() == cookiesFile) {
			t.Errorf("%s holds the cookie: %v", file.Name(), holds)
		}
		if file.Name() == cookiesFile && strings.Contains(string(data), `"run"`) {
			t.Error("the file holds a cookie of the run alone")
		}
	}
	if info, failed := os.Stat(filepath.Join(dir, cookiesFile)); failed != nil {
		t.Fatal("the cookies were not written")
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("the cookies' file has mode %o, want 600", info.Mode().Perm())
	}

	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Transport: n.transport})
	again.visit("http://site.test/next")
	if got := n.sent("site.test/next"); got != 1 {
		t.Errorf("the next run's navigation came with %d cookies, want the one that lasts", got)
	}
}

func TestAFileOfCookiesThatIsNotOneDoesNotStopLight(t *testing.T) {
	n := cookied(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, cookiesFile),
		[]byte(`{"version": 1, "cookies": [{"name": "`+keptValue), 0o600); err != nil {
		t.Fatal(err)
	}
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Transport: n.transport})
	if s.jar.Len() != 0 || s.unreadable == nil || strings.Contains(s.unreadable.Error(), keptValue) {
		t.Fatalf("a file that is not one gave %d cookies, and the reason %v", s.jar.Len(), s.unreadable != nil)
	}
	s.visit("http://site.test/")
	s.visit("http://site.test/next")
	if got := n.sent("site.test/next"); got < 2 {
		t.Errorf("after a file that is not one, a navigation came with %d cookies", got)
	}
}

func TestCookiesAreSettingsAndCleared(t *testing.T) {
	n := cookied(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.settings()
	if !s.doc.Checked(s.node("set-cookies")) || s.doc.Checked(s.node("set-third-party-cookies")) {
		t.Fatal("cookies are not on, or a third party's not off, as light starts")
	}
	s.clean()
	s.click("set-cookies")
	if s.config.Cookies || s.jar.Enabled() {
		t.Fatal("the setting did not turn the cookies off")
	}
	s.visit("http://site.test/")
	s.visit("http://site.test/next")
	if got := n.sent("site.test/next"); got != 0 || s.jar.Len() != 0 {
		t.Errorf("with cookies off a navigation came with %d, and the jar holds %d", got, s.jar.Len())
	}

	s.settings()
	s.click("set-cookies")
	s.click("set-third-party-cookies")
	if !s.jar.Enabled() || !s.config.ThirdPartyCookies || !s.jar.ThirdParty() {
		t.Fatal("the settings did not turn the cookies on again, and a third party's")
	}
	s.reloadTo("http://site.test/")
	s.reloadTo("http://site.test/next")
	if got := n.sent("site.test/next"); got != 2 && got != 3 {
		t.Errorf("with cookies on again a navigation came with %d cookies", got)
	}

	s.settings()
	s.click("set-clear-cookies")
	if s.jar.Len() != 0 {
		t.Fatalf("cleared, the jar holds %d cookies", s.jar.Len())
	}
	s.reloadTo("http://site.test/next")
	if got := n.sent("site.test/next"); got != 0 {
		t.Errorf("after the cookies were cleared a navigation came with %d", got)
	}
	s.settings()
	s.click("set-reset")
	if !s.jar.Enabled() || s.jar.ThirdParty() {
		t.Error("a reset left the cookies off, or a third party's on")
	}
}

// reloadTo shows address fetched again.
func (s *shown) reloadTo(address string) {
	s.t.Helper()
	delete(s.session().pages, address)
	s.visit(address)
}

func TestALinkFromAnotherSiteCarriesNoStrictCookie(t *testing.T) {
	n := names(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.Host + r.URL.Path {
		case "site.test/set":
			w.Header().Add("Set-Cookie", "strict=1; Path=/; SameSite=Strict")
			w.Header().Add("Set-Cookie", "lax=1; Path=/")
			_, _ = w.Write([]byte(`<title>Set</title><a id="in" href="/inside" style="display: block; height: 20px">in</a>`))
		case "other.test/":
			_, _ = w.Write([]byte(`<title>Other</title>
				<a id="out" href="http://site.test/linked" style="display: block; height: 20px">out</a>`))
		default:
			_, _ = w.Write([]byte(`<title>There</title><p>there</p>`))
		}
	})
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.visit("http://site.test/set")
	s.clickAt(s.inPage("in"))
	s.until("the link within the site was followed", func() bool { return n.came("site.test/inside") })
	if got := n.sent("site.test/inside"); got != 2 {
		t.Errorf("a link within the site came with %d cookies, want 2", got)
	}
	s.visit("http://other.test/")
	s.clickAt(s.inPage("out"))
	s.until("the link to the site was followed", func() bool { return n.came("site.test/linked") })
	if got := n.sent("site.test/linked"); got != 1 {
		t.Errorf("a link from another site came with %d cookies, want the Lax one", got)
	}
	s.visit("http://site.test/typed")
	if got := n.sent("site.test/typed"); got != 2 {
		t.Errorf("an address the player asked for came with %d cookies, want 2", got)
	}
}

func TestTheNetworkPanelListsTheCookiesARequestWasSentWith(t *testing.T) {
	n := cookied(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.toggleTools()
	s.visit("http://site.test/")
	s.pictured(n)
	s.click(s.requestRow("/"))
	if text := s.node("dt-details").Text(); !strings.Contains(text, "Set-Cookie") ||
		strings.Contains(text, "Cookierun=1") {
		t.Errorf("the page's request does not list the cookies it set, or lists one it was sent with")
	}
	s.click(s.requestRow("/site.css"))
	if text := s.node("dt-details").Text(); !strings.Contains(text, "Cookie") || !strings.Contains(text, "run=1") {
		t.Errorf("the sheet's request does not list the Cookie header it was sent with")
	}
}

func TestTheStorageTabListsWhatThePagesSiteKeeps(t *testing.T) {
	n := cookied(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.sites.Area("http://site.test").Set("theme", "dusk")
	s.sites.Area("http://other.test").Set("elsewhere", "1")
	s.toggleTools()
	s.click("dt-tab-storage")
	if text := s.node("dt-storage").Text(); !strings.Contains(text, "keeps nothing") {
		t.Errorf("for a page of light's own the tab reads %q", text)
	}
	s.visit("http://site.test/")
	s.pictured(n)
	s.visit("http://site.test/next")
	text := s.node("dt-storage").Text()
	for _, want := range []string{"Cookies", "kept", "site.test", "HttpOnly", "SameSite=lax", "run", "until the run ends",
		"Local storage", "theme", "dusk"} {
		if !strings.Contains(text, want) {
			t.Errorf("the tab lacks %q", want)
		}
	}
	if strings.Contains(text, "elsewhere") {
		t.Error("the tab lists what another site keeps")
	}
	s.clean()
}
