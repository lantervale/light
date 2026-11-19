package fetch_test

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/fetch"
)

const page = "https://site.test"

func at(t *testing.T, address string) *url.URL {
	t.Helper()
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func header(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Add(pairs[i], pairs[i+1])
	}
	return h
}

func TestOriginOf(t *testing.T) {
	for address, want := range map[string]string{
		"https://Site.Test/a?b#c":      "https://site.test",
		"https://site.test:443/":       "https://site.test",
		"https://site.test:8443/":      "https://site.test:8443",
		"http://site.test:80/":         "http://site.test",
		"http://site.test:8080/":       "http://site.test:8080",
		"http://user:pw@site.test/":    "http://site.test",
		"http://[::1]:8080/":           "http://[::1]:8080",
		"light://scripts":              "null",
		"file:///etc/passwd":           "null",
		"data:text/plain,x":            "null",
		"about:blank":                  "null",
		"https:///nohost":              "null",
		"javascript:alert(1)":          "null",
		"blob:https://site.test/1-2-3": "null",
	} {
		if got := fetch.OriginOf(at(t, address)); got != want {
			t.Errorf("the origin of %s is %q, want %q", address, got, want)
		}
	}
	if got := fetch.OriginOf(nil); got != "null" {
		t.Errorf("the origin of no address is %q", got)
	}
}

func TestWhatAScriptSeesOfAnAnswer(t *testing.T) {
	const other = "https://other.test/data"
	for _, c := range []struct {
		name        string
		origin      string
		mode        fetch.Mode
		credentials fetch.Credentials
		to          string
		crossed     bool
		answer      http.Header
		want        fetch.Kind
		why         string
	}{
		{name: "its own origin", to: page + "/data", want: fetch.Basic},
		{name: "its own origin, whatever the headers", to: page + "/data",
			answer: header("Access-Control-Allow-Origin", "https://evil.test"), want: fetch.Basic},
		{name: "its own origin in same-origin mode", mode: fetch.ModeSameOrigin, to: page + "/data", want: fetch.Basic},
		{name: "its own origin in no-cors mode", mode: fetch.ModeNoCORS, to: page + "/data", want: fetch.Basic},
		{name: "its own origin with credentials", credentials: fetch.CredentialsInclude, to: page + "/data",
			want: fetch.Basic},
		{name: "another port", to: "https://site.test:8443/data", why: "No 'Access-Control-Allow-Origin' header"},
		{name: "another scheme", to: "http://site.test/data", why: "No 'Access-Control-Allow-Origin' header"},
		{name: "a host under it", to: "https://api.site.test/data", why: "No 'Access-Control-Allow-Origin' header"},
		{name: "another origin that says nothing", to: other, why: "No 'Access-Control-Allow-Origin' header is " +
			"present on the requested resource."},
		{name: "another origin shared with all", to: other, answer: header("Access-Control-Allow-Origin", "*"),
			want: fetch.Shared},
		{name: "another origin shared with the page", to: other, answer: header("Access-Control-Allow-Origin", page),
			want: fetch.Shared},
		{name: "another origin shared with another", to: other,
			answer: header("Access-Control-Allow-Origin", "https://else.test"),
			why:    "has a value 'https://else.test' that is not equal to the supplied origin."},
		{name: "the origin in other letters", to: other, answer: header("Access-Control-Allow-Origin", "HTTPS://SITE.TEST"),
			why: "not equal to the supplied origin"},
		{name: "the origin with a slash", to: other, answer: header("Access-Control-Allow-Origin", page+"/"),
			why: "not equal to the supplied origin"},
		{name: "the origin among others", to: other, answer: header("Access-Control-Allow-Origin", page+", https://b.test"),
			why: "contains multiple values"},
		{name: "the origin said twice", to: other,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Origin", page),
			why:    "contains multiple values"},
		{name: "an empty origin", to: other, answer: header("Access-Control-Allow-Origin", ""),
			why: "not equal to the supplied origin"},
		{name: "null for a page that has an origin", to: other, answer: header("Access-Control-Allow-Origin", "null"),
			why: "not equal to the supplied origin"},
		{name: "with credentials, shared with all", credentials: fetch.CredentialsInclude, to: other,
			answer: header("Access-Control-Allow-Origin", "*", "Access-Control-Allow-Credentials", "true"),
			why:    "must not be the wildcard '*' when the request's credentials mode is 'include'."},
		{name: "with credentials, shared with the page alone", credentials: fetch.CredentialsInclude, to: other,
			answer: header("Access-Control-Allow-Origin", page),
			why:    "'Access-Control-Allow-Credentials' header in the response is '' which must be 'true'"},
		{name: "with credentials, in other letters", credentials: fetch.CredentialsInclude, to: other,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Credentials", "True"),
			why:    "is 'True' which must be 'true'"},
		{name: "with credentials, allowed", credentials: fetch.CredentialsInclude, to: other,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Credentials", "true"),
			want:   fetch.Shared},
		{name: "without credentials, shared with all", credentials: fetch.CredentialsOmit, to: other,
			answer: header("Access-Control-Allow-Origin", "*"), want: fetch.Shared},
		{name: "same-origin mode, another origin", mode: fetch.ModeSameOrigin, to: other,
			answer: header("Access-Control-Allow-Origin", "*"), why: `Request mode is "same-origin"`},
		{name: "no-cors mode, another origin", mode: fetch.ModeNoCORS, to: other, want: fetch.Opaque},
		{name: "no-cors mode, another origin that shares", mode: fetch.ModeNoCORS, to: other,
			answer: header("Access-Control-Allow-Origin", "*"), want: fetch.Opaque},
		{name: "back to its own origin through another", to: page + "/data", crossed: true,
			why: "No 'Access-Control-Allow-Origin' header"},
		{name: "back to its own origin through another, shared", to: page + "/data", crossed: true,
			answer: header("Access-Control-Allow-Origin", "*"), want: fetch.Shared},
		{name: "back to its own origin through another, no-cors", mode: fetch.ModeNoCORS, to: page + "/data",
			crossed: true, want: fetch.Opaque},
		{name: "back to its own origin through another, same-origin", mode: fetch.ModeSameOrigin, to: page + "/data",
			crossed: true, why: `Request mode is "same-origin"`},
		{name: "a page of no origin, to the web", origin: "null", to: other, why: "No 'Access-Control-Allow-Origin'"},
		{name: "a page of no origin, shared with all", origin: "null", to: other,
			answer: header("Access-Control-Allow-Origin", "*"), want: fetch.Shared},
		{name: "a page of no origin, shared with null", origin: "null", to: other,
			answer: header("Access-Control-Allow-Origin", "null"), why: "not equal to the supplied origin"},
		{name: "a page of no origin, to an address of none", origin: "null", to: "light://scripts/data.json",
			why: "No 'Access-Control-Allow-Origin'"},
		{name: "a page of no origin, same-origin mode", origin: "null", mode: fetch.ModeSameOrigin,
			to: "light://scripts/data.json", why: `Request mode is "same-origin"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := fetch.Policy{Origin: page, Mode: fetch.ModeCORS, Credentials: fetch.CredentialsSameOrigin}
			if c.origin != "" {
				p.Origin = c.origin
			}
			if c.mode != "" {
				p.Mode = c.mode
			}
			if c.credentials != "" {
				p.Credentials = c.credentials
			}
			kind, err := p.Decide(at(t, c.to), c.crossed, c.answer)
			if c.why == "" {
				if err != nil || kind != c.want {
					t.Fatalf("the script sees %q, %v; want %q", kind, err, c.want)
				}
				return
			}
			var blocked *fetch.BlockedError
			if !errors.As(err, &blocked) || kind != "" || !strings.Contains(blocked.Why, c.why) {
				t.Fatalf("the script sees %q, %v; want it blocked for %q", kind, err, c.why)
			}
			if cors := c.mode != fetch.ModeSameOrigin; blocked.CORS != cors || blocked.Sent != cors {
				t.Errorf("blocked as %+v", blocked)
			}
		})
	}
}

func TestWhichRequestsCarryCookies(t *testing.T) {
	own, other := page+"/a", "https://other.test/a"
	for _, c := range []struct {
		credentials fetch.Credentials
		to          string
		crossed     bool
		want        bool
	}{
		{fetch.CredentialsSameOrigin, own, false, true},
		{fetch.CredentialsSameOrigin, other, false, false},
		{fetch.CredentialsSameOrigin, own, true, false},
		{fetch.CredentialsSameOrigin, "https://api.site.test/a", false, false},
		{fetch.CredentialsSameOrigin, "http://site.test/a", false, false},
		{fetch.CredentialsOmit, own, false, false},
		{fetch.CredentialsOmit, other, false, false},
		{fetch.CredentialsInclude, own, false, true},
		{fetch.CredentialsInclude, other, false, true},
		{fetch.CredentialsInclude, other, true, true},
		{"", own, false, false},
		{"whatever", own, false, false},
	} {
		p := fetch.Policy{Origin: page, Mode: fetch.ModeCORS, Credentials: c.credentials}
		if got := p.Credentialed(at(t, c.to), c.crossed); got != c.want {
			t.Errorf("credentials %q to %s (crossed %v): %v, want %v", c.credentials, c.to, c.crossed, got, c.want)
		}
	}
	null := fetch.Policy{Origin: "null", Credentials: fetch.CredentialsSameOrigin}
	if null.Credentialed(at(t, "light://scripts/data"), false) {
		t.Error("a page of no origin sent cookies as to its own")
	}
}

func TestWhichRequestsArePreflighted(t *testing.T) {
	long := strings.Repeat("a", 129)
	for _, c := range []struct {
		method string
		header http.Header
		unsafe []string
	}{
		{"GET", nil, nil},
		{"HEAD", nil, nil},
		{"POST", nil, nil},
		{"PUT", nil, nil},
		{"DELETE", nil, nil},
		{"PATCH", nil, nil},
		{"OPTIONS", nil, nil},
		{"POST", header("Content-Type", "application/x-www-form-urlencoded"), nil},
		{"POST", header("Content-Type", "multipart/form-data; boundary=x"), nil},
		{"POST", header("Content-Type", "text/plain;charset=UTF-8"), nil},
		{"POST", header("Content-Type", "TEXT/PLAIN"), nil},
		{"POST", header("Content-Type", "application/json"), []string{"content-type"}},
		{"POST", header("Content-Type", "text/plain; a=\"(\""), []string{"content-type"}},
		{"POST", header("Content-Type", "nonsense"), []string{"content-type"}},
		{"POST", header("Content-Type", ""), []string{"content-type"}},
		{"GET", header("Accept", "application/json", "Accept-Language", "fr-FR, en;q=0.8", "Content-Language", "en"), nil},
		{"GET", header("Accept", "a(b)"), []string{"accept"}},
		{"GET", header("Accept-Language", "en@x"), []string{"accept-language"}},
		{"GET", header("Accept", long), []string{"accept"}},
		{"GET", header("Range", "bytes=0-99"), nil},
		{"GET", header("Range", "bytes=10-"), nil},
		{"GET", header("Range", "bytes=-10"), []string{"range"}},
		{"GET", header("Range", "bytes=0-1,5-9"), []string{"range"}},
		{"GET", header("Range", "items=0-1"), []string{"range"}},
		{"GET", header("X-Requested-With", "XMLHttpRequest"), []string{"x-requested-with"}},
		{"GET", header("Authorization", "Bearer x", "X-B", "1", "Accept", "*/*"), []string{"authorization", "x-b"}},
		{"GET", header("Cache-Control", "no-cache"), []string{"cache-control"}},
	} {
		if got := fetch.Unsafe(c.header); !slices.Equal(got, c.unsafe) {
			t.Errorf("of %v, %q need leave; want %q", c.header, got, c.unsafe)
		}
		p := fetch.Policy{Origin: page, Mode: fetch.ModeCORS}
		want := len(c.unsafe) > 0 || (c.method != "GET" && c.method != "HEAD" && c.method != "POST")
		if got := p.Preflighted(c.method, c.header); got != want {
			t.Errorf("%s with %v is preflighted: %v, want %v", c.method, c.header, got, want)
		}
		p.Mode = fetch.ModeNoCORS
		if p.Preflighted(c.method, c.header) {
			t.Errorf("a no-cors %s with %v is preflighted", c.method, c.header)
		}
	}
}

func TestThePreflightRequest(t *testing.T) {
	p := fetch.Policy{Origin: page, Mode: fetch.ModeCORS}
	got := p.Preflight("PUT", header("X-B", "1", "Content-Type", "application/json", "Accept", "text/html"))
	want := header("Accept", "*/*", "Access-Control-Request-Method", "PUT", "Origin", page,
		"Access-Control-Request-Headers", "content-type,x-b")
	if len(got) != len(want) {
		t.Fatalf("the preflight says %v, want %v", got, want)
	}
	for name := range want {
		if got.Get(name) != want.Get(name) {
			t.Errorf("the preflight says %s: %q, want %q", name, got.Get(name), want.Get(name))
		}
	}
	if simple := p.Preflight("DELETE", nil); simple.Get("Access-Control-Request-Headers") != "" || len(simple) != 3 {
		t.Errorf("the preflight of a request with no header says %v", simple)
	}
}

func TestWhatAPreflightAllows(t *testing.T) {
	json := header("Content-Type", "application/json")
	for _, c := range []struct {
		name        string
		credentials fetch.Credentials
		status      int
		answer      http.Header
		method      string
		header      http.Header
		why         string
	}{
		{name: "nothing said", status: 200, method: "PUT",
			why: "Response to preflight request doesn't pass access control check: No 'Access-Control-Allow-Origin'"},
		{name: "another origin", status: 200, answer: header("Access-Control-Allow-Origin", "https://b.test"), method: "PUT",
			why: "Response to preflight request doesn't pass access control check: The 'Access-Control-Allow-Origin' " +
				"header has a value"},
		{name: "not ok", status: 404, answer: header("Access-Control-Allow-Origin", "*", "Access-Control-Allow-Methods",
			"PUT"), method: "PUT", why: "It does not have HTTP ok status."},
		{name: "a redirect", status: 301, answer: header("Access-Control-Allow-Origin", "*", "Location", "/b"),
			method: "PUT", why: "Redirect is not allowed for a preflight request."},
		{name: "a redirect that says nothing", status: 302, method: "PUT",
			why: "Redirect is not allowed for a preflight request."},
		{name: "no content", status: 204, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Methods", "PUT"), method: "PUT"},
		{name: "the method allowed", status: 200, answer: header("Access-Control-Allow-Origin", page,
			"Access-Control-Allow-Methods", "GET, PUT ,DELETE"), method: "DELETE"},
		{name: "the method on two lines", status: 200, answer: header("Access-Control-Allow-Origin", page,
			"Access-Control-Allow-Methods", "GET", "Access-Control-Allow-Methods", "PATCH"), method: "PATCH"},
		{name: "the method not listed", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Methods", "GET, POST"), method: "PUT",
			why: "Method PUT is not allowed by Access-Control-Allow-Methods in preflight response."},
		{name: "no method listed", status: 200, answer: header("Access-Control-Allow-Origin", "*"), method: "DELETE",
			why: "Method DELETE is not allowed"},
		{name: "the method in other letters", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Methods", "put"), method: "PUT", why: "Method PUT is not allowed"},
		{name: "every method", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Methods", "*"), method: "PUT"},
		{name: "every method, with credentials", credentials: fetch.CredentialsInclude, status: 200,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Credentials", "true",
				"Access-Control-Allow-Methods", "*"), method: "PUT", why: "Method PUT is not allowed"},
		{name: "with credentials, shared with all", credentials: fetch.CredentialsInclude, status: 200,
			answer: header("Access-Control-Allow-Origin", "*", "Access-Control-Allow-Methods", "PUT"), method: "PUT",
			why: "must not be the wildcard"},
		{name: "with credentials, not allowed them", credentials: fetch.CredentialsInclude, status: 200,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Methods", "PUT"), method: "PUT",
			why: "which must be 'true'"},
		{name: "with credentials, allowed", credentials: fetch.CredentialsInclude, status: 200,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Credentials", "true",
				"Access-Control-Allow-Methods", "PUT"), method: "PUT"},
		{name: "a simple method with a header, allowed", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Headers", "X-A, Content-Type"), method: "POST", header: json},
		{name: "a header not listed", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Headers", "X-A"), method: "POST", header: json,
			why: "Request header field content-type is not allowed by Access-Control-Allow-Headers in preflight response."},
		{name: "no header listed", status: 200, answer: header("Access-Control-Allow-Origin", "*"), method: "POST",
			header: json, why: "Request header field content-type is not allowed"},
		{name: "every header", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Headers", "*"), method: "POST", header: header("X-A", "1", "X-B", "2")},
		{name: "every header, with credentials", credentials: fetch.CredentialsInclude, status: 200,
			answer: header("Access-Control-Allow-Origin", page, "Access-Control-Allow-Credentials", "true",
				"Access-Control-Allow-Headers", "*"), method: "POST", header: json,
			why: "Request header field content-type is not allowed"},
		{name: "every header but Authorization", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Headers", "*"), method: "GET", header: header("Authorization", "Bearer x"),
			why: "Request header field authorization is not allowed"},
		{name: "Authorization named", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Headers", "*, AUTHORIZATION"), method: "GET", header: header("Authorization", "Bearer x")},
		{name: "a simple header is not asked for", status: 200, answer: header("Access-Control-Allow-Origin", "*",
			"Access-Control-Allow-Methods", "PUT"), method: "PUT", header: header("Accept", "text/html")},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := fetch.Policy{Origin: page, Mode: fetch.ModeCORS, Credentials: fetch.CredentialsSameOrigin}
			if c.credentials != "" {
				p.Credentials = c.credentials
			}
			err := p.Allows(c.status, c.answer, c.method, c.header)
			if c.why == "" {
				if err != nil {
					t.Fatalf("the preflight refuses: %v", err)
				}
				return
			}
			var blocked *fetch.BlockedError
			if !errors.As(err, &blocked) || !blocked.CORS || !blocked.Sent || !strings.Contains(blocked.Why, c.why) {
				t.Fatalf("the preflight says %v, want it to refuse for %q", err, c.why)
			}
		})
	}
}

func TestTheHeadersAScriptReads(t *testing.T) {
	answer := header("Content-Type", "text/plain", "Content-Length", "3", "Cache-Control", "no-store",
		"Content-Language", "en", "Expires", "0", "Last-Modified", "then", "Pragma", "no-cache", "X-Secret", "s",
		"X-Shown", "v", "Etag", "e", "Set-Cookie", "a=b", "Set-Cookie2", "c=d", "Access-Control-Allow-Origin", "*",
		"Server", "s")
	simple := []string{"Cache-Control", "Content-Language", "Content-Length", "Content-Type", "Expires",
		"Last-Modified", "Pragma"}
	everything := append([]string{"Access-Control-Allow-Origin", "Etag", "Server", "X-Secret", "X-Shown"}, simple...)
	for _, c := range []struct {
		name        string
		kind        fetch.Kind
		credentials fetch.Credentials
		expose      []string
		want        []string
	}{
		{name: "its own origin", kind: fetch.Basic, want: everything},
		{name: "another origin", kind: fetch.Shared, want: simple},
		{name: "another origin that exposes some", kind: fetch.Shared, expose: []string{"x-shown, ETAG"},
			want: append([]string{"Etag", "X-Shown"}, simple...)},
		{name: "another origin that exposes on two lines", kind: fetch.Shared, expose: []string{"x-shown", "Etag"},
			want: append([]string{"Etag", "X-Shown"}, simple...)},
		{name: "another origin that exposes its cookies", kind: fetch.Shared, expose: []string{"Set-Cookie, Set-Cookie2"},
			want: simple},
		{name: "another origin that exposes all", kind: fetch.Shared, expose: []string{"*"},
			want: append(everything, "Access-Control-Expose-Headers")},
		{name: "another origin that exposes all, with credentials", kind: fetch.Shared,
			credentials: fetch.CredentialsInclude, expose: []string{"*"}, want: simple},
		{name: "opaque", kind: fetch.Opaque, expose: []string{"*"}},
		{name: "a redirect not followed", kind: fetch.OpaqueRedirect},
		{name: "no kind", kind: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			sent := answer.Clone()
			for _, line := range c.expose {
				sent.Add("Access-Control-Expose-Headers", line)
			}
			seen := fetch.Policy{Origin: page, Credentials: c.credentials}.Visible(c.kind, sent)
			var got []string
			for name := range seen {
				got = append(got, name)
			}
			slices.Sort(got)
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("the script reads %q, want %q", got, want)
			}
		})
	}
}

func TestMixedContent(t *testing.T) {
	for _, c := range []struct {
		origin, to string
		want       bool
	}{
		{"https://site.test", "http://site.test/a", true},
		{"https://site.test", "http://other.test/a", true},
		{"https://site.test", "http://192.168.1.1/a", true},
		{"https://site.test", "http://localhost.evil.test/a", true},
		{"https://site.test", "https://other.test/a", false},
		{"https://site.test", "http://localhost:8080/a", false},
		{"https://site.test", "http://LOCALHOST./a", false},
		{"https://site.test", "http://app.localhost/a", false},
		{"https://site.test", "http://127.0.0.1:9/a", false},
		{"https://site.test", "http://127.8.8.8/a", false},
		{"https://site.test", "http://[::1]:9/a", false},
		{"http://site.test", "http://other.test/a", false},
		{"http://site.test", "https://other.test/a", false},
		{"null", "http://other.test/a", false},
	} {
		if got := fetch.Mixed(c.origin, at(t, c.to)); got != c.want {
			t.Errorf("%s asking for %s is mixed content: %v, want %v", c.origin, c.to, got, c.want)
		}
	}
}

func TestTheMethodsAScriptSends(t *testing.T) {
	for name, want := range map[string]string{
		"GET": "GET", "get": "GET", "Head": "HEAD", "post": "POST", "pUt": "PUT", "delete": "DELETE",
		"options": "OPTIONS", "PATCH": "PATCH", "patch": "patch", "Custom": "Custom", "M-SEARCH": "M-SEARCH",
		"CONNECT": "", "connect": "", "TRACE": "", "Trace": "", "TRACK": "", "track": "",
		"": "", "GET ": "", "G ET": "", "GET\r\nX: y": "", "GÉT": "", "GET/1": "", "a(b)": "",
	} {
		got, err := fetch.Method(name)
		if got != want || (err != nil) != (want == "") || (err != nil && !errors.Is(err, fetch.ErrMethod)) {
			t.Errorf("the method %q is sent as %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestTheHeadersAScriptMayNotSet(t *testing.T) {
	for _, name := range []string{"Accept-Charset", "Accept-Encoding", "Access-Control-Request-Headers",
		"Access-Control-Request-Method", "Connection", "Content-Length", "Cookie", "Cookie2", "Date", "DNT", "Expect",
		"Host", "Keep-Alive", "Origin", "Referer", "Set-Cookie", "TE", "Trailer", "Transfer-Encoding", "Upgrade", "Via",
		"cookie", "HOST", "oRiGiN", "Proxy-Authorization", "proxy-anything", "Sec-Fetch-Site", "sec-ch-ua", "SEC-",
		"Bad Name", "Bad:Name", "", "Bad\nName"} {
		if !fetch.Forbidden(name, "x") {
			t.Errorf("a script may set %q", name)
		}
	}
	for _, name := range []string{"Accept", "Accept-Language", "Authorization", "Content-Type", "Content-Language",
		"X-Requested-With", "User-Agent", "Cache-Control", "If-None-Match", "Range", "X-Cookie", "Security", "Secx",
		"Proxied", "X-Sec-Thing"} {
		if fetch.Forbidden(name, "x") {
			t.Errorf("a script may not set %q", name)
		}
	}
	for value, want := range map[string]bool{
		"PUT": false, "DELETE, PATCH": false, "": false, "TRACE": true, "get, connect": true, " Track ": true,
		"GET,": false,
	} {
		for _, name := range []string{"X-HTTP-Method", "x-http-method-override", "X-Method-Override"} {
			if got := fetch.Forbidden(name, value); got != want {
				t.Errorf("%s: %q is kept from scripts: %v, want %v", name, value, got, want)
			}
		}
	}
}

func TestTheHeadersOfANoCORSRequest(t *testing.T) {
	got := fetch.Plain(header("Accept", "text/html", "Accept-Language", "fr", "Content-Language", "fr",
		"Content-Type", "text/plain", "X-A", "1", "Authorization", "Bearer x", "Range", "bytes=0-1",
		"Cache-Control", "no-cache"))
	var names []string
	for name := range got {
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"Accept", "Accept-Language", "Content-Language", "Content-Type"}; !slices.Equal(names, want) {
		t.Errorf("a no-cors request carries %q, want %q", names, want)
	}
	if kept := fetch.Plain(header("Content-Type", "application/json")); len(kept) != 0 {
		t.Errorf("a no-cors request carries %v", kept)
	}
}

func TestWhatTheConsoleSaysOfABlockedRequest(t *testing.T) {
	for _, c := range []struct {
		blocked fetch.BlockedError
		api     string
		want    string
	}{
		{fetch.BlockedError{Address: "https://b.test/x", Origin: page, Why: "No header.", CORS: true}, "fetch",
			"Access to fetch at 'https://b.test/x' from origin 'https://site.test' has been blocked by CORS policy: " +
				"No header."},
		{fetch.BlockedError{Address: "https://c.test/y", From: "https://b.test/x", Origin: page, Why: "No header.",
			CORS: true}, "XMLHttpRequest", "Access to XMLHttpRequest at 'https://c.test/y' (redirected from " +
			"'https://b.test/x') from origin 'https://site.test' has been blocked by CORS policy: No header."},
		{fetch.BlockedError{Address: "http://b.test/x", Page: page + "/p", Mixed: true, Why: "mixed"}, "fetch",
			"Mixed Content: The page at 'https://site.test/p' was loaded over HTTPS, but requested an insecure resource " +
				"'http://b.test/x'. This request has been blocked; the content must be served over HTTPS."},
		{fetch.BlockedError{Address: "https://b.test/x", Why: "Why."}, "fetch",
			"Fetch API cannot load https://b.test/x. Why."},
		{fetch.BlockedError{Address: "https://b.test/x", Why: "Why."}, "XMLHttpRequest",
			"XMLHttpRequest cannot load https://b.test/x. Why."},
	} {
		if got := c.blocked.Console(c.api); got != c.want {
			t.Errorf("the console says\n%s\nwant\n%s", got, c.want)
		}
		if c.blocked.Error() != c.blocked.Why {
			t.Errorf("the error says %q", c.blocked.Error())
		}
	}
}
