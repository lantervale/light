//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

// relay is the script of a document that keeps the ports it is given,
// notes what comes through them, and does what a message asks: hand
// the port on to another frame of its parent, or close it.
const relay = `<script>window.got = []; window.held = [];
	const hear = (port) => {
		held.push(port);
		port.onmessage = (m) => {
			got.push(String(m.data instanceof Blob ? "blob " + m.data.size : m.data) + (m.ports.length ? " +port" : ""));
			if (m.ports.length) hear(m.ports[0]);
			if (m.data === "close") port.close();
		};
		port.addEventListener("close", () => got.push("closed"));
	};
	addEventListener("message", (e) => {
		if (e.data === "keep") { window.unread = e.ports[0]; return; }
		if (typeof e.data === "object" && e.data.onward !== undefined) {
			parent.frames[e.data.onward].postMessage("handed on", "*", [window.unread || held.pop()]);
			return;
		}
		got.push(e.origin + "|" + e.data + (e.ports.length ? " +port" : ""));
		if (e.ports.length) hear(e.ports[0]);
	});</script>`

func ported(t *testing.T) *shown {
	t.Helper()
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="a" src="https://other.test/kid"></iframe>
			<iframe id="b" src="https://third.test/kid"></iframe>
			<iframe id="c" sandbox="allow-scripts" src="/kid"></iframe>` + relay),
		"other.test/kid": html(`<p>kid</p>` + relay),
		"third.test/kid": html(`<p>kid</p>` + relay),
		"site.test/kid":  html(`<p>kid</p>` + relay),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "a", "b", "c")
	return s
}

func (s *shown) ends() int { return len(s.framesOfPage().ports) }

func TestAChannelJoinsAnyTwoFramesOfATab(t *testing.T) {
	s := ported(t)
	s.does("a", `window.chan = new MessageChannel(); hear(chan.port1);
		chan.port1.postMessage("to no one yet");
		parent.frames[1].postMessage("a port", "*", [chan.port2]);`)
	s.becomes("b", `got.join()`, "https://other.test|a port +port,to no one yet")
	s.does("a", `chan.port1.postMessage("one"); chan.port1.postMessage(new Blob(["12345"]));
		chan.port1.postMessage("three");`)
	s.becomes("b", `got.slice(2).join()`, "one,blob 5,three")
	s.does("b", `held[0].postMessage("back");`)
	s.becomes("a", `got.join()`, "back")
	if got := s.within("", `got.join()`); got != "" {
		t.Errorf("the page between two frames heard %q", got)
	}
	s.does("b", `postMessage({onward: 2}, "*");`)
	s.becomes("c", `got.join()`, "https://third.test|handed on +port")
	s.does("a", `chan.port1.postMessage("to the third");`)
	s.becomes("c", `got.slice(1).join()`, "to the third")
	if got := s.within("b", `(() => { held.length; try { got.length = 0; } catch (_) {} return held.length; })()`); got !=
		"0" {
		t.Errorf("the frame that handed the port on still holds %s", got)
	}
	s.does("c", `held[0].postMessage("from a document with no origin");`)
	s.becomes("a", `got.slice(1).join()`, "from a document with no origin")
	if got := s.within("b", `got.join()`); got != "" {
		t.Errorf("the frame that gave the port away heard %q", got)
	}
	s.does("a", `chan.port1.postMessage("close");`)
	s.becomes("a", `got.at(-1)`, "closed")
	s.until("the channel is no more", func() bool { return s.ends() == 0 })
}

func TestAPortKeepsWhatItNeverReadWhenItGoesOn(t *testing.T) {
	s := ported(t)
	s.does("", `window.chan = new MessageChannel(); hear(chan.port1);
		frames[0].postMessage("keep", "*", [chan.port2]);`)
	s.becomes("a", `typeof unread`, "object")
	s.does("", `chan.port1.postMessage("first"); chan.port1.postMessage("second");`)
	for range 5 {
		s.frame()
	}
	s.does("a", `postMessage({onward: 1}, "*");`)
	s.does("", `chan.port1.postMessage("third");`)
	s.becomes("b", `got.join()`, "https://other.test|handed on +port,first,second,third")
}

func TestAPortOfAFrameThatGoesAwayCloses(t *testing.T) {
	s := ported(t)
	base := script.Live()
	s.does("", `window.closes = 0; window.answers = 0;`)
	for i := range 50 {
		n := strconv.Itoa(i)
		s.does("", `const chan = new MessageChannel(), frame = document.getElementById("a");
			chan.port1.onmessage = () => { answers++; frame.remove(); };
			chan.port1.addEventListener("close", () => {
				closes++;
				const next = document.createElement("iframe");
				next.id = "a"; next.src = "https://other.test/kid?`+n+`";
				document.body.prepend(next);
			});
			window.kept = chan.port1;
			frames[0].postMessage("a port", "*", [chan.port2]);`)
		s.becomes("a", `got.length`, "1")
		s.does("a", `held[0].postMessage("got it"); window.more = new MessageChannel();
			parent.postMessage("another", "*", [more.port1]);`)
		s.becomes("", `closes`, strconv.Itoa(i+1))
		s.running("a")
		s.becomes("a", `got.length`, "0")
	}
	if got := s.within("", `answers + " " + held.length`); got != "50 50" {
		t.Errorf("the page got %s answers and ports", got)
	}
	s.does("", `for (const port of held) port.close(); held.length = 0;`)
	s.until("the channels of the frames that left are no more", func() bool { return s.ends() == 0 })
	if set := s.framesOfPage(); len(set.mail) != 0 || len(set.all) > 3+idleFrames {
		t.Errorf("the page keeps %d messages and %d frames", len(set.mail), len(set.all))
	}
	if got := script.Live() - base; got != 0 {
		t.Errorf("%d more engines run after frames with ports came and went", got)
	}
}

func TestATalkBetweenFramesDoesNotWaitAFrameForEachAnswer(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="a" src="https://other.test/kid"></iframe><script>
			window.count = 0; window.through = 0;
			addEventListener("message", (e) => {
				if (e.ports.length) { window.port = e.ports[0]; port.onmessage = (m) => {
					through = m.data; if (m.data < 100) port.postMessage(m.data + 1); }; return; }
				if (e.data === "a port") return;
				count = e.data; if (e.data < 100) e.source.postMessage(e.data + 1, "*");
				else window.done = true;
			});</script>`),
		"other.test/kid": html(`<script>
			window.chan = new MessageChannel();
			chan.port1.onmessage = (m) => chan.port1.postMessage(m.data);
			addEventListener("message", (e) => e.source.postMessage(e.data, "*"));
			parent.postMessage("a port", "*", [chan.port2]);</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "a")
	s.becomes("", `typeof port`, "object")
	for name, talk := range map[string][2]string{
		"windows": {`frames[0].postMessage(0, "*");`, `count`}, "ports": {`port.postMessage(0);`, `through`},
	} {
		s.does("", talk[0])
		began, ticks := time.Now(), 0
		for ; ticks < 400 && s.within("", talk[1]) != "100"; ticks++ {
			s.frame()
		}
		t.Logf("100 messages between two frames through %s: %d frames of the tab, %s", name, ticks, time.Since(began))
		if ticks > 10 {
			t.Errorf("a talk of 100 messages through %s took %d frames of the tab", name, ticks)
		}
	}
}

func TestMessagesBetweenFramesKeepTheirOrder(t *testing.T) {
	s := ported(t)
	s.does("", `for (let i = 0; i < 300; i++) frames[0].postMessage("m" + i, "*");`)
	want := make([]string, 300)
	for i := range want {
		want[i] = "https://site.test|m" + strconv.Itoa(i)
	}
	s.becomes("a", `got.join()`, strings.Join(want, ","))
}

func TestABroadcastChannelReachesTheDocumentsOfItsOriginThatStoreTogether(t *testing.T) {
	const tuned = `<script>window.aired = []; window.news = new BroadcastChannel("news");
		news.onmessage = (e) => aired.push(e.data + "@" + e.origin);</script>`
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="same" src="/kid"></iframe>` +
			`<iframe id="far" src="https://other.test/kid"></iframe>` + tuned),
		"site.test/kid":    html(tuned),
		"site.test/second": html(`<iframe id="shut" sandbox="allow-scripts" src="/kid"></iframe>` + tuned),
		"other.test/":      html(`<iframe id="theirs" src="https://site.test/kid"></iframe>` + tuned),
		"other.test/kid":   html(tuned),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "same", "far")
	s.does("", `news.postMessage("top");`)
	s.becomes("same", `aired.join()`, "top@https://site.test")
	s.does("far", `news.postMessage("far");`)
	s.newTab()
	s.visit("https://other.test/")
	s.running("", "theirs")
	s.does("theirs", `news.postMessage("site.test under other.test");`)
	s.does("", `news.postMessage("other.test");`)
	s.newTab()
	s.visit("https://site.test/second")
	s.running("", "shut")
	s.does("", `news.postMessage("second");`)
	s.does("shut", `news.postMessage("a document with no origin");`)
	s.newWindow("https://site.test/kid")
	s.until("the second window opened", func() bool { return len(s.screens) == 2 })
	other := s.window(1)
	other.until("the second window's page ran", func() bool { return other.session().scripts != nil })
	other.does("", `news.postMessage("window");`)
	s.becomes("", `aired.join()`, "window@https://site.test")
	for range 10 {
		s.frame()
	}
	if got := s.within("shut", `aired.join()`) + "|" + other.within("", `aired.join()`); got != "|" {
		t.Errorf("a document with no origin, and a window opened since, heard %q", got)
	}
	s.selectTab(1)
	for range 10 {
		s.frame()
	}
	if got := s.within("", `aired.join()`) + "|" + s.within("theirs", `aired.join()`); got != "|" {
		t.Errorf("the documents under another site heard %q", got)
	}
	s.selectTab(0)
	s.becomes("", `aired.join()`, "second@https://site.test,window@https://site.test")
	s.becomes("same", `aired.join()`, "top@https://site.test,second@https://site.test,window@https://site.test")
	if got := s.within("far", `aired.join()`); got != "" {
		t.Errorf("a frame of another site heard %q", got)
	}
}

func TestAWindowLoadsAfterItsFramesAndDOMContentLoadedDoesNotWait(t *testing.T) {
	const told = `<script>
		document.addEventListener("DOMContentLoaded", () => top.postMessage(NAME + " content", "*"));
		addEventListener("load", () => top.postMessage(NAME + " load", "*"));</script>`
	named := func(name string) string { return strings.ReplaceAll(told, "NAME", `"`+name+`"`) }
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<script>window.order = [];
			addEventListener("message", (e) => order.push(e.data));
			document.addEventListener("DOMContentLoaded", () => order.push("top content"));
			addEventListener("load", () => order.push("top load"));</script>
			<iframe id="a" src="https://other.test/mid"></iframe><iframe id="plain" src="/plain"></iframe>
			<script>document.getElementById("a").addEventListener("load", () => order.push("element load"));</script>`),
		"other.test/mid":  html(`<iframe id="n" src="/leaf"></iframe>` + named("mid")),
		"other.test/leaf": html(named("leaf")),
		"site.test/plain": html(`<p>plain</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.becomes("", `order.includes("top load") && order.length >= 6`, "true")
	order := strings.Split(s.within("", `order.join()`), ",")
	at := func(what string) int {
		for i, o := range order {
			if o == what {
				return i
			}
		}
		t.Fatalf("%q never came: %v", what, order)
		return -1
	}
	if order[0] != "top content" {
		t.Errorf("DOMContentLoaded waited for the frames: %v", order)
	}
	for _, pair := range [][2]string{{"leaf load", "mid load"}, {"mid load", "top load"}, {"mid content", "mid load"},
		{"element load", "top load"}, {"mid load", "element load"}} {
		if at(pair[0]) > at(pair[1]) {
			t.Errorf("%q came after %q: %v", pair[0], pair[1], order)
		}
	}
}

func TestAWindowFocusedTakesTheKeyboard(t *testing.T) {
	const keyed = `<script>window.keys = []; addEventListener("keydown", (e) => keys.push(e.key));</script>`
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<body style="margin:0"><button id="go" style="display:block;height:30px">go</button>
			<iframe id="a" src="https://other.test/kid" width="200" height="60"></iframe>
			<iframe id="b" src="https://third.test/kid" width="200" height="60"></iframe>` + keyed + `<script>
			document.getElementById("go").addEventListener("click", () => frames[0].focus());</script>`),
		"other.test/kid": html(`<p>kid</p>` + keyed),
		"third.test/kid": html(`<p>kid</p>` + keyed + `<script>setTimeout(() => { focus(); parent.focus(); }, 0);</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "a", "b")
	s.clickAt(s.inPage("go"))
	s.in.AddText('x')
	s.frame()
	s.frame()
	if got := s.within("a", `keys.join()`) + "|" + s.within("", `keys.join()`) + "|" + s.within("b", `keys.join()`); got !=
		"x||" {
		t.Errorf("after the page focused its frame on a click the keys went to %s", got)
	}
	if !s.session().pages[s.browser.Tab().Current()].Doc.Focused(s.frameNamed("a").node) {
		t.Errorf("the iframe of the window focused does not hold the focus of its document")
	}
	s.does("a", `parent.focus();`)
	s.in.AddText('y')
	s.frame()
	s.frame()
	if got := s.within("a", `keys.join()`) + "|" + s.within("", `keys.join()`); got != "x|y" {
		t.Errorf("after the frame that held the keyboard gave it to the page the keys went to %s", got)
	}
	s.does("b", `focus(); parent.frames[0].focus();`)
	s.in.AddText('z')
	s.frame()
	s.frame()
	if got := s.within("a", `keys.join()`) + "|" + s.within("", `keys.join()`) + "|" + s.within("b", `keys.join()`); got !=
		"x|y,z|" {
		t.Errorf("a frame that held no keyboard took it: %s", got)
	}
}

func TestAWindowCountsItsFramesAsTheyComeAndGo(t *testing.T) {
	s := ported(t)
	if got := s.within("", `length + " " + (frames[2] === window[2]) + " " + typeof window.late`); got !=
		"3 true undefined" {
		t.Fatalf("the page's window reads %s", got)
	}
	s.does("", `const late = document.createElement("iframe");
		late.name = "late"; late.id = "late"; late.src = "https://other.test/kid";
		document.body.insertBefore(late, document.getElementById("b"));`)
	s.running("late")
	if got := s.within("", `length + " " + (window.late === frames[1]) + " " + (late.parent === window)`); got !=
		"4 true true" {
		t.Errorf("with a frame more the page's window reads %s", got)
	}
	if got := s.within("late", `parent.length + " " + (parent.frames[1] === window) + " " +
		parent.frames[1].closed`); got != "4 true false" {
		t.Errorf("the new frame reads its parent as %s", got)
	}
	s.does("", `window.kept = window.late; document.getElementById("late").remove();
		document.getElementById("a").remove();`)
	for range 5 {
		s.frame()
	}
	if got := s.within("", `length + " " + typeof window.late + " " + kept.closed + " " + (frames[0] === frames[0]) + " " +
		typeof frames[2]`); got != "2 undefined true true undefined" {
		t.Errorf("with two frames fewer the page's window reads %s", got)
	}
	if got := s.within("b", `parent.length + " " + (parent.frames[0] === window)`); got != "2 true" {
		t.Errorf("the frame that is now the first reads its parent as %s", got)
	}
}
