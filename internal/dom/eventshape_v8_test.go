//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"strings"
	"testing"
)

func TestAnEventShowsWhatItIsThroughItsInterface(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><div id="outer"><p id="p"></p></div>`)
	const tools = `const $ = (id) => document.getElementById(id),
		thrown = (run) => { try { run(); return "none" } catch (e) { return e.name } };`
	for _, c := range []struct{ name, script, want string }{
		{"what an event is cannot be written, and is the interface's", `(() => { const e = new Event("x", { bubbles:
			true }); e.type = "y"; e.target = document; e.bubbles = false; e.isTrusted = true;
			const own = Object.getOwnPropertyDescriptor(e, "isTrusted"); return [e.type, e.target, e.bubbles, e.cancelable,
			e.isTrusted, Object.keys(e).join("+"), "type" in Event.prototype, own.configurable, typeof own.get,
			e.timeStamp > 0, thrown(() => Event.prototype.type), new Event("x", null).bubbles,
			thrown(() => new Event("x", 5)), thrown(() => new Event()), new CustomEvent("c").detail,
			new CustomEvent("c", { detail: 4 }).detail, new UIEvent("u").view].join() })()`,
			"x,,true,false,false,isTrusted,true,false,function,true,TypeError,false,TypeError,TypeError,,4,"},
		{"an event goes down to its target and back, and is clean after", `(() => { const seen = [], e =
			new Event("x", { bubbles: true, cancelable: true }), note = (at) => (ev) => seen.push(at + ev.eventPhase +
			ev.currentTarget.nodeName + ev.composedPath().length + (window.event === ev));
			$("outer").addEventListener("x", note("down"), true); $("p").addEventListener("x", note("at"));
			$("outer").addEventListener("x", (ev) => { note("up")(ev); ev.preventDefault() });
			const stood = $("p").dispatchEvent(e); return seen.concat(stood, e.defaultPrevented, e.returnValue,
			e.eventPhase, e.currentTarget, e.target.id, e.srcElement.id, e.composedPath().length, window.event,
			e.cancelBubble).join() })()`,
			"down1DIV6true,at2P6true,up3DIV6true,false,true,false,0,,p,p,0,,false"},
		{"an event being sent, or not set up, is not sent", `(() => { const made = document.createEvent("Event"), e =
			new Event("x"), seen = [thrown(() => $("p").dispatchEvent(made))]; made.initEvent("x", true);
			$("p").addEventListener("x", (ev) => { seen.push(thrown(() => $("p").dispatchEvent(ev))); ev.initEvent("y");
			seen.push(ev.type) }, { once: true }); seen.push($("p").dispatchEvent(made), made.bubbles, made.cancelable,
			$("p").dispatchEvent(e)); return seen.join() })()`,
			"InvalidStateError,InvalidStateError,x,true,true,false,true"},
		{"the old names of the kinds of event make them, and no other name", `[document.createEvent("HTMLEvents")
			.constructor === Event, document.createEvent("mouseevents") instanceof MouseEvent,
			document.createEvent("CUSTOMEVENT") instanceof CustomEvent, thrown(() => document.createEvent("Events2")),
			thrown(() => document.createEvent("KeyEvents")), thrown(() => document.createEvent())].join()`,
			"true,true,true,NotSupportedError,NotSupportedError,TypeError"},
		{"a passive listener prevents nothing, and stopping is kept", `(() => { const e = new Event("x", { cancelable:
			true, bubbles: true }), seen = []; $("p").addEventListener("x", (ev) => { ev.preventDefault();
			seen.push(ev.defaultPrevented); ev.cancelBubble = true }, { passive: true });
			$("outer").addEventListener("x", () => seen.push("reached")); $("p").dispatchEvent(e);
			e.returnValue = false; return seen.concat(e.defaultPrevented, e.cancelBubble).join() })()`,
			"false,true,false"},
		{"nodes of different trees are apart, the same way each time", `(() => { const a = document.createElement("a"),
			b = document.createElement("b"), ab = a.compareDocumentPosition(b), ba = b.compareDocumentPosition(a);
			return [[35, 37].includes(ab), ab + ba, $("p").compareDocumentPosition(a) & 33,
			a.compareDocumentPosition(a)].join() })()`, "true,72,33,0"},
	} {
		if got := s.eval(tools + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestAClickAScriptSendsActsOnWhatItLandsOn(t *testing.T) {
	const page = `<!DOCTYPE html><body><input type="checkbox" id="c"><input type="radio" name="r" id="r1" checked>` +
		`<input type="radio" name="r" id="r2"><form id="f"><button id="b">go</button>` +
		`<input type="submit" id="off" disabled></form><p id="p"></p>`
	const tools = `const $ = (id) => document.getElementById(id), seen = [];`
	for _, c := range []struct{ name, script, want string }{
		{"a checkbox is set before its listeners run, then tells of its input and change", `$("c").onclick = () =>
			seen.push("click:" + $("c").checked); $("c").oninput = () => seen.push("input"); $("c").onchange = (e) =>
			seen.push("change:" + e.bubbles); $("c").click(); seen.join()`, "click:true,input,change:true"},
		{"a cancelled click sets it back", `$("c").onclick = (e) => { seen.push($("c").checked); e.preventDefault() };
			$("c").onchange = () => seen.push("change"); $("c").click(); seen.concat($("c").checked).join()`, "true,false"},
		{"a radio takes the place of the one of its group, and gives it back", `$("r2").click(); seen.push($("r1").checked,
			$("r2").checked); $("r1").onclick = (e) => e.preventDefault(); $("r1").click();
			seen.concat($("r1").checked, $("r2").checked).join()`, "false,true,false,true"},
		{"one that is disabled is set by a click sent to it, and click() does nothing", `$("c").disabled = true;
			$("c").onclick = () => seen.push("click"); $("c").click(); seen.push($("c").checked);
			$("c").dispatchEvent(new MouseEvent("click")); seen.concat($("c").checked).join()`, "false,click,true"},
		{"one that became another kind while its click was cancelled is set back all the same", `$("c").onclick = (e) =>
			{ $("c").type = "date"; e.preventDefault() }; $("c").dispatchEvent(new MouseEvent("click", { cancelable: true }));
			$("r2").onclick = (e) => { $("r2").type = "date"; e.preventDefault() };
			$("r2").dispatchEvent(new MouseEvent("click", { cancelable: true }));
			[$("c").checked, $("r1").checked, $("r2").checked].join()`, "false,true,false"},
		{"one out of the page is set and tells nothing", `(() => { const c = document.createElement("input");
			c.type = "checkbox"; c.onchange = () => seen.push("change"); c.click(); return seen.concat(c.checked).join() })()`,
			"true"},
		{"a submit button sends its form, a disabled one does not", `$("f").onsubmit = (e) => { seen.push("submit:" +
			e.submitter.id); e.preventDefault() }; $("b").click(); $("off").dispatchEvent(new MouseEvent("click"));
			seen.join()`, "submit:b"},
		{"a click that bubbles acts on the nearest ancestor that does", `(() => { const s = document.createElement("span");
			$("b").append(s); $("f").onsubmit = (e) => { seen.push("submit"); e.preventDefault() };
			s.dispatchEvent(new MouseEvent("click")); s.dispatchEvent(new MouseEvent("click", { bubbles: true }));
			return seen.join() })()`, "submit"},
		{"at its target an event meets the capturing listeners first", `$("p").addEventListener("x", () =>
			seen.push("bubble")); $("p").addEventListener("x", (e) => seen.push("capture:" + e.eventPhase), true);
			$("p").dispatchEvent(new Event("x")); $("p").addEventListener("y", (e) => e.stopPropagation(), true);
			$("p").addEventListener("y", () => seen.push("late")); $("p").dispatchEvent(new Event("y")); seen.join()`,
			"capture:2,bubble"},
		{"a listener of the wheel on the window is passive unless it says not", `window.addEventListener("wheel", (e) =>
			{ e.preventDefault(); seen.push(e.defaultPrevented) }); $("p").addEventListener("wheel", (e) => {
			e.preventDefault(); seen.push(e.defaultPrevented) }); const e = new Event("wheel", { cancelable: true,
			bubbles: true }); $("p").dispatchEvent(e); window.addEventListener("touchstart", (e) => { e.preventDefault();
			seen.push(e.defaultPrevented) }, { passive: false }); window.dispatchEvent(new Event("touchstart",
			{ cancelable: true })); window.dispatchEvent(new Event("wheel", { cancelable: true })); seen.join()`,
			"true,true,true,false"},
		{"a listener's options are read, and its handleEvent each time", `(() => { const read = [];
			$("p").addEventListener("x", null, { get capture() { read.push("capture"); return false },
			get passive() { read.push("passive"); return false }, get once() { read.push("once"); return false } });
			const o = { handleEvent() { read.push("first") } }; $("p").addEventListener("z", o);
			$("p").dispatchEvent(new Event("z")); o.handleEvent = () => read.push("second");
			$("p").dispatchEvent(new Event("z")); try { $("p").addEventListener("z", 5) } catch (e) { read.push(e.name) }
			return read.join() })()`, "capture,once,passive,first,second,TypeError"},
	} {
		s := run(t, page)
		if got := s.eval(tools + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
		s.quiet()
	}
}

func TestABodyGivesTheWindowItsHandlers(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body onresize="window.resized = this === window || this === document.body">`)
	for _, c := range []struct{ name, script, want string }{
		{"the attribute the page wrote is the window's handler", `[typeof window.onresize,
			window.onresize === document.body.onresize, (window.dispatchEvent(new Event("resize")), window.resized)].join()`,
			"function,true,true"},
		{"one set on any body or frameset is the window's, and one of another event is its own", `(() => {
			const b = document.createElement("body"), f = document.createElement("frameset"), nop = () => {};
			b.setAttribute("onblur", "return"); const written = window.onblur, same = written === b.onblur;
			f.onblur = nop; b.onclick = nop;
			return [typeof written, same, window.onblur === nop, b.onblur === nop, window.onclick,
			(b.removeAttribute("onblur"), window.onblur)].join() })()`, "function,true,true,true,,"},
		{"events of the keyboard, the wheel and writing carry what they are made with", `[new WheelEvent("wheel",
			{ deltaY: 3, deltaMode: 1 }).deltaY, new WheelEvent("wheel").deltaMode, new CompositionEvent("c").data,
			new KeyboardEvent("k", { location: 2 }).location, (() => { try { new UIEvent("x", { view: 7 }) } catch (e) {
			return e.name } })(), new UIEvent("x", { view: window }).view === window, (() => { const e =
			document.createEvent("MouseEvent"); e.initMouseEvent("click", true, true, window, 2, 1, 2, 3, 4);
			return [e.type, e.detail, e.clientX, e.bubbles] })(), (() => { const e = document.createEvent("KeyboardEvent");
			e.initKeyboardEvent("keyup", false, false, null, "a", 1); return e.key + e.location })()].join()`,
			"3,0,,2,TypeError,true,click,2,3,true,a1"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestAnExceptionListsTheCodesItCanCarry(t *testing.T) {
	s := run(t, `<p id="p"></p>`)
	got := s.eval(`(() => { let e; try { document.appendChild(document) } catch (thrown) { e = thrown }
		const named = []; for (const name in e) if (/_ERR$/.test(name) && e[name] === e.code) named.push(name);
		const held = Object.getOwnPropertyDescriptor(DOMException, "NOT_FOUND_ERR");
		return [named, e.code, held.enumerable, held.writable, held.configurable,
			DOMException.prototype.ABORT_ERR].join() })()`)
	if want := "HIERARCHY_REQUEST_ERR,3,true,false,false,20"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	s.quiet()
}

func TestAClickAScriptSendsFollowsALinkButNoScriptedOne(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><a id="a" href="/next">next</a><a id="j" href=" JavaScript:window.ran = 1">run</a>`)
	s.eval(`document.getElementById("j").dispatchEvent(new MouseEvent("click", { bubbles: true }));
		document.getElementById("j").click()`)
	if len(s.went) != 0 || s.eval(`String(window.ran)`) != "undefined" {
		t.Errorf("a javascript: link went to %v and ran %s", s.went, s.eval(`String(window.ran)`))
	}
	s.eval(`document.getElementById("a").dispatchEvent(new MouseEvent("click"))`)
	if len(s.went) != 1 || !strings.HasSuffix(s.went[0], "/next") {
		t.Errorf("the link went to %v, want /next", s.went)
	}
	s.quiet()
}
