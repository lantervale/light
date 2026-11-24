//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

func TestATokenListIsItsAttributeAsASet(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p" class="  a  b a ">p</p><link id="l" rel="icon"><a id="a" href="/x">a</a>
		<iframe id="f"></iframe><output id="o"></output><svg id="s" class="x"></svg>`)
	const (
		p      = `document.getElementById("p")`
		thrown = `const thrown = (fn) => { try { fn(); return "none" } catch (e) { return e.name } };`
		with   = `const p = document.createElement("p"); const l = p.classList;
			const w = (v) => { v === null ? p.removeAttribute("class") : p.setAttribute("class", v); return l };
			const after = (v, fn) => { const got = fn(w(v)); return [got, JSON.stringify(p.getAttribute("class"))] };`
	)
	for _, c := range []struct{ name, script, want string }{
		{"it reads its attribute as an ordered set", `(l => [l.length, l[0], l[1], l[2], l.item(1), l.item(2), l.item(-1),
			l.value, String(l), l + "", l.contains("a"), l.contains("c"), l.contains(""), l.contains("a b"), "0" in l, "2"
			in l,
			Object.prototype.toString.call(l), l instanceof DOMTokenList].join("|"))(` + p + `.classList)`,
			"2|a|b||b|||  a  b a |  a  b a |  a  b a |true|false|false|false|true|false|[object DOMTokenList]|true"},
		{"it is the same list each time", `[` + p + `.classList === ` + p + `.classList,
			document.getElementById("l").relList === document.getElementById("l").relList,
			document.getElementById("a").relList === document.getElementById("a").relList,
			document.getElementById("f").sandbox === document.getElementById("f").sandbox,
			document.getElementById("o").htmlFor === document.getElementById("o").htmlFor,
			document.getElementById("s").classList === document.getElementById("s").classList].join()`,
			"true,true,true,true,true,true"},
		{"it follows its attribute and its attribute follows it", `(() => { const e = document.createElement("i"), l =
			e.classList;
			const seen = [l.length]; e.setAttribute("class", "x y"); seen.push(l.length, l[1]); e.className = "z";
			seen.push(l[0]); l.add("w"); seen.push(e.className, e.getAttribute("class")); e.removeAttribute("class");
			seen.push(l.length, l.value === ""); l.value = " q  r "; seen.push(e.getAttribute("class"), l.length);
			return seen.join("|") })()`, "0|2|y|z|z w|z w|0|true| q  r |2"},
		{"it is set through its element", `(() => { "use strict"; const e = document.createElement("i"), l = e.classList;
			e.classList = "a  b"; const o = document.createElement("output"), h = o.htmlFor; o.htmlFor = "x y";
			const f = document.createElement("iframe"), sb = f.sandbox; f.sandbox = "allow-scripts";
			const k = document.createElement("link"), r = k.relList; k.relList = "icon";
			return [e.classList === l, e.getAttribute("class"), l.length, o.htmlFor === h, o.getAttribute("for"), h.length,
				f.sandbox === sb, f.getAttribute("sandbox"), k.relList === r, k.rel].join("|") })()`,
			"true|a  b|2|true|x y|2|true|allow-scripts|true|icon"},
		{"add writes the set back", with + `[after(null, (l) => l.add("a")), after("a a a  b", (l) => l.add("a")),
			after("a", (l) => l.add("b", "a", "c")), after("  a ", (l) => l.add()), after(null, (l) => l.add()),
			after("", (l) => l.add())].join("|")`, `,"a"|,"a b"|,"a b c"|,"a"|,null|,""`},
		{"remove writes the set back and makes no attribute", with + `[after(null, (l) => l.remove("a")),
			after("a b  a c", (l) => l.remove("a")), after("a", (l) => l.remove("a")), after("a b c", (l) => l.remove("c",
			"a")),
			after(" a  a ", (l) => l.remove("b")), after("", (l) => l.remove("b"))].join("|")`,
			`,null|,"b c"|,""|,"b"|,"a"|,""`},
		{"toggle", with + `[after(null, (l) => l.toggle("a")), after("a b", (l) => l.toggle("a")),
			after("a  a b", (l) => l.toggle("a", true)), after("b", (l) => l.toggle("a", false)),
			after(null, (l) => l.toggle("a", false)), after("b", (l) => l.toggle("a", true)),
			after("a b", (l) => l.toggle("a", false)), after("a", (l) => l.toggle("a", 0))].join("|")`,
			`true,"a"|false,"b"|true,"a  a b"|false,"b"|false,null|true,"b a"|false,"b"|false,""`},
		{"replace", with + `[after("a b c", (l) => l.replace("b", "x")), after("a b c", (l) => l.replace("x", "y")),
			after(null, (l) => l.replace("x", "y")), after("a b a c", (l) => l.replace("c", "a")),
			after("a b c", (l) => l.replace("a", "c")), after("a  b", (l) => l.replace("a", "a")),
			after(" a ", (l) => l.replace("b", "b"))].join("|")`,
			`true,"a x c"|false,"a b c"|false,null|true,"a b"|true,"c b"|true,"a b"|false," a "`},
		{"a token is checked before anything changes", thrown + with + `[
			thrown(() => w("a").add("")), thrown(() => l.add("b", "c d")), thrown(() => l.add("b", "")),
			thrown(() => l.remove("")), thrown(() => l.remove("a\tb")), thrown(() => l.toggle("")),
			thrown(() => l.toggle("a b")), thrown(() => l.replace("", "a")), thrown(() => l.replace("a", "")),
			thrown(() => l.replace(" ", "")), thrown(() => l.replace("a b", "c")), thrown(() => l.replace("a", "c\n")),
			thrown(() => l.replace("", " ")), thrown(() => l.contains("")), thrown(() => l.contains("a b")),
			p.getAttribute("class")].join()`,
			"SyntaxError,InvalidCharacterError,SyntaxError,SyntaxError,InvalidCharacterError,SyntaxError," +
				"InvalidCharacterError,SyntaxError,SyntaxError,SyntaxError,InvalidCharacterError,InvalidCharacterError," +
				"SyntaxError,none,none,a"},
		{"its changes are seen as its attribute's", `(() => { const e = document.createElement("i"); e.className = "a";
			const o = new MutationObserver(() => {}); o.observe(e, { attributes: true, attributeOldValue: true });
			e.classList.add("b"); e.classList.replace("x", "y"); e.classList.replace("a", "c"); e.classList.toggle("c",
			true);
			return o.takeRecords().map((r) => r.attributeName + ":" + r.oldValue).join() })()`, "class:a,class:a b"},
		{"it is walked", `(l => { const seen = []; l.forEach(function (v, i, all) { seen.push(v + i + (all === l) +
			(this === seen)) }, seen);
			return [seen, [...l], [...l.keys()], [...l.values()], [...l.entries()].map((e) => e.join(":")),
			l[Symbol.iterator] === l.values, Object.keys(l), Array.from(l).length].join("|") })(` + p + `.classList)`,
			"a0truetrue,b1truetrue|a,b|0,1|a,b|0:a,1:b|true|0,1|2"},
		{"what its attribute may hold is said where a list is given", thrown + `[
			thrown(() => document.body.classList.supports("a")), thrown(() => document.getElementById("o").htmlFor
			.supports("a")),
			document.getElementById("l").relList.supports("stylesheet"), document.getElementById("l").relList.supports("ICON"),
			document.getElementById("l").relList.supports("bookmark"), document.getElementById("a").relList.supports(
			"noopener"),
			document.getElementById("a").relList.supports("stylesheet"), document.getElementById("f").sandbox.supports(
			"allow-scripts"),
			document.getElementById("f").sandbox.supports("allow-everything"), document.createElement("area").relList
			.supports("noopener")].join()`, "TypeError,TypeError,true,true,false,true,false,true,false,true"},
		{"no script makes one", thrown + `[thrown(() => new DOMTokenList()), typeof DOMTokenList.prototype.add,
			DOMTokenList.prototype.toString === Object.prototype.toString].join()`, "TypeError,function,false"},
		{"elements of every kind have their classes", `[document.createElementNS("http://www.w3.org/2000/svg", "g"),
			document.createElementNS("http://www.w3.org/1998/Math/MathML", "mi"), document.createElementNS(null, "x"),
			document.createElementNS("urn:x", "x"), document.implementation.createDocument(null, "r").documentElement,
			document.getElementById("s")].map((e) => { e.classList.add("a", "b"); e.classList.remove("a");
			return e.getAttribute("class") + e.classList.length }).join()`, "b1,b1,b1,b1,b1,x b2"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}
