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
	// mail is what was posted to windows, sent what went through ends,
	// by the end; ends counts those made and shut lists those closed.
	mail    []dom.Posted
	sent    map[int][]dom.Posted
	put     map[int][]dom.Posted
	ends    int
	shut    []int
	focused []int
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

func (t *tab) Post(to int, message dom.Posted, target string) error {
	t.posted = append(t.posted, fmt.Sprintf("%d %s %s", to, target, message.Text))
	t.mail = append(t.mail, message)
	return nil
}

func (t *tab) Focus(to int) { t.focused = append(t.focused, to) }

func (t *tab) Pipe() (int, int, bool) {
	if t.ends >= 8 {
		return 0, 0, false
	}
	t.ends += 2
	return 100 + t.ends - 1, 100 + t.ends, true
}

func (t *tab) Send(port int, m dom.Posted) {
	if t.sent == nil {
		t.sent = map[int][]dom.Posted{}
	}
	t.sent[port] = append(t.sent[port], m)
}

func (t *tab) Requeue(port int, m dom.Posted) {
	if t.put == nil {
		t.put = map[int][]dom.Posted{}
	}
	t.put[port] = append(t.put[port], m)
}

func (t *tab) Shut(port int) { t.shut = append(t.shut, port) }

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
		`parent.postMessage("x", "*", [{}])`:                            "throws DataCloneError",
		`parent.postMessage(new MessageChannel().port1, "*")`:           "throws DataCloneError",
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
	s.rt.Message(dom.Posted{Text: text, Bytes: windows.mail[0].Bytes}, "https://other.test", 1)
	s.rt.Message(dom.Posted{Text: `["o",[["__proto__",["o",[["polluted",true]]]]]]`}, "null", 2)
	s.rt.Message(dom.Posted{Text: `["r",0]` + "garbage"}, "https://other.test", 1)
	s.rt.Message(dom.Posted{Text: `["zz"]`}, "https://other.test", 7)
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

func texts(list []dom.Posted) string {
	var out []string
	for _, m := range list {
		out = append(out, fmt.Sprintf("%s %v", m.Text, m.Ports))
	}
	return strings.Join(out, " | ")
}

func TestAPortThatLeavesIsAnEndTheBrowserHolds(t *testing.T) {
	s, windows := framedPage(t, `<script>
		window.got = [];
		window.channel = new MessageChannel();
		channel.port1.onmessage = (e) => got.push("one " + e.data);
		channel.port1.addEventListener("close", () => got.push("closed"));
	</script>`, dom.Host{})
	s.eval(`channel.port2.postMessage("waits"); kid.postMessage({port: channel.port2}, "*", [channel.port2])`)
	if got := texts(windows.mail); got != `["o",[["port",["P",0]]]] [102]` {
		t.Fatalf("the window was posted %s", got)
	}
	s.eval(`channel.port1.postMessage({n: 1}); channel.port1.postMessage("two")`)
	if got := texts(windows.sent[101]); got != `["o",[["n",1]]] [] | "two" []` {
		t.Errorf("the end was sent %s", got)
	}
	for expression, want := range map[string]string{
		`channel.port2.postMessage("gone")`:                           "undefined",
		`kid.postMessage(1, "*", [channel.port2])`:                    "throws DataCloneError",
		`channel.port1.postMessage(1, [channel.port1])`:               "throws DataCloneError",
		`kid.postMessage(1, "*", [channel.port1, channel.port1])`:     "throws DataCloneError",
		`kid.postMessage(channel.port1, "*")`:                         "throws DataCloneError",
		`kid.postMessage(1, "nowhere", [channel.port1])`:              "throws SyntaxError",
		`structuredClone(channel.port1)`:                              "throws DataCloneError",
		`Object.keys(channel.port1).length + JSON.stringify(channel)`: "0{}",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s gave %s, want %s", expression, got, want)
		}
	}
	if len(windows.mail) != 1 || windows.ends != 2 {
		t.Errorf("a refused message took something: %d posted, %d ends", len(windows.mail), windows.ends)
	}
	s.rt.PortMessage(101, dom.Posted{Text: `"back"`})
	s.rt.PortMessage(999, dom.Posted{Text: `"to no end"`})
	s.rt.PortMessage(101, dom.Posted{Text: `["zz"]`})
	s.rt.PortClosed(101)
	s.rt.PortClosed(101)
	s.eval(`channel.port1.postMessage("after")`)
	s.frames("the port to close", func() bool { return strings.HasSuffix(s.eval("got.join()"), "closed") })
	if got := s.eval("got.join()"); got != "one waits,one back,closed" {
		t.Errorf("the port heard %s", got)
	}
	if len(windows.sent[101]) != 2 {
		t.Errorf("a closed end was sent to: %s", texts(windows.sent[101]))
	}
	s.quiet()
}

func TestAPortThatComesIsThePagesOwnObject(t *testing.T) {
	s, windows := framedPage(t, `<script>
		window.got = [];
		addEventListener("message", (e) => { window.event = e; window.port = e.ports[0]; });
	</script>`, dom.Host{})
	s.rt.Message(dom.Posted{Text: `["o",[["port",["P",0]],["again",["P",0]]]]`, Ports: []int{7}}, "https://other.test", 1)
	s.rt.PortMessage(7, dom.Posted{Text: `"first"`})
	s.rt.PortMessage(7, dom.Posted{Text: `["a",1,[["0",["P",0]]]]`, Ports: []int{9}})
	for expression, want := range map[string]string{
		"event.ports.length":                                    "1",
		"port instanceof MessagePort":                           "true",
		"event.data.port === port && event.data.again === port": "true",
		"Object.isFrozen(event.ports)":                          "true",
		"got.length":                                            "0",
		"Object.getOwnPropertyNames(port).length":               "0",
		"Reflect.ownKeys(Object.getPrototypeOf(port)).join()": "constructor,postMessage,start,close," +
			"onmessageerror,onclose,onmessage",
		"Object.getPrototypeOf(port) === MessagePort.prototype":  "true",
		"port.constructor === MessagePort && typeof port.remote": "undefined",
	} {
		if got := s.eval(probe(expression)); got != want {
			t.Errorf("%s is %s, want %s", expression, got, want)
		}
	}
	s.eval(`port.onmessage = (e) => got.push(typeof e.data === "string" ? e.data : "a port " + (e.data[0] === e.ports[0]));
		port.postMessage("answer")`)
	s.frames("the port to be read", func() bool { return s.eval("got.length") == "2" })
	if got := s.eval("got.join()"); got != "first,a port true" {
		t.Errorf("the port read %q", got)
	}
	if got := texts(windows.sent[7]); got != `"answer" []` {
		t.Errorf("the end was sent %s", got)
	}
	s.eval(`port.close(); port.postMessage("late"); port.close()`)
	if !slices.Equal(windows.shut, []int{7}) || len(windows.sent[7]) != 1 {
		t.Errorf("closing shut %v and sent %s", windows.shut, texts(windows.sent[7]))
	}
	s.rt.Message(dom.Posted{Text: `["zz"]`, Ports: []int{11}}, "https://other.test", 1)
	if !slices.Equal(windows.shut, []int{7, 11}) {
		t.Errorf("the end of a message that cannot be read stays open: %v", windows.shut)
	}
	s.quiet()
}

func TestAPortGoesOnWithWhatItNeverRead(t *testing.T) {
	s, windows := framedPage(t, `<script>
		addEventListener("message", (e) => { window.port = e.ports[0]; });
	</script>`, dom.Host{})
	s.rt.Message(dom.Posted{Text: `["P",0]`, Ports: []int{7}}, "https://other.test", 1)
	s.rt.PortMessage(7, dom.Posted{Text: `"unread"`})
	s.rt.PortMessage(7, dom.Posted{Text: `["P",0]`, Ports: []int{9}})
	s.eval(`kid.postMessage(port, "*", [port])`)
	if got := texts(windows.put[7]); got != `"unread" [] | ["P",0] [9]` {
		t.Errorf("the end was given back %s", got)
	}
	if got := texts(windows.mail); got != `["P",0] [7]` || windows.ends != 0 {
		t.Errorf("the window was posted %s, with %d new ends", got, windows.ends)
	}
	s.rt.PortMessage(7, dom.Posted{Text: `"for the next holder"`})
	if got := s.eval(probe(`port.postMessage("no more mine")`)); got != "undefined" || len(windows.sent[7]) != 0 {
		t.Errorf("a port that left still sends: %s, %s", got, texts(windows.sent[7]))
	}
	s.quiet()
}

func TestBytesCrossAsBytes(t *testing.T) {
	s, windows := framedPage(t, `<script>
		addEventListener("message", (e) => { window.data = e.data; });
	</script>`, dom.Host{})
	s.eval(`(() => {
		window.buffer = new Uint8Array([1, 2, 3, 250]).buffer;
		window.kept = new Uint8Array([9, 8]);
		const file = new File(["héllo"], "a.txt", {type: "Text/Plain", lastModified: 5});
		kid.postMessage({buffer, view: new Uint16Array(buffer, 2, 1), kept, blob: new Blob(["ab", "c"], {type: "x/y"}),
			file, twice: file}, "*", [buffer]);
	})()`)
	if len(windows.mail) != 1 {
		t.Fatalf("posted %d messages", len(windows.mail))
	}
	m := windows.mail[0]
	if string(m.Bytes) != "\x01\x02\x03\xfa\t\babchéllo" {
		t.Errorf("the bytes are %q", m.Bytes)
	}
	if strings.Contains(m.Text, "abc") || strings.Contains(m.Text, "llo") {
		t.Errorf("the text holds bytes: %s", m.Text)
	}
	if got := s.eval("buffer.byteLength + ' ' + buffer.detached + ' ' + kept.length"); got != "0 true 2" {
		t.Errorf("after the transfer the sender holds %s", got)
	}
	s.rt.Message(m, "https://other.test", 1)
	for _, held := range []string{
		`data.buffer instanceof ArrayBuffer && String([...new Uint8Array(data.buffer)]) === "1,2,3,250"`,
		`data.view instanceof Uint16Array && data.view.buffer === data.buffer && data.view[0] === 0xfa03`,
		`String([...data.kept]) === "9,8" && data.kept !== kept`,
		`data.blob instanceof Blob && !(data.blob instanceof File) && data.blob.type === "x/y" && data.blob.size === 3`,
		`data.file instanceof File && data.file.name === "a.txt" && data.file.type === "text/plain"`,
		`data.file.lastModified === 5 && data.twice === data.file && data.file.size === 6`,
	} {
		if got := s.eval(probe(held)); got != "true" {
			t.Errorf("%s is %s", held, got)
		}
	}
	s.eval(`data.file.text().then((text) => { window.read = text; })`)
	s.frames("the file read", func() bool { return s.eval("window.read") == "héllo" })
	for _, forged := range []string{`["b",0,99]`, `["B","t",-1,2]`, `["F","n","t",0,2,1e9]`,
		`["v","Uint8Array",["b",0,2],1,9]`} {
		s.eval("window.data = 'kept'")
		s.rt.Message(dom.Posted{Text: forged, Bytes: []byte{1, 2}}, "https://other.test", 1)
		if got := s.eval("data"); got != "kept" {
			t.Errorf("%s was read as %s", forged, got)
		}
	}
	if got := s.eval(probe(`kid.postMessage(new Uint8Array(33 << 20), "*")`)); got != "throws DataCloneError" {
		t.Errorf("a message too long gave %s", got)
	}
	if len(windows.mail) != 1 {
		t.Errorf("a message too long went")
	}
}

func TestATabHasOnlySoManyChannels(t *testing.T) {
	s, windows := framedPage(t, `<p>x</p>`, dom.Host{})
	got := s.eval(probe(`(() => {
		for (let i = 0; i < 9; i++) kid.postMessage(1, "*", [new MessageChannel().port1]);
	})()`))
	if got != "throws DataCloneError" || len(windows.mail) != 4 {
		t.Errorf("past its channels the page got %s, %d messages went", got, len(windows.mail))
	}
}

func TestABroadcastChannelReachesTheBrowser(t *testing.T) {
	var aired []string
	host := dom.Host{Broadcast: func(name string, m dom.Posted) {
		aired = append(aired, fmt.Sprintf("%s %s %q %v", name, m.Text, m.Bytes, m.Ports))
	}}
	s, _ := framedPage(t, `<script>
		window.got = [];
		window.a = new BroadcastChannel("news"); window.b = new BroadcastChannel("news");
		window.other = new BroadcastChannel("other");
		for (const [name, c] of [["a", a], ["b", b], ["other", other]]) {
			c.onmessage = (e) => got.push(name + " " + JSON.stringify(e.data) + " " + e.origin + " " + e.source +
				e.ports.length);
		}
	</script>`, host)
	s.eval(`a.postMessage({n: new Uint8Array([7])})`)
	if want := []string{`news ["o",[["n",["v","Uint8Array",["b",0,1],0,1]]]] "\a" []`}; !slices.Equal(aired, want) {
		t.Errorf("the browser was given %q, want %q", aired, want)
	}
	s.rt.Broadcast("news", "https://site.test", dom.Posted{Text: `["o",[["from",1]]]`, Ports: []int{4}})
	s.rt.Broadcast("news", "https://site.test", dom.Posted{Text: `["zz"]`})
	s.rt.Broadcast("none", "https://site.test", dom.Posted{Text: `1`})
	s.frames("the broadcasts", func() bool { return s.eval("got.length") == "3" })
	want := `a {"from":1} https://site.test null0,b {"from":1} https://site.test null0,` +
		`b {"n":{"0":7}} https://site.test null0`
	if got := s.eval("got.join()"); got != want {
		t.Errorf("the channels heard %s", got)
	}
	if got := s.eval(probe(`a.postMessage(new MessageChannel().port1)`)); got != "throws DataCloneError" {
		t.Errorf("a port in a broadcast gave %s", got)
	}
	aired = nil
	closed, _ := framedPage(t, `<p>x</p>`, dom.Host{Broadcast: host.Broadcast, Sandboxed: dom.Sandboxed{Opaque: true}})
	closed.eval(`new BroadcastChannel("news").postMessage(1)`)
	if len(aired) != 0 {
		t.Errorf("a document with no origin broadcast %q", aired)
	}
}

func TestAWindowAsksTheBrowserForTheKeyboard(t *testing.T) {
	s, windows := framedPage(t, `<p>x</p>`, dom.Host{})
	s.eval(`kid.focus(); parent.focus(); kid.blur()`)
	if !slices.Equal(windows.focused, []int{2, 1}) {
		t.Errorf("the browser was asked to focus %v", windows.focused)
	}
}

func TestAWindowLoadsOnceItsFramesDid(t *testing.T) {
	loading := true
	s, _ := framedPage(t, `<script>
		window.got = [];
		document.addEventListener("DOMContentLoaded", () => got.push("content"));
		addEventListener("load", () => got.push("load"));
	</script>`, dom.Host{Loading: func() bool { return loading }})
	s.rt.Frame()
	s.rt.Frame()
	if got := s.eval("got.join() + ' ' + document.readyState"); got != "content interactive" || s.rt.Loaded() {
		t.Errorf("while a frame loads the page is at %s", got)
	}
	loading = false
	s.rt.Frame()
	if got := s.eval("got.join() + ' ' + document.readyState"); got != "content,load complete" || !s.rt.Loaded() {
		t.Errorf("once its frames loaded the page is at %s", got)
	}
}
