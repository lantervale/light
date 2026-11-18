package identity_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/identity"
)

func TestTheUserAgentNamesThePlatformAndLightAlone(t *testing.T) {
	for _, c := range []struct{ goos, build, want string }{
		{"darwin", "dev", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) light/0.1.0-alpha"},
		{"darwin", "", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) light/0.1.0-alpha"},
		{"windows", "v0.3.1", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) light/0.3.1"},
		{"linux", "0.2.0-beta.1", "Mozilla/5.0 (X11; Linux x86_64) light/0.2.0-beta.1"},
		{"freebsd", "1.2.3", "Mozilla/5.0 (X11; Linux x86_64) light/1.2.3"},
	} {
		got := identity.UserAgent(c.goos, identity.Version(c.build))
		if got != c.want {
			t.Errorf("on %s, build %q: %q, want %q", c.goos, c.build, got, c.want)
		}
		for _, token := range []string{"ember", "AppleWebKit", "Chrome", "Safari", "Gecko", "Firefox"} {
			if strings.Contains(got, token) {
				t.Errorf("%q names %s", got, token)
			}
		}
	}
}

func TestThePlatformScriptsRead(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "MacIntel"},
		{"darwin", "amd64", "MacIntel"},
		{"windows", "amd64", "Win32"},
		{"linux", "amd64", "Linux x86_64"},
		{"linux", "arm64", "Linux aarch64"},
	} {
		if got := identity.Platform(c.goos, c.goarch); got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestARunIsDrivenWhenItsEnvironmentSaysSo(t *testing.T) {
	for _, c := range []struct {
		environ []string
		want    bool
	}{
		{nil, false},
		{[]string{"HOME=/home/me", "EMBER_WGPU_LIB=/lib", "EMBER_CRASH_DIR=/tmp"}, false},
		{[]string{"EMBER_AUTOMATION=127.0.0.1:9000"}, true},
		{[]string{"EMBER_AUTOMATION="}, false},
		{[]string{"EMBER_HIDDEN=1"}, true},
		{[]string{"EMBER_CAPTURE=x.png"}, true},
		{[]string{"EMBER_CAPTURE_KEYS=enter:5-7"}, true},
		{[]string{"EMBER_CAPTURE_MOUSE=1,1@0"}, true},
	} {
		if got := identity.Driven(c.environ); got != c.want {
			t.Errorf("%q: driven %t, want %t", c.environ, got, c.want)
		}
	}
}

func TestLanguagesAreListedAsBrowsersListThem(t *testing.T) {
	for _, c := range []struct {
		preferred []string
		list      string
		header    string
	}{
		{[]string{"fr-FR", "en-US"}, "fr-FR fr en-US en", "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		{[]string{"en-US"}, "en-US en", "en-US,en;q=0.9"},
		{[]string{"en-US", "en-GB", "fr"}, "en-US en-GB en fr", "en-US,en-GB;q=0.9,en;q=0.8,fr;q=0.7"},
		{[]string{"fr", "fr-CA"}, "fr fr-CA", "fr,fr-CA;q=0.9"},
		{[]string{"zh-Hans-CN", "en"}, "zh-Hans-CN zh en", "zh-Hans-CN,zh;q=0.9,en;q=0.8"},
		{[]string{"fr_FR", "FR-fr", "bad tag", ""}, "fr-FR fr", "fr-FR,fr;q=0.9"},
		{nil, "en-US en", "en-US,en;q=0.9"},
		{[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}, "a b c d e f g h i j k l",
			"a,b;q=0.9,c;q=0.8,d;q=0.7,e;q=0.6,f;q=0.5,g;q=0.4,h;q=0.3,i;q=0.2,j;q=0.1,k;q=0.1,l;q=0.1"},
	} {
		list := identity.Languages(c.preferred)
		if got := strings.Join(list, " "); got != c.list {
			t.Errorf("%q: listed %q, want %q", c.preferred, got, c.list)
		}
		if got := identity.AcceptLanguage(list); got != c.header {
			t.Errorf("%q: Accept-Language %q, want %q", c.preferred, got, c.header)
		}
	}
}

func TestLanguagesAreReadFromWhatTheSystemSays(t *testing.T) {
	defaults := "(\n    \"fr-FR\",\n    \"en-US\",\n    de\n)\n"
	if got := identity.Tags(defaults); !slices.Equal(got, []string{"fr-FR", "en-US", "de"}) {
		t.Errorf("AppleLanguages: %q", got)
	}
	if got := identity.Tags("fr-CH, fr;q=0.9, en;q=0.8"); !slices.Equal(got, []string{"fr-CH", "fr", "en"}) {
		t.Errorf("a typed list: %q", got)
	}
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"LANG": "fr_FR.UTF-8"}, "fr-FR"},
		{map[string]string{"LANG": "fr_FR.UTF-8", "LC_ALL": "de_DE.UTF-8@euro"}, "de-DE"},
		{map[string]string{"LANG": "fr_FR.UTF-8", "LC_MESSAGES": "it_IT"}, "it-IT"},
		{map[string]string{"LANG": "fr_FR.UTF-8", "LANGUAGE": "pt_BR:en"}, "pt-BR en"},
		{map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "pt_BR"}, ""},
		{map[string]string{"LANG": "C.UTF-8"}, ""},
		{map[string]string{"LANG": "POSIX"}, ""},
		{map[string]string{}, ""},
	} {
		got := identity.Environment(func(name string) string { return c.env[name] })
		if strings.Join(got, " ") != c.want {
			t.Errorf("%v: %q, want %q", c.env, got, c.want)
		}
	}
}

func TestWhatEachDestinationAccepts(t *testing.T) {
	for dest, want := range map[identity.Destination]string{
		identity.Document: "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		identity.Image:    "image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5",
		identity.Style:    "text/css,*/*;q=0.1",
		identity.Script:   "*/*",
		identity.Font:     "*/*",
		identity.Empty:    "*/*",
	} {
		if got := identity.Accept(dest); got != want {
			t.Errorf("%s accepts %q, want %q", dest, got, want)
		}
		if strings.Contains(identity.Accept(dest), "avif") {
			t.Errorf("%s accepts AVIF, which light does not read", dest)
		}
	}
}

func at(t *testing.T, address string) *url.URL {
	t.Helper()
	if address == "" {
		return nil
	}
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func trail(t *testing.T, addresses string) []*url.URL {
	t.Helper()
	var out []*url.URL
	for address := range strings.FieldsSeq(addresses) {
		out = append(out, at(t, address))
	}
	return out
}

func TestTheSiteARequestComesFrom(t *testing.T) {
	for _, c := range []struct{ from, trail, want string }{
		{"", "https://site.test/", "none"},
		{"light://start", "https://site.test/", "none"},
		{"https://site.test/page", "https://site.test/next", "same-origin"},
		{"https://www.site.test/page", "https://img.site.test/a.png", "same-site"},
		{"https://site.test/page", "https://site.test:8443/a.png", "same-site"},
		{"http://site.test/page", "https://site.test/a.png", "cross-site"},
		{"https://site.test/page", "https://other.test/a.png", "cross-site"},
		{"https://a.github.io/", "https://b.github.io/x", "cross-site"},
		{"http://127.0.0.1:8000/", "http://127.0.0.1:9000/x", "same-site"},
		{"http://127.0.0.1:8000/", "http://localhost:8000/x", "cross-site"},
		{"https://site.test/page", "https://site.test/a https://other.test/b https://site.test/c", "cross-site"},
		{"https://site.test/page", "https://site.test/a https://cdn.site.test/b https://site.test/c", "same-site"},
		{"https://site.test/page", "https://cdn.site.test/a https://other.test/b https://cdn.site.test/c", "cross-site"},
		{"", "https://site.test/a https://other.test/b", "none"},
	} {
		if got := identity.Site(at(t, c.from), trail(t, c.trail)); got != c.want {
			t.Errorf("from %q through %q: %q, want %q", c.from, c.trail, got, c.want)
		}
	}
}

func TestWhereFetchMetadataIsSent(t *testing.T) {
	for address, want := range map[string]bool{
		"https://site.test/":      true,
		"http://site.test/":       false,
		"http://localhost:8080/":  true,
		"http://app.localhost/":   true,
		"http://127.0.0.1:8080/":  true,
		"http://127.8.9.1/":       true,
		"http://[::1]:8080/":      true,
		"http://192.168.1.1/":     false,
		"http://localhost.test/":  false,
		"http://notlocalhost/":    false,
		"http://127.0.0.1.test/x": false,
	} {
		if got := identity.Trustworthy(at(t, address)); got != want {
			t.Errorf("%s: trustworthy %t, want %t", address, got, want)
		}
	}
}

func TestWhatARequestSaysOfWhereItComesFrom(t *testing.T) {
	for _, c := range []struct{ from, to, want string }{
		{"https://site.test/dir/page?q=1#top", "https://site.test/api", "https://site.test/dir/page?q=1"},
		{"https://user:pw@site.test/page", "https://site.test/api", "https://site.test/page"},
		{"https://site.test/dir/page?q=1", "https://other.test/api", "https://site.test/"},
		{"https://site.test/dir/page", "https://site.test:8443/api", "https://site.test/"},
		{"http://site.test/dir/page", "https://site.test/api", "http://site.test/"},
		{"https://site.test/dir/page", "http://site.test/api", ""},
		{"https://site.test/dir/page", "http://127.0.0.1/api", ""},
		{"http://site.test/dir/page", "http://other.test/api", "http://site.test/"},
		{"light://start", "https://site.test/", ""},
		{"file:///home/me/page.html", "https://site.test/api", ""},
		{"about:blank", "https://site.test/a.woff2", ""},
		{"https://fonts.test/css2?family=A", "https://static.test/a.woff2", "https://fonts.test/"},
		{"", "https://site.test/", ""},
	} {
		if got := identity.Referrer(at(t, c.from), at(t, c.to)); got != c.want {
			t.Errorf("from %q to %q: %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

func TestTheHeadersOfEachRequest(t *testing.T) {
	const agent, languages = "Mozilla/5.0 (X11; Linux x86_64) light/0.1.0-alpha", "fr-FR,fr;q=0.9"
	who := identity.Identity{Agent: agent, Languages: languages}
	common := "Accept-Language: " + languages + "\nUser-Agent: " + agent
	for _, c := range []struct {
		name  string
		given http.Header
		q     *identity.Request
		trail string
		want  string
	}{
		{"an address typed", nil, &identity.Request{Destination: identity.Document, User: true}, "https://site.test/", `
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8
Sec-Fetch-Dest: document
Sec-Fetch-Mode: navigate
Sec-Fetch-Site: none
Sec-Fetch-User: ?1
Upgrade-Insecure-Requests: 1`},
		{"an address typed, in the clear", nil, &identity.Request{Destination: identity.Document, User: true},
			"http://site.test/", `
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8
Upgrade-Insecure-Requests: 1`},
		{"a link clicked", nil, &identity.Request{Destination: identity.Document, User: true,
			Initiator: at(t, "https://site.test/page?q=1#top")}, "https://other.test/", `
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8
Referer: https://site.test/
Sec-Fetch-Dest: document
Sec-Fetch-Mode: navigate
Sec-Fetch-Site: cross-site
Sec-Fetch-User: ?1
Upgrade-Insecure-Requests: 1`},
		{"a script's move", nil, &identity.Request{Destination: identity.Document,
			Initiator: at(t, "https://site.test/page")}, "https://site.test/next", `
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8
Referer: https://site.test/page
Sec-Fetch-Dest: document
Sec-Fetch-Mode: navigate
Sec-Fetch-Site: same-origin
Upgrade-Insecure-Requests: 1`},
		{"a navigation redirected to the clear", http.Header{"Referer": {"https://site.test/page"},
			"Sec-Fetch-Site": {"same-origin"}, "Sec-Fetch-User": {"?1"}, "Sec-Fetch-Mode": {"navigate"},
			"Sec-Fetch-Dest": {"document"}},
			&identity.Request{Destination: identity.Document, User: true, Initiator: at(t, "https://site.test/page")},
			"https://site.test/next http://site.test/plain", `
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8
Upgrade-Insecure-Requests: 1`},
		{"a sheet", nil, &identity.Request{Destination: identity.Style, Initiator: at(t, "https://site.test/page")},
			"https://cdn.site.test/site.css", `
Accept: text/css,*/*;q=0.1
Referer: https://site.test/
Sec-Fetch-Dest: style
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-site`},
		{"a picture redirected elsewhere and back", nil,
			&identity.Request{Destination: identity.Image, Initiator: at(t, "https://site.test/page")},
			"https://site.test/a.png https://other.test/b.png https://site.test/c.png", `
Accept: image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5
Referer: https://site.test/page
Sec-Fetch-Dest: image
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: cross-site`},
		{"a font, from its sheet", nil, &identity.Request{Destination: identity.Font,
			Initiator: at(t, "https://site.test/page"), Referrer: at(t, "https://cdn.test/css/site.css")},
			"https://cdn.test/fonts/a.woff2", `
Accept: */*
Referer: https://cdn.test/css/site.css
Sec-Fetch-Dest: font
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: cross-site`},
		{"a script", nil, &identity.Request{Destination: identity.Script, Initiator: at(t, "http://localhost:8080/")},
			"http://localhost:8080/app.js", `
Accept: */*
Referer: http://localhost:8080/
Sec-Fetch-Dest: script
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-origin`},
		{"a module", nil, &identity.Request{Destination: identity.Script, Mode: identity.CORS,
			Initiator: at(t, "https://site.test/")}, "https://site.test/app.mjs", `
Accept: */*
Referer: https://site.test/
Sec-Fetch-Dest: script
Sec-Fetch-Mode: cors
Sec-Fetch-Site: same-origin`},
		{"a fetch that says what it accepts, and in which language",
			http.Header{"Accept": {"application/json"}, "Accept-Language": {"de"}, "User-Agent": {"a script's"}},
			&identity.Request{Destination: identity.Empty, Initiator: at(t, "https://site.test/")},
			"https://api.test/data", `
Accept: application/json
Accept-Language: de
Referer: https://site.test/
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: cors
Sec-Fetch-Site: cross-site
User-Agent: ` + agent},
		{"a fetch of the page's origin alone", nil, &identity.Request{Destination: identity.Empty,
			Mode: identity.SameOrigin, Initiator: at(t, "https://site.test/")}, "https://site.test/data", `
Accept: */*
Referer: https://site.test/
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: same-origin
Sec-Fetch-Site: same-origin`},
		{"a download the player asked for", http.Header{"Referer": {"https://site.test/before"}},
			&identity.Request{Destination: identity.Empty, Mode: identity.NoCORS}, "https://site.test/file.zip", `
Accept: */*
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: none`},
		{"a request that is not described", http.Header{"Accept": {"text/plain"}, "Referer": {"https://site.test/"}},
			nil, "https://site.test/", `
Accept: text/plain
Referer: https://site.test/`},
	} {
		h := c.given.Clone()
		if h == nil {
			h = http.Header{}
		}
		who.Apply(h, c.q, trail(t, c.trail))
		want := strings.TrimSpace(c.want)
		if !strings.Contains(want, "User-Agent: ") {
			want += "\n" + common
		}
		if got := lines(h); got != sorted(want) {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, sorted(want))
		}
	}
}

func lines(h http.Header) string {
	var out []string
	for name, values := range h {
		out = append(out, name+": "+strings.Join(values, ", "))
	}
	slices.Sort(out)
	return strings.Join(out, "\n")
}

func sorted(text string) string {
	out := strings.Split(text, "\n")
	slices.Sort(out)
	return strings.Join(out, "\n")
}

func TestAnIdentityThatSaysNothingLeavesAHeaderAlone(t *testing.T) {
	h := http.Header{"User-Agent": {"kept"}}
	identity.Identity{}.Apply(h, nil, trail(t, "https://site.test/"))
	if got := lines(h); got != "User-Agent: kept" {
		t.Errorf("headers: %s", got)
	}
}
