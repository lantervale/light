package web //nolint:testpackage // reads the tree the parser makes, node by node

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// shape is a tree written so a test reads it: an element as
// <namespace|name attributes>, a text in quotes, a CDATA section in
// brackets, a comment as <!-- -->, an instruction as <?target data?>.
func shape(n *html.Node) string {
	var out strings.Builder
	var write func(n *html.Node)
	write = func(n *html.Node) {
		switch n.Type { //nolint:exhaustive // the tree has no other
		case html.TextNode:
			if n.Namespace == CDATASection {
				out.WriteString("[" + n.Data + "]")
				return
			}
			out.WriteString(`"` + n.Data + `"`)
			return
		case html.CommentNode:
			if n.Namespace == Instruction {
				out.WriteString("<?" + InstructionTarget(n) + " " + n.Data + "?>")
				return
			}
			out.WriteString("<!--" + n.Data + "-->")
			return
		case html.DoctypeNode:
			out.WriteString("<!" + n.Data + " " + attr(n, doctypePublic) + " " + attr(n, doctypeSystem) + ">")
			return
		case html.ElementNode:
			out.WriteString("<")
			if n.Namespace != "" {
				out.WriteString(n.Namespace + "|")
			}
			out.WriteString(n.Data)
			for _, a := range n.Attr {
				name := a.Key
				if a.Namespace != "" {
					name = a.Namespace + ":" + a.Key
				}
				out.WriteString(" " + name + "=" + a.Val)
			}
			out.WriteString(">")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			write(c)
		}
		if n.Type == html.ElementNode {
			out.WriteString("</>")
		}
	}
	write(n)
	return out.String()
}

const xhtml = `xmlns="http://www.w3.org/1999/xhtml"`

func TestXMLIsReadByItsOwnRules(t *testing.T) {
	strict := `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" ` +
		`"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`
	for name, c := range map[string]struct{ source, want string }{
		"a CDATA section is text, its markup characters as they are": {
			`<html ` + xhtml + `><style><![CDATA[ p > a { color: red } ]]></style></html>`,
			`<html xmlns=http://www.w3.org/1999/xhtml><style>[ p > a { color: red } ]</></>`,
		},
		"an element closed in its start tag holds nothing, a script too": {
			`<html ` + xhtml + `><script src="a.js"/><div/><p>after</p></html>`,
			`<html xmlns=http://www.w3.org/1999/xhtml><script src=a.js></><div></><p>"after"</></>`,
		},
		"nothing is implied or closed for the page": {
			`<html ` + xhtml + `><table><tr><td><p>a<p>b</p></p></td></tr></table></html>`,
			`<html xmlns=http://www.w3.org/1999/xhtml><table><tr><td><p>"a"<p>"b"</></></></></></>`,
		},
		"names keep their case": {
			`<html ` + xhtml + `><DIV Class="x">a</DIV></html>`,
			`<html xmlns=http://www.w3.org/1999/xhtml><DIV Class=x>"a"</></>`,
		},
		"the five entities of XML and character references": {
			`<r>&lt;&gt;&amp;&quot;&apos;&#65;&#x42;</r>`, `<-|r>"<>&"'AB"</>`,
		},
		"HTML's entities under an XHTML doctype": {
			strict + `<html ` + xhtml + `><p title="&eacute;">a&nbsp;b&hellip;</p></html>`,
			`<!html -//W3C//DTD XHTML 1.0 Strict//EN http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd>` +
				"<html xmlns=http://www.w3.org/1999/xhtml><p title=é>\"a\u00a0b…\"</></>",
		},
		"namespaces by default and by prefix": {
			`<h:html xmlns:h="http://www.w3.org/1999/xhtml" xmlns:s="http://www.w3.org/2000/svg" ` +
				`xmlns:x="http://www.w3.org/1999/xlink" xmlns:o="urn:other"><s:svg><s:a x:href="#b"/></s:svg>` +
				`<math xmlns="http://www.w3.org/1998/Math/MathML"><mi/></math><o:thing o:kind="1" xml:lang="fr"/>` +
				`<plain xmlns=""/></h:html>`,
			`<html xmlns:h=http://www.w3.org/1999/xhtml xmlns:s=http://www.w3.org/2000/svg ` +
				`xmlns:x=http://www.w3.org/1999/xlink xmlns:o=urn:other><svg|svg><svg|a xlink:href=#b></></>` +
				`<math|math xmlns=http://www.w3.org/1998/Math/MathML><math|mi></></><urn:other|thing o:kind=1 xml:lang=fr></>` +
				`<-|plain xmlns=></></>`,
		},
		"comments and instructions around and in the root, the declaration left out": {
			`<?xml version="1.0" encoding="UTF-8"?><!-- before --><?style a="b"?><r><?do it?><!--in--></r><!--after-->`,
			`<!-- before --><?style a="b"?><-|r><?do it?><!--in--></><!--after-->`,
		},
		"white space is kept in the root and left out around it": {
			"  <r> <a/>\n</r>\n", "<-|r>\" \"<-|a></>\"\n\"</>",
		},
		"a doctype of a system identifier, its declarations unread": {
			`<!DOCTYPE r SYSTEM "r.dtd" [<!ELEMENT r EMPTY>]><r/>`, `<!r  r.dtd><-|r></>`,
		},
	} {
		root, err := ParseXML(c.source)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := shape(root); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}

func TestTheElementsOfXHTMLAreThoseHTMLIsReadInto(t *testing.T) {
	root, err := ParseXML(`<html ` + xhtml + `><head><title>T</title></head><body><DIV/>` +
		`<svg xmlns="http://www.w3.org/2000/svg"/>` +
		`<div xmlns="urn:other"/></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	top := root.FirstChild
	body := top.LastChild
	if top.DataAtom != atom.Html || top.FirstChild.DataAtom != atom.Head || body.DataAtom != atom.Body {
		t.Fatalf("html, head and body are %v, %v, %v", top.DataAtom, top.FirstChild.DataAtom, body.DataAtom)
	}
	if upper, other := body.FirstChild, body.LastChild; upper.DataAtom != 0 || other.DataAtom != 0 ||
		body.FirstChild.NextSibling.DataAtom != atom.Svg {
		t.Fatalf("DIV is %v, svg %v, another namespace's div %v", upper.DataAtom, body.FirstChild.NextSibling.DataAtom,
			other.DataAtom)
	}
	if got := titleOf(root); got != "T" {
		t.Fatalf("the title is %q", got)
	}
}

func TestWhatIsNotWellFormedSaysWhere(t *testing.T) {
	for name, c := range map[string]struct {
		source       string
		line, column int
		reason       string
	}{
		"an end tag of another element":                {"<a>\n  <b></a>", 2, 10, "</a> closes the element b"},
		"an element left open":                         {"<a><b></b>", 1, 11, "not closed"},
		"an end tag alone":                             {"<a/></a>", 1, 9, "closes nothing"},
		"a second root":                                {"<a/>\n<b/>", 2, 5, "second root"},
		"text after the root":                          {"<a/>after", 1, 10, "text outside"},
		"text before the root":                         {"before<a/>", 1, 7, "text outside"},
		"no element":                                   {"<!-- only -->", 1, 14, "no element"},
		"an attribute without quotes":                  {"<a b=c/>", 1, 7, "unquoted"},
		"an attribute written twice":                   {`<a b="1" b="2"/>`, 1, 17, "twice"},
		"twice by two prefixes":                        {`<a xmlns:p="u" xmlns:q="u" p:b="1" q:b="2"/>`, 1, 45, "twice"},
		"an entity XML does not have":                  {"<a>&nbsp;</a>", 1, 10, "entity"},
		"a prefix nothing declares":                    {"<p:a/>", 1, 7, "not declared"},
		"an attribute's prefix":                        {`<a p:b="1"/>`, 1, 13, "not declared"},
		"a declaration after the start":                {`<a/><?xml version="1.0"?>`, 1, 26, "declaration"},
		"a doctype after the root":                     {"<a/><!DOCTYPE a>", 1, 17, "doctype"},
		"a markup character in a value":                {`<a b="<"/>`, 1, 8, "<"},
		"an attribute with no prefix before its colon": {`<a :b="c"/>`, 1, 12, "not a name"},
		"an element with no name after its colon":      {`<a:/>`, 1, 6, "not a name"},
		"a comment with two hyphens":                   {"<a><!-- a -- b --></a>", 1, 14, "--"},
		"an unterminated CDATA section":                {"<a><![CDATA[ x </a>", 1, 20, "EOF"},
		"a prefix declared as nothing":                 {`<a xmlns:p=""/>`, 1, 16, "declared"},
		"bytes that are not the encoding":              {"<a>\xff</a>", 1, 5, "UTF-8"},
	} {
		_, err := ParseXML(c.source)
		var where *XMLError
		if !errors.As(err, &where) {
			t.Errorf("%s: read, or failed with %v", name, err)
			continue
		}
		if where.Line != c.line || where.Column != c.column || !strings.Contains(where.Reason, c.reason) {
			t.Errorf("%s: line %d, column %d: %s; want %d, %d and %q", name, where.Line, where.Column, where.Reason,
				c.line, c.column, c.reason)
		}
	}
}

func TestAnXMLDocumentIsDecodedByItsMarkItsHeaderThenItsDeclaration(t *testing.T) {
	latin := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><r>\xe9</r>"
	for name, c := range map[string]struct{ body, contentType, want string }{
		"its declaration": {latin, "application/xhtml+xml", "é"},
		"its header first": {"<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><r>\xc3\xa9</r>",
			"text/xml; charset=utf-8", "é"},
		"its header alone":         {"<r>\xe9</r>", "text/xml; charset=windows-1252", "é"},
		"a UTF-8 mark":             {"\xef\xbb\xbf<r>\xc3\xa9</r>", "text/xml; charset=iso-8859-1", "é"},
		"a UTF-16 mark":            {"\xff\xfe<\x00r\x00>\x00\xe9\x00<\x00/\x00r\x00>\x00", "", "é"},
		"UTF-8 for nothing said":   {"<r>\xc3\xa9</r>", "", "é"},
		"UTF-8 said by its header": {"<r>\xc3\xa9</r>", "application/xml; charset=UTF-8", "é"},
	} {
		root, err := ParseXML(DecodeXML([]byte(c.body), c.contentType))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := root.FirstChild.FirstChild.Data; got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestAFragmentOfXMLIsReadWithinItsElement(t *testing.T) {
	within := XMLScope{Default: xhtmlNamespace, Prefixes: map[string]string{"s": svgAddress}, Entities: true}
	nodes, err := ParseXMLFragment(`text<b>bold</b><s:g/><![CDATA[<]]>&nbsp;<!--c-->`, within)
	if err != nil {
		t.Fatal(err)
	}
	holder := &html.Node{Type: html.DocumentNode}
	for _, n := range nodes {
		holder.AppendChild(n)
	}
	if got, want := shape(holder), "\"text\"<b>\"bold\"</><svg|g></>[<]\"\u00a0\"<!--c-->"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	for _, markup := range []string{"<b>", "</b>", "<p:a/>", "<!DOCTYPE a>", `<?xml version="1.0"?>`} {
		if _, err = ParseXMLFragment(markup, XMLScope{}); err == nil {
			t.Errorf("%s is read", markup)
		}
	}
	if nodes, err = ParseXMLFragment("&nbsp;", XMLScope{}); err == nil {
		t.Errorf("an entity of HTML is read without its doctype: %v", nodes)
	}
}

func TestATreeIsWrittenBackAsXML(t *testing.T) {
	for _, source := range []string{
		`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>a &amp; b</title></head><body><br/>` +
			`<p class="a&quot;b">&lt;<![CDATA[<]]></p><svg xmlns="http://www.w3.org/2000/svg"><g/></svg><!--c--><?t d?>` +
			`</body></html>`,
		`<!DOCTYPE r SYSTEM "r.dtd"><r><a xmlns="urn:a"><b xmlns=""/></a></r>`,
		`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "x.dtd"><html xmlns="http://www.w3.org/1999/xhtml"/>`,
	} {
		root, err := ParseXML(source)
		if err != nil {
			t.Fatal(err)
		}
		if got := MarkupXML(root, false); got != source {
			t.Errorf("written %s\n   read %s", got, source)
		}
	}
	root, _ := ParseXML(`<r xmlns="urn:r"><a><b/>text</a></r>`)
	a := root.FirstChild.FirstChild
	if got := MarkupXML(a, true); got != `<a xmlns="urn:r"><b/>text</a>` {
		t.Errorf("an element written alone is written %s, without its namespace", got)
	}
	if got := MarkupXML(a, false); got != `<b xmlns="urn:r"/>text` {
		t.Errorf("what it holds is written %s, without its namespace", got)
	}
}
