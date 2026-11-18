package web

import (
	"strings"

	"golang.org/x/net/html"
)

// XML reports whether the page was read by XML's rules: what its
// scripts set as markup is too.
func (t *Tree) XML() bool { return t.xml }

// Parsed is markup read as a whole HTML document of its own, for a
// script: no script of it runs, so what its noscript elements hold is
// read as elements.
func (t *Tree) Parsed(markup string) *html.Node {
	root, err := html.ParseWithOptions(strings.NewReader(cut(markup)), html.ParseOptionEnableScripting(false))
	if err != nil {
		return &html.Node{Type: html.DocumentNode}
	}
	writtenIn(cut(markup)).restore(root)
	t.apart(root)
	return root
}

// XMLScopeOf is what markup set as what n holds is read within: n's
// namespace, the prefixes declared from it up, and HTML's entities
// when its document has a doctype that gives them.
func XMLScopeOf(n *html.Node) XMLScope {
	scope := XMLScope{Prefixes: map[string]string{}}
	if n.Type == html.ElementNode {
		scope.Default = NamespaceOf(n.Namespace)
	}
	top := n
	for at := n; at != nil; at = at.Parent {
		top = at
		for _, a := range at.Attr {
			if _, declared := scope.Prefixes[a.Key]; a.Namespace == xmlnsPrefix && !declared {
				scope.Prefixes[a.Key] = a.Val
			}
		}
	}
	for c := top.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.DoctypeNode && givesEntities(attr(c, doctypePublic)) {
			scope.Entities = true
		}
	}
	return scope
}

// givesEntities reports whether a doctype of that public identifier
// gives a document the names of HTML's entities, as browsers have it.
func givesEntities(public string) bool {
	return strings.HasPrefix(public, "-//W3C//DTD XHTML ") || strings.HasPrefix(public, "-//W3C//DTD MathML ") ||
		strings.HasPrefix(public, "-//WAPFORUM//DTD XHTML Mobile ")
}

// Unshow lets go of what ui has of n and leaves the rest of it as it
// is: a node that left the page for a document a script made, which
// shows nothing.
func (t *Tree) Unshow(n *html.Node) {
	if u := t.shown[n]; u != nil {
		delete(t.of, u)
		delete(t.shown, n)
	}
}
