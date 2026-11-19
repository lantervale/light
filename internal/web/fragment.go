package web

import (
	"net/url"
	"strings"

	"github.com/lantervale/ember/ui"
)

// Indicated is the element of doc an address names after its "#", as
// browsers find it: the first whose id is the fragment, else the first a
// of that name, by the fragment as written, then with its escapes read.
// The root stands for the top of the page: an address without a
// fragment, an empty one, and "top" where nothing has that name. It is
// nil for a fragment that names nothing.
func Indicated(doc *ui.Document, address string) *ui.Node {
	root := doc.Body().Parent()
	fragment := Fragment(address)
	if fragment == "" {
		return root
	}
	names := []string{fragment}
	if plain, err := url.PathUnescape(fragment); err == nil && plain != fragment {
		names = append(names, plain)
	}
	for _, name := range names {
		if n := doc.ByID(name); n != nil {
			return n
		}
		if n := anchored(root, name); n != nil {
			return n
		}
	}
	if strings.EqualFold(names[len(names)-1], "top") {
		return root
	}
	return nil
}

// anchored is the first a element under root with that name.
func anchored(root *ui.Node, name string) *ui.Node {
	var found *ui.Node
	root.Walk(func(n *ui.Node) bool {
		if found == nil && !n.IsText() && n.Tag().String() == "a" {
			if of, _ := n.Attr("name"); of == name {
				found = n
			}
		}
		return found == nil
	})
	return found
}

// Reveal scrolls doc so the element address names after its "#" is at
// the top of its view, once it is laid out, and leaves it where it is
// for one that names nothing.
func Reveal(doc *ui.Document, address string) {
	if n := Indicated(doc, address); n != nil {
		doc.ScrollToStart(n)
	}
}

// SamePage reports whether to is a place in the page at from: the same
// address but for what follows a "#", which to has.
func SamePage(from, to string) bool {
	page, _, _ := strings.Cut(from, "#")
	same, _, placed := strings.Cut(to, "#")
	return placed && same == page
}

// Fragment is what follows the "#" of an address, "" for none.
func Fragment(address string) string {
	_, fragment, _ := strings.Cut(address, "#")
	return fragment
}
