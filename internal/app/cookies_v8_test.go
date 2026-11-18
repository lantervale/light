//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"strings"
	"testing"
)

// cookieCount is how many cookies a text of document.cookie names: a
// failure says no more of them.
func cookieCount(text string) int { return strings.Count(text, "=") }

// scriptedCookies is a site whose page reads and sets cookies from a
// script, then fetches from its own site and from another.
func scriptedCookies(t *testing.T) *hosts {
	t.Helper()
	return names(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Host + r.URL.Path {
		case "site.test/dir/page":
			w.Header().Add("Set-Cookie", "session=1; Path=/; HttpOnly")
			w.Header().Add("Set-Cookie", "seen=1; Path=/")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Scripted</title><p id="before"></p><p id="after"></p><p id="got"></p><script>
				document.getElementById("before").textContent = document.cookie + "|" + navigator.cookieEnabled;
				document.cookie = "mine=1; path=/";
				document.cookie = "here=1";
				document.cookie = "hidden=1; HttpOnly";
				document.cookie = "session=2; path=/";
				document.getElementById("after").textContent = document.cookie;
				fetch("http://api.other.test/data.json").catch(() => {});
				fetch("/data.json").then((r) => {
					document.getElementById("got").textContent = String(r.headers.get("set-cookie")) + "|" + document.cookie;
				});</script>`))
		case "site.test/data.json", "api.other.test/data.json":
			w.Header().Add("Set-Cookie", "answered=1; Path=/")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Next</title><p>next</p>`))
		}
	})
}

func TestAPagesScriptsReadAndSetItsCookies(t *testing.T) {
	n := scriptedCookies(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.visit("http://site.test/dir/page")
	s.until("the script's answer came", func() bool { return s.pageText("got") != "" })
	if got, want := s.pageText("before"), "seen=1|true"; got != want {
		t.Errorf("before it set any the script read %d cookies, want %d", cookieCount(got), cookieCount(want))
	}
	if got, want := s.pageText("after"), "here=1; seen=1; mine=1"; got != want {
		t.Errorf("after it set its own the script read %d cookies, want %d", cookieCount(got), cookieCount(want))
	}
	if got, want := s.pageText("got"), "null|here=1; seen=1; mine=1; answered=1"; got != want {
		t.Errorf("of its fetch's answer the script read %d cookies, want %d and no Set-Cookie header",
			cookieCount(got), cookieCount(want))
	}
	if got := n.sent("site.test/data.json"); got != 3 {
		t.Errorf("the script's fetch came with %d cookies, want the 3 of its path", got)
	}
	s.until("the fetch to another site came", func() bool { return n.came("api.other.test/data.json") })
	if got := n.sent("api.other.test/data.json"); got != 0 {
		t.Errorf("the script's fetch to another site came with %d cookies", got)
	}
	s.visit("http://site.test/next")
	if got := n.sent("site.test/next"); got != 4 {
		t.Errorf("the next navigation came with %d cookies, want the server's 2, the script's and its fetch's", got)
	}
}

func TestWithCookiesOffAPagesScriptsKeepNone(t *testing.T) {
	n := scriptedCookies(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.settings()
	s.click("set-cookies")
	s.visit("http://site.test/dir/page")
	s.until("the script's answer came", func() bool { return s.pageText("got") != "" })
	if got, want := s.pageText("before")+s.pageText("after"), "|false"; got != want {
		t.Errorf("with cookies off the script read %d cookies, or that they are kept", cookieCount(got))
	}
	if s.jar.Len() != 0 {
		t.Errorf("with cookies off the jar holds %d", s.jar.Len())
	}
}
