//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
)

// file is what a site answers at a path.
type file struct {
	kind, body string
	header     map[string]string
	status     int
	// to is where it redirects, and held what it waits for before it
	// answers.
	to   string
	held chan struct{}
}

func js(body string) file   { return file{kind: "text/javascript", body: body} }
func page(body string) file { return file{kind: "text/html", body: body} }

// site is a small site served over HTTP, and how often each of its
// paths was asked for.
type site struct {
	*httptest.Server
	sync.Mutex
	asked map[string]int
}

func serve(t *testing.T, files map[string]file) *site {
	t.Helper()
	s := &site{asked: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Lock()
		s.asked[r.URL.Path]++
		s.Unlock()
		f, known := files[r.URL.Path]
		if !known {
			http.NotFound(w, r)
			return
		}
		if f.held != nil {
			select {
			case <-f.held:
			case <-r.Context().Done():
				return
			}
		}
		for name, value := range f.header {
			w.Header().Set(name, value)
		}
		if f.to != "" {
			http.Redirect(w, r, f.to, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", f.kind)
		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *site) count(path string) int {
	s.Lock()
	defer s.Unlock()
	return s.asked[path]
}

// visit loads the page a site has at its root with its scripts running,
// what they ask for fetched from the sites themselves.
func visit(t *testing.T, s *site) *scripted {
	t.Helper()
	p := &scripted{t: t}
	client := fetch.New(nil)
	loader := &web.Loader{Client: client, Scripts: true}
	p.page = loader.Load(context.Background(), s.URL+"/", web.Links{Open: func(string) {}, Over: func(string) {}})
	if p.page.Err != nil {
		t.Fatal(p.page.Err)
	}
	at, err := url.Parse(p.page.Address)
	if err != nil {
		t.Fatal(err)
	}
	var live sync.WaitGroup
	rt, err := dom.New(p.page, dom.Host{
		Console:  func(m dom.Message) { p.console = append(p.console, m) },
		Viewport: func() (float32, float32) { return 800, 600 },
		Fetch: func(q dom.Request, done func(dom.Response)) func() {
			p.fetched = append(p.fetched, q)
			ctx, cancel := context.WithCancel(context.Background())
			live.Add(1)
			go func() {
				defer live.Done()
				result, failed := client.Script(ctx, fetch.Call{Method: q.Method, Address: q.Address, Page: at,
					Mode: fetch.Mode(q.Mode), Credentials: fetch.Credentials(q.Credentials)})
				if failed != nil {
					done(dom.Response{Err: failed})
					return
				}
				answer := got(&result.Answer, nil)
				answer.Text, answer.Body = "", result.Body
				done(answer)
			}()
			return cancel
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	p.rt = rt
	t.Cleanup(func() {
		rt.Close()
		live.Wait()
	})
	p.layout()
	rt.Start()
	p.layout()
	return p
}

func got(answer *fetch.Answer, err error) dom.Response {
	if err != nil {
		return dom.Response{Err: err}
	}
	header := map[string]string{}
	for name := range answer.Header {
		header[name] = answer.Header.Get(name)
	}
	return dom.Response{Status: answer.Status, StatusText: http.StatusText(answer.Status), Address: answer.Address,
		Header: header, Text: answer.Code()}
}

// order waits for the page's scripts to have said want, in window.order.
func (s *scripted) order(want string) {
	s.t.Helper()
	said := func() string { return s.eval(`(window.order || []).join(" ")`) }
	s.frames("the scripts' "+want+", after "+said(), func() bool { return said() == want })
}

// errors are the console's errors and warnings, a line each.
func (s *scripted) errors() []dom.Message {
	var lines []dom.Message
	for _, m := range s.console {
		if m.Level == "error" || m.Level == "warn" {
			lines = append(lines, m)
		}
	}
	return lines
}

// about is what the console's errors say of the script at address, a
// line each.
func (s *scripted) about(address string) string {
	var lines []string
	for _, m := range s.errors() {
		if m.Source == address {
			lines = append(lines, m.Level+": "+m.Text)
		}
	}
	return strings.Join(lines, "\n")
}

const said = `<script>window.order = []; const say = (...what) => order.push(what.join(":"));</script>`

func TestModuleScriptsRunOnceThePageIsReadInItsOrder(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/first.js"></script>
			<script defer src="/deferred.js"></script>
			<script type="module">import { name } from "/dep.js";
				say("inline", name, document.currentScript, typeof this);</script>
			<script type="module" async src="/async.js"></script>
			<script nomodule>say("nomodule")</script><script nomodule src="/nomodule.js"></script>
			<script>say("classic", document.readyState);
				document.addEventListener("DOMContentLoaded", () => say("ready"));
				window.addEventListener("load", () => say("loaded"));</script>
			<p id="last">the last element</p>`),
		"/first.js": js(`import "/dep.js";
			say("first", document.getElementById("last") !== null, document.readyState);`),
		"/deferred.js": js(`say("deferred")`),
		"/dep.js":      js(`say("dep"); export const name = "dep";`),
		"/async.js":    js(`window.raced = true;`),
		"/nomodule.js": js(`say("nomodule")`),
	})
	p := visit(t, s)
	p.order("classic:loading dep first:true:loading deferred inline:dep::undefined ready loaded")
	if p.eval("raced") != "true" || s.count("/nomodule.js") != 0 || s.count("/dep.js") != 1 {
		t.Errorf("the async module ran: %s; nomodule.js was asked for %d times and dep.js %d", p.eval("raced"),
			s.count("/nomodule.js"), s.count("/dep.js"))
	}
	p.quiet()
}

func TestAModuleIsFetchedAndRunOnceWhoeverImportsIt(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/app.js"></script>
			<script type="module">import { n, grow } from "./dir/shared.js"; grow(); say("inline", n);</script>
			<script type="module" src="/app.js"></script>`),
		"/app.js": js(`import "./dir/left.js"; import "/dir/right.js"; import { n } from "./dir/shared.js";
			say("app", n);`),
		"/dir/left.js":   js(`import { n, grow } from "./shared.js"; grow(); say("left", n);`),
		"/dir/right.js":  js(`import { n } from "ORIGIN/dir/shared.js"; import "../dir/left.js"; say("right", n);`),
		"/dir/shared.js": js(`say("shared"); export let n = 0; export function grow() { n += 1 }`),
	})
	files := s.Config.Handler
	s.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		files.ServeHTTP(rec, r)
		for name, values := range rec.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write([]byte(strings.ReplaceAll(rec.Body.String(), "ORIGIN", s.URL)))
	})
	p := visit(t, s)
	p.order("shared left:1 right:1 app:1 inline:2")
	for _, path := range []string{"/app.js", "/dir/left.js", "/dir/right.js", "/dir/shared.js"} {
		if s.count(path) != 1 {
			t.Errorf("%s was asked for %d times", path, s.count(path))
		}
	}
	p.quiet()
}

func TestModulesThatImportEachOtherRunWithLiveBindings(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/a.js"></script>`),
		"/a.js": js(`import { b, grow } from "./b.js"; export function name() { return "a" }
			say("a", b); grow(); say("a", b);`),
		"/b.js": js(`import { name } from "./a.js"; export let b = 1; export function grow() { b += 1 }
			say("b", name());`),
	})
	p := visit(t, s)
	p.order("b:a a:1 a:2")
	p.quiet()
}

func TestAnAwaitAtTheTopOfAModuleHoldsThoseThatImportIt(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/app.js"></script>
			<script type="module">say("next");</script>
			<script>document.addEventListener("DOMContentLoaded", () => say("ready"))</script>`),
		"/app.js": js(`import { late } from "./slow.js"; say("app", late);`),
		"/slow.js": js(`say("slow"); export let late = "early";
			await new Promise((done) => setTimeout(done, 5)); late = "late";`),
	})
	p := visit(t, s)
	p.order("slow next ready app:late")
	p.quiet()
}

func TestImportCallsFetchModulesWhenAsked(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script src="/lib/classic.js"></script>
			<script type="module" src="/app.js"></script>`),
		"/lib/classic.js": js(`import("./chunk.js").then((m) => say("classic", m.default, m.where));
			window.again = () => import("/lib/chunk.js").then((m) => say("again", m.default));`),
		"/lib/chunk.js": js(`say("chunk"); export default "chunk";
			export const where = import.meta.url.split("/").slice(3).join("/");`),
		"/app.js": js(`const chunk = await import("./lib/chunk.js"); say("module", chunk.default);
			try { await import("./gone.js") } catch (e) { say(e.name, e.message.replace(location.origin, "")) }
			try { await import("bare") } catch (e) { say(e.name, e.message.split(".")[0]) }
			try { await import("./throws.js") } catch (e) { say("caught", e.message) }
			const data = await import("./data.json", { with: { type: "json" } }); say("json", data.default.n);
			await again();`),
		"/throws.js": js(`throw new Error("thrown by the module")`),
		"/data.json": {kind: "application/json", body: `{"n": 4}`},
	})
	p := visit(t, s)
	p.order(`chunk classic:chunk:lib/chunk.js module:chunk ` +
		`TypeError:Failed to fetch dynamically imported module: /gone.js ` +
		`TypeError:Failed to resolve module specifier "bare" caught:thrown by the module json:4 again:chunk`)
	if s.count("/lib/chunk.js") != 1 {
		t.Errorf("the chunk was asked for %d times", s.count("/lib/chunk.js"))
	}
	if lines := p.errors(); len(lines) != 1 || !strings.Contains(lines[0].Text, "/gone.js") ||
		!strings.Contains(lines[0].Text, "404") {
		t.Errorf("the console says %s, want only the module that is not there", p.said())
	}
}

func TestAModuleKnowsTheAddressItCameFrom(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/old.js?v=2"></script>
			<script type="module">say(import.meta.url === location.href)</script>`),
		"/old.js": {to: "/moved/new.js"},
		"/moved/new.js": js(`import { where } from "./dep.js";
			const path = (address) => new URL(address).pathname;
			say(path(import.meta.url), where, path(import.meta.resolve("../up.js")));`),
		"/moved/dep.js": js("\ufeff" + `export const where = new URL(import.meta.url).pathname;`),
		"/dep.js":       js(`export const where = "the wrong one";`),
	})
	p := visit(t, s)
	p.order("/moved/new.js:/moved/dep.js:/up.js true")
	p.quiet()
}

func TestAnImportMapNamesModules(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="importmap">{
				"imports": { "lib": "/vendor/lib.js", "kit/": "/vendor/kit/" },
				"scopes": { "/legacy/": { "lib": "/vendor/lib-1.js" } }
			}</script>
			<script type="module">import lib from "lib"; import part from "kit/part.js";
				say(lib, part, import.meta.resolve("kit/x.js").split("/").slice(3).join("/"));</script>
			<script type="module" src="/legacy/app.js"></script>
			<script type="module">import("lib").then((m) => say("import()", m.default));</script>`),
		"/vendor/lib.js":      js(`export default "lib 2";`),
		"/vendor/lib-1.js":    js(`export default "lib 1";`),
		"/vendor/kit/part.js": js(`export default "part";`),
		"/legacy/app.js":      js(`import lib from "lib"; say("legacy", lib);`),
	})
	p := visit(t, s)
	p.order("lib 2:part:vendor/kit/x.js legacy:lib 1 import():lib 2")
	p.quiet()
}

func TestAModuleOfAnotherTypeThanAScriptsIsRefused(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/plain.js" onerror="say('error', 'plain')"
				onload="say('load', 'plain')"></script>
			<script type="module" src="/app.js" onerror="say('error', 'app')"></script>
			<script type="module" src="/fine.js" onload="say('load', 'fine')"></script>
			<script type="module">import data from "/data.txt" with { type: "json" }; say("never");</script>
			<script type="module">import data from "/data.json" with { type: "json" }; say("json", data.ok);</script>`),
		"/plain.js":  {kind: "text/plain", body: `say("plain ran")`},
		"/app.js":    js(`import "/typed.ts"; say("app ran")`),
		"/typed.ts":  {kind: "video/mp2t", body: `say("typed ran")`},
		"/fine.js":   {kind: "application/javascript; charset=utf-8", body: `say("fine")`},
		"/data.txt":  {kind: "text/plain", body: `{"ok": true}`},
		"/data.json": {kind: "application/manifest+json", body: `{"ok": true}`},
	})
	p := visit(t, s)
	p.order("error:plain error:app fine load:fine json:true")
	if len(p.errors()) != 3 {
		t.Fatalf("the console says %s, want a line for each of the three", p.said())
	}
	want := `error: Failed to load module script: Expected a JavaScript-or-Wasm module script but the server ` +
		`responded with a MIME type of "text/plain". Strict MIME type checking is enforced for module scripts per ` +
		`HTML spec.`
	if got := p.about(s.URL + "/plain.js"); got != want {
		t.Errorf("the console says %q, want %q", got, want)
	}
	if got := p.about(s.URL + "/typed.ts"); !strings.Contains(got, `a MIME type of "video/mp2t"`) {
		t.Errorf("of the imported module the console says %q", got)
	}
	if got := p.about(s.URL + "/data.txt"); !strings.Contains(got, "Expected a JSON module script") ||
		!strings.Contains(got, `"text/plain"`) {
		t.Errorf("of the JSON module the console says %q", got)
	}
}

func TestAModuleOfAnotherOriginIsRunOnlyWhenSharedWithThePage(t *testing.T) {
	shared := func(with, body string) file {
		return file{kind: "text/javascript", body: body, header: map[string]string{"Access-Control-Allow-Origin": with}}
	}
	other := serve(t, map[string]file{
		"/shared.js":   shared("*", `say("shared"); import "./dep.js";`),
		"/dep.js":      shared("*", `say("dep")`),
		"/kept.js":     js(`say("kept ran")`),
		"/elsewhere":   shared("https://elsewhere.test", `say("elsewhere ran")`),
		"/wildcard.js": shared("*", `say("wildcard ran")`),
	})
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="` + other.URL + `/shared.js" onload="say('load')"></script>
			<script type="module" src="` + other.URL + `/kept.js" onerror="say('error', 'kept')"></script>
			<script type="module" src="` + other.URL + `/elsewhere" onerror="say('error', 'elsewhere')"></script>
			<script type="module" crossorigin="use-credentials" src="` + other.URL + `/wildcard.js"
				onerror="say('error', 'wildcard')"></script>
			<script type="module" crossorigin="anonymous" src="/own.js"></script>`),
		"/own.js": js(`say("own")`),
	})
	p := visit(t, s)
	p.order("dep shared load error:kept error:elsewhere error:wildcard own")
	if len(p.errors()) != 3 {
		t.Fatalf("the console says %s, want a line for each module kept from the page", p.said())
	}
	want := "error: Access to script at '" + other.URL + "/kept.js' from origin '" + s.URL + "' has been blocked by " +
		"CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource."
	if got := p.about(other.URL + "/kept.js"); got != want {
		t.Errorf("the console says %q, want %q", got, want)
	}
	if got := p.about(other.URL + "/elsewhere"); !strings.Contains(got,
		"has a value 'https://elsewhere.test' that is not equal to the supplied origin") {
		t.Errorf("of an answer shared with another origin the console says %q", got)
	}
	if got := p.about(other.URL + "/wildcard.js"); !strings.Contains(got,
		"must not be the wildcard '*' when the request's credentials mode is 'include'") {
		t.Errorf("of an answer shared with all, asked with credentials, the console says %q", got)
	}
	asked := map[string]dom.Request{}
	for _, q := range p.fetched {
		asked[strings.TrimPrefix(strings.TrimPrefix(q.Address, other.URL), s.URL)] = q
	}
	for path, credentials := range map[string]string{
		"/shared.js": "same-origin", "/dep.js": "same-origin", "/wildcard.js": "include", "/own.js": "same-origin",
	} {
		if q := asked[path]; q.Credentials != credentials || q.Mode != "cors" || !q.Script ||
			q.Method != http.MethodGet {
			t.Errorf("%s was asked for as %+v, want a script in cors mode with credentials %s", path, q, credentials)
		}
	}
}

func TestWhatStopsAModuleIsSaidOnceAndStopsNoOther(t *testing.T) {
	const (
		unexported = "Uncaught SyntaxError: The requested module './fine.js' does not provide an export named " +
			"'nothing'"
		unresolved = `Uncaught TypeError: Failed to resolve module specifier "lodash". Relative references must ` +
			`start with either "/", "./", or "../".`
	)
	s := serve(t, map[string]file{
		"/": page(said + `<script>window.addEventListener("error", (e) => {
				say("onerror", e.message, e.filename.split("/").pop(), e.lineno);
			});</script>
			<script type="module" src="/gone.js" onerror="say('error', 'gone')"></script>
			<script type="module" src="/needs-gone.js" onerror="say('error', 'needs-gone')"></script>
			<script type="module" src="/syntax.js" onerror="say('error', 'syntax')"
				onload="say('load', 'syntax')"></script>
			<script type="module" src="/throws.js" onload="say('load', 'throws')"></script>
			<script type="module" src="/needs-throws.js"></script>
			<script type="module" src="/missing.js"></script>
			<script type="module" src="/bare.js"></script>
			<script type="module">say("inline ran");
				null.inline;</script>
			<script type="module" src="/fine.js" onload="say('load', 'fine')"></script>`),
		"/needs-gone.js": js(`import "./gone.js"; say("never")`),
		"/syntax.js":     js("const fine = 1;\nconst = broken;"),
		"/throws.js": js("say('throws ran');\nfunction fail() { throw new RangeError('out of luck') }\n\n" +
			"fail();"),
		"/needs-throws.js": js(`import "./throws.js"; say("never")`),
		"/missing.js":      js("import { one } from './fine.js';\nimport { nothing } from './fine.js';"),
		"/bare.js":         js(`import "./fine.js"; import "lodash"; say("never")`),
		"/fine.js":         js(`say("fine"); export const one = 1;`),
	})
	p := visit(t, s)
	p.order("error:gone error:needs-gone " +
		"onerror:Uncaught SyntaxError: Unexpected token '=':syntax.js:2 load:syntax " +
		"throws ran onerror:Uncaught RangeError: out of luck:throws.js:2 load:throws " +
		"onerror:Uncaught RangeError: out of luck:throws.js:2 " +
		"onerror:" + unexported + ":missing.js:2 " +
		"onerror:" + unresolved + ":bare.js:0 " +
		"inline ran onerror:Uncaught TypeError: Cannot read properties of null (reading 'inline'):" + "" + ":2 " +
		"fine load:fine")
	type line struct {
		text, source string
		line         int
	}
	var lines []line
	for _, m := range p.errors() {
		lines = append(lines, line{m.Text, strings.TrimPrefix(m.Source, s.URL), m.Line})
	}
	want := []line{
		{"Failed to load module script " + s.URL + "/gone.js: the server answered 404 Not Found.", "/gone.js", 0},
		{"Uncaught SyntaxError: Unexpected token '='", "/syntax.js", 2},
		{"Uncaught RangeError: out of luck", "/throws.js", 2},
		{"Uncaught RangeError: out of luck", "/throws.js", 2},
		{unexported, "/missing.js", 2},
		{unresolved, "/bare.js", 0},
		{"Uncaught TypeError: Cannot read properties of null (reading 'inline')", "/", 2},
	}
	if len(lines) != len(want) {
		t.Fatalf("the console says %s, want %d lines", p.said(), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d of the console is %+v, want %+v", i, lines[i], want[i])
		}
	}
	if s.count("/gone.js") != 1 || s.count("/throws.js") != 1 {
		t.Errorf("gone.js was asked for %d times and throws.js %d", s.count("/gone.js"), s.count("/throws.js"))
	}
}

func TestAModuleScriptAddedToThePageRuns(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<script>
			const add = (set) => {
				const el = document.createElement("script");
				el.type = "module";
				set(el);
				document.head.append(el);
			};
			add((el) => { el.src = "/added.js"; el.onload = () => say("load"); });
			add((el) => { el.textContent = 'import { n } from "/dep.js"; say("inline", n)'; });
			add((el) => { el.src = "/gone.js"; el.onerror = () => say("error"); });
			say("classic", HTMLScriptElement.supports("module"), HTMLScriptElement.supports("importmap"));</script>`),
		"/added.js": js(`import { n } from "./dep.js"; say("added", n);`),
		"/dep.js":   js(`export const n = 1;`),
	})
	p := visit(t, s)
	p.frames("the added modules", func() bool { return p.eval("order.length") == "5" })
	got := " " + p.eval(`order.join(" ")`) + " "
	for _, want := range []string{" classic:true:true ", " added:1 load ", " inline:1 ", " error "} {
		if !strings.Contains(got, want) {
			t.Errorf("the scripts said %q, without %q", got, want)
		}
	}
	if s.count("/dep.js") != 1 {
		t.Errorf("dep.js was asked for %d times", s.count("/dep.js"))
	}
}

func TestAModuleThePageSaysItWillNeedIsFetchedOnce(t *testing.T) {
	s := serve(t, map[string]file{
		"/": page(said + `<link rel="modulepreload" href="/dep.js"><link rel="modulepreload" href="/unused.js">
			<link rel="modulepreload" href="/gone.js"><script type="module" src="/app.js"></script>`),
		"/app.js":    js(`import { n } from "./dep.js"; say("app", n);`),
		"/dep.js":    js(`say("dep"); export const n = 1;`),
		"/unused.js": js(`say("unused")`),
	})
	p := visit(t, s)
	p.order("dep app:1")
	p.frames("the module the page does not use", func() bool { return s.count("/unused.js") == 1 })
	if s.count("/dep.js") != 1 {
		t.Errorf("dep.js was asked for %d times", s.count("/dep.js"))
	}
}

func TestAPageLeftWhileItsModulesComeRunsNothingMore(t *testing.T) {
	held := make(chan struct{})
	s := serve(t, map[string]file{
		"/": page(said + `<script type="module" src="/app.js"></script>
			<script type="module">say("after")</script>`),
		"/app.js":  js(`import "./slow.js"; say("app")`),
		"/slow.js": {kind: "text/javascript", body: `say("slow")`, held: held},
	})
	p := visit(t, s)
	p.frames("the slow module to be asked for", func() bool { return s.count("/slow.js") == 1 })
	if got := p.eval("order.length"); got != "0" {
		t.Fatalf("%s scripts ran before the module they wait for came", got)
	}
	if !p.rt.Frame() {
		t.Error("a page whose modules are on their way says it has nothing left to run")
	}
	p.rt.Close()
	close(held)
	for end := time.Now().Add(50 * time.Millisecond); time.Now().Before(end); time.Sleep(time.Millisecond) {
		if p.rt.Frame() {
			t.Fatal("a closed page has something left to run")
		}
	}
	if len(p.console) != 0 {
		t.Errorf("the console of the page left says %s", p.said())
	}
}

func TestTheConsolesLineIsAClassicScriptOnAPageOfModules(t *testing.T) {
	s := serve(t, map[string]file{
		"/":       page(said + `<script type="module" src="/app.js"></script>`),
		"/app.js": js(`const hidden = 1; window.shown = 2; say("app");`),
	})
	p := visit(t, s)
	p.order("app")
	if got := p.eval("typeof hidden + ' ' + shown + ' ' + (1 + 1)"); got != "undefined 2 2" {
		t.Errorf("the console's line gave %q", got)
	}
	if _, err := p.rt.Evaluate(`import x from "/app.js"`); err == nil || !strings.Contains(err.Error(), "SyntaxError") {
		t.Errorf("an import declaration in the console's line gave %v", err)
	}
}
