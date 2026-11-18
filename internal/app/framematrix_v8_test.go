//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/frames"
)

// kin is how the documents of a page's frames stand to the page: where
// each comes from, and the sandbox of the iframes that show them.
type kin struct {
	name               string
	first, second, far string
	sandbox            string
	// origins are those of the documents of the frames a (and the one
	// in it) and b, as the browser holds them.
	origins [2]string
}

func kins() []kin {
	return []kin{
		{name: "same origin", first: "site.test", second: "site.test", far: "site.test",
			origins: [2]string{"https://site.test", "https://site.test"}},
		{name: "same site", first: "a.site.test", second: "b.site.test", far: "b.site.test",
			origins: [2]string{"https://a.site.test", "https://b.site.test"}},
		{name: "cross site", first: "other.test", second: "third.test", far: "third.test",
			origins: [2]string{"https://other.test", "https://third.test"}},
		{name: "sandboxed", first: "site.test", second: "site.test", far: "site.test", sandbox: ` sandbox="allow-scripts"`,
			origins: [2]string{"null", "null"}},
	}
}

// keeper is the script of every document of the matrix: it holds a
// secret under each name another document might reach for, notes the
// messages it gets, and answers a ping.
func keeper(name string) string {
	return `<script>
		window.secret = "secret of ` + name + `"; window.obj = {secret}; window.fn = () => secret;
		document.title = secret;
		try { localStorage.setItem("kept", secret); document.cookie = "kept=" + secret; } catch (_) {}
		window.got = [];
		addEventListener("message", (e) => {
			const own = Object.getPrototypeOf(e) === MessageEvent.prototype && e.target === window &&
				(typeof e.data !== "object" || Object.getPrototypeOf(e.data) === Object.prototype) &&
				(e.source === window || Object.getPrototypeOf(e.source) === null);
			got.push(e.origin + "|" + (typeof e.data === "object" ? JSON.stringify(e.data) : e.data) + "|" + own);
			if (typeof e.data === "string" && e.data.startsWith("ping ")) e.source.postMessage("pong " + e.data.slice(5), "*");
		});
	</script>`
}

func (k kin) pages() map[string]answer {
	pages := map[string]answer{
		"site.test/": html(`<title>top</title><body style="margin:0">
			<iframe id="a" name="first" src="https://` + k.first + `/mid"` + k.sandbox + `></iframe>
			<iframe id="b" name="second" src="https://` + k.second + `/sib"` + k.sandbox + `></iframe>` + keeper("top")),
	}
	pages[k.first+"/mid"] = html(`<iframe id="n" name="nested" src="https://` + k.far + `/leaf"></iframe>` + keeper("mid"))
	pages[k.second+"/sib"] = html(`<p>sib</p>` + keeper("sib"))
	pages[k.far+"/leaf"] = html(`<p>leaf</p>` + keeper("leaf"))
	return pages
}

// way is one document of the matrix looking at the window of another:
// who looks, by the id of its frame ("" for the page), how it names
// the other, and what it must find there.
type way struct {
	name, from, window string
	// is are the expressions that name the same window as window.parent
	// and window.top, and length how many frames the other has.
	parent, top string
	length      int
	// asker and asked are which document looks and which is looked at,
	// and origins the index of their origins in a kin's, -1 for the
	// page's.
	asker, asked   string
	origin, target int
}

func ways() []way {
	return []way{
		{name: "parent to child", from: "", window: "frames[0]", parent: "window", top: "window", length: 1,
			asker: "top", asked: "mid", origin: -1, target: 0},
		{name: "child to parent", from: "a", window: "parent", parent: "parent", top: "parent", length: 2,
			asker: "mid", asked: "top", origin: 0, target: -1},
		{name: "sibling", from: "a", window: "parent.frames[1]", parent: "parent", top: "top", length: 0,
			asker: "mid", asked: "sib", origin: 0, target: 1},
		{name: "nested top", from: "n", window: "top", parent: "top", top: "top", length: 2,
			asker: "leaf", asked: "top", origin: 1, target: -1},
	}
}

// refused is what reading, writing or calling something of another
// window that the table does not hold gives.
const refused = "throws SecurityError"

// denied are the ways a script reaches for what is another document's,
// each with what it must get: W is the other window.
func denied(asker string) map[string]string {
	own := "secret of " + asker
	out := map[string]string{
		`Object.getPrototypeOf(W)`: "null", `Object.getPrototypeOf(W.location)`: "null",
		`Object.getOwnPropertySymbols(W).length`: "0", `Reflect.ownKeys(W).some((k) => typeof k !== "string")`: "false",
		`Object.getOwnPropertyDescriptor(W, "top").get`:                              "undefined",
		`Object.getOwnPropertyDescriptor(W, "location").value === W.location`:        "true",
		`Object.getOwnPropertyDescriptor(W.location, "href").get`:                    "undefined",
		`typeof Object.getOwnPropertyDescriptor(W.location, "href").set`:             "function",
		`W.postMessage.constructor === Function && W.close.constructor === Function`: "true",
		`W.postMessage.constructor("return secret")()`:                               own,
		`W.location.replace.constructor("return secret")()`:                          own,
		`Object.setPrototypeOf(W, {})`:                                               "throws TypeError",
		`Reflect.setPrototypeOf(W, Object.prototype)`:                                "false",
		`Reflect.preventExtensions(W)`:                                               "false",
		`structuredClone(W)`:                                                         "throws DataCloneError",
		`W.postMessage(() => secret, "*")`:                                           "throws DataCloneError",
		`W.postMessage({reach() { return secret; }}, "*")`:                           "throws DataCloneError",
		`W.postMessage(document.body, "*")`:                                          "throws DataCloneError",
		`W.postMessage(window, "*")`:                                                 "throws DataCloneError",
		`W.postMessage(W, "*")`:                                                      "throws DataCloneError",
		`W.postMessage(Symbol.iterator, "*")`:                                        "throws DataCloneError",
		`W.postMessage("x", "*", [new MessageChannel().port1])`:                      "throws DataCloneError",
		`W.postMessage("x", "nowhere")`:                                              "throws SyntaxError",
		`W.postMessage.call(W).secret`:                                               "throws TypeError",
		`Object.entries(W).length + Object.keys(W).length`:                           "0",
		`JSON.stringify(W)`:                                                          refused,
		`(() => { try { W.document; } catch (e) { return e instanceof DOMException && ` +
			`Object.getPrototypeOf(e) === DOMException.prototype && e.constructor.constructor === Function && ` +
			`!/secret of/.test(e.message + e.stack + Object.keys(e)); } })()`: "true",
		`(() => { const seen = []; Promise.resolve(W).then((v) => seen.push(v === W)); return seen.length; })()`: "0",
	}
	for _, reach := range []string{
		`W.secret`, `W.obj`, `W.fn`, `W.fn()`, `W.document`, `W.document.title`, `W.document.cookie`, `W.localStorage`,
		`W.sessionStorage`, `W.origin`, `W.name`, `W.frameElement`, `W.history`, `W.navigator`, `W.got`, `W.eval`,
		`W.eval("secret")`, `W.Function`, `W.Object`, `W.Array`, `W.constructor`, `W.constructor.constructor`,
		`W.__proto__`, `W.prototype`, `W.__lookupGetter__`, `W.hasOwnProperty`, `W.toString`, `W.toString()`,
		`W.valueOf`, `W.valueOf()`, `String(W)`, `W + ""`, "`${W}`", `+W`, `W[Symbol.toPrimitive]`, `W[Symbol.iterator]`,
		`W[Symbol.for("secret")]`, `W[Symbol.asyncIterator]`, `[...W]`, `W.location.href`, `W.location.origin`,
		`W.location.host`, `W.location.pathname`, `W.location.assign`, `W.location.reload`, `W.location.toString`,
		`W.location.toString()`, `String(W.location)`, `W.location.constructor`, `W.location.ancestorOrigins`,
		`W.location.hash = "#x"`, `W.location.secret = 1`, `delete W.location.href`, `W.secret = 1`, `W.document = 1`,
		`W.name = "x"`, `W.onmessage = () => {}`, `W.length = 9`, `W.closed = true`, `W.top = window`, `W.parent = window`,
		`W.opener = window`, `delete W.location`, `delete W.secret`, `Object.defineProperty(W, "secret", {get() {}})`,
		`Object.defineProperty(W, "top", {value: window})`, `"secret" in W`, `"document" in W`,
		`Object.getOwnPropertyDescriptor(W, "secret")`, `Object.getOwnPropertyDescriptor(W, "document")`,
		`W.addEventListener("message", () => {})`, `W.dispatchEvent(new Event("x"))`, `W.setTimeout(() => {})`,
		`W.fetch("/")`, `W.open()`, `W.alert(1)`, `W.getComputedStyle(document.body)`, `W.self.secret`,
		`W.window.document`, `W.frames.secret`, `Object.assign(W, {secret: 1})`,
	} {
		out[reach] = refused
	}
	for _, name := range frames.Quiet() {
		out[`W.`+name] = "undefined"
		out[`"`+name+`" in W`] = "false"
	}
	return out
}

// crawl walks everything a script can get from another window by its
// names, four deep, and lists what it finds that is no window of
// another frame, no function of the script's own document and no
// plain value.
const crawl = `(() => {
	const seen = new Set(), out = [];
	const walk = (v, path, depth) => {
		if (v === null || (typeof v !== "object" && typeof v !== "function")) {
			if (typeof v === "string" && v.includes("secret of")) out.push(path + " holds " + v);
			return;
		}
		if (v === window || seen.has(v) || depth > 4) return;
		seen.add(v);
		if (typeof v === "function") {
			if (Object.getPrototypeOf(v) !== Function.prototype) out.push(path + " is no function of this document");
			if (Reflect.ownKeys(v).some((k) => !["length", "name", "prototype"].includes(k))) {
				out.push(path + " holds more than a function does");
			}
			return;
		}
		if (Object.getPrototypeOf(v) !== null) { out.push(path + " is an object with a prototype"); return; }
		for (const k of Reflect.ownKeys(v)) {
			let x;
			try { x = v[k]; } catch (e) {
				if (e.name !== "SecurityError") out.push(path + "." + String(k) + " throws " + e.name);
				continue;
			}
			walk(x, path + "." + String(k), depth + 1);
		}
	};
	walk(W, "W", 0);
	return out.length + " " + out.join("; ");
})()`

// matrix is a page of the matrix shown, its documents' scripts running.
type matrix struct {
	*shown
	kin    kin
	checks int
}

func showMatrix(t *testing.T, k kin) *matrix {
	t.Helper()
	_, transport := framingOf(t, k.pages())
	m := &matrix{shown: showWith(t, Options{Transport: transport, Downloads: t.TempDir()}), kin: k}
	m.visit("https://site.test/")
	m.until("every document's scripts ran", func() bool {
		if m.session().scripts == nil {
			return false
		}
		for _, id := range []string{"a", "b", "n"} {
			if f := m.frameNamed(id); f == nil || f.run == nil {
				return false
			}
		}
		return true
	})
	return m
}

// frameNamed is the frame whose element has that id, nil for none.
func (s *shown) frameNamed(id string) *frame {
	page := s.session().pages[s.browser.Tab().Current()]
	for _, f := range s.session().frames[page].frameList() {
		if f.node.ID() == id && f.live {
			return f
		}
	}
	return nil
}

// does runs statements in the document of the frame from.
func (s *shown) does(from, statements string) {
	s.t.Helper()
	if got := s.within(from, `(() => { `+statements+` })()`); got != "undefined" {
		s.t.Fatalf("%s gave %s", statements, got)
	}
}

// within is the value of an expression in the document of the frame from,
// the page's for "": what it is as a text, or the name of what it
// throws.
func (s *shown) within(from, expression string) string {
	s.t.Helper()
	live := s.session().scripts
	if from != "" {
		live = s.frameNamed(from).run
	}
	if live == nil {
		s.t.Fatalf("the document of %q runs no script", from)
	}
	value, err := live.rt.Evaluate(`(() => { try { const v = (` + expression + `); return typeof v === "function" ? ` +
		`"function" : String(v); } catch (e) { return "throws " + (e && e.name); } })()`)
	if err != nil {
		s.t.Fatalf("%s: %v", expression, err)
	}
	if text, quoted := strconv.Unquote(value); quoted == nil {
		return text
	}
	return value
}

func (m *matrix) expect(w way, expression, want string) {
	m.t.Helper()
	m.checks++
	if got := m.within(w.from, strings.ReplaceAll(expression, "W", "("+w.window+")")); got != want {
		m.t.Errorf("%s, %s: %s is %s, want %s", m.kin.name, w.name, expression, got, want)
	}
}

// members checks what the table says another window answers.
func (m *matrix) members(w way) {
	m.t.Helper()
	for _, member := range frames.WindowMembers() {
		at := "W." + member.Name
		switch member.Shape {
		case frames.ItsWindow:
			m.expect(w, at+" === W", "true")
		case frames.AWindow:
			m.expect(w, at+" === "+map[string]string{"parent": w.parent, "top": w.top}[member.Name], "true")
		case frames.ANull:
			m.expect(w, at, "null")
		case frames.ANumber:
			m.expect(w, at, strconv.Itoa(w.length))
		case frames.ABoolean:
			m.expect(w, at, "false")
		case frames.ItsLocation:
			m.expect(w, at+" === "+at+" && typeof "+at, "object")
		case frames.AMethod:
			m.expect(w, at+" === "+at+" && "+at+" instanceof Function", "true")
		case frames.ASetter:
			m.t.Errorf("a window has no member that is only set, and the table has %s", member.Name)
		}
		m.expect(w, `Reflect.ownKeys(W).includes("`+member.Name+`") && "`+member.Name+`" in W`, "true")
	}
	for _, member := range frames.LocationMembers() {
		at := "W.location." + member.Name
		switch member.Shape {
		case frames.ASetter:
			m.expect(w, at, refused)
		case frames.AMethod:
			m.expect(w, at+" === "+at+" && "+at+" instanceof Function", "true")
		case frames.ItsWindow, frames.AWindow, frames.ANull, frames.ANumber, frames.ABoolean, frames.ItsLocation:
			m.t.Errorf("the table gives a location a %s", member.Shape)
		}
	}
	m.expect(w, `Reflect.ownKeys(W.location).join()`, "href,replace")
	m.expect(w, fmt.Sprintf(`Reflect.ownKeys(W).length - %d`, len(frames.WindowMembers())), strconv.Itoa(w.length))
}

// exchange has the document that looks post the other a ping, and
// checks each got the other's message from the origin the browser
// holds, with the window that sent it and nothing of its document.
func (m *matrix) exchange(w way) {
	m.t.Helper()
	origin := func(i int) string {
		if i < 0 {
			return "https://site.test"
		}
		return m.kin.origins[i]
	}
	asked := map[string]string{"top": "", "mid": "a", "sib": "b", "leaf": "n"}[w.asked]
	ping := "ping " + m.kin.name + " " + w.name
	m.expect(w, `W.postMessage("`+ping+`", "*")`, "undefined")
	m.expect(w, `W.postMessage({deep: {list: [1, "two"]}}, "*")`, "undefined")
	pong := origin(w.target) + "|pong " + strings.TrimPrefix(ping, "ping ") + "|true"
	m.until(w.name+" answered", func() bool { return strings.Contains(m.within(w.from, `got.join("\n")`), pong) })
	m.checks += 2
	heard := m.within(asked, `got.join("\n")`)
	from := origin(w.origin)
	for _, want := range []string{from + "|" + ping + "|true", from + `|{"deep":{"list":[1,"two"]}}|true`} {
		if !strings.Contains(heard, want) {
			m.t.Errorf("%s, %s: the other document heard %q, want %q", m.kin.name, w.name, heard, want)
		}
	}
}

func TestNothingButCopiedDataCrossesFromOneFrameToAnother(t *testing.T) {
	checks := 0
	for _, k := range kins() {
		m := showMatrix(t, k)
		for _, w := range ways() {
			m.members(w)
			for expression, want := range denied(w.asker) {
				m.expect(w, expression, want)
			}
			m.expect(w, strings.ReplaceAll(crawl, "walk(W,", "walk(("+w.window+"),"), "0 ")
			m.exchange(w)
			if got := m.within(w.from, `secret + "|" + document.title + "|" + fn()`); got != strings.Repeat("|secret of "+
				w.asker, 3)[1:] {
				t.Errorf("%s, %s: the document that looked now holds %s", k.name, w.name, got)
			}
		}
		for from, name := range map[string]string{"": "top", "a": "mid", "b": "sib", "n": "leaf"} {
			if got := m.within(from, `secret + "|" + obj.secret + "|" + Object.keys(window).includes("x")`); got !=
				"secret of "+name+"|secret of "+name+"|false" {
				t.Errorf("%s: the document %s was changed: %s", k.name, name, got)
			}
		}
		if said := m.said(); strings.Contains(said, "error:") {
			t.Errorf("%s: the console says %s", k.name, said)
		}
		checks += m.checks
		_ = m.Close()
	}
	t.Logf("%d checks, over %d kinds of frame and %d ways to look", checks, len(kins()), len(ways()))
}

func itoa(n int) string { return strconv.Itoa(n) }

// add has the web answer one more page.
func (f *framing) add(at string, a answer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[at] = a
}
