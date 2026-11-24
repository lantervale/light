//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/netlog"
)

// wire is a server that notes what it is asked, each request a line:
// its method and its path, what it carries of the headers a browser or
// a script sets, and its body.
type wire struct {
	*httptest.Server
	mu    sync.Mutex
	lines []string
	// page is what the server shows at /.
	page string
}

func (w *wire) note(r *http.Request) {
	line := r.Method + " " + r.URL.RequestURI()
	for _, name := range []string{"Origin", "Cookie", "Content-Type", "X-A", "Access-Control-Request-Method",
		"Access-Control-Request-Headers"} {
		if value := r.Header.Get(name); value != "" {
			line += " " + name + "=" + value
		}
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) > 0 {
		line += fmt.Sprintf(" body=%q", body)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines = append(w.lines, line)
}

// took is what the server was asked for but its page and its icon,
// sorted.
func (w *wire) took() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var lines []string
	for _, line := range w.lines {
		if !strings.HasPrefix(line, "GET / ") && line != "GET /" && !strings.HasPrefix(line, "GET /favicon.ico") {
			lines = append(lines, line)
		}
	}
	slices.Sort(lines)
	return lines
}

// wired is a server that answers with handle, and at / with its page.
func wired(t *testing.T, handle http.HandlerFunc) *wire {
	t.Helper()
	w := &wire{}
	w.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.note(r)
		if r.URL.Path == "/" {
			rw.Header().Add("Set-Cookie", "session=1")
			rw.Header().Add("Set-Cookie", "strict=1; SameSite=Strict")
			rw.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(rw, w.page)
			return
		}
		handle(rw, r)
	}))
	t.Cleanup(w.Close)
	return w
}

// took answers every request with its method, and sets the cookie its
// query names.
func took(w http.ResponseWriter, r *http.Request) {
	if name := r.URL.Query().Get("set"); name != "" {
		w.Header().Set("Set-Cookie", name+"=1; Path=/")
	}
	w.Header().Set("X-Secret", "s")
	_, _ = io.WriteString(w, "took "+r.Method)
}

// shares answers as an origin that shares with the one that asks, its
// credentials too, and allows what a preflight asks for.
func shares(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Expose-Headers", "X-Shown")
	w.Header().Set("X-Shown", "v")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
		w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		w.Header().Set("Access-Control-Max-Age", "0")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	took(w, r)
}

// byName is a server's address by its name, another site than by its
// number.
func byName(address string) string { return strings.Replace(address, "127.0.0.1", "localhost", 1) }

// asks shows the page of site whose script is source, which pushes to
// seen what it sees and calls done at its end, and gives what it saw,
// sorted.
func (s *shown) asks(site *wire, source string) []string {
	s.t.Helper()
	site.page = `<title>Asks</title><p id="out"></p><script>const seen = [];
		const done = () => { document.getElementById("out").textContent = seen.sort().join("|") || "-"; };
		const say = (name, asked) => asked.then(
			(r) => r.text().then((text) => seen.push([name, r.type, r.status, text].join(" "))),
			(e) => seen.push(name + " " + e.name));
		` + source + `</script>`
	s.visit(site.URL + "/")
	s.until("the script was done", func() bool { return s.pageText("out") != "" })
	return strings.Split(s.pageText("out"), "|")
}

func equal(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n%s\nwant\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// row is the row of the shown tab's log for that method and address.
func (s *shown) row(method, address string) netlog.Row {
	s.t.Helper()
	for _, r := range s.session().log.Rows() {
		if r.Method == method && r.Address == address {
			return r
		}
	}
	s.t.Fatalf("no row for %s %s", method, address)
	return netlog.Row{}
}

func TestAPagesScriptPostsToItsOwnOriginAndToThoseThatShare(t *testing.T) {
	site, open, closed := wired(t, took), wired(t, shares), wired(t, took)
	s := show(t)
	s.toggleTools()
	saw := s.asks(site, fmt.Sprintf(`const open = %q, closed = %q;
		const xhr = (name, method, url) => new Promise((settle) => {
			const x = new XMLHttpRequest(); x.open(method, url);
			x.onload = () => { seen.push([name, x.status, x.responseText].join(" ")); settle(); };
			x.onerror = () => { seen.push([name, "error", x.status, JSON.stringify(x.responseText)].join(" ")); settle(); };
			x.send(method === "GET" ? null : "b");
		});
		Promise.all([
			say("own", fetch("/api", { method: "POST", body: "b", headers: { Cookie: "stolen=1", Host: "evil.test",
				Origin: "https://evil.test", "Sec-Fetch-Site": "none", "Content-Length": "99" } })),
			say("open", fetch(open + "/api", { method: "POST", body: new URLSearchParams({ a: "1" }) })),
			say("closed", fetch(closed + "/api", { method: "POST", body: "b" })),
			say("closed-get", fetch(closed + "/api")),
			xhr("xhr-own", "PUT", "/api"), xhr("xhr-open", "POST", open + "/api"), xhr("xhr-closed", "POST", closed + "/api"),
			xhr("xhr-closed-get", "GET", closed + "/api"),
		]).then(done);`, open.URL, closed.URL))
	equal(t, "the script saw", saw, []string{
		"closed TypeError", "closed-get TypeError", "open cors 200 took POST", "own basic 200 took POST",
		`xhr-closed error 0 ""`, `xhr-closed-get error 0 ""`, "xhr-open 200 took POST", "xhr-own 200 took PUT",
	})
	const text = " Content-Type=text/plain;charset=UTF-8"
	from := " Origin=" + site.URL
	equal(t, "its own origin was asked", site.took(), []string{
		"POST /api" + from + " Cookie=session=1; strict=1" + text + ` body="b"`,
		"PUT /api" + from + " Cookie=session=1; strict=1" + text + ` body="b"`,
	})
	equal(t, "the origin that shares was asked", open.took(), []string{
		"POST /api" + from + ` Content-Type=application/x-www-form-urlencoded;charset=UTF-8 body="a=1"`,
		"POST /api" + from + text + ` body="b"`,
	})
	equal(t, "the origin that does not share was asked all the same", closed.took(), []string{
		"GET /api" + from, "GET /api" + from, "POST /api" + from + text + ` body="b"`,
		"POST /api" + from + text + ` body="b"`,
	})
	const why = "No 'Access-Control-Allow-Origin' header is present on the requested resource."
	said := s.said()
	for _, want := range []string{
		"error: Access to fetch at '" + closed.URL + "/api' from origin '" + site.URL + "' has been blocked by CORS " +
			"policy: " + why,
		"error: Access to XMLHttpRequest at '" + closed.URL + "/api' from origin '" + site.URL + "' has been blocked by " +
			"CORS policy: " + why,
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the console lacks %q in\n%s", want, said)
		}
	}
	if kept := s.row("POST", closed.URL+"/api"); !kept.Blocked || kept.Err != why || kept.Status != http.StatusOK ||
		kept.Type != netlog.XHR || kept.Request.Get("Origin") != site.URL {
		t.Errorf("the request whose answer was kept is listed as %+v", kept)
	}
	if read := s.row("POST", open.URL+"/api"); read.Blocked || read.Err != "" || read.Type != netlog.XHR ||
		read.Initiator == "" {
		t.Errorf("the request whose answer was read is listed as %+v", read)
	}
	s.click("dt-chip-xhr")
	blocked := 0
	for _, row := range s.node("dt-rows").Children() {
		if row.HasClass("blocked") {
			blocked++
		}
	}
	if blocked != 4 {
		t.Errorf("the Network panel shows %d blocked rows, want 4", blocked)
	}
	s.clean()
}

func TestAPagesScriptAsksAnotherOriginBeforeARequestThatIsNotSimple(t *testing.T) {
	site, open := wired(t, took), wired(t, shares)
	strict := wired(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		took(w, r)
	})
	s := show(t)
	s.toggleTools()
	saw := s.asks(site, fmt.Sprintf(`const open = %q, strict = %q;
		const json = { method: "PUT", body: "{}", headers: { "Content-Type": "application/json", "X-A": "1" } };
		const xhr = (name, url) => new Promise((settle) => {
			const x = new XMLHttpRequest(); x.open("POST", url); x.setRequestHeader("X-A", "2");
			x.onloadend = () => { seen.push([name, x.status, x.responseText, x.getResponseHeader("X-Shown"),
				x.getResponseHeader("X-Secret")].join(" ")); settle(); };
			x.send("b");
		});
		Promise.all([
			say("open", fetch(open + "/put", json)), say("strict", fetch(strict + "/put", json)),
			say("strict-delete", fetch(strict + "/delete", { method: "DELETE" })),
			say("own", fetch("/put", json)), xhr("xhr-open", open + "/xhr"), xhr("xhr-strict", strict + "/xhr"),
		]).then(done);`, open.URL, strict.URL))
	equal(t, "the script saw", saw, []string{"open cors 200 took PUT", "own basic 200 took PUT", "strict TypeError",
		"strict-delete TypeError", "xhr-open 200 took POST v ", "xhr-strict 0   "})
	from := " Origin=" + site.URL
	equal(t, "the origin that allows was asked first, then sent the request", open.took(), []string{
		"OPTIONS /put" + from + " Access-Control-Request-Method=PUT Access-Control-Request-Headers=content-type,x-a",
		"OPTIONS /xhr" + from + " Access-Control-Request-Method=POST Access-Control-Request-Headers=x-a",
		"POST /xhr" + from + ` Content-Type=text/plain;charset=UTF-8 X-A=2 body="b"`,
		"PUT /put" + from + ` Content-Type=application/json X-A=1 body="{}"`,
	})
	equal(t, "the origin that does not allow was asked, and sent nothing", strict.took(), []string{
		"OPTIONS /delete" + from + " Access-Control-Request-Method=DELETE",
		"OPTIONS /put" + from + " Access-Control-Request-Method=PUT Access-Control-Request-Headers=content-type,x-a",
		"OPTIONS /xhr" + from + " Access-Control-Request-Method=POST Access-Control-Request-Headers=x-a",
	})
	equal(t, "its own origin was not asked first", site.took(), []string{
		"PUT /put" + from + ` Cookie=session=1; strict=1 Content-Type=application/json X-A=1 body="{}"`})
	said := s.said()
	for _, want := range []string{
		"Access to fetch at '" + strict.URL + "/delete' from origin '" + site.URL + "' has been blocked by CORS policy: " +
			"Method DELETE is not allowed by Access-Control-Allow-Methods in preflight response.",
		"Access to fetch at '" + strict.URL + "/put' from origin '" + site.URL + "' has been blocked by CORS policy: " +
			"Method PUT is not allowed by Access-Control-Allow-Methods in preflight response.",
		"Access to XMLHttpRequest at '" + strict.URL + "/xhr' from origin '" + site.URL + "' has been blocked by CORS " +
			"policy: Request header field x-a is not allowed by Access-Control-Allow-Headers in preflight response.",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the console lacks %q in\n%s", want, said)
		}
	}
	asked, sent := s.row("OPTIONS", open.URL+"/put"), s.row("PUT", open.URL+"/put")
	if asked.Type != netlog.Preflight || asked.Kind() != "preflight" || asked.Blocked || asked.ID > sent.ID ||
		asked.Initiator != sent.Initiator || asked.Status != http.StatusNoContent || sent.Type != netlog.XHR {
		t.Errorf("the preflight is listed as %+v, then the request as %+v", asked, sent)
	}
	if refused := s.row("OPTIONS", strict.URL+"/delete"); refused.Type != netlog.Preflight || !refused.Blocked ||
		refused.Err != "Method DELETE is not allowed by Access-Control-Allow-Methods in preflight response." {
		t.Errorf("the preflight that was refused is listed as %+v", refused)
	}
	s.click("dt-chip-xhr")
	row := s.node(s.requestRowOf("OPTIONS", open.URL+"/put"))
	if text := row.Text(); !strings.Contains(text, "OPTIONS") || !strings.Contains(text, "preflight") {
		t.Errorf("the Network panel shows the preflight as %q", text)
	}
	if !s.node(s.requestRowOf("OPTIONS", strict.URL+"/delete")).HasClass("blocked") {
		t.Error("the Network panel does not show the refused preflight as blocked")
	}
	s.clean()
}

// requestRowOf is the id of the developer tools' row of that request.
func (s *shown) requestRowOf(method, address string) string {
	s.t.Helper()
	return fmt.Sprintf("dt-row-%d", s.row(method, address).ID)
}

func TestTheCookiesOfAPagesScriptsRequests(t *testing.T) {
	site, other := wired(t, took), wired(t, shares)
	star := wired(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		took(w, r)
	})
	s := show(t)
	saw := s.asks(site, fmt.Sprintf(`const other = %q, away = %q, star = %q;
		const xhr = (name, url, withCredentials) => new Promise((settle) => {
			const x = new XMLHttpRequest(); x.open("POST", url); x.withCredentials = withCredentials;
			x.onloadend = () => { seen.push(name + " " + x.status); settle(); };
			x.send("b");
		});
		const post = (credentials) => ({ method: "POST", body: "b", credentials });
		Promise.all([
			say("own", fetch("/own?set=own", post())), say("own-omit", fetch("/own-omit?set=ownomit", post("omit"))),
			say("own-include", fetch("/own-include?set=owninclude", post("include"))),
			say("other", fetch(other + "/other?set=other", post())),
			say("other-omit", fetch(other + "/other-omit?set=otheromit", post("omit"))),
			say("other-include", fetch(other + "/other-include?set=otherinclude", post("include"))),
			say("away-include", fetch(away + "/away-include?set=awayinclude", post("include"))),
			say("star-include", fetch(star + "/star-include?set=starinclude", post("include"))),
			say("star", fetch(star + "/star", post())),
			xhr("xhr-other", other + "/xhr-other?set=xhrother", false),
			xhr("xhr-other-with", other + "/xhr-other-with?set=xhrotherwith", true),
		]).then(done);`, other.URL, byName(other.URL), star.URL))
	equal(t, "the script saw", saw, []string{
		"away-include cors 200 took POST", "other cors 200 took POST", "other-include cors 200 took POST",
		"other-omit cors 200 took POST", "own basic 200 took POST", "own-include basic 200 took POST",
		"own-omit basic 200 took POST", "star cors 200 took POST", "star-include TypeError", "xhr-other 200",
		"xhr-other-with 200",
	})
	carried := map[string]bool{}
	for _, line := range slices.Concat(site.took(), other.took(), star.took()) {
		path := strings.TrimPrefix(strings.Fields(line)[1], "/")
		path, _, _ = strings.Cut(path, "?")
		carried[path] = strings.Contains(line, "Cookie=session=1; strict=1")
		if !carried[path] && strings.Contains(line, "Cookie=") {
			t.Errorf("a request carried some of the cookies: %s", line)
		}
	}
	want := map[string]bool{"own": true, "own-omit": false, "own-include": true, "other": false, "other-omit": false,
		"other-include": true, "away-include": false, "star-include": true, "star": false, "xhr-other": false,
		"xhr-other-with": true}
	for path, sent := range want {
		if got, asked := carried[path]; !asked || got != sent {
			t.Errorf("/%s carried the page's cookies: %v (asked: %v), want %v", path, got, asked, sent)
		}
	}
	at, _ := url.Parse(site.URL + "/")
	var kept []string
	for _, c := range s.jar.For(at) {
		kept = append(kept, c.Name)
	}
	slices.Sort(kept)
	equal(t, "the jar kept the cookies of the answers to requests with credentials", kept,
		[]string{"otherinclude", "own", "owninclude", "session", "starinclude", "strict", "xhrotherwith"})
	if said := s.said(); !strings.Contains(said, "Access to fetch at '"+star.URL+"/star-include?set=starinclude' from "+
		"origin '"+site.URL+"' has been blocked by CORS policy: The value of the 'Access-Control-Allow-Origin' header in "+
		"the response must not be the wildcard '*' when the request's credentials mode is 'include'.") {
		t.Errorf("the console says\n%s", said)
	}
}

func TestWhatAPagesScriptReadsOfAnswersAndRedirects(t *testing.T) {
	var open, closed *wire
	hops := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if to := r.URL.Query().Get("to"); to != "" {
				if r.URL.Query().Get("share") != "" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				w.Header().Set("Location", to)
				w.WriteHeader(http.StatusFound)
				return
			}
			next(w, r)
		}
	}
	site := wired(t, hops(took))
	open, closed = wired(t, hops(shares)), wired(t, hops(took))
	s := show(t)
	saw := s.asks(site, fmt.Sprintf(`const open = %q, closed = %q;
		const keys = (r) => [...r.headers.keys()].sort().join();
		Promise.all([
			fetch("/own").then((r) => seen.push("own-headers " + keys(r))),
			fetch(open + "/open").then((r) => seen.push("open-headers " + keys(r) + " " + r.headers.get("x-shown"))),
			fetch(closed + "/hidden", { method: "POST", body: "b", mode: "no-cors" }).then((r) => r.text().then((text) =>
				seen.push(["opaque", r.type, r.status, r.ok, JSON.stringify(text), JSON.stringify(r.url), keys(r)].join(" ")))),
			say("same-origin", fetch(open + "/never", { mode: "same-origin" })),
			fetch("/hop?to=/landing", { method: "POST", body: "b" }).then((r) =>
				seen.push("followed " + r.redirected + " " + new URL(r.url).pathname + " " + r.type)),
			say("refused", fetch("/hop?to=/landing", { redirect: "error" })),
			say("manual", fetch("/hop?to=/landing", { redirect: "manual" })),
			say("away-closed", fetch("/hop?to=" + encodeURIComponent(closed + "/landing"))),
			fetch("/hop?to=" + encodeURIComponent(open + "/landing")).then((r) =>
				seen.push("away-open " + r.redirected + " " + r.type + " " + (r.url === open + "/landing"))),
			say("back", fetch(open + "/hop?share=1&to=" + encodeURIComponent(location.origin + "/secret"))),
		]).then(done);`, open.URL, closed.URL))
	equal(t, "the script saw", saw, []string{
		"away-closed TypeError", "away-open true cors true", "back TypeError", "followed true /landing basic",
		"manual opaqueredirect 0 ", `opaque opaque 0 false "" "" `,
		"open-headers content-length,content-type,x-shown v", "own-headers content-length,content-type,date,x-secret",
		"refused TypeError",
		"same-origin TypeError",
	})
	if asked := strings.Join(open.took(), "\n"); strings.Contains(asked, "/never") {
		t.Errorf("a same-origin request went to another origin:\n%s", asked)
	}
	equal(t, "the origin that does not share was sent the no-cors request and the redirect", closed.took(), []string{
		"GET /landing Origin=" + site.URL,
		"POST /hidden Origin=" + site.URL + ` Content-Type=text/plain;charset=UTF-8 body="b"`,
	})
	if asked := site.took(); !slices.Contains(asked, "GET /secret Origin=null") {
		t.Errorf("redirected back to its own origin the request was sent as\n%s", strings.Join(asked, "\n"))
	}
	said := s.said()
	for _, want := range []string{
		"Access to fetch at '" + closed.URL + "/landing' (redirected from '" + site.URL + "/hop?to=",
		"Fetch API cannot load " + open.URL + `/never. Request mode is "same-origin" but the URL's origin is not same ` +
			"as the request initiator origin.",
		"Fetch API cannot load " + site.URL + `/hop?to=/landing. The request's redirect mode is "error", and it was ` +
			"redirected.",
		"Access to fetch at '" + site.URL + "/secret' (redirected from '" + open.URL + "/hop?share=1&to=",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the console lacks %q in\n%s", want, said)
		}
	}
	if unsent := s.row("GET", open.URL+"/never"); !unsent.Blocked || unsent.Status != 0 || unsent.Request != nil {
		t.Errorf("the request that was not sent is listed as %+v", unsent)
	}
	if landed := s.row("GET", open.URL+"/landing"); landed.From == 0 {
		t.Errorf("the redirect's end is listed as %+v", landed)
	}
}

// secured is a transport that takes what is asked of an https site by
// its name to the server at address, in the clear.
type secured struct{ address string }

func (s secured) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return http.DefaultTransport.RoundTrip(req)
	}
	sent := req.Clone(req.Context())
	sent.URL.Scheme, sent.URL.Host = "http", s.address
	resp, err := http.DefaultTransport.RoundTrip(sent)
	if resp != nil {
		resp.Request = req
	}
	return resp, err
}

func TestAnHTTPSPagesScriptSendsNothingInTheClear(t *testing.T) {
	site := wired(t, took)
	local := wired(t, shares)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: secured{site.Listener.Addr().String()}})
	s.toggleTools()
	site.page = fmt.Sprintf(`<title>Secure</title><p id="out"></p><script>const seen = [];
		const say = (name, asked) => asked.then((r) => seen.push(name + " " + r.type + " " + r.status),
			(e) => seen.push(name + " " + e.name));
		Promise.all([
			say("clear", fetch("http://site.test/api", { method: "POST", body: "secret" })),
			say("other", fetch("http://other.test/api")),
			say("local", fetch(%q + "/api", { method: "POST", body: "b" })),
			say("own", fetch("/api", { method: "POST", body: "b" })),
			new Promise((settle) => { const x = new XMLHttpRequest(); x.open("POST", "http://other.test/xhr");
				x.onloadend = () => { seen.push("xhr " + x.status); settle(); }; x.send("b"); }),
		]).then(() => { document.getElementById("out").textContent = seen.sort().join("|"); });</script>`, local.URL)
	s.visit("https://site.test/")
	s.until("the script was done", func() bool { return s.pageText("out") != "" })
	if got, want := s.pageText("out"), "clear TypeError|local cors 200|other TypeError|own basic 200|xhr 0"; got != want {
		t.Errorf("the script saw %s, want %s", got, want)
	}
	equal(t, "its own origin was asked", site.took(), []string{
		`POST /api Origin=https://site.test Cookie=session=1; strict=1 Content-Type=text/plain;charset=UTF-8 body="b"`})
	equal(t, "this machine was asked", local.took(), []string{
		`POST /api Origin=https://site.test Content-Type=text/plain;charset=UTF-8 body="b"`})
	said := s.said()
	for _, address := range []string{"http://site.test/api", "http://other.test/api", "http://other.test/xhr"} {
		if want := "Mixed Content: The page at 'https://site.test/' was loaded over HTTPS, but requested an insecure " +
			"resource '" + address + "'. This request has been blocked; the content must be served over HTTPS."; !strings.
			Contains(said, want) {
			t.Errorf("the console lacks %q in\n%s", want, said)
		}
	}
	if unsent := s.row("POST", "http://site.test/api"); !unsent.Blocked || unsent.Type != netlog.XHR ||
		!strings.HasPrefix(unsent.Err, "Mixed Content") {
		t.Errorf("the request in the clear is listed as %+v", unsent)
	}
}

func TestABeaconOutlivesItsPageAndARequestStoppedDoesNot(t *testing.T) {
	release := make(chan struct{})
	stats := wired(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
		shares(w, r)
	})
	site := wired(t, took)
	s := show(t)
	s.toggleTools()
	site.page = fmt.Sprintf(`<title>Leaving</title><p id="out"></p><script>const stats = %q;
		const stop = new AbortController();
		fetch(stats + "/stopped", { method: "POST", body: "s", signal: stop.signal }).catch(() => {});
		fetch(stats + "/slow", { method: "POST", body: "slow", signal: stop.signal }).catch((e) => {
			const xhr = new XMLHttpRequest(); xhr.open("POST", stats + "/slow"); xhr.timeout = 30;
			xhr.ontimeout = () => { document.getElementById("out").textContent = e.name + " timeout " + xhr.status +
				" " + navigator.sendBeacon(stats + "/beacon", new Blob(["left=1"], { type: "text/plain" })); };
			xhr.send("again");
		});
		setTimeout(() => stop.abort(), 50);</script>`, stats.URL)
	s.visit(site.URL + "/")
	s.until("the requests were stopped", func() bool { return s.pageText("out") != "" })
	if got, want := s.pageText("out"), "AbortError timeout 0 true"; got != want {
		t.Errorf("the script saw %q, want %q", got, want)
	}
	s.visit(site.URL + "/next")
	close(release)
	beacon := "POST /beacon Origin=" + site.URL + ` Cookie=session=1; strict=1 Content-Type=text/plain body="left=1"`
	for end := time.Now().Add(5 * time.Second); !slices.Contains(stats.took(), beacon); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("the beacon never came:\n%s", strings.Join(stats.took(), "\n"))
		}
	}
}
