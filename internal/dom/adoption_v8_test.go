//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/dom"
)

// gives checks what expressions are in a document of the family, each
// as the console prints it, a string without its quotes.
func (m *member) gives(t *testing.T, cases ...[2]string) {
	t.Helper()
	for _, c := range cases {
		got, err := m.rt.Evaluate(c[0])
		if text, quoted := strconv.Unquote(got); quoted == nil {
			got = text
		}
		if err != nil || got != c[1] {
			t.Errorf("window %d: %s\n gives %s, %v\n  want %s", m.self, c[0], got, err, c[1])
		}
	}
}

// shows is the text ui shows of a document of the family.
func (m *member) shows() string {
	m.page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	return strings.Join(strings.Fields(m.page.Doc.Body().Text()), " ")
}

func TestANodeMovesBetweenTheDocumentsOfKin(t *testing.T) {
	kin, hold := dom.NewKin("https://site.test"), &dom.Hold{}
	f := &family{tab: &tab{closed: map[int]bool{}}, frames: 2}
	top := f.joins(t, kin, hold, 1, `<body><p id="mine">mine</p><div id="keep"><b>kept</b></div><i id="it">it</i>
		<script id="ran">window.runs = (window.runs || 0) + 1; window.mark = "top";</script>`)
	one := f.joins(t, kin, hold, 2, `<body><p id="own">own</p><script>window.mark = "one";
		window.addEventListener("rise", (e) => { window.rose = (window.rose || "") + e.target.id; });
		window.seen = []; new MutationObserver((records) => { for (const r of records) window.seen.push(
			r.type + ":" + Array.from(r.addedNodes, (n) => n.id || n.nodeName).join() + "/" +
			Array.from(r.removedNodes, (n) => n.id || n.nodeName).join() + (r.attributeName || "")); }).observe(
			document.body, { childList: true, subtree: true, attributes: true });</script>`)
	two := f.joins(t, kin, hold, 3, `<body><script>window.mark = "two";</script>`)

	top.gives(t,
		[2]string{`window.d = frames[0].document; window.w = frames[0]; window.p = document.getElementById("mine");
			window.gone = []; new MutationObserver((records) => { for (const r of records) window.gone.push(
			Array.from(r.removedNodes, (n) => n.id).join()); }).observe(document.body, { childList: true });
			p.tag = 7; p.addEventListener("ping", () => { window.heard = (window.heard || 0) + 1; });
			d.body.appendChild(p) === p`, "true"},
		[2]string{`[p.ownerDocument === d, p.parentNode === d.body, d.body.lastChild === p, p.isConnected,
			document.getElementById("mine"), d.getElementById("mine") === p, p.getRootNode() === d,
			Object.getPrototypeOf(p) === HTMLParagraphElement.prototype].join()`,
			"true,true,true,true,,true,true,true"},
	)
	if got, was := one.shows(), top.shows(); got != "ownmine" || strings.Contains(was, "mine") {
		t.Fatalf("the frame shows %q and the page %q, want the paragraph in the frame alone", got, was)
	}
	one.gives(t,
		[2]string{`(() => { const p = document.getElementById("mine"); p.dispatchEvent(new Event("ping"));
			return [p.tag, document.body.lastChild === p, document.querySelector("#mine") === p, p.ownerDocument ===
			document, p.textContent, document.body.children.length, p.previousElementSibling.tagName].join(); })()`,
			"7,true,true,true,mine,3,SCRIPT"},
		[2]string{`Promise.resolve().then(() => 0), seen.join(" ")`, "childList:mine/"},
	)
	top.gives(t,
		[2]string{`String(window.heard) + gone.join()`, "1mine"},
		[2]string{`p.textContent = "changed"; p.setAttribute("class", "k"); p.append(document.createElement("b"));
			[p.className, d.querySelector(".k") === p, p.lastChild.ownerDocument === d, p.lastChild.tagName].join()`,
			"k,true,true,B"},
		[2]string{`window.addEventListener("rise", () => { window.rose = "here"; });
			p.dispatchEvent(new Event("rise", { bubbles: true })); [w.rose, window.rose].join()`, "mine,"},
	)
	if got := one.shows(); got != "ownchanged" {
		t.Errorf("the frame shows %q once the page's script changed the paragraph it holds", got)
	}
	one.gives(t, [2]string{`seen.slice(1).join(" ")`, "childList:#text/#text attributes:/class childList:B/"})

	top.gives(t,
		[2]string{`(() => { const it = document.getElementById("it"); const got = d.adoptNode(it);
			return [got === it, it.ownerDocument === d, it.parentNode, document.getElementById("it"),
			d.body.contains(it)].join(); })()`, "true,true,,,false"},
		[2]string{`(() => { const keep = document.getElementById("keep"); const copy = d.importNode(keep, true);
			const flat = d.importNode(keep);
			return [copy !== keep, copy.ownerDocument === d, keep.ownerDocument === document, copy.innerHTML,
			flat.childNodes.length, Object.getPrototypeOf(copy) === w.HTMLDivElement.prototype,
			copy instanceof w.HTMLDivElement, copy.firstChild.ownerDocument === d, keep.parentNode ===
			document.body].join(); })()`, "true,true,true,<b>kept</b>,0,true,true,true,true"},
		[2]string{`(() => { const keep = document.getElementById("keep"); const copy = keep.cloneNode(true);
			copy.id = "copy"; d.body.insertBefore(copy, d.body.firstChild);
			const text = document.createTextNode("t"); d.body.appendChild(text);
			const fragment = document.createDocumentFragment(); fragment.append(document.createElement("u"), "v");
			d.body.append(fragment, "w");
			const own = d.getElementById("own"); const swap = document.createElement("s"); swap.id = "swap";
			const old = d.body.replaceChild(swap, own);
			return [d.body.firstChild === copy, copy.ownerDocument === d, d.body.childNodes.length, text.parentNode ===
			d.body, text.ownerDocument === d, old === own, own.parentNode, d.getElementById("swap") === swap,
			d.body.lastChild.data, fragment.childNodes.length].join(); })()`, "true,true,8,true,true,true,,true,w,0"},
	)
	one.gives(t, [2]string{`[document.body.firstChild.id, document.querySelector("u").nextSibling.data,
		document.getElementById("swap").tagName, document.getElementById("own")].join()`, "copy,v,S,"})

	top.gives(t,
		[2]string{`(() => { const ran = document.getElementById("ran"); d.body.appendChild(ran);
			const fresh = document.createElement("script"); fresh.textContent = "window.ranIn = window.mark;";
			d.body.appendChild(fresh); return [window.runs, window.ranIn, w.runs, w.ranIn].join(); })()`, "1,,,"},
	)
	one.rt.Frame()
	top.gives(t,
		[2]string{`[window.runs, window.ranIn, w.runs, w.ranIn].join()`, "1,,,one"},
		[2]string{`document.body.appendChild(p); [p.ownerDocument === document, p.parentNode === document.body,
			d.getElementById("mine"), document.getElementById("mine") === p].join()`, "true,true,,true"},
	)
	if got, was := top.shows(), one.shows(); !strings.Contains(got, "changed") || strings.Contains(was, "changed") {
		t.Errorf("the page shows %q and the frame %q, want the paragraph back in the page alone", got, was)
	}

	one.gives(t, [2]string{`(() => { const swap = document.getElementById("swap"); const to = parent.frames[1].document;
		to.body.appendChild(swap); swap.textContent = "third";
		return [swap.ownerDocument === to, to.body.lastChild === swap, document.getElementById("swap")].join(); })()`,
		"true,true,"})
	if got := two.shows(); got != "third" {
		t.Errorf("the second frame shows %q, want what the first put in it", got)
	}
	two.gives(t, [2]string{`[document.body.lastChild.id, document.body.lastChild instanceof HTMLElement,
		Object.getPrototypeOf(document.body.lastChild) === parent.HTMLElement.prototype].join()`, "swap,true,true"})

	top.gives(t, [2]string{`window.born = d.createElement("em"); born.textContent = "born"; born.id = "born";
		document.body.appendChild(born); window.left = document.createElement("q"); left.textContent = "left";
		d.body.appendChild(left); born.ownerDocument === document && left.ownerDocument === d`, "true"})
	one.rt.Close()
	if !one.rt.Closed() {
		t.Fatal("the frame's document did not close")
	}
	top.gives(t, [2]string{`[document.getElementById("born").textContent, document.body.lastChild.id,
		document.body.lastChild.ownerDocument === document].join()`, "born,born,true"},
		[2]string{`left.textContent = "still"; left.textContent + left.tagName`, "stillQ"})
	if got := top.shows(); !strings.Contains(got, "born") {
		t.Errorf("the page shows %q, want the node the closed frame's document made", got)
	}
}
