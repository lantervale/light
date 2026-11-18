//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// shelf is a document without a space between its tags, so its nodes
// are these, in document order: html, head, title and its text, body, a
// comment, then what the ids name and their texts.
const shelf = `<!doctype html><html><head><title>Shelf</title></head><body><!--top-->` +
	`<div id="shelf" class="x"><h1 id="h">Books</h1><ul id="list">` +
	`<li id="a" class="book" data-n="1">Alpha</li>` +
	`<li id="b" class="book new" data-n="2">Beta <b id="bb">bold</b> tail</li>` +
	`<li id="c" data-n="3">Gamma</li>` +
	`<li id="d" class="book" data-n="10"><a id="link" href="/d" title="Delta">Delta</a></li></ul>` +
	`<p id="note" lang="en">  spaced   out  </p></div>` +
	`<p id="sum"><span id="n1">1</span><span id="n2">2.5</span><span id="n3">-3</span></p>` +
	`<svg id="pic" xml:lang="en-GB" viewBox="0 0 1 1"><g id="g"><a id="sa" xlink:href="#x"></a></g></svg>` +
	`</body></html>`

// shown gives a page x(expression, context): the value of an XPath
// expression as text. A number is as JavaScript writes it, -0 told
// apart; a string is quoted; a node-set is its nodes in the order the
// result gives them, an element by its id or its name, an attribute as
// @name=value, a text quoted, the document as /.
const shown = `window.by = (id) => document.getElementById(id);
window.label = (n) => n === document ? "/" : n.nodeType === 2 ? "@" + n.name + "=" + n.value
	: n.nodeType === 3 ? "'" + n.data + "'" : n.nodeType === 8 ? "<!--" + n.data + "-->" : n.id ? "#" + n.id : n.localName;
window.spaces = (prefix) => ({ svg: "http://www.w3.org/2000/svg", h: "http://www.w3.org/1999/xhtml",
	xlink: "http://www.w3.org/1999/xlink", xml: "http://www.w3.org/XML/1998/namespace" })[prefix] || null;
window.x = (expression, context = document) => {
	const r = document.evaluate(expression, context, spaces, XPathResult.ANY_TYPE, null);
	switch (r.resultType) {
		case XPathResult.NUMBER_TYPE: return Object.is(r.numberValue, -0) ? "-0" : String(r.numberValue);
		case XPathResult.STRING_TYPE: return '"' + r.stringValue + '"';
		case XPathResult.BOOLEAN_TYPE: return String(r.booleanValue);
	}
	const nodes = [];
	for (let n = r.iterateNext(); n; n = r.iterateNext()) nodes.push(label(n));
	return nodes.join(" ");
};
window.thrown = (fn) => { try { return "no error: " + fn(); } catch (e) { return e.constructor.name + " " + e.name; } };
"ready"`

func xpathPage(t *testing.T, markup string) *scripted {
	t.Helper()
	s := run(t, markup)
	if got := s.eval(shown); got != "ready" {
		t.Fatalf("the helpers are %q", got)
	}
	return s
}

func literal(t *testing.T, text string) string {
	t.Helper()
	out, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// xpathCase is an expression, the script of its context node (the
// document for none) and its value as x gives it.
type xpathCase struct{ expression, context, want string }

func checkXPath(t *testing.T, s *scripted, cases []xpathCase) {
	t.Helper()
	for _, c := range cases {
		context := c.context
		if context == "" {
			context = "document"
		}
		if got := s.eval("x(" + literal(t, c.expression) + ", " + context + ")"); got != c.want {
			t.Errorf("%s from %s\n is %s\nwant %s", c.expression, context, got, c.want)
		}
	}
	s.quiet()
}

const (
	classOfA = `by("a").attributes[1]`
	betaText = `by("b").firstChild`
	allText  = `"ShelfBooksAlphaBeta bold tailGammaDelta  spaced   out  12.5-3"`
)

func TestXPathAxes(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{"child::*", `by("list")`, "#a #b #c #d"},
		{"child::node()", `by("b")`, "'Beta ' #bb ' tail'"},
		{"child::node()", betaText, ""},
		{"descendant::node()", `by("b")`, "'Beta ' #bb 'bold' ' tail'"},
		{"descendant::li", "", "#a #b #c #d"},
		{"descendant-or-self::*", `by("b")`, "#b #bb"},
		{"descendant-or-self::node()", `by("bb")`, "#bb 'bold'"},
		{"parent::*", `by("b")`, "#list"},
		{"parent::node()", `document.documentElement`, "/"},
		{"parent::node()", "", ""},
		{"parent::li", `by("bb")`, "#b"},
		{"parent::ul", `by("bb")`, ""},
		{"ancestor::*", `by("bb")`, "html body #shelf #list #b"},
		{"ancestor::node()", `by("bb")`, "/ html body #shelf #list #b"},
		{"ancestor-or-self::li", `by("bb")`, "#b"},
		{"ancestor-or-self::*", `by("b")`, "html body #shelf #list #b"},
		{"following-sibling::*", `by("b")`, "#c #d"},
		{"following-sibling::node()", betaText, "#bb ' tail'"},
		{"preceding-sibling::*", `by("c")`, "#a #b"},
		{"preceding-sibling::node()", `by("b").lastChild`, "'Beta ' #bb"},
		{"following::*", `by("c")`, "#d #link #note #sum #n1 #n2 #n3 #pic #g #sa"},
		{"following::text()", `by("c")`, "'Delta' '  spaced   out  ' '1' '2.5' '-3'"},
		{"following::*", `by("list")`, "#note #sum #n1 #n2 #n3 #pic #g #sa"},
		{"preceding::*", `by("c")`, "head title #h #a #b #bb"},
		{"preceding::node()", `by("h")`, "head title 'Shelf' <!--top-->"},
		{"attribute::*", `by("a")`, "@id=a @class=book @data-n=1"},
		{"attribute::class", `by("a")`, "@class=book"},
		{"attribute::node()", `by("h")`, "@id=h"},
		{"attribute::text()", `by("h")`, ""},
		{"attribute::*", betaText, ""},
		{"namespace::*", `document.documentElement`, ""},
		{"namespace::node()", `by("pic")`, ""},
		{"self::li", `by("b")`, "#b"},
		{"self::p", `by("b")`, ""},
		{"self::node()", "", "/"},
		{"self::*", "", ""},
		{"self::text()", betaText, "'Beta '"},
	})
}

func TestXPathFromEveryKindOfNode(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{".", "", "/"},
		{"*", "", "html"},
		{"node()", "", "html"},
		{"/", "", "/"},
		{"/", `by("bb")`, "/"},
		{"string(/)", "", allText},
		{"string()", "", allText},
		{"count(//node())", "", "34"},
		{"count(//.)", "", "35"},
		{"count(//*)", "", "21"},
		{"count(//text())", "", "12"},
		{"count(//comment())", "", "1"},
		{"count(//@*)", "", "31"},

		{"li[2]/b", `by("list")`, "#bb"},
		{"string()", `by("b")`, `"Beta bold tail"`},
		{"count(.//text())", `by("b")`, "3"},
		{"/html/body/*[1]", `by("b")`, "#shelf"},

		{".", classOfA, "@class=book"},
		{"..", classOfA, "#a"},
		{"parent::*", classOfA, "#a"},
		{"ancestor::*", classOfA, "html body #shelf #list #a"},
		{"ancestor-or-self::node()[1]", classOfA, "@class=book"},
		{"self::node()", classOfA, "@class=book"},
		{"self::*", classOfA, ""},
		{"child::node()", classOfA, ""},
		{"descendant-or-self::node()", classOfA, "@class=book"},
		{"following-sibling::node()", classOfA, ""},
		{"preceding-sibling::node()", classOfA, ""},
		{"following::node()[1]", classOfA, "'Alpha'"},
		{"following::*[1]", classOfA, "#b"},
		{"preceding::*", classOfA, "head title #h"},
		{"../@id", classOfA, "@id=a"},
		{"string()", classOfA, `"book"`},
		{"name()", classOfA, `"class"`},
		{"/html/head/title", classOfA, "title"},

		{".", betaText, "'Beta '"},
		{"..", betaText, "#b"},
		{"string()", betaText, `"Beta "`},
		{"following::node()[1]", betaText, "#bb"},
		{"preceding::text()[1]", betaText, "'Alpha'"},
		{"ancestor::ul", betaText, "#list"},
		{"name()", betaText, `""`},

		{".", `document.body.firstChild`, "<!--top-->"},
		{"string()", `document.body.firstChild`, `"top"`},
		{"following-sibling::*[1]", `document.body.firstChild`, "#shelf"},
	})
}

func TestXPathNodeTestsAndNames(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{`//text()[. = "bold"]`, "", "'bold'"},
		{"//comment()", "", "<!--top-->"},
		{"//processing-instruction()", "", ""},
		{`//processing-instruction("x")`, "", ""},
		{"/html/body/node()[1]", "", "<!--top-->"},
		{"//li[2]/node()", "", "'Beta ' #bb ' tail'"},
		{"//li[2]/text()", "", "'Beta ' ' tail'"},
		{"//li[2]/*", "", "#bb"},
		{"//text", "", ""},

		{"//svg:*", "", "#pic #g #sa"},
		{"//svg:a", "", "#sa"},
		{"//a", "", "#link"},
		{"//h:a", "", "#link"},
		{"//h:*[@href]", "", "#link"},
		{`//*[local-name() = "a"]`, "", "#link #sa"},
		{"//svg", "", ""},
		{"//svg:svg/@viewBox", "", "@viewBox=0 0 1 1"},
		{"//@xlink:href", "", "@xlink:href=#x"},
		{"//svg:*/@xlink:*", "", "@xlink:href=#x"},
		{"//svg:g/svg:a/@*", "", "@id=sa @xlink:href=#x"},
		{"//@xml:lang", "", "@xml:lang=en-GB"},
		{"//@lang", "", "@lang=en"},
		{"//@href", "", "@href=/d"},

		{`//LI[@CLASS = "book"]/@ID`, "", "@id=a @id=d"},
		{"//DIV/H1", "", "#h"},
		{"/HTML/Body//uL/Li[1]", "", "#a"},
		{"name(//LI)", "", `"li"`},
		{"local-name(/*)", "", `"html"`},
		{"//svg:G", "", ""},
		{"//svg:svg/@viewbox", "", ""},
		{"//svg:SVG", "", ""},
	})
}

func TestXPathPredicates(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{"//li[1]", "", "#a"},
		{"//li[last()]", "", "#d"},
		{"//li[position() = 2]", "", "#b"},
		{"//li[position() > 1][1]", "", "#b"},
		{"//li[@class][2]", "", "#b"},
		{"//li[2][@class]", "", "#b"},
		{"//li[3][@class]", "", ""},
		{"//li[last() - 1]", "", "#c"},
		{"//li[position() mod 2 = 0]", "", "#b #d"},
		{"//li[1.5]", "", ""},
		{"//li[0]", "", ""},
		{`//li[number("x")]`, "", ""},
		{"//li[1 + 1]", "", "#b"},
		{"//li[count(//span)]", "", "#c"},

		{`//li[@id = "d"]/preceding-sibling::li[1]`, "", "#c"},
		{`(//li[@id = "d"]/preceding-sibling::li)[1]`, "", "#a"},
		{`//li[@id = "d"]/preceding-sibling::li[position() < 3]`, "", "#b #c"},
		{"//b/ancestor::*[1]", "", "#b"},
		{"//b/ancestor::*[last()]", "", "html"},
		{"//b/ancestor-or-self::*[2]", "", "#b"},
		{`//li[@id = "c"]/preceding::*[2]`, "", "#b"},
		{`//li[@id = "a"]/following::*[2]`, "", "#bb"},
		{`//li[@id = "a"]/following-sibling::*[last()]`, "", "#d"},

		{"(//li)[2]", "", "#b"},
		{"(//span)[last()]", "", "#n3"},
		{"//p[1]", "", "#note #sum"},
		{"(//p)[1]", "", "#note"},
		{"/descendant::p[1]", "", "#note"},
		{"/descendant::p[2]", "", "#sum"},
		{"//p[2]", "", ""},
		{`//*[@id = "shelf"]//*[1]`, "", "#h #a #bb #link"},
		{"//ul/li[position() < 3]", "", "#a #b"},
		{`//div[@class = "x"]//li[position() < 3]`, "", "#a #b"},

		{"//li[@data-n > 2]", "", "#c #d"},
		{"//li[@data-n = 10]", "", "#d"},
		{"//li[b]", "", "#b"},
		{"//li[not(@class)]", "", "#c"},
		{"//li[a/@href]", "", "#d"},
		{"//li[string-length() = 5][2]", "", "#c"},
		{`//li[b[. = "bold"]]`, "", "#b"},
		{`//ul[li[last()][@id = "d"]]`, "", "#list"},
		{`//li[.//text()[contains(., "tail")]]`, "", "#b"},
		{"//li[@data-n*2=4]/@id", "", "@id=b"},
		{"//*[self::b or self::h1]", "", "#h #bb"},
		{"//li [ @id = 'a' ]", "", "#a"},
		{"//li[true()]", "", "#a #b #c #d"},
		{`//li["x"]`, "", "#a #b #c #d"},
		{`//li[""]`, "", ""},
	})
}

func TestXPathAbbreviationsUnionsAndDocumentOrder(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{".//b", `by("list")`, "#bb"},
		{"..", `by("a")`, "#list"},
		{"./li", `by("list")`, "#a #b #c #d"},
		{"../li[3]/..", `by("a")`, "#list"},
		{"//li/..", "", "#list"},
		{"//li/@id", "", "@id=a @id=b @id=c @id=d"},
		{"/html//b/text()", "", "'bold'"},
		{"//ul//text()", "", "'Alpha' 'Beta ' 'bold' ' tail' 'Gamma' 'Delta'"},
		{"/*", "", "html"},
		{"/*/*", "", "head body"},
		{"//ul//.", "", "#list #a 'Alpha' #b 'Beta ' #bb 'bold' ' tail' #c 'Gamma' #d #link 'Delta'"},
		{"/descendant-or-self::node()/child::li", "", "#a #b #c #d"},

		{"//b | //h1", "", "#h #bb"},
		{`//li[@id = "d"] | //li[@id = "a"] | //li[@id = "a"]`, "", "#a #d"},
		{"//li/@id | //ul", "", "#list @id=a @id=b @id=c @id=d"},
		{`//text()[. = "Gamma"] | //li[3] | //li[3]/@*`, "", "#c @id=c @data-n=3 'Gamma'"},
		{"/ | //title", "", "/ title"},
		{"//span/.. | //b/../..", "", "#list #sum"},
		{"(//li | //p)[last()]", "", "#sum"},
		{"(//li | //p)[5]", "", "#note"},
		{"//a/ancestor::li | //b/ancestor::li", "", "#b #d"},
		{"count(//li | //li)", "", "4"},
		{"//nope | //h1", "", "#h"},

		{"//li/following::span", "", "#n1 #n2 #n3"},
		{"//li/ancestor::*", "", "html body #shelf #list"},
		{"//span/preceding-sibling::span", "", "#n1 #n2"},
		{"//li/preceding::li", "", "#a #b #c"},
		{"//text()/..", "", "title #h #a #b #bb #c #link #note #n1 #n2 #n3"},
		{"//span/following-sibling::*/preceding-sibling::*", "", "#n1 #n2"},
		{"//li/descendant-or-self::*/ancestor-or-self::li", "", "#a #b #c #d"},
		{"//b/ancestor::*/child::*[1]", "", "head #shelf #h #a #bb"},
		{"//@id/..", "", "#shelf #h #list #a #b #bb #c #d #link #note #sum #n1 #n2 #n3 #pic #g #sa"},
	})
}

func TestXPathOperators(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{"1 + 2 * 3", "", "7"},
		{"(1 + 2) * 3", "", "9"},
		{"2*3", "", "6"},
		{"7 div 2", "", "3.5"},
		{"7 mod 2", "", "1"},
		{"-7 mod 2", "", "-1"},
		{"7 mod -2", "", "1"},
		{"5 mod 2.5", "", "0"},
		{"1 div 0", "", "Infinity"},
		{"-1 div 0", "", "-Infinity"},
		{"0 div 0", "", "NaN"},
		{"- - 2", "", "2"},
		{"-(2)", "", "-2"},
		{"2 - -2", "", "4"},
		{"8 div 2 div 2", "", "2"},
		{"2 - 3 - 4", "", "-5"},
		{"1.5 + .5", "", "2"},
		{"1. + 1", "", "2"},
		{`"3" + "4"`, "", "7"},
		{`"a" + 1`, "", "NaN"},
		{"true() + 1", "", "2"},
		{"//span + 1", "", "2"},
		{"-//span[3]", "", "3"},
		{"count(div) div count(div)", `document.body`, "1"},
		{"div", `document.body`, "#shelf"},
		{"count(//li) * count(//span)", "", "12"},

		{"true() and false()", "", "false"},
		{"true() or false()", "", "true"},
		{"1 or 0", "", "true"},
		{"0 and 1", "", "false"},
		{"false() and false() or true()", "", "true"},
		{"true() or false() and false()", "", "true"},
		{"1 = 1 = 1", "", "true"},
		{"1 = 2 = 0", "", "true"},
		{"1 < 2 < 3", "", "true"},
		{"3 > 2 > 1", "", "false"},
		{"1 + 1 = 2", "", "true"},
		{"1 < 2 = true()", "", "true"},

		{`//li = "Gamma"`, "", "true"},
		{`//li != "Gamma"`, "", "true"},
		{`//li[@id = "c"] != "Gamma"`, "", "false"},
		{`"Gamma" = //li`, "", "true"},
		{`//nope = "x"`, "", "false"},
		{`//nope != "x"`, "", "false"},
		{"//span = 2.5", "", "true"},
		{"//span > 2", "", "true"},
		{"//span < -2", "", "true"},
		{"//span > 3", "", "false"},
		{"//span >= 2.5", "", "true"},
		{"//span <= -3", "", "true"},
		{"//li/@data-n = 10", "", "true"},
		{"3 > //span", "", "true"},
		{"-3 > //span", "", "false"},
		{"//li < 0", "", "false"},
		{`//span > "2"`, "", "true"},
		{`//li/@data-n > "9"`, "", "true"},
		{"//li = true()", "", "true"},
		{"//li != true()", "", "false"},
		{"//nope = false()", "", "true"},
		{"//nope = true()", "", "false"},
		{"//nope < true()", "", "true"},
		{"//li/@data-n = //span", "", "true"},
		{"//li/@id = //span", "", "false"},
		{"//span != //span", "", "true"},
		{"//span[1] != //span[1]", "", "false"},
		{"//span > //li/@data-n", "", "true"},
		{"//li/@data-n < //span[3]", "", "false"},
		{"//nope = //nope", "", "false"},
		{"//li/@id = //li/@id", "", "true"},

		{`"a" = "a"`, "", "true"},
		{`"1" = 1`, "", "true"},
		{`"1.0" = 1`, "", "true"},
		{`"1.0" = "1"`, "", "false"},
		{"true() = 1", "", "true"},
		{"true() = 2", "", "true"},
		{`true() = "x"`, "", "true"},
		{`false() = ""`, "", "true"},
		{`"a" < "b"`, "", "false"},
		{`"2" > "10"`, "", "false"},
		{`"10" > "9"`, "", "true"},
		{"1 != 1", "", "false"},
		{"0 = -0", "", "true"},
		{"(0 div 0) = (0 div 0)", "", "false"},
		{"(0 div 0) != (0 div 0)", "", "true"},
		{"true() > false()", "", "true"},
		{"(1 div 0) = (1 div 0)", "", "true"},
	})
}

func TestXPathFunctions(t *testing.T) {
	checkXPath(t, xpathPage(t, shelf), []xpathCase{
		{"position()", "", "1"},
		{"last()", "", "1"},
		{"count(//li)", "", "4"},
		{"count(//nope)", "", "0"},

		{`id("a")`, "", "#a"},
		{`id("d a  nope")`, "", "#a #d"},
		{"id(//li/@id)", "", "#a #b #c #d"},
		{`id("list")/li[2]`, "", "#b"},
		{"id(1)", "", ""},
		{`id("a")/@class`, `by("n1")`, "@class=book"},

		{"local-name(//svg:a)", "", `"a"`},
		{"local-name()", `by("a")`, `"li"`},
		{"local-name(//@xlink:href)", "", `"href"`},
		{"local-name(//comment())", "", `""`},
		{"local-name(/)", "", `""`},
		{"local-name(//nope)", "", `""`},
		{"local-name(//span | //h1)", "", `"h1"`},
		{"namespace-uri(//li)", "", `"http://www.w3.org/1999/xhtml"`},
		{"namespace-uri(//svg:g)", "", `"http://www.w3.org/2000/svg"`},
		{"namespace-uri(//@xlink:href)", "", `"http://www.w3.org/1999/xlink"`},
		{"namespace-uri(//@id)", "", `""`},
		{"namespace-uri(//text())", "", `""`},
		{"name(//@xlink:href)", "", `"xlink:href"`},
		{"name(//li)", "", `"li"`},
		{"name(//text())", "", `""`},
		{"name()", `by("pic")`, `"svg"`},

		{"string(//li)", "", `"Alpha"`},
		{"string(//li[2])", "", `"Beta bold tail"`},
		{"string(12)", "", `"12"`},
		{"string(1.5)", "", `"1.5"`},
		{"string(-0.5)", "", `"-0.5"`},
		{"string(0 div 0)", "", `"NaN"`},
		{"string(1 div 0)", "", `"Infinity"`},
		{"string(-1 div 0)", "", `"-Infinity"`},
		{"string(-0)", "", `"0"`},
		{"string(1 div 3)", "", `"0.3333333333333333"`},
		{"string(1000000 * 1000000 * 1000000 * 1000000)", "", `"1000000000000000000000000"`},
		{"string(1.5 * 1000000 * 1000000 * 1000000 * 1000000)", "", `"1500000000000000000000000"`},
		{"string(1 div 10000000)", "", `"0.0000001"`},
		{"string(-(1 div 10000000))", "", `"-0.0000001"`},
		{"string(1.25 div 100000000)", "", `"0.0000000125"`},
		{"string(0.000001)", "", `"0.000001"`},
		{"string(100)", "", `"100"`},
		{"string(true())", "", `"true"`},
		{"string(false())", "", `"false"`},
		{`string("x")`, "", `"x"`},
		{"string()", `by("a")`, `"Alpha"`},
		{"string(//nope)", "", `""`},
		{"string(//comment())", "", `"top"`},
		{"string(//li/@class)", "", `"book"`},

		{`concat("a", "b")`, "", `"ab"`},
		{`concat("a", 1, true(), //li)`, "", `"a1trueAlpha"`},
		{`starts-with("abc", "ab")`, "", "true"},
		{`starts-with("abc", "")`, "", "true"},
		{`starts-with("abc", "bc")`, "", "false"},
		{`contains("abc", "b")`, "", "true"},
		{`contains("abc", "")`, "", "true"},
		{`contains("abc", "d")`, "", "false"},
		{`//li[contains(@class, "new")]`, "", "#b"},
		{`//li[starts-with(., "G")]`, "", "#c"},
		{`substring-before("1999/04/01", "/")`, "", `"1999"`},
		{`substring-after("1999/04/01", "/")`, "", `"04/01"`},
		{`substring-after("1999/04/01", "19")`, "", `"99/04/01"`},
		{`substring-before("abc", "x")`, "", `""`},
		{`substring-after("abc", "x")`, "", `""`},
		{`substring-before("abc", "")`, "", `""`},
		{`substring-after("abc", "")`, "", `"abc"`},

		{`substring("12345", 2, 3)`, "", `"234"`},
		{`substring("12345", 2)`, "", `"2345"`},
		{`substring("12345", 1.5, 2.6)`, "", `"234"`},
		{`substring("12345", 0, 3)`, "", `"12"`},
		{`substring("12345", 0 div 0, 3)`, "", `""`},
		{`substring("12345", 1, 0 div 0)`, "", `""`},
		{`substring("12345", -42, 1 div 0)`, "", `"12345"`},
		{`substring("12345", -1 div 0, 1 div 0)`, "", `""`},
		{`substring("12345", 6)`, "", `""`},
		{`substring("12345", 1 div 0)`, "", `""`},
		{`substring("12345", -1 div 0)`, "", `"12345"`},
		{`substring("12345", 5, 1)`, "", `"5"`},
		{`substring("12345", 2, -1)`, "", `""`},
		{`substring("a𝒳b", 2, 1)`, "", `"𝒳"`},
		{`string-length("a𝒳b")`, "", "3"},
		{`string-length("")`, "", "0"},
		{`string-length("abc")`, "", "3"},
		{"string-length()", `by("a")`, "5"},
		{"normalize-space(\"  a  b \t\n c \")", "", `"a b c"`},
		{`normalize-space(//p[@id = "note"])`, "", `"spaced out"`},
		{"normalize-space()", `by("note")`, `"spaced out"`},
		{"normalize-space(\" a  \")", "", "\"a \""},
		{`translate("bar", "abc", "ABC")`, "", `"BAr"`},
		{`translate("--aaa--", "abc-", "ABC")`, "", `"AAA"`},
		{`translate("abc", "aa", "xy")`, "", `"xbc"`},

		{"boolean(0)", "", "false"},
		{"boolean(0 div 0)", "", "false"},
		{"boolean(-1)", "", "true"},
		{`boolean("")`, "", "false"},
		{`boolean("false")`, "", "true"},
		{"boolean(//li)", "", "true"},
		{"boolean(//nope)", "", "false"},
		{"not(true())", "", "false"},
		{"not(//nope)", "", "true"},
		{`//svg:g[lang("en")]`, "", "#g"},
		{`lang("en")`, `by("sa")`, "true"},
		{`lang("EN-gb")`, `by("sa")`, "true"},
		{`lang("en-US")`, `by("sa")`, "false"},
		{`lang("e")`, `by("sa")`, "false"},
		{`lang("en")`, `by("pic").attributes[0]`, "true"},
		{`lang("en")`, `by("note")`, "false"},
		{`lang("en")`, "", "false"},

		{`number("12")`, "", "12"},
		{`number(" 12.5 ")`, "", "12.5"},
		{`number("-.5")`, "", "-0.5"},
		{`number("1.")`, "", "1"},
		{`number("+1")`, "", "NaN"},
		{`number("1e3")`, "", "NaN"},
		{`number("")`, "", "NaN"},
		{`number(".")`, "", "NaN"},
		{`number("0x10")`, "", "NaN"},
		{`number("Infinity")`, "", "NaN"},
		{`number("1 2")`, "", "NaN"},
		{`number("- 1")`, "", "NaN"},
		{"number(\" 1\")", "", "NaN"},
		{"number(true())", "", "1"},
		{"number(false())", "", "0"},
		{"number(//span[2])", "", "2.5"},
		{"number()", `by("n3")`, "-3"},
		{"number(//nope)", "", "NaN"},
		{"sum(//span)", "", "0.5"},
		{"sum(//li/@data-n)", "", "16"},
		{"sum(//nope)", "", "0"},
		{"sum(//li)", "", "NaN"},
		{"floor(2.6)", "", "2"},
		{"floor(-2.5)", "", "-3"},
		{"ceiling(2.1)", "", "3"},
		{"ceiling(-2.5)", "", "-2"},
		{"round(2.5)", "", "3"},
		{"round(2.4)", "", "2"},
		{"round(1.5)", "", "2"},
		{"round(-1.5)", "", "-1"},
		{"round(-2.5)", "", "-2"},
		{"round(-0.5)", "", "-0"},
		{"round(-0.2)", "", "-0"},
		{"1 div round(-0.5)", "", "-Infinity"},
		{"string(round(-0.5))", "", `"0"`},
		{"round(0 div 0)", "", "NaN"},
		{"round(1 div 0)", "", "Infinity"},
		{"round(-1 div 0)", "", "-Infinity"},
		{`round("2.5")`, "", "3"},
	})
}

func TestXPathRefusesWhatIsNotXPath(t *testing.T) {
	s := xpathPage(t, shelf)
	for _, expression := range []string{
		"", " ", "//li[", "//", "/..../", "1 +", "foo()", "count()", "count(1)", `count("li")`, `concat("a")`, "$x",
		"//li[$n]", "1 | 2", "//li | 1", `"a"/b`, "(1)[1]", "//li[1]]", "child::", "bogus::li", "1e3", `//li[@id='a"]`,
		"a b", ".[1]", "..[1]", "!", "//li[@id=]", "text(", "node(1)", "processing-instruction(x)", "//li[1", "@",
		"string(1, 2)", `sum("1")`, "1 2", "//a::b", "last(1)", " //li", "p:f()", "//li/", "//li//", "li:", "//*:li",
		"1 = ", "()", "//li[]", "true(", "not()", "local-name(1)", `substring("a")`, "//li,", "#", "//li = = 1", "a/-b",
	} {
		script := "thrown(() => document.evaluate(" + literal(t, expression) + ", document, spaces, 0, null).resultType)"
		if got := s.eval(script); got != "DOMException SyntaxError" {
			t.Errorf("%q gives %s, want a SyntaxError", expression, got)
		}
	}
	for _, c := range []struct{ script, want string }{
		{`document.evaluate("//x:li", document, null, 0, null)`, "DOMException NamespaceError"},
		{`document.evaluate("//x:li", document, () => null, 0, null)`, "DOMException NamespaceError"},
		{`document.evaluate("//@x:*", document, { lookupNamespaceURI: () => "" }, 0, null)`, "DOMException NamespaceError"},
		{`document.createExpression("//x:li")`, "DOMException NamespaceError"},
		{`document.createExpression("//li[")`, "DOMException SyntaxError"},
		{`new XPathEvaluator().createExpression("1 +")`, "DOMException SyntaxError"},
		{`document.evaluate("//li")`, "TypeError TypeError"},
		{`document.evaluate("//li", {})`, "TypeError TypeError"},
		{`document.evaluate("//li", null)`, "TypeError TypeError"},
		{`document.evaluate("//li", document.doctype)`, "DOMException NotSupportedError"},
		{`document.evaluate("//li", document, null, 42)`, "DOMException NotSupportedError"},
		{`document.evaluate("1", document, null, XPathResult.ORDERED_NODE_SNAPSHOT_TYPE)`, "TypeError TypeError"},
		{`document.evaluate("'a'", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE)`, "TypeError TypeError"},
		{`document.evaluate("true()", document, null, XPathResult.UNORDERED_NODE_ITERATOR_TYPE)`, "TypeError TypeError"},
		{`new XPathResult()`, "TypeError TypeError"},
		{`new XPathExpression()`, "TypeError TypeError"},
	} {
		if got := s.eval("thrown(() => " + c.script + ")"); got != c.want {
			t.Errorf("%s gives %s, want %s", c.script, got, c.want)
		}
	}
	s.quiet()
}

func TestXPathResults(t *testing.T) {
	s := xpathPage(t, shelf)
	s.eval(`window.ev = (expression, type) => document.evaluate(expression, document, null, type, null);
		window.typed = (read) => {
			try { read(); return true; } catch (e) { if (e instanceof TypeError) return false; throw e; }
		};
		window.reads = (r) => ["numberValue", "stringValue", "booleanValue", "singleNodeValue", "snapshotLength"]
			.filter((name) => typed(() => r[name]))
			.concat(["iterateNext", "snapshotItem"].filter((name) => typed(() => r[name](0)))).join();
		window.ids = (r) => {
			const out = [];
			for (let n = r.iterateNext(); n; n = r.iterateNext()) out.push(n.id);
			return out.join();
		};
		0`)
	for _, c := range []struct{ name, script, want string }{
		{"the types are numbered", `["ANY_TYPE", "NUMBER_TYPE", "STRING_TYPE", "BOOLEAN_TYPE",
			"UNORDERED_NODE_ITERATOR_TYPE", "ORDERED_NODE_ITERATOR_TYPE", "UNORDERED_NODE_SNAPSHOT_TYPE",
			"ORDERED_NODE_SNAPSHOT_TYPE", "ANY_UNORDERED_NODE_TYPE", "FIRST_ORDERED_NODE_TYPE"]
			.map((name) => XPathResult[name] + "" + ev("1", 0)[name]).join()`, "00,11,22,33,44,55,66,77,88,99"},
		{"any type is the expression's", `[ev("1 + 1", 0), ev("concat('a', 'b')", 0), ev("1 = 1", 0), ev("//li", 0)]
			.map((r) => r.resultType).join()`, "1,2,3,4"},
		{"a number", `(r => [r.resultType, r.numberValue, reads(r)].join(" "))(ev("count(//li)", 1))`, "1 4 numberValue"},
		{"a string", `(r => [r.resultType, r.stringValue, reads(r)].join(" "))(ev("string(//h1)", 2))`,
			"2 Books stringValue"},
		{"a boolean", `(r => [r.resultType, r.booleanValue, reads(r)].join(" "))(ev("//li", 3))`, "3 true booleanValue"},
		{"a node-set as a number", `ev("//span", 1).numberValue`, "1"},
		{"a node-set as a string", `ev("//li", 2).stringValue`, "Alpha"},
		{"a number as a string", `ev("1 div 3", 2).stringValue`, "0.3333333333333333"},
		{"a string as a number", `ev("' 7 '", 1).numberValue + "," + ev("'seven'", 1).numberValue`, "7,NaN"},
		{"a string as a boolean", `ev("''", 3).booleanValue + "," + ev("'0'", 3).booleanValue`, "false,true"},
		{"a boolean as a number and a string", `ev("true()", 1).numberValue + ev("false()", 2).stringValue`, "1false"},
		{"an unordered iterator", `(r => [r.resultType, reads(ev("//li", 4)), r.invalidIteratorState, ids(r),
			r.iterateNext(), r.iterateNext()].join(" "))(ev("//li", 4))`, "4 iterateNext false a,b,c,d  "},
		{"an ordered iterator", `(r => [r.resultType, reads(ev("//li", 5)), ids(r)].join(" "))(
			ev("//li[@id = 'd'] | //h1 | //li[1]", 5))`, "5 iterateNext h,a,d"},
		{"an unordered snapshot", `(r => [r.resultType, reads(r), r.snapshotLength, r.snapshotItem(0).id,
			r.snapshotItem(3).id, r.snapshotItem(4), r.snapshotItem(-1), r.invalidIteratorState].join(" "))(ev("//li", 6))`,
			"6 snapshotLength,snapshotItem 4 a d   false"},
		{"an ordered snapshot", `(r => [r.resultType, r.snapshotLength, Array.from({ length: 3 }, (_, i) =>
			r.snapshotItem(i).id)].join(" "))(ev("//span/.. | //b | //h1", 7))`, "7 3 h,bb,sum"},
		{"an empty snapshot", `(r => r.snapshotLength + " " + r.snapshotItem(0))(ev("//nope", 7))`, "0 null"},
		{"any node", `(r => [r.resultType, reads(r), r.singleNodeValue.id].join(" "))(ev("//li", 8))`, "8 singleNodeValue a"},
		{"the first node", `(r => [r.resultType, reads(r), r.singleNodeValue.id].join(" "))(ev("//p | //h1", 9))`,
			"9 singleNodeValue h"},
		{"no first node", `ev("//nope", 9).singleNodeValue`, "null"},
		{"an attribute found twice is one object", `ev("//li[1]/@class", 9).singleNodeValue === ev("//@class[. = 'book']", 9)
			.singleNodeValue`, "true"},
		{"an attribute is an Attr", `(a => [a instanceof Attr, a.nodeType, a.name, a.value, a.ownerElement.id].join())(
			ev("//a/@href", 9).singleNodeValue)`, "true,2,href,/d,link"},
		{"an attribute given is the attribute found", `(a => document.evaluate(".", a, null, 9, null).singleNodeValue === a &&
			ev("//li[1]/@data-n", 9).singleNodeValue === a)(by("a").attributes[2])`, "true"},
		{"the result given is not needed", `document.evaluate("//li", document, null, 7).snapshotLength +
			document.evaluate("//li", document, null, 7, ev("1", 0)).snapshotLength`, "8"},
		{"an expression kept", `(e => [e instanceof XPathExpression, e.evaluate(document).numberValue,
			e.evaluate(by("b"), XPathResult.STRING_TYPE).stringValue, e.evaluate(by("list"), 0, null).numberValue].join())(
			document.createExpression("count(.//text())"))`, "true,12,3,6"},
		{"an evaluator", `(e => [e.evaluate("//li", document, null, 7, null).snapshotLength,
			e.createExpression("//h:li", spaces).evaluate(document, 7).snapshotLength,
			e.createNSResolver(document.body) === document.body, document.createNSResolver(by("a")) === by("a"),
			e.evaluate("//li", document) instanceof XPathResult].join())(new XPathEvaluator())`, "4,4,true,true,true"},
		{"a resolver that is an object", `document.evaluate("//s:g", document, { lookupNamespaceURI: (prefix) =>
			prefix === "s" ? "http://www.w3.org/2000/svg" : null }, 9, null).singleNodeValue.id`, "g"},
		{"a resolver is asked once a prefix", `(asked => { document.evaluate("//h:li | //svg:g/@xlink:href", document,
			(prefix) => { asked.push(prefix); return spaces(prefix); }, 0, null); return asked.join(); })([])`, "h,svg,xlink"},
		{"a tree of its own", `(d => { d.innerHTML = "<p id=one>a</p><p id=two>b<i>c</i></p>"; return [
			x("p[2]", d), x("/", d) === label(d), x("//i/..", d), x("count(//node())", d), x("id('two one')", d),
			x("//i/ancestor::*", d), x("//p | //i", d), by("one") === null].join(" "); })(document.createElement("div"))`,
			"#two true #two 6 #one #two div #two #one #two i true"},
		{"a fragment", `(f => { f.append(document.createElement("b"), "text", document.createElement("i")); return [
			x("*", f), x("count(node())", f), x("string()", f), x("i/preceding-sibling::node()", f),
			document.evaluate("/", f, null, 9, null).singleNodeValue === f].join(" "); })(
			document.createDocumentFragment())`, `b i 3 "text" b 'text' true`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := s.eval(c.script); got != c.want {
				t.Errorf("%s\n is %q\nwant %q", c.script, got, c.want)
			}
		})
	}
	s.quiet()
}

func TestAnXPathIteratorEndsWhenTheDocumentChanges(t *testing.T) {
	s := xpathPage(t, shelf)
	state := func(name string) string {
		return s.eval(`(r => r.invalidIteratorState + " " + thrown(() => r.iterateNext().id))(` + name + `)`)
	}
	s.eval(`window.heard = 0; new MutationObserver((records) => { heard += records.length; })
		.observe(document.body, { subtree: true, childList: true, attributes: true, characterData: true });
		window.fresh = () => document.evaluate("//li", document, null, XPathResult.ORDERED_NODE_ITERATOR_TYPE, null);
		window.it = fresh(); window.snap = document.evaluate("//li", document, null, 7, null);
		window.done = fresh(); while (done.iterateNext());
		window.other = document.evaluate("//li", document.createElement("div"), null, 4, null); 0`)
	if got := state("it"); got != "false no error: a" {
		t.Errorf("before a change the iterator is %s", got)
	}
	if got := s.eval(`by("c").remove(); it.invalidIteratorState + " " + thrown(() => it.iterateNext())`); got !=
		"true DOMException InvalidStateError" {
		t.Errorf("after a removal the iterator is %s", got)
	}
	if got := state("it"); got != "true DOMException InvalidStateError" {
		t.Errorf("a turn later the iterator is %s", got)
	}
	if got := s.eval(`[snap.snapshotLength, snap.snapshotItem(2).id, snap.invalidIteratorState, done.invalidIteratorState,
		done.iterateNext(), other.invalidIteratorState, other.iterateNext()].join()`); got != "4,c,false,false,,false," {
		t.Errorf("a snapshot, an iterator that ended and one of another tree are %s", got)
	}
	for _, change := range []string{
		`by("a").setAttribute("class", "read")`, `by("a").firstChild.data = "Alef"`, `by("list").append("more")`,
		`by("h").textContent = "Shelf"`, `by("note").innerHTML = "<i>n</i>"`, `by("d").removeAttribute("class")`,
	} {
		s.eval(`window.it = fresh(); it.iterateNext(); 0`)
		s.eval(change + "; 0")
		if got := state("it"); got != "true DOMException InvalidStateError" {
			t.Errorf("after %s the iterator is %s", change, got)
		}
	}
	s.eval(`window.it = fresh(); 0`)
	if got := state("it"); got != "false no error: a" {
		t.Errorf("an iterator made after the changes is %s", got)
	}
	if got := s.eval(`heard`); got != "7" {
		t.Errorf("the page's own observer heard %s records, want 7", got)
	}
	s.quiet()
}

// A page of 10,020 nodes: 334 blocks of a div, a list and four items of
// three links each, every other div of the class x.
func TestXPathOnALargeDocument(t *testing.T) {
	var markup strings.Builder
	markup.WriteString("<body>")
	for i := range 334 {
		class := "y"
		if i%2 == 0 {
			class = "x"
		}
		markup.WriteString(`<div class="` + class + `"><ul>`)
		for range 4 {
			markup.WriteString(`<li><a href="#1">1</a><a href="#2">2</a><a href="#3">3</a></li>`)
		}
		markup.WriteString("</ul></div>")
	}
	s := xpathPage(t, markup.String())
	if got := s.eval(`x("count(//body//node())")`); got != "10020" {
		t.Fatalf("the page has %s nodes", got)
	}
	for _, c := range []struct{ expression, want string }{
		{`//div[@class="x"]//a[position() < 3]`, "1336"},
		{`//div[@class="x"]//a`, "2004"},
		{`//a[@href="#2"]/..`, "1336"},
		{`//li[last()]/a[2]/text()`, "334"},
		{`//text()/ancestor::div`, "334"},
		{`(//a)[last()]/preceding::a`, "4007"},
	} {
		script := `document.evaluate(` + literal(t, c.expression) + `, document, null, 7, null).snapshotLength`
		var first, best time.Duration
		for i := range 5 {
			start := time.Now()
			if got := s.eval(script); got != c.want {
				t.Fatalf("%s finds %s nodes, want %s", c.expression, got, c.want)
			}
			took := time.Since(start)
			if i == 0 {
				first, best = took, took
			}
			best = min(best, took)
		}
		t.Logf("%s: %v at first, %v at best", c.expression, first, best)
		if first > 2*time.Second {
			t.Errorf("%s takes %v on 10,020 nodes", c.expression, first)
		}
	}
	s.quiet()
}
