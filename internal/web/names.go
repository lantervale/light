package web

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// Prefixes are the prefixes elements were written or made with, by
// element: the tree keeps an element under its local name.
type Prefixes map[*html.Node]string

// AttrName is the name of an attribute as the DOM has it: its
// namespace, "" for none, its prefix and its local name. The tree
// keeps the attributes of XML, XLink and the declarations of
// namespaces as HTML's parser does, under those short names when they
// have their usual prefix, and any other attribute of a namespace
// under its address, its key the name it was written with.
func AttrName(a html.Attribute) (string, string, string) {
	switch a.Namespace {
	case "":
		return "", "", a.Key
	case xmlPrefix:
		return xmlAddress, xmlPrefix, a.Key
	case xmlnsPrefix:
		return xmlnsAddress, xmlnsPrefix, a.Key
	case xlinkPrefix:
		return xlinkAddress, xlinkPrefix, a.Key
	}
	if before, after, found := strings.Cut(a.Key, ":"); found {
		return a.Namespace, before, after
	}
	return a.Namespace, "", a.Key
}

// Namespaced reports whether an attribute is of a namespace under its
// address: one XML or a script gave it, which nothing of HTML reads.
func Namespaced(a html.Attribute) bool {
	switch a.Namespace {
	case "", xmlPrefix, xmlnsPrefix, xlinkPrefix:
		return false
	}
	return true
}

// AttrNS is the attribute of n of a local name in a namespace, and
// whether it has one.
func (t *Tree) AttrNS(n *html.Node, space, local string) (string, bool) {
	if space == "" && local == "open" {
		if u := t.shown[n]; u != nil {
			return u.Attr(local)
		}
	}
	if at := attrAt(n, space, local); at >= 0 {
		return n.Attr[at].Val, true
	}
	return "", false
}

// QualifiedAttr is the name of an attribute as its markup writes it.
func QualifiedAttr(a html.Attribute) string {
	switch a.Namespace {
	case xmlPrefix, xmlnsPrefix, xlinkPrefix:
		return a.Namespace + ":" + a.Key
	}
	return a.Key
}

// NamedAttr is an attribute of a namespace, a prefix and a local name
// as the tree keeps it.
func NamedAttr(space, prefix, local, value string) html.Attribute {
	switch {
	case space == "":
		return html.Attribute{Key: local, Val: value}
	case space == xmlAddress && prefix == xmlPrefix, space == xmlnsAddress && prefix == xmlnsPrefix,
		space == xlinkAddress && prefix == xlinkPrefix:
		return html.Attribute{Namespace: prefix, Key: local, Val: value}
	case prefix == "":
		return html.Attribute{Namespace: space, Key: local, Val: value}
	}
	return html.Attribute{Namespace: space, Key: prefix + ":" + local, Val: value}
}

func attrAt(n *html.Node, space, local string) int {
	return slices.IndexFunc(n.Attr, func(a html.Attribute) bool {
		in, _, name := AttrName(a)
		return in == space && name == local
	})
}

// SetAttrNS sets the attribute of n of a local name in a namespace,
// which keeps the prefix it had, and what ui has of one that has no
// namespace.
func (t *Tree) SetAttrNS(n *html.Node, space, prefix, local, value string) {
	if at := attrAt(n, space, local); at < 0 {
		n.Attr = append(n.Attr, NamedAttr(space, prefix, local, value))
	} else {
		n.Attr[at].Val = value
	}
	t.redraw(n)
	if u := t.shown[n]; u != nil && space == "" {
		t.b.attribute(n, u, local, value)
	}
}

// ReplaceAttrNS puts an attribute in the place of the one n has of its
// local name in its namespace, under the prefix given.
func (t *Tree) ReplaceAttrNS(n *html.Node, space, prefix, local, value string) {
	if at := attrAt(n, space, local); at >= 0 {
		n.Attr[at] = NamedAttr(space, prefix, local, value)
	}
	t.SetAttrNS(n, space, prefix, local, value)
}

// RemoveAttrNS takes the attribute of a local name in a namespace off
// n, and off what ui has of it for one that has no namespace.
func (t *Tree) RemoveAttrNS(n *html.Node, space, local string) {
	at := attrAt(n, space, local)
	if at < 0 {
		return
	}
	n.Attr = slices.Delete(n.Attr, at, at+1)
	t.redraw(n)
	if space == "" {
		t.unset(n, local)
	}
}

// Prefixes are the prefixes the page's elements were written with,
// when it was read by XML's rules.
func (t *Tree) Prefixes() Prefixes { return t.prefixes }
