//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

func TestAListOfNodesIsNoArrayAndFollowsTheTree(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><div id="d"><p id="foo">a</p><span name="bar" id="one">b</span><p name="baz"
		class="c">c</p><svg id="svg" name="qux"></svg><a id="item" name="length">x</a></div>
		<form id="f"><input name="r" type="radio" value="1"><input name="r" type="radio" value="2" checked><input id="t"
		name="t"><input type="image" name="i"></form>
		<select id="s"><option id="o1" value="a">A</option><option name="o2" value="b" selected>B</option></select>
		<table id="tb"><tbody><tr><td>1</td><th>2</th></tr></tbody></table>`)
	const d = `const d = document.getElementById("d"), thrown = (fn) => { try { fn(); return "none" } catch (e) {
		return e.name } };`
	for _, c := range []struct{ name, script, want string }{
		{"a collection is of its class and no array", d + `(c => [c instanceof HTMLCollection, Array.isArray(c),
			Object.prototype.toString.call(c), typeof c.map, c.length, c.item(1).id, c.item(9), c[9], c[1] === c.item(1),
			"0" in c, "9" in c, Object.hasOwn(c, "length"), thrown(() => new HTMLCollection()),
			thrown(() => HTMLCollection.prototype.item.call({}, 0)), thrown(() => new NodeList())].join())(d.children)`,
			"true,false,[object HTMLCollection],undefined,5,one,,,true,true,false,false,TypeError,TypeError,TypeError"},
		{"it names its elements by id, then by name where they are HTML's", d + `(c => [Object.getOwnPropertyNames(c),
			Object.keys(c), c.foo.id, c.bar.id, c.baz.className, c.namedItem("qux"), c.qux, c.namedItem(""),
			c.namedItem("svg").id, "foo" in c, "qux" in c, Object.hasOwn(c, "bar"), typeof c.item, typeof c.length,
			JSON.stringify(Object.getOwnPropertyDescriptor(c, "foo", (k, v) => v instanceof Node ? v.id : v)),
			Object.getOwnPropertyDescriptor(c, "0").enumerable].join("|"))(d.children)`,
			`0,1,2,3,4,foo,one,bar,baz,svg|0,1,2,3,4|foo|one|c|||` +
				`|svg|true|false|true|function|number|{"value":{},"writable":false,"enumerable":false,"configurable":true}|true`},
		{"what inherits from it sets its own properties, but for an index it has", d + `(c => { const o =
			Object.create(c); o.foo = "mine"; o[0] = "no"; o[9] = "far"; return [o.foo, o[0] === c[0], o[9], c.foo.id,
			thrown(() => o.length)].join() })(d.children)`, "mine,true,far,foo,TypeError"},
		{"it is not written to", d + `(c => { "use strict"; return [thrown(() => { c[0] = 1 }), thrown(() => { c.foo = 1 }),
			thrown(() => { delete c[0] }), thrown(() => { delete c.foo }), delete c[9], (c.mine = 4, c.mine),
			Object.getOwnPropertyNames(c).pop(), thrown(() => Object.defineProperty(c, "1", { value: 1 }))].join() })(
			d.children)`, "TypeError,TypeError,TypeError,TypeError,true,4,mine,TypeError"},
		{"the lists of an element are the same and follow it", d + `(() => { const kids = d.children, nodes =
			d.childNodes, tags = d.getElementsByTagName("p"), cls = document.getElementsByClassName("c"), all =
			document.querySelectorAll("#d p"); const seen = [kids === d.children, nodes === d.childNodes, kids.length,
			nodes.length, tags.length, cls.length, all.length]; const p = document.createElement("p"); p.className = "c x";
			p.id = "new"; d.append(p, "text"); seen.push(kids.length, nodes.length, tags.length, cls.length, all.length,
			kids.new === p, kids[5] === p); p.className = ""; p.id = ""; seen.push(cls.length, kids.new, "new" in kids);
			p.remove(); d.lastChild.remove(); seen.push(kids.length, tags.length); return seen.join() })()`,
			"true,true,5,5,2,1,2,6,7,3,2,2,true,true,1,,false,5,2"},
		{"a list of nodes is walked", d + `(l => { const seen = []; l.forEach(function (n, i, all) { seen.push(n.id + i +
			(all === l) + (this === seen)) }, seen); return [seen.length, seen[0], [...l].length, [...l.keys()].join(""),
			[...l.entries()][1][1].id, [...l.values()][0].id, l[Symbol.iterator] === l.values, l instanceof NodeList,
			[...d.children].length, d.children[Symbol.iterator] === Array.prototype[Symbol.iterator],
			typeof d.children.forEach, Array.from(d.childNodes).length, Array.prototype.map.call(d.children, (e) =>
			e.localName).join("")].join() })(document.querySelectorAll("#d > *"))`,
			"5,foo0truetrue,5,01234,one,foo,true,true,5,false,undefined,5,pspanpsvga"},
		{"a form's controls of one name are a list that has their value", `(e => [e instanceof HTMLFormControlsCollection,
			e.length, e.r instanceof RadioNodeList, e.r.length, e.r.value, e.t.id, e.namedItem("t") === e.t, e.namedItem(
			"none"), e.i, (e.r.value = "1", e.r[0].checked), e.r.value, document.getElementById("f").length,
			document.getElementById("f").r.length, e === document.getElementById("f").elements].join())(
			document.getElementById("f").elements)`, "true,3,true,2,2,t,true,,,true,1,3,2,true"},
		{"a select's options are set as they are read", `(s => { const o = s.options, seen = [o instanceof
			HTMLOptionsCollection, o.length, o.o1.value, o.o2.value, o.selectedIndex, o === s.options, s.length,
			s.selectedOptions.length, s.selectedOptions[0].value]; o.add(new Option("C", "c")); o[3] = new Option("D", "d");
			seen.push(o.length, o[3].value); o.length = 6; seen.push(s.querySelectorAll("option").length); o.length = 1;
			o[0] = null; seen.push(o.length, s.children.length); o.add(new Option("E", "e")); o.add(new Option("F", "f"), 0);
			o.remove(1); seen.push([...o].map((x) => x.value).join("")); return seen.join() })(
			document.getElementById("s"))`, "true,2,a,b,1,true,2,1,b,4,d,6,0,0,f"},
		{"a table's and a document's lists are theirs", `[document.getElementById("tb").rows.length,
			document.getElementById("tb").tBodies.length, document.getElementById("tb").rows[0].cells.length,
			document.forms.length, document.forms.f === document.getElementById("f"), document.forms === document.forms,
			document.images.length, document.links.length, document.scripts.length, document.embeds.length,
			document.all.length > 20, typeof document.all.item, document.all.t.id, document.all.bar,
			document.getElementsByName("bar").length, document.getElementsByName("bar") instanceof NodeList,
			document.createElement("i").childNodes.length, document.createTextNode("t").childNodes.length,
			document.body.childNodes instanceof NodeList].join()`,
			"1,1,2,1,true,true,0,0,0,0,true,function,t,,1,true,0,0,true"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}
