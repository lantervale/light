//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"strings"
	"testing"
)

// logged is what the page pushed to its log once it said it was done.
func (s *scripted) logged() string {
	s.t.Helper()
	s.frames("the page to be done", func() bool { return s.eval(`String(window.done === true)`) == "true" })
	return s.eval(`log.join(" ")`)
}

// later is a script that says the page is done once what was posted
// had the time to arrive.
const later = `setTimeout(() => { done = true; }, 30);`

// thrown is a script that names what a function throws, with a ! after
// the name of a DOMException.
const thrown = `const thrown = (fn) => {
	try { fn(); return "none"; } catch (error) { return error.name + (error instanceof DOMException ? "!" : ""); }
};`

func TestAMessageArrivesAfterTheScriptThatPostedIt(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const c = new MessageChannel();
		c.port1.onmessage = (e) => {
			log.push("got:" + e.data);
			if (e.data === "one") { c.port2.postMessage("three"); log.push("handler-ends"); }
			if (e.data === "three") done = true;
		};
		c.port2.postMessage("one");
		c.port2.postMessage("two");
		Promise.resolve().then(() => log.push("microtask"));
		log.push("posted");
	</script>`)
	if got := s.eval(`log.join(" ")`); got != "posted microtask" {
		t.Fatalf("before a frame the page logged %q", got)
	}
	if got, want := s.logged(), "posted microtask got:one handler-ends got:two got:three"; got != want {
		t.Errorf("the page logged %q, want %q", got, want)
	}
	s.quiet()
}

func TestPortsAnswerEachOther(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const { port1, port2 } = new MessageChannel();
		port1.onmessage = (e) => {
			log.push("1:" + e.data);
			if (e.data < 4) port1.postMessage(e.data + 1);
			else done = true;
		};
		port2.onmessage = (e) => { log.push("2:" + e.data); port2.postMessage(e.data + 1); };
		port1.postMessage(0);
	</script>`)
	if got, want := s.logged(), "2:0 1:1 2:2 1:3 2:4 1:5"; got != want {
		t.Errorf("the ports said %q, want %q", got, want)
	}
	s.quiet()
}

func TestAPortWithListenersAloneWaitsForStart(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const c = new MessageChannel();
		c.port1.addEventListener("message", (e) => { log.push(e.data); if (e.data === "c") done = true; });
		c.port2.postMessage("a");
		c.port2.postMessage("b");
		setTimeout(() => { log.push("start"); c.port1.start(); c.port1.start(); c.port2.postMessage("c"); }, 20);
	</script>`)
	if left := s.rt.Frame(); !left {
		t.Error("a timer waits, yet the frame says nothing is left to run")
	}
	if got, want := s.logged(), "start a b c"; got != want {
		t.Errorf("the port heard %q, want %q", got, want)
	}
	s.quiet()
}

func TestAPortNeverStartedLeavesNothingToRun(t *testing.T) {
	s := run(t, `<script>
		const c = new MessageChannel();
		for (let i = 0; i < 100; i++) c.port2.postMessage(i);
	</script>`)
	if s.rt.Frame() {
		t.Error("messages no port was started for keep the page's frames running")
	}
}

func TestAClosedPortHearsAndSaysNothing(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const hear = (name, port) => { port.onmessage = (e) => log.push(name + ":" + e.data); };
		const a = new MessageChannel(), b = new MessageChannel(), c = new MessageChannel(), d = new MessageChannel();
		hear("a1", a.port1); a.port1.close(); a.port2.postMessage("to-closed");
		hear("b1", b.port1); b.port2.postMessage("before-receiver-closed"); b.port1.close();
		hear("c1", c.port1); c.port2.postMessage("before-sender-closed"); c.port2.close(); c.port2.postMessage("after");
		hear("d1", d.port1); hear("d2", d.port2); d.port1.close(); d.port1.postMessage("from-closed"); d.port1.close();
		d.port2.onclose = () => log.push("d2-closed");
		d.port1.onclose = () => log.push("d1-closed");
		`+later+`
	</script>`)
	if got, want := s.logged(), "c1:before-sender-closed d2-closed"; got != want {
		t.Errorf("the ports heard %q, want %q", got, want)
	}
	s.quiet()
}

func TestAPortTravelsThroughAPort(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		`+thrown+`
		const a = new MessageChannel(), b = new MessageChannel();
		const sent = b.port2;
		a.port1.onmessage = (e) => {
			if (e.data === "options") return;
			const port = e.ports[0];
			log.push(e.ports.length, port instanceof MessagePort, port !== sent, e.data.port === port);
			log.push(Object.isFrozen(e.ports));
			port.onmessage = (m) => {
				log.push(m.data);
				if (m.data === "late") port.postMessage("back");
			};
		};
		b.port1.onmessage = (e) => { log.push(e.data); done = true; };
		b.port1.postMessage("early");
		a.port2.postMessage({ port: sent }, [sent]);
		b.port1.postMessage("late");
		sent.postMessage("from-the-port-that-left");
		var again = thrown(() => a.port2.postMessage(null, [sent]));
		var unlisted = thrown(() => a.port2.postMessage({ port: b.port1 }));
		var itself = thrown(() => a.port2.postMessage(null, [a.port2]));
		var twice = thrown(() => a.port2.postMessage(null, [b.port1, b.port1]));
		var plain = thrown(() => a.port2.postMessage(null, [{}]));
		var options = thrown(() => a.port2.postMessage("options", { transfer: [] }));
	</script>`)
	if got, want := s.logged(), "1 true true true true early late back"; got != want {
		t.Errorf("the ports said %q, want %q", got, want)
	}
	got := s.eval(`[again, unlisted, itself, twice, plain, options].join(" ")`)
	if want := "DataCloneError! DataCloneError! DataCloneError! DataCloneError! DataCloneError! none"; got != want {
		t.Errorf("the transfers threw %q, want %q", got, want)
	}
	s.quiet()
}

func TestBothPortsOfAChannelTravelTogether(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const a = new MessageChannel(), b = new MessageChannel();
		a.port1.onmessage = (e) => {
			const [one, two] = e.ports;
			two.onmessage = (m) => { log.push(m.data); done = true; };
			one.postMessage("still-entangled");
		};
		a.port2.postMessage(null, [b.port1, b.port2]);
		var doomed = new MessageChannel();
		doomed.port1.onmessage = () => log.push("doomed");
		doomed.port2.postMessage("to my own port", [doomed.port1]);
	</script>`)
	if got, want := s.logged(), "still-entangled"; got != want {
		t.Errorf("the ports said %q, want %q", got, want)
	}
	s.quiet()
}

func TestAMessageIsACopy(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const c = new MessageChannel();
		const shared = { n: 1 };
		const sent = {
			text: "hi", list: [1, , 3], shared, again: shared, when: new Date(5), re: /a+/gi,
			map: new Map([["k", shared]]), set: new Set([1, "two"]), bytes: new Uint8Array([1, 2, 3]).subarray(1),
			error: new RangeError("far"), big: 10n, nothing: undefined, none: null, wrapped: new String("w"),
		};
		sent.self = sent;
		c.port1.onmessage = ({ data }) => {
			log.push(data !== sent, data.text, data.self === data, data.again === data.shared, data.shared !== shared);
			log.push(data.shared.n, data.list.length, 1 in data.list, data.when instanceof Date, data.when.getTime());
			log.push(data.re.source + "/" + data.re.flags, data.map.get("k") === data.shared, data.set.has("two"));
			log.push(data.bytes instanceof Uint8Array, Array.from(data.bytes).join(), data.bytes.byteOffset);
			log.push(data.bytes.buffer.byteLength, data.error instanceof RangeError, data.error.message);
			log.push(data.big === 10n, "nothing" in data, data.none, data.wrapped instanceof String, String(data.wrapped));
			done = true;
		};
		c.port2.postMessage(sent);
		shared.n = 2;
		sent.text = "changed";
	</script>`)
	want := "true hi true true true 1 3 false true 5 a+/gi true true true 2,3 1 3 true far true true  true w"
	if got := s.logged(); got != want {
		t.Errorf("the copy says\n%q, want\n%q", got, want)
	}
	s.quiet()
}

func TestStructuredCloneCopiesWhatItCanAndThrowsForTheRest(t *testing.T) {
	s := run(t, `<p id="p"></p><script>
		`+thrown+`
		class Point { constructor() { this.x = 1; } get length() { return 2; } }
	</script>`)
	for _, c := range []struct{ expression, want string }{
		{`thrown(() => structuredClone(() => 1))`, "DataCloneError!"},
		{`thrown(() => structuredClone({ deep: [function named() {}] }))`, "DataCloneError!"},
		{`thrown(() => structuredClone(Symbol("s")))`, "DataCloneError!"},
		{`thrown(() => structuredClone(document.getElementById("p")))`, "DataCloneError!"},
		{`thrown(() => structuredClone(window))`, "DataCloneError!"},
		{`thrown(() => structuredClone({ at: location }))`, "DataCloneError!"},
		{`thrown(() => structuredClone(new MessageChannel().port1))`, "DataCloneError!"},
		{`thrown(() => structuredClone(new WeakMap()))`, "DataCloneError!"},
		{`thrown(() => structuredClone(Promise.resolve()))`, "DataCloneError!"},
		{`thrown(() => structuredClone())`, "TypeError"},
		{`thrown(() => structuredClone({ get x() { throw new RangeError("mine"); } }))`, "RangeError"},
		{`[1, "s", null, undefined, true].map((value) => structuredClone(value)).join()`, "1,s,,,true"},
		{`Object.is(structuredClone(-0), -0) + " " + structuredClone(NaN)`, "true NaN"},
		{`(() => { const p = structuredClone(new Point());
			return Object.getPrototypeOf(p) === Object.prototype && p.x === 1 && p.length === undefined; })()`, "true"},
		{`(() => { const a = [1]; a.self = a; a.name = "n"; const b = structuredClone(a);
			return Array.isArray(b) && b.self === b && b.name === "n" && b !== a; })()`, "true"},
		{`(() => { const e = structuredClone(new TypeError("bad", { cause: { why: 1 } }));
			return e instanceof TypeError && e.name + e.message + e.cause.why; })()`, "TypeErrorbad1"},
		{`(() => { const e = new Error("odd"); e.name = "Custom"; return structuredClone(e).name; })()`, "Error"},
		{`(() => { const e = new Error("odd"); e.name = "toString"; return structuredClone(e) instanceof Error; })()`,
			"true"},
		{`(() => { const e = structuredClone(new DOMException("gone", "NotFoundError"));
			return e instanceof DOMException && e.name + e.message; })()`, "NotFoundErrorgone"},
		{`(() => { const b = new Uint8Array([7, 8]).buffer;
			const v = structuredClone({ a: new DataView(b, 1), b: new Int8Array(b) });
			return v.a.buffer === v.b.buffer && v.a.buffer !== b && v.a.getUint8(0); })()`, "8"},
		{`(() => { const m = new Map(); m.set(m, new Set([m])); const c = structuredClone(m);
			return c !== m && c.get(c).has(c); })()`, "true"},
		{`(() => { const r = /a/g; r.lastIndex = 3; return structuredClone(r).lastIndex; })()`, "0"},
		{`(() => { const b = new Uint8Array([1, 2, 3]).buffer, c = structuredClone({ b }, { transfer: [b] });
			return [b.byteLength, c.b.byteLength, new Uint8Array(c.b).join("")].join(); })()`, "0,3,123"},
		{`(() => { const b = new ArrayBuffer(2); structuredClone(b, { transfer: [b] });
			return thrown(() => structuredClone(b)) + " " + thrown(() => structuredClone(null, { transfer: [b] })); })()`,
			"DataCloneError! DataCloneError!"},
		{`(() => { const b = new ArrayBuffer(2);
			return thrown(() => structuredClone({ b, f() {} }, { transfer: [b] })) + " " + b.byteLength; })()`,
			"DataCloneError! 2"},
		{`(() => { const { port1 } = new MessageChannel(), moved = structuredClone(port1, { transfer: [port1] });
			return moved instanceof MessagePort && moved !== port1; })()`, "true"},
		{`(() => { const c = structuredClone(new Blob(["text"], { type: "text/plain" }));
			return c instanceof Blob && c.size + c.type; })()`, "4text/plain"},
	} {
		if got := s.eval(c.expression); got != c.want {
			t.Errorf("%s is %q, want %q", c.expression, got, c.want)
		}
	}
	s.quiet()
}

func TestWhatAPortCannotCloneIsThrownToTheSender(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const c = new MessageChannel();
		c.port1.onmessage = (e) => log.push(e.data);
		try { c.port2.postMessage({ fn() {} }); } catch (error) { log.push(error.name, error instanceof DOMException); }
		c.port2.postMessage("next");
		`+later+`
	</script>`)
	if got, want := s.logged(), "DataCloneError true next"; got != want {
		t.Errorf("the page logged %q, want %q", got, want)
	}
	s.quiet()
}

func TestAWindowPostsToItself(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		`+thrown+`
		addEventListener("message", (e) => {
			const from = [e.origin, e.source === window, e.lastEventId, e.ports.length, e.isTrusted];
			log.push(e.data + "|" + from.join("|"));
		});
		postMessage("bare");
		postMessage("star", "*");
		postMessage("slash", "/");
		postMessage("origin", "https://site.test");
		postMessage("address", "https://site.test/else/where?x#y");
		postMessage("other", "https://other.test");
		postMessage("scheme", "http://site.test");
		postMessage("port", "https://site.test:8443");
		postMessage("options", { targetOrigin: "https://site.test" });
		postMessage("empty-options", {});
		postMessage("other-options", { targetOrigin: "https://other.test" });
		log.push(thrown(() => postMessage("bad", "nowhere")), thrown(() => postMessage(() => 1, "*")));
		log.push(thrown(() => postMessage()));
		const sent = { n: 1 };
		window.postMessage(sent, "*");
		sent.n = 2;
		log.push("posted");
		`+later+`
	</script>`)
	const from = "|https://site.test|true||0|true"
	want := "SyntaxError! DataCloneError! TypeError posted " + strings.Join([]string{
		"bare", "star", "slash", "origin", "address", "options", "empty-options", "[object Object]",
	}, from+" ") + from
	if got := s.logged(); got != want {
		t.Errorf("the window heard\n%q, want\n%q", got, want)
	}
	s.quiet()
}

func TestAWindowIsSentAPort(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const c = new MessageChannel();
		onmessage = (e) => {
			log.push(e.data.n, e.ports.length, e.ports[0] === e.data.port);
			e.ports[0].postMessage("through");
		};
		c.port1.onmessage = (e) => { log.push(e.data, e.origin === "", e.source, e.ports.length); done = true; };
		postMessage({ n: 1, port: c.port2 }, "*", [c.port2]);
		const d = new MessageChannel();
		d.port1.onmessage = () => log.push("lost");
		postMessage(null, { targetOrigin: "https://other.test", transfer: [d.port2] });
		d.port2.postMessage("from a port that left");
	</script>`)
	if got, want := s.logged(), "1 1 true through true  0"; got != want {
		t.Errorf("the page logged %q, want %q", got, want)
	}
	s.quiet()
}

func TestAWindowIsItsOwnParent(t *testing.T) {
	s := run(t, `<p>a page</p>`)
	got := s.eval(`[parent === window, top === window, self === window, frames === window, length].join()`)
	if want := "true,true,true,true,0"; got != want {
		t.Errorf("the window says %q, want %q", got, want)
	}
	kinds := s.eval(`[typeof Worker, typeof SharedWorker, typeof MessageChannel, typeof BroadcastChannel].join()`)
	if want := "undefined,undefined,function,function"; kinds != want {
		t.Errorf("the window has %q, want %q", kinds, want)
	}
}

func TestAMessageEventSaysWhatItWasMadeWith(t *testing.T) {
	s := run(t, `<script>
		`+thrown+`
		var { port1 } = new MessageChannel();
	</script>`)
	for _, c := range []struct{ expression, want string }{
		{`(() => { const e = new MessageEvent("message");
			return [e.type, e.data, e.origin, e.lastEventId, e.source, e.ports.length, e.bubbles, e.cancelable].join(); })()`,
			"message,,,,,0,false,false"},
		{`(() => { const init = { data: { a: 1 }, origin: "https://o.test", lastEventId: "7", source: window };
			const e = new MessageEvent("x", { ...init, ports: [port1], bubbles: true });
			return [e.data.a, e.origin, e.lastEventId, e.source === window, e.ports[0] === port1, Object.isFrozen(e.ports),
				e.bubbles, e instanceof Event].join(); })()`,
			"1,https://o.test,7,true,true,true,true,true"},
		{`(() => { const e = new MessageEvent("x"); e.initMessageEvent("y", true, true, "d", "o", "3", window, [port1]);
			return [e.type, e.bubbles, e.cancelable, e.data, e.origin, e.lastEventId, e.source === window,
				e.ports.length].join(); })()`,
			"y,true,true,d,o,3,true,1"},
		{`thrown(() => new MessagePort())`, "TypeError"},
		{`thrown(() => MessagePort.prototype.postMessage.call({}, 1))`, "TypeError"},
		{`thrown(() => port1.postMessage())`, "TypeError"},
		{`(() => { const c = new MessageChannel();
			return [c.port1 instanceof MessagePort, c.port1 instanceof EventTarget, c.port1 === c.port1,
				c.port1 !== c.port2, c.port1.onmessage, c.port1.onmessageerror, c.port1.onclose].join(); })()`,
			"true,true,true,true,,,"},
		{`(() => { const c = new MessageChannel(), f = () => {}; c.port1.onmessage = f; return c.port1.onmessage === f; })()`,
			"true"},
	} {
		if got := s.eval(c.expression); got != c.want {
			t.Errorf("%s is %q, want %q", c.expression, got, c.want)
		}
	}
	s.quiet()
}

func TestAHandlerThatThrowsDoesNotStopTheNextMessage(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false;
		const c = new MessageChannel();
		c.port1.onmessage = (e) => {
			log.push(e.data);
			if (e.data === "one") throw new RangeError("in a port's handler");
			done = true;
		};
		c.port1.addEventListener("message", (e) => log.push("also:" + e.data));
		addEventListener("message", (e) => { log.push("window:" + e.data); throw new TypeError("in the window's handler"); });
		c.port2.postMessage("one");
		postMessage("mine", "*");
		c.port2.postMessage("two");
	</script>`)
	if got, want := s.logged(), "one also:one window:mine two also:two"; got != want {
		t.Errorf("the page logged %q, want %q", got, want)
	}
	for _, text := range []string{
		"Uncaught RangeError: in a port's handler", "Uncaught TypeError: in the window's handler",
	} {
		if m := s.line(text); m.Level != "error" || m.Source != address {
			t.Errorf("the console tells of %q as %s from %q", text, m.Level, m.Source)
		}
	}
}

func TestABroadcastReachesTheOtherChannelsOfItsName(t *testing.T) {
	s := run(t, `<script>
		var log = [], done = false, heard;
		`+thrown+`
		const hear = (name, channel) => {
			channel.onmessage = (e) => {
				log.push([name + ":" + e.data.n, e.origin, e.source === null, e.ports.length].join("|"));
			};
			return channel;
		};
		const sender = hear("sender", new BroadcastChannel("news"));
		const first = hear("first", new BroadcastChannel("news"));
		const second = new BroadcastChannel("news");
		second.addEventListener("message", (e) => log.push("second:" + e.data.n + (e.data !== sent) + (e.data !== heard)));
		first.addEventListener("message", (e) => { heard = e.data; });
		hear("other", new BroadcastChannel("weather"));
		const closed = hear("closed", new BroadcastChannel("news"));
		const closing = hear("closing", new BroadcastChannel("news"));
		closed.close();
		const sent = { n: 1 };
		sender.postMessage(sent);
		sent.n = 2;
		closing.close();
		log.push(sender.name, thrown(() => closed.postMessage(1)), thrown(() => sender.postMessage(() => 1)));
		log.push(thrown(() => new BroadcastChannel()));
		`+later+`
	</script>`)
	want := "news InvalidStateError! DataCloneError! TypeError first:1|https://site.test|true|0 second:1truetrue"
	if got := s.logged(); got != want {
		t.Errorf("the channels heard\n%q, want\n%q", got, want)
	}
	s.quiet()
}

// The cases of the web platform tests' webmessaging/message-channels and
// webmessaging/MessagePort_*, written again by hand for light: each
// ends by calling pass or fail.
func TestChannelsAsTheWebPlatformTestsAskOfThem(t *testing.T) {
	for _, c := range []struct{ name, script string }{
		{"basics: a message crosses the channel", `
			const channel = new MessageChannel();
			channel.port1.postMessage(1);
			channel.port2.onmessage = (e) => (e.data === 1 ? pass() : fail(e.data));`},
		{"basics: a message crosses back", `
			const channel = new MessageChannel();
			channel.port2.postMessage(1);
			channel.port1.onmessage = (e) => (e.data === 1 ? pass() : fail(e.data));`},
		{"basics: messages sent before the handler is set are kept", `
			const channel = new MessageChannel();
			channel.port1.postMessage(1);
			channel.port1.postMessage(2);
			setTimeout(() => {
				const got = [];
				channel.port2.onmessage = (e) => {
					got.push(e.data);
					if (got.length === 2) (got.join() === "1,2" ? pass() : fail(got));
				};
			}, 10);`},
		{"close: a closed port may be closed again and started", `
			const channel = new MessageChannel();
			channel.port1.close(); channel.port1.close(); channel.port1.start();
			channel.port1.postMessage("unheard");
			pass();`},
		{"close: a message sent to a closed port does not arrive", `
			const channel = new MessageChannel();
			channel.port1.onmessage = () => fail("arrived");
			channel.port1.close();
			channel.port2.postMessage("TEST");
			setTimeout(pass, 30);`},
		{"close: a message sent from a closed port does not arrive", `
			const channel = new MessageChannel();
			channel.port1.onmessage = () => fail("arrived");
			channel.port2.close();
			channel.port2.postMessage("TEST");
			setTimeout(pass, 30);`},
		{"close: a message sent to a closed port from a transferred port does not arrive", `
			const channel = new MessageChannel(), carrier = new MessageChannel();
			channel.port1.onmessage = () => fail("arrived");
			channel.port1.close();
			carrier.port1.onmessage = (e) => { e.ports[0].postMessage("TEST"); setTimeout(pass, 30); };
			carrier.port2.postMessage(null, [channel.port2]);`},
		{"close: a closed port cannot be transferred", `
			const channel = new MessageChannel(), carrier = new MessageChannel();
			channel.port1.close();
			try { carrier.port1.postMessage(null, [channel.port1]); fail("no throw"); }
			catch (error) { error.name === "DataCloneError" ? pass() : fail(error.name); }`},
		{"MessagePort_initial_disabled: a listener alone hears nothing", `
			const channel = new MessageChannel();
			channel.port2.addEventListener("message", () => fail("arrived"), true);
			channel.port1.postMessage("ping");
			setTimeout(pass, 30);`},
		{"MessagePort_onmessage_start: the handler starts the port", `
			const channel = new MessageChannel();
			channel.port2.onmessage = (e) => (e.data === "ping" ? pass() : fail(e.data));
			channel.port1.postMessage("ping");`},
		{"Channel_postMessage_clone_port: the port sent is another object", `
			const channel = new MessageChannel(), other = new MessageChannel();
			channel.port2.onmessage = (e) => {
				const port = e.ports[0];
				const fresh = port instanceof MessagePort && port !== other.port1;
				fresh && e.data === "ports" && e.target === channel.port2 ? pass() : fail("not a new port");
			};
			channel.port1.postMessage("ports", [other.port1]);`},
		{"Channel_postMessage_event_properties: the event says little", `
			const channel = new MessageChannel();
			channel.port2.onmessage = (e) => {
				const plain = e instanceof MessageEvent && e.type === "message" && !e.bubbles && !e.cancelable;
				const empty = e.origin === "" && e.lastEventId === "" && e.source === null && e.ports.length === 0;
				plain && empty && e.target === channel.port2 ? pass() : fail("fields");
			};
			channel.port1.postMessage(1);`},
		{"Channel_postMessage_target_source: a port does not hear itself", `
			const channel = new MessageChannel();
			channel.port1.onmessage = () => fail("the sender heard itself");
			channel.port2.onmessage = () => setTimeout(pass, 20);
			channel.port1.postMessage("ping");`},
		{"Channel_postMessage_transfer_xsite_incoming_messages: what waited in a port travels with it", `
			const channel = new MessageChannel(), carrier = new MessageChannel();
			channel.port1.postMessage("first");
			channel.port1.postMessage("second");
			carrier.port1.onmessage = (e) => {
				const got = [];
				e.ports[0].onmessage = (m) => {
					got.push(m.data);
					if (got.length === 3) (got.join() === "first,second,third" ? pass() : fail(got));
				};
				channel.port1.postMessage("third");
			};
			carrier.port2.postMessage(null, [channel.port2]);`},
		{"postMessage_MessagePorts_sorigin: a window gets the ports in their order", `
			const a = new MessageChannel(), b = new MessageChannel();
			onmessage = (e) => {
				e.ports.length === 2 && e.ports[0] !== e.ports[1] && e.data === "ports" ? pass() : fail(e.ports.length);
			};
			postMessage("ports", "*", [a.port1, b.port1]);`},
		{"postMessage_dup_transfer_objects: a port listed twice is refused", `
			const channel = new MessageChannel();
			try { postMessage("", "*", [channel.port1, channel.port1]); fail("no throw"); }
			catch (error) { error.name === "DataCloneError" ? pass() : fail(error.name); }`},
		{"postMessage_invalid_targetOrigin: an origin that is no address is refused", `
			try { postMessage("", "example.com"); fail("no throw"); }
			catch (error) { error instanceof DOMException && error.name === "SyntaxError" ? pass() : fail(error.name); }`},
		{"order holds across many messages", `
			const channel = new MessageChannel(), got = [];
			channel.port2.onmessage = (e) => {
				got.push(e.data);
				if (got.length === 50) (got.every((n, i) => n === i) ? pass() : fail(got));
			};
			for (let i = 0; i < 50; i++) channel.port1.postMessage(i);`},
		{"a task each: a timer set between two messages runs between them", `
			const channel = new MessageChannel(), got = [];
			channel.port2.onmessage = (e) => got.push(e.data);
			channel.port1.postMessage(1);
			setTimeout(() => got.push("timer"), 0);
			channel.port1.postMessage(2);
			setTimeout(() => (got.join() === "1,timer,2" ? pass() : fail(got)), 20);`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, `<script>
				var outcome = "";
				const pass = () => { outcome = outcome || "pass"; };
				const fail = (why) => { outcome = "fail: " + why; };
			</script><script>`+c.script+`</script>`)
			s.frames("the case to end", func() bool { return s.eval(`outcome`) != "" })
			for range 3 {
				s.rt.Frame()
			}
			if got := s.eval(`outcome`); got != "pass" {
				t.Errorf("the case ended with %q", got)
			}
			s.quiet()
		})
	}
}

func TestPortsAPageLetGoAreCollected(t *testing.T) {
	s := run(t, `<script>
		var refs = [];
		function churn() {
			for (let i = 0; i < 200; i++) {
				const c = new MessageChannel(), payload = { bytes: new Uint8Array(4096) };
				for (let n = 0; n < 5; n++) c.port1.postMessage(payload);
				c.port2.addEventListener("message", () => {});
				const carried = new MessageChannel();
				c.port1.postMessage(null, [carried.port1]);
				const heard = new BroadcastChannel("churn");
				heard.onmessage = () => {};
				heard.close();
				if (i > 0) continue;
				refs.push(new WeakRef(c.port1), new WeakRef(c.port2), new WeakRef(carried.port2), new WeakRef(heard));
			}
		}
		function alive() { return refs.filter((ref) => ref.deref() !== undefined).length; }
		churn();
	</script>`)
	s.frames("the ports to be collected", func() bool {
		s.eval(`churn(), new Array(1e6).fill(0).length`)
		return s.eval(`String(alive() < refs.length / 2)`) == "true"
	})
	if s.rt.Frame() {
		t.Error("ports no script holds keep the page's frames running")
	}
}
