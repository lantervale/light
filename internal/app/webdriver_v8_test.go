//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the driver is checked against the screens it drives: their tabs, their windows

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/webdriver"
)

// drivable is a small site for a driver: a page with a counter, a
// link, fields and a script, a page that runs tests as testharness.js
// does, and a page with a frame.
func drivable(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Driven</title><style>body { margin: 0 }
			button, a, input, p { display: block; height: 24px; margin: 0 }</style>
			<p id="count" class="shown" data-kind="counter">none</p><button id="more">More</button>
			<a id="next" href="/next">the next page</a><input id="field" value="was">
			<input id="box" type="checkbox" checked><button id="off" disabled>Off</button>
			<p id="hidden" style="display: none">hidden</p>
			<script>
			let n = 0;
			document.getElementById("more").addEventListener("click", () => {
				document.getElementById("count").textContent = "clicked " + (++n);
			});
			document.getElementById("field").addEventListener("input", (e) => { window.typed = e.target.value; });
			window.keys = [];
			document.addEventListener("keydown", (e) => window.keys.push(e.key));
			setTimeout(() => {
				const p = document.createElement("p");
				p.id = "late";
				p.textContent = "late";
				document.body.appendChild(p);
			}, 250);
			</script>`),
		"/next": typed("text/html", `<title>Next</title><p id="here">the next page</p>`),
		"/harness": typed("text/html", `<title>Harness</title><p id="log"></p><script>
			(function () {
				const tests = [], callbacks = [];
				let pending = 0, done = false;
				const finish = () => { done = true; callbacks.forEach((cb) => cb(tests, { status: 0 })); };
				window.test = (fn, name) => {
					const t = { name, status: 0, message: null };
					try { fn(); } catch (e) { t.status = 1; t.message = String(e.message); }
					tests.push(t);
				};
				window.async_test = (name) => {
					const t = { name, status: 3, message: null };
					tests.push(t);
					pending++;
					return { done() { t.status = 0; if (--pending === 0) finish(); } };
				};
				window.add_completion_callback = (cb) => { if (done) cb(tests, { status: 0 }); else callbacks.push(cb); };
				window.addEventListener("load", () => { if (pending === 0) finish(); });
			})();
			test(() => { if (1 + 1 !== 2) throw new Error("math"); }, "adds");
			test(() => { throw new Error("expected to fail"); }, "fails");
			const waits = async_test("waits");
			setTimeout(() => waits.done(), 120);
			test(() => { if (navigator.webdriver !== true) throw new Error("not driven"); }, "is driven");
			</script>`),
		"/frames": typed("text/html", `<title>Frames</title><body style="margin: 0"><p id="top">top</p>
			<iframe id="inner" src="/framed" width="300" height="100"></iframe>
			<script>window.where = "top";</script>`),
		"/framed": typed("text/html", `<title>Framed</title><body style="margin: 0"><p id="said">framed</p>
			<script>window.where = "frame";</script>`),
	})
	return srv
}

// run runs a script and gives what it returned, as JSON.
func (d *driven) run(script string, args ...any) string {
	d.t.Helper()
	if args == nil {
		args = []any{}
	}
	return d.ok("POST", "/{s}/execute/sync", string(mustJSON(d.t, map[string]any{"script": script, "args": args})))
}

// find is the reference of the one element a selector finds.
func (d *driven) find(selector string) string {
	d.t.Helper()
	var found map[string]string
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/element", `{"using":"css selector","value":`+literal(selector)+`}`)),
		&found)
	if found[webdriver.ElementKey] == "" {
		d.t.Fatalf("%s is found as %v", selector, found)
	}
	return found[webdriver.ElementKey]
}

func drivenAt(t *testing.T, path string) (*driven, string) {
	t.Helper()
	srv := drivable(t)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + path)
	return d, srv.URL
}

func TestAScriptRunsInThePageAndItsValueComesBack(t *testing.T) {
	d, _ := drivenAt(t, "/")
	for _, c := range []struct {
		script string
		args   []any
		want   string
	}{
		{`return 1 + 1`, nil, `2`},
		{`return "a" + arguments[0] + arguments[1]`, []any{"b", 3}, `"ab3"`},
		{``, nil, `null`},
		{`return undefined`, nil, `null`},
		{`return null`, nil, `null`},
		{`return true`, nil, `true`},
		{`return 0 / 0`, nil, `null`},
		{`return [1, "two", null, undefined, [3]]`, nil, `[1,"two",null,null,[3]]`},
		{`return {a: 1, b: {c: [true]}, d: undefined}`, nil, `{"a":1,"b":{"c":[true]},"d":null}`},
		{`return arguments[0]`, []any{map[string]any{"a": []any{1, "x"}, "b": nil}}, `{"a":[1,"x"],"b":null}`},
		{`return arguments.length`, []any{1, 2, 3}, `3`},
		{`return document.title`, nil, `"Driven"`},
		{`return navigator.webdriver`, nil, `true`},
		{`return this === window`, nil, `true`},
		{`return new Promise((resolve) => setTimeout(() => resolve("later"), 50))`, nil, `"later"`},
		{`return Promise.resolve({a: 1})`, nil, `{"a":1}`},
		{`return document.querySelectorAll("button").length`, nil, `2`},
		{`return new Date(0)`, nil, `"1970-01-01T00:00:00.000Z"`},
		{`window.kept = 5`, nil, `null`},
		{`return window.kept`, nil, `5`},
		{`return document.readyState`, nil, `"complete"`},
	} {
		if got := d.run(c.script, c.args...); got != c.want {
			t.Errorf("%q returned %s, want %s", c.script, got, c.want)
		}
	}
}

func TestAScriptsFailureIsAJavaScriptError(t *testing.T) {
	d, _ := drivenAt(t, "/")
	for _, c := range []struct{ script, message string }{
		{`throw new TypeError("no good")`, "TypeError: no good"},
		{`return missing.name`, "ReferenceError"},
		{`return Promise.reject(new Error("rejected"))`, "Error: rejected"},
		{`const a = {}; a.self = a; return a`, "refers to itself"},
		{`return (`, "SyntaxError"},
		{`return 10n`, "bigint"},
	} {
		a := d.fails(webdriver.JavaScriptError, "POST", "/{s}/execute/sync", string(mustJSON(t,
			map[string]any{"script": c.script, "args": []any{}})))
		if !strings.Contains(a.message, c.message) {
			t.Errorf("%q fails with %q, want %q in it", c.script, a.message, c.message)
		}
	}
	a := d.fails(webdriver.JavaScriptError, "POST", "/{s}/execute/sync",
		`{"script":"function inner() { throw new Error('deep'); } inner()","args":[]}`)
	if !strings.Contains(a.stack, "inner") {
		t.Errorf("the stack of a script's error is %q", a.stack)
	}
	if got := d.run(`return 1`); got != `1` {
		t.Fatalf("after its failures a script returns %s", got)
	}
}

func TestAnAsyncScriptEndsWhenItsCallbackIsCalled(t *testing.T) {
	d, _ := drivenAt(t, "/")
	async := func(script string, args ...any) wdReply {
		if args == nil {
			args = []any{}
		}
		return d.ask("POST", "/{s}/execute/async", string(mustJSON(t, map[string]any{"script": script, "args": args})))
	}
	for _, c := range []struct {
		script string
		args   []any
		want   string
	}{
		{`arguments[0](7)`, nil, `7`},
		{`const done = arguments[arguments.length - 1]; setTimeout(() => done([arguments[0], "late"]), 80)`,
			[]any{"x"}, `["x","late"]`},
		{`arguments[0]()`, nil, `null`},
		{`const done = arguments[0]; Promise.resolve().then(() => done({ok: true})); return "ignored"`, nil,
			`{"ok":true}`},
		{`const done = arguments[0]; done(1); done(2)`, nil, `1`},
	} {
		if a := async(c.script, c.args...); a.status != 200 || a.text != c.want {
			t.Errorf("%q answered %d %s, want %s", c.script, a.status, a.text, c.want)
		}
	}
	if a := async(`throw new Error("at once")`); a.code != "javascript error" || !strings.Contains(a.message, "at once") {
		t.Errorf("an async script that throws is answered %s", a.text)
	}
	if a := async(`return Promise.reject(new Error("refused"))`); a.code != "javascript error" {
		t.Errorf("an async script whose promise is rejected is answered %s", a.text)
	}
	d.ok("POST", "/{s}/timeouts", `{"script":200}`)
	began := time.Now()
	a := async(`window.never = arguments[0]`)
	if took := time.Since(began); a.code != "script timeout" || a.status != 500 || took < 200*time.Millisecond ||
		took > 5*time.Second {
		t.Fatalf("a script that never calls back is answered %s after %s", a.text, took)
	}
	if a = d.ask("POST", "/{s}/execute/sync", `{"script":"return new Promise(() => {})","args":[]}`); a.code !=
		"script timeout" {
		t.Fatalf("a promise that never settles is answered %s", a.text)
	}
	d.ok("POST", "/{s}/timeouts", `{"script":null}`)
	if a = async(`const done = arguments[0]; setTimeout(() => done("endless"), 300)`); a.text != `"endless"` {
		t.Fatalf("with no script timeout a slow script is answered %s", a.text)
	}
}

func TestElementsAreFoundAndRead(t *testing.T) {
	d, _ := drivenAt(t, "/")
	count := d.find("#count")
	if again := d.find("p.shown"); again != count {
		t.Fatalf("an element found twice is %q then %q", count, again)
	}
	el := func(path string) string { return d.ok("GET", "/{s}/element/"+path, "") }
	for path, want := range map[string]string{
		count + "/text":                        `"none"`,
		count + "/name":                        `"p"`,
		count + "/attribute/data-kind":         `"counter"`,
		count + "/attribute/missing":           `null`,
		count + "/property/id":                 `"count"`,
		count + "/property/nothing":            `null`,
		count + "/css/display":                 `"block"`,
		count + "/enabled":                     `true`,
		count + "/displayed":                   `true`,
		count + "/selected":                    `false`,
		d.find("#off") + "/enabled":            `false`,
		d.find("#off") + "/attribute/disabled": `"true"`,
		d.find("#box") + "/selected":           `true`,
		d.find("#hidden") + "/displayed":       `false`,
		d.find("#field") + "/property/value":   `"was"`,
	} {
		if got := el(path); got != want {
			t.Errorf("%s is %s, want %s", strings.SplitN(path, "/", 2)[1], got, want)
		}
	}
	var rect webdriver.Rect
	_ = json.Unmarshal([]byte(el(d.find("#more")+"/rect")), &rect)
	if rect.Y != 24 || rect.Height != 24 || rect.Width <= 0 {
		t.Errorf("the second block is at %+v", rect)
	}
	for body, want := range map[string]int{
		`{"using":"css selector","value":"button"}`:     2,
		`{"using":"tag name","value":"input"}`:          2,
		`{"using":"link text","value":"the next page"}`: 1,
		`{"using":"partial link text","value":"next"}`:  1,
		`{"using":"link text","value":"next"}`:          0,
		`{"using":"css selector","value":"article"}`:    0,
	} {
		var found []map[string]string
		_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/elements", body)), &found)
		if len(found) != want {
			t.Errorf("%s finds %d elements, want %d", body, len(found), want)
		}
	}
	var within map[string]string
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/element/"+d.find("body")+"/element",
		`{"using":"css selector","value":"#next"}`)), &within)
	if within[webdriver.ElementKey] != d.find("#next") {
		t.Errorf("an element found from another is %v", within)
	}
	d.fails(webdriver.NoSuchElement, "POST", "/{s}/element", `{"using":"css selector","value":"article"}`)
	d.fails(webdriver.InvalidSelector, "POST", "/{s}/element", `{"using":"css selector","value":"p[["}`)
	d.fails(webdriver.InvalidSelector, "POST", "/{s}/element", `{"using":"xpath","value":"//p["}`)
	d.fails(webdriver.NoSuchElement, "GET", "/{s}/element/unknown/text", "")
	if got := d.run(`return [document.getElementById("count"), arguments[0].id]`,
		map[string]string{webdriver.ElementKey: count}); got != `[{"`+webdriver.ElementKey+`":"`+count+`"},"count"]` {
		t.Errorf("an element through a script is %s", got)
	}
	d.run(`arguments[0].remove()`, map[string]string{webdriver.ElementKey: count})
	d.fails(webdriver.StaleElementReference, "GET", "/{s}/element/"+count+"/text", "")
	d.fails(webdriver.StaleElementReference, "POST", "/{s}/execute/sync", string(mustJSON(t, map[string]any{
		"script": "return 1", "args": []any{map[string]string{webdriver.ElementKey: count}}})))
	if got := d.text("/{s}/source"); !strings.HasPrefix(got, "<html><head><title>Driven</title>") ||
		!strings.Contains(got, `id="more"`) || strings.Contains(got, `id="count"`) {
		t.Errorf("the source is %q", got)
	}
}

func TestAnElementIsWaitedForAsLongAsTheImplicitTimeout(t *testing.T) {
	d, _ := drivenAt(t, "/")
	d.run(`setTimeout(() => { const p = document.createElement("p"); p.id = "soon"; document.body.appendChild(p); },
		200)`)
	d.fails(webdriver.NoSuchElement, "POST", "/{s}/element", `{"using":"css selector","value":"#soon"}`)
	d.ok("POST", "/{s}/timeouts", `{"implicit":5000}`)
	if d.find("#soon") == "" {
		t.Fatal("an element added later is not found")
	}
	began := time.Now()
	d.ok("POST", "/{s}/timeouts", `{"implicit":150}`)
	d.fails(webdriver.NoSuchElement, "POST", "/{s}/element", `{"using":"css selector","value":"#never"}`)
	if took := time.Since(began); took < 150*time.Millisecond {
		t.Fatalf("an element that never comes was looked for during %s", took)
	}
}

func TestAClickAndKeysChangeThePage(t *testing.T) {
	d, base := drivenAt(t, "/")
	more := d.find("#more")
	d.ok("POST", "/{s}/element/"+more+"/click", "{}")
	d.ok("POST", "/{s}/element/"+more+"/click", "{}")
	if got := d.text("/{s}/element/" + d.find("#count") + "/text"); got != "clicked 2" {
		t.Fatalf("after two clicks the counter reads %q", got)
	}
	field := d.find("#field")
	d.ok("POST", "/{s}/element/"+field+"/value", `{"text":" then typed"}`)
	if got := d.run(`return [document.getElementById("field").value, window.typed, document.activeElement.id]`); got !=
		`["was then typed","was then typed","field"]` {
		t.Fatalf("after Send Keys the field holds %s", got)
	}
	if got := d.ok("GET", "/{s}/element/active", ""); got != `{"`+webdriver.ElementKey+`":"`+field+`"}` {
		t.Fatalf("the active element is %s", got)
	}
	d.ok("POST", "/{s}/element/"+field+"/value", `{"text":"!"}`)
	if got := d.run(`return document.getElementById("field").value`); got != `"was then typ!"` {
		t.Fatalf("after Backspace twice the field holds %s", got)
	}
	d.ok("POST", "/{s}/element/"+field+"/clear", "{}")
	if got := d.run(`return [document.getElementById("field").value, window.typed]`); got != `["",""]` {
		t.Fatalf("after Clear the field holds %s", got)
	}
	d.fails(webdriver.InvalidElementState, "POST", "/{s}/element/"+more+"/clear", "{}")
	d.fails(webdriver.ElementNotInteractable, "POST", "/{s}/element/"+d.find("#hidden")+"/click", "{}")
	d.run(`window.keys = []`)
	d.ok("POST", "/{s}/actions", `{"actions":[{"type":"pointer","id":"mouse","actions":[
		{"type":"pointerMove","x":0,"y":0,"origin":{"`+webdriver.ElementKey+`":"`+more+`"}},
		{"type":"pointerDown","button":0},{"type":"pointerUp","button":0}]},
		{"type":"key","id":"keys","actions":[{"type":"pause"},{"type":"pause"},{"type":"pause"},
		{"type":"keyDown","value":"k"},{"type":"keyUp","value":"k"},
		{"type":"keyDown","value":""},{"type":"keyUp","value":""}]}]}`)
	if got := d.run(`return [document.getElementById("count").textContent, window.keys]`); got !=
		`["clicked 3",["k","Enter"]]` {
		t.Fatalf("after the actions the page has %s", got)
	}
	d.ok("DELETE", "/{s}/actions", "")
	d.ok("POST", "/{s}/element/"+d.find("#next")+"/click", "{}")
	if got := d.text("/{s}/url"); got != base+"/next" || d.text("/{s}/title") != "Next" {
		t.Fatalf("a click on a link answered at %q", got)
	}
	d.fails(webdriver.NoSuchElement, "GET", "/{s}/element/"+more+"/text", "")
	d.clean()
}

func TestAPageOfTestsIsRunAsWPTRunsOne(t *testing.T) {
	d, base := drivenAt(t, "/")
	runner := d.text("/{s}/window")
	d.run(`document.title = "runner"`)
	var opened newWindow
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/window/new", `{"type":"tab"}`)), &opened)
	d.ok("POST", "/{s}/window", `{"handle":"`+opened.Handle+`"}`)
	loaded := `const done = arguments[arguments.length - 1];
		if (document.readyState === "complete") done(); else window.addEventListener("load", () => done());`
	if got := d.ok("POST", "/{s}/execute/async", string(mustJSON(t, map[string]any{"script": loaded,
		"args": []any{}}))); got != `null` {
		t.Fatalf("the empty page of a new window answered %s", got)
	}
	var root map[string]string
	_ = json.Unmarshal([]byte(d.run(`return document.documentElement;`)), &root)
	d.ok("POST", "/{s}/element/"+root[webdriver.ElementKey]+"/click", "{}")
	d.ok("POST", "/{s}/timeouts", `{"script":15000}`)
	d.open(base + "/harness")
	resume := `const callback = arguments[arguments.length - 1], url = arguments[0];
		add_completion_callback((tests, status) => {
			callback([url, "complete", [status.status, tests.map((t) => [t.name, t.status, t.message])]]);
		});`
	got := d.ok("POST", "/{s}/execute/async", string(mustJSON(t, map[string]any{"script": resume,
		"args": []any{"/harness"}})))
	want := `["/harness","complete",[0,[["adds",0,null],["fails",1,"expected to fail"],["waits",0,null],` +
		`["is driven",0,null]]]]`
	if got != want {
		t.Fatalf("the tests answered\n%s, want\n%s", got, want)
	}
	if left := d.ok("DELETE", "/{s}/window", ""); left != `["`+runner+`"]` {
		t.Fatalf("closing the test's window leaves %s", left)
	}
	d.ok("POST", "/{s}/window", `{"handle":"`+runner+`"}`)
	if title := d.text("/{s}/title"); title != "runner" {
		t.Fatalf("the runner's window is titled %q", title)
	}
}

func TestAFrameIsSwitchedTo(t *testing.T) {
	d, _ := drivenAt(t, "/frames")
	d.until("the frame's document shows", func() bool {
		set := d.session().frames[d.shownWeb()]
		return set != nil && len(set.within(nil)) == 1 && set.within(nil)[0].load == nil
	})
	if got := d.run(`return window.where`); got != `"top"` {
		t.Fatalf("the page's script says %s", got)
	}
	d.fails(webdriver.NoSuchFrame, "POST", "/{s}/frame", `{"id":3}`)
	d.fails(webdriver.NoSuchFrame, "POST", "/{s}/frame", `{"id":{"`+webdriver.ElementKey+`":"`+d.find("#top")+`"}}`)
	d.ok("POST", "/{s}/frame", `{"id":0}`)
	if got := d.run(`return [window.where, document.title, window.top === window]`); got != `["frame","Framed",false]` {
		t.Fatalf("in the frame a script says %s", got)
	}
	if got := d.text("/{s}/element/" + d.find("#said") + "/text"); got != "framed" {
		t.Fatalf("the frame's element reads %q", got)
	}
	d.fails(webdriver.NoSuchElement, "POST", "/{s}/element", `{"using":"css selector","value":"#top"}`)
	d.ok("POST", "/{s}/frame/parent", "{}")
	if got := d.run(`return window.where`); got != `"top"` {
		t.Fatalf("back in the page a script says %s", got)
	}
	d.ok("POST", "/{s}/frame", `{"id":{"`+webdriver.ElementKey+`":"`+d.find("#inner")+`"}}`)
	if got := d.run(`return window.where`); got != `"frame"` {
		t.Fatalf("in the frame named by its element a script says %s", got)
	}
	d.ok("POST", "/{s}/frame", `{"id":null}`)
	if got := d.run(`return window.where`); got != `"top"` {
		t.Fatalf("at the top a script says %s", got)
	}
}
