//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/internal/dom"
)

func TestEventsReachTheListenersOfThePage(t *testing.T) {
	const page = `<div id="outer" onclick="seen.push('outer attribute ' + event.type)"><button id="inner">Go</button>
		<a id="link" href="/next">next</a><input id="field"></div><script>var seen = [];
		const inner = document.getElementById("inner"), outer = document.getElementById("outer");</script>`
	for _, c := range []struct{ name, script, fire, want string }{
		{"a listener hears its event", `inner.addEventListener("click", (e) => seen.push(e.type + " " + e.target.id +
			" " + e.currentTarget.id + " " + (e instanceof MouseEvent) + " " + e.isTrusted))`, "click",
			"click inner inner true true,outer attribute click"},
		{"events bubble to the document and the window", `outer.addEventListener("click", (e) => seen.push("outer from " +
			e.target.id)); document.addEventListener("click", () => seen.push("document"));
			window.addEventListener("click", () => seen.push("window"))`, "click",
			"outer attribute click,outer from inner,document,window"},
		{"capture comes first", `document.addEventListener("click", () => seen.push("capture"), true);
			inner.addEventListener("click", () => seen.push("target"))`, "click", "capture,target,outer attribute click"},
		{"stopPropagation", `inner.addEventListener("click", (e) => { e.stopPropagation(); seen.push("stopped") })`, "click",
			"stopped"},
		{"stopImmediatePropagation", `inner.addEventListener("click", (e) => e.stopImmediatePropagation());
			inner.addEventListener("click", () => seen.push("second"))`, "click", ""},
		{"removeEventListener", `const fn = () => seen.push("removed"); inner.addEventListener("click", fn);
			inner.removeEventListener("click", fn)`, "click", "outer attribute click"},
		{"once", `inner.addEventListener("click", () => seen.push("once"), { once: true }); inner.click()`, "click",
			"once,outer attribute click,outer attribute click"},
		{"an on property", `inner.onclick = function (e) { seen.push("property " + (this === inner)) }`, "click",
			"property true,outer attribute click"},
		{"a listener object", `inner.addEventListener("click", { handleEvent(e) { seen.push("object") } })`, "click",
			"object,outer attribute click"},
		{"keys", `inner.addEventListener("keydown", (e) => seen.push(e.key + " " + e.code + " " + e.ctrlKey + " " +
			e.keyCode + " " + e.which + " " + new KeyboardEvent("keyup", { key: "a" }).keyCode))`, "keydown",
			"Enter Enter true 13 13 65"},
		{"a throwing listener stops nothing", `inner.addEventListener("click", () => { throw new Error("broken") });
			inner.addEventListener("click", () => seen.push("after"))`, "click", "after,outer attribute click"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, page+"<script>"+c.script+"</script>")
			s.rt.Fire(s.byID("inner"), c.fire, map[string]any{"key": "Enter", "code": "Enter", "ctrlKey": true})
			if got := s.eval(`seen.join()`); got != c.want {
				t.Errorf("the page saw %q\n    want %q", got, c.want)
			}
		})
	}
}

func TestScriptsDispatchTheirOwnEvents(t *testing.T) {
	s := run(t, `<div id="box"><p id="p">text</p></div><script>var seen = [];
		box.addEventListener("saved", (e) => seen.push(e.type + " " + e.detail.n + " " + e.target.id));
		const kept = p.dispatchEvent(new CustomEvent("saved", { bubbles: true, detail: { n: 3 } }));
		p.addEventListener("ask", (e) => e.preventDefault());
		seen.push(kept, p.dispatchEvent(new Event("ask", { cancelable: true })));</script>`)
	if got := s.eval(`seen.join()`); got != "saved 3 p,true,false" {
		t.Errorf("the page saw %q", got)
	}
	s.quiet()
}

func TestAPreventedClickFollowsNoLink(t *testing.T) {
	const page = `<a id="plain" href="/plain"><b id="deep">plain</b></a><a id="held" href="/held">held</a>
		<a id="late" href="/old">late</a><script>
		document.getElementById("held").addEventListener("click", (e) => e.preventDefault());
		document.getElementById("late").addEventListener("click", (e) => { e.currentTarget.href = "/new" });</script>`
	for _, c := range []struct{ name, clicked, link, want string }{
		{"a link is followed", "plain", "plain", "https://site.test/plain"},
		{"a click inside a link follows it", "deep", "plain", "https://site.test/plain"},
		{"a prevented click follows nothing", "held", "held", ""},
		{"a link goes where it points when clicked", "late", "late", "https://site.test/new"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, page)
			s.rt.Frame()
			s.rt.Click(s.byID(c.clicked), 5, 5, dom.Keys{})
			s.page.Tree.Activate(s.byID(c.link), func() {
				href, _ := s.byID(c.link).Attr("href")
				s.opened = append(s.opened, "https://site.test"+href)
			})
			if got := strings.Join(s.opened, " "); got != c.want {
				t.Errorf("the tab opened %q, want %q", got, c.want)
			}
		})
	}
}

func TestALinkActivatedByTheKeyboardIsAClickFirst(t *testing.T) {
	s := run(t, `<a id="held" href="/held">held</a><a id="free" href="/free">free</a><script>var clicks = 0;
		document.addEventListener("click", (e) => { clicks++; if (e.target.id === "held") e.preventDefault() });</script>`)
	follow := func(to string) func() { return func() { s.opened = append(s.opened, to) } }
	s.page.Tree.Activate(s.byID("held"), follow("held"))
	s.page.Tree.Activate(s.byID("free"), follow("free"))
	if len(s.opened) != 0 {
		t.Fatalf("a link opened %q before scripts heard of it", s.opened)
	}
	s.rt.Frame()
	if got := strings.Join(s.opened, " ") + " after " + s.eval(`clicks`) + " clicks"; got != "free after 2 clicks" {
		t.Errorf("the tab opened %q", got)
	}
}

func TestTimersRunAtTheirFrames(t *testing.T) {
	s := run(t, `<script>var seen = [];
		setTimeout(() => seen.push("later"), 30); setTimeout((a, b) => seen.push("soon " + a + b), 0, 1, 2);
		const gone = setTimeout(() => seen.push("cleared"), 0); clearTimeout(gone);
		let n = 0; const every = setInterval(() => { seen.push("tick " + ++n); if (n === 2) clearInterval(every) }, 5);
		requestAnimationFrame((time) => seen.push("frame " + (typeof time)));
		queueMicrotask(() => seen.push("micro")); Promise.resolve().then(() => seen.push("promise"));
		seen.push("script");</script>`)
	if got := s.eval(`seen.join()`); got != "script,micro,promise" {
		t.Fatalf("before a frame the page saw %q", got)
	}
	s.frames("the last timer", func() bool { return strings.Contains(s.eval(`seen.join()`), "later") })
	got := s.eval(`seen.join()`)
	for _, want := range []string{"soon 12", "frame number", "tick 1", "tick 2", "later"} {
		if !strings.Contains(got, want) {
			t.Errorf("the page saw %q, without %q", got, want)
		}
	}
	if strings.Contains(got, "cleared") || strings.Contains(got, "tick 3") {
		t.Errorf("a cleared timer ran: %q", got)
	}
	if s.rt.Frame() {
		t.Error("with nothing left to run, the page still asks for frames")
	}
}

func TestARunawayScriptIsStopped(t *testing.T) {
	for _, c := range []struct{ name, page string }{
		{"a script", `<script>for (;;) {}</script>`},
		{"a listener", `<script>document.addEventListener("DOMContentLoaded", () => { for (;;) {} })</script>`},
		{"a timer", `<script>setTimeout(() => { for (;;) {} }, 0)</script>`},
		{"promises", `<script>(function again() { Promise.resolve().then(again) })()</script>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &scripted{t: t}
			began := time.Now()
			s.start(c.page+`<p id="p">kept</p><script>document.getElementById("p").textContent = "alive"</script>`,
				dom.Host{Budget: 50 * time.Millisecond})
			s.frames("the script's end", func() bool { return strings.Contains(s.said(), "too long") })
			if took := time.Since(began); took > 2*time.Second {
				t.Errorf("the page was held %v", took)
			}
			s.rt.Frame()
			s.rt.Frame()
			if got := s.eval(`document.getElementById("p").textContent + (1 + 1)`); got != "alive2" {
				t.Errorf("after the runaway script the page says %q", got)
			}
			if n := strings.Count(s.said(), "too long"); n != 1 {
				t.Errorf("the console tells of %d scripts stopped, want 1:\n%s", n, s.said())
			}
		})
	}
}

func TestAClosedPageRunsNothing(t *testing.T) {
	s := run(t, `<p id="p">text</p><script>var n = 0; setInterval(() => n++, 1)</script>`)
	s.rt.Close()
	s.rt.Close()
	if s.rt.Frame() || !s.rt.Closed() || s.rt.Nodes() != 0 {
		t.Errorf("a closed page asks for frames, or holds %d nodes", s.rt.Nodes())
	}
	s.rt.Fire(s.byID("p"), "click", nil)
	if _, err := s.rt.Evaluate("n"); err == nil {
		t.Error("a closed page evaluated a script")
	}
	if s.page.Tree.Activate != nil {
		t.Error("a closed page still answers for its links")
	}
}

func TestNodesScriptsLetGoAreForgotten(t *testing.T) {
	s := run(t, `<ul id="list"></ul><script>
		function churn() {
			const list = document.getElementById("list");
			for (let i = 0; i < 2000; i++) {
				const li = document.createElement("li");
				li.textContent = "item " + i;
				list.appendChild(li);
			}
			list.innerHTML = "";
			for (let i = 0; i < 2000; i++) document.createElement("div").appendChild(document.createTextNode("x"));
		}
		var kept = document.createElement("section"); kept.id = "kept";
		churn();</script>`)
	before := s.rt.Nodes()
	if before < 4000 {
		t.Fatalf("the scripts hold %d nodes before a sweep", before)
	}
	s.frames("the nodes to be forgotten", func() bool {
		s.eval(`churn(), new Array(1e6).fill(0).length`)
		s.rt.Sweep()
		return s.rt.Nodes() < 3*before
	})
	for range 20 {
		s.eval(`churn()`)
		s.rt.Sweep()
	}
	got := s.eval(`kept.id + " " + (document.getElementById("list") === list) + " " + list.children.length`)
	if got != "kept true 0" {
		t.Errorf("after the sweeps the page says %q", got)
	}
}

func TestScriptsFetch(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/data.json": {Status: 200, StatusText: "OK", Text: `{"n": 3}`,
			Header: map[string]string{"Content-Type": "application/json"}},
		"https://other.test/open": {Status: 200, Text: "open",
			Header: map[string]string{"Access-Control-Allow-Origin": "*"}},
		"https://other.test/closed": {Status: 200, Text: "secret"},
		"https://site.test/down":    {Err: errors.New("no route")},
	}}
	s.start(`<script>var seen = [];
		fetch("/data.json").then((r) => r.json().then((d) => seen.push("json " + r.status + " " + r.ok + " " + d.n + " " +
			r.headers.get("content-type"))));
		fetch("https://other.test/open").then((r) => r.text()).then((t) => seen.push("cors " + t));
		fetch("https://other.test/closed").then(() => seen.push("leaked"), (e) => seen.push("blocked " + e.name));
		fetch("/down").catch((e) => seen.push("down " + e.name));
		fetch("/missing").then((r) => seen.push("missing " + r.status + " " + r.ok));
		fetch("/post", { method: "POST", body: "x" }).then((r) => seen.push("post " + r.status));
		const xhr = new XMLHttpRequest(); xhr.open("GET", "/data.json");
		xhr.onload = () => seen.push("xhr " + xhr.status + " " + xhr.readyState + " " + JSON.parse(xhr.responseText).n);
		xhr.send();</script>`, dom.Host{})
	s.frames("the answers", func() bool { return s.eval(`seen.length`) == "7" })
	got := s.eval(`seen.sort().join("|")`)
	want := "blocked TypeError|cors open|down TypeError|json 200 true 3 application/json|missing 404 false|" +
		"post 404|xhr 200 4 3"
	if got != want {
		t.Errorf("the page saw %s\n        want %s", got, want)
	}
	for _, q := range s.fetched {
		if posted := strings.HasSuffix(q.Address, "/post"); posted != (q.Method == http.MethodPost) ||
			posted && string(q.Body) != "x" {
			t.Errorf("light was asked to send %s %s with %q for a script", q.Method, q.Address, q.Body)
		}
	}
	if kept := "Access to fetch at 'https://other.test/closed' from origin 'https://site.test' has been blocked by " +
		"CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource."; !strings.Contains(
		s.said(), kept) {
		t.Errorf("the console does not tell of what was kept from the script:\n%s", s.said())
	}
}

func TestAScriptAddedToThePageRuns(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/lib.js": {Status: 200, Text: `window.lib = "loaded"`},
	}}
	s.start(`<body><script>var seen = [];
		const inline = document.createElement("script"); inline.textContent = "seen.push('inline')";
		document.body.appendChild(inline);
		const lib = document.createElement("script"); lib.src = "/lib.js"; lib.onload = () => seen.push("lib " + window.lib);
		document.head.appendChild(lib);
		const gone = document.createElement("script"); gone.src = "/gone.js"; gone.onerror = () => seen.push("gone");
		document.head.appendChild(gone);
		document.body.insertAdjacentHTML("beforeend", "<script>seen.push('never')<\/script>");</script>`, dom.Host{})
	s.frames("the scripts", func() bool { return s.eval(`seen.length`) == "3" })
	if got := s.eval(`seen.sort().join()`); got != "gone,inline,lib loaded" {
		t.Errorf("the page saw %q", got)
	}
	if len(s.fetched) != 2 || !s.fetched[0].Script {
		t.Errorf("the scripts were asked for as %+v", s.fetched)
	}
}

func TestScriptsMoveThroughThePagesHistory(t *testing.T) {
	s := run(t, `<a id="top" href="#top">top</a><script>var seen = [];
		window.addEventListener("popstate", (e) => seen.push("pop " + JSON.stringify(e.state) + " " + location.pathname));
		window.addEventListener("hashchange", (e) => seen.push("hash " + location.hash));
		history.pushState({ page: 2 }, "", "/two"); seen.push(location.href + " " + history.state.page);
		history.replaceState(null, "", "/three?x=1");
		location.hash = "part";
		try { history.pushState(null, "", "https://other.test/") } catch (e) { seen.push(e.name) }</script>`)
	want := "https://site.test/two 2,hash #part,SecurityError"
	if got := s.eval(`seen.join()`); got != want {
		t.Errorf("the page saw %q\n    want %q", got, want)
	}
	moves := "push https://site.test/two replace https://site.test/three?x=1 push https://site.test/three?x=1#part"
	if got := strings.Join(s.went, " "); got != moves {
		t.Errorf("the tab was told %q\n           want %q", got, moves)
	}
	s.rt.Arrived("https://site.test/two")
	if got := s.eval(`seen[seen.length - 1]`); got != `pop {"page":2} /two` {
		t.Errorf("back at a pushed address the page saw %q", got)
	}
	s.eval(`location.href = "/elsewhere"; location.replace("https://other.test/x"); location.assign("#same")`)
	want = "https://site.test/elsewhere replace https://other.test/x push https://site.test/two#same"
	if got := strings.Join(s.went[3:], " "); got != want {
		t.Errorf("the tab went %q", got)
	}
}

type area map[string]string

func (a area) Get(key string) (string, bool) { v, ok := a[key]; return v, ok }
func (a area) Set(key, value string)         { a[key] = value }
func (a area) Remove(key string)             { delete(a, key) }
func (a area) Clear()                        { clear(a) }
func (a area) Keys() []string {
	var keys []string
	for key := range a {
		keys = append(keys, key)
	}
	return keys
}

func TestScriptsStore(t *testing.T) {
	local := area{"seen": "1"}
	s := &scripted{t: t, stores: map[string]dom.Storage{"local": local}}
	s.start(`<script>localStorage.setItem("name", "Ann"); localStorage.count = 2; sessionStorage.setItem("tab", "t");
		document.cookie = "a=1; path=/"; document.cookie = "b=2"; document.cookie = "a=; path=/; max-age=0";</script>`,
		dom.Host{})
	if local["name"] != "Ann" || local["count"] != "2" {
		t.Errorf("the browser's storage holds %v", local)
	}
	want := "1 Ann 3  t  b=2"
	if got := s.eval(`[localStorage.getItem("seen"), localStorage.name, localStorage.length, localStorage.getItem("no"),
		sessionStorage.tab, localStorage.getItem("tab") || "", document.cookie].join(" ")`); got != want {
		t.Errorf("the page reads %q, want %q", got, want)
	}
	s.eval(`localStorage.removeItem("seen"); delete localStorage.count`)
	if len(local) != 1 {
		t.Errorf("after two removals the storage holds %v", local)
	}
	s.quiet()
}

func TestMutationObserversAreTold(t *testing.T) {
	s := run(t, `<div id="box"><p id="p">text</p></div><script>var seen = [];
		new MutationObserver((records) => { for (const r of records) seen.push(r.type + " " + r.target.id + " " +
			(r.attributeName || r.addedNodes.length + "+" + r.removedNodes.length)) })
			.observe(box, { childList: true, attributes: true, subtree: true });
		p.setAttribute("title", "T"); box.appendChild(document.createElement("i")); p.remove();
		seen.push("script");</script>`)
	if got := s.eval(`seen.join()`); got != "script,attributes p title,childList box 1+0,childList box 0+1" {
		t.Errorf("the observer saw %q", got)
	}
}

func TestLayoutIsReadByScripts(t *testing.T) {
	s := run(t, `<body style="margin: 0"><div id="box" style="width: 200px; height: 50px; margin: 10px 0 0 20px">box</div>
		<div id="gone" style="display: none">gone</div>`)
	want := "20,10,200,50 200x50 220 0 none block 200px"
	if got := s.eval(`(r => [r.left, r.top, r.width, r.height].join() + " " + box.offsetWidth + "x" +
		box.offsetHeight + " " + r.right + " " + gone.offsetWidth + " " + getComputedStyle(gone).display + " " +
		getComputedStyle(box).display + " " +
		getComputedStyle(box).width)(box.getBoundingClientRect())`); got != want {
		t.Errorf("the page measures %q, want %q", got, want)
	}
	if got := s.eval(`document.elementFromPoint(30, 20).id + matchMedia("(min-width: 700px)").matches +
		matchMedia("(max-width: 700px)").matches`); got != "boxtruefalse" {
		t.Errorf("the page finds %q", got)
	}
}

func TestWhatFieldsTellUIIsToldToScripts(t *testing.T) {
	s := run(t, `<form id="form"><input id="name"><input id="agree" type="checkbox"><button id="send">Send</button>
		<button id="plain" type="button">Plain</button></form><input id="alone"><script>var seen = [];
		for (const type of ["input", "change", "submit"]) {
			document.addEventListener(type, (e) => seen.push(type + " " + e.target.id + " " + (e.target.value || "")));
		}
		alone.addEventListener("keydown", (e) => {
			seen.push("key " + e.key);
			if (window.hold) e.preventDefault();
		});</script>`)
	s.page.Doc.SetValue(s.byID("name"), "Ann")
	s.page.Tree.Heard(s.byID("name"), "input", dom.Keys{})
	s.page.Tree.Heard(s.byID("agree"), "change", dom.Keys{})
	s.page.Tree.Heard(s.byID("name"), "submit", dom.Keys{})
	s.page.Tree.Heard(s.byID("alone"), "submit", dom.Keys{})
	if got := s.eval(`seen.join()`); got != "" {
		t.Fatalf("scripts ran while ui told of its fields: %q", got)
	}
	s.rt.Frame()
	if got := s.eval(`seen.join()`); got != "input name Ann,change agree on,submit form ,key Enter,submit alone " {
		t.Errorf("the page saw %q", got)
	}
	s.eval(`seen.length = 0; window.hold = true`)
	s.page.Tree.Heard(s.byID("alone"), "submit", dom.Keys{})
	s.rt.Frame()
	if got := s.eval(`seen.join()`); got != "key Enter" {
		t.Errorf("with Enter prevented the page saw %q", got)
	}
	s.eval(`seen.length = 0`)
	s.rt.Click(s.byID("plain"), 1, 1, dom.Keys{})
	s.rt.Click(s.byID("send"), 1, 1, dom.Keys{})
	if got := s.eval(`seen.join()`); got != "submit form " {
		t.Errorf("after its buttons were clicked the page saw %q", got)
	}
	s.eval(`seen.length = 0; send.addEventListener("click", (e) => e.preventDefault())`)
	s.rt.Click(s.byID("send"), 1, 1, dom.Keys{})
	if got := s.eval(`seen.join()`); got != "" {
		t.Errorf("a prevented click submitted: %q", got)
	}
}

func TestKeysCarryTheModifiersHeldWithThem(t *testing.T) {
	s := run(t, `<script>var seen = [];
		for (const type of ["keydown", "keyup"]) {
			document.addEventListener(type, (e) => seen.push([type, e.key, e.shiftKey, e.altKey, e.ctrlKey, e.metaKey,
				e.getModifierState("Shift"), e.getModifierState("Control")].join(" ")));
		}</script>`)
	s.rt.Key("ArrowDown", "ArrowDown", dom.Keys{Shift: true, Ctrl: true})
	s.rt.Key("x", "KeyX", dom.Keys{Alt: true, Meta: true})
	s.rt.Frame()
	want := "keydown ArrowDown true false true false true true,keyup ArrowDown true false true false true true," +
		"keydown x false true false true false false,keyup x false true false true false false"
	if got := s.eval(`seen.join()`); got != want {
		t.Errorf("the page saw %q\n    want %q", got, want)
	}
}

func TestKeysAreToldToTheFocusedElement(t *testing.T) {
	s := run(t, `<input id="name"><input id="other"><script>var seen = [];
		for (const type of ["keydown", "keyup"]) {
			document.addEventListener(type, (e) => seen.push([type, e.target.id || e.target.tagName, e.key, e.code].join(" ")));
		}</script>`)
	s.rt.Key("a", "KeyA", dom.Keys{})
	s.rt.Frame()
	frame := s.window()
	s.page.Doc.Focus(s.byID("name"))
	frame()
	s.rt.Frame()
	s.rt.Key("Enter", "Enter", dom.Keys{})
	s.rt.Frame()
	want := "keydown BODY a KeyA,keyup BODY a KeyA,keydown name Enter Enter,keyup name Enter Enter"
	if got := s.eval(`seen.join()`); got != want {
		t.Errorf("the page saw %q\n    want %q", got, want)
	}
	if got := s.eval(`document.activeElement.id + " " + seen.length`); got != "name 4" {
		t.Errorf("the active element is %q", got)
	}
	s.eval(`document.getElementById("name").addEventListener("blur", (e) => seen.push("blur " + e.target.id));
		document.addEventListener("focusin", (e) => seen.push("focusin " + e.target.id)); other.focus()`)
	frame()
	s.rt.Frame()
	if got := s.eval(`seen.slice(4).join() + " " + document.activeElement.id`); got != "blur name,focusin other other" {
		t.Errorf("as the focus moved the page saw %q", got)
	}
	s.eval(`document.getElementById("name").blur(); other.blur()`)
	frame()
	s.rt.Frame()
	if s.page.Doc.Focused(s.byID("other")) || s.eval(`document.activeElement === document.body`) != "true" {
		t.Error("blur() left the focus where it was")
	}
}
