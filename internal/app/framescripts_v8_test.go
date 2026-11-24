//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/script"
	"github.com/lantervale/light/pkg/tabs"
)

// noted is the script of a document that notes the messages it gets.
const noted = `<script>window.got = [];
	addEventListener("message", (e) => { got.push(e.origin + "|" + e.data); });</script>`

// running waits for the documents of those frames to run their
// scripts, the page's for "".
func (s *shown) running(ids ...string) {
	s.t.Helper()
	s.until("the scripts of "+strings.Join(ids, ", ")+" ran", func() bool {
		for _, id := range ids {
			if id == "" && s.session().scripts == nil {
				return false
			}
			if f := s.frameNamed(id); id != "" && (f == nil || f.run == nil || f.load != nil) {
				return false
			}
		}
		return true
	})
}

// becomes waits for an expression in the document of a frame to be
// want.
func (s *shown) becomes(from, expression, want string) {
	s.t.Helper()
	got := ""
	for range 400 {
		s.frame()
		if f := s.frameNamed(from); from == "" && s.session().scripts != nil || f != nil && f.run != nil {
			if got = s.within(from, expression); got == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.t.Fatalf("in %q, %s stayed %s, want %s; the console says %s", from, expression, got, want, s.said())
}

func TestTheScriptsOfAFramesDocumentRun(t *testing.T) {
	base, baseContexts := script.Live(), script.Contexts()
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<body style="margin:0"><iframe id="one" src="https://other.test/inner" width="400"
			height="120"></iframe><iframe id="plain" src="/plain"></iframe>
			<iframe id="shut" sandbox src="/inner2"></iframe><script>
			window.heard = [];
			addEventListener("click", () => heard.push("click"));
			addEventListener("keydown", (e) => heard.push("key " + e.key));
			console.log("the page says hi");</script>`),
		"other.test/inner": html(`<body style="margin:0"><button id="more" style="display:block;height:24px">more</button>
			<p id="out">none</p><iframe id="deep" src="/deep" width="100" height="30"></iframe>
			<script src="/classic.js"></script><script type="module" src="/mod.js"></script>
			<noscript>no script</noscript>`),
		"other.test/classic.js": typed("text/javascript", `document.getElementById("out").textContent = "classic";
			window.heard = [];
			setTimeout(() => { window.timed = "timed"; }, 10);
			fetch("/data").then((r) => r.text()).then((text) => { window.fetched = text; });
			fetch("https://site.test/shared").then((r) => r.text()).then((text) => { window.shared = text; });
			fetch("https://site.test/closed").catch((e) => { window.kept = e.name; });
			document.getElementById("more").addEventListener("click", () => heard.push("click"));
			addEventListener("keydown", (e) => heard.push("key " + e.key));
			document.getElementById("deep").addEventListener("load", () => heard.push("deep loaded"));
			addEventListener("message", (e) => heard.push(e.origin + " " + e.data));
			console.log("the frame says hi");`),
		"other.test/mod.js": typed("text/javascript", `import { v } from "/dep.js"; window.mod = v;`),
		"other.test/dep.js": typed("text/javascript", `export const v = "module ran";`),
		"other.test/data":   typed("text/plain", "data"),
		"site.test/shared":  with(typed("text/plain", "shared"), "Access-Control-Allow-Origin", "https://other.test"),
		"site.test/closed":  typed("text/plain", "closed"),
		"other.test/deep":   html(`<p>deep</p><script>parent.postMessage("from deep", "*");</script>`),
		"site.test/plain":   html(`<p>plain</p>`),
		"site.test/inner2": html(`<p>shut</p><script>fetch("/ran2");</script>
			<noscript><p>shown without scripts</p></noscript>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "one", "deep")
	for expression, want := range map[string]string{
		`document.getElementById("out").textContent`: "classic", `mod`: "module ran", `timed`: "timed", `fetched`: "data",
		`shared`: "shared", `kept`: "TypeError",
		`origin + " " + location.href`: "https://other.test https://other.test/inner",
		`heard.join()`:                 "https://other.test from deep,deep loaded",
	} {
		s.becomes("one", expression, want)
	}
	if got := s.framedText("one"); !strings.HasPrefix(got, "more classic") || strings.Contains(got, "no script") {
		t.Errorf("the frame whose scripts ran shows %q", got)
	}
	if got := s.framedText("shut"); got != "shut shown without scripts" || f.came("site.test/ran2") {
		t.Errorf("a sandboxed frame without allow-scripts shows %q; its script asked: %t", got, f.came("site.test/ran2"))
	}
	s.framedText("plain")
	if engines, documents := script.Live()-base, script.Contexts()-baseContexts; engines != 2 || documents != 3 {
		t.Errorf("%d engines run %d documents, want 2 and 3: the page's, and one for the two frames of another site "+
			"that have scripts, which are of one origin", engines, documents)
	}
	for at, want := range map[string]map[string]string{
		"other.test/data": {"Origin": "", "Referer": "https://other.test/inner", "Sec-Fetch-Site": "same-origin",
			"Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"},
		"site.test/shared": {"Origin": "https://other.test", "Referer": "https://other.test/",
			"Sec-Fetch-Site": "cross-site"},
		"other.test/mod.js": {"Referer": "https://other.test/inner", "Sec-Fetch-Dest": "script"},
	} {
		for name, value := range want {
			if got := f.header(at, name); got != value {
				t.Errorf("the request for %s says %s: %q, want %q", at, name, got, value)
			}
		}
	}

	s.clickAt(s.inFrame(s.frameAt("one"), "more"))
	s.press(input.KeyEnter)
	s.becomes("one", `heard.slice(2).join()`, "click,key Enter")
	if got := s.within("", `heard.join()`); got != "" {
		t.Errorf("the page heard %q of a click and a key in its frame", got)
	}
	for text, frame := range map[string]string{"the frame says hi": "other.test", "the page says hi": ""} {
		found := false
		for _, line := range s.session().said.Lines() {
			found = found || line.Text == text && line.Frame == frame
		}
		if !found {
			t.Errorf("the console has no line %q of the frame %q: %s", text, frame, s.said())
		}
	}
	if s.consoleSays(console.Error, "Uncaught") || !s.consoleSays(console.Error, "'https://site.test/closed' from origin "+
		"'https://other.test' has been blocked by CORS policy") {
		t.Errorf("the console says %s", s.said())
	}
}

func TestAMessageComesToTheDocumentAndTheOriginItWasPostedFor(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="f" src="https://other.test/a"></iframe>
			<iframe id="boxed" sandbox="allow-scripts" src="https://other.test/a"></iframe>` + noted),
		"other.test/a": html(`<p>a</p>` + noted),
		"third.test/b": html(`<p>b</p>` + noted),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "f", "boxed")
	const w = `document.getElementById("f").contentWindow`
	s.does("", w+`.postMessage("1", "https://other.test"); `+w+`.postMessage("2", "https://third.test"); `+
		w+`.postMessage("3", "*"); `+w+`.postMessage("4", "/"); `+w+`.postMessage("5", "https://other.test/a?x#y")`)
	s.becomes("f", `got.join()`, "https://site.test|1,https://site.test|3,https://site.test|5")

	s.within("f", `(() => {
		try { window.origin = "https://site.test"; } catch (_) {}
		try { Object.defineProperty(window, "origin", {value: "https://site.test"}); } catch (_) {}
		try { Object.defineProperty(location, "origin", {value: "https://site.test"}); } catch (_) {}
		try { document.domain = "site.test"; } catch (_) {}
		try { history.replaceState(null, "", "https://site.test/"); } catch (_) {}
		parent.postMessage("forged", "*"); parent.postMessage("own", "/"); parent.postMessage("aimed", "https://site.test");
		parent.postMessage("astray", "https://other.test");
	})()`)
	if got := s.within("f", `(() => { try { parent.dispatchEvent(new MessageEvent("message", {origin: "https://site.test",
		data: "fake"})); } catch (e) { return e.name; } })()`); got != "SecurityError" {
		t.Errorf("a frame told its parent of an event of its own making: %s", got)
	}
	s.does("boxed", `parent.postMessage("boxed", "*")`)
	s.becomes("", `got.join()`, "https://other.test|forged,https://other.test|aimed,null|boxed")
	s.does("", `frames[1].postMessage("to its address", "https://other.test"); frames[1].postMessage("to all", "*")`)
	s.becomes("boxed", `got.join()`, "https://site.test|to all")

	f := s.frameAt("f")
	s.does("", w+`.postMessage("late", "*"); `+w+`.postMessage("late too", "https://third.test")`)
	page := s.frameLoader(f).Load(context.Background(), "https://third.test/b", s.frameLinks(f))
	s.showIn(f, page)
	f.origin = f.originOf(page)
	s.running("f")
	s.does("", w+`.postMessage("for a", "https://other.test"); `+w+`.postMessage("for b", "https://third.test")`)
	s.becomes("f", `origin + " " + got.join()`, "https://third.test https://site.test|for b")

	s.does("", w+`.postMessage("gone", "*"); window.was = `+w+`; document.getElementById("f").remove()`)
	for range 5 {
		s.frame()
	}
	const after = `was.closed + " " + frames.length + " " + (() => { was.postMessage("x", "*"); return "ok"; })()`
	if got := s.within("", after); got != "true 1 ok" {
		t.Errorf("the window of a frame that was removed is %s", got)
	}
	if s.consoleSays(console.Error, "") {
		t.Errorf("the console says %s", s.said())
	}
}

// keeps is a document that notes what it finds kept for it, then keeps
// its own name.
const keeps = `<script>
	const name = location.search.slice(1);
	const read = (area) => { try { return String(area().getItem("who")); } catch (e) { return e.name; } };
	window.found = [read(() => localStorage), read(() => sessionStorage), document.cookie].join("|");
	try { localStorage.setItem("who", name); sessionStorage.setItem("who", name); } catch (_) {}
	document.cookie = "who=" + name + "; SameSite=None; Secure";
	fetch("/asked?" + name, {credentials: "include"})
		.then(() => { window.asked = "read"; }, () => { window.asked = "kept"; });
</script>`

func TestWhatASiteKeepsInAFrameOfAnotherIsKeptApart(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/":   html(`<iframe id="w" src="https://widget.test/w?under-site"></iframe>` + noted),
		"blog.test/":   html(`<iframe id="w" src="https://widget.test/w?under-blog"></iframe>` + noted),
		"widget.test/": html(`<iframe id="w" src="/w?in-itself"></iframe>` + noted),
		"site.test/boxed": html(`<iframe id="w" sandbox="allow-scripts" src="https://widget.test/w?boxed"></iframe>` +
			noted),
		"widget.test/w":      html(`<p>widget</p>` + keeps),
		"widget.test/asked":  typed("text/plain", "ok"),
		"widget.test/direct": html(`<p>direct</p>` + keeps),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.configure(func(c *config.Config) { c.ThirdPartyCookies = true })
	found := func(top, want, cookies string) {
		t.Helper()
		s.visit(top)
		if top == "https://widget.test/direct?first" {
			s.becomes("", `found + " " + asked`, want)
		} else {
			s.becomes("w", `found + " " + asked`, want)
		}
		if got := f.header("widget.test/asked", "Cookie"); got != cookies {
			t.Errorf("under %s the document's request carried %q, want %q", top, got, cookies)
		}
		s.Open(tabs.Home)
		s.frame()
	}
	found("https://site.test/", "null|null| read", "who=under-site")
	found("https://widget.test/direct?first", "null|null| read", "who=first")
	found("https://blog.test/", "null|null| read", "who=under-blog")
	found("https://site.test/", "under-site|under-site|who=under-site read", "who=under-site")
	found("https://widget.test/direct?first", "first|first|who=first read", "who=first")
	found("https://widget.test/", "first|first|who=first read", "who=in-itself")
	found("https://site.test/boxed", "SecurityError|SecurityError| kept", "")
	found("https://blog.test/", "under-blog|under-blog|who=under-blog read", "who=under-blog")
	for name, area := range map[string]interface{ Get(string) (string, bool) }{
		"in-itself":  s.sites.Area("https://widget.test"),
		"under-site": s.sites.Framed("https://site.test", "https://widget.test"),
		"under-blog": s.sites.Framed("https://blog.test", "https://widget.test"),
	} {
		if got, _ := area.Get("who"); got != name {
			t.Errorf("the storage kept %q where %q belongs", got, name)
		}
	}

	s.configure(func(c *config.Config) { c.ThirdPartyCookies = false })
	s.visit("https://site.test/")
	s.becomes("w", `found.split("|")[2] + "," + document.cookie + "," + asked`, ",,read")
	if got := f.header("widget.test/asked", "Cookie"); got != "" {
		t.Errorf("with third parties kept out the frame's request carried %q", got)
	}
}

// acts is a document whose scripts try what a sandbox may keep them
// from, when asked to.
const acts = `<body style="margin:0"><button id="go" style="display:block;height:24px">go</button>
	<form id="form" action="/sent"><input name="q" value="1"></form><script>
	window.act = (what) => {
		try {
			if (what === "top") top.location = "https://other.test/away";
			if (what === "top replace") top.location.replace("https://other.test/away");
			if (what === "sibling") parent.frames[1].location = "https://other.test/away";
			if (what === "script") parent.location = "javascript:document.title = 'ran'";
			if (what === "self") location.href = "/away";
			if (what === "submit") document.getElementById("form").submit();
			if (what === "alert") return String(alert("hello")) + confirm("sure") + prompt("what");
			return "asked";
		} catch (e) { return e.name; }
	};
	document.getElementById("go").addEventListener("click", () => act(new URLSearchParams(location.search).get("click")));
</script>`

func TestAFramesScriptsSendOtherFramesOnlyWhereTheyMay(t *testing.T) {
	web, transport := framingOf(t, map[string]answer{
		"site.test/acts":   html(acts),
		"other.test/acts":  html(acts),
		"other.test/away":  html(`<title>Away</title><p>away</p>`),
		"site.test/away":   html(`<p>away here</p>`),
		"site.test/sent":   html(`<p>sent</p>`),
		"site.test/second": html(`<p>second</p>`),
		"other.test/sent":  html(`<p>sent there</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	for _, c := range []struct {
		name, host, sandbox, act string
		click                    bool
		// tab is whether the tab left its page, one and two what the
		// frames then show.
		tab      bool
		one, two string
		says     string
	}{
		{name: "another site, unasked", host: "other.test", act: "top", one: "go", two: "second", says: "Unsafe attempt"},
		{name: "another site, on a click", host: "other.test", act: "top", click: true, tab: true},
		{name: "its own site", host: "site.test", act: "top replace", tab: true},
		{name: "sandboxed", host: "other.test", sandbox: "allow-scripts", act: "top", click: true, one: "go", two: "second",
			says: "Unsafe attempt"},
		{name: "sandboxed with allow-top-navigation", host: "other.test", sandbox: "allow-scripts allow-top-navigation",
			act: "top", tab: true},
		{name: "sandboxed with allow-top-navigation-by-user-activation, unasked", host: "other.test",
			sandbox: "allow-scripts allow-top-navigation-by-user-activation", act: "top", one: "go", two: "second",
			says: "Unsafe attempt"},
		{name: "sandboxed with allow-top-navigation-by-user-activation, on a click", host: "other.test",
			sandbox: "allow-scripts allow-top-navigation-by-user-activation", act: "top", click: true, tab: true},
		{name: "a sibling of another site", host: "other.test", act: "sibling", click: true, one: "go", two: "second",
			says: "Unsafe attempt"},
		{name: "a sibling of its own site", host: "site.test", act: "sibling", one: "go", two: "away"},
		{name: "a sibling of its own site, sandboxed", host: "site.test", sandbox: "allow-scripts allow-same-origin",
			act: "sibling", one: "go", two: "second", says: "Unsafe attempt"},
		{name: "a script address", host: "site.test", act: "script", one: "go", two: "second", says: "Unsafe attempt"},
		{name: "itself", host: "other.test", sandbox: "allow-scripts", act: "self", one: "away", two: "second"},
		{name: "a form, sandboxed", host: "other.test", sandbox: "allow-scripts", act: "submit", one: "go", two: "second",
			says: "A form was not sent"},
		{name: "a form, with allow-forms", host: "other.test", sandbox: "allow-scripts allow-forms", act: "submit",
			one: "sent there", two: "second"},
		{name: "a question, sandboxed", host: "other.test", sandbox: "allow-scripts", act: "alert", one: "go", two: "second",
			says: "'allow-modals' keyword is not set"},
	} {
		sandbox := ""
		if c.sandbox != "" {
			sandbox = ` sandbox="` + c.sandbox + `"`
		}
		top := "https://site.test/" + strings.ReplaceAll(c.name, " ", "-")
		src := "https://" + c.host + "/acts?click=" + strings.ReplaceAll(c.act, " ", "+")
		web.add(strings.TrimPrefix(top, "https://"), html(`<title>Top</title><body style="margin:0">
			<iframe id="one" width="300" height="60" src="`+src+`"`+sandbox+`></iframe>
			<iframe id="two" src="/second"></iframe>`+noted))
		s.visit(top)
		s.running("", "one")
		s.framedText("two")
		s.session().said.Clear()
		if c.click {
			s.clickAt(s.inFrame(s.frameAt("one"), "go"))
		} else if got := s.within("one", `act("`+c.act+`")`); c.act == "alert" && got != "undefinedfalsenull" {
			t.Errorf("%s: the questions answered %s", c.name, got)
		}
		if c.tab {
			s.until(c.name+": the tab left its page", func() bool {
				return s.browser.Tab().Current() == "https://other.test/away" && s.session().pages["https://other.test/away"] != nil
			})
			continue
		}
		s.until(c.name+": the frames settled", func() bool {
			one, two := s.frameNamed("one"), s.frameNamed("two")
			return one != nil && two != nil && one.load == nil && two.load == nil && one.page != nil && two.page != nil &&
				strings.Join(strings.Fields(one.page.Doc.Body().Text()), " ") == c.one
		})
		for range 4 {
			s.frame()
		}
		if got := s.browser.Tab().Current(); got != top {
			t.Fatalf("%s: the tab went to %s", c.name, got)
		}
		if got := s.framedText("two"); got != c.two {
			t.Errorf("%s: the other frame shows %q, want %q", c.name, got, c.two)
		}
		if c.says != "" && !strings.Contains(s.said(), c.says) || c.says == "" && s.said() != "" {
			t.Errorf("%s: the console says %q, want %q in it", c.name, s.said(), c.says)
		}
		if title := s.session().pages[top].Title; title != "Top" {
			t.Errorf("%s: the page's title became %q", c.name, title)
		}
	}
}

func TestAFrameThatFailsLeavesTheOthersRunning(t *testing.T) {
	base, baseContexts := script.Live(), script.Contexts()
	const ticking = `<script>window.ticks = 0; setInterval(() => { ticks++; }, 5);</script>`
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="throws" src="https://other.test/throws"></iframe>
			<iframe id="spins" src="https://third.test/spins"></iframe><iframe id="fine" src="https://other.test/fine"></iframe>
			<iframe id="breaks" src="/breaks"></iframe>` + ticking),
		"other.test/throws": html(`<script>setTimeout(() => { throw new Error("thrown in a frame"); }, 20);
			window.started = true; missing.call();</script>` + ticking),
		"third.test/spins": html(`<script>window.spun = 0; setTimeout(() => { spun = 1; for (;;) {} }, 30);</script>` +
			ticking),
		"other.test/fine":  html(ticking),
		"site.test/breaks": html(ticking),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir(), ScriptBudget: 100 * time.Millisecond})
	s.visit("https://site.test/")
	s.running("", "throws", "spins", "fine", "breaks")
	s.until("the frame that spins was stopped", func() bool { return s.consoleSays(console.Error, "ran too long") })
	s.until("the frame that throws threw", func() bool { return s.consoleSays(console.Error, "thrown in a frame") })
	s.frameAt("breaks").page.Doc.Body().Append(breaking())
	s.until("the frame that broke light was emptied", func() bool { return s.consoleSays(console.Error, "drawn wrong") })
	if engines, documents := script.Live()-base, script.Contexts()-baseContexts; engines != 3 || documents != 4 {
		t.Errorf("%d engines run %d documents once a frame broke light, want 3 and 4", engines, documents)
	}
	before := map[string]string{}
	for _, id := range []string{"", "throws", "spins", "fine"} {
		before[id] = s.within(id, `String(ticks)`)
	}
	for _, id := range []string{"", "throws", "spins", "fine"} {
		s.becomes(id, `ticks > `+before[id]+` + 3`, "true")
	}
	if got := s.within("throws", `String(window.started)`) + s.within("spins", `String(spun)`); got != "true1" {
		t.Errorf("the frames that failed hold %s", got)
	}
	for text, frame := range map[string]string{"thrown in a frame": "other.test", "ran too long": "third.test",
		"missing is not defined": "other.test"} {
		found := false
		for _, line := range s.session().said.Lines() {
			found = found || strings.Contains(line.Text, text) && line.Frame == frame
		}
		if !found {
			t.Errorf("the console has no line %q of the frame %s: %s", text, frame, s.said())
		}
	}
}

func TestFramesThatComeAndGoLeaveNoEngineBehind(t *testing.T) {
	base, baseContexts := script.Live(), script.Contexts()
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<p id="said"></p><script>
			addEventListener("message", (e) => { document.getElementById("said").textContent = e.data; });
			window.add = (n) => {
				const frame = document.createElement("iframe");
				frame.id = "made";
				frame.src = "https://other.test/inner?" + n;
				document.body.append(frame);
			};
			window.drop = () => document.getElementById("made").remove();</script>`),
		"other.test/inner": html(`<iframe id="deep" src="/deep"></iframe><script>
			setInterval(() => {}, 5); fetch("/slow").catch(() => {});
			parent.postMessage("ready " + location.search.slice(1), "*");</script>`),
		"other.test/deep": html(`<script>setInterval(() => {}, 5);</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("")
	cycle := func(n int) {
		s.does("", `add(`+itoa(n)+`)`)
		s.until("the frame said it was ready", func() bool { return s.pageText("said") == "ready "+itoa(n) })
		s.running("deep")
		if engines, documents := script.Live()-base, script.Contexts()-baseContexts; engines != 2 || documents != 3 {
			t.Fatalf("round %d: %d engines run %d documents, want 2 and 3", n, engines, documents)
		}
		s.does("", `drop()`)
		s.frame()
		s.frame()
		if engines, documents := script.Live()-base, script.Contexts()-baseContexts; engines != 1 || documents != 1 {
			t.Fatalf("round %d: %d engines run %d documents once the frame was removed, want the page's alone", n, engines,
				documents)
		}
	}
	cycle(0)
	settle := func() int {
		for range 20 {
			s.frame()
			time.Sleep(5 * time.Millisecond)
		}
		return runtime.NumGoroutine()
	}
	routines := settle()
	for n := 1; n <= 50; n++ {
		cycle(n)
	}
	if after := settle(); after > routines+4 {
		t.Errorf("%d goroutines run after 50 frames came and went, %d before", after, routines)
	}
	if set := s.framesOfPage(); len(set.all) > idleFrames || len(set.mail) != 0 {
		t.Errorf("the page still holds %d frames and %d messages", len(set.all), len(set.mail))
	}

	s.does("", `add(51)`)
	s.running("made", "deep")
	s.OpenInNewTab(tabs.Home)
	s.frame()
	if engines, documents := script.Live()-base, script.Contexts()-baseContexts; engines != 2 || documents != 3 {
		t.Errorf("%d engines and %d documents are kept for a tab that is not shown, want 2 and 3", engines, documents)
	}
	s.shut()
	if got := script.Live() - base; got != 0 {
		t.Errorf("%d engines still run once the window is shut", got)
	}
}
