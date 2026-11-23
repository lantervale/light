package app //nolint:testpackage // the driver is checked against the screens it drives: their tabs, their windows

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/platform"

	"github.com/lantervale/light/pkg/tabs"
	"github.com/lantervale/light/pkg/webdriver"
)

// driven is a hidden light driven over HTTP, as a WebDriver client
// drives it: each command is sent off the test's goroutine, which runs
// light's frames until it is answered.
type driven struct {
	*shown
	driver *Driver
	id     string
}

// reply is what a command was answered: its status, its value, and the
// error code of a value that is an error.
type wdReply struct {
	status  int
	value   json.RawMessage
	text    string
	code    string
	message string
	stack   string
}

func drive(t *testing.T) *driven {
	t.Helper()
	return driveWith(t, Options{Downloads: t.TempDir()})
}

func driveWith(t *testing.T, opts Options) *driven {
	t.Helper()
	s := showWith(t, opts)
	driver, err := Drive(context.Background(), s.Profile, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	s.world.AddSystems(driver)
	return &driven{shown: s, driver: driver}
}

func (d *driven) ask(method, path, body string) wdReply {
	d.t.Helper()
	var sent io.Reader
	if method == http.MethodPost {
		sent = strings.NewReader(body)
	}
	address := "http://" + d.driver.Address() + strings.ReplaceAll(path, "{s}", "session/"+d.id)
	req, err := http.NewRequestWithContext(context.Background(), method, address, sent)
	if err != nil {
		d.t.Fatal(err)
	}
	type result struct {
		status int
		body   []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		res, failed := http.DefaultClient.Do(req)
		if failed != nil {
			done <- result{err: failed}
			return
		}
		defer res.Body.Close()
		raw, failed := io.ReadAll(res.Body)
		done <- result{status: res.StatusCode, body: raw, err: failed}
	}()
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case got := <-done:
			if got.err != nil {
				d.t.Fatal(got.err)
			}
			return d.read(method, path, got.status, got.body)
		default:
		}
		if time.Now().After(deadline) {
			d.t.Fatalf("%s %s was never answered", method, path)
		}
		d.frame()
		time.Sleep(time.Millisecond)
	}
}

func (d *driven) read(method, path string, status int, body []byte) wdReply {
	d.t.Helper()
	var whole struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(body, &whole); err != nil || whole.Value == nil {
		d.t.Fatalf("%s %s answered %q, no JSON object with a value", method, path, body)
	}
	a := wdReply{status: status, value: whole.Value, text: string(whole.Value)}
	if status != http.StatusOK {
		var failed struct {
			Error      string `json:"error"`
			Message    string `json:"message"`
			Stacktrace string `json:"stacktrace"`
		}
		if err := json.Unmarshal(whole.Value, &failed); err != nil {
			d.t.Fatalf("%s %s failed with %s", method, path, whole.Value)
		}
		a.code, a.message, a.stack = failed.Error, failed.Message, failed.Stacktrace
	}
	return a
}

// ok asks for a command that must succeed, and gives its value.
func (d *driven) ok(method, path, body string) string {
	d.t.Helper()
	a := d.ask(method, path, body)
	if a.status != http.StatusOK {
		d.t.Fatalf("%s %s %s failed: %d %s", method, path, body, a.status, a.text)
	}
	return a.text
}

// fails asks for a command that must fail with code.
func (d *driven) fails(code webdriver.Code, method, path, body string) wdReply {
	d.t.Helper()
	a := d.ask(method, path, body)
	if a.code != string(code) || a.status != code.Status() {
		d.t.Fatalf("%s %s %s is answered %d %s, want %q", method, path, body, a.status, a.text, code)
	}
	return a
}

// begin creates the session.
func (d *driven) begin(capabilities string) {
	d.t.Helper()
	var made struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(d.ok("POST", "/session", `{"capabilities":`+capabilities+`}`)), &made); err != nil {
		d.t.Fatal(err)
	}
	d.id = made.SessionID
}

func (d *driven) open(address string) {
	d.t.Helper()
	d.ok("POST", "/{s}/url", `{"url":`+literal(address)+`}`)
}

// text is a command's value that is a string.
func (d *driven) text(path string) string {
	d.t.Helper()
	var value string
	if err := json.Unmarshal([]byte(d.ok("GET", path, "")), &value); err != nil {
		d.t.Fatalf("%s gives no string: %v", path, err)
	}
	return value
}

// newWindow is what New Window answers.
type newWindow struct {
	Handle string `json:"handle"`
	Type   string `json:"type"`
}

func (d *driven) handles() []string {
	d.t.Helper()
	var all []string
	if err := json.Unmarshal([]byte(d.ok("GET", "/{s}/window/handles", "")), &all); err != nil {
		d.t.Fatal(err)
	}
	return all
}

func TestADrivenLightIsServedOnThisMachineAloneAndSaysItIsDriven(t *testing.T) {
	s := show(t)
	for _, address := range []string{"0.0.0.0:0", ":0", "192.168.0.1:0", "example.test:4444"} {
		if driver, err := Drive(context.Background(), s.Profile, address); err == nil {
			_ = driver.Close()
			t.Errorf("light is driven from %s", address)
		}
	}
	if s.opts.Driven || s.navigator().Driven {
		t.Fatal("a light nothing drives says it is driven")
	}
	d := drive(t)
	if !d.navigator().Driven {
		t.Fatal("a driven light does not say navigator.webdriver")
	}
	for _, c := range []struct {
		header, value string
		status        int
	}{{"", "", 200}, {"Origin", "http://127.0.0.1", 403}, {"Origin", "https://evil.test", 403}} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+d.driver.Address()+"/status",
			nil)
		if c.header != "" {
			req.Header.Set(c.header, c.value)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != c.status {
			t.Errorf("a request with %s %q is answered %d, want %d", c.header, c.value, res.StatusCode, c.status)
		}
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+d.driver.Address()+"/status", nil)
	req.Host = "rebound.evil.test:4444"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("a request for another host's name is answered %d", res.StatusCode)
	}
}

func TestASessionNavigatesAndTellsWhereItIs(t *testing.T) {
	srv, seen := crypt(t)
	d := drive(t)
	a := d.ask("GET", "/status", "")
	if !strings.Contains(a.text, `"ready":true`) {
		t.Fatalf("the status is %s", a.text)
	}
	d.begin(`{"alwaysMatch":{"browserName":"light"}}`)
	d.fails(webdriver.SessionNotCreated, "POST", "/session", `{"capabilities":{}}`)
	if got := d.text("/{s}/url"); got != blankAddress {
		t.Fatalf("a new session is at %q", got)
	}
	d.open(srv.URL + "/")
	if got := d.text("/{s}/url"); got != srv.URL+"/" {
		t.Fatalf("after a navigation the session is at %q", got)
	}
	if got := d.text("/{s}/title"); got != "A crypt" {
		t.Fatalf("the title is %q", got)
	}
	if d.session().pages[srv.URL+"/"] == nil || d.host == nil {
		t.Fatal("Navigate To answered before the page was shown")
	}
	d.open(srv.URL + "/old")
	if got := d.text("/{s}/url"); got != srv.URL+"/vaults" {
		t.Fatalf("after a redirect the session is at %q", got)
	}
	d.ok("POST", "/{s}/back", "{}")
	if got := d.text("/{s}/title"); got != "A crypt" {
		t.Fatalf("after Back the title is %q", got)
	}
	d.ok("POST", "/{s}/forward", "{}")
	if got := d.text("/{s}/title"); got != "Vaults" {
		t.Fatalf("after Forward the title is %q", got)
	}
	before := seen.count("/vaults")
	d.ok("POST", "/{s}/refresh", "{}")
	if got := seen.count("/vaults"); got != before+1 || d.text("/{s}/title") != "Vaults" {
		t.Fatalf("a refresh fetched the page %d times more", got-before)
	}
	d.open(srv.URL + "/vaults")
	if got := seen.count("/vaults"); got != before+2 {
		t.Fatalf("a navigation to the page shown fetched it %d times more", got-before-1)
	}
	d.open("light://about")
	if got := d.text("/{s}/title"); got != "About" && got == "" {
		t.Fatalf("the title of a page of light's is %q", got)
	}
	d.fails(webdriver.InvalidArgument, "POST", "/{s}/url", `{"url":"no address"}`)
	d.fails(webdriver.UnsupportedOperation, "POST", "/{s}/url", `{"url":"ftp://example.test/"}`)
	d.clean()
}

func TestANavigationIsTheUsersOwn(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/typed" {
			got = r.Header.Clone()
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>t</title><p>page</p>`))
	}))
	t.Cleanup(srv.Close)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	d.open(srv.URL + "/typed")
	if got.Get("Referer") != "" || got.Get("Sec-Fetch-Site") != "none" || got.Get("Sec-Fetch-User") != "?1" {
		t.Fatalf("a driven navigation is sent as %v", got)
	}
}

func TestANavigationEndsOnceThePicturesOfThePageShow(t *testing.T) {
	pic := picture(t, 30, 20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pic.png" {
			time.Sleep(300 * time.Millisecond)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pic)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<body style="margin: 0"><img id="pic" src="/pic.png">`))
	}))
	t.Cleanup(srv.Close)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	doc := d.session().pages[srv.URL+"/"].Doc
	if got, _ := doc.BoxOf(doc.ByID("pic")); got.W != 30 || got.H != 20 {
		t.Fatalf("Navigate To answered with the picture laid out %v by %v, want 30 by 20", got.W, got.H)
	}
}

func TestAPageThatFailsOrNeverEndsIsAnErrorAndLightGoesOn(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/never" {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Here</title><p>page</p>`))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	d := drive(t)
	d.begin("{}")
	d.ok("POST", "/{s}/timeouts", `{"pageLoad":150}`)
	began := time.Now()
	a := d.fails(webdriver.Timeout, "POST", "/{s}/url", `{"url":"`+srv.URL+`/never"}`)
	if took := time.Since(began); took < 150*time.Millisecond || took > 5*time.Second {
		t.Fatalf("the page load timeout fired after %s: %s", took, a.message)
	}
	if got := d.text("/{s}/url"); got != blankAddress {
		t.Fatalf("while the page loads the session is at %q, want the page it shows", got)
	}
	d.open(srv.URL + "/")
	if got := d.text("/{s}/title"); got != "Here" {
		t.Fatalf("after a page that never ended the title is %q", got)
	}
	d.fails(webdriver.Timeout, "POST", "/{s}/url", `{"url":"`+srv.URL+`/never"}`)
	if got := d.text("/{s}/url"); got != srv.URL+"/" {
		t.Fatalf("while the next page loads the session is at %q, want the page it shows", got)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	gone := closed.URL
	closed.Close()
	if a = d.fails(webdriver.UnknownError, "POST", "/{s}/url", `{"url":"`+gone+`/"}`); !strings.Contains(a.message,
		"was not reached") {
		t.Fatalf("a page that is not reached says %q", a.message)
	}
	d.OpenInNewTab(srv.URL + "/never")
	var all []string
	if err := json.Unmarshal([]byte(d.ok("GET", "/{s}/window/handles", "")), &all); err != nil || len(all) != 2 {
		t.Fatalf("the windows are %v, %v", all, err)
	}
	d.ok("POST", "/{s}/window", `{"handle":"`+all[1]+`"}`)
	if got := d.text("/{s}/url"); got != blankAddress {
		t.Fatalf("a tab that has shown nothing yet is at %q", got)
	}
	d.ok("DELETE", "/{s}", "")
	d.begin(`{"alwaysMatch":{"pageLoadStrategy":"none"}}`)
	began = time.Now()
	d.open(srv.URL + "/never")
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("with the strategy none a navigation waited %s", took)
	}
}

func TestWindowsAreLightsTabsAndWindows(t *testing.T) {
	srv, _ := crypt(t)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	first := d.text("/{s}/window")
	if got := d.handles(); len(got) != 1 || got[0] != first {
		t.Fatalf("the handles are %v, the window %q", got, first)
	}
	var opened newWindow
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/window/new", `{"type":"tab"}`)), &opened)
	if opened.Type != "tab" || len(d.browser.Tabs) != 2 || len(d.screens) != 1 || d.browser.Active != 0 {
		t.Fatalf("a new tab is %+v: %d tabs, %d windows, tab %d shown", opened, len(d.browser.Tabs), len(d.screens),
			d.browser.Active)
	}
	if got := d.text("/{s}/window"); got != first {
		t.Fatalf("a new window became the session's: %q", got)
	}
	d.ok("POST", "/{s}/window", `{"handle":"`+opened.Handle+`"}`)
	if got := d.text("/{s}/url"); got != blankAddress || d.browser.Active != 1 || d.host == nil {
		t.Fatalf("the new tab is at %q, tab %d shown", got, d.browser.Active)
	}
	d.open(srv.URL + "/vaults")
	d.ok("POST", "/{s}/window", `{"handle":"`+first+`"}`)
	if got := d.text("/{s}/title"); got != "A crypt" {
		t.Fatalf("back in the first tab the title is %q", got)
	}
	var apart newWindow
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/window/new", `{"type":"window"}`)), &apart)
	if apart.Type != "window" || len(d.screens) != 2 || len(d.handles()) != 3 {
		t.Fatalf("a new window is %+v: %d windows, handles %v", apart, len(d.screens), d.handles())
	}
	d.fails(webdriver.NoSuchWindow, "POST", "/{s}/window", `{"handle":"none"}`)
	d.ok("POST", "/{s}/window", `{"handle":"`+apart.Handle+`"}`)
	if got := d.ok("DELETE", "/{s}/window", ""); got != `["`+first+`","`+opened.Handle+`"]` || len(d.screens) != 1 {
		t.Fatalf("closing the window leaves %s and %d windows", got, len(d.screens))
	}
	d.fails(webdriver.NoSuchWindow, "GET", "/{s}/url", "")
	d.fails(webdriver.NoSuchWindow, "GET", "/{s}/window", "")
	d.ok("POST", "/{s}/window", `{"handle":"`+opened.Handle+`"}`)
	if got := d.ok("DELETE", "/{s}/window", ""); got != `["`+first+`"]` || len(d.browser.Tabs) != 1 {
		t.Fatalf("closing the tab leaves %s and %d tabs", got, len(d.browser.Tabs))
	}
	d.ok("POST", "/{s}/window", `{"handle":"`+first+`"}`)
	if got := d.ok("DELETE", "/{s}/window", ""); got != `[]` {
		t.Fatalf("closing the last window leaves %s", got)
	}
	d.fails(webdriver.InvalidSessionID, "GET", "/{s}/url", "")
	if len(d.screens) != 1 || len(d.browser.Tabs) != 1 || d.browser.Tab().Current() != tabs.Home {
		t.Fatalf("after its last window light has %d windows, at %s", len(d.screens), d.browser.Tab().Current())
	}
	d.begin("{}")
	d.open(srv.URL + "/vaults")
	d.clean()
}

func TestDeletingTheSessionLeavesLightUsable(t *testing.T) {
	srv, _ := crypt(t)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	d.ok("POST", "/{s}/window/new", `{"type":"tab"}`)
	d.ok("POST", "/{s}/window/new", `{"type":"window"}`)
	d.ok("DELETE", "/{s}", "")
	d.frame()
	d.frame()
	if len(d.screens) != 1 || len(d.browser.Tabs) != 1 || d.browser.Tab().Current() != tabs.Home {
		t.Fatalf("after its session light has %d windows and %d tabs, at %s", len(d.screens), len(d.browser.Tabs),
			d.browser.Tab().Current())
	}
	d.fails(webdriver.InvalidSessionID, "GET", "/{s}/url", "")
	d.enter("light://guide")
	d.at("light://guide")
	d.begin("{}")
	d.open(srv.URL + "/vaults")
	if got := d.text("/{s}/title"); got != "Vaults" {
		t.Fatalf("a second session reads the title %q", got)
	}
	d.clean()
}

func TestTheWindowIsMovedAndSized(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	var rect webdriver.Rect
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/window/rect", `{"x":40,"y":30,"width":800,"height":600}`)), &rect)
	if rect != (webdriver.Rect{X: 40, Y: 30, Width: 800, Height: 600}) || d.win.Size() != geom.V2(800, 600) {
		t.Fatalf("the window is %+v, %v", rect, d.win.Size())
	}
	if got := d.ok("GET", "/{s}/window/rect", ""); got != `{"x":40,"y":30,"width":800,"height":600}` {
		t.Fatalf("the window's rect is %s", got)
	}
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/window/rect", `{"width":100,"height":100}`)), &rect)
	if least := d.win.MinSize(); rect.Width != float64(least.X) || rect.Height != float64(least.Y) || rect.X != 40 {
		t.Fatalf("a window asked smaller than light's go is %+v", rect)
	}
	d.ok("POST", "/{s}/window/maximize", "{}")
	if !d.win.Maximized() {
		t.Fatal("the window is not maximized")
	}
	d.ok("POST", "/{s}/window/fullscreen", "{}")
	if !d.win.Fullscreen() || d.win.Maximized() {
		t.Fatal("the window is not full screen")
	}
	d.ok("POST", "/{s}/window/minimize", "{}")
	if !d.win.Minimized() || d.win.Fullscreen() {
		t.Fatal("the window is not minimized")
	}
	d.ok("POST", "/{s}/window/rect", `{"width":900,"height":700}`)
	if d.win.Minimized() || d.win.Size() != geom.V2(900, 700) {
		t.Fatalf("a rect leaves the window minimized, or %v", d.win.Size())
	}
}

func TestCookiesAreTheProfilesJar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "served", Value: "1", Path: "/", HttpOnly: true})
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Jar</title><p id="sent">` + r.Header.Get("Cookie") + `</p>`))
	}))
	t.Cleanup(srv.Close)
	d := drive(t)
	d.begin("{}")
	d.fails(webdriver.InvalidCookieDomain, "GET", "/{s}/cookie", "")
	d.open(srv.URL + "/")
	if got := d.ok("GET", "/{s}/cookie", ""); got !=
		`[{"name":"served","value":"1","path":"/","domain":"127.0.0.1","secure":false,"httpOnly":true,"sameSite":"Lax"}]` {
		t.Fatalf("the cookies are %s", got)
	}
	expiry := time.Now().Add(time.Hour).Unix()
	d.ok("POST", "/{s}/cookie", `{"cookie":{"name":"added","value":"two","path":"/","expiry":`+
		strings.TrimSpace(string(mustJSON(t, expiry)))+`,"sameSite":"Strict"}}`)
	var named webdriver.Cookie
	_ = json.Unmarshal([]byte(d.ok("GET", "/{s}/cookie/added", "")), &named)
	if named.Value != "two" || named.Expiry == nil || *named.Expiry != expiry || named.SameSite != "Strict" {
		t.Fatalf("the cookie added is %+v", named)
	}
	d.open(srv.URL + "/next")
	if got := d.session().pages[srv.URL+"/next"].Doc.ByID("sent").Text(); !strings.Contains(got, "added=two") ||
		!strings.Contains(got, "served=1") {
		t.Fatalf("the next request carried %q", got)
	}
	d.fails(webdriver.InvalidCookieDomain, "POST", "/{s}/cookie", `{"cookie":{"name":"a","value":"b",
		"domain":"other.test"}}`)
	d.fails(webdriver.NoSuchCookie, "GET", "/{s}/cookie/none", "")
	d.ok("DELETE", "/{s}/cookie/added", "")
	d.fails(webdriver.NoSuchCookie, "GET", "/{s}/cookie/added", "")
	d.ok("POST", "/{s}/cookie", `{"cookie":{"name":"more","value":"3"}}`)
	d.ok("DELETE", "/{s}/cookie", "")
	if got := d.ok("GET", "/{s}/cookie", ""); got != `[]` || d.jar.Len() != 0 {
		t.Fatalf("after deleting them all the cookies are %s", got)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAWindowThatAsksNothingHasNoAlert(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	d.fails(webdriver.NoSuchAlert, "POST", "/{s}/alert/accept", "{}")
	d.fails(webdriver.NoSuchAlert, "POST", "/{s}/alert/dismiss", "{}")
	d.fails(webdriver.NoSuchAlert, "GET", "/{s}/alert/text", "")
	d.fails(webdriver.NoSuchAlert, "POST", "/{s}/alert/text", `{"text":"a"}`)
}

func TestPointerActionsClickThroughTheWindowsInput(t *testing.T) {
	srv, _ := crypt(t)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	d.frame()
	view, _ := d.pageView()
	at := d.inPage("vault").Sub(geom.V2(view.X, view.Y))
	d.ok("POST", "/{s}/actions", `{"actions":[{"type":"pointer","id":"mouse","actions":[
		{"type":"pointerMove","x":`+string(mustJSON(t, at.X))+`,"y":`+string(mustJSON(t, at.Y))+`},
		{"type":"pointerDown","button":0},{"type":"pause","duration":20},{"type":"pointerUp","button":0}]}]}`)
	d.until("the link clicked was followed", func() bool { return d.session().pages[srv.URL+"/vaults"] != nil })
	if got := d.text("/{s}/url"); got != srv.URL+"/vaults" {
		t.Fatalf("after a click on a link the session is at %q", got)
	}
	d.fails(webdriver.MoveTargetOutOfBounds, "POST", "/{s}/actions", `{"actions":[{"type":"pointer","id":"mouse",
		"actions":[{"type":"pointerMove","x":-5,"y":10}]}]}`)
	d.ok("POST", "/{s}/actions", `{"actions":[{"type":"pointer","id":"mouse","actions":[
		{"type":"pointerMove","x":5,"y":5},{"type":"pointerDown","button":2}]},
		{"type":"key","id":"keys","actions":[{"type":"pause"},{"type":"keyDown","value":""}]}]}`)
	if len(d.driver.keys.buttons) != 1 || len(d.driver.keys.keys) != 1 {
		t.Fatalf("the driver holds %v and %v", d.driver.keys.buttons, d.driver.keys.keys)
	}
	d.ok("DELETE", "/{s}/actions", "")
	if len(d.driver.keys.buttons) != 0 || len(d.driver.keys.keys) != 0 {
		t.Fatal("Release Actions left a key or a button held")
	}
}

func TestAWheelActionScrollsByItsPixelsAtOnce(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<body style="margin: 0"><div id="tall" style="height: 5000px"></div>`),
	})
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	d.frame()
	page := d.session().pages[srv.URL+"/"].Doc
	for _, step := range []struct{ delta, at float32 }{{130, 130}, {-50, 80}} {
		d.ok("POST", "/{s}/actions", `{"actions":[{"type":"wheel","id":"wheel","actions":[
			{"type":"scroll","x":20,"y":20,"deltaX":0,"deltaY":`+string(mustJSON(t, step.delta))+`}]}]}`)
		if got := page.Scroll(page.HTML()); got != geom.V2(0, step.at) {
			t.Fatalf("once a scroll of %v is performed the page is scrolled to %v, want %v", step.delta, got, step.at)
		}
	}
}

// painted is a capture of a window that draws its page's view in one
// colour on a window of another, at two pixels a point.
func painted(view func() geom.Rect, page color.Color) func(*platform.Window, func(shot, error)) step {
	return func(win *platform.Window, done func(shot, error)) step {
		return func() (bool, error) {
			size, area := win.Size(), view()
			picture := image.NewRGBA(image.Rect(0, 0, int(size.X)*2, int(size.Y)*2))
			draw.Draw(picture, picture.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 40, B: 40, A: 255}), image.Point{},
				draw.Src)
			inside := image.Rect(int(area.X)*2, int(area.Y)*2, int(area.X+area.W)*2, int(area.Y+area.H)*2)
			draw.Draw(picture, inside, image.NewUniform(page), image.Point{}, draw.Src)
			done(shot{picture: picture, scale: 2}, nil)
			return true, nil
		}
	}
}

func decoded(t *testing.T, value string) image.Image {
	t.Helper()
	var encoded string
	if err := json.Unmarshal([]byte(value), &encoded); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return picture
}

func TestAScreenshotIsThePagesViewAPixelACSSPixel(t *testing.T) {
	srv, _ := crypt(t)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	a := d.fails(webdriver.UnableToCaptureScreen, "GET", "/{s}/screenshot", "")
	if !strings.Contains(a.message, "drawn by nothing") {
		t.Fatalf("a window nothing draws says %q", a.message)
	}
	blue := color.RGBA{R: 30, G: 144, B: 255, A: 255}
	d.driver.capture = painted(func() geom.Rect {
		view, _ := d.pageView()
		return view
	}, blue)
	view, _ := d.pageView()
	outer := d.win.Size()
	d.ok("POST", "/{s}/window/rect", string(mustJSON(t, map[string]float32{"width": 800 + outer.X - view.W,
		"height": 600 + outer.Y - view.H})))
	picture := decoded(t, d.ok("GET", "/{s}/screenshot", ""))
	if picture.Bounds().Size() != image.Pt(800, 600) {
		t.Fatalf("the screenshot is %v, want 800 by 600", picture.Bounds().Size())
	}
	for _, at := range []image.Point{{0, 0}, {799, 0}, {0, 599}, {799, 599}, {400, 300}} {
		if r, g, b, alpha := picture.At(at.X, at.Y).RGBA(); r>>8 != 30 || g>>8 != 144 || b>>8 != 255 || alpha>>8 != 255 {
			t.Fatalf("the pixel at %v is %v, not the page's", at, picture.At(at.X, at.Y))
		}
	}
	d.open("light://about")
	d.fails(webdriver.UnableToCaptureScreen, "GET", "/{s}/screenshot", "")
}

func TestACutOfAShotIsTheMeanOfThePixelsItStandsFor(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := range 4 {
		for x := range 8 {
			picture.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 20), A: 255})
		}
	}
	got := cut(shot{picture: picture, scale: 2}, geom.Rect{X: 1, Y: 0, W: 2, H: 2}, 2, 2)
	want := map[image.Point]color.NRGBA{
		{0, 0}: {R: 25, G: 10, A: 255}, {1, 0}: {R: 45, G: 10, A: 255},
		{0, 1}: {R: 25, G: 50, A: 255}, {1, 1}: {R: 45, G: 50, A: 255},
	}
	for at, c := range want {
		if got.NRGBAAt(at.X, at.Y) != c {
			t.Errorf("the pixel at %v is %v, want %v", at, got.NRGBAAt(at.X, at.Y), c)
		}
	}
}
