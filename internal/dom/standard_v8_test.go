//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

func TestTheSmallRulesOfTheDOMStandard(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="a">a</p><p id="b">b</p><p id="c">c</p><applet name="war"></applet>
		<div id="held">t</div><span id="x"></span><span></span>`)
	const thrown = `const thrown = (fn) => { try { fn(); return "none" } catch (e) { return e.name } };`
	for _, c := range []struct{ name, script, want string }{
		{"an event stopped before it is sent goes nowhere, and goes the next time", `(() => { const e = new Event("foo"),
			seen = []; let called = 0; document.head.addEventListener("foo", () => called++); e.stopPropagation();
			seen.push(e.cancelBubble); document.head.dispatchEvent(e); seen.push(called, e.cancelBubble);
			document.head.dispatchEvent(e); seen.push(called); e.cancelBubble = true; document.head.dispatchEvent(e);
			seen.push(called); return seen.join() })()`, "true,0,false,1,1"},
		{"an event set up again forgets what was done to it, and is not while it is sent", thrown + `(() => { const e =
			new Event("foo", { cancelable: true }); e.preventDefault(); e.stopPropagation(); e.initEvent("bar", true, true);
			const seen = [e.type, e.defaultPrevented, e.cancelBubble, thrown(() => e.initEvent())];
			document.head.addEventListener("bar", (ev) => { ev.initEvent("baz", false, false); seen.push(ev.type, ev.bubbles) },
			{ once: true }); document.head.dispatchEvent(e);
			const c = new CustomEvent("x", { detail: 4 }); c.initCustomEvent("y"); seen.push(c.type, c.detail,
			thrown(() => c.initCustomEvent())); return seen.join() })()`, "bar,false,false,TypeError,bar,true,y,,TypeError"},
		{"an observer refuses the old value of what it does not watch", thrown + `(() => { const m =
			new MutationObserver(() => {}), d = document; return [
			thrown(() => m.observe(d, { childList: true, attributeOldValue: true, attributes: false })),
			thrown(() => m.observe(d, { childList: true, attributeFilter: ["a"], attributes: false })),
			thrown(() => m.observe(d, { childList: true, characterDataOldValue: true, characterData: false })),
			thrown(() => m.observe(d, { attributeOldValue: true })), thrown(() => m.observe(d, { attributeFilter: ["a"] })),
			thrown(() => m.observe(d, { characterDataOldValue: true })), thrown(() => m.observe(d, {})),
			thrown(() => m.observe(d, { childList: true, attributeOldValue: false, attributes: false }))].join() })()`,
			"TypeError,TypeError,TypeError,none,none,none,TypeError,none"},
		{"a node is put beside itself where it stands", `(() => { const order = () => [...document.querySelectorAll("p")]
			.map((p) => p.id).join(""), [a, b, c] = document.querySelectorAll("p"), seen = [];
			b.after(b); seen.push(order()); b.after(c, b); seen.push(order()); b.before(a, b); seen.push(order());
			a.before(b, a); seen.push(order()); document.body.insertBefore(a, a); seen.push(order());
			c.replaceWith(a, c); seen.push(order()); a.replaceWith(b, a, c); seen.push(order());
			const t = document.createTextNode("t"); document.body.append(t); t.after(t, "u"); t.before("s", t);
			seen.push(t.previousSibling.data + t.data + t.nextSibling.data); t.previousSibling.remove();
			t.nextSibling.remove(); t.remove(); return seen.join() })()`, "abc,acb,cab,cba,cba,acb,bac,stu"},
		{"what is no node is refused before anything else", thrown + `(() => { const t = document.createTextNode("t"),
			b = document.body; return [thrown(() => t.appendChild(null)), thrown(() => t.insertBefore(null, null)),
			thrown(() => t.appendChild(document.createElement("i"))), thrown(() => b.insertBefore(t)),
			thrown(() => b.insertBefore(t, {})), thrown(() => b.insertBefore({}, null)), thrown(() => b.removeChild(null)),
			thrown(() => b.removeChild({})), thrown(() => b.removeChild(t)), thrown(() => b.replaceChild(null, t)),
			thrown(() => t.appendData()), thrown(() => document.createComment("c").appendData()),
			b.insertBefore(t, undefined) === t, t.remove()].join() })()`,
			"TypeError,TypeError,HierarchyRequestError,TypeError,TypeError,TypeError,TypeError,TypeError,NotFoundError," +
				"TypeError,TypeError,TypeError,true,"},
		{"the text an element is given is a new node", `(() => { const e = document.getElementById("held"), old =
			e.firstChild; e.textContent = 42; const seen = [old.parentNode, e.firstChild === old, e.firstChild.data,
			old.data]; e.textContent = "t"; e.textContent = "u"; seen.push(e.childNodes.length, e.textContent);
			return seen.join() })()`, ",false,42,t,1,u"},
		{"what is no more is not found", `[document.applets.length, document.applets instanceof HTMLCollection,
			document.all.war, document.all.a === document.getElementById("a"), typeof self.HTMLAppletElement,
			document.querySelector("applet") instanceof HTMLUnknownElement, document.body.children.namedItem(""),
			document.body.children.namedItem("x") === document.getElementById("x")].join()`,
			"0,true,,true,undefined,true,,true"},
		{"a query is written in ASCII", `(() => { const d = document.implementation.createHTMLDocument("");
			d.body.innerHTML = '<a href="http://example.org/?\u00E4 b&quot;<>\'"></a>';
			return [d.querySelector("a").href, new URL("http://example.org/?\u00E4=1&b=%C3%A4").search,
			new URL("?x y", "http://example.org/a").href, new URL("data:text/plain,?\u00E4 '").search].join(" ") })()`,
			"http://example.org/?%C3%A4%20b%22%3C%3E%27 ?%C3%A4=1&b=%C3%A4 http://example.org/a?x%20y ?%C3%A4%20'"},
		{"a document is made of a namespace, a name and a doctype or none", thrown + `(d => [thrown(() =>
			d.createDocument()), thrown(() => d.createDocument("")), thrown(() => d.createDocument(null, null, false)),
			thrown(() => d.createDocument(null, null, {})), d.createDocument(null, null).childNodes.length,
			d.createDocument(null, undefined).documentElement.localName, d.createDocument(null, "", null).childNodes.length,
			d.createDocument(null, "r", undefined).childNodes.length].join())(document.implementation)`,
			"TypeError,TypeError,TypeError,TypeError,0,undefined,0,1"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}
