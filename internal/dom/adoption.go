package dom

import (
	"strconv"

	"golang.org/x/net/html"
)

// A node moves between the documents of kin as it does between a page
// and a document its script made: the node is the same, and so is the
// number scripts know it by, which the kin share. What changes is who
// keeps it: the document whose tree shows it, whose sheets style it and
// whose scripts it runs. Every operation on a node is done by its
// keeper, whichever document's script asks.

// keeper is the document of r's kin that keeps n, r itself for a node
// of its own, or of a document that closed.
func (r *Runtime) keeper(n *html.Node) *Runtime {
	if kept := r.keeps[n]; kept != nil && kept != r && !kept.closed {
		return kept
	}
	return r
}

// askedByKin does for a kin the operation it was asked on a node r
// keeps. The object of a node is made where the node is kept, so the
// kin that asks what it is is told who keeps it.
func (r *Runtime) askedByKin(name string, a arguments) (any, error) {
	if name == "tag" {
		return "@" + strconv.Itoa(r.host.Self), nil
	}
	r.sheets.stale = r.sheets.stale || mutates[name]
	was := r.forKin
	r.asked, r.forKin = name, true
	out, err := r.ops[name](a)
	r.forKin = was
	return out, err
}

// pageOr is the number of a node of a document a script made, and for
// none what the page's document is told as: to its own script the
// word, to a kin, whose page is another, the document's number.
func (r *Runtime) pageOr(made *html.Node, word any) any {
	switch {
	case made != nil:
		return r.ref(made)
	case r.forKin:
		return r.ref(r.tree.Root())
	}
	return word
}

// receive makes n and what is under it nodes r keeps, of its page until
// they are put elsewhere.
func (r *Runtime) receive(n *html.Node) {
	if from := r.keeper(n); from != r {
		r.receiveFrom(from, n)
	}
}

// receiveFrom takes n and what is under it from the kin that kept
// them: it lets go of what its ui showed of them, and what it knew of
// them alone comes along, the prefixes they were written with and the
// scripts among them that ran.
func (r *Runtime) receiveFrom(from *Runtime, n *html.Node) {
	prefixes := map[*html.Node]string{}
	from.each(n, func(c *html.Node) {
		if prefix := from.prefixOf(c); prefix != "" {
			prefixes[c] = prefix
		}
	})
	r.tree.Take(from.tree, n)
	r.each(n, func(c *html.Node) {
		if from.ran[c] {
			r.ran[c] = true
		}
		delete(from.ran, c)
		delete(from.owners, c)
		delete(from.prefixes, c)
		if prefix := prefixes[c]; prefix != "" {
			r.prefixes[c] = prefix
		}
		if _, numbered := r.ids[c]; numbered {
			r.keeps[c] = r
		}
	})
}

// copiedFor is a copy of a node a kin keeps, of what it holds too when
// deep, made for the document to, which r keeps: the kin copies it,
// and the copy is r's from the start.
func (r *Runtime) copiedFor(to, n *html.Node, deep bool) (any, error) {
	if r.made++; r.made > maxNodes {
		return nil, errTooManyNodes
	}
	from := r.keeper(n)
	c := from.copyOf(n, deep)
	if from != r {
		r.receiveFrom(from, c)
	}
	if r.docs[to] == nil {
		to = nil
	}
	r.adopt(c, to)
	return r.ref(c), nil
}

// keptBy is the window of the kin that keeps the node a script names:
// the page's own for a node light does not know.
func (r *Runtime) keptBy(a arguments) (any, error) {
	return r.keeper(r.nodes[int(a.number(0))]).host.Self, nil
}

// named is the window a kin names for r's script: each names its own
// window as itself.
func (r *Runtime) named(by *Runtime, window int) int {
	switch {
	case by == r:
	case window == OwnWindow:
		return by.host.Self
	case window == r.host.Self:
		return OwnWindow
	}
	return window
}

// unkept takes out of the numbers the kin share those of the nodes r
// kept that only its own script knew, which leave answered: the others
// stay for the kin whose scripts hold them, out of any page.
func (r *Runtime) unkept(gone any) {
	numbers, _ := gone.([]any)
	for _, number := range numbers {
		id, is := number.(float64)
		if n := r.nodes[int(id)]; is && n != nil && r.keeps[n] == r {
			r.release(int(id))
		}
	}
}
