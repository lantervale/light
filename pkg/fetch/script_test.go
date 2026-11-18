package fetch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

// origin is a server that notes what it is asked, each request a line:
// its method and its path, then what it carries of the headers a
// browser or a script sets, then its body.
type origin struct {
	*httptest.Server
	mu    sync.Mutex
	asked []string
}

func (o *origin) note(r *http.Request) {
	line := r.Method + " " + r.URL.RequestURI()
	for _, name := range []string{"Origin", "Referer", "Cookie", "Content-Type", "Authorization", "X-A",
		"Access-Control-Request-Method", "Access-Control-Request-Headers", "Sec-Fetch-Site", "Proxy-Authorization"} {
		if value := r.Header.Get(name); value != "" {
			line += " " + name + "=" + value
		}
	}
	if body, _ := io.ReadAll(r.Body); len(body) > 0 {
		line += fmt.Sprintf(" body=%q", body)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.asked = append(o.asked, line)
}

// took is what the server was asked since took last told.
func (o *origin) took() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	asked := o.asked
	o.asked = nil
	return asked
}

// serve is an origin that answers with handle.
func serve(t *testing.T, handle http.HandlerFunc) *origin {
	t.Helper()
	o := &origin{}
	o.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.note(r)
		handle(w, r)
	}))
	t.Cleanup(o.Close)
	return o
}

// elsewhere is the server at address as another site's: by its name
// where address has its number.
func elsewhere(address string) string { return strings.Replace(address, "127.0.0.1", "localhost", 1) }

// browser is a client with a jar and a log, and the page whose scripts
// ask.
type browser struct {
	t      *testing.T
	client *fetch.Client
	jar    *cookies.Jar
	log    *netlog.Log
	page   *url.URL
}

func browse(t *testing.T, page string) *browser {
	t.Helper()
	jar := cookies.New()
	return &browser{t: t, jar: jar, log: netlog.New(100), page: at(t, page),
		client: fetch.New(&cookies.Transport{Jar: jar, Base: &netlog.Transport{}}, fetch.Agent("light/test"))}
}

func (b *browser) ctx() context.Context {
	return cookies.Within(netlog.With(b.t.Context(), b.log, "app.js:1", netlog.XHR), b.page)
}

func (b *browser) call(c fetch.Call) (*fetch.Result, error) {
	c.Page = b.page
	return b.client.Script(b.ctx(), c)
}

// blocked is the call kept from the script, the test failed when it
// was not.
func (b *browser) blocked(c fetch.Call) *fetch.BlockedError {
	b.t.Helper()
	result, err := b.call(c)
	var blocked *fetch.BlockedError
	if !errors.As(err, &blocked) {
		b.t.Fatalf("%s %s gave %+v, %v; want it blocked", c.Method, c.Address, result, err)
	}
	return blocked
}

// seen is the call's answer, the test failed when there is none.
func (b *browser) seen(c fetch.Call) *fetch.Result {
	b.t.Helper()
	result, err := b.call(c)
	if err != nil {
		b.t.Fatalf("%s %s: %v", c.Method, c.Address, err)
	}
	return result
}

func same(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n%s\nwant\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func echo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Secret", "s")
	w.Header().Add("Set-Cookie", "set=1; Path=/elsewhere")
	_, _ = io.WriteString(w, "took "+r.Method)
}

// sharing answers as an origin that shares with the one that asks,
// credentials and all, and allows what a preflight asks for.
func sharing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
		w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		w.Header().Set("Access-Control-Max-Age", "0")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	echo(w, r)
}

func TestAScriptSendsEveryMethodToItsOwnOrigin(t *testing.T) {
	site := serve(t, echo)
	b := browse(t, site.URL+"/dir/page?q=1#top")
	referer := " Referer=" + site.URL + "/dir/page?q=1"
	for _, c := range []struct {
		method, body string
		want         string
	}{
		{"GET", "", "GET /api" + referer},
		{"get", "ignored", "GET /api" + referer},
		{"HEAD", "", "HEAD /api" + referer},
		{"POST", "a=1&b=2", "POST /api Origin=" + site.URL + referer + ` Content-Type=text/plain body="a=1&b=2"`},
		{"PUT", "\x00\xff\r\n", "PUT /api Origin=" + site.URL + referer + ` Content-Type=text/plain body="\x00\xff\r\n"`},
		{"PATCH", "{}", "PATCH /api Origin=" + site.URL + referer + ` Content-Type=text/plain body="{}"`},
		{"DELETE", "", "DELETE /api Origin=" + site.URL + referer + ` Content-Type=text/plain`},
		{"OPTIONS", "", "OPTIONS /api Origin=" + site.URL + referer + ` Content-Type=text/plain`},
		{"patch", "x", "patch /api Origin=" + site.URL + referer + ` Content-Type=text/plain body="x"`},
	} {
		result := b.seen(fetch.Call{Method: c.method, Address: site.URL + "/api", Body: []byte(c.body),
			Header: http.Header{"Content-Type": {"text/plain"}}})
		want := c.want
		if strings.HasPrefix(want, "GET") || strings.HasPrefix(want, "HEAD") {
			want += " Content-Type=text/plain"
		}
		same(t, c.method, site.took(), []string{want})
		if result.Kind != fetch.Basic || result.Status != http.StatusOK || result.Redirected ||
			result.Header.Get("X-Secret") != "s" || result.Header.Get("Set-Cookie") != "" ||
			result.Address != site.URL+"/api" {
			t.Errorf("%s: the script sees %+v", c.method, result)
		}
		if body := "took " + strings.ToUpper(c.method); c.method != "HEAD" && c.method != "patch" &&
			string(result.Body) != body {
			t.Errorf("%s: the script reads %q, want %q", c.method, result.Body, body)
		}
	}
}

func TestAScriptSendsNoForbiddenMethodAndNoForbiddenHeader(t *testing.T) {
	site := serve(t, echo)
	b := browse(t, site.URL+"/page")
	for _, method := range []string{"CONNECT", "TRACE", "TRACK", "trace", "BAD METHOD", ""} {
		if blocked := b.blocked(fetch.Call{Method: method, Address: site.URL + "/api"}); blocked.Sent ||
			blocked.Why != "'"+method+"' HTTP method is unsupported." {
			t.Errorf("%q was blocked as %+v", method, blocked)
		}
	}
	for _, address := range []string{"ftp://site.test/a", "file:///etc/passwd", "data:text/plain,x", "/relative", ":",
		"http://user:pw@" + strings.TrimPrefix(site.URL, "http://") + "/api"} {
		if blocked := b.blocked(fetch.Call{Method: "GET", Address: address}); blocked.Sent {
			t.Errorf("%s was blocked as %+v", address, blocked)
		}
	}
	for _, c := range []fetch.Call{
		{Mode: "navigate"}, {Credentials: "all"}, {Redirect: "maybe"}, {Mode: fetch.ModeNoCORS, Method: "PUT"},
	} {
		c.Address = site.URL + "/api"
		if c.Method == "" {
			c.Method = "POST"
		}
		if blocked := b.blocked(c); blocked.Sent {
			t.Errorf("%+v was blocked as %+v", c, blocked)
		}
	}
	same(t, "what was not to be sent", site.took(), nil)

	b.seen(fetch.Call{Method: "POST", Address: site.URL + "/api", Header: http.Header{
		"Cookie": {"stolen=1"}, "Host": {"evil.test"}, "Origin": {"https://evil.test"}, "Referer": {"https://evil.test/"},
		"Sec-Fetch-Site": {"same-origin"}, "Proxy-Authorization": {"x"}, "Content-Length": {"99"},
		"X-A": {"kept"}, "Authorization": {"Bearer t"}, "User-Agent": {"script/1"},
	}, Body: []byte("b")})
	same(t, "the headers sent", site.took(), []string{"POST /api Origin=" + site.URL + " Referer=" + site.URL +
		`/page Authorization=Bearer t X-A=kept body="b"`})
}

func TestAScriptReadsAnotherOriginOnlyWhenItShares(t *testing.T) {
	site := serve(t, echo)
	open := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "X-Shown")
		w.Header().Set("X-Shown", "v")
		echo(w, r)
	})
	closed := serve(t, echo)
	b := browse(t, site.URL+"/dir/page")
	said := " Origin=" + site.URL + " Referer=" + site.URL + "/"

	for _, method := range []string{"GET", "POST"} {
		result := b.seen(fetch.Call{Method: method, Address: open.URL + "/api", Body: []byte("x")})
		if result.Kind != fetch.Shared || string(result.Body) != "took "+method || result.Header.Get("X-Shown") != "v" ||
			result.Header.Get("X-Secret") != "" || result.Header.Get("Set-Cookie") != "" ||
			result.Header.Get("Content-Type") == "" || result.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: the script sees %+v", method, result)
		}
		blocked := b.blocked(fetch.Call{Method: method, Address: closed.URL + "/api", Body: []byte("x")})
		if !blocked.CORS || !blocked.Sent || blocked.Address != closed.URL+"/api" || blocked.From != "" ||
			blocked.Origin != site.URL || blocked.Why != "No 'Access-Control-Allow-Origin' header is present on the "+
			"requested resource." {
			t.Errorf("%s: blocked as %+v", method, blocked)
		}
	}
	want := []string{"GET /api" + said, "POST /api" + said + ` body="x"`}
	same(t, "the origin that shares was asked", open.took(), want)
	same(t, "the origin that does not was asked all the same", closed.took(), want)
	if b.jar.Len() != 0 {
		t.Errorf("the jar kept %d cookies of other origins", b.jar.Len())
	}
}

func TestAScriptAsksAnotherOriginFirst(t *testing.T) {
	site := serve(t, echo)
	allowed := true
	other := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions && !allowed {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			return
		}
		sharing(w, r)
	})
	b := browse(t, site.URL+"/page")
	b.jar.Store(cookies.Request{URL: at(t, other.URL+"/")}, []string{"session=1"})
	said := " Origin=" + site.URL + " Referer=" + site.URL + "/"
	json := http.Header{"Content-Type": {"application/json"}, "X-A": {"1"}}

	result := b.seen(fetch.Call{Method: "PUT", Address: other.URL + "/api", Header: json, Body: []byte("{}"),
		Credentials: fetch.CredentialsInclude})
	if result.Kind != fetch.Shared || string(result.Body) != "took PUT" {
		t.Errorf("the script sees %+v", result)
	}
	same(t, "a request that is not simple", other.took(), []string{
		"OPTIONS /api" + said + " Access-Control-Request-Method=PUT Access-Control-Request-Headers=content-type,x-a",
		"PUT /api" + said + ` Cookie=session=1 Content-Type=application/json X-A=1 body="{}"`,
	})

	b.seen(fetch.Call{Method: "POST", Address: other.URL + "/api", Header: json, Body: []byte("{}")})
	same(t, "a simple method with a header that is not", other.took(), []string{
		"OPTIONS /api" + said + " Access-Control-Request-Method=POST Access-Control-Request-Headers=content-type,x-a",
		"POST /api" + said + ` Content-Type=application/json X-A=1 body="{}"`,
	})

	b.seen(fetch.Call{Method: "POST", Address: other.URL + "/api", Body: []byte("a=1"),
		Header: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}})
	same(t, "a simple request", other.took(), []string{
		"POST /api" + said + ` Content-Type=application/x-www-form-urlencoded body="a=1"`})

	b.seen(fetch.Call{Method: "DELETE", Address: site.URL + "/api", Header: json})
	same(t, "its own origin is not asked first", site.took(), []string{
		"DELETE /api Origin=" + site.URL + " Referer=" + site.URL + "/page Cookie=session=1 Content-Type=application/json " +
			"X-A=1"})

	allowed = false
	blocked := b.blocked(fetch.Call{Method: "DELETE", Address: other.URL + "/api"})
	if !blocked.CORS || !blocked.Sent || blocked.Why != "Method DELETE is not allowed by Access-Control-Allow-Methods "+
		"in preflight response." {
		t.Errorf("blocked as %+v", blocked)
	}
	blocked = b.blocked(fetch.Call{Method: "POST", Address: other.URL + "/api", Header: json})
	if blocked.Why != "Request header field content-type is not allowed by Access-Control-Allow-Headers in preflight "+
		"response." {
		t.Errorf("blocked as %+v", blocked)
	}
	same(t, "a request the preflight refuses is not sent", other.took(), []string{
		"OPTIONS /api" + said + " Access-Control-Request-Method=DELETE",
		"OPTIONS /api" + said + " Access-Control-Request-Method=POST Access-Control-Request-Headers=content-type,x-a",
	})

	rows := b.log.Rows()
	if first := rows[0]; first.Method != http.MethodOptions || first.Type != netlog.Preflight ||
		first.Kind() != "preflight" || rows[1].Method != http.MethodPut || rows[1].Type != netlog.XHR ||
		first.Initiator != "app.js:1" {
		t.Errorf("the log lists %+v then %+v", rows[0], rows[1])
	}
}

func TestAPreflightIsRememberedForAsLongAsItSays(t *testing.T) {
	site := serve(t, echo)
	age := "60"
	other := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "PUT, DELETE")
		if age != "" {
			w.Header().Set("Access-Control-Max-Age", age)
		}
	})
	b := browse(t, site.URL+"/page")
	methods := func(asked []string) string {
		var out []string
		for _, line := range asked {
			method, _, _ := strings.Cut(line, " ")
			out = append(out, method)
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		age, method, path string
		header            http.Header
		want              string
	}{
		{"60", "PUT", "/a", nil, "OPTIONS PUT"},
		{"60", "PUT", "/a", nil, "PUT"},
		{"60", "DELETE", "/a", nil, "DELETE"},
		{"60", "PUT", "/a", http.Header{"X-A": {"1"}}, "OPTIONS"},
		{"60", "PUT", "/b", nil, "OPTIONS PUT"},
		{"0", "PUT", "/c", nil, "OPTIONS PUT"},
		{"0", "PUT", "/c", nil, "OPTIONS PUT"},
		{"-5", "PUT", "/d", nil, "OPTIONS PUT"},
		{"-5", "PUT", "/d", nil, "OPTIONS PUT"},
		{"", "PUT", "/e", nil, "OPTIONS PUT"},
		{"", "PUT", "/e", nil, "PUT"},
	} {
		age = c.age
		_, _ = b.call(fetch.Call{Method: c.method, Address: other.URL + c.path, Header: c.header})
		if got := methods(other.took()); got != c.want {
			t.Errorf("%s %s (max-age %q) asked %q, want %q", c.method, c.path, c.age, got, c.want)
		}
	}
	_, _ = b.call(fetch.Call{Method: "PUT", Address: other.URL + "/a", Credentials: fetch.CredentialsInclude})
	if got := methods(other.took()); got != "OPTIONS" {
		t.Errorf("a preflight without credentials allowed a request with them: %q", got)
	}
}

func TestTheCookiesAScriptsRequestCarriesAndKeeps(t *testing.T) {
	site := serve(t, echo)
	other := serve(t, sharing)
	away := elsewhere(other.URL)
	for _, c := range []struct {
		name        string
		credentials fetch.Credentials
		to          string
		third       bool
		method      string
		sent, kept  bool
	}{
		{name: "its own origin", to: site.URL, method: "POST", sent: true, kept: true},
		{name: "its own origin, omit", credentials: fetch.CredentialsOmit, to: site.URL, method: "POST"},
		{name: "its own origin, include", credentials: fetch.CredentialsInclude, to: site.URL, method: "GET", sent: true,
			kept: true},
		{name: "another origin of its site", to: other.URL, method: "POST"},
		{name: "another origin of its site, omit", credentials: fetch.CredentialsOmit, to: other.URL, method: "GET"},
		{name: "another origin of its site, include", credentials: fetch.CredentialsInclude, to: other.URL,
			method: "POST", sent: true, kept: true},
		{name: "another site", to: away, method: "POST"},
		{name: "another site, include", credentials: fetch.CredentialsInclude, to: away, method: "POST"},
		{name: "another site, include, third parties allowed", credentials: fetch.CredentialsInclude, to: away,
			third: true, method: "POST"},
		{name: "another site, third parties allowed", to: away, third: true, method: "GET"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := browse(t, site.URL+"/page")
			b.jar.SetThirdParty(c.third)
			to := at(t, c.to+"/api")
			b.jar.Store(cookies.Request{URL: to}, []string{"lax=1", "strict=1; SameSite=Strict", "none=1; SameSite=None"})
			held := b.jar.Len()
			if _, err := b.call(fetch.Call{Method: c.method, Address: to.String(), Credentials: c.credentials}); err != nil {
				t.Fatal(err)
			}
			asked := strings.Join(append(site.took(), other.took()...), "\n")
			if sent := strings.Contains(asked, "Cookie=lax=1; strict=1"); sent != c.sent {
				t.Errorf("the request carried cookies: %v, want %v\n%s", sent, c.sent, asked)
			}
			if !c.sent && strings.Contains(asked, "Cookie=") {
				t.Errorf("the request carried a cookie:\n%s", asked)
			}
			if kept := b.jar.Len() > held; kept != c.kept {
				t.Errorf("the cookie the answer sets was kept: %v, want %v", kept, c.kept)
			}
		})
	}
}

func TestAnOriginSharedWithAllIsNotReadWithCredentials(t *testing.T) {
	site := serve(t, echo)
	other := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.URL.Path == "/flagged" {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		echo(w, r)
	})
	b := browse(t, site.URL+"/page")
	b.jar.Store(cookies.Request{URL: at(t, other.URL+"/")}, []string{"session=1"})
	for _, path := range []string{"/api", "/flagged"} {
		blocked := b.blocked(fetch.Call{Method: "GET", Address: other.URL + path, Credentials: fetch.CredentialsInclude})
		if !blocked.CORS || !strings.Contains(blocked.Why, "must not be the wildcard '*'") {
			t.Errorf("blocked as %+v", blocked)
		}
	}
	if result := b.seen(fetch.Call{Method: "GET", Address: other.URL + "/api"}); result.Kind != fetch.Shared {
		t.Errorf("without credentials the script sees %+v", result)
	}
	said := " Origin=" + site.URL + " Referer=" + site.URL + "/"
	same(t, "the cookies went with the first two alone", other.took(), []string{
		"GET /api" + said + " Cookie=session=1", "GET /flagged" + said + " Cookie=session=1", "GET /api" + said})
}

func TestANoCORSRequestIsOpaque(t *testing.T) {
	site := serve(t, echo)
	other := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		echo(w, r)
	})
	b := browse(t, site.URL+"/page")
	result := b.seen(fetch.Call{Method: "POST", Address: other.URL + "/collect", Mode: fetch.ModeNoCORS,
		Header: http.Header{"Content-Type": {"application/json"}, "X-A": {"1"}, "Accept-Language": {"fr"}},
		Body:   []byte("{}")})
	if result.Kind != fetch.Opaque || result.Status != 0 || len(result.Header) != 0 || len(result.Body) != 0 ||
		result.Address != "" || result.Redirected {
		t.Errorf("the script sees %+v", result)
	}
	same(t, "a no-cors request", other.took(), []string{
		"POST /collect Origin=" + site.URL + " Referer=" + site.URL + `/ body="{}"`})
	unsent := b.blocked(fetch.Call{Method: "DELETE", Address: other.URL + "/collect", Mode: fetch.ModeNoCORS})
	if unsent.Sent || len(other.took()) != 0 {
		t.Errorf("a no-cors DELETE was blocked as %+v", unsent)
	}
	own := b.seen(fetch.Call{Method: "GET", Address: site.URL + "/api", Mode: fetch.ModeNoCORS})
	if own.Kind != fetch.Basic || string(own.Body) != "took GET" {
		t.Errorf("of its own origin the script sees %+v", own)
	}
	blocked := b.blocked(fetch.Call{Method: "GET", Address: other.URL + "/api", Mode: fetch.ModeSameOrigin})
	if blocked.Sent || blocked.CORS || len(other.took()) != 0 {
		t.Errorf("a same-origin request to another origin was blocked as %+v", blocked)
	}
}

// hopping is a site that redirects /to/<status>/<address> to that
// address, and answers the rest as answer does.
func hopping(t *testing.T, answer http.HandlerFunc) *origin {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		var status int
		var to string
		if n, _ := fmt.Sscanf(r.URL.Path, "/to/%d/", &status); n == 1 {
			to = r.URL.Query().Get("u")
			if r.URL.Query().Get("share") != "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Location", to)
			w.WriteHeader(status)
			return
		}
		answer(w, r)
	})
}

func hop(from *origin, status int, to string, share bool) string {
	address := fmt.Sprintf("%s/to/%d/?u=%s", from.URL, status, url.QueryEscape(to))
	if share {
		address += "&share=1"
	}
	return address
}

func methodsOf(asked []string) string {
	var out []string
	for _, line := range asked {
		parts := strings.SplitN(line, " ", 3)
		out = append(out, parts[0]+" "+strings.SplitN(parts[1], "?", 2)[0])
	}
	return strings.Join(out, ", ")
}

func TestAScriptsRequestFollowsItsRedirects(t *testing.T) {
	site := hopping(t, echo)
	b := browse(t, site.URL+"/page")
	for _, c := range []struct {
		method string
		status int
		want   string
	}{
		{"POST", 301, "POST /to/301/, GET /landing"},
		{"POST", 302, "POST /to/302/, GET /landing"},
		{"POST", 303, "POST /to/303/, GET /landing"},
		{"POST", 307, "POST /to/307/, POST /landing"},
		{"POST", 308, "POST /to/308/, POST /landing"},
		{"PUT", 301, "PUT /to/301/, PUT /landing"},
		{"PUT", 303, "PUT /to/303/, GET /landing"},
		{"DELETE", 302, "DELETE /to/302/, DELETE /landing"},
		{"HEAD", 303, "HEAD /to/303/, HEAD /landing"},
		{"GET", 302, "GET /to/302/, GET /landing"},
	} {
		result := b.seen(fetch.Call{Method: c.method, Address: hop(site, c.status, "/landing", false), Body: []byte("b"),
			Header: http.Header{"Content-Type": {"text/plain"}}})
		asked := site.took()
		if got := methodsOf(asked); got != c.want {
			t.Errorf("%s answered %d asked %q, want %q", c.method, c.status, got, c.want)
		}
		last := asked[len(asked)-1]
		if carried := strings.Contains(last, `body="b"`); carried != (!strings.HasPrefix(last, "GET") &&
			!strings.HasPrefix(last, "HEAD")) {
			t.Errorf("%s answered %d went on as %s", c.method, c.status, last)
		}
		became := c.method != "GET" && c.method != "HEAD" && strings.HasPrefix(last, "GET")
		if typed := strings.Contains(last, "Content-Type"); typed == became {
			t.Errorf("%s answered %d went on as %s", c.method, c.status, last)
		}
		if !result.Redirected || result.Address != site.URL+"/landing" || result.Kind != fetch.Basic || result.Hops != 1 {
			t.Errorf("%s answered %d: the script sees %+v", c.method, c.status, result)
		}
	}

	blocked := b.blocked(fetch.Call{Method: "GET", Address: hop(site, 302, "/landing", false),
		Redirect: fetch.RedirectError})
	if !blocked.Sent || blocked.CORS || !strings.Contains(blocked.Why, `redirect mode is "error"`) {
		t.Errorf("blocked as %+v", blocked)
	}
	manual := b.seen(fetch.Call{Method: "GET", Address: hop(site, 302, "/landing", false),
		Redirect: fetch.RedirectManual})
	if manual.Kind != fetch.OpaqueRedirect || manual.Status != 0 || len(manual.Header) != 0 || manual.Address != "" {
		t.Errorf("of a redirect not followed the script sees %+v", manual)
	}
	if got := methodsOf(site.took()); got != "GET /to/302/, GET /to/302/" {
		t.Errorf("redirects not followed asked %q", got)
	}

	if _, err := b.call(fetch.Call{Method: "GET", Address: hop(site, 302, "ftp://site.test/x", false)}); err == nil ||
		!strings.Contains(err.Error(), "only http and https") {
		t.Errorf("a redirect to ftp gave %v", err)
	}
	rows := b.log.Rows()
	if rows[1].From != rows[0].ID {
		t.Errorf("the log does not say where a redirect comes from: %+v", rows[1])
	}
}

func TestARedirectForEverIsStopped(t *testing.T) {
	var site *origin
	site = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", site.URL+"/again")
		w.WriteHeader(http.StatusFound)
	})
	b := browse(t, site.URL+"/page")
	if _, err := b.call(fetch.Call{Method: "GET", Address: site.URL + "/again"}); !errors.Is(err,
		fetch.ErrTooManyRedirects) {
		t.Errorf("a redirect to itself gave %v", err)
	}
	if asked := len(site.took()); asked != fetch.MaxRedirects+1 {
		t.Errorf("it was asked %d times", asked)
	}
}

func TestARedirectToAnotherOriginIsCheckedAgain(t *testing.T) {
	site := hopping(t, echo)
	open := hopping(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		echo(w, r)
	})
	star := hopping(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		echo(w, r)
	})
	closed := hopping(t, echo)
	b := browse(t, site.URL+"/page")
	b.jar.Store(cookies.Request{URL: at(t, site.URL+"/")}, []string{"session=1"})
	took := func() string {
		return strings.Join(slices.Concat(site.took(), open.took(), star.took(), closed.took()), "\n")
	}

	result := b.seen(fetch.Call{Method: "GET", Address: hop(site, 302, open.URL+"/data", false),
		Header: http.Header{"Authorization": {"Bearer t"}}})
	if result.Kind != fetch.Shared || !result.Redirected || result.Address != open.URL+"/data" ||
		result.Header.Get("X-Secret") != "" {
		t.Errorf("from its own origin to one that shares, the script sees %+v", result)
	}
	if asked := took(); !strings.Contains(asked, "Cookie=session=1 Authorization=Bearer t") ||
		!strings.HasSuffix(asked, "GET /data Origin="+site.URL+" Referer="+site.URL+"/") {
		t.Errorf("it asked\n%s", asked)
	}

	blocked := b.blocked(fetch.Call{Method: "GET", Address: hop(site, 302, closed.URL+"/data", false)})
	if !blocked.CORS || blocked.Address != closed.URL+"/data" || blocked.From == "" {
		t.Errorf("from its own origin to one that does not share: %+v", blocked)
	}
	took()

	blocked = b.blocked(fetch.Call{Method: "GET", Address: hop(closed, 302, site.URL+"/data", false)})
	if !blocked.CORS || blocked.From != "" || len(site.took()) != 0 {
		t.Errorf("a redirect of an origin that does not share was followed: %+v", blocked)
	}
	took()

	blocked = b.blocked(fetch.Call{Method: "GET", Address: hop(open, 302, site.URL+"/data", true)})
	if !blocked.CORS || blocked.Address != site.URL+"/data" {
		t.Errorf("back to its own origin, which shares nothing: %+v", blocked)
	}
	same(t, "back to its own origin it asks as no origin, without its cookies", site.took(),
		[]string{"GET /data Origin=null Referer=" + site.URL + "/page"})
	took()

	result = b.seen(fetch.Call{Method: "GET", Address: hop(star, 302, star.URL+"/data", true)})
	if result.Kind != fetch.Shared {
		t.Errorf("within another origin the script sees %+v", result)
	}
	if asked := took(); !strings.HasSuffix(asked, "GET /data Origin="+site.URL+" Referer="+site.URL+"/") {
		t.Errorf("within another origin it asked\n%s", asked)
	}

	blocked = b.blocked(fetch.Call{Method: "GET", Address: hop(star, 302, open.URL+"/data", true)})
	if !strings.Contains(blocked.Why, "'null' that is not equal to the supplied origin") {
		t.Errorf("through two other origins: %+v", blocked)
	}
	result = b.seen(fetch.Call{Method: "GET", Address: hop(open, 302, star.URL+"/data", true)})
	if result.Kind != fetch.Shared || string(result.Body) != "took GET" {
		t.Errorf("through two other origins to one shared with all, the script sees %+v", result)
	}
	if asked := took(); !strings.Contains(asked, "GET /data Origin=null Referer="+site.URL+"/") {
		t.Errorf("through two other origins it asked\n%s", asked)
	}
}

func TestARedirectIsNotAWayRoundWhatARequestMayDo(t *testing.T) {
	site := hopping(t, echo)
	star := hopping(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		echo(w, r)
	})
	closed := hopping(t, echo)
	b := browse(t, site.URL+"/page")
	took := func() { _, _, _ = site.took(), star.took(), closed.took() }

	blocked := b.blocked(fetch.Call{Method: "POST", Address: hop(site, 307, star.URL+"/data", false),
		Mode: fetch.ModeSameOrigin, Body: []byte("secret")})
	if blocked.Sent || blocked.CORS || blocked.Address != star.URL+"/data" || len(star.took()) != 0 {
		t.Errorf("a same-origin request redirected away: %+v", blocked)
	}
	took()

	asked := serve(t, sharing)
	result := b.seen(fetch.Call{Method: "PUT", Address: hop(site, 307, asked.URL+"/data", false), Body: []byte("b")})
	if result.Kind != fetch.Shared {
		t.Errorf("a PUT redirected to another origin: the script sees %+v", result)
	}
	if got := methodsOf(asked.took()); got != "OPTIONS /data, PUT /data" {
		t.Errorf("a PUT redirected to another origin asked it %q", got)
	}
	took()

	away := strings.Replace(star.URL, "http://", "http://user:pw@", 1)
	blocked = b.blocked(fetch.Call{Method: "GET", Address: hop(site, 302, away+"/data", false)})
	if len(star.took()) != 0 || strings.Contains(blocked.Why, "pw") {
		t.Errorf("a redirect to another origin with a password: %+v", blocked)
	}

	opaque := b.seen(fetch.Call{Method: "GET", Address: hop(closed, 302, site.URL+"/data", false),
		Mode: fetch.ModeNoCORS})
	if opaque.Kind != fetch.Opaque || len(opaque.Body) != 0 {
		t.Errorf("a no-cors request redirected back to its own origin: the script sees %+v", opaque)
	}
	if blocked = b.blocked(fetch.Call{Method: "GET", Address: closed.URL + "/data", Mode: fetch.ModeNoCORS,
		Redirect: fetch.RedirectManual}); blocked.Sent {
		t.Errorf("a no-cors request that follows no redirect: %+v", blocked)
	}
}

func TestAnHTTPSPageAsksNothingInTheClear(t *testing.T) {
	plainSite := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		echo(w, r)
	})
	b := browse(t, "https://site.test/page")
	for _, address := range []string{"http://site.test/api", "http://other.test/api", "http://192.0.2.1/api"} {
		blocked := b.blocked(fetch.Call{Method: "POST", Address: address, Body: []byte("x")})
		if !blocked.Mixed || blocked.Sent || blocked.CORS || blocked.Address != address ||
			blocked.Page != "https://site.test/page" {
			t.Errorf("%s was blocked as %+v", address, blocked)
		}
	}
	result := b.seen(fetch.Call{Method: "GET", Address: plainSite.URL + "/api"})
	if result.Kind != fetch.Shared {
		t.Errorf("of this machine the script sees %+v", result)
	}
	same(t, "this machine was asked, and told nothing of where from", plainSite.took(),
		[]string{"GET /api Origin=https://site.test"})
	hops := hopping(t, func(http.ResponseWriter, *http.Request) {})
	blocked := b.blocked(fetch.Call{Method: "GET", Address: hop(hops, 302, "http://other.test/x", true)})
	if !blocked.Mixed || blocked.Address != "http://other.test/x" || blocked.From == "" {
		t.Errorf("a redirect to an address in the clear was blocked as %+v", blocked)
	}
}

func TestAScriptsRequestEndsWithItsContext(t *testing.T) {
	started := make(chan struct{})
	site := serve(t, func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	b := browse(t, site.URL+"/page")
	ctx, cancel := context.WithCancel(b.ctx())
	go func() {
		<-started
		cancel()
	}()
	if _, err := b.client.Script(ctx, fetch.Call{Method: "POST", Address: site.URL + "/slow", Page: b.page,
		Body: []byte("x")}); !errors.Is(err, context.Canceled) {
		t.Errorf("a request stopped gave %v", err)
	}
}

func TestAPageOfNoOriginAsksAsNull(t *testing.T) {
	other := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		if r.URL.Path == "/open" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		echo(w, r)
	})
	b := browse(t, "light://scripts")
	b.jar.Store(cookies.Request{URL: at(t, other.URL+"/")}, []string{"session=1"})
	if result := b.seen(fetch.Call{Method: "POST", Address: other.URL + "/open"}); result.Kind != fetch.Shared {
		t.Errorf("the script sees %+v", result)
	}
	if blocked := b.blocked(fetch.Call{Method: "POST", Address: other.URL + "/null"}); !blocked.CORS {
		t.Errorf("blocked as %+v", blocked)
	}
	same(t, "a page of no origin", other.took(), []string{"POST /open Origin=null", "POST /null Origin=null"})
}

func TestAScriptOfADocumentWithNoOriginAsksAsNull(t *testing.T) {
	o := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		echo(w, r)
	})
	b := browse(t, o.URL+"/sandboxed")
	result, err := b.client.Script(cookies.Without(b.ctx()), fetch.Call{Method: "GET", Address: o.URL + "/data",
		Page: b.page, Opaque: true})
	if err != nil || result.Kind != fetch.Shared {
		t.Fatalf("its own address answered %+v, %v; want an answer shared with all, read as another origin's", result,
			err)
	}
	same(t, "what a document with no origin asked its own address", o.took(),
		[]string{"GET /data Origin=null Referer=" + o.URL + "/sandboxed"})

	for name, handle := range map[string]http.HandlerFunc{"not shared": echo, "shared with null": sharing} {
		closed := serve(t, handle)
		call := fetch.Call{Method: "GET", Address: closed.URL + "/data", Page: b.page, Opaque: true}
		if blocked := b.blocked(call); !blocked.CORS {
			t.Errorf("an answer %s was kept for %q", name, blocked.Why)
		}
	}
}

func TestAFramesRequestInTheClearIsRefusedUnderAPageThatIsNot(t *testing.T) {
	b := browse(t, "http://frame.test/")
	blocked := b.blocked(fetch.Call{Method: "GET", Address: "http://plain.test/data", Top: at(t, "https://top.test/")})
	if !blocked.Mixed || blocked.Sent {
		t.Errorf("the request was kept as %+v, want mixed content, nothing sent", blocked)
	}
}
