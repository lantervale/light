//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/script"
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
	// aired is what each document hears of the others' broadcasts.
	aired map[string]string
	// near are the ways to look whose two documents are kin: they
	// share an engine and reach each other. more are ways to look only
	// this kin has, and engines how many engines its documents run in.
	near    map[string]bool
	more    []way
	engines int
}

// ways are the ways to look the kin is checked by.
func (k kin) ways() []way { return append(ways(), k.more...) }

func kins() []kin {
	all := map[string]bool{"parent to child": true, "child to parent": true, "sibling": true, "nested top": true}
	everyone := map[string]string{"": "leaf,mid,sib", "a": "leaf,sib,top", "b": "leaf,mid,top", "n": "mid,sib,top"}
	return []kin{
		{name: "same origin", first: "site.test", second: "site.test", far: "site.test",
			origins: [2]string{"https://site.test", "https://site.test"}, aired: everyone, near: all, engines: 1},
		{name: "same site", first: "a.site.test", second: "b.site.test", far: "b.site.test",
			origins: [2]string{"https://a.site.test", "https://b.site.test"},
			aired:   map[string]string{"b": "leaf", "n": "sib"}, engines: 3},
		{name: "cross site", first: "other.test", second: "third.test", far: "third.test",
			origins: [2]string{"https://other.test", "https://third.test"},
			aired:   map[string]string{"b": "leaf", "n": "sib"}, engines: 3},
		{name: "sandboxed", first: "site.test", second: "site.test", far: "site.test", sandbox: ` sandbox="allow-scripts"`,
			origins: [2]string{"null", "null"}, engines: 4},
		{name: "sandboxed with allow-same-origin", first: "site.test", second: "site.test", far: "site.test",
			sandbox: ` sandbox="allow-scripts allow-same-origin"`,
			origins: [2]string{"https://site.test", "https://site.test"}, aired: everyone, near: all, engines: 1},
		// The page and its first frame are kin, the others of another
		// site: every check of a window of another origin is made from
		// a kin, into one, and of a window reached through one.
		{name: "kin beside strangers", first: "site.test", second: "other.test", far: "other.test",
			origins: [2]string{"https://site.test", "https://other.test"},
			aired:   map[string]string{"": "mid", "a": "top", "b": "leaf", "n": "sib"},
			near:    map[string]bool{"parent to child": true, "child to parent": true}, engines: 2,
			more: []way{
				{name: "through a kin", from: "", window: "frames[0].frames[0]", parent: "frames[0]", top: "window",
					asker: "top", asked: "leaf", origin: -1, target: 1},
				{name: "stranger to a kin", from: "b", window: "parent.frames[0]", parent: "parent", top: "top", length: 1,
					asker: "sib", asked: "mid", origin: 1, target: 0},
			}},
		// The innermost document is of the page's origin under another
		// site's: it keeps its storage elsewhere, and is kin to neither
		// the page nor the page's own frame.
		{name: "kin under a stranger", first: "other.test", second: "site.test", far: "site.test",
			origins: [2]string{"https://other.test", "https://site.test"},
			aired:   map[string]string{"": "sib", "b": "top"},
			near:    map[string]bool{"page to its kin": true}, engines: 3,
			more: []way{
				{name: "page to its kin", from: "", window: "frames[1]", parent: "window", top: "window",
					asker: "top", asked: "sib", origin: -1, target: 1},
				{name: "under a stranger to the page's kin", from: "n", window: "top.frames[1]", parent: "top", top: "top",
					asker: "leaf", asked: "sib", origin: 1, target: 1},
			}},
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
		window.aired = [];
		window.news = new BroadcastChannel("matrix");
		news.onmessage = (e) => aired.push(e.data);
		const through = (port) => {
			window.port = port;
			port.onmessage = (m) => {
				if (m.data && m.data.kinds) { window.last = m.data.kinds; return; }
				const own = Object.getPrototypeOf(m) === MessageEvent.prototype && m.target === port && m.source === null &&
					m.origin === "" && m.ports.length === 0;
				got.push("port|" + m.data + "|" + own);
				if (m.data === "close") port.close();
				else port.postMessage("pong " + m.data);
			};
		};
		addEventListener("message", (e) => {
			if (e.data && e.data.kinds) { window.last = e.data.kinds; return; }
			if (e.ports.length) through(e.ports[0]);
			const own = Object.getPrototypeOf(e) === MessageEvent.prototype && e.target === window &&
				(typeof e.data !== "object" || Object.getPrototypeOf(e.data) === Object.prototype) &&
				(e.source === window || Object.getPrototypeOf(e.source) === null || typeof e.source.secret === "string");
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
		`W.postMessage(new MessageChannel().port1, "*")`:                             "throws DataCloneError",
		`W.postMessage("x", "*", [{}])`:                                              "throws DataCloneError",
		`W.postMessage(new Blob([() => secret]), "nowhere")`:                         "throws SyntaxError",
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

// reached are the ways a script reaches what is the document of its
// kin's, each with what it must get: the other document's own, as it
// is, from globals that are not the script's.
func (m *matrix) reached(w way) map[string]string {
	theirs := "secret of " + w.asked
	at := map[string]string{"top": "site.test/", "mid": m.kin.first + "/mid", "sib": m.kin.second + "/sib",
		"leaf": m.kin.far + "/leaf"}[w.asked]
	out := map[string]string{
		`W.secret`: theirs, `W.obj.secret`: theirs, `W.fn()`: theirs, `W.document.title`: theirs,
		`W.window.secret + W.self.secret + W.frames.secret`:                         strings.Repeat(theirs, 3),
		`Object.getOwnPropertyDescriptor(W, "secret").value`:                        theirs,
		`"secret" in W && Reflect.ownKeys(W).includes("secret")`:                    "true",
		`W.window === W && W.self === W && W.frames === W && W !== window`:          "true",
		`W.document === W.document && W.document.body.ownerDocument === W.document`: "true",
		`W.document.defaultView === W`:                                              "true",
		`document.defaultView === window`:                                           "true",
		`W.parent === ` + w.parent + ` && W.top === ` + w.top:                       "true",
		`W.length`: strconv.Itoa(w.length),
		`W.location.href + "|" + W.location + (W.location === W.location)`: "https://" + at + "|https://" + at + "true",
		`W.document.location.href`: "https://" + at,
		`W.Object !== Object && W.Element !== Element && W.Node !== Node && W.DOMException !== DOMException`: "true",
		`W.document instanceof W.Document && !(W.document instanceof Document)`:                              "true",
		`W.document.body instanceof W.HTMLElement && !(W.obj instanceof Object)`:                             "true",
		`W.obj instanceof W.Object && W.fn instanceof W.Function`:                                            "true",
		`W.document.createElement("p") instanceof W.HTMLParagraphElement`:                                    "true",
		`Object.getPrototypeOf(W) !== null && Object.getPrototypeOf(W.location) !== null`:                    "true",
		`(W.reached = {by: secret}, W.reached.by + (Object.getPrototypeOf(W.reached) === Object.prototype) + ` +
			`(delete W.reached) + ("reached" in W))`: "secret of " + w.asker + "truetruefalse",
		`(W.mine = window, (W.mine === window) + "" + (delete W.mine))`:        "truetrue",
		`W.postMessage === W.postMessage && W.postMessage instanceof Function`: "true",
		`structuredClone(W)`:                                               "throws DataCloneError",
		`W.postMessage(W, "*")`:                                            "throws DataCloneError",
		`W.postMessage(W.document, "*")`:                                   "throws DataCloneError",
		`W.postMessage(W.fn, "*")`:                                         "throws DataCloneError",
		`W.Function("return secret")()`:                                    "throws TypeError",
		`W.eval("secret")`:                                                 "undefined",
		`W.postMessage(new W.Date(5), "*")`:                                "throws DataCloneError",
		`W.fn.constructor("return secret")()`:                              "throws TypeError",
		`W.document.body.appendChild(document.createElement("p"))`:         "throws TypeError",
		`document.body.appendChild(W.document.createElement("p"))`:         "throws TypeError",
		`document.importNode(W.document.body, true)`:                       "throws TypeError",
		`document.adoptNode(W.document.createElement("p"))`:                "throws TypeError",
		`document.body.insertBefore(W.document.createElement("p"), null)`:  "throws TypeError",
		`document.body.contains(W.document.body)`:                          "false",
		`typeof __light + typeof W.__light + typeof W.kin + typeof W.join`: strings.Repeat("undefined", 4),
		`Reflect.ownKeys(W).filter((k) => !Reflect.ownKeys(window).includes(k) && !/^(secret|obj|fn|got|aired|news|` +
			`port|last|chan|portGot|spare|\d+)$/.test(String(k))).join()`: "",
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
		if (Object.getPrototypeOf(v) !== null) {
			if (!KIN.includes(String(v.secret))) out.push(path + " is an object with a prototype");
			return;
		}
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

// kinds is a message that holds a value of every kind the table says
// a message carries, by its mark, and held what the document that gets
// it must find under each: an object of its own.
const kinds = `(() => {
	const shared = {n: 1};
	window.spare = new MessageChannel();
	return {kinds: {u: undefined, n: NaN, i: 12n, r: [shared, shared], p: Object("s"), d: new Date(5), x: /a+/g,
		b: new Uint8Array([1, 2]).buffer, v: new Uint16Array([513]), m: new Map([["k", shared]]), s: new Set([1]),
		e: new RangeError("bad"), a: [1, , 3], o: {secret: "copied"}, B: new Blob(["blob"], {type: "t/x"}),
		F: new File(["file"], "f.txt"), P: spare.port1}};
})()`

func held() map[string]string {
	return map[string]string{
		"u": `"u" in last && last.u === undefined`, "n": `Number.isNaN(last.n)`, "i": `last.i === 12n`,
		"r": `last.r[0] === last.r[1] && Object.getPrototypeOf(last.r[0]) === Object.prototype`,
		"p": `last.p instanceof String && last.p == "s"`, "d": `last.d instanceof Date && last.d.getTime() === 5`,
		"x": `last.x instanceof RegExp && last.x.source === "a+"`,
		"b": `last.b instanceof ArrayBuffer && new Uint8Array(last.b)[1] === 2`,
		"v": `last.v instanceof Uint16Array && last.v[0] === 513`,
		"m": `last.m instanceof Map && last.m.get("k") === last.r[0]`, "s": `last.s instanceof Set && last.s.has(1)`,
		"e": `last.e instanceof RangeError && last.e.message === "bad" && last.e.constructor.constructor === Function`,
		"a": `Array.isArray(last.a) && last.a.length === 3 && !(1 in last.a)`,
		"o": `Object.getPrototypeOf(last.o) === Object.prototype && last.o.secret === "copied"`,
		"B": `last.B instanceof Blob && last.B.size === 4 && last.B.type === "t/x"`,
		"F": `last.F instanceof File && last.F.name === "f.txt" && last.F.size === 4`,
		"P": `last.P instanceof MessagePort && Object.getPrototypeOf(last.P) === MessagePort.prototype`,
	}
}

// portCrawl walks everything a script can get from a port, its
// prototypes too, and lists what is not of the script's own document
// or tells of another's.
const portCrawl = `(() => {
	const seen = new Set(), out = [];
	const walk = (v, path, depth) => {
		if (typeof v === "string") {
			if (v.includes("secret of") && v !== secret) out.push(path + " holds " + v);
			return;
		}
		if (v === null || (typeof v !== "object" && typeof v !== "function") || seen.has(v) || depth > 5) return;
		seen.add(v);
		if (!(v instanceof Object) && Object.getPrototypeOf(v) !== null) out.push(path + " is not of this document");
		for (const k of Reflect.ownKeys(v)) {
			let x;
			try { x = v[k]; } catch (_) { continue; }
			walk(x, path + "." + String(k), depth + 1);
		}
		walk(Object.getPrototypeOf(v), path + ".__proto__", depth + 1);
	};
	walk(PORT, "port", 0);
	return out.length + " " + out.join("; ");
})()`

// portDenied are the ways a script reaches through a port for what
// is another document's, each with what it must get.
func portDenied(holder string) map[string]string {
	return map[string]string{
		`Reflect.ownKeys(PORT).length`: "0", `Object.getPrototypeOf(PORT) === MessagePort.prototype`: "true",
		`PORT.constructor.constructor("return secret")()`: "secret of " + holder,
		`PORT.postMessage(() => secret)`:                  "throws DataCloneError",
		`PORT.postMessage({reach() { return secret; }})`:  "throws DataCloneError",
		`PORT.postMessage(window)`:                        "throws DataCloneError",
		`PORT.postMessage(parent)`:                        "throws DataCloneError",
		`PORT.postMessage(document.body)`:                 "throws DataCloneError",
		`PORT.postMessage(PORT)`:                          "throws DataCloneError",
		`PORT.postMessage(1, [PORT])`:                     "throws DataCloneError",
		`PORT.postMessage(1, [window])`:                   "throws DataCloneError",
		`structuredClone(PORT)`:                           "throws DataCloneError",
		`JSON.stringify(PORT)`:                            "{}",
		`PORT.secret + "|" + PORT.document + "|" + PORT.remote + "|" + PORT.other + "|" + PORT.window`: "" +
			"undefined|undefined|" +
			"undefined|undefined|undefined",
		portCrawl: "0 ",
	}
}

// ports has the document that looks hand the other one end of a
// channel, and checks that each end gives its holder messages and
// nothing else: copies made of its own objects, of every kind the
// table carries, through the window and through the port.
func (m *matrix) ports(w way) {
	m.t.Helper()
	asked := map[string]string{"top": "", "mid": "a", "sib": "b", "leaf": "n"}[w.asked]
	in := func(from, expression, want string) {
		m.t.Helper()
		m.checks++
		if got := m.within(from, expression); got != want {
			m.t.Errorf("%s, %s, in %q: %s is %s, want %s", m.kin.name, w.name, from, expression, got, want)
		}
	}
	m.does(asked, `window.port = null; window.last = null; got.length = 0;`)
	m.does(w.from, strings.ReplaceAll(`window.chan = new MessageChannel(); window.portGot = [];
		chan.port1.onmessage = (m) => portGot.push(m.data + "|" + (m.source === null && m.origin === "" &&
			m.target === chan.port1 && Object.getPrototypeOf(m) === MessageEvent.prototype));
		chan.port1.addEventListener("close", () => portGot.push("closed"));
		W.postMessage("a port", "*", [chan.port2]);`, "W", "("+w.window+")"))
	m.until(w.name+": the port came", func() bool { return m.within(asked, `port instanceof MessagePort`) == "true" })
	in(asked, `got.join()`, m.originOf(w.origin)+"|a port|true")
	in(w.from, `chan.port1.postMessage("ping")`, "undefined")
	m.until(w.name+": the port answered", func() bool { return m.within(w.from, `portGot.join()`) == "pong ping|true" })
	in(asked, `got[1]`, "port|ping|true")
	for expression, want := range portDenied(w.asker) {
		in(w.from, strings.ReplaceAll(expression, "PORT", "chan.port1"), want)
	}
	for expression, want := range portDenied(w.asked) {
		in(asked, strings.ReplaceAll(expression, "PORT", "port"), want)
	}
	for _, through := range []string{`W.postMessage(` + kinds + `, "*", [spare.port1])`,
		`chan.port1.postMessage(` + kinds + `, [spare.port1])`} {
		m.does(asked, `window.last = null;`)
		in(w.from, strings.ReplaceAll(through, "W", "("+w.window+")"), "undefined")
		m.until(w.name+": every kind came", func() bool { return m.within(asked, `last !== null`) == "true" })
		for _, kind := range frames.Carries() {
			check, known := held()[kind.Mark]
			if !known {
				m.t.Errorf("the matrix sends nothing for %s", kind.Name)
			}
			in(asked, check, "true")
		}
		in(asked, `Object.keys(last).length`, strconv.Itoa(len(frames.Carries())))
	}
	in(w.from, `chan.port1.postMessage("close")`, "undefined")
	m.until(w.name+": the port closed", func() bool {
		return strings.HasSuffix(m.within(w.from, `portGot.join()`), "closed")
	})
	in(w.from, `chan.port1.postMessage("late")`, "undefined")
	in(asked, `got.length`, "3")
}

func (m *matrix) originOf(i int) string {
	if i < 0 {
		return "https://site.test"
	}
	return m.kin.origins[i]
}

// broadcasts has every document broadcast its name, and checks which
// heard which: those of one origin that keep their storage together.
func (m *matrix) broadcasts() {
	m.t.Helper()
	names := map[string]string{"": "top", "a": "mid", "b": "sib", "n": "leaf"}
	for from, name := range names {
		m.does(from, `news.postMessage("`+name+`");`)
	}
	heard := func(from string) string {
		return m.within(from, `aired.slice().sort().join()`)
	}
	m.until("the broadcasts came", func() bool {
		for from := range names {
			if heard(from) != m.kin.aired[from] {
				return false
			}
		}
		return true
	})
	for range 5 {
		m.frame()
	}
	for from, name := range names {
		m.checks++
		if got := heard(from); got != m.kin.aired[from] {
			m.t.Errorf("%s: %s heard the broadcasts of %q, want %q", m.kin.name, name, got, m.kin.aired[from])
		}
	}
}

func TestNothingButCopiedDataCrossesFromOneFrameToAnother(t *testing.T) {
	checks := 0
	for _, k := range kins() {
		before := script.Live()
		m := showMatrix(t, k)
		if got := script.Live() - before; got != k.engines {
			t.Errorf("%s: its documents run in %d engines, want %d", k.name, got, k.engines)
		}
		m.grouped()
		for _, w := range k.ways() {
			if k.near[w.name] {
				for expression, want := range m.reached(w) {
					m.expect(w, expression, want)
				}
			} else {
				m.members(w)
				for expression, want := range denied(w.asker) {
					m.expect(w, expression, want)
				}
				m.expect(w, strings.NewReplacer("walk(W,", "walk(("+w.window+"),", "KIN", m.kinsOf(w.asker)).Replace(crawl), "0 ")
			}
			m.exchange(w)
			m.ports(w)
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
		m.broadcasts()
		if said := m.said(); strings.Contains(said, "error:") {
			t.Errorf("%s: the console says %s", k.name, said)
		}
		if set := m.session().frames[m.session().pages[m.browser.Tab().Current()]]; len(set.ports) != 2*len(k.ways())*2 {
			t.Errorf("%s: %d ends are left open, want the spare ones", k.name, len(set.ports))
		}
		checks += m.checks
		_ = m.Close()
	}
	t.Logf("%d checks, over %d kinds of frame", checks, len(kins()))
}

// documents are the scripts of the documents of the matrix, by name.
func (m *matrix) documents() map[string]*running {
	return map[string]*running{"top": m.session().scripts, "mid": m.frameNamed("a").run, "sib": m.frameNamed("b").run,
		"leaf": m.frameNamed("n").run}
}

// kinsOf are the secrets of the documents that are kin to the one of
// that name, as a list a script reads: the windows of those are no
// windows of another origin to it.
func (m *matrix) kinsOf(name string) string {
	live := m.documents()
	secrets := []string{}
	for other, run := range live {
		if other != name && run.kin != nil && run.kin == live[name].kin {
			secrets = append(secrets, `"secret of `+other+`"`)
		}
	}
	return "[" + strings.Join(secrets, ",") + "]"
}

// grouped checks that the documents that share an engine are those the
// kin says reach each other, and that those are of one origin kept in
// one place.
func (m *matrix) grouped() {
	m.t.Helper()
	live := m.documents()
	for _, w := range m.kin.ways() {
		m.checks++
		one, other := live[w.asker], live[w.asked]
		if shared := one.kin != nil && one.kin == other.kin; shared != m.kin.near[w.name] {
			m.t.Errorf("%s, %s: the two share an engine is %v, want %v", m.kin.name, w.name, shared, m.kin.near[w.name])
		}
	}
	keys := map[*dom.Kin]string{}
	set := m.session().frames[m.session().scripts.page]
	for name, run := range live {
		var f *frame
		if name != "top" {
			f = m.frameNamed(map[string]string{"mid": "a", "sib": "b", "leaf": "n"}[name])
		}
		key, has := set.kinKey(f)
		m.checks++
		switch {
		case has != (run.kin != nil), has && run.kin.Key() != key:
			m.t.Errorf("%s: %s is of the kin %v, its key being %q (%v)", m.kin.name, name, run.kin, key, has)
		case has && keys[run.kin] != "" && keys[run.kin] != key:
			m.t.Errorf("%s: one engine holds documents of %q and of %q", m.kin.name, keys[run.kin], key)
		case has:
			keys[run.kin] = key
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// add has the web answer one more page.
func (f *framing) add(at string, a answer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[at] = a
}

// boxedTop is a page with a sandboxed frame of its own origin beside
// one that is not, each with a link, a form and a function that sends
// its own document elsewhere.
func boxedTop(sandbox string) string {
	return `<title>Top</title><body style="margin:0"><iframe id="one"` + sandbox + ` src="/boxed"></iframe>
		<iframe id="two" src="/second"></iframe>` + boxedHeld
}

const boxedHeld = `<a id="l" href="/away">away</a><form id="f" action="/sent"></form>
	<script>window.leave = (to) => { location.href = to; };</script>`

// The sandbox of a frame that is kin to its page is asked of every
// navigation its script makes, straight or through what it reaches of
// its kin: a sibling it never sends, its tab only with the token, a
// form only with allow-forms, a question only with allow-modals.
func TestASandboxedKinSendsNothingItsSandboxKeepsItFrom(t *testing.T) {
	const two, tab = `parent.frames[1]`, `top`
	web, transport := framingOf(t, map[string]answer{
		"site.test/boxed":  html(`<p>boxed</p><script>window.mark = "boxed";</script>`),
		"site.test/second": html(`<p>second</p>` + boxedHeld),
		"site.test/away":   html(`<title>Away</title><p>away</p>`),
		"site.test/sent":   html(`<title>Sent</title><p>sent</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	const scripts, unsafe, unsent = "allow-scripts allow-same-origin", "Unsafe attempt", "A form was not sent"
	type attempt struct {
		tokens, act string
		// says is what the console holds of an attempt that is refused;
		// frame is what the other frame shows after one that is not, and
		// left where the tab went.
		says, frame, left string
	}
	var attempts []attempt
	ways := func(of string) []string {
		return []string{
			of + `.location = "/away"`, of + `.location.href = "/away"`, of + `.location.assign("/away")`,
			of + `.location.replace("/away")`, of + `.document.location = "/away"`,
			of + `.document.location.href = "/away"`, of + `.document.location.assign("/away")`,
			of + `.document.location.replace("/away")`, of + `.document.defaultView.location = "/away"`,
			of + `.document.getElementById("l").click()`, of + `.leave("/away")`,
		}
	}
	for _, tokens := range []string{scripts, scripts + " allow-top-navigation allow-forms allow-modals"} {
		for _, act := range append(ways(two), two+`.document.location.reload()`, two+`.history.go(-1)`) {
			attempts = append(attempts, attempt{tokens: tokens, act: act, says: unsafe})
		}
	}
	attempts = append(attempts,
		attempt{tokens: scripts, act: two + `.document.getElementById("f").submit()`, says: unsent},
		attempt{tokens: scripts + " allow-forms", act: two + `.document.getElementById("f").submit()`, says: unsafe})
	for _, act := range ways(two) {
		attempts = append(attempts, attempt{act: act, frame: "away"})
	}
	attempts = append(attempts, attempt{act: two + `.document.getElementById("f").submit()`, frame: "sent"})
	for _, act := range append(ways(tab), tab+`.document.location.reload()`) {
		attempts = append(attempts, attempt{tokens: scripts, act: act, says: unsafe},
			attempt{tokens: scripts + " allow-top-navigation-by-user-activation", act: act, says: unsafe})
	}
	for _, act := range ways(tab) {
		attempts = append(attempts, attempt{tokens: scripts + " allow-top-navigation", act: act,
			left: "https://site.test/away"})
	}
	submit := tab + `.document.getElementById("f").submit()`
	attempts = append(attempts,
		attempt{tokens: scripts + " allow-forms", act: submit, says: unsafe},
		attempt{tokens: scripts + " allow-top-navigation", act: submit, says: unsent},
		attempt{tokens: scripts + " allow-top-navigation allow-forms", act: submit, left: "https://site.test/sent"},
		attempt{tokens: scripts, act: `parent.alert("asked")`, says: "'allow-modals' keyword is not set"},
		attempt{tokens: scripts, act: `parent.frames[1].confirm("asked")`, says: "'allow-modals' keyword is not set"},
	)
	for i, a := range attempts {
		name := fmt.Sprintf("%q with %q", a.act, a.tokens)
		sandbox := ""
		if a.tokens != "" {
			sandbox = ` sandbox="` + a.tokens + `"`
		}
		top := "https://site.test/boxed-" + strconv.Itoa(i)
		web.add(strings.TrimPrefix(top, "https://"), html(boxedTop(sandbox)))
		s.visit(top)
		s.running("", "one", "two")
		if warned := s.consoleSays(console.Warn, "can escape its sandboxing"); warned != (a.tokens != "") {
			t.Errorf("%s: the console warns of a sandbox that is no wall: %t", name, warned)
		}
		s.session().said.Clear()
		if got := s.within("one", a.act); strings.HasPrefix(got, "throws") {
			t.Errorf("%s gave %s", name, got)
		}
		switch {
		case a.left != "":
			s.until(name+": the tab left its page for "+a.left, func() bool {
				at := s.browser.Tab().Current()
				return strings.HasPrefix(at, a.left) && s.session().pages[at] != nil
			})
			continue
		case a.frame != "":
			s.until(name+": the frame was sent", func() bool {
				f := s.frameNamed("two")
				return f != nil && f.load == nil && f.page != nil && strings.TrimSpace(f.page.Doc.Body().Text()) == a.frame
			})
		}
		for range 6 {
			s.frame()
		}
		if got := s.browser.Tab().Current(); got != top || s.doc.ByID("asking") != nil {
			t.Fatalf("%s: the tab went to %s, or asks: %t", name, got, s.doc.ByID("asking") != nil)
		}
		if got := s.framedText("two"); a.frame == "" && !strings.HasPrefix(got, "second") {
			t.Errorf("%s: the other frame shows %q", name, got)
		}
		if a.says != "" && !strings.Contains(s.said(), a.says) || a.says == "" && strings.Contains(s.said(), "error:") {
			t.Errorf("%s: the console says %q, want %q in it", name, s.said(), a.says)
		}
	}
	web.add("site.test/asks", html(boxedTop(` sandbox="`+scripts+` allow-modals"`)))
	s.visit("https://site.test/asks")
	s.running("", "one", "two")
	s.does("one", `setTimeout(() => { window.answered = String(parent.confirm("asked")); }, 0);`)
	s.until("the frame's question shows", func() bool { return s.doc.ByID("asking") != nil })
	s.click("asking-ok")
	s.becomes("one", `String(window.answered)`, "true")
	t.Logf("%d attempts", len(attempts)+1)
}
