//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/ui"
)

// showing are the children ui shows under n: an element by its tag, a
// text by its text in quotes.
func showing(n *ui.Node) string {
	parts := make([]string, len(n.Children()))
	for i, c := range n.Children() {
		if c.IsText() {
			parts[i] = `"` + c.Text() + `"`
		} else {
			parts[i] = c.Tag().String()
		}
	}
	return strings.Join(parts, " ")
}

func TestScriptsChangeThePageAroundWhatUIDoesNotShow(t *testing.T) {
	const page = `<div id="box"><!--a--><p id="first">One</p><svg id="pic"><path d="M0 0"/></svg><!--b-->` +
		`<p id="second">Two</p><script type="text/plain">data</script>tail<!--c--></div>` +
		`<template id="tpl"><li>row</li><!--kept--></template>` +
		`<script>const hr = document.createElement("hr");
		const comments = [...box.childNodes].filter((n) => n.nodeType === 8);
		const [a, b, c] = comments;</script>`
	const rest = `<p id="second">Two</p><script type="text/plain">data</script>tail<!--c-->`
	const pic = `<svg id="pic"><path d="M0 0"></path></svg>`
	for _, c := range []struct{ name, script, html, shown string }{
		{"nothing", ``, `<!--a--><p id="first">One</p>` + pic + `<!--b-->` + rest, `p svg p "tail"`},
		{"insertBefore a comment", `box.insertBefore(hr, b)`,
			`<!--a--><p id="first">One</p>` + pic + `<hr><!--b-->` + rest, `p svg hr p "tail"`},
		{"insertBefore the first comment", `box.insertBefore(hr, box.firstChild)`,
			`<hr><!--a--><p id="first">One</p>` + pic + `<!--b-->` + rest, `hr p svg p "tail"`},
		{"insertBefore an svg", `box.insertBefore(hr, pic)`,
			`<!--a--><p id="first">One</p><hr>` + pic + `<!--b-->` + rest, `p hr svg p "tail"`},
		{"insertBefore the last comment", `box.insertBefore(hr, c)`,
			`<!--a--><p id="first">One</p>` + pic +
				`<!--b--><p id="second">Two</p><script type="text/plain">data</script>tail<hr><!--c-->`,
			`p svg p "tail" hr`},
		{"after a comment", `a.after(hr, "x")`,
			`<!--a--><hr>x<p id="first">One</p>` + pic + `<!--b-->` + rest, `hr "x" p svg p "tail"`},
		{"a comment removed", `b.remove(); box.removeChild(a)`, `<p id="first">One</p>` + pic + rest, `p svg p "tail"`},
		{"a comment replaced", `box.replaceChild(hr, b)`,
			`<!--a--><p id="first">One</p>` + pic + `<hr>` + rest, `p svg hr p "tail"`},
		{"an element replaced by a comment", `box.replaceChild(document.createComment("gone"), first)`,
			`<!--a--><!--gone-->` + pic + `<!--b-->` + rest, `svg p "tail"`},
		{"a comment's text", `b.data = "B"; c.nodeValue += "!"; a.textContent = ""`,
			`<!----><p id="first">One</p>` + pic + `<!--B-->` + rest[:len(rest)-8] + `<!--c!-->`, `p svg p "tail"`},
		{"an svg moved", `box.appendChild(pic); box.prepend(box.querySelector("script"))`,
			`<script type="text/plain">data</script><!--a--><p id="first">One</p><!--b--><p id="second">Two</p>tail<!--c-->` +
				pic,
			`p p "tail" svg`},
		{"an element in an svg, then out of it", `pic.appendChild(hr); const held = box.innerHTML.includes("<hr></svg>");
			if (held) pic.after(hr)`, `<!--a--><p id="first">One</p>` + pic + `<hr><!--b-->` + rest, `p svg hr p "tail"`},
		{"an element built around what is not shown", `const d = document.createElement("div");
			d.innerHTML = "<!--x--><b>y</b><svg><g></g></svg>z<template><i>t</i></template>"; box.insertBefore(d, b);
			d.insertBefore(hr, d.childNodes[2])`,
			`<!--a--><p id="first">One</p>` + pic +
				`<div><!--x--><b>y</b><hr><svg><g></g></svg>z<template><i>t</i></template></div><!--b-->` + rest,
			`p svg div p "tail"`},
		{"a fragment with a comment", `const f = document.createDocumentFragment();
			f.append(document.createComment("f"), hr, "t"); box.insertBefore(f, pic)`,
			`<!--a--><p id="first">One</p><!--f--><hr>t` + pic + `<!--b-->` + rest, `p hr "t" svg p "tail"`},
		{"a template's content", `box.insertBefore(tpl.content.cloneNode(true), b)`,
			`<!--a--><p id="first">One</p>` + pic + `<li>row</li><!--kept--><!--b-->` + rest, `p svg li p "tail"`},
		{"cloneNode", `const copy = box.cloneNode(true); copy.id = "copy"; const same = copy.innerHTML === box.innerHTML;
			if (same && copy.firstChild.nodeType === 8 && !copy.firstChild.isConnected)
				box.replaceChildren(copy.childNodes[1], copy.childNodes[2])`,
			`<p id="first">One</p>` + pic, `p svg`},
		{"innerHTML", `first.innerHTML = "<!--x--><b>y</b><svg><g></g></svg>z"; second.textContent = "";
			box.insertBefore(hr, first.nextSibling)`,
			`<!--a--><p id="first"><!--x--><b>y</b><svg><g></g></svg>z</p><hr>` + pic + `<!--b--><p id="second"></p>` +
				`<script type="text/plain">data</script>tail<!--c-->`, `p hr svg p "tail"`},
		{"textContent", `box.textContent = "only"`, `only`, `"only"`},
		{"textContent of nothing", `box.textContent = ""`, ``, ``},
		{"normalize", `box.replaceChildren("a", "", a, "b", "", "c", hr, ""); const before = box.childNodes.length;
			box.normalize(); box.append(String(before), String(box.childNodes.length))`,
			`a<!--a-->bc<hr>84`, `"a" "bc" hr "8" "4"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, page+"<script>"+c.script+"</script>")
			if got := s.eval(`box.innerHTML`); got != c.html {
				t.Errorf("after %s the box holds\n     %s\nwant %s", c.script, got, c.html)
			}
			if got := showing(s.byID("box")); got != c.shown {
				t.Errorf("after %s the box shows %s, want %s", c.script, got, c.shown)
			}
			links := s.eval(`(() => { const kids = [...box.childNodes]; return kids.every((n, i) => n.parentNode === box &&
				n.previousSibling === (kids[i - 1] || null) && n.nextSibling === (kids[i + 1] || null) && n.isConnected) &&
				box.firstChild === (kids[0] || null) && box.lastChild === (kids.at(-1) || null) })()`)
			if links != "true" {
				t.Errorf("after %s the box's children do not link to each other", c.script)
			}
			s.quiet()
		})
	}
}

func TestTextApartByACommentIsTwoTexts(t *testing.T) {
	s := run(t, `<p id="t">Hello <!-- -->world<!-- -->!</p>`)
	for _, c := range []struct{ script, want string }{
		{`[t.childNodes.length, t.childNodes[1].nodeType, t.childNodes[1].data, t.firstChild.nextSibling.nextSibling.data,
			t.textContent, t.firstChild.wholeText].join("|")`, "5|8| |world|Hello world!|Hello "},
		{`t.childNodes[2].data = "light"; t.lastChild.nodeValue = "."; t.textContent`, "Hello light."},
		{`t.innerHTML`, "Hello <!-- -->light<!-- -->."},
		{`t.childNodes[1].remove(); t.normalize(); [t.childNodes.length, t.firstChild.data, t.innerHTML].join("|")`,
			"3|Hello light|Hello light<!-- -->."},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s\n is %q\nwant %q", c.script, got, c.want)
		}
	}
	if got := showing(s.byID("t")); got != `"Hello light" "."` {
		t.Errorf("the paragraph shows %s", got)
	}
	s.quiet()
}

func TestANodeOutOfThePageIsStillANode(t *testing.T) {
	s := run(t, `<div id="box"><!--a--><p id="first">One</p><svg id="pic"><path/></svg></div>`)
	for _, c := range []struct{ script, want string }{
		{`const a = box.firstChild; a.remove(); [a.parentNode, a.isConnected, a.ownerDocument === document, a.data,
			a.nextSibling, box.contains(a)].join("|")`, "|false|true|a||false"},
		{`const pic = document.getElementById("pic"), path = pic.firstChild; pic.remove();
			[pic.isConnected, path.parentNode === pic, path.isConnected, document.getElementById("pic"),
			pic.getBoundingClientRect().width, pic.outerHTML].join("|")`,
			`false|true|false||0|<svg id="pic"><path></path></svg>`},
		{`const c = document.createComment("made"), e = document.createElementNS("http://www.w3.org/2000/svg", "circle");
			e.setAttribute("viewBox", "0 0 1 1"); e.appendChild(c);
			[c.nodeType, c.nodeName, c.data, c.parentNode === e, e.namespaceURI.endsWith("svg"), e.tagName,
			e.outerHTML, new Comment("x").data, new Text("y").data, e.compareDocumentPosition(c)].join("|")`,
			`8|#comment|made|true|true|circle|<circle viewBox="0 0 1 1"><!--made--></circle>|x|y|20`},
		{`try { box.insertBefore(document.createElement("i"), document.createComment("stray")) } catch (e) { e.name }`,
			"NotFoundError"},
		{`try { first.appendChild(box) } catch (e) { e.name }`, "HierarchyRequestError"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s\n is %q\nwant %q", c.script, got, c.want)
		}
	}
	s.quiet()
}

func TestAScriptPutInThePageRunsOnceWhereverItGoes(t *testing.T) {
	s := run(t, `<div id="box"><!--a--></div><script>window.ran = [];
		const one = document.createElement("script"); one.textContent = "ran.push(document.currentScript.parentNode.id)";
		box.insertBefore(one, box.firstChild);</script>`)
	s.frames("the script", func() bool { return s.eval(`ran.length`) == "1" })
	if got := s.eval(`document.body.appendChild(one); one.parentNode.tagName + ran.join()`); got != "BODYbox" {
		t.Errorf("the script ran as %s", got)
	}
	s.rt.Frame()
	if got := s.eval(`ran.join() + box.innerHTML`); got != "box<!--a-->" {
		t.Errorf("after it moved the script ran as %s", got)
	}
	s.quiet()
}

func TestAChangeOfManyNodesIsToldOnce(t *testing.T) {
	const page = `<!DOCTYPE html><body><div id="d"><i id="a"></i><i id="b"></i><i id="c"></i></div>` +
		`<p id="p"><u id="u"></u></p>`
	const tools = `const $ = (id) => document.getElementById(id), told = (run) => {
		const o = new MutationObserver(() => {}); o.observe(document.body, { childList: true, subtree: true });
		run(); return o.takeRecords().map((r) =>
		(r.target.id || r.target.nodeName) + ":+" + [...r.addedNodes].map((n) => n.id || n.nodeName) + ":-" +
		[...r.removedNodes].map((n) => n.id || n.nodeName) + ":" + (r.previousSibling ? r.previousSibling.id : "") +
		"<>" + (r.nextSibling ? r.nextSibling.id : "")).join(" ") },
		thrown = (run) => { try { run(); return "none" } catch (e) { return e.name } };`
	for _, c := range []struct{ name, script, want string }{
		{"a child replaced is one change, after the new one left where it was", `told(() =>
			$("d").replaceChild($("u"), $("b")))`, "p:+:-u:<> d:+u:-b:a<>c"},
		{"a child replaced by itself, or by the one after it", `told(() => { $("d").replaceChild($("a"), $("a"));
			$("d").replaceChild($("c"), $("b")) })`, "d:+:-a:<>b d:+a:-:<>b d:+:-c:b<> d:+c:-b:a<>"},
		{"children replaced by one of them are one change, and no text for none is none", `told(() => {
			$("d").replaceChildren($("b")); $("u").textContent = ""; $("u").textContent = "t" })`,
			"d:+b:-a,b,c:<> u:+#text:-:<>"},
		{"a live list is walked to the end it has by then, a fragment finds by id", `(() => { const seen = [],
			f = document.createDocumentFragment(), q = document.createElement("q"); q.id = "x y"; f.append(q);
			for (const n of $("d").childNodes) { seen.push(n.id); if (n.id === "a") $("d").append($("u")) }
			return [seen, f.getElementById("x y") === q, f.getElementById(""), f.getElementById("no")].join() })()`,
			"a,b,c,u,true,,"},
		{"a document's children are replaced as if it were empty", `(() => { const d =
			document.implementation.createHTMLDocument(""), h = d.createElement("html");
			d.replaceChildren(h);
			return [d.childNodes.length, d.documentElement === h, thrown(() => d.replaceChildren(h, d.createElement("p")))]
			.join() })()`, "1,true,HierarchyRequestError"},
		{"a fragment's nodes come in as one change", `told(() => { const f =
			document.createDocumentFragment(); f.append(document.createElement("s"), document.createElement("q"));
			$("d").insertBefore(f, $("b")) })`, "d:+S,Q:-:a<>b"},
		{"children replaced are one change", `told(() => $("p").replaceChildren($("c"), "t")) + "|" +
			$("p").childNodes.length + told(() => $("p").replaceChildren())`,
			"d:+:-c:b<> p:+c,#text:-u:<>|2p:+:-c,#text:<>"},
		{"a node is moved within its tree, and only there", `(() => { $("d").moveBefore($("u"), $("a"));
			$("d").moveBefore($("a"), null); $("d").moveBefore($("u"), $("u")); return [$("d").firstChild.id,
			$("d").lastChild.id, document.body.lastChild.id, thrown(() => $("d").moveBefore(document.createElement("i"),
			null)), thrown(() => $("a").moveBefore($("d"), null)), thrown(() => $("d").moveBefore($("a"), $("p"))),
			thrown(() => $("d").moveBefore($("a"))), typeof document.moveBefore, typeof document.prepend].join() })()`,
			"u,a,p,HierarchyRequestError,HierarchyRequestError,NotFoundError,TypeError,function,function"},
	} {
		s := run(t, page)
		if got := s.eval(tools + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
		s.quiet()
	}
}

func TestWhatLeavesAWatchedSubtreeIsFollowedUntilItsObserverIsTold(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><div id="d"><i id="a"></i></div>`)
	s.eval(`window.got = []; window.o = new MutationObserver((records) => got.push(records.length));
		o.observe(document.getElementById("d"), { childList: true, subtree: true, attributes: true });
		window.a = document.getElementById("a"); a.remove(); a.append("late"); a.id = "z";
		window.first = o.takeRecords().map((r) => r.type + ":" + r.target.nodeName).join(); a.append("more");
		Promise.resolve().then(() => { a.append("unseen"); })`)
	const want = "childList:DIV,childList:I,attributes:I|1|0"
	if got := s.eval(`[first, got, o.takeRecords().length].join("|")`); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	s.quiet()
}
