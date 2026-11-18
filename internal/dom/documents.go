package dom

import (
	"errors"
	"maps"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/light/internal/web"
)

// A document a script makes (createHTMLDocument, createDocument, new
// Document(), DOMParser, an XMLHttpRequest's, a clone of a document)
// is a tree of its own beside the page's: a document node the page's
// tree does not hold, and the nodes under it. Every node made by it,
// read into it or put in it is its own (owners), and one that moves
// between documents changes hands as it is put. Such a document has
// no window: ui has nothing of its nodes, so nothing of it is fetched,
// shown or focused, and no script of it runs.

// made is what a document a script made says of itself.
type made struct {
	// media is its content type, and address where what it was read
	// from came from, "" for none.
	media, address string
	// xml is one read and written by XML's rules, and created one that
	// createDocument made.
	xml, created bool
}

const (
	htmlMedia = "text/html"
	// blank is the markup of the document createHTMLDocument makes.
	blank = "<!DOCTYPE html><html><head></head><body></body></html>"
)

// home is the document a script made that n is of, n itself for such
// a document, nil for a node of the page.
func (r *Runtime) home(n *html.Node) *html.Node {
	if r.docs[n] != nil {
		return n
	}
	return r.owners[n]
}

// xmlOf reports whether n is of a document read by XML's rules.
func (r *Runtime) xmlOf(n *html.Node) bool {
	if d := r.home(n); d != nil {
		return r.docs[d].xml
	}
	return r.tree.XML()
}

// each visits n, what is under it and what its templates hold.
func (r *Runtime) each(n *html.Node, visit func(*html.Node)) {
	visit(n)
	if content := r.tree.Content(n); content != nil {
		r.each(content, visit)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.each(c, visit)
	}
}

// adopt makes n and what is under it the nodes of a document a script
// made, of the page for nil. What ui had of them is let go of: a
// document a script made shows nothing.
func (r *Runtime) adopt(n, to *html.Node) {
	r.each(n, func(c *html.Node) {
		if to == nil {
			delete(r.owners, c)
			return
		}
		r.owners[c] = to
		r.tree.Unshow(c)
	})
}

// own makes n a node of the document the argument at names, when it
// names one a script made.
func (r *Runtime) own(n *html.Node, a arguments, at int) int {
	if d := r.nodes[int(a.number(at))]; d != nil && r.docs[d] != nil {
		r.adopt(n, d)
	}
	return r.ref(n)
}

// register makes root a document a script made: its nodes are its
// own, and its scripts never run, wherever they are put.
func (r *Runtime) register(root *html.Node, what made) (int, error) {
	count := 0
	r.each(root, func(c *html.Node) {
		count++
		if c != root {
			r.owners[c] = root
		}
		if c.Type == html.ElementNode && c.DataAtom == atom.Script {
			r.ran[c] = true
		}
	})
	if r.made += count; r.made > maxNodes {
		return 0, errTooManyNodes
	}
	r.docs[root] = &what
	return r.ref(root), nil
}

// documents answers what scripts ask of documents, by the name of what
// they ask.
func (r *Runtime) documents(a arguments) (any, error) {
	what := a.text(0)
	switch what {
	case "page":
		return r.ref(r.tree.Root()), nil
	case "new":
		return r.register(&html.Node{Type: html.DocumentNode}, made{media: a.text(1), xml: a.text(1) != htmlMedia,
			created: a.flag(2), address: a.text(3)})
	case "html":
		return r.register(r.tree.Parsed(blank), made{media: htmlMedia})
	case "parse":
		return r.parsed(a)
	case "doctype":
		return r.own(&html.Node{Type: html.DoctypeNode, Data: a.text(1), Attr: []html.Attribute{
			{Key: "public", Val: a.text(2)}, {Key: "system", Val: a.text(3)}}}, a, 4), nil
	case "pi":
		return r.own(&html.Node{Type: html.CommentNode, Namespace: web.Instruction, Data: a.text(2),
			Attr: []html.Attribute{{Key: a.text(1)}}}, a, 3), nil
	case "cdata":
		return r.own(&html.Node{Type: html.TextNode, Namespace: web.CDATASection, Data: a.text(1)}, a, 2), nil
	}
	n, err := r.node(a, 1)
	if err != nil {
		return nil, err
	}
	return r.asked(what, n, a)
}

// asked answers what scripts ask of a document, or of a node about
// its document.
func (r *Runtime) asked(what string, n *html.Node, a arguments) (any, error) {
	switch what {
	case "owner":
		return r.ref(r.owners[n]), nil
	case "xml":
		return r.xmlOf(n), nil
	case "prefix":
		return r.prefixOf(n), nil
	case "info":
		if d := r.docs[n]; d != nil {
			return []any{d.media, d.xml, d.address}, nil
		}
		return []any{r.page.Media, r.tree.XML(), r.page.Address}, nil
	case "parts":
		top := element(n, 0)
		return []int{r.ref(firstElement(n)), r.ref(element(top, atom.Head)), r.ref(element(top, atom.Body))}, nil
	case "byId":
		return r.ref(identified(n, a.text(2))), nil
	case "tagged":
		return r.refs(r.tagged(n, a)), nil
	case "adopt":
		child, missing := r.node(a, 2)
		if missing != nil {
			return nil, missing
		}
		to := n
		if r.docs[n] == nil {
			to = nil
		}
		r.adopt(child, to)
		return nothing()
	case "clone":
		return r.cloned(n, a.flag(2))
	case "markup":
		return web.MarkupXML(n, true, r.prefixOf), nil
	}
	return nil, errors.New("light: no operation of documents named " + what)
}

// firstElement is the first child of n that is an element, whatever
// its namespace.
func firstElement(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

// identified is the first element under root of that id, nil for none.
func identified(root *html.Node, id string) *html.Node {
	var found *html.Node
	if id != "" {
		walk(root, func(n *html.Node) bool {
			if n.Type == html.ElementNode && idOf(n) == id {
				found = n
			}
			return found == nil
		})
	}
	return found
}

// parsed is markup read as a document of its own, by the rules of its
// type: its number and "", or 0 and why XML was not well-formed. A
// body that came as bytes is decoded as an XML document is.
func (r *Runtime) parsed(a arguments) (any, error) {
	media, address := a.text(2), a.text(3)
	source := a.text(1)
	if len(a) > 1 {
		if body, raw := a[1].([]byte); raw {
			source = web.DecodeXML(body, a.text(4))
		}
	}
	root := r.tree.Parsed
	if web.IsXML(media) {
		read, prefixes, err := web.ParseXML(source)
		if err != nil {
			return []any{0, err.Error()}, nil //nolint:nilerr // told to the script, which decides
		}
		maps.Copy(r.prefixes, prefixes)
		root = func(string) *html.Node { return read }
	}
	ref, err := r.register(root(source), made{media: media, address: address, xml: web.IsXML(media)})
	return []any{ref, ""}, err
}

// cloned is a copy of the document n, of what it holds too when deep:
// a document a script made, of the kind n is.
func (r *Runtime) cloned(n *html.Node, deep bool) (any, error) {
	what := made{media: r.page.Media, xml: r.tree.XML()}
	if d := r.docs[n]; d != nil {
		what = *d
	}
	root := &html.Node{Type: html.DocumentNode}
	if deep {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			root.AppendChild(r.copyOf(c, true))
		}
	}
	return r.register(root, what)
}

// lowerASCII is text with its ASCII capitals in lower case, the only
// ones the names of HTML fold.
func lowerASCII(text string) string {
	return strings.Map(func(c rune) rune {
		if c >= 'A' && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}, text)
}

// tagged are the elements under root of a name, as getElementsByTagName
// finds them: every one for "*", HTML's by the name in lower case in a
// document of HTML, the others as they are written, after the prefix
// of one a script made with one. With a namespace
// named, as the tree names it, they are those of that local name in
// it, "*" for any.
func (r *Runtime) tagged(root *html.Node, a arguments) []*html.Node {
	name, lower := a.text(2), lowerASCII(a.text(2))
	space, spaced := "", len(a) > 3 && a[3] != nil
	if spaced {
		space = a.text(3)
	}
	folds := !r.xmlOf(root)
	var found []*html.Node
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case spaced:
			if (space == "*" || n.Namespace == space) && (name == "*" || n.Data == name) {
				found = append(found, n)
			}
		default:
			written := n.Data
			if prefix := r.prefixOf(n); prefix != "" {
				written = prefix + ":" + n.Data
			}
			if name == "*" || written == name && (!folds || n.Namespace != "" || name == lower) ||
				folds && n.Namespace == "" && written == lower {
				found = append(found, n)
			}
		}
		return true
	})
	return found
}

// markupIn is markup read as what n holds: by XML's rules in a
// document of XML, where what is not well-formed is refused whole.
// Its scripts never run, and its nodes are of n's document.
func (r *Runtime) markupIn(text string, n *html.Node) ([]*html.Node, error) {
	var nodes []*html.Node
	if r.xmlOf(n) {
		read, prefixes, err := web.ParseXMLFragment(text, web.XMLScopeOf(n))
		if err != nil {
			return nil, errors.New("SyntaxError: " + err.Error())
		}
		maps.Copy(r.prefixes, prefixes)
		nodes = read
		for _, c := range nodes {
			r.each(c, func(c *html.Node) {
				if c.Type == html.ElementNode && c.DataAtom == atom.Script {
					r.ran[c] = true
				}
			})
		}
	} else {
		nodes = r.markup(text, n.Data)
	}
	if d := r.home(n); d != nil {
		for _, c := range nodes {
			r.adopt(c, d)
		}
	}
	return nodes, nil
}
