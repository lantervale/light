//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/frames"
)

// tab is the windows of a tab as a test says them: the page is window
// 0, in a parent 1 that is the top, with a frame 2 named kid.
type tab struct {
	posted []string
	went   []string
	closed map[int]bool
}

func (*tab) Parent(of int) int {
	if of == 2 {
		return dom.OwnWindow
	}
	return 1
}
func (*tab) Top(int) int { return 1 }
func (*tab) Length(of int) int {
	if of == dom.OwnWindow || of == 1 {
		return 1
	}
	return 0
}

func (*tab) Child(of, index int) int {
	switch {
	case index != 0:
	case of == dom.OwnWindow:
		return 2
	case of == 1:
		return dom.OwnWindow
	}
	return dom.NoWindow
}

func (*tab) Named(of int, name string) int {
	if of == dom.OwnWindow && name == "kid" {
		return 2
	}
	return dom.NoWindow
}
func (t *tab) Closed(of int) bool { return t.closed[of] }
func (*tab) Of(n *ui.Node) int {
	if n != nil {
		return 2
	}
	return dom.NoWindow
}

func (t *tab) Post(to int, message, target string) error {
	if _, ok := frames.Reaches(target, "https://site.test", "https://site.test"); !ok {
		return dom.ErrTargetOrigin
	}
	t.posted = append(t.posted, fmt.Sprintf("%d %s %s", to, target, message))
	return nil
}

func (t *tab) Go(to int, address string, replace bool) {
	t.went = append(t.went, fmt.Sprintf("%d %s %t", to, address, replace))
}

func framedPage(t *testing.T, markup string, host dom.Host) (*scripted, *tab) {
	t.Helper()
	windows := &tab{closed: map[int]bool{}}
	host.Windows = windows
	s := &scripted{t: t, answers: map[string]dom.Response{}, stores: map[string]dom.Storage{}}
	s.start(markup, host)
	return s, windows
}

// probe is the expression that says what reading or doing something
// gives: its value written down, or the name of what it throws.
func probe(expression string) string {
	return `(() => { try { const v = (` + expression + `); return typeof v === "function" ? "function" : String(v); }
		catch (e) { return "throws " + (e && e.name); } })()`
}

func TestAnotherWindowAnswersTheTableAndNothingElse(t *testing.T) {
	s, _ := framedPage(t, `<iframe id="f" name="kid" srcdoc="x"></iframe>`, dom.Host{})
	for _, w := range []string{"parent", "top", "frames[0]", "kid", "document.getElementById('f').contentWindow"} {
		for _, m := range frames.WindowMembers() {
			at := w + "." + m.Name
			got, want := s.eval(probe(at)), ""
			switch m.Shape {
			case frames.ItsWindow:
				got, want = s.eval(probe(at+" === "+w)), "true"
			case frames.AWindow, frames.ItsLocation:
				got, want = s.eval(probe("typeof "+at)), "object"
			case frames.ANumber:
				got, want = s.eval(probe("typeof "+at)), "number"
			case frames.ABoolean:
				want = "false"
			case frames.ANull:
				want = "null"
			case frames.AMethod:
				want = "function"
			case frames.ASetter:
				want = "throws SecurityError"
			}
			if got != want {
				t.Errorf("%s.%s is %s, want %s", w, m.Name, got, want)
			}
		}
		for _, name := range frames.Quiet() {
			if got := s.eval(probe(w + "." + name)); got != "undefined" {
				t.Errorf("%s.%s is %s, want undefined", w, name, got)
			}
		}
		for _, m := range frames.LocationMembers() {
			got, want := s.eval(probe(w+".location."+m.Name)), "function"
			if m.Shape == frames.ASetter {
				want = "throws SecurityError"
			}
			if got != want {
				t.Errorf("%s.location.%s is %s, want %s", w, m.Name, got, want)
			}
		}
		for _, denied := range []string{
			w + ".document", w + ".origin", w + ".name", w + ".localStorage", w + ".eval", w + ".Object", w + ".constructor",
			w + ".__proto__.x", w + ".toString", w + ".valueOf", w + "[Symbol.iterator]", w + "[Symbol.toPrimitive]",
			w + ".frameElement", w + ".history", w + ".fetch", w + ".location.href", w + ".location.origin",
			w + ".location.assign", w + ".location.toString", w + ".location.constructor", w + " + ''",
			"String(" + w + ".location)", "Object.defineProperty(" + w + ", 'x', {value: 1})", "delete " + w + ".closed",
			w + ".x = 1", w + ".name = 'x'", w + ".location.hash = '#x'", "'document' in " + w,
			"Object.getOwnPropertyDescriptor(" + w + ", 'document')",
			"Reflect.setPrototypeOf(" + w + ", {}) || (() => { throw new DOMException('', 'SecurityError'); })()",
			"JSON.stringify(" + w + ")", "structuredClone(" + w + ")",
		} {
			got := s.eval(probe(denied))
			if got != "throws SecurityError" && got != "throws DataCloneError" && got != "throws TypeError" {
				t.Errorf("%s gave %s, want it refused", denied, got)
			}
		}
		for expression, want := range map[string]string{
			"Object.getPrototypeOf(" + w + ")":                             "null",
			w + ".postMessage.constructor === Function":                    "true",
			w + ".postMessage === " + w + ".postMessage":                   "true",
			w + ".location === " + w + ".location":                         "true",
			"Object.getPrototypeOf(" + w + ".location)":                    "null",
			"Object.isFrozen(" + w + ")":                                   "false",
			"Reflect.preventExtensions(" + w + ")":                         "false",
			"Object.prototype.toString.call(" + w + ")":                    "[object Object]",
			"Reflect.ownKeys(" + w + ").includes('document')":              "false",
			"Reflect.ownKeys(" + w + ").includes('postMessage')":           "true",
			"typeof Object.getOwnPropertyDescriptor(" + w + ", 'top').get": "undefined",
		} {
			if got := s.eval(probe(expression)); got != want {
				t.Errorf("%s is %s, want %s", expression, got, want)
			}
		}
	}
	for expression, want := range map[string]string{
		"parent === top": "true", "parent === parent": "true", "parent !== window": "true", "frames === window": "true",
		"frames[0] === kid": "true", "kid.parent === window": "true", "length": "1", "frameElement": "null",
		"document.getElementById('f').contentDocument": "null", "frames[1]": "undefined", "top.top === top": "true",
		"top[0] === window": "true", "kid.length": "0", "window.self === window": "true",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s is %s, want %s", expression, got, want)
		}
	}
	s.quiet()
}

func TestWhatAWindowIsAskedGoesToTheBrowserAsData(t *testing.T) {
	s, windows := framedPage(t, `<p>x</p>`, dom.Host{})
	s.eval(`parent.postMessage({a: [1, "two"]}, "https://site.test"); kid.postMessage("hi", "*"); top.postMessage(1)`)
	want := []string{`1 https://site.test ["o",[["a",["a",2,[["0",1],["1","two"]]]]]]`, `2 * "hi"`, `1 / 1`}
	if !slices.Equal(windows.posted, want) {
		t.Errorf("the browser was posted %q, want %q", windows.posted, want)
	}
	for expression, want := range map[string]string{
		`parent.postMessage("x", "not an origin")`:                      "throws SyntaxError",
		`parent.postMessage(() => 1, "*")`:                              "throws DataCloneError",
		`parent.postMessage(document.body, "*")`:                        "throws DataCloneError",
		`parent.postMessage(parent, "*")`:                               "throws DataCloneError",
		`parent.postMessage(Symbol("s"), "*")`:                          "throws DataCloneError",
		`parent.postMessage("x", "*", [new MessageChannel().port1])`:    "throws DataCloneError",
		`parent.postMessage({get a() { throw new RangeError() }}, "*")`: "throws RangeError",
		`parent.postMessage()`:                                          "throws TypeError",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s gave %s, want %s", expression, got, want)
		}
	}
	if len(windows.posted) != 3 {
		t.Errorf("a message that could not be written went: %q", windows.posted)
	}
	s.eval(`parent.location = "/a"; parent.location.href = "b?c"; kid.location.replace("https://other.test/");
		top.focus(); kid.blur(); kid.close()`)
	went := []string{"1 https://site.test/a false", "1 https://site.test/dir/b?c false", "2 https://other.test/ true"}
	if !slices.Equal(windows.went, went) {
		t.Errorf("the browser was sent %q, want %q", windows.went, went)
	}
	windows.closed[2] = true
	if got := s.eval(`kid.closed + " " + parent.closed`); got != "true false" {
		t.Errorf("closed reads %s", got)
	}
}

func TestAMessageIsReadAsThePagesOwnObjects(t *testing.T) {
	s, windows := framedPage(t, `<script>
		window.got = [];
		addEventListener("message", (e) => got.push(e));
		addEventListener("messageerror", (e) => got.push("error from " + e.origin));
	</script>`, dom.Host{})
	s.eval(`(() => {
		const shared = {n: 1}, cycle = [shared, shared]; cycle.push(cycle);
		const bytes = new Uint8Array([1, 2, 250]);
		parent.postMessage({cycle, date: new Date(5), re: /a+/gi, map: new Map([[shared, new Set([1n])]]),
			bytes, view: new DataView(bytes.buffer, 1, 2), nan: NaN, zero: -0, none: undefined, wrapped: Object("s"),
			err: new TypeError("bad", {cause: shared}), dom: new DOMException("m", "AbortError"), holes: [, 1],
			text: "\u0000\ud800é"}, "*");
	})()`)
	if len(windows.posted) != 1 {
		t.Fatalf("posted %q", windows.posted)
	}
	text := strings.SplitN(windows.posted[0], " ", 3)[2]
	s.rt.Message(text, "https://other.test", 1)
	s.rt.Message(`["o",[["__proto__",["o",[["polluted",true]]]]]]`, "null", 2)
	s.rt.Message(`["r",0]`+"garbage", "https://other.test", 1)
	s.rt.Message(`["zz"]`, "https://other.test", 7)
	for expression, want := range map[string]string{
		"got.length": "4", "got[0].origin": "https://other.test", "got[0].source === parent": "true",
		"got[0].isTrusted": "true", "got[0].ports.length": "0", "got[1].origin": "null", "got[1].source === kid": "true",
		"got[2]": "error from https://other.test", "got[3]": "error from https://other.test",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s is %s, want %s", expression, got, want)
		}
	}
	for _, held := range []string{
		`(d => d.cycle[0] === d.cycle[1] && d.cycle[2] === d.cycle && d.cycle[0].n === 1)(got[0].data)`,
		`(d => d.date instanceof Date && d.date.getTime() === 5 && d.re.source === "a+" && d.re.flags === "gi")(got[0].data)`,
		`(d => [...d.map.keys()][0] === d.cycle[0] && [...d.map.values()][0].has(1n))(got[0].data)`,
		`(d => d.bytes instanceof Uint8Array && String([...d.bytes]) === "1,2,250")(got[0].data)`,
		`(d => d.view.buffer === d.bytes.buffer && d.view.getUint8(1) === 250)(got[0].data)`,
		`(d => Number.isNaN(d.nan) && Object.is(d.zero, -0) && "none" in d && d.none === undefined)(got[0].data)`,
		`(d => d.wrapped instanceof String && d.err instanceof TypeError && d.err.cause === d.cycle[0])(got[0].data)`,
		`(d => d.dom instanceof DOMException && d.dom.name === "AbortError")(got[0].data)`,
		`(d => !(0 in d.holes) && d.holes[1] === 1 && d.holes.length === 2 && d.text === "\u0000\ud800é")(got[0].data)`,
		`(d => Object.getPrototypeOf(d) === Object.prototype && Object.hasOwn(d, "__proto__"))(got[1].data)`,
		`({}).polluted === undefined`,
	} {
		if got := s.eval(probe(held)); got != "true" {
			t.Errorf("%s is %s", held, got)
		}
	}
}

func TestTheOriginOfAPageIsTheOneTheBrowserHolds(t *testing.T) {
	s, _ := framedPage(t, `<script>
		window.got = [];
		addEventListener("message", (e) => got.push(e.origin + " " + (e.source === window)));
	</script>`, dom.Host{Origin: func() string { return "https://held.test" }})
	s.eval(`(() => {
		try { window.origin = "https://forged.test"; } catch (_) {}
		try { Object.defineProperty(location, "origin", {value: "https://forged.test"}); } catch (_) {}
		try { Object.defineProperty(window, "origin", {value: "https://forged.test"}); } catch (_) {}
		postMessage("a", "*"); postMessage("b", "https://held.test"); postMessage("c", "https://forged.test");
		postMessage("d", "/");
	})()`)
	s.frames("its own messages", func() bool { return s.eval("got.length") == "3" })
	if got := s.eval("got.join()"); got != "https://held.test true,https://held.test true,https://held.test true" {
		t.Errorf("a page's messages to itself came as %s", got)
	}
}

func TestASandboxedDocumentHasNoOriginOfItsOwn(t *testing.T) {
	s, _ := framedPage(t, `<p>x</p>`, dom.Host{Sandboxed: dom.Sandboxed{Opaque: true, NoModals: true},
		Origin: func() string { return "https://site.test" }})
	for expression, want := range map[string]string{
		"origin": "null", "document.cookie = 'a=1', document.cookie": "", "navigator.cookieEnabled": "false",
		"localStorage": "throws SecurityError", "sessionStorage.length": "throws SecurityError",
		"localStorage.setItem('a', 1)": "throws SecurityError", "alert('x')": "undefined", "confirm('x')": "false",
		"prompt('x')": "null", "document.domain = 'site.test', document.domain": "site.test",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s is %s, want %s", expression, got, want)
		}
	}
	if said := s.said(); strings.Count(said, "'allow-modals' keyword is not set") != 3 || strings.Contains(said, "info:") {
		t.Errorf("the console says %s", said)
	}
}

func TestAPageIsNotIsolatedAndIsToldCoarseTimes(t *testing.T) {
	s := run(t, `<p>x</p>`)
	for expression, want := range map[string]string{
		"typeof SharedArrayBuffer": "undefined", "'SharedArrayBuffer' in window": "false", "crossOriginIsolated": "false",
		"Array.from({length: 200}, () => performance.now() * 10).every((t) => Math.abs(t - Math.round(t)) < 1e-6)": "true",
		"new TextEncoder().encode('é').length": "2",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s is %s, want %s", expression, got, want)
		}
	}
}
