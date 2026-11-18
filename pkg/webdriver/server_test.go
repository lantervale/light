package webdriver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/webdriver"
)

func about() webdriver.About {
	return webdriver.About{Name: "light", Version: "1.2.3", Platform: "mac", UserAgent: "Mozilla/5.0 light/1.2.3"}
}

// driven is a Server over a fake browser, and a client of it.
type driven struct {
	t       *testing.T
	browser *fake
	srv     *httptest.Server
	session string
}

func drive(t *testing.T) *driven {
	t.Helper()
	browser := &fake{handles: []string{"w1"}}
	srv := httptest.NewServer(webdriver.New(browser, about()))
	t.Cleanup(srv.Close)
	return &driven{t: t, browser: browser, srv: srv}
}

// answer is what a command was answered: its status, its value, and
// the error code of a value that is an error.
type answer struct {
	status int
	value  json.RawMessage
	code   string
	text   string
}

func (d *driven) ask(method, path, body string) answer {
	d.t.Helper()
	return d.send(method, path, body, nil)
}

func (d *driven) send(method, path, body string, header map[string]string) answer {
	d.t.Helper()
	var sent io.Reader
	if method == http.MethodPost {
		sent = strings.NewReader(body)
	}
	path = strings.ReplaceAll(path, "{s}", d.session)
	req, err := http.NewRequestWithContext(context.Background(), method, d.srv.URL+path, sent)
	if err != nil {
		d.t.Fatal(err)
	}
	for name, value := range header {
		if name == "Host" {
			req.Host = value
			continue
		}
		req.Header.Set(name, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		d.t.Fatal(err)
	}
	defer res.Body.Close()
	var whole struct {
		Value json.RawMessage `json:"value"`
	}
	raw, _ := io.ReadAll(res.Body)
	if err = json.Unmarshal(raw, &whole); err != nil || whole.Value == nil {
		d.t.Fatalf("%s %s answered %q, no JSON object with a value", method, path, raw)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		d.t.Errorf("%s %s is answered as %q", method, path, got)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		d.t.Errorf("%s %s is answered with Cache-Control %q", method, path, got)
	}
	a := answer{status: res.StatusCode, value: whole.Value, text: string(whole.Value)}
	var failed struct {
		Error      *string `json:"error"`
		Message    *string `json:"message"`
		Stacktrace *string `json:"stacktrace"`
	}
	if res.StatusCode != http.StatusOK {
		if json.Unmarshal(whole.Value, &failed) != nil || failed.Error == nil || failed.Message == nil ||
			failed.Stacktrace == nil {
			d.t.Fatalf("%s %s failed with %s, no error with a message and a stacktrace", method, path, whole.Value)
		}
		a.code = *failed.Error
	}
	return a
}

// begin creates the session.
func (d *driven) begin(capabilities string) {
	d.t.Helper()
	a := d.ask("POST", "/session", `{"capabilities":`+capabilities+`}`)
	var made struct {
		SessionID string `json:"sessionId"`
	}
	if a.status != http.StatusOK || json.Unmarshal(a.value, &made) != nil || made.SessionID == "" {
		d.t.Fatalf("no session was created: %d %s", a.status, a.text)
	}
	d.session = made.SessionID
}

func TestStatusSaysWhetherASessionCanBegin(t *testing.T) {
	d := drive(t)
	if a := d.ask("GET", "/status", ""); a.status != 200 || !strings.Contains(a.text, `"ready":true`) {
		t.Fatalf("before a session the status is %d %s", a.status, a.text)
	}
	d.begin("{}")
	if a := d.ask("GET", "/status", ""); !strings.Contains(a.text, `"ready":false`) {
		t.Fatalf("during a session the status is %s", a.text)
	}
}

func TestANewSessionAnswersItsCapabilities(t *testing.T) {
	d := drive(t)
	a := d.ask("POST", "/session", `{"capabilities":{"alwaysMatch":{"pageLoadStrategy":"eager",
		"timeouts":{"script":null,"implicit":250},"unhandledPromptBehavior":"accept","moz:firefoxOptions":{"a":1}},
		"firstMatch":[{"browserName":"other"},{"browserName":"light","platformName":"mac"}]}}`)
	var made struct {
		SessionID    string         `json:"sessionId"`
		Capabilities map[string]any `json:"capabilities"`
	}
	if a.status != 200 || json.Unmarshal(a.value, &made) != nil {
		t.Fatalf("the session was answered %d %s", a.status, a.text)
	}
	want := map[string]any{
		"browserName": "light", "browserVersion": "1.2.3", "platformName": "mac", "acceptInsecureCerts": false,
		"pageLoadStrategy": "eager", "proxy": map[string]any{}, "setWindowRect": true,
		"strictFileInteractability": false, "unhandledPromptBehavior": "accept",
		"userAgent": "Mozilla/5.0 light/1.2.3",
		"timeouts":  map[string]any{"script": nil, "pageLoad": 300000.0, "implicit": 250.0},
	}
	if !reflect.DeepEqual(made.Capabilities, want) {
		t.Errorf("the capabilities are %v, want %v", made.Capabilities, want)
	}
	got := d.browser.settings
	if got.PageLoadStrategy != "eager" || !got.Timeouts.ScriptEndless || got.Timeouts.Implicit != 250*time.Millisecond ||
		string(got.Extensions["moz:firefoxOptions"]) != `{"a":1}` {
		t.Errorf("the browser was given %+v", got)
	}
}

func TestCapabilitiesAreValidatedAndMatched(t *testing.T) {
	for _, c := range []struct {
		body, code string
	}{
		{`{}`, "invalid argument"},
		{`{"capabilities":[]}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":[]}}`, "invalid argument"},
		{`{"capabilities":{"firstMatch":[]}}`, "invalid argument"},
		{`{"capabilities":{"firstMatch":{}}}`, "invalid argument"},
		{`{"capabilities":{"firstMatch":[1]}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"unknown":1}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"acceptInsecureCerts":"yes"}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"browserName":1}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"pageLoadStrategy":"fast"}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"proxy":1}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"timeouts":{"script":-1}}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"timeouts":{"implicit":1.5}}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"unhandledPromptBehavior":"shout"}}}`, "invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"browserName":"light"},"firstMatch":[{"browserName":"light"}]}}`,
			"invalid argument"},
		{`{"capabilities":{"alwaysMatch":{"browserName":"chrome"}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"browserVersion":"9"}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"platformName":"linux"}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"acceptInsecureCerts":true}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"webSocketUrl":true}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"proxy":{"proxyType":"pac"}}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"setWindowRect":false}}}`, "session not created"},
		{`{"capabilities":{"alwaysMatch":{"browserName":null,"acceptInsecureCerts":false,"webSocketUrl":false,
			"proxy":{},"setWindowRect":true,"unhandledPromptBehavior":{"alert":"accept","default":"ignore"}}}}`, ""},
		{`{"capabilities":{"alwaysMatch":null,"firstMatch":null}}`, ""},
	} {
		d := drive(t)
		a := d.ask("POST", "/session", c.body)
		if a.code != c.code {
			t.Errorf("%s is answered %q (%s), want %q", c.body, a.code, a.text, c.code)
		}
		if created := len(d.browser.calls) > 0; created != (c.code == "") {
			t.Errorf("%s: the browser was asked %v", c.body, d.browser.calls)
		}
	}
}

func TestThereIsOneSessionAtATime(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	first := d.session
	if a := d.ask("POST", "/session", `{"capabilities":{}}`); a.code != "session not created" || a.status != 500 {
		t.Fatalf("a second session is answered %d %s", a.status, a.text)
	}
	if a := d.ask("DELETE", "/session/{s}", ""); a.status != 200 || a.text != "null" {
		t.Fatalf("deleting the session is answered %d %s", a.status, a.text)
	}
	if a := d.ask("GET", "/session/{s}/url", ""); a.code != "invalid session id" || a.status != 404 {
		t.Fatalf("a command of an ended session is answered %d %s", a.status, a.text)
	}
	d.begin("{}")
	if d.session == first {
		t.Fatal("the second session has the id of the first")
	}
	if got := strings.Join(d.browser.calls, ","); got != "NewSession,EndSession,NewSession" {
		t.Fatalf("the browser was asked %s", got)
	}
}

func TestRoutingAnswersTheErrorsOfTheStandard(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	for _, c := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"GET", "/nothing", "", 404, "unknown command"},
		{"GET", "/session/{s}/nothing", "", 404, "unknown command"},
		{"POST", "/session/{s}/print", "{}", 404, "unknown command"},
		{"GET", "/session/{s}/element/e1/computedrole", "", 404, "unknown command"},
		{"PUT", "/session/{s}/url", "", 405, "unknown method"},
		{"DELETE", "/session/{s}/url", "", 405, "unknown method"},
		{"GET", "/session", "", 405, "unknown method"},
		{"POST", "/status", "{}", 405, "unknown method"},
		{"GET", "/session/other/url", "", 404, "invalid session id"},
		{"POST", "/session/{s}/url", "", 400, "invalid argument"},
		{"POST", "/session/{s}/url", "[1]", 400, "invalid argument"},
		{"POST", "/session/{s}/url", "{nope", 400, "invalid argument"},
		{"POST", "/session/{s}/url", `{"url":1}`, 400, "invalid argument"},
		{"POST", "/session/{s}/window", `{}`, 400, "invalid argument"},
		{"POST", "/session/{s}/window/new", `{"type":1}`, 400, "invalid argument"},
		{"POST", "/session/{s}/window/rect", `{"width":-1}`, 400, "invalid argument"},
		{"POST", "/session/{s}/window/rect", `{"x":"left"}`, 400, "invalid argument"},
		{"POST", "/session/{s}/frame", `{}`, 400, "invalid argument"},
		{"POST", "/session/{s}/frame", `{"id":"name"}`, 400, "invalid argument"},
		{"POST", "/session/{s}/frame", `{"id":70000}`, 400, "invalid argument"},
		{"POST", "/session/{s}/element", `{"using":"id","value":"a"}`, 400, "invalid argument"},
		{"POST", "/session/{s}/element", `{"using":"css selector"}`, 400, "invalid argument"},
		{"POST", "/session/{s}/element", `{"using":"css selector","value":"p"}`, 404, "no such element"},
		{"POST", "/session/{s}/execute/sync", `{"script":"return 1"}`, 400, "invalid argument"},
		{"POST", "/session/{s}/execute/sync", `{"script":1,"args":[]}`, 400, "invalid argument"},
		{"POST", "/session/{s}/execute/async", `{"script":"1","args":{}}`, 400, "invalid argument"},
		{"POST", "/session/{s}/timeouts", `{"script":"long"}`, 400, "invalid argument"},
		{"POST", "/session/{s}/timeouts", `{"pageLoad":null}`, 400, "invalid argument"},
		{"POST", "/session/{s}/timeouts", `{"implicit":9007199254740992}`, 400, "invalid argument"},
		{"POST", "/session/{s}/cookie", `{"cookie":{"name":"a"}}`, 400, "invalid argument"},
		{"POST", "/session/{s}/cookie", `{"cookie":{"name":"a","value":"b","sameSite":"Loose"}}`, 400,
			"invalid argument"},
		{"POST", "/session/{s}/cookie", `{"cookie":{"name":"a","value":"b","expiry":"soon"}}`, 400, "invalid argument"},
		{"GET", "/session/{s}/cookie/none", "", 404, "no such cookie"},
		{"POST", "/session/{s}/element/e1/value", `{}`, 400, "invalid argument"},
		{"POST", "/session/{s}/alert/accept", `{}`, 404, "no such alert"},
		{"POST", "/session/{s}/alert/dismiss", `{}`, 404, "no such alert"},
		{"GET", "/session/{s}/alert/text", "", 404, "no such alert"},
		{"POST", "/session/{s}/alert/text", `{"text":"a"}`, 404, "no such alert"},
		{"GET", "/session/{s}/element/e1/shadow", "", 404, "no such shadow root"},
		{"POST", "/session/{s}/shadow/r1/element", `{"using":"css selector","value":"p"}`, 404,
			"no such shadow root"},
	} {
		if a := d.ask(c.method, c.path, c.body); a.status != c.status || a.code != c.code {
			t.Errorf("%s %s %s is answered %d %q, want %d %q", c.method, c.path, c.body, a.status, a.code,
				c.status, c.code)
		}
	}
}

func TestCommandsReachTheBrowserAndAnswerItsValues(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	d.browser.found = []string{"e1", "e2"}
	d.browser.cookies = []webdriver.Cookie{{Name: "a", Value: "1", Path: "/", Domain: "a.test", SameSite: "Lax"}}
	el := `{"` + webdriver.ElementKey + `":"e1"}`
	for _, c := range []struct {
		method, path, body, call, value string
	}{
		{"POST", "url", `{"url":"http://a.test/x"}`, "Navigate http://a.test/x", `null`},
		{"GET", "url", "", "CurrentURL", `"http://a.test/"`},
		{"POST", "back", `{}`, "Back", `null`},
		{"POST", "forward", `{}`, "Forward", `null`},
		{"POST", "refresh", `{}`, "Refresh", `null`},
		{"GET", "title", "", "Title", `"A title"`},
		{"GET", "source", "", "PageSource", `"<html></html>"`},
		{"GET", "window", "", "WindowHandle", `"w1"`},
		{"GET", "window/handles", "", "WindowHandles", `["w1"]`},
		{"POST", "window", `{"handle":"w2"}`, "SwitchToWindow w2", `null`},
		{"POST", "window/new", `{}`, "NewWindow tab", `{"handle":"w2","type":"tab"}`},
		{"POST", "window/new", `{"type":"window"}`, "NewWindow window", `{"handle":"w2","type":"window"}`},
		{"POST", "window/new", `{"type":"other"}`, "NewWindow tab", `{"handle":"w2","type":"tab"}`},
		{"GET", "window/rect", "", "WindowRect", `{"x":1,"y":2,"width":800,"height":600}`},
		{"POST", "window/rect", `{"width":800,"height":600,"x":null}`, "SetWindowRect",
			`{"x":0,"y":0,"width":800,"height":600}`},
		{"POST", "window/maximize", `{}`, "Maximize", `{"x":0,"y":0,"width":0,"height":0}`},
		{"POST", "window/minimize", `{}`, "Minimize", `{"x":0,"y":0,"width":0,"height":0}`},
		{"POST", "window/fullscreen", `{}`, "Fullscreen", `{"x":0,"y":0,"width":0,"height":0}`},
		{"POST", "frame", `{"id":null}`, "SwitchToFrame", `null`},
		{"POST", "frame/parent", `{}`, "SwitchToParentFrame", `null`},
		{"GET", "element/active", "", "ActiveElement", `{"` + webdriver.ElementKey + `":"e9"}`},
		{"POST", "element", `{"using":"css selector","value":"p"}`, "FindElements", el},
		{"POST", "elements", `{"using":"tag name","value":"p"}`, "FindElements",
			`[` + el + `,{"` + webdriver.ElementKey + `":"e2"}]`},
		{"POST", "element/e7/element", `{"using":"link text","value":"p"}`, "FindElements", el},
		{"POST", "element/e7/elements", `{"using":"xpath","value":"//p"}`, "FindElements",
			`[` + el + `,{"` + webdriver.ElementKey + `":"e2"}]`},
		{"GET", "element/e1/selected", "", "ElementSelected e1", `false`},
		{"GET", "element/e1/attribute/data-a%2Fb", "", "ElementAttribute e1 data-a/b", `"v"`},
		{"GET", "element/e1/attribute/missing", "", "ElementAttribute e1 missing", `null`},
		{"GET", "element/e1/property/value", "", "ElementProperty e1 value", `{"a":[1,null]}`},
		{"GET", "element/e1/css/color", "", "ElementCSSValue e1 color", `"red"`},
		{"GET", "element/e1/text", "", "ElementText e1", `"words"`},
		{"GET", "element/e1/name", "", "ElementTagName e1", `"p"`},
		{"GET", "element/e1/rect", "", "ElementRect e1", `{"x":3,"y":4,"width":5,"height":6}`},
		{"GET", "element/e1/enabled", "", "ElementEnabled e1", `true`},
		{"GET", "element/e1/displayed", "", "ElementDisplayed e1", `true`},
		{"POST", "element/e1/click", `{}`, "ElementClick e1", `null`},
		{"POST", "element/e1/clear", `{}`, "ElementClear e1", `null`},
		{"POST", "element/e1/value", `{"text":"abc"}`, "ElementSendKeys e1", `null`},
		{"POST", "execute/sync", `{"script":"return 1","args":[1]}`, "Execute",
			`[1,{"` + webdriver.ElementKey + `":"e1"}]`},
		{"GET", "cookie", "", "Cookies",
			`[{"name":"a","value":"1","path":"/","domain":"a.test","secure":false,"httpOnly":false,"sameSite":"Lax"}]`},
		{"GET", "cookie/a", "", "Cookies",
			`{"name":"a","value":"1","path":"/","domain":"a.test","secure":false,"httpOnly":false,"sameSite":"Lax"}`},
		{"POST", "cookie", `{"cookie":{"name":"n","value":"v","expiry":12,"secure":true}}`, "AddCookie", `null`},
		{"DELETE", "cookie/a", "", "DeleteCookies a", `null`},
		{"DELETE", "cookie", "", "DeleteCookies ", `null`},
		{"POST", "actions", `{"actions":[]}`, "PerformActions", `null`},
		{"DELETE", "actions", "", "ReleaseActions", `null`},
		{"GET", "screenshot", "", "Screenshot", `"cG5n"`},
		{"GET", "element/e1/screenshot", "", "ElementScreenshot e1", `"cG5n"`},
	} {
		d.browser.calls = nil
		a := d.ask(c.method, "/session/{s}/"+c.path, c.body)
		if a.status != 200 || a.text != c.value {
			t.Errorf("%s %s is answered %d %s, want %s", c.method, c.path, a.status, a.text, c.value)
		}
		if got := strings.Join(d.browser.calls, ","); got != c.call {
			t.Errorf("%s %s asked the browser %q, want %q", c.method, c.path, got, c.call)
		}
	}
	if got := d.browser.cookie; got.Name != "n" || got.Expiry == nil || *got.Expiry != 12 || !got.Secure {
		t.Errorf("the cookie added is %+v", got)
	}
	if got := d.browser.locator; got != (webdriver.Locator{Using: "xpath", Value: "//p", From: "e7"}) {
		t.Errorf("the last locator is %+v", got)
	}
	if got := d.browser.placed; got.X != nil || got.Y != nil || got.Width == nil || *got.Width != 800 {
		t.Errorf("the window was placed at %+v", got)
	}
}

func TestAnErrorOfTheBrowserIsSentWithItsCodeAndStatus(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	d.browser.err = &webdriver.Error{Code: webdriver.JavaScriptError, Message: "boom", Stack: "at a (b:1:2)"}
	a := d.ask("POST", "/session/{s}/execute/sync", `{"script":"x","args":[]}`)
	if a.status != 500 || a.text != `{"error":"javascript error","message":"boom","stacktrace":"at a (b:1:2)"}` {
		t.Fatalf("a script's error is answered %d %s", a.status, a.text)
	}
	d.browser.err = context.DeadlineExceeded
	if a = d.ask("GET", "/session/{s}/title", ""); a.status != 500 || a.code != "unknown error" {
		t.Fatalf("an error with no code is answered %d %s", a.status, a.text)
	}
}

func TestEveryCodeHasTheStatusOfTheStandard(t *testing.T) {
	for code, status := range map[webdriver.Code]int{
		webdriver.ElementClickIntercepted: 400, webdriver.ElementNotInteractable: 400,
		webdriver.InsecureCertificate: 400, webdriver.InvalidArgument: 400, webdriver.InvalidCookieDomain: 400,
		webdriver.InvalidElementState: 400, webdriver.InvalidSelector: 400, webdriver.InvalidSessionID: 404,
		webdriver.JavaScriptError: 500, webdriver.MoveTargetOutOfBounds: 500, webdriver.NoSuchAlert: 404,
		webdriver.NoSuchCookie: 404, webdriver.NoSuchElement: 404, webdriver.NoSuchFrame: 404,
		webdriver.NoSuchWindow: 404, webdriver.NoSuchShadowRoot: 404, webdriver.ScriptTimeout: 500,
		webdriver.SessionNotCreated: 500, webdriver.StaleElementReference: 404, webdriver.DetachedShadowRoot: 404,
		webdriver.Timeout: 500, webdriver.UnableToSetCookie: 500, webdriver.UnableToCaptureScreen: 500,
		webdriver.UnexpectedAlertOpen: 500, webdriver.UnknownCommand: 404, webdriver.UnknownError: 500,
		webdriver.UnknownMethod: 405, webdriver.UnsupportedOperation: 500,
	} {
		if got := code.Status(); got != status || !code.Known() {
			t.Errorf("%q has the status %d, want %d", code, got, status)
		}
	}
	if webdriver.Code("made up").Known() {
		t.Error("a made up code is known")
	}
}

func TestTimeoutsAreKeptAndGivenToTheCommands(t *testing.T) {
	d := drive(t)
	d.begin(`{"alwaysMatch":{"pageLoadStrategy":"none"}}`)
	if a := d.ask("GET", "/session/{s}/timeouts", ""); a.text != `{"implicit":0,"pageLoad":300000,"script":30000}` {
		t.Fatalf("the timeouts of a new session are %s", a.text)
	}
	if a := d.ask("POST", "/session/{s}/timeouts", `{"script":1500,"pageLoad":700,"implicit":40}`); a.text != "null" {
		t.Fatalf("setting the timeouts is answered %s", a.text)
	}
	if a := d.ask("POST", "/session/{s}/timeouts", `{"script":null}`); a.text != "null" {
		t.Fatalf("an endless script timeout is answered %s", a.text)
	}
	if a := d.ask("GET", "/session/{s}/timeouts", ""); a.text != `{"implicit":40,"pageLoad":700,"script":null}` {
		t.Fatalf("the timeouts are %s", a.text)
	}
	d.ask("POST", "/session/{s}/url", `{"url":"http://a.test/"}`)
	if got := d.browser.load; got != (webdriver.Load{Strategy: "none", Timeout: 700 * time.Millisecond}) {
		t.Errorf("a navigation waits as %+v", got)
	}
	d.ask("POST", "/session/{s}/execute/async", `{"script":"x","args":[1,"a"]}`)
	if got := d.browser.script; !got.Async || !got.Endless || got.Source != "x" || string(got.Args) != `[1,"a"]` {
		t.Errorf("the script is %+v", got)
	}
	d.ask("POST", "/session/{s}/timeouts", `{"script":20}`)
	d.ask("POST", "/session/{s}/execute/sync", `{"script":"x","args":[]}`)
	if got := d.browser.script; got.Async || got.Endless || got.Timeout != 20*time.Millisecond {
		t.Errorf("the script is %+v", got)
	}
	d.ask("POST", "/session/{s}/elements", `{"using":"css selector","value":"p"}`)
	if d.browser.wait != 40*time.Millisecond {
		t.Errorf("elements are looked for during %s", d.browser.wait)
	}
}

func TestFramesAreNamedByNullANumberOrAnElement(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	for body, want := range map[string]webdriver.Frame{
		`{"id":null}`: {Top: true},
		`{"id":2}`:    {Index: 2},
		`{"id":{"` + webdriver.ElementKey + `":"e4"}}`: {Element: "e4"},
	} {
		if a := d.ask("POST", "/session/{s}/frame", body); a.status != 200 || d.browser.frame != want {
			t.Errorf("%s switches to %+v (%s), want %+v", body, d.browser.frame, a.text, want)
		}
	}
}

func TestClosingTheLastWindowEndsTheSession(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	d.browser.handles = []string{"w2"}
	if a := d.ask("DELETE", "/session/{s}/window", ""); a.text != `["w2"]` {
		t.Fatalf("closing a window of two is answered %s", a.text)
	}
	d.browser.handles = nil
	if a := d.ask("DELETE", "/session/{s}/window", ""); a.text != `[]` {
		t.Fatalf("closing the last window is answered %s", a.text)
	}
	if a := d.ask("GET", "/session/{s}/window", ""); a.code != "invalid session id" {
		t.Fatalf("after the last window the session answers %s", a.text)
	}
	if a := d.ask("GET", "/status", ""); !strings.Contains(a.text, `"ready":true`) {
		t.Fatalf("after the last window the status is %s", a.text)
	}
}

func TestOnlyAClientOfThisMachineWithNoOriginIsAnswered(t *testing.T) {
	d := drive(t)
	for _, c := range []struct {
		header map[string]string
		status int
	}{
		{nil, 200},
		{map[string]string{"Host": "localhost:4444"}, 200},
		{map[string]string{"Host": "LOCALHOST"}, 200},
		{map[string]string{"Host": "127.0.0.1"}, 200},
		{map[string]string{"Host": "127.8.9.1:80"}, 200},
		{map[string]string{"Host": "[::1]:4444"}, 200},
		{map[string]string{"Host": "evil.test:4444"}, 403},
		{map[string]string{"Host": "localhost.evil.test"}, 403},
		{map[string]string{"Host": "192.168.1.10:4444"}, 403},
		{map[string]string{"Host": "0.0.0.0:4444"}, 403},
		{map[string]string{"Origin": "https://evil.test"}, 403},
		{map[string]string{"Origin": "http://localhost:4444"}, 403},
		{map[string]string{"Origin": "null"}, 403},
	} {
		a := d.send("GET", "/status", "", c.header)
		if a.status != c.status {
			t.Errorf("a request with %v is answered %d, want %d", c.header, a.status, c.status)
		}
		if c.status == 403 && a.code != "unknown error" {
			t.Errorf("a request with %v is refused as %s", c.header, a.text)
		}
	}
	a := d.send("POST", "/session", `{"capabilities":{}}`, map[string]string{"Origin": "https://evil.test",
		"Content-Type": "text/plain"})
	if a.status != 403 || len(d.browser.calls) != 0 {
		t.Fatalf("a page's POST is answered %d and reached the browser: %v", a.status, d.browser.calls)
	}
}

func TestListenBindsThisMachineAlone(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		l, err := webdriver.Listen(context.Background(), address)
		if err != nil {
			if address == "[::1]:0" {
				continue
			}
			t.Fatalf("%s: %v", address, err)
		}
		if tcp := l.Addr().String(); !strings.HasPrefix(tcp, "127.0.0.1:") && !strings.HasPrefix(tcp, "[::1]:") {
			t.Errorf("%s listens at %s", address, tcp)
		}
		_ = l.Close()
	}
	for _, address := range []string{":4444", "0.0.0.0:4444", "[::]:4444", "192.168.1.10:4444", "example.test:4444",
		"4444", "10.0.0.1:0"} {
		l, err := webdriver.Listen(context.Background(), address)
		if err == nil {
			_ = l.Close()
			t.Errorf("%s is listened at", address)
		}
	}
}

func TestACommandWaitsForTheOneBeforeIt(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	d.browser.hold = make(chan struct{})
	first, second := make(chan answer), make(chan answer)
	go func() { first <- d.ask("POST", "/session/{s}/url", `{"url":"http://a.test/"}`) }()
	time.Sleep(50 * time.Millisecond)
	go func() { second <- d.ask("GET", "/session/{s}/title", "") }()
	select {
	case a := <-second:
		t.Fatalf("a command was answered %s while another ran", a.text)
	case <-time.After(100 * time.Millisecond):
	}
	close(d.browser.hold)
	<-first
	if a := <-second; a.text != `"A title"` {
		t.Fatalf("the second command is answered %s", a.text)
	}
	if got := strings.Join(d.browser.calls, ","); got != "NewSession,Navigate http://a.test/,Title" {
		t.Fatalf("the browser was asked %s", got)
	}
}
