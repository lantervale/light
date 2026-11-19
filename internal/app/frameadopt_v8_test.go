//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"
)

func TestANodeGoesFromADocumentToTheOneOfAKinsFrame(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><body style="margin:0"><p id="mine">mine</p>
			<div id="holder"><iframe id="inner" src="/inner"></iframe></div><input id="b" name="b" value="2">
			<iframe id="one" src="/one"></iframe><iframe id="x" src="/feed.xml"></iframe>
			<iframe id="far" src="https://other.test/far"></iframe>
			<iframe id="boxed" sandbox="allow-scripts" src="/one"></iframe><script>window.mark = "top";</script>`),
		"site.test/one": html(`<body style="margin:0"><p id="own">own</p><form id="f"><input name="a" value="1"></form>
			<script>window.mark = "one";</script>`),
		"site.test/inner":    html(`<p>inner</p><script>window.mark = "inner";</script>`),
		"site.test/feed.xml": typed("application/xml", feedXML),
		"other.test/far":     html(`<p>far</p><script>window.mark = "far";</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "one", "inner", "far", "boxed")
	s.framedText("x")

	shows := func() string { return s.session().pages[s.browser.Tab().Current()].Doc.Body().Text() }

	s.does("", `window.d = one.contentDocument; window.p = document.getElementById("mine");
		p.onclick = () => { window.clicked = (window.clicked || 0) + 1; }; d.body.appendChild(p);`)
	s.gives("", map[string]string{
		`[p.ownerDocument === d, d.body.lastChild === p, document.getElementById("mine")].join()`: "true,true,",
	})
	if got := s.framedText("one"); !strings.Contains(got, "mine") || strings.Contains(shows(), "mine") {
		t.Errorf("the frame shows %q, want the paragraph the page put in it, and the page none", got)
	}
	s.gives("one", map[string]string{
		`(() => { const p = document.getElementById("mine"); p.click(); return [p.textContent, p.parentNode ===
			document.body, parent.clicked].join(); })()`: "mine,true,1",
	})

	s.does("", `d.getElementById("f").appendChild(document.getElementById("b"));`)
	s.gives("one", map[string]string{
		`(() => { const f = document.getElementById("f"); return [f.elements.length, f.elements.b.value,
			f.elements.b.form === f, new URLSearchParams(new FormData(f)).toString()].join(); })()`: "2,2,true,a=1&b=2",
	})

	s.does("", `d.body.appendChild(document.getElementById("holder"));`)
	s.until("the frame that moved with its holder shows its document in the other frame", func() bool {
		return s.within("one", `(() => { const f = document.querySelector("iframe"); return f && f.contentDocument &&
			f.contentDocument.querySelector("p") ? f.contentDocument.querySelector("p").textContent +
			f.contentWindow.mark + frames[0].mark : ""; })()`) == "innerinnerinner"
	})
	s.gives("", map[string]string{
		`[document.getElementById("inner"), frames.length, d.defaultView.frames.length].join()`: ",4,1",
	})

	s.does("", `window.xd = x.contentDocument; window.entry = xd.getElementById("e"); document.body.appendChild(entry);
		xd.documentElement.appendChild(document.createElement("p")).textContent = "html";`)
	s.gives("", map[string]string{
		`[entry.ownerDocument === document, entry.namespaceURI, entry.prefix, xd.getElementById("e"),
			document.body.lastChild === entry].join()`: "true,urn:a,a,,true",
		`new XMLSerializer().serializeToString(xd.documentElement)`: `<feed xmlns:a="urn:a"><entry>two</entry>` +
			`<p xmlns="http://www.w3.org/1999/xhtml">html</p></feed>`,
		`far.contentWindow.postMessage(document.body, "*")`:           "throws DataCloneError",
		`boxed.contentWindow.postMessage(document.body, "*")`:         "throws DataCloneError",
		`String(far.contentDocument) + String(boxed.contentDocument)`: "nullnull",
	})
	if !strings.Contains(shows(), "one") {
		t.Errorf("the page shows %q, want the element of the document of XML it took", shows())
	}
}
