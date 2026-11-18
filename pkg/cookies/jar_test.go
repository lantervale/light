package cookies_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/cookies"
)

// start is when the tests' clocks begin.
func start() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }

// ticking is a clock a test moves.
type ticking struct{ at time.Time }

func (c *ticking) now() time.Time { return c.at }

func jar() (*cookies.Jar, *ticking) {
	clock := &ticking{at: start()}
	return cookies.New(cookies.Clock(clock.now)), clock
}

func at(t *testing.T, address string) *url.URL {
	t.Helper()
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// visit is a navigation the player asked for.
func visit(t *testing.T, address string) cookies.Request {
	t.Helper()
	return cookies.Request{URL: at(t, address)}
}

// within is a request the page at top makes.
func within(t *testing.T, top, address string) cookies.Request {
	t.Helper()
	return cookies.Request{URL: at(t, address), Top: at(t, top)}
}

func TestACookieGoesWhereItsDomainAndPathSay(t *testing.T) {
	for _, tc := range []struct {
		name, from, set, to, want string
	}{
		{"host-only to its host", "https://www.site.test/", "a=1", "https://www.site.test/x", "a=1"},
		{"host-only not under its host", "https://site.test/", "a=1", "https://www.site.test/", ""},
		{"host-only not above its host", "https://www.site.test/", "a=1", "https://site.test/", ""},
		{"domain to the hosts under it", "https://www.site.test/", "a=1; Domain=site.test", "https://api.site.test/", "a=1"},
		{"domain with a leading dot", "https://www.site.test/", "a=1; Domain=.site.test", "https://site.test/", "a=1"},
		{"domain in any case", "https://www.site.test/", "a=1; Domain=SITE.test", "https://site.test/", "a=1"},
		{"domain of another site", "https://www.site.test/", "a=1; Domain=other.test", "https://other.test/", ""},
		{"domain under the host", "https://site.test/", "a=1; Domain=www.site.test", "https://www.site.test/", ""},
		{"domain ending like the host", "https://site.test/", "a=1; Domain=te.test", "https://te.test/", ""},
		{"any port", "http://site.test:8080/", "a=1", "http://site.test:9090/", "a=1"},
		{"an address in numbers", "http://127.0.0.1:8080/", "a=1", "http://127.0.0.1:9090/", "a=1"},
		{"a domain in numbers is its host only", "http://127.0.0.1/", "a=1; Domain=127.0.0.1", "http://127.0.0.1/", "a=1"},
		{"a domain in numbers of another host", "http://127.0.0.1/", "a=1; Domain=0.0.1", "http://127.0.0.1/", ""},
		{"path of the folder by default", "https://site.test/dir/page", "a=1", "https://site.test/dir/other", "a=1"},
		{"path of the folder not above", "https://site.test/dir/page", "a=1", "https://site.test/", ""},
		{"path named", "https://site.test/dir/page", "a=1; Path=/", "https://site.test/", "a=1"},
		{"path under", "https://site.test/", "a=1; Path=/dir", "https://site.test/dir/x", "a=1"},
		{"path whole names only", "https://site.test/", "a=1; Path=/dir", "https://site.test/directory", ""},
		{"path ending in a slash", "https://site.test/", "a=1; Path=/dir/", "https://site.test/dir", ""},
		{"path not from the root is the default", "https://site.test/dir/page", "a=1; Path=dir",
			"https://site.test/dir/x", "a=1"},
		{"no path at the root", "https://site.test", "a=1", "https://site.test/any", "a=1"},
		{"a value with an equals sign", "https://site.test/", "a=b=c", "https://site.test/", "a=b=c"},
		{"a value in quotes", "https://site.test/", `a="1 2"`, "https://site.test/", `a="1 2"`},
		{"spaces round the pair", "https://site.test/", "  a = 1 ; Path=/", "https://site.test/", "a=1"},
		{"no name", "https://site.test/", "=1", "https://site.test/", ""},
		{"no pair", "https://site.test/", "token", "https://site.test/", ""},
		{"a control character", "https://site.test/", "a=1\x00", "https://site.test/", ""},
		{"not the web", "file:///home/page.html", "a=1", "file:///home/page.html", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, _ := jar()
			j.Store(visit(t, tc.from), []string{tc.set})
			if got := j.Header(visit(t, tc.to)); got != tc.want {
				t.Errorf("the request carries %d cookies, want %d", count(got), count(tc.want))
			}
		})
	}
}

// count is how many cookies a header names: a failure says no more of
// them.
func count(header string) int {
	if header == "" {
		return 0
	}
	return strings.Count(header, ";") + 1
}

func TestCookiesAreSentLongestPathFirstThenOldest(t *testing.T) {
	j, clock := jar()
	j.Store(visit(t, "https://site.test/"), []string{"first=1; Path=/"})
	clock.at = clock.at.Add(time.Second)
	j.Store(visit(t, "https://site.test/"), []string{"second=2; Path=/", "deep=3; Path=/dir"})
	if got, want := j.Header(visit(t, "https://site.test/dir/page")), "deep=3; first=1; second=2"; got != want {
		t.Errorf("the cookies are sent in another order than the longest path, then the oldest")
	}
}

func TestACookieSetAgainReplacesItself(t *testing.T) {
	j, _ := jar()
	j.Store(visit(t, "https://site.test/"), []string{"a=1", "a=2", "a=3; Path=/dir"})
	if got := j.Header(visit(t, "https://site.test/")); got != "a=2" {
		t.Errorf("the root carries %d cookies, or not the last one set", count(got))
	}
	if got := j.Header(visit(t, "https://site.test/dir/")); got != "a=3; a=2" {
		t.Errorf("a cookie of another path replaced the first")
	}
}

func TestPublicSuffixes(t *testing.T) {
	for _, tc := range []struct {
		name, from, set, to string
		held                int
		sent                bool
	}{
		{"a top-level domain", "https://www.example.com/", "a=1; Domain=com", "https://other.com/", 0, false},
		{"a suffix of two labels", "https://www.example.co.uk/", "a=1; Domain=co.uk", "https://other.co.uk/", 0, false},
		{"a suffix sites are hosted under", "https://ann.github.io/", "a=1; Domain=github.io",
			"https://bob.github.io/", 0, false},
		{"the domain registered under one", "https://www.example.co.uk/", "a=1; Domain=example.co.uk",
			"https://example.co.uk/", 1, true},
		{"a host that is a suffix, for itself only", "http://localhost/", "a=1; Domain=localhost",
			"http://localhost/", 1, true},
		{"a host that is a suffix, not those under it", "https://github.io/", "a=1; Domain=github.io",
			"https://ann.github.io/", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, _ := jar()
			j.Store(visit(t, tc.from), []string{tc.set})
			if j.Len() != tc.held {
				t.Errorf("the jar holds %d cookies, want %d", j.Len(), tc.held)
			}
			if got := j.Header(visit(t, tc.to)) != ""; got != tc.sent {
				t.Errorf("the cookie is sent: %v, want %v", got, tc.sent)
			}
			for _, c := range j.For(at(t, tc.from)) {
				if isSuffix := tc.held == 1 && !tc.sent || tc.from == "http://localhost/"; c.HostOnly != isSuffix {
					t.Errorf("the cookie is its host's only: %v, want %v", c.HostOnly, isSuffix)
				}
			}
		})
	}
}

func TestSecure(t *testing.T) {
	j, _ := jar()
	j.Store(visit(t, "https://site.test/"), []string{"s=1; Secure", "p=1"})
	if got := j.Header(visit(t, "http://site.test/")); got != "p=1" {
		t.Errorf("a request over http carries %d cookies, want the one that is not Secure", count(got))
	}
	if got := j.Header(visit(t, "https://site.test/")); count(got) != 2 {
		t.Errorf("a request over https carries %d cookies, want 2", count(got))
	}
	j.Store(visit(t, "http://site.test/"), []string{"fromhttp=1; Secure"})
	if got := j.Header(visit(t, "https://site.test/")); count(got) != 2 {
		t.Errorf("a Secure cookie was set over http")
	}
	j.Store(visit(t, "http://site.test/"), []string{"s=2", "s=3; Domain=site.test"})
	if got := j.Header(visit(t, "https://site.test/")); got != "s=1; p=1" {
		t.Errorf("a page over http changed or shadowed a Secure cookie")
	}
	if got := j.Header(visit(t, "http://site.test/")); got != "p=1" {
		t.Errorf("a page over http set a cookie named as a Secure one")
	}
}

func TestHTTPOnlyIsKeptFromScripts(t *testing.T) {
	j, _ := jar()
	page := at(t, "https://site.test/")
	j.Store(visit(t, "https://site.test/"), []string{"session=1; HttpOnly", "seen=1"})
	if got := j.Script(page); got != "seen=1" {
		t.Errorf("a script reads %d cookies, want the one that is not HttpOnly", count(got))
	}
	if got := j.Header(visit(t, "https://site.test/")); count(got) != 2 {
		t.Errorf("a request carries %d cookies, want 2", count(got))
	}
	j.SetScript(page, "session=2")
	j.SetScript(page, "session=; Max-Age=0")
	j.SetScript(page, "mine=1; HttpOnly")
	if got := j.Header(visit(t, "https://site.test/")); got != "session=1; seen=1" {
		t.Errorf("a script changed, ended or set an HttpOnly cookie")
	}
	j.SetScript(page, "seen=2")
	if got := j.Script(page); got != "seen=2" {
		t.Errorf("a script did not change its own cookie")
	}
}

func TestExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, set string
		after     time.Duration
		kept      bool
	}{
		{"a session cookie", "a=1", 1000 * time.Hour, true},
		{"Max-Age not yet over", "a=1; Max-Age=60", 59 * time.Second, true},
		{"Max-Age over", "a=1; Max-Age=60", 60 * time.Second, false},
		{"Max-Age of nothing", "a=1; Max-Age=0", 0, false},
		{"Max-Age below nothing", "a=1; Max-Age=-5", 0, false},
		{"Max-Age that is no number", "a=1; Max-Age=soon", 1000 * time.Hour, true},
		{"Max-Age too long to count", "a=1; Max-Age=99999999999999999999999", time.Hour, true},
		{"Expires not yet come", "a=1; Expires=Wed, 18 Nov 2026 13:00:00 GMT", 59 * time.Minute, true},
		{"Expires come", "a=1; Expires=Wed, 18 Nov 2026 13:00:00 GMT", time.Hour, false},
		{"Expires past", "a=1; Expires=Thu, 01 Jan 1970 00:00:00 GMT", 0, false},
		{"Expires with dashes and two digits", "a=1; Expires=Wednesday, 18-Nov-26 13:00:00 GMT", time.Hour, false},
		{"Expires as C writes it", "a=1; Expires=Wed Nov 18 13:00:00 2026", time.Hour, false},
		{"Expires that is no date", "a=1; Expires=tomorrow", 1000 * time.Hour, true},
		{"Max-Age over Expires", "a=1; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=60", 30 * time.Second, true},
		{"Max-Age over a later Expires", "a=1; Max-Age=60; Expires=Wed, 18 Nov 2030 13:00:00 GMT", time.Hour, false},
		{"no longer than 400 days", "a=1; Max-Age=99999999", 401 * 24 * time.Hour, false},
		{"as long as 399 days", "a=1; Expires=Wed, 18 Nov 2099 13:00:00 GMT", 399 * 24 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, clock := jar()
			j.Store(visit(t, "https://site.test/"), []string{tc.set})
			clock.at = clock.at.Add(tc.after)
			if got := j.Header(visit(t, "https://site.test/")) != ""; got != tc.kept {
				t.Errorf("the cookie is sent: %v, want %v", got, tc.kept)
			}
		})
	}
}

func TestACookieIsEndedByItsOwnExpiry(t *testing.T) {
	j, _ := jar()
	j.Store(visit(t, "https://site.test/"), []string{"a=1; Path=/; Max-Age=3600", "b=1; Path=/"})
	j.Store(visit(t, "https://site.test/"), []string{"a=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT"})
	if got := j.Header(visit(t, "https://site.test/")); got != "b=1" || j.Len() != 1 {
		t.Errorf("the jar holds %d cookies after one was ended, want 1", j.Len())
	}
}

func TestSameSite(t *testing.T) {
	const page, other = "https://site.test/page", "https://other.test/page"
	set := []string{"lax=1", "named=1; SameSite=Lax", "strict=1; SameSite=Strict", "none=1; SameSite=None; Secure",
		"odd=1; SameSite=Sometimes"}
	followed := func(t *testing.T, from string) cookies.Request {
		t.Helper()
		return cookies.Request{URL: at(t, page), From: at(t, from)}
	}
	for _, tc := range []struct {
		name string
		ask  func(t *testing.T) cookies.Request
		want string
	}{
		{"a navigation the player asked for", func(t *testing.T) cookies.Request { return visit(t, page) },
			"lax=1; named=1; strict=1; none=1; odd=1"},
		{"a link within the site", func(t *testing.T) cookies.Request { return followed(t, "https://www.site.test/") },
			"lax=1; named=1; strict=1; none=1; odd=1"},
		{"a link from another site", func(t *testing.T) cookies.Request { return followed(t, other) },
			"lax=1; named=1; none=1; odd=1"},
		{"a link from the site over http", func(t *testing.T) cookies.Request { return followed(t, "http://site.test/") },
			"lax=1; named=1; none=1; odd=1"},
		{"a link from a page of light's own", func(t *testing.T) cookies.Request { return followed(t, "light://home") },
			"lax=1; named=1; strict=1; none=1; odd=1"},
		{"what the site's own page asks for",
			func(t *testing.T) cookies.Request { return within(t, "https://www.site.test/", page) },
			"lax=1; named=1; strict=1; none=1; odd=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, _ := jar()
			j.Store(visit(t, page), set)
			if got := j.Header(tc.ask(t)); got != tc.want {
				t.Errorf("the request carries %d cookies, want %d", count(got), count(tc.want))
			}
		})
	}
	t.Run("None must be Secure", func(t *testing.T) {
		j, _ := jar()
		j.Store(visit(t, page), []string{"none=1; SameSite=None"})
		if j.Len() != 0 {
			t.Error("a SameSite=None cookie that is not Secure was kept")
		}
	})
}

func TestThirdParties(t *testing.T) {
	const top, tracker = "https://site.test/", "https://tracker.test/pixel"
	set := []string{"lax=1", "strict=1; SameSite=Strict", "none=1; SameSite=None; Secure"}

	t.Run("blocked by default", func(t *testing.T) {
		j, _ := jar()
		j.Store(visit(t, tracker), set)
		if got := j.Header(within(t, top, tracker)); got != "" {
			t.Errorf("a request to a third party carries %d cookies", count(got))
		}
		j.Store(within(t, top, tracker), []string{"set=1; SameSite=None; Secure"})
		if j.Len() != len(set) {
			t.Error("a third party set a cookie")
		}
	})
	t.Run("allowed by the setting, SameSite=None only", func(t *testing.T) {
		j, _ := jar()
		j.SetThirdParty(true)
		j.Store(visit(t, tracker), set)
		if got := j.Header(within(t, top, tracker)); got != "none=1" {
			t.Errorf("a request to a third party carries %d cookies, want the SameSite=None one", count(got))
		}
		j.Store(within(t, top, tracker), []string{"more=1", "open=1; SameSite=None; Secure"})
		if got := j.Header(within(t, top, tracker)); got != "none=1; open=1" {
			t.Errorf("a third party set a cookie that is not SameSite=None, or could not set one that is")
		}
		if got := j.Header(visit(t, tracker)); count(got) != 4 {
			t.Errorf("the third party's own page carries %d cookies, want its 3 and the one it set as a third party",
				count(got))
		}
	})
	t.Run("another host of the page's site is no third party", func(t *testing.T) {
		j, _ := jar()
		j.Store(within(t, "https://www.site.test/", "https://static.site.test/a.css"), []string{"a=1; Domain=site.test"})
		if got := j.Header(within(t, "https://www.site.test/", "https://api.site.test/")); got != "a=1" {
			t.Errorf("a request within the page's site carries %d cookies, want 1", count(got))
		}
	})
	t.Run("what a page of light's own asks of the web is a third party's", func(t *testing.T) {
		j, _ := jar()
		j.Store(visit(t, top), []string{"a=1"})
		if got := j.Header(within(t, "light://scripts", top)); got != "" {
			t.Errorf("a request of a page of light's own carries %d cookies", count(got))
		}
	})
}

func TestPrefixes(t *testing.T) {
	for _, tc := range []struct {
		name, from, set string
		kept            bool
	}{
		{"__Secure- with Secure", "https://site.test/", "__Secure-a=1; Secure", true},
		{"__Secure- without Secure", "https://site.test/", "__Secure-a=1", false},
		{"__Secure- over http", "http://site.test/", "__Secure-a=1; Secure", false},
		{"__secure- in another case", "https://site.test/", "__SECURE-a=1", false},
		{"__Host- as it must be", "https://site.test/", "__Host-a=1; Secure; Path=/", true},
		{"__Host- without Secure", "https://site.test/", "__Host-a=1; Path=/", false},
		{"__Host- with a domain", "https://site.test/", "__Host-a=1; Secure; Path=/; Domain=site.test", false},
		{"__Host- with another path", "https://site.test/dir/", "__Host-a=1; Secure; Path=/dir", false},
		{"__Host- with no path", "https://site.test/", "__Host-a=1; Secure", false},
		{"__Host- over http", "http://site.test/", "__Host-a=1; Secure; Path=/", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, _ := jar()
			j.Store(visit(t, tc.from), []string{tc.set})
			if got := j.Len() == 1; got != tc.kept {
				t.Errorf("the cookie is kept: %v, want %v", got, tc.kept)
			}
		})
	}
}

func TestLimits(t *testing.T) {
	t.Run("a cookie's size", func(t *testing.T) {
		j, _ := jar()
		j.Store(visit(t, "https://site.test/"), []string{
			"fits=" + strings.Repeat("x", cookies.MaxSize-len("fits")),
			"over=" + strings.Repeat("x", cookies.MaxSize-len("over")+1),
		})
		if j.Len() != 1 {
			t.Errorf("the jar holds %d cookies, want the one within %d bytes", j.Len(), cookies.MaxSize)
		}
	})
	t.Run("an attribute's size", func(t *testing.T) {
		j, _ := jar()
		j.Store(visit(t, "https://site.test/dir/page"), []string{"a=1; Path=/" + strings.Repeat("x", cookies.MaxAttribute)})
		if got := j.Header(visit(t, "https://site.test/dir/other")); got != "a=1" {
			t.Error("a path over the size of an attribute was not left for the default one")
		}
	})
	t.Run("a site's cookies", func(t *testing.T) {
		j, clock := jar()
		for i := range cookies.MaxPerSite {
			j.Store(visit(t, "https://www.site.test/"), []string{fmt.Sprintf("c%d=1; Domain=site.test", i)})
			clock.at = clock.at.Add(time.Second)
		}
		j.Header(visit(t, "https://www.site.test/"))
		clock.at = clock.at.Add(time.Second)
		j.Store(visit(t, "https://other.test/"), []string{"kept=1"})
		j.Store(visit(t, "https://api.site.test/"), []string{"new=1; Max-Age=60", "newer=1"})
		if got := j.Len(); got != cookies.MaxPerSite+1 {
			t.Errorf("the jar holds %d cookies, want %d for the site and 1 for another", got, cookies.MaxPerSite+1)
		}
		got := j.Header(visit(t, "https://api.site.test/"))
		if !strings.HasSuffix(got, "; new=1; newer=1") || strings.HasPrefix(got, "c0=") || !strings.HasPrefix(got, "c2=") {
			t.Error("the cookies that went are not the two least lately sent")
		}
	})
	t.Run("the jar's cookies", func(t *testing.T) {
		j, clock := jar()
		for i := range cookies.MaxTotal + 5 {
			j.Store(visit(t, fmt.Sprintf("https://site%d.test/", i)), []string{"a=1"})
			clock.at = clock.at.Add(time.Second)
		}
		if got := j.Len(); got != cookies.MaxTotal {
			t.Errorf("the jar holds %d cookies, want %d", got, cookies.MaxTotal)
		}
		if j.Header(visit(t, "https://site0.test/")) != "" || j.Header(visit(t, "https://site5.test/")) == "" {
			t.Error("the oldest cookies are not the ones that went")
		}
	})
}

func TestAJarThatIsOffKeepsWhatItHolds(t *testing.T) {
	j, _ := jar()
	page := at(t, "https://site.test/")
	j.Store(visit(t, "https://site.test/"), []string{"a=1"})
	j.SetEnabled(false)
	j.Store(visit(t, "https://site.test/"), []string{"b=1"})
	j.SetScript(page, "c=1")
	if j.Header(visit(t, "https://site.test/")) != "" || j.Script(page) != "" || j.Enabled() {
		t.Error("a jar that is off gives cookies")
	}
	j.SetEnabled(true)
	if got := j.Header(visit(t, "https://site.test/")); got != "a=1" {
		t.Errorf("the jar carries %d cookies once on again, want the one it held", count(got))
	}
}

func TestClear(t *testing.T) {
	j, _ := jar()
	j.Store(visit(t, "https://site.test/"), []string{"a=1; Max-Age=60", "b=1"})
	j.Clear()
	if j.Len() != 0 || !j.Changed() {
		t.Errorf("a cleared jar holds %d cookies, changed %v", j.Len(), j.Changed())
	}
}

func TestForListsThePagesCookies(t *testing.T) {
	j, _ := jar()
	j.Store(visit(t, "https://site.test/"), []string{
		"a=1; HttpOnly; Secure; Max-Age=60; SameSite=Strict", "b=1; Path=/dir"})
	j.Store(visit(t, "https://other.test/"), []string{"c=1"})
	got := j.For(at(t, "https://site.test/"))
	if len(got) != 1 {
		t.Fatalf("the page has %d cookies, want 1", len(got))
	}
	c := got[0]
	if c.Name != "a" || !c.HTTPOnly || !c.Secure || !c.HostOnly || c.Domain != "site.test" || c.Path != "/" ||
		c.SameSite != cookies.Strict || !c.Expires.Equal(start().Add(time.Minute)) {
		t.Errorf("the cookie is listed with other attributes than it was set with")
	}
}

func TestANilJarIsNone(t *testing.T) {
	var j *cookies.Jar
	j.Store(visit(t, "https://site.test/"), []string{"a=1"})
	j.SetScript(at(t, "https://site.test/"), "a=1")
	if j.Header(visit(t, "https://site.test/")) != "" || j.Script(at(t, "https://site.test/")) != "" || j.Enabled() ||
		j.For(at(t, "https://site.test/")) != nil {
		t.Error("no jar gives cookies")
	}
}
