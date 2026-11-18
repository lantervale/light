//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import "testing"

const attributing = `const thrown = (fn) => { try { fn(); return "none" } catch (e) { return e.name } };
	const XLINK = "http://www.w3.org/1999/xlink", XMLNS = "http://www.w3.org/2000/xmlns/", XML =
		"http://www.w3.org/XML/1998/namespace";
	const said = (a) => a === null ? "null" : [a.namespaceURI, a.prefix, a.localName, a.name, a.value].join(" ");`

func TestAnAttributeIsANode(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p" class="c" TITLE="t">p</p><svg id="s" viewBox="0 0 1 1"
		xlink:href="#a"></svg>`)
	for _, c := range []struct{ name, script, want string }{
		{"it is the same one while its element has it", `(p => { const a = p.attributes[0], t = p.getAttributeNode("TITLE");
			return [a === p.attributes[0], a === p.getAttributeNode("id"), a === p.attributes.getNamedItem("id"),
			a === p.attributes.id, a === p.getAttributeNodeNS(null, "id"), p.attributes === p.attributes, t ===
			p.attributes[2],
			a instanceof Attr, a instanceof Node, a.nodeType, Node.ATTRIBUTE_NODE, a.nodeName, a.name, a.localName, a.prefix,
			a.namespaceURI, a.value, a.nodeValue, a.textContent, a.specified, a.ownerElement === p, a.ownerDocument ===
			document,
			a.parentNode, a.parentElement, a.childNodes.length, a.firstChild, a.nextSibling, a.isConnected, a.getRootNode()
			=== a,
			t.name, p.getAttributeNode("nope")].join("|") })(document.getElementById("p"))`,
			"true|true|true|true|true|true|true|true|true|2|2|id|id|id|||p|p|p|true|true|true|||0|||false|true|title|"},
		{"its value is its element's", attributing + `(() => { const e = document.createElement("i");
			e.setAttribute("a", "1");
			const a = e.getAttributeNode("a"); a.value = "2"; const seen = [e.getAttribute("a")]; e.setAttribute("a", "3");
			seen.push(a.value); a.nodeValue = "4"; seen.push(e.getAttribute("a")); a.textContent = null;
			seen.push(JSON.stringify(e.getAttribute("a"))); e.setAttribute("a", "5"); e.removeAttribute("a");
			seen.push(a.value, a.ownerElement, e.attributes.length, e.getAttributeNode("a")); a.value = "6";
			seen.push(a.value, e.hasAttribute("a")); e.setAttribute("a", "7");
			seen.push(e.getAttributeNode("a") === a, a.value); return seen.join("|") })()`,
			`2|3|4|""|5||0||6|false|false|6`},
		{"a field follows the attribute a script sets through its node", `(() => { const i = document.createElement("input");
			document.body.append(i); i.setAttribute("value", "a"); i.getAttributeNode("value").value = "b";
			const id = document.getElementById("p").getAttributeNode("id"); id.value = "q";
			const found = [i.value, document.getElementById("p"), document.getElementById("q").textContent]; id.value = "p";
			i.remove(); return found.join() })()`, "b,,p"},
		{"its element's attributes are a map by name and by place", attributing + `(p => { const m = p.attributes;
			return [m.length, m.item(1).name, m.item(9), m[1].name, m[9], m["class"].value, m.TITLE, m.title.value,
			m.getNamedItem("TITLE").name, m.getNamedItemNS(null, "class").value, m.getNamedItemNS("urn:x", "class"),
			Object.getOwnPropertyNames(m), "class" in m, "length" in m, m instanceof NamedNodeMap, [...m].map((a) =>
			a.name),
			Object.keys(m), thrown(() => new NamedNodeMap())].join("|") })(document.getElementById("p"))`,
			"3|class||class||c||t|title|c||0,1,2,id,class,title|true|true|true|id,class,title|0,1,2|TypeError"},
		{"a map adds and takes off", attributing + `(() => { const e = document.createElement("i"), m = e.attributes;
			const a = document.createAttribute("A"); a.value = "1"; const seen = [m.setNamedItem(a), m.length, a.ownerElement
			=== e,
			e.getAttribute("a"), m.setNamedItem(a) === a]; const b = document.createAttribute("a"); b.value = "2";
			seen.push(m.setNamedItem(b) === a, a.ownerElement, a.value, e.getAttribute("a"), m.removeNamedItem("a") === b,
			m.length, thrown(() => m.removeNamedItem("a")), thrown(() => m.removeNamedItemNS(null, "a")));
			const n = document.createAttributeNS("urn:x", "p:n"); m.setNamedItemNS(n);
			seen.push(said(m.getNamedItemNS("urn:x", "n")), m.removeNamedItemNS("urn:x", "n") === n, m.length);
			return seen.join("|") })()`,
			"|1|true|1|true|true||1|2|true|0|NotFoundError|NotFoundError|urn:x p n p:n |true|0"},
		{"an attribute a script makes", attributing + `(() => { const a = document.createAttribute("FoO"), n =
			document.createAttributeNS("urn:x", "p:Loc"), x = document.implementation.createDocument(null, "r")
			.createAttribute("FoO");
			const e = document.createElement("i"), f = document.createElement("b"); a.value = "v";
			const seen = [said(a), said(n), x.name, a.ownerElement, a.ownerDocument === document, e.setAttributeNode(a),
			e.setAttributeNode(a) === a, thrown(() => f.setAttributeNode(a)), e.setAttributeNodeNS(n), e.attributes.length,
			e.getAttributeNS("urn:x", "Loc"), e.removeAttributeNode(a) === a, a.ownerElement, thrown(() =>
			e.removeAttributeNode(a)),
			f.setAttributeNode(a), a.ownerElement === f, f.outerHTML, thrown(() => document.createAttribute("")),
			thrown(() => document.createAttribute("a b")), thrown(() => document.createAttributeNS(null, "p:a")),
			thrown(() => document.createAttributeNS("urn:x", "xmlns:a")), thrown(() => e.setAttributeNode(null))];
			return seen.join("|") })()`,
			"  foo foo v|urn:x p Loc p:Loc |FoO||true||true|InUseAttributeError||2||true||NotFoundError||true|" +
				`<b foo="v"></b>|InvalidCharacterError|InvalidCharacterError|NamespaceError|NamespaceError|TypeError`},
		{"attributes of a namespace", attributing + `(() => { const e = document.createElement("i");
			e.setAttributeNS("urn:a", "foo", "1"); e.setAttributeNS("urn:b", "q:foo", "2"); e.setAttribute("foo", "3");
			e.setAttributeNS("urn:b", "r:foo", "4"); e.setAttributeNS(null, "FOO", "5"); e.setAttribute("FOO", "6");
			e.setAttributeNS(XLINK, "xlink:href", "h"); e.setAttributeNS(XML, "xml:lang", "en");
			const seen = [[...e.attributes].map(said).join(), e.getAttribute("foo"), e.getAttribute("q:foo"),
			e.getAttributeNS("urn:b", "foo"), e.getAttributeNS("", "FOO"), e.getAttributeNS(null, "foo"), e.hasAttributeNS(
			"urn:a", "foo"), e.hasAttributeNS("urn:c", "foo"), e.getAttributeNames(), e.getAttribute("xlink:href"),
			e.getAttributeNS(XLINK, "href"), e.getAttributeNodeNS(XML, "lang").name];
			e.removeAttributeNS("urn:b", "foo"); e.removeAttributeNS(null, "FOO"); e.removeAttributeNS(XLINK, "href");
			seen.push([...e.attributes].map((a) => a.name), thrown(() => e.setAttributeNS(null, "a:b", "")),
			thrown(() => e.setAttributeNS("urn:x", "xml:b", "")), thrown(() => e.setAttributeNS("urn:x", "", "")),
			thrown(() => e.setAttribute("", "")), thrown(() => e.setAttributeNS(XMLNS, "x", "")));
			return seen.join("|") })()`,
			"urn:a  foo foo 6,urn:b q foo q:foo 4,  FOO FOO 5,http://www.w3.org/1999/xlink xlink href xlink:href h," +
				"http://www.w3.org/XML/1998/namespace xml lang xml:lang en|6|4|4|5||true|false|" +
				"foo,q:foo,FOO,xlink:href,xml:lang|h|h|xml:lang|foo,xml:lang|NamespaceError|NamespaceError|" +
				"InvalidCharacterError|InvalidCharacterError|NamespaceError"},
		{"what the page's markup named", attributing + `(s => [...s.attributes].map(said).join())(
			document.getElementById("s"))`,
			"  id id s,  viewBox viewBox 0 0 1 1,http://www.w3.org/1999/xlink xlink href xlink:href #a"},
		{"attributes go with their element to another document", attributing + `(() => { const d =
			document.implementation.createHTMLDocument(), e = document.createElement("i"); e.setAttribute("a", "1");
			const a = e.getAttributeNode("a"), lone = document.createAttribute("l"); d.body.append(e);
			const seen = [a.ownerDocument === d, a.ownerElement === e, d.adoptNode(lone) === lone, lone.ownerDocument === d];
			const taken = document.adoptNode(a); seen.push(taken === a, a.ownerElement, a.ownerDocument === document,
			e.hasAttribute("a"), a.value); const copy = d.importNode(a), same = a.cloneNode(true);
			seen.push(copy !== a, said(copy), copy.ownerDocument === d, same.ownerDocument === document, same.isEqualNode(a),
			a.isEqualNode(document.createAttribute("a")), copy.isEqualNode(e));
			const el = document.createElement("u"); el.setAttributeNS("urn:x", "p:n", "v"); const cl = el.cloneNode();
			seen.push(said(cl.attributes[0]), cl.attributes[0] !== el.attributes[0], cl.isEqualNode(el));
			cl.setAttribute("z", ""); seen.push(cl.isEqualNode(el)); return seen.join("|") })()`,
			"true|true|true|true|true||true|false|1|true|  a a 1|true|true|true|false|false|urn:x p n p:n v|true|true|false"},
		{"a change of an attribute is told with its name and namespace", `(() => { const e = document.createElement("i");
			const o = new MutationObserver(() => {}); o.observe(e, { attributes: true, attributeOldValue: true });
			e.setAttributeNS("urn:x", "p:n", "1"); e.getAttributeNodeNS("urn:x", "n").value = "2"; e.removeAttributeNS(
			"urn:x", "n"); const a = document.createAttribute("b"); a.value = "3"; e.setAttributeNode(a);
			e.removeAttributeNode(a); a.value = "4";
			return o.takeRecords().map((r) => [r.type, r.attributeName, r.attributeNamespace, r.oldValue].join(":"))
			.join() })()`,
			"attributes:n:urn:x:,attributes:n:urn:x:1,attributes:n:urn:x:2,attributes:b::,attributes:b::3"},
		{"where an attribute stands", `(() => { const e = document.createElement("i"), k = document.createElement("b");
			e.append(k); e.setAttribute("a", ""); e.setAttribute("b", ""); const [a, b] = e.attributes;
			return [a.compareDocumentPosition(b), b.compareDocumentPosition(a), a.compareDocumentPosition(e),
			e.compareDocumentPosition(a), a.compareDocumentPosition(k), a.compareDocumentPosition(a), a.contains(a),
			e.contains(a), a.lookupNamespaceURI(null), a.isDefaultNamespace("http://www.w3.org/1999/xhtml")].join() })()`,
			"36,34,10,20,4,0,true,false,http://www.w3.org/1999/xhtml,true"},
		{"the first attribute of a name is the one changed and taken off", attributing + `(() => { const e =
			document.createElement("i"); e.setAttribute("foo", "first"); e.setAttributeNS("urn:x", "foo", "second");
			e.removeAttribute("foo"); const seen = [e.attributes.length, e.getAttribute("foo"), e.getAttributeNS("urn:x",
			"foo")]; const old = document.createAttributeNS("urn:x", "p:foo"); old.value = "third";
			seen.push(said(e.setAttributeNodeNS(old)), said(e.attributes[0]), e.getAttribute("p:foo"), e.getAttribute("foo"),
			thrown(() => e.toggleAttribute("", false)), thrown(() => e.toggleAttribute("a b")), e.toggleAttribute("'"),
			e.toggleAttribute("Ä"), e.hasAttribute("Ä"), e.hasAttribute("ä"), e.getAttributeNames());
			return seen.join("|") })()`,
			"1|second|second|urn:x  foo foo second|urn:x p foo p:foo third|third||InvalidCharacterError|" +
				"InvalidCharacterError|true|true|true|false|p:foo,',Ä"},
		{"nodes are equal by what they are, hold and say", `(() => { const d = document.implementation, e = (ns, name,
			attrs = [], kids = []) => { const n = document.createElementNS(ns, name);
			for (const [space, key, value] of attrs) n.setAttributeNS(space, key, value); n.append(...kids); return n };
			const pairs = [[d.createDocumentType("a", "p", "s"), d.createDocumentType("a", "p", "s")],
			[d.createDocumentType("a", "p", "s"), d.createDocumentType("b", "p", "s")],
			[d.createDocumentType("a", "p", "s"), d.createDocumentType("a", "p", "t")],
			[e("urn:x", "p:a"), e("urn:x", "p:a")], [e("urn:x", "p:a"), e("urn:y", "p:a")], [e("urn:x", "p:a"), e("urn:x",
			"q:a")],
			[e(null, "a", [[null, "b", "1"], ["urn:x", "c", "2"]]), e(null, "a", [["urn:x", "c", "2"], [null, "b", "1"]])],
			[e(null, "a", [["urn:x", "b", "1"]]), e(null, "a", [["urn:y", "b", "1"]])],
			[e(null, "a", [["urn:x", "p:b", "1"]]), e(null, "a", [["urn:x", "q:b", "1"]])],
			[e(null, "a", [], ["t", e(null, "k")]), e(null, "a", [], ["t", e(null, "k")])],
			[e(null, "a", [], ["t", e(null, "k")]), e(null, "a", [], ["t", e(null, "j")])],
			[e(null, "a", [], ["t"]), e(null, "a", [], ["t", "u"])],
			[document.createProcessingInstruction("t", "d"), document.createProcessingInstruction("t", "d")],
			[document.createProcessingInstruction("t", "d"), document.createProcessingInstruction("u", "d")],
			[document.createComment("c"), document.createComment("c")], [document.createComment("c"), document.createTextNode(
			"c")], [document.createDocumentFragment(), document.createDocumentFragment()]];
			return pairs.map(([a, b]) => a.isEqualNode(b)).concat(document.body.isEqualNode(null)).join() })()`,
			"true,false,false,true,false,false,true,false,true,true,false,false,true,false,true,false,true,false"},
		{"a namespace is looked up through the attributes that declare it", attributing + `(() => { const e =
			document.createElementNS("fooNamespace", "prefix:elem"), k = document.createElementNS("childNamespace",
			"childElem");
			e.setAttributeNS(XMLNS, "xmlns:baz", "bazURI"); e.setAttributeNS(XMLNS, "xmlns", "defaultURI");
			e.setAttribute("xmlns:plain", "notOne"); e.append(k);
			return [e.lookupNamespaceURI(null), e.lookupNamespaceURI("prefix"), e.lookupNamespaceURI("baz"),
			e.lookupNamespaceURI("plain"), k.lookupNamespaceURI(null), k.lookupNamespaceURI("baz"), k.lookupNamespaceURI(
			"prefix"), e.isDefaultNamespace("defaultURI"), e.isDefaultNamespace("fooNamespace"), e.lookupPrefix("bazURI"),
			e.lookupPrefix("fooNamespace"), k.lookupPrefix("fooNamespace"), e.lookupPrefix("notOne")].join() })()`,
			"defaultURI,fooNamespace,bazURI,,childNamespace,bazURI,fooNamespace,true,false,baz,prefix,prefix,"},
		{"a name of HTML keeps what is not ASCII as it is written", `(() => { const e = document.createElement("İnpuT"),
			c = document.createElement("f:o:o"), k = document.createElement("marK");
			e.setAttribute("DÄta-X", "1"); return [e.localName, e.tagName, e.getAttributeNames(), e.getAttribute("dÄta-x"),
			e.getAttribute("däta-x"), c.localName, c.prefix, c.tagName, c.namespaceURI, c instanceof HTMLUnknownElement,
			k.localName === "marK", c.outerHTML].join() })()`,
			"İnput,İNPUT,dÄta-x,1,,f:o:o,,F:O:O,http://www.w3.org/1999/xhtml,true,true,<f:o:o></f:o:o>"},
		{"an attribute of a namespace is not its element's own", `(() => { const ns = "urn:x", i =
			document.createElement("input"), d = document.createElement("div"), l = document.createElement("i");
			i.setAttributeNS(ns, "type", "file"); i.setAttributeNS(ns, "disabled", ""); d.setAttributeNS(ns, "style",
			"color: red"); d.setAttributeNS(ns, "id", "spaced"); d.setAttributeNS(null, "style", "color: green");
			l.setAttributeNS(ns, "class", "a b"); document.body.append(d);
			const seen = [i.type, i.disabled, d.style.color, d.id, document.querySelector("#spaced"),
			document.querySelector("[style*=red]"), l.classList.length, l.className, d.getAttribute("style")]; d.remove();
			return seen.join() })()`, "text,false,green,,,,0,,color: red"},
		{"a name of HTML is in capitals where ASCII has them", `[document.createElement("ınput").tagName,
			document.createElement("x-É").tagName, (() => { try { document.createProcessingInstruction("\ud800", "x") }
			catch (e) { return e.name } })(), document.getElementsByTagName("BODY").length].join()`,
			"ıNPUT,X-É,InvalidCharacterError,1"},
		{"it holds nothing", attributing + `(a => [thrown(() => a.appendChild(document.createElement("i"))),
			thrown(() => document.body.appendChild(a)), thrown(() => a.removeChild(a)), a.hasChildNodes(),
			thrown(() => new Attr())].join())(document.createAttribute("a"))`,
			"HierarchyRequestError,HierarchyRequestError,NotFoundError,false,TypeError"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestADocumentOfXMLKeepsThePrefixesItWasWrittenWith(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body>`)
	const parse = attributing + `const parse = (source) => new DOMParser().parseFromString(source, "text/xml");
		const write = (node) => new XMLSerializer().serializeToString(node);`
	for _, c := range []struct{ name, script, want string }{
		{"its elements and attributes say theirs", parse + `(d => { const r = d.documentElement, c = r.firstChild, e =
			r.lastChild;
			return [r.prefix, r.localName, r.tagName, r.nodeName, r.namespaceURI, c.prefix, c.tagName, e.tagName,
			[...r.attributes].map(said).join(), said(c.attributes[0]), d.getElementsByTagName("p:e").length,
			d.getElementsByTagName("e").length, d.getElementsByTagNameNS("urn:p", "e").length, r.cloneNode(true).lastChild
			.tagName, d.importNode(e).prefix, r.lookupPrefix("urn:p"), c.lookupNamespaceURI("p")].join("|") })(
			parse('<p:r xmlns:p="urn:p" xmlns="urn:d" p:a="1" b="2"><c p:d="3"/><p:e/></p:r>'))`,
			"p|r|p:r|p:r|urn:p||c|p:e|http://www.w3.org/2000/xmlns/ xmlns p xmlns:p urn:p,http://www.w3.org/2000/xmlns/  " +
				"xmlns xmlns urn:d,urn:p p a p:a 1,  b b 2|urn:p p d p:d 3|1|0|1|p:e|p|p|urn:p"},
		{"it is written back as it was", parse + `['<p:r xmlns:p="urn:p" xmlns="urn:d" p:a="1" b="2"><c p:d="3"/><p:e/>' +
			'</p:r>',
			'<r xmlns:xx="uri"><b/></r>', '<root xmlns:x="uri1"><table xmlns="uri1"/></root>'].map((source) =>
			write(parse(source)) === source).join()`, "true,true,false"},
		{"a namespace a script gives is declared where it is written", parse + `(() => { const r = parse(
			'<r xmlns:xx="uri"><b/></r>').documentElement; r.firstChild.setAttributeNS("uri", "p:name", "v");
			r.setAttributeNS("uri2", "xx:other", "w"); const made = new Document().createElementNS("uri1", "p:root");
			made.setAttributeNS(XMLNS, "xmlns:p", "uri2"); made.appendChild(made.ownerDocument.createElementNS("uri3",
			"q:kid"));
			const x = r.ownerDocument.createElementNS(XML, "foo"); return [write(r), write(made), write(x),
			write(document.createElementNS("urn:x", "a:b")), document.createElementNS("urn:x", "a:b").outerHTML,
			write(document.createAttribute("a")), write(parse('<root xmlns:p1="u1"><child xmlns:p2="u1"/></root>')
			.documentElement.firstChild.appendChild(document.createElementNS("u1", "child2")).appendChild(
			document.createElementNS("u1", "grandchild")).parentNode.parentNode.parentNode)].join("|") })()`,
			`<r xmlns:xx="uri" xmlns:ns1="uri2" ns1:other="w"><b xx:name="v"/></r>|` +
				`<ns1:root xmlns:ns1="uri1" xmlns:p="uri2"><q:kid xmlns:q="uri3"/></ns1:root>|<xml:foo/>|` +
				`<a:b xmlns:a="urn:x"/>|<a:b></a:b>||` +
				`<root xmlns:p1="u1"><child xmlns:p2="u1"><p2:child2><p2:grandchild/></p2:child2></child></root>`},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestXPathWalksTheNodesTheDOMHas(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><div id="c" foo="bar" data-x="1"><span lang="en">t</span></div>`)
	const all = `const all = (expression, context, resolver = null, d = document) => { const r = d.evaluate(expression,
		context, resolver, 7), out = []; for (let i = 0; i < r.snapshotLength; i++) out.push(r.snapshotItem(i));
		return out };
		const thrown = (fn) => { try { fn(); return "none" } catch (e) { return e.name } };`
	for _, c := range []struct{ name, script, want string }{
		{"an attribute it finds is its element's own", all + `(c => [all("@foo", c)[0] === c.getAttributeNode("foo"),
			all("//@*", document).every((a) => a === a.ownerElement.getAttributeNode(a.name)), all("@*", c).length,
			all("/", c.getAttributeNode("foo"))[0] === document, all("//div[@foo]", c.getAttributeNode("foo"))[0] === c,
			all("..", c.attributes[1])[0] === c, all("/", document.createAttribute("lone")).length]
			.join())(document.getElementById("c"))`, "true,true,3,true,true,true,0"},
		{"a name is said with its prefix, and a language whatever its ASCII case", all + `(() => { const e =
			document.createElementNS("urn:x", "alpha:beta"); e.setAttributeNS("urn:y", "p:q", "");
			e.setAttributeNS("http://www.w3.org/XML/1998/namespace", "xml:lang", "\u212Ao");
			const text = (x, n) => document.evaluate(x, n, null, 2).stringValue;
			return [text("name()", e), text("local-name()", e), text("name(@*)", e), text("namespace-uri(@*)", e),
			document.evaluate('lang("ko")', e, null, 3).booleanValue, document.evaluate('lang("EN")',
			document.querySelector("span"), null, 3).booleanValue].join() })()`,
			"alpha:beta,beta,p:q,urn:y,false,false"},
		{"what resolves a prefix is asked as a script would be", all + `(() => { let asked = 0;
			const none = all("/foo:bar", document.documentElement, () => { asked++; return "" }).length;
			return [none, asked, thrown(() => all("/foo:bar", document, () => null)), thrown(() => all("/foo:bar", document,
			null)), all("//h:p", document, document.createElementNS("urn:r", "h:r")).length].join() })()`,
			"0,1,NamespaceError,NamespaceError,0"},
		{"a document a script made is walked from its own root", all + `(() => { const d =
			document.implementation.createHTMLDocument("t"); d.body.innerHTML = '<p id="a">one</p><p>two</p>';
			return [all("//p", d, null, d).length, all("/", d.body, null, d)[0] === d, all("id('a')", d, null, d)[0] ===
			d.getElementById("a"), all("//p", d.body, null, document).length, d.evaluate("string(//p[2])", d, null, 2)
			.stringValue, d.evaluate("count(/html/body/*)", d, null, 1).numberValue].join() })()`, "2,true,true,2,two,2"},
		{"a document of XML is walked by its names as they are written", all + `(() => { const d =
			new DOMParser().parseFromString('<a xmlns:p="urn:p"><B x="1" p:y="2"/><b>t</b><p:e/></a>', "text/xml");
			const n = (x, resolver = null) => all(x, d, resolver, d).length;
			return [n("//B"), n("//b"), n("//*"), n("/a/b/text()"), n("//@x"), n("//@X"), n("//p:e", () => "urn:p"),
			n("//e"), n("//@p:y", { lookupNamespaceURI: () => "urn:p" }), n("//*[local-name()='e']"),
			d.evaluate("name(/a/*[3])", d, null, 2).stringValue, d.evaluate("string(/a)", d, null, 2).stringValue,
			d.evaluate("count(//node())", d, null, 1).numberValue].join() })()`, "1,1,4,1,1,0,1,0,1,1,p:e,t,5"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestXPathSaysWhatItsResolverThrewAndNamesNoNamespace(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body>`)
	got := s.eval(`(() => { const heard = []; addEventListener("error", (e) => heard.push(e.error && e.error.name ||
		String(e.error))); const thrown = (resolver) => { try { document.evaluate("/foo:bar", document, resolver) }
		catch (e) { return e.name } };
		return [thrown({ lookupNamespaceURI: 5 }), thrown({}), thrown(() => { throw new RangeError("said") }),
		thrown(() => Symbol()), thrown(() => ({ toString() { throw new EvalError("said") } }))].concat(heard).join() })()`)
	want := "NamespaceError,NamespaceError,NamespaceError,NamespaceError,NamespaceError," +
		"TypeError,TypeError,RangeError,TypeError,EvalError"
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}
