//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	_ "embed"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// fidelity is a page with what ui shows nothing of among what it
// shows: comments, an SVG, scripts, styles, a noscript, a template,
// elements ui has none for, and whitespace everywhere.
//
//go:embed testdata/fidelity.html
var fidelity string

// fidelityInChrome is fidelity as headless Chrome writes it back
// (--dump-dom): its doctype, then its html element's outerHTML.
//
//go:embed testdata/fidelity.chrome.html
var fidelityInChrome string

// outline writes the tree under n as its parser made it, a node a
// line: what scripts must see. An element's attributes are by name,
// since the parser sorts those of some (a, b, i…) where a browser and
// light keep them as written, which Chrome's own writing checks.
func outline(out *strings.Builder, n *html.Node, depth int) {
	pad := strings.Repeat(" ", max(depth, 0))
	switch n.Type { //nolint:exhaustive // a document's children are these
	case html.ElementNode:
		attributes := make([]string, len(n.Attr))
		for i, a := range n.Attr {
			name := a.Key
			if a.Namespace != "" {
				name = a.Namespace + ":" + name
			}
			attributes[i] = " " + name + "=" + quoted(a.Val)
		}
		slices.Sort(attributes)
		out.WriteString(pad + "<" + n.Data + strings.Join(attributes, "") + ">\n")
	case html.TextNode:
		out.WriteString(pad + "#text " + quoted(n.Data) + "\n")
	case html.CommentNode:
		out.WriteString(pad + "#comment " + quoted(n.Data) + "\n")
	case html.DoctypeNode:
		out.WriteString(pad + "#doctype " + n.Data + "\n")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		outline(out, c, depth+1)
	}
}

// quoted is text on one line, as outlined writes it too.
func quoted(text string) string {
	return `"` + strings.NewReplacer("\n", `\n`, "\r", `\r`).Replace(text) + `"`
}

func parsed(t *testing.T, markup string) string {
	t.Helper()
	root, err := html.ParseWithOptions(strings.NewReader(markup), html.ParseOptionEnableScripting(true))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	outline(&out, root, -1)
	return out.String()
}

// outlined is the page as a script walks it from the document through
// childNodes, written as outline writes it, a template's content in
// its place. A line beginning with !! is a node whose links disagree.
const outlined = `(() => {
  const quote = (s) => '"' + s.replace(/\n/g, "\\n").replace(/\r/g, "\\r") + '"';
  let out = "";
  const walk = (n, depth) => {
    const pad = " ".repeat(Math.max(depth, 0));
    if (n.nodeType === 1) {
      out += pad + "<" + n.localName +
        Array.from(n.attributes, (a) => " " + a.name + "=" + quote(a.value)).sort().join("") + ">\n";
      if (n.nodeName !==
          (n.namespaceURI === "http://www.w3.org/1999/xhtml" ? n.localName.toUpperCase() : n.localName)) {
        out += "!! " + n.localName + " is named " + n.nodeName + "\n";
      }
    } else if (n.nodeType === 3) out += pad + "#text " + quote(n.data) + "\n";
    else if (n.nodeType === 8) out += pad + "#comment " + quote(n.data) + "\n";
    else if (n.nodeType === 10) out += pad + "#doctype " + n.name + "\n";
    const holder = n.localName === "template" && n.content ? n.content : n;
    const kids = holder.childNodes;
    if (n.localName === "template" && n.childNodes.length) out += "!! a template has children\n";
    if (holder.firstChild !== (kids[0] || null) || holder.lastChild !== (kids[kids.length - 1] || null) ||
      holder.hasChildNodes() !== kids.length > 0) out += "!! the ends of " + n.nodeName + "\n";
    kids.forEach((c, i) => {
      if (c.parentNode !== holder) out += "!! the parent of " + c.nodeName + " in " + n.nodeName + "\n";
      if (c.previousSibling !== (kids[i - 1] || null) || c.nextSibling !== (kids[i + 1] || null)) {
        out += "!! the siblings of " + c.nodeName + " in " + n.nodeName + "\n";
      }
      if (c.ownerDocument !== document || c.isConnected !== (holder === n && n.isConnected)) {
        out += "!! the document of " + c.nodeName + " in " + n.nodeName + "\n";
      }
      walk(c, depth + 1);
    });
  };
  walk(document, -1);
  return out;
})()`

// differences are the lines two outlines differ by, the first few.
func differences(got, want string) string {
	has := map[string]int{}
	for _, line := range strings.Split(want, "\n") {
		has[line]++
	}
	var out []string
	for _, line := range strings.Split(got, "\n") {
		if has[line] == 0 {
			out = append(out, "+ "+line)
		}
		has[line]--
	}
	for line, left := range has {
		if left > 0 {
			out = append(out, "- "+line)
		}
	}
	return strings.Join(out[:min(len(out), 12)], "\n")
}

func TestScriptsSeeThePageAsItsParserMadeIt(t *testing.T) {
	s := run(t, fidelity)
	want := parsed(t, fidelity)
	if got := s.eval(outlined); got != want {
		t.Errorf("scripts see the page apart from its parser's tree:\n%s", differences(got, want))
	}
	for _, c := range []struct{ script, want string }{
		{`[document.childNodes.length, document.firstChild === document.doctype, document.doctype.name,
			document.childNodes[1].data, document.lastChild.nodeType, document.documentElement.parentNode === document,
			document.doctype.parentNode === document].join()`, "4,true,html, before the page ,8,true,true"},
		{`[document.head === document.documentElement.firstElementChild, document.body.parentNode ===
			document.documentElement, document.head.nextSibling.nodeType, document.head.nextElementSibling ===
			document.body].join()`, "true,true,3,true"},
		{`(s => [s.namespaceURI, s.tagName, s.getAttribute("viewBox"), s.getAttribute("viewbox"),
			s.firstElementChild.localName, s.querySelector("use").getAttribute("xlink:href"), s instanceof SVGElement,
			s instanceof HTMLElement, foreign.namespaceURI, foreign instanceof HTMLDivElement].join())(
			document.querySelector("svg"))`,
			"http://www.w3.org/2000/svg,svg,0 0 1 1,,linearGradient,#g,true,false,http://www.w3.org/1999/xhtml,true"},
		{`[root.getAttribute("tabindex"), root.tabIndex, root.getAttribute("href"), root.getAttribute("style"),
			root.hidden, root.dataset.upper, document.querySelector("input").autofocus,
			document.querySelector("img").srcset].join("|")`, "-1|-1|x|color:red;;  margin:0|true|V|true|b.png 2x"},
		{`[document.querySelector("noscript").textContent, typeof without,
			document.querySelectorAll("noscript")[1].firstChild.nodeType].join("|")`,
			"<style>body { display: none }</style>|undefined|3"},
		{`(c => [c.nodeType, c.childNodes.length, c.firstChild.localName, c.childNodes[1].data,
			c.lastChild.content.firstChild.textContent, t.innerHTML, c.firstChild.isConnected].join("|"))(t.content)`,
			`11|3|li|in|nested|<li class="row">row</li><!--in--><template><b>nested</b></template>|false`},
		{`[document.scripts.length, document.scripts[1].parentNode === document.body, data.parentNode ===
			document.head, document.querySelectorAll("style").length, document.styleSheets.length >= 0].join()`,
			"2,true,true,2,true"},
		{`(w => { const seen = []; for (let n = w.nextNode(); n; n = w.nextNode()) seen.push(n.data);
			return seen.join("|") })(
			document.createTreeWalker(document.body, NodeFilter.SHOW_COMMENT))`, "$| ||/$"},
		{`root.innerHTML`, "\n    text &amp; &lt;entity&gt; \"q\" 'r' &nbsp;<!-- -->more<!---->\n" +
			"    <span></span> <span> </span>\n  "},
		{`root.textContent`, "\n    text & <entity> \"q\" 'r' \u00a0more\n      \n  "},
		{`document.querySelector("iframe").outerHTML + document.querySelector("textarea").outerHTML`,
			`<iframe src="about:blank" title="frame">not &amp; shown</iframe><textarea> text &lt;</textarea>`},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s\n is %q\nwant %q", c.script, got, c.want)
		}
	}
	s.quiet()
}

func TestAPageIsWrittenBackAsChromeWritesIt(t *testing.T) {
	s := run(t, fidelity)
	got := "<!DOCTYPE " + s.eval(`document.doctype.name`) + ">\n" + s.eval(`document.documentElement.outerHTML`) + "\n"
	if got != fidelityInChrome {
		t.Errorf("the page is written back apart from Chrome:\n%s", differences(got, fidelityInChrome))
	}
	if again, want := parsed(t, got), parsed(t, fidelityInChrome); again != want {
		t.Errorf("what is written back reads apart:\n%s", differences(again, want))
	}
	s.quiet()
}

func TestWhatUIShowsIsWhatItHasAnElementFor(t *testing.T) {
	s := run(t, fidelity)
	doc := s.page.Doc
	for _, id := range []string{"data", "g", "foreign", "t", "without"} {
		if doc.ByID(id) != nil {
			t.Errorf("ui shows #%s", id)
		}
	}
	body := strings.Join(strings.Fields(doc.Body().Text()), " ")
	want := `text & <entity> "q" 'r' more x custom kept cell text <onetwoGo next onetwo kept as written ab moretold`
	if body != want {
		t.Errorf("the page shows %q\nwant         %q", body, want)
	}
	if n := s.byID("root"); n.Parent() != doc.Body() || len(n.Children()) != 7 {
		t.Errorf("#root shows %d nodes under %v", len(n.Children()), n.Parent().Tag())
	}
}
