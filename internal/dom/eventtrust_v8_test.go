//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import "testing"

// What light keeps of an event is out of a page's reach: a script
// neither reads it nor makes an event light takes for its own.
func TestAScriptCannotMakeAnEventLightTrusts(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><button id="b">go</button><a id="a" href="/next" target="_blank">next</a>`)
	const tools = `const thrown = (run) => { try { run(); return "none" } catch (e) { return e.name } },
		b = document.getElementById("b");`
	for _, c := range []struct{ name, script, want string }{
		{"an event shows nothing of light's on itself", `(() => { const e = new MouseEvent("click");
			return [Object.getOwnPropertySymbols(e).length, Reflect.ownKeys(e).filter((k) => typeof k === "symbol").length,
			Object.getOwnPropertySymbols(Event.prototype).map(String), Object.getOwnPropertyNames(e).filter((k) =>
			k.startsWith("_") || /light/i.test(k)).length, e[Symbol.for("light:event")], typeof globalThis.__light,
			Object.getOwnPropertyNames(globalThis).filter((k) => /^_|^light/i.test(k))].join() })()`,
			// import() is a call of this one name (pkg/script), the only one light leaves on the window.
			"0,0,Symbol(Symbol.toStringTag),0,,undefined,__light_import__"},
		{"isTrusted is false, and is neither written nor made over", `(() => { "use strict"; const e = new Event("click"),
			d = Object.getOwnPropertyDescriptor(e, "isTrusted");
			e[Symbol.for("light:event")] = { trusted: true, made: true, type: "click" };
			return [e.isTrusted, d.configurable, d.set, "value" in d, thrown(() => { e.isTrusted = true }),
			thrown(() => Object.defineProperty(e, "isTrusted", { value: true })),
			thrown(() => Object.defineProperty(e, "isTrusted", { get: () => true })), thrown(() => delete e.isTrusted),
			e.isTrusted, d.get.call(e), thrown(() => d.get.call({})),
			thrown(() => d.get.call(Object.create(e)))].join() })()`,
			"false,false,,false,TypeError,TypeError,TypeError,TypeError,false,false,TypeError,TypeError"},
		{"what a script sends is heard as a script's, whatever it is made of", `(() => { const seen = [];
			b.addEventListener("click", (e) => seen.push(e.isTrusted)); b.click(); b.dispatchEvent(new MouseEvent("click"));
			const forged = Object.create(MouseEvent.prototype, { isTrusted: { value: true }, type: { value: "click" } });
			seen.push(thrown(() => b.dispatchEvent(forged)), thrown(() => b.dispatchEvent({ type: "click", isTrusted: true })));
			class Mine extends MouseEvent { get isTrusted() { return true } }
			const mine = new Mine("click"); b.dispatchEvent(mine);
			seen.push(Object.getOwnPropertyDescriptor(mine, "isTrusted").get.call(mine));
			return seen.join() })()`, "false,false,TypeError,TypeError,false,false"},
		{"a click a script sends opens no window", `document.getElementById("a").click();
			b.onclick = () => { window.opened = window.open("/popup") }; b.click(); String(window.opened)`, "null"},
	} {
		if got := s.eval(tools + "\n" + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	if len(s.opened) != 0 {
		t.Errorf("clicks a script sent opened %v", s.opened)
	}
}
