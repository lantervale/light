//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/cookies"
)

func TestDocumentCookieFollowsTheRulesOfCookies(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{"a cookie is read back", `document.cookie = "a=1"`, "a=1"},
		{"each on its own", `document.cookie = "a=1"; document.cookie = "b=2; path=/"`, "a=1; b=2"},
		{"set again", `document.cookie = "a=1"; document.cookie = "a=2"`, "a=2"},
		{"ended by its age", `document.cookie = "a=1"; document.cookie = "a=; max-age=0"`, ""},
		{"ended by its date", `document.cookie = "a=1"; document.cookie = "a=; expires=Thu, 01 Jan 1970 00:00:00 GMT"`, ""},
		{"not ended from another path", `document.cookie = "a=1; path=/"; document.cookie = "a=; max-age=0"`, "a=1"},
		{"of another path", `document.cookie = "a=1; path=/elsewhere"`, ""},
		{"of another site", `document.cookie = "a=1; domain=other.test"`, ""},
		{"of the page's domain", `document.cookie = "a=1; domain=site.test"`, "a=1"},
		{"of a public suffix", `document.cookie = "a=1; domain=test"`, ""},
		{"kept from scripts", `document.cookie = "a=1; HttpOnly"`, ""},
		{"secure over https", `document.cookie = "a=1; Secure"`, "a=1"},
		{"for every site, not secure", `document.cookie = "a=1; SameSite=None"`, ""},
		{"a host's own", `document.cookie = "__Host-a=1; Secure; path=/"`, "__Host-a=1"},
		{"a host's own with a domain", `document.cookie = "__Host-a=1; Secure; path=/; domain=site.test"`, ""},
		{"not a text", `document.cookie = 5`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, `<script>`+c.script+`</script>`)
			if got := s.eval(`document.cookie`); got != c.want {
				t.Errorf("document.cookie reads %d cookies, want %d", strings.Count(got, "="), strings.Count(c.want, "="))
			}
			if got := s.eval(`String(navigator.cookieEnabled)`); got != "true" {
				t.Errorf("navigator.cookieEnabled is %s", got)
			}
			s.quiet()
		})
	}
}

func TestDocumentCookieIsTheBrowsersJar(t *testing.T) {
	at, _ := url.Parse(address)
	jar := cookies.New()
	jar.Store(cookies.Request{URL: at}, []string{"session=1; Path=/; HttpOnly", "seen=1; Path=/"})
	s := &scripted{t: t}
	s.start(`<script>document.cookie = "mine=1; path=/"; document.cookie = "session=2; path=/"</script>`,
		dom.Host{Cookies: jar.Page(func() *url.URL { return at })})
	if got, want := s.eval(`document.cookie`), "seen=1; mine=1"; got != want {
		t.Errorf("document.cookie reads %d cookies, want %d", strings.Count(got, "="), strings.Count(want, "="))
	}
	if jar.Header(cookies.Request{URL: at}) != "session=1; seen=1; mine=1" {
		t.Errorf("a request carries other cookies than the server's two and the script's")
	}
	jar.SetEnabled(false)
	if got := s.eval(`document.cookie = "late=1"; document.cookie + String(navigator.cookieEnabled)`); got != "false" {
		t.Errorf("with the cookies off the page reads %d cookies, or that they are kept", strings.Count(got, "="))
	}
	s.quiet()
}
