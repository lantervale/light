//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"strings"
	"testing"
)

func TestScriptsReadAndChangeThePage(t *testing.T) {
	const page = `<head><title>Old title</title><meta name="kind" content="test"></head>
		<body class="home"><div id="box" class="a b" data-user-id="7" title="T"><p id="first">One <b>bold</b></p>
		<p id="second" hidden>Two</p><span>three</span></div><ul id="list"><li>a</li><li class="on">b</li><li>c</li></ul>
		<input id="name" value="Ann"><input id="agree" type="checkbox" checked><textarea id="notes">hi</textarea>
		<select id="pick"><option value="x">X</option><option value="y" selected>Y</option></select>
		<button id="go" disabled>Go</button><template id="tpl"><i>in</i></template>`
	for _, c := range []struct{ name, script, want string }{
		{"the globals are the window", `window === self && window === globalThis && window.document === document`, "true"},
		{"nothing of the engine's shows", `typeof __light + typeof host + typeof require + typeof process`,
			"undefinedundefinedundefinedundefined"},
		{"getElementById", `document.getElementById("first").tagName`, "P"},
		{"getElementById misses", `document.getElementById("nope")`, "null"},
		{"querySelector", `document.querySelector("#list li.on").textContent`, "b"},
		{"querySelectorAll", `document.querySelectorAll("#list li").length`, "3"},
		{"querySelectorAll is iterable", `[...document.querySelectorAll("li")].map((li) => li.textContent).join("")`, "abc"},
		{"a selector of a node", `document.getElementById("box").querySelector("b").textContent`, "bold"},
		{"a bad selector throws", `try { document.querySelector("li[") } catch (e) { e.name }`, "SyntaxError"},
		{"getElementsByTagName", `document.getElementsByTagName("p").length`, "2"},
		{"getElementsByClassName", `document.getElementsByClassName("on")[0].textContent`, "b"},
		{"the head's meta", `document.querySelector('meta[name="kind"]').content`, "test"},
		{"body, head and html", `[document.body.tagName, document.head.tagName, document.documentElement.tagName].join()`,
			"BODY,HEAD,HTML"},
		{"head is html's first child", `document.documentElement.firstElementChild === document.head`, "true"},
		{"title", `document.title`, "Old title"},
		{"textContent", `document.getElementById("first").textContent`, "One bold"},
		{"innerHTML", `document.getElementById("first").innerHTML`, "One <b>bold</b>"},
		{"outerHTML", `document.querySelector("li.on").outerHTML`, `<li class="on">b</li>`},
		{"id and className", `(b => b.id + ":" + b.className)(document.getElementById("box"))`, "box:a b"},
		{"classList", `(l => [l.contains("a"), l.contains("z"), l.length].join())(document.getElementById("box").classList)`,
			"true,false,2"},
		{"attributes", `(b => [b.getAttribute("title"), b.hasAttribute("nope"), b.getAttribute("nope")].join())(
			document.getElementById("box"))`, "T,false,"},
		{"dataset", `document.getElementById("box").dataset.userId`, "7"},
		{"parent and children", `(b => [b.parentNode === document.body, b.children.length, b.childNodes.length > 3,
			b.firstElementChild.id, b.lastElementChild.tagName].join())(document.getElementById("box"))`,
			"true,3,true,first,SPAN"},
		{"siblings", `(p => [p.nextElementSibling.id, p.previousElementSibling, p.nextSibling.nodeType].join())(
			document.getElementById("first"))`, "second,,3"},
		{"the same node is the same object", `document.getElementById("box") === document.querySelector("div.a") &&
			document.getElementById("first").parentNode === document.getElementById("box")`, "true"},
		{"matches and closest", `(b => [b.matches("b"), b.closest("div").id, b.closest("ul")].join())(
			document.querySelector("b"))`, "true,box,"},
		{"hidden", `[document.getElementById("second").hidden, document.getElementById("first").hidden].join()`,
			"true,false"},
		{"a field's value", `document.getElementById("name").value + document.getElementById("notes").value`, "Annhi"},
		{"checked and disabled", `[document.getElementById("agree").checked, document.getElementById("go").disabled].join()`,
			"true,true"},
		{"a select's value", `(s => s.value + s.selectedIndex + s.options.length)(document.getElementById("pick"))`, "y12"},
		{"a template's content", `document.getElementById("tpl").content.firstChild.tagName`, "I"},
		{"contains", `document.body.contains(document.querySelector("b")) &&
			!document.querySelector("b").contains(document.body)`,
			"true"},
		{"instanceof", `[document.body instanceof HTMLElement, document.getElementById("name") instanceof HTMLInputElement,
			document instanceof Node, document.body.firstChild instanceof Element].join()`, "true,true,true,true"},
		{"location", `[location.href, location.origin, location.pathname, location.search, location.hash].join(" ")`,
			address + " https://site.test /dir/page.html ?q=1 #top"},
		{"navigator", `navigator.userAgent`, "light/test"},
		{"performance.now", `typeof performance.now() === "number" && performance.now() >= 0`, "true"},
		{"the view's size", `innerWidth + "x" + innerHeight`, "800x600"},
		{"readyState", `document.readyState`, "complete"},
		{"URL", `(u => [u.host, u.pathname, u.searchParams.get("a"), u.hash].join())(
			new URL("../x?a=1&b=2#h", location.href))`,
			"site.test,/x,1,#h"},
		{"a URL keeps the brackets of its path", `(u => { u.pathname = "/[locale]/a%2Fb"; return u.pathname + " " +
			new URL("/[id]?x=1", location.href).pathname })(new URL("https://site.test/"))`, "/[locale]/a%2Fb /[id]"},
		{"a media feature named alone", `[matchMedia("(prefers-reduced-motion)").matches, matchMedia("(hover)").matches,
			matchMedia("(min-width)").matches].join()`, "false,true,false"},
		{"URLSearchParams", `(p => { p.append("c", "a b"); return p.toString() })(new URLSearchParams("a=1"))`, "a=1&c=a+b"},
		{"atob and btoa", `atob(btoa("light")) + btoa("é")`, "light6Q=="},
		{"TextEncoder and TextDecoder", `new TextDecoder().decode(new TextEncoder().encode("né")) +
			new TextEncoder().encode("é").length`, "né2"},
		{"what light has not answers with nothing", `[document.body instanceof ShadowRoot,
			new PerformanceObserver(() => 0).takeRecords().length].join()`, "false,0"},
		{"JSON and the language", `JSON.stringify([...new Set([1, 1, 2])].map((n) => n ** 2))`, "[1,4]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, page)
			if got := s.eval(c.script); got != c.want {
				t.Errorf("%s\n is %q\nwant %q", c.script, got, c.want)
			}
			s.quiet()
		})
	}
}

func TestScriptsBuildThePage(t *testing.T) {
	const page = `<div id="box"><p id="first">One</p><p id="second">Two</p></div>`
	for _, c := range []struct{ name, script, want string }{
		{"textContent", `first.textContent = "<b>1</b>"`, `<p id="first">&lt;b&gt;1&lt;/b&gt;</p><p id="second">Two</p>`},
		{"innerHTML", `first.innerHTML = "a <i class='x'>b</i>"`,
			`<p id="first">a <i class="x">b</i></p><p id="second">Two</p>`},
		{"innerText", `first.innerText = "T"`, `<p id="first">T</p><p id="second">Two</p>`},
		{"createElement and appendChild", `const e = document.createElement("span"); e.textContent = "new";
			box.appendChild(e)`,
			`<p id="first">One</p><p id="second">Two</p><span>new</span>`},
		{"createTextNode", `first.appendChild(document.createTextNode(" more"))`,
			`<p id="first">One more</p><p id="second">Two</p>`},
		{"insertBefore", `box.insertBefore(document.createElement("hr"), second)`,
			`<p id="first">One</p><hr><p id="second">Two</p>`},
		{"insertBefore moves a node", `box.insertBefore(second, first)`, `<p id="second">Two</p><p id="first">One</p>`},
		{"removeChild", `box.removeChild(first)`, `<p id="second">Two</p>`},
		{"remove", `second.remove()`, `<p id="first">One</p>`},
		{"replaceChild", `box.replaceChild(document.createElement("em"), first)`, `<em></em><p id="second">Two</p>`},
		{"append and prepend", `box.append("z", document.createElement("u")); box.prepend("a")`,
			`a<p id="first">One</p><p id="second">Two</p>z<u></u>`},
		{"before, after and replaceWith", `first.before("b"); first.after("a"); second.replaceWith("r")`,
			`b<p id="first">One</p>ar`},
		{"insertAdjacentHTML", `first.insertAdjacentHTML("afterend", "<i>x</i>");
			first.insertAdjacentHTML("afterbegin", "<s>y</s>")`,
			`<p id="first"><s>y</s>One</p><i>x</i><p id="second">Two</p>`},
		{"a fragment", `const f = document.createDocumentFragment(); f.append(document.createElement("a"), "t");
			box.appendChild(f)`,
			`<p id="first">One</p><p id="second">Two</p><a></a>t`},
		{"cloneNode", `const c = first.cloneNode(true); c.id = "copy"; box.appendChild(c)`,
			`<p id="first">One</p><p id="second">Two</p><p id="copy">One</p>`},
		{"id and className", `first.id = "f"; f.className = "x y"`, `<p id="f" class="x y">One</p><p id="second">Two</p>`},
		{"classList", `first.classList.add("a", "b"); first.classList.remove("a"); first.classList.toggle("c")`,
			`<p id="first" class="b c">One</p><p id="second">Two</p>`},
		{"attributes", `first.setAttribute("title", "T"); second.removeAttribute("id")`,
			`<p id="first" title="T">One</p><p>Two</p>`},
		{"dataset", `first.dataset.userId = 7`, `<p id="first" data-user-id="7">One</p><p id="second">Two</p>`},
		{"style", `first.style.color = "red"; first.style.marginTop = "2px"`,
			`<p id="first" style="color: red; margin-top: 2px;">One</p><p id="second">Two</p>`},
		{"hidden", `first.hidden = true`, `<p id="first" hidden="">One</p><p id="second">Two</p>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, page+"<script>"+c.script+"</script>")
			if got := s.eval(`document.getElementById("box").innerHTML`); got != c.want {
				t.Errorf("after %s the box holds\n     %s\nwant %s", c.script, got, c.want)
			}
			s.quiet()
		})
	}
}

func TestWhatScriptsBuildIsTheUITree(t *testing.T) {
	s := run(t, `<ul id="list"><li>a</li></ul><input id="name"><input id="agree" type="checkbox"><script>
		const li = document.createElement("li");
		li.textContent = "b";
		li.className = "new";
		document.getElementById("list").appendChild(li);
		document.getElementById("name").value = "typed";
		document.getElementById("agree").checked = true;
		document.title = "Made by a script";
	</script>`)
	list := s.byID("list")
	if len(list.Children()) != 2 || list.Children()[1].Text() != "b" || !list.Children()[1].HasClass("new") {
		t.Errorf("the list holds %q, its second item of class new: %v", list.Text(), len(list.Children()))
	}
	if got := s.page.Doc.Value(s.byID("name")); got != "typed" {
		t.Errorf("the field's value is %q, want typed", got)
	}
	if !s.page.Doc.Checked(s.byID("agree")) {
		t.Error("the checkbox a script checked is not")
	}
	s.rt.Frame()
	if s.page.Title != "Made by a script" {
		t.Errorf("the page's title is %q", s.page.Title)
	}
	s.quiet()
}

func TestScriptsRunInTheOrderOfThePage(t *testing.T) {
	s := run(t, `<script>var order = ["head"]; document.addEventListener("DOMContentLoaded", () => order.push("ready"));
		window.onload = () => order.push("onload"); window.addEventListener("load", () => order.push("load"))</script>
		<p id="p">text</p><script>order.push("body sees " + document.getElementById("p").textContent)</script>
		<script>order.push(document.currentScript.tagName + " " + document.readyState)</script>`)
	want := "head,body sees text,SCRIPT loading,ready,onload,load"
	if got := s.eval(`order.join()`); got != want {
		t.Errorf("the page ran %s\nwant %s", got, want)
	}
}

func TestTheConsoleIsTold(t *testing.T) {
	s := run(t, `<script>console.log("a", 1, {b: [true, null]}, document.body);
		console.warn("careful %s: %d", "now", 4.7); console.error(new TypeError("bad"));
		alert("hello"); console.info("after");</script><script>
		undefinedFunction()</script><script>console.log("still runs")</script>
		<script>Promise.reject(new Error("lost"))</script><script src="ftp://site.test/y.js"></script>`)
	for i, want := range []string{
		`log: a 1 {b: [true, null]} <body>`, "warn: careful now: 4", "error: TypeError: bad", "info: alert: hello",
		"info: after", "error: Uncaught ReferenceError: undefinedFunction is not defined", "log: still runs",
		"error: Uncaught (in promise) Error: lost", "warn: A script was not run: its src",
	} {
		if i >= len(s.console) {
			t.Fatalf("the console has %d lines, want %q next:\n%s", len(s.console), want, s.said())
		}
		if got := s.console[i].Level + ": " + s.console[i].Text; !strings.HasPrefix(got, want) {
			t.Errorf("line %d of the console is %q, want %q", i, got, want)
		}
	}
	if first := s.console[0]; first.Source != address || first.Line != 1 {
		t.Errorf("the first line is from %s:%d, want %s:1", first.Source, first.Line, address)
	}
	if thrown := s.console[5]; thrown.Source != address || thrown.Line != 2 {
		t.Errorf("the exception is from %s:%d, want %s:2", thrown.Source, thrown.Line, address)
	}
}
