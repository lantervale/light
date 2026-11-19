package dom

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// family is a small tree: ids name its elements.
func family(t *testing.T) *html.Node {
	t.Helper()
	nodes, err := html.ParseFragment(strings.NewReader(`<div id="root" class="page">`+
		`<header id="head" data-role="banner top" lang="en-GB"></header>`+
		`<ul id="list" class="menu wide"><li id="one" class="item">one</li>`+
		`<li id="two" class="item chosen"><a id="link" href="https://site.test/a.PDF"></a></li>`+
		`<li id="three" class="item"><input id="field" type="checkbox" disabled></li><li id="four"></li></ul>`+
		`<p id="para"><span id="span">text</span> and <em id="em"></em></p><p id="empty"><!-- none --></p>`+
		`<x-gauge id="gauge"></x-gauge></div>`), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("the family is %d nodes: %v", len(nodes), err)
	}
	return nodes[0]
}

// each visits root and the elements under it, in the page's order.
func each(root *html.Node, visit func(*html.Node)) {
	visit(root)
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			visit(n)
		}
		return true
	})
}

func TestSelectorsMatchAsABrowsersDo(t *testing.T) {
	root := family(t)
	view := matcher{
		checked: func(n *html.Node) bool { return idOf(n) == "field" },
		focused: func(n *html.Node) bool { return idOf(n) == "link" },
		scope:   root,
	}
	for selector, want := range map[string]string{
		"li":                              "one two three four",
		"LI.item":                         "one two three",
		"*":                               "root head list one two link three field four para span em empty gauge",
		"#two":                            "two",
		".item.chosen":                    "two",
		"ul > li":                         "one two three four",
		"div li a":                        "link",
		"header + ul":                     "list",
		"#one ~ li":                       "two three four",
		"#one + li":                       "two",
		"p > span, p > em":                "span em",
		"[data-role]":                     "head",
		"[data-role=banner]":              "",
		`[data-role="banner top"]`:        "head",
		"[data-role~=top]":                "head",
		"[lang|=en]":                      "head",
		`[href^="https:"]`:                "link",
		`[href$=".pdf" i]`:                "link",
		`[href$=".pdf"]`:                  "",
		"[href*=site]":                    "link",
		"input[type=checkbox]:checked":    "field",
		":disabled":                       "field",
		"input:enabled":                   "",
		"li:first-child":                  "one",
		"li:last-child":                   "four",
		"li:nth-child(2)":                 "two",
		"li:nth-child(odd)":               "one three",
		"li:nth-child(2n)":                "two four",
		"li:nth-child(-n+2)":              "one two",
		"li:nth-last-child(1)":            "four",
		"p:first-of-type":                 "para",
		"p:last-of-type":                  "empty",
		"p:nth-of-type(2)":                "empty",
		"em:only-of-type":                 "em",
		"a:only-child":                    "link",
		"li:not(.item)":                   "four",
		"li:not(.chosen, #four)":          "one three",
		":is(header, ul) + ul, :where(p)": "list para empty",
		"p:empty":                         "empty",
		"li:has(> a)":                     "two",
		":is(ns|li, li.chosen, :unheard)": "two",
		"li:where(!!, [a=5], .chosen)":    "two",
		"ul:has(.chosen)":                 "list",
		":root":                           "",
		"li::before, #one:after":          "",
		`[lang="en-GB`:                    "head",
		`#\6f ne, [lang=en\-GB], *|em`:    "head one em",
		"header:lang(en), :lang(fr)":      "head",
		":scope > p":                      "para empty",
		"a:focus":                         "link",
		"a:link, a:any-link":              "link",
		"a:hover, a:visited":              "",
		"x-gauge":                         "gauge",
		"div":                             "root",
		"  ul   >   .chosen  ":            "two",
		`#one\:x, .a\.b`:                  "",
	} {
		parsed, err := parseSelectors(selector)
		if err != nil {
			t.Errorf("%s: %v", selector, err)
			continue
		}
		var got []string
		each(root, func(n *html.Node) {
			if parsed.matches(n, view) {
				got = append(got, idOf(n))
			}
		})
		if strings.Join(got, " ") != want {
			t.Errorf("%s matches %q, want %q", selector, strings.Join(got, " "), want)
		}
	}
}

func TestASelectorThatIsNoneIsRefused(t *testing.T) {
	for _, selector := range []string{"", "li >", "> li", "li:unheard-of", "[a=", "li::unheard-of", "#", ".", "li,,p",
		".5", "[a=5]", "ns|li", "|", "li:lang()", "li:nth-child(x)", ":not(", "a b)"} {
		if parsed, err := parseSelectors(selector); err == nil {
			t.Errorf("%q is read as %v", selector, parsed)
		}
	}
}

func TestClosestIsTheNearestAncestorMatching(t *testing.T) {
	root := family(t)
	view := matcher{scope: root}
	var link *html.Node
	each(root, func(n *html.Node) {
		if idOf(n) == "link" {
			link = n
		}
	})
	parsed, _ := parseSelectors("ul, .page")
	var chain []string
	for n := link; n != nil; n = n.Parent {
		if parsed.matches(n, view) {
			chain = append(chain, idOf(n))
		}
	}
	if !slices.Equal(chain, []string{"list", "root"}) {
		t.Errorf("the ancestors matching are %v", chain)
	}
}
