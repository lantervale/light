package web //nolint:testpackage // reads the tree the parser makes, node by node

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func noPrefix(*html.Node) string { return "" }

func TestAnXMLDocumentKeepsItsNamesAsWritten(t *testing.T) {
	root, prefixes, err := ParseXML(`<p:r xmlns:p="urn:p" xmlns="urn:d" xmlns:xlink="http://www.w3.org/1999/xlink" ` +
		`a="1" p:b="2" xml:lang="en" xlink:href="h"><c p:d="3"/><p:e/></p:r>`)
	if err != nil {
		t.Fatal(err)
	}
	r := root.FirstChild
	var got []string
	for _, a := range r.Attr {
		space, prefix, local := AttrName(a)
		got = append(got, strings.Join([]string{space, prefix, local, QualifiedAttr(a), a.Val}, "|"))
	}
	want := []string{
		"http://www.w3.org/2000/xmlns/|xmlns|p|xmlns:p|urn:p",
		"http://www.w3.org/2000/xmlns/||xmlns|xmlns|urn:d",
		"http://www.w3.org/2000/xmlns/|xmlns|xlink|xmlns:xlink|http://www.w3.org/1999/xlink",
		"||a|a|1",
		"urn:p|p|b|p:b|2",
		"http://www.w3.org/XML/1998/namespace|xml|lang|xml:lang|en",
		"http://www.w3.org/1999/xlink|xlink|href|xlink:href|h",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the attributes are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	c, e := r.FirstChild, r.LastChild
	if prefixes[r] != "p" || prefixes[c] != "" || prefixes[e] != "p" || r.Data != "r" || r.Namespace != "urn:p" ||
		c.Namespace != "urn:d" {
		t.Errorf("the prefixes are %q %q %q, the root %s in %s, c in %s", prefixes[r], prefixes[c], prefixes[e], r.Data,
			r.Namespace, c.Namespace)
	}
	for _, c := range []struct{ space, prefix, local, key, in string }{
		{"", "", "a:b", "a:b", ""},
		{"urn:x", "", "a", "a", "urn:x"},
		{"urn:x", "q", "a", "q:a", "urn:x"},
		{"http://www.w3.org/XML/1998/namespace", "xml", "id", "id", "xml"},
		{"http://www.w3.org/XML/1998/namespace", "other", "id", "other:id", "http://www.w3.org/XML/1998/namespace"},
		{"http://www.w3.org/1999/xlink", "", "href", "href", "http://www.w3.org/1999/xlink"},
	} {
		a := NamedAttr(c.space, c.prefix, c.local, "v")
		space, prefix, local := AttrName(a)
		if a.Key != c.key || a.Namespace != c.in || space != c.space || prefix != c.prefix || local != c.local {
			t.Errorf("%s %s:%s is kept as %q in %q and read back as %s %s:%s", c.space, c.prefix, c.local, a.Key,
				a.Namespace, space, prefix, local)
		}
	}
}

func TestXMLIsWrittenWithThePrefixesItsNamespacesNeed(t *testing.T) {
	for _, c := range []struct{ name, source, want string }{
		{"as written", `<r xmlns="urn:d" xmlns:p="urn:p" a="1"><p:c p:x="2">t</p:c><d/></r>`, ""},
		{"a prefix that is not needed is dropped", `<root xmlns="u1"><p:child xmlns:p="u1"/></root>`,
			`<root xmlns="u1"><child xmlns:p="u1"/></root>`},
		{"a declaration that changes nothing is dropped", `<root><child xmlns=""/></root>`, `<root><child/></root>`},
		{"a namespace declared around is written with its prefix",
			`<root xmlns:x="uri1"><table xmlns="uri1"></table></root>`, `<root xmlns:x="uri1"><x:table xmlns="uri1"/></root>`},
		{"the default namespace is reset", `<root xmlns="urn:bar"><outer xmlns=""><inner>v</inner></outer></root>`, ""},
		{"HTML's elements are closed as HTML closes them",
			`<html xmlns="http://www.w3.org/1999/xhtml"><body><p></p><br/><svg xmlns="http://www.w3.org/2000/svg"/>` +
				`</body></html>`,
			`<html xmlns="http://www.w3.org/1999/xhtml"><body><p></p><br /><svg xmlns="http://www.w3.org/2000/svg"/>` +
				`</body></html>`},
		{"the rest of a document", `<?xml-stylesheet href="a"?><!DOCTYPE r SYSTEM "s"><!--c--><r><![CDATA[<]]>&amp;</r>`,
			`<?xml-stylesheet href="a"?><!DOCTYPE r SYSTEM "s"><!--c--><r><![CDATA[<]]>&amp;</r>`},
		{"XML's own prefix needs no declaration", `<r xml:lang="en"/>`, ""},
	} {
		root, prefixes, err := ParseXML(c.source)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := c.want
		if want == "" {
			want = c.source
		}
		if got := MarkupXML(root, false, func(n *html.Node) string { return prefixes[n] }); got != want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, want)
		}
	}
	made := &html.Node{Type: html.ElementNode, Data: "root", Namespace: "uri1", Attr: []html.Attribute{
		NamedAttr("http://www.w3.org/2000/xmlns/", "xmlns", "p", "uri2"), NamedAttr("uri3", "p", "a", "1"),
		NamedAttr("uri3", "", "b", "2"), NamedAttr("uri4", "q", "c", "3"),
	}}
	child := &html.Node{Type: html.ElementNode, Data: "child", Namespace: "uri5"}
	made.AppendChild(child)
	prefixed := func(*html.Node) string { return "p" }
	want := `<ns1:root xmlns:ns1="uri1" xmlns:p="uri2" xmlns:ns2="uri3" ns2:a="1" ns2:b="2" xmlns:ns3="uri4" ns3:c="3">` +
		`<p:child xmlns:p="uri5"/></ns1:root>`
	if got := MarkupXML(made, true, prefixed); got != want {
		t.Errorf("an element a script made:\n got %s\nwant %s", got, want)
	}
}
