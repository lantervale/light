//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
)

func TestADocumentAScriptMakesIsWhole(t *testing.T) {
	s := run(t, `<!DOCTYPE html><title>Page</title><body><p id="own">own</p>`)
	for _, c := range []struct{ name, script, want string }{
		{"createHTMLDocument", `(d => [d instanceof Document, d instanceof HTMLDocument, d instanceof XMLDocument, d ===
			document,
			d.nodeType, d.nodeName, d.title, d.documentElement.tagName, d.head.tagName, d.body.tagName, d.doctype.name,
			d.childNodes.length, d.contentType, d.URL, d.documentURI, d.defaultView, d.compatMode, d.characterSet,
			d.ownerDocument, d.parentNode, d.isConnected].join())(document.implementation.createHTMLDocument("T"))`,
			"true,true,false,false,9,#document,T,HTML,HEAD,BODY,html,2,text/html,about:blank,about:blank,," +
				"CSS1Compat,UTF-8,,,true"},
		{"createHTMLDocument without a title has no title element",
			`(d => d.title + d.querySelectorAll("title").length)(document.implementation.createHTMLDocument())`, "0"},
		{"its title is set", `(d => { d.title = "New"; return d.title + d.head.firstChild.tagName + document.title })(
			document.implementation.createHTMLDocument("Old"))`, "NewTITLEPage"},
		{"what it makes is its own", `(d => [d.createElement("p"), d.createTextNode("t"), d.createComment("c"),
			d.createDocumentFragment(), d.documentElement, d.body, d.doctype].map((n) => n.ownerDocument === d).join())(
			document.implementation.createHTMLDocument())`, "true,true,true,true,true,true,true"},
		{"the page's nodes stay the page's", `[document.createElement("p"), document.body, document.createTextNode("t"),
			document.createDocumentFragment()].map((n) => n.ownerDocument === document).join()`, "true,true,true,true"},
		{"it is searched as the page is", `(d => { d.body.innerHTML =
			'<p id="a" class="x">hi<b name="n">b</b></p><p class="x y">yo</p>';
			return [d.getElementById("a").textContent, d.getElementById("own"), d.querySelector("p.y").textContent,
			d.querySelectorAll(".x").length, d.getElementsByTagName("P").length, d.getElementsByClassName("x y").length,
			d.getElementsByName("n").length, d.body.children.length, d.all.length, document.getElementById("a"),
			d.getElementById("a").ownerDocument === d, d.body.firstChild.isConnected, document.contains(d.body.firstChild),
			d.contains(d.body.firstChild), d.contains(document.body)].join() })(
			document.implementation.createHTMLDocument())`,
			"hib,,yo,2,2,1,1,2,6,,true,true,false,true,false"},
		{"a node is adopted where it is put", `(d => { const div = document.createElement("div"); div.innerHTML = "<i>in</i>";
			const was = [div.ownerDocument === document, div.firstChild.ownerDocument === document];
			d.body.appendChild(div);
			const there = [div.ownerDocument === d, div.firstChild.ownerDocument === d, div.firstChild.firstChild.ownerDocument
				=== d];
			document.body.appendChild(div);
			return was.concat(there, [div.ownerDocument === document, div.firstChild.ownerDocument === document,
			d.body.childNodes.length]).join() })(document.implementation.createHTMLDocument())`,
			"true,true,true,true,true,true,true,0"},
		{"adoptNode and importNode", `(d => { const own = document.getElementById("own");
			const copy = d.importNode(own, true), bare = d.importNode(own, false), taken =
				d.adoptNode(document.createElement("u"));
			return [copy !== own, copy.ownerDocument === d, copy.textContent, bare.childNodes.length, own.ownerDocument ===
				document,
			own.parentNode === document.body, taken.ownerDocument === d, taken.parentNode,
			document.adoptNode(copy).ownerDocument === document].join() })(document.implementation.createHTMLDocument())`,
			"true,true,own,0,true,true,true,,true"},
		{"a document is neither adopted nor imported", `(d => { const names = [];
			for (const f of [() => document.adoptNode(d), () => document.importNode(d, true),
				() => document.body.appendChild(d)]) {
				try { f(); names.push("none") } catch (e) { names.push(e.name) } }
			return names.join() })(document.implementation.createHTMLDocument())`,
			"NotSupportedError,NotSupportedError,HierarchyRequestError"},
		{"a clone is of its document", `(d => { const p = d.createElement("p"); p.innerHTML = "<i>x</i>"; const c =
			p.cloneNode(true);
			return [c.ownerDocument === d, c.firstChild.ownerDocument === d].join() })(
			document.implementation.createHTMLDocument())`, "true,true"},
		{"createDocument", `(d => [d instanceof XMLDocument, d instanceof Document, d.contentType, d.documentElement,
			d.childNodes.length, d.doctype, d.URL, d.defaultView, d.head, d.body, d.title].join())(
			document.implementation.createDocument(null, null, null))`, "true,true,application/xml,,0,,about:blank,,,,"},
		{"createDocument with a root and a doctype", `(t => (d => [d.documentElement.localName,
			d.documentElement.namespaceURI,
			d.documentElement.tagName, d.doctype === t, t.ownerDocument === d, d.childNodes.length, d.contentType,
			d.documentElement instanceof Element, d.documentElement instanceof HTMLElement].join())(
			document.implementation.createDocument("urn:x", "Root", t)))(
			document.implementation.createDocumentType("Root", "pub", "sys"))`,
			"Root,urn:x,Root,true,true,2,application/xml,true,false"},
		{"createDocument by its namespace", `["http://www.w3.org/1999/xhtml", "http://www.w3.org/2000/svg"].map((ns) =>
			(d => d.contentType + " " + d.documentElement.constructor.name)(
			document.implementation.createDocument(ns, ns.endsWith("svg") ? "svg" : "html", null))).join()`,
			"application/xhtml+xml HTMLHtmlElement,image/svg+xml SVGElement"},
		{"createDocumentType", `(t => [t instanceof DocumentType, t.nodeType, t.name, t.nodeName, t.publicId, t.systemId,
			t.ownerDocument === document, t.parentNode].join())(document.implementation.createDocumentType("a:b", "p", "s"))`,
			"true,10,a:b,a:b,p,s,true,"},
		{"the page's doctype has no identifiers", `[document.doctype.name, document.doctype.publicId,
			document.doctype.systemId].join()`,
			"html,,"},
		{"new Document()", `(d => [d instanceof Document, d instanceof XMLDocument, d.contentType, d.URL, d.childNodes.length,
			d.createElement("Foo").localName, d.createElement("Foo").namespaceURI, d.createElement("Foo").tagName,
			d.createElement("Foo").ownerDocument === d, d.implementation instanceof DOMImplementation].join())(new Document())`,
			"true,false,application/xml,about:blank,0,Foo,,Foo,true,true"},
		{"an XML document's elements", `(d => { const e = d.createElement("Big"); e.setAttribute("Attr", "1");
			const h = d.createElementNS("http://www.w3.org/1999/xhtml", "DIV");
			return [e.localName, e.tagName, e.namespaceURI, e.getAttribute("Attr"), e.getAttribute("attr"), e instanceof
				HTMLElement,
			h.tagName, h.localName, h.namespaceURI, d.createElementNS("urn:other", "p:Q").localName,
			d.createElementNS("urn:other", "Q").namespaceURI, d.createElementNS("urn:other", "p:Q").prefix,
			d.createElementNS("urn:other", "p:Q").tagName, d.createElementNS("urn:other", "p:Q").cloneNode().tagName,
			d.createElementNS("urn:other", "Q").prefix, document.createElementNS("http://www.w3.org/1999/xhtml", "x:b").tagName,
			d.appendChild(d.createElementNS("urn:other", "p:Q")) && d.getElementsByTagName("p:Q").length +
			d.getElementsByTagName("Q").length + d.getElementsByTagNameNS("urn:other", "Q").length].join() })(
			document.implementation.createDocument(null, null))`,
			"Big,Big,,1,,false,DIV,DIV,http://www.w3.org/1999/xhtml,Q,urn:other,p,p:Q,p:Q,,X:B,2"},
		{"a made HTML document without a doctype is in quirks mode", `[new DOMParser().parseFromString("<p>",
			"text/html").compatMode,
			new DOMParser().parseFromString("<!DOCTYPE html><p>", "text/html").compatMode,
			document.implementation.createHTMLDocument().compatMode, new Document().compatMode].join()`,
			"BackCompat,CSS1Compat,CSS1Compat,CSS1Compat"},
		{"the page's elements are HTML's", `(e => { e.setAttribute("Attr", "1"); return [e.localName, e.tagName,
			e.getAttribute("attr"), e.namespaceURI, document.createElementNS("urn:other", "Q").namespaceURI,
			document.createElementNS(null, "Q").namespaceURI, document.createElementNS("urn:other", "Q") instanceof
				HTMLElement].join() })(
			document.createElement("DIV"))`, "div,DIV,1,http://www.w3.org/1999/xhtml,urn:other,,false"},
		{"a processing instruction", `(d => { const pi = d.createProcessingInstruction("target", "some data");
			d.appendChild(pi); pi.data += "!";
			return [pi instanceof ProcessingInstruction, pi instanceof CharacterData, pi.nodeType, pi.nodeName, pi.target,
				pi.data,
			pi.nodeValue, pi.textContent, pi.length, pi.ownerDocument === d, d.firstChild === pi, pi.parentNode === d,
			document.createProcessingInstruction("t", "d").nodeType].join() })(document.implementation.createDocument(null,
				null))`,
			"true,true,7,target,target,some data!,some data!,some data!,10,true,true,true,7"},
		{"a processing instruction's names are checked", `["a b", "", "t?>"].map((bad, i) => { try {
			document.createProcessingInstruction(i < 2 ? bad : "t", i < 2 ? "" : bad); return "none" } catch (e) {
				return e.name } }).join()`,
			"InvalidCharacterError,InvalidCharacterError,InvalidCharacterError"},
		{"a CDATA section", `(d => { const c = d.createCDATASection("a < b"); const e = d.createElement("e");
			e.appendChild(c);
			return [c instanceof CDATASection, c instanceof Text, c.nodeType, c.nodeName, c.data, c.length, c.ownerDocument ===
				d,
			e.firstChild === c, e.textContent].join() })(new Document())`, "true,true,4,#cdata-section,a < b,5,true,true,a < b"},
		{"no CDATA section in an HTML document", `[document, document.implementation.createHTMLDocument()].map((d) => {
			try { d.createCDATASection("x"); return "none" } catch (e) { return e.name } }).join()`,
			"NotSupportedError,NotSupportedError"},
		{"what a document holds", `(d => { const out = []; const tried = (f) => { try { f(); out.push("in") } catch (e) {
			out.push(e.name) } };
			tried(() => d.appendChild(d.createComment("c")));
			tried(() => d.appendChild(d.createElement("a")));
			tried(() => d.appendChild(d.createElement("b")));
			tried(() => d.appendChild(d.createTextNode("t")));
			tried(() => d.insertBefore(d.createProcessingInstruction("p", ""), d.firstChild));
			tried(() => d.appendChild(document.implementation.createDocumentType("a", "", "")));
			tried(() => d.removeChild(d.documentElement));
			tried(() => d.appendChild(d.createElement("b")));
			return out.concat([d.childNodes.length, d.documentElement.localName, d.firstChild.nodeType,
				d.lastChild.localName]).join() })(
			document.implementation.createDocument(null, null))`,
			"in,in,HierarchyRequestError,HierarchyRequestError,in,HierarchyRequestError,in,in,3,b,7,b"},
		{"a made HTML document takes a comment after its root", `(d => { d.appendChild(d.createComment("after"));
			return d.childNodes.length + d.lastChild.data })(document.implementation.createHTMLDocument())`, "3after"},
		{"what holds nothing takes nothing", `[document.createTextNode("t"), document.createComment("c"),
			document.doctype].map((n) => {
			try { n.appendChild(document.createElement("p")); return "in" } catch (e) { return e.name } }).join()`,
			"HierarchyRequestError,HierarchyRequestError,HierarchyRequestError"},
		{"cloning a document", `(c => [c instanceof Document, c !== document, c.title, c.getElementById("own").textContent,
			c.getElementById("own") !== document.getElementById("own"), c.body.ownerDocument === c, c.defaultView,
				c.doctype.name,
			c.contentType, document.cloneNode(false).childNodes.length, document.cloneNode().contentType].join())(
			document.cloneNode(true))`, "true,true,Page,own,true,true,,html,text/html,0,text/html"},
		{"cloning a made document", `(d => { d.appendChild(d.createElement("Root")).appendChild(d.createTextNode("t"));
			const c = d.cloneNode(true);
			return [c instanceof XMLDocument, c !== d, c.documentElement.localName, c.documentElement.firstChild.data,
			c.documentElement !== d.documentElement, c.documentElement.ownerDocument === c].join() })(
			document.implementation.createDocument(null, null))`, "true,true,Root,t,true,true"},
		{"a DOMException has its old number", `(() => { try { document.body.appendChild(document.documentElement) } catch
			(e) {
			return [e instanceof DOMException, e.name, e.code, e.code === DOMException.HIERARCHY_REQUEST_ERR,
			new DOMException("m", "NotFoundError").code, new DOMException("m").code, new DOMException("m", "Mine").code].join()
				} })()`,
			"true,HierarchyRequestError,3,true,8,0,0"},
		{"a node's constants", `[Node.ENTITY_REFERENCE_NODE, Node.ENTITY_NODE, Node.NOTATION_NODE, Node.CDATA_SECTION_NODE,
			Node.PROCESSING_INSTRUCTION_NODE, document.DOCUMENT_POSITION_IMPLEMENTATION_SPECIFIC].join()`, "5,6,12,4,7,32"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestNamespacesAreLookedUpFromANode(t *testing.T) {
	s := run(t, `<body><svg id="pic"><g id="g"></g></svg><p id="p">text</p>`)
	for _, c := range []struct{ name, script, want string }{
		{"an HTML element", `(p => [p.lookupNamespaceURI(null), p.lookupNamespaceURI(""), p.lookupNamespaceURI("x"),
			p.isDefaultNamespace("http://www.w3.org/1999/xhtml"), p.isDefaultNamespace("urn:x"), p.isDefaultNamespace(null),
			p.lookupPrefix("http://www.w3.org/1999/xhtml"), p.lookupNamespaceURI("xml"), p.lookupNamespaceURI("xmlns")].join())(
			document.getElementById("p"))`,
			"http://www.w3.org/1999/xhtml,http://www.w3.org/1999/xhtml,,true,false,false,," +
				"http://www.w3.org/XML/1998/namespace,http://www.w3.org/2000/xmlns/"},
		{"an SVG element, a text in it, the document", `[document.getElementById("g").lookupNamespaceURI(null),
			document.getElementById("p").firstChild.lookupNamespaceURI(null), document.lookupNamespaceURI(null),
			document.isDefaultNamespace("http://www.w3.org/1999/xhtml"), document.doctype,
				document.createDocumentFragment().lookupNamespaceURI(null),
			document.createTextNode("t").lookupNamespaceURI(null)].join()`,
			"http://www.w3.org/2000/svg,http://www.w3.org/1999/xhtml,http://www.w3.org/1999/xhtml,true,,,"},
		{"prefixes an element declares", `(d => { const root = d.documentElement, child = root.firstChild.firstChild;
			return [child.lookupNamespaceURI("a"), child.lookupNamespaceURI("b"), child.lookupNamespaceURI(null),
			child.lookupPrefix("urn:a"), child.lookupPrefix("urn:b"), child.lookupPrefix("urn:none"), child.lookupPrefix(null),
			root.lookupNamespaceURI("b"), child.isDefaultNamespace("urn:d"), root.isDefaultNamespace(null),
			child.firstChild.lookupPrefix("urn:a"), d.lookupPrefix("urn:a")].join() })(new DOMParser().parseFromString(
			'<r xmlns:a="urn:a"><m xmlns="urn:d" xmlns:b="urn:b"><c>t</c></m></r>', "application/xml"))`,
			"urn:a,urn:b,urn:d,a,b,,,,true,true,a,a"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestDOMParserReadsADocumentByItsType(t *testing.T) {
	s := run(t, `<body>`)
	for _, c := range []struct{ name, script, want string }{
		{"HTML", `(d => [d instanceof Document, d instanceof XMLDocument, d.contentType, d.title, d.body.firstChild.tagName,
			d.getElementById("a").textContent, d.querySelector("noscript").firstChild.nodeType, d.URL, d.defaultView,
			d.body.ownerDocument === d, d.doctype.name, d.documentElement.getAttribute("lang")].join())(
			new DOMParser().parseFromString('<!DOCTYPE html><html lang="en"><title>T</title>' +
			'<p id="a">one<noscript><i>n</i></noscript>',
			"text/html"))`, "true,false,text/html,T,P,onen,1," + address + ",,true,html,en"},
		{"XML", `(d => [d instanceof XMLDocument, d.contentType, d.documentElement.localName, d.documentElement.namespaceURI,
			d.documentElement.firstChild.nodeType, d.documentElement.firstChild.data, d.documentElement.lastChild.namespaceURI,
			d.documentElement.lastChild.tagName, d.childNodes.length, d.firstChild.target, d.documentElement.ownerDocument === d,
			d.getElementsByTagName("Leaf").length, d.querySelector("Leaf") === d.documentElement.lastChild].join())(
			new DOMParser().parseFromString('<?pi d?><Root><![CDATA[a<b]]><Leaf xmlns="urn:leaf"/></Root>', "application/xml"))`,
			"false,application/xml,Root,,4,a<b,urn:leaf,Leaf,2,pi,true,1,true"},
		{"each XML type", `["text/xml", "application/xml", "application/xhtml+xml", "image/svg+xml"].map((type) =>
			(d => d.contentType + ":" + d.documentElement.constructor.name)(new DOMParser().parseFromString(
			type === "image/svg+xml" ? '<svg xmlns="http://www.w3.org/2000/svg"/>' :
				'<html xmlns="http://www.w3.org/1999/xhtml"/>', type))).join()`,
			"text/xml:HTMLHtmlElement,application/xml:HTMLHtmlElement,application/xhtml+xml:HTMLHtmlElement," +
				"image/svg+xml:SVGElement"},
		{"XML that is not well-formed", `(d => [d instanceof XMLDocument, d.documentElement.localName,
			d.getElementsByTagName("parsererror").length, d.documentElement.namespaceURI,
			/line 1, column \d+/.test(d.documentElement.textContent), d.querySelector("b")].join())(
			new DOMParser().parseFromString("<a><b></a>", "text/xml"))`,
			"false,parsererror,1,http://www.mozilla.org/newlayout/xml/parsererror.xml,true,"},
		{"another type", `["text/plain", "", undefined, "TEXT/HTML"].map((type) => { try {
			new DOMParser().parseFromString("<p>", type); return "read" } catch (e) { return e.constructor.name } }).join()`,
			"TypeError,TypeError,TypeError,TypeError"},
		{"XMLSerializer", `(x => [x.serializeToString(new
			DOMParser().parseFromString('<r a="1"><b/>t<![CDATA[<]]><!--c--></r>', "text/xml")),
			x.serializeToString((p => { p.innerHTML = "a<br>b"; p.title = '"<'; return p })(document.createElement("p"))),
			x.serializeToString(document.createTextNode("a<b")),
			x.serializeToString(document.implementation.createDocumentType("html", "p", "s"))].join("|"))(new XMLSerializer())`,
			`<r a="1"><b/>t<![CDATA[<]]><!--c--></r>|<p xmlns="http://www.w3.org/1999/xhtml" title="&quot;&lt;">a<br/>b</p>|` +
				`a&lt;b|<!DOCTYPE html PUBLIC "p" "s">`},
		{"markup in an XML document", `(d => { const r = d.documentElement; r.innerHTML = '<a x="1">t</a><b/>';
			const read = [r.childNodes.length, r.firstChild.localName, r.firstChild.namespaceURI, r.firstChild.ownerDocument
				=== d,
			r.innerHTML, r.lastChild.outerHTML];
			try { r.innerHTML = "<a><b></a>"; read.push("read") } catch (e) { read.push(e.name) }
			try { r.innerHTML = "<br>"; read.push("read") } catch (e) { read.push(e.name) }
			return read.concat([r.childNodes.length]).join() })(new DOMParser().parseFromString('<r xmlns="urn:r"/>',
				"text/xml"))`,
			`2,a,urn:r,true,<a xmlns="urn:r" x="1">t</a><b xmlns="urn:r"/>,<b xmlns="urn:r"/>,SyntaxError,SyntaxError,2`},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

// asked are the addresses a transport was asked for, which it answers
// with nothing.
type asked struct {
	sync.Mutex
	addresses []string
}

func (a *asked) RoundTrip(q *http.Request) (*http.Response, error) {
	a.Lock()
	a.addresses = append(a.addresses, q.URL.String())
	a.Unlock()
	return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{},
		Body:    io.NopCloser(strings.NewReader("")),
		Request: q}, nil
}

func (a *asked) all() string {
	a.Lock()
	defer a.Unlock()
	return strings.Join(a.addresses, " ")
}

func TestADocumentAScriptMakesFetchesAndRunsNothing(t *testing.T) {
	network := &asked{}
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	loader := &web.Loader{Client: fetch.New(network), Scripts: true}
	s.page = loader.Local(address, `<body><div id="shown"><img id="pic" src="/shown.png"></div>`, web.Links{
		Open: func(string) {}, Over: func(string) {}})
	rt, err := dom.New(s.page, dom.Host{
		Console:  func(m dom.Message) { s.console = append(s.console, m) },
		Viewport: func() (float32, float32) { return 800, 600 },
		Fetch:    s.fetch,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.rt = rt
	t.Cleanup(rt.Close)
	s.layout()
	rt.Start()
	s.layout()
	s.frames("the page's own picture is asked for", func() bool { return strings.Contains(network.all(), "/shown.png") })
	before := network.all()

	for _, c := range []struct{ name, script, want string }{
		{"a script put in it", `(d => { const code = d.createElement("script"); code.textContent = "window.ran = 1";
			d.body.appendChild(code); const far = d.createElement("script"); far.src = "/far.js"; d.head.appendChild(far);
			return String(window.ran) })(document.implementation.createHTMLDocument())`, "undefined"},
		{"scripts and handlers read into it", `(d => { d.body.firstChild.click(); return String(window.parsed) +
			String(window.clicked) })(
			new DOMParser().parseFromString('<p onclick="window.clicked = 1">x</p><script>window.parsed = 1</` + `script>' +
			'<script src="/parsed.js"></` + `script><img src="/parsed.png"><link rel="stylesheet" href="/parsed.css">',
				"text/html"))`,
			"undefinedundefined"},
		{"a picture made in it", `(d => { const img = d.createElement("img"); img.src = "/made.png"; d.body.appendChild(img);
			img.getBoundingClientRect(); img.offsetWidth; img.focus(); img.scrollIntoView();
			return img.getBoundingClientRect().width })(document.implementation.createHTMLDocument())`, "0"},
		{"a sheet and a frame made in it", `(d => { const link = d.createElement("link"); link.rel = "stylesheet"; link.href
			= "/made.css";
			d.head.appendChild(link); const frame = d.createElement("iframe"); frame.src = "/made.html";
				d.body.appendChild(frame);
			const style = d.createElement("style"); style.textContent =
				"@import url(/imported.css); p { background: url(/bg.png) }";
			d.head.appendChild(style); return d.styleSheets.length })(document.implementation.createHTMLDocument())`, "0"},
		{"the page's shown elements moved into it", `(d => { const shown = document.getElementById("shown");
			shown.getBoundingClientRect(); d.body.appendChild(shown);
			const img = document.createElement("img"); img.src = "/moved.png"; shown.appendChild(img);
			shown.firstChild.src = "/changed.png"; shown.innerHTML += '<img src="/markup.png">';
			return [shown.ownerDocument === d, document.getElementById("shown"), shown.getBoundingClientRect().width].join() })(
			document.implementation.createHTMLDocument())`, "true,,0"},
		{"an XML document's", `(d => { const img = d.createElementNS("http://www.w3.org/1999/xhtml", "img");
			img.setAttribute("src", "/xml.png"); d.documentElement.appendChild(img); img.getBoundingClientRect();
			const code = d.createElementNS("http://www.w3.org/1999/xhtml", "script"); code.textContent = "window.xml = 1";
			d.documentElement.appendChild(code); return String(window.xml) })(new DOMParser().parseFromString(
			'<html xmlns="http://www.w3.org/1999/xhtml"><script>window.xml = 2</` + `script><img src="/read.png"/></html>',
			"application/xhtml+xml"))`, "undefined"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	for range 20 {
		s.rt.Frame()
		s.layout()
	}
	if after := network.all(); after != before {
		t.Errorf("the documents a script made asked for %s", strings.TrimPrefix(after, before))
	}
	if len(s.fetched) != 0 {
		t.Errorf("they had the page fetch %+v", s.fetched)
	}
	if got := s.eval(`(img => { img.src = "/back.png"; document.body.appendChild(img); return img.ownerDocument ===
		document })(
		document.implementation.createHTMLDocument().createElement("img"))`); got != "true" {
		t.Errorf("a picture taken back into the page is the page's: %s", got)
	}
	s.frames("a picture adopted by the page is asked for", func() bool {
		return strings.Contains(network.all(),
			"/back.png")
	})
	s.quiet()
}

// xmlPage is a page sent as that media type with its scripts running.
func xmlPage(t *testing.T, media, source string) *scripted {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", media)
		_, _ = io.WriteString(w, source)
	}))
	t.Cleanup(srv.Close)
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	loader := &web.Loader{Client: fetch.New(nil), Scripts: true}
	s.page = loader.Load(t.Context(), srv.URL+"/page.xht", web.Links{Open: func(string) {}, Over: func(string) {}})
	if s.page.Err != nil {
		t.Fatal(s.page.Err)
	}
	rt, err := dom.New(s.page, dom.Host{
		Console:  func(m dom.Message) { s.console = append(s.console, m) },
		Viewport: func() (float32, float32) { return 800, 600 },
		Fetch:    s.fetch,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.rt = rt
	t.Cleanup(rt.Close)
	s.layout()
	rt.Start()
	s.layout()
	return s
}

func TestAPageOfXHTMLIsAnXMLDocumentToItsScripts(t *testing.T) {
	s := xmlPage(t, "application/xhtml+xml", `<?xml version="1.0"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:ev="urn:ev"><head><title>X</title>
<script type="text/javascript"><![CDATA[
  window.early = 1 < 2 && document.readyState;
  document.write("<p id='written'>w</p>");
]]></script><script>window.plain = 1 &lt; 2;</script></head>
<body><p id="late" Data-Mixed="m">text<![CDATA[ & more]]></p><DIV id="upper"/><div id="box"/></body></html>`)
	for _, c := range []struct{ name, script, want string }{
		{"its scripts ran, their text read as XML's", `[window.early, window.plain,
			document.getElementById("written")].join()`,
			"loading,true,"},
		{"the document", `[document.contentType, document instanceof XMLDocument, document instanceof Document,
			document.title,
			document.doctype.publicId, document.doctype.systemId, document.documentElement.tagName, document.body.tagName,
			document.compatMode, document.defaultView === window].join()`,
			"application/xhtml+xml,false,true,X,-//W3C//DTD XHTML 1.0 Strict//EN," +
				"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd,html,body,CSS1Compat,true"},
		{"names as they were written", `(p => [p.tagName, p.localName, p.getAttribute("Data-Mixed"),
			p.getAttribute("data-mixed"),
			p.hasAttribute("DATA-MIXED"), document.getElementById("upper").tagName, document.getElementById("upper") instanceof
				HTMLElement,
			document.getElementById("upper").namespaceURI].join())(document.getElementById("late"))`,
			"p,p,m,,false,DIV,true,http://www.w3.org/1999/xhtml"},
		{"a CDATA section is a node of its own", `(p => [p.childNodes.length, p.lastChild.nodeType, p.lastChild instanceof
			CDATASection,
			p.lastChild.data, p.textContent, p.firstChild.nodeType].join())(document.getElementById("late"))`,
			"2,4,true, & more,text & more,3"},
		{"createElement keeps the case, in XHTML's namespace", `(e => [e.localName, e.tagName, e.namespaceURI, e instanceof
			HTMLElement,
			document.createElement("div") instanceof HTMLDivElement, document.createElement("div").tagName].join())(
			document.createElement("Div"))`, "Div,Div,http://www.w3.org/1999/xhtml,true,true,div"},
		{"attributes keep theirs", `(e => { e.setAttribute("viewBox", "1"); return [e.getAttribute("viewBox"),
			e.getAttribute("viewbox"),
			e.getAttributeNames().join("+")].join() })(document.createElement("div"))`, "1,,viewBox"},
		{"markup is read as XML", `(box => { box.innerHTML =
			'<p>a<br/>b &amp; c</p><svg xmlns="http://www.w3.org/2000/svg"><g/></svg>';
			return [box.childNodes.length, box.firstChild.namespaceURI, box.firstChild.childNodes.length,
				box.firstChild.textContent,
			box.lastChild.namespaceURI, box.lastChild.firstChild.localName].join() })(document.getElementById("box"))`,
			"2,http://www.w3.org/1999/xhtml,3,ab & c,http://www.w3.org/2000/svg,g"},
		{"and written as XML", `document.getElementById("box").innerHTML`,
			`<p xmlns="http://www.w3.org/1999/xhtml">a<br/>b &amp; c</p><svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`},
		{"markup that is not well-formed is refused whole", `(box => { const tries = ["<p>open", "a<br>b", "&nbsp;x",
			"<x:y/>", "<ev:y/>"].map((markup) => {
			try { box.innerHTML = markup; return "read" } catch (e) { return e.name } });
			return tries.concat([box.childNodes.length, box.lastChild.namespaceURI]).join() })(document.getElementById("box"))`,
			"SyntaxError,SyntaxError,read,SyntaxError,read,1,urn:ev"},
		{"outerHTML and insertAdjacentHTML", `(box => { box.innerHTML = "<i/>"; box.firstChild.outerHTML = "<b>o</b><u/>";
			box.insertAdjacentHTML("beforeend", "<s/>"); return [box.innerHTML.replace(/ xmlns="[^"]*"/g, ""),
			box.firstChild.outerHTML].join() })(document.getElementById("box"))`,
			`<b>o</b><u/><s/>,<b xmlns="http://www.w3.org/1999/xhtml">o</b>`},
		{"createCDATASection and a processing instruction", `(box => { box.textContent = "";
			box.appendChild(document.createCDATASection("<x>"));
			return [box.firstChild.nodeType, box.innerHTML, document.createProcessingInstruction("t", "d").target].join() })(
			document.getElementById("box"))`, "4,<![CDATA[<x>]]>,t"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.eval(`document.getElementById("box").innerHTML = '<span id="made" style="display:block;width:50px;height:20px"/>'`)
	s.layout()
	if got := s.eval(`document.getElementById("made").getBoundingClientRect().width`); got != "50" {
		t.Errorf("what markup read as XML made is shown %s wide, want 50", got)
	}
	for _, m := range s.console {
		if m.Level == "error" {
			t.Errorf("the console says %s", m.Text)
		}
	}
}

func TestAnXMLHttpRequestGivesTheDocumentOfItsAnswer(t *testing.T) {
	s := run(t, `<body>`)
	s.answers["https://site.test/doc.xml"] = dom.Response{Status: 200,
		Header: map[string]string{"Content-Type": "text/xml"},
		Body:   []byte(`<list><item id="a">one</item></list>`)}
	s.answers["https://site.test/doc.html"] = dom.Response{Status: 200,
		Header: map[string]string{"Content-Type": "text/html"},
		Body:   []byte(`<title>T</title><p id="a">one</p>`)}
	s.answers["https://site.test/broken.xml"] = dom.Response{Status: 200,
		Header: map[string]string{"Content-Type": "application/xml"}, Body: []byte(`<list><item></list>`)}
	s.answers["https://site.test/doc.txt"] = dom.Response{Status: 200,
		Header: map[string]string{"Content-Type": "text/plain"},
		Body:   []byte(`<list/>`)}
	s.eval(`window.got = {}; window.ask = (name, path, type) => { const x = new XMLHttpRequest(); x.open("GET", path);
		if (type) x.responseType = type;
		x.onload = () => { try { window.got[name] = [x.responseXML, x.response] } catch (e) { window.got[name] = e.name } };
			x.send() };
		ask("xml", "/doc.xml"); ask("typed", "/doc.xml", "document"); ask("html", "/doc.html"); ask("page", "/doc.html",
			"document");
		ask("broken", "/broken.xml"); ask("text", "/doc.txt", "document"); ask("json", "/doc.xml", "json"); 0`)
	s.frames("the answers", func() bool { return s.eval(`Object.keys(window.got).length`) == "7" })
	for name, c := range map[string]struct{ script, want string }{
		"XML by default": {`(d => [d instanceof XMLDocument, d.contentType, d.querySelector("#a").textContent, d.URL,
			typeof got.xml[1], d === got.xml[0]].join())(got.xml[0])`,
			"false,text/xml,one,https://site.test/doc.xml,string,true"},
		"XML asked for as a document": {`[got.typed[0] instanceof XMLDocument, got.typed[0] === got.typed[1],
			got.typed[0].documentElement.localName].join()`, "false,true,list"},
		"HTML is no document by default": {`String(got.html[0])`, "null"},
		"HTML asked for as a document": {`(d => [d instanceof XMLDocument, d.contentType, d.title,
			d.getElementById("a").textContent,
			d === got.page[1], d.defaultView].join())(got.page[0])`, "false,text/html,T,one,true,"},
		"XML that is not well-formed": {`String(got.broken[0])`, "null"},
		"a text is no document":       {`String(got.text[0]) + String(got.text[1])`, "nullnull"},
		"responseXML of another type": {`got.json`, "InvalidStateError"},
		"a document of an answer is not the page's": {`got.xml[0].documentElement.ownerDocument === got.xml[0] &&
			document.querySelector("item") === null`, "true"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s: got %s, want %s", name, got, c.want)
		}
	}
}
