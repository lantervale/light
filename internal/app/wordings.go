package app

import (
	"strings"
	"unicode"

	"github.com/lantervale/ember/ui"
)

// The kinds of a part of a passage: text of the block itself, white
// space alone, and what stands in an element of the line, by the mark
// Markdown gives it.
const (
	plainPart = ""
	spacePart = " "
	boldPart  = "**"
	slantPart = "*"
	codePart  = "`"
	heldPart  = "["
)

// The elements named here and elsewhere in the package.
const (
	scriptTag = "script"
	areaTag   = "textarea"
	inputTag  = "input"
	codeTag   = "code"
	strongTag = "strong"
)

// wording is a text of a page that is translated whole: the text nodes
// of one block that nothing but text and the elements of a line stand
// between.
//
// A page's text is replaced without touching the page: only the text
// of its text nodes changes, never an element, an attribute or a
// node's place. The text nodes of a passage do not map one to one onto
// its translation, so a passage is given to the model as a line of
// Markdown, what stands in an element of the line between marks (** **
// for bold, * * for slanted, ` ` for code, [ ] for a link or anything
// else), and the translation is written back part by part only when it
// holds the same marks in the same order: the text between two marks
// then goes into the text nodes between the two elements. A
// translation of another shape is written whole, its marks taken off,
// into the passage's first text node of the block itself, and the
// other nodes are emptied: the words are all there, what was bold or a
// link is not until the original is put back. Code is never rewritten.
type wording struct {
	block *ui.Node
	parts []piece
	// original is the text of each node as the page had it, and wrote
	// what was written in its place, nil while the original shows.
	original, wrote []string
	nodes           []*ui.Node
}

// piece is one or more text nodes of a passage that follow each other
// in one element: nodes[from:to].
type piece struct {
	kind     string
	from, to int
}

// unread are the elements whose text no reader reads as the page's, and
// apart those a line holds that are no text: a passage ends at each.
func unread(tag string) bool {
	switch tag {
	case scriptTag, "style", "noscript", "template", "head", "title", "svg", areaTag, "select", "option", "pre",
		inputTag, "iframe", "object", "canvas", "video", "audio", "math":
		return true
	}
	return false
}

func breaks(tag string) bool { return tag == "br" || tag == "img" || tag == "hr" || tag == "wbr" }

// wordingsOf are the passages of a Document a reader reads, in order:
// not its scripts, its sheets, its form fields, its preformatted text
// or what it does not show, and not a text without a letter.
func wordingsOf(doc *ui.Document) []*wording {
	var out []*wording
	var open *wording
	end := func() {
		if open != nil && open.read(doc) {
			out = append(out, open)
		}
		open = nil
	}
	doc.Body().Walk(func(n *ui.Node) bool {
		if !n.IsText() {
			if tag := n.Tag().String(); unread(tag) || breaks(tag) {
				end()
				return false
			}
			return true
		}
		if block := blockOf(n); open == nil || open.block != block {
			end()
			open = &wording{block: block}
		}
		open.add(n)
		return true
	})
	end()
	return out
}

// add takes a text node into the passage: into its last part when it
// stands in the same element, or is white space alone.
func (p *wording) add(n *ui.Node) {
	p.nodes, p.original = append(p.nodes, n), append(p.original, n.Text())
	at := len(p.nodes) - 1
	kind := p.kindOf(n)
	if last := len(p.parts) - 1; last >= 0 && p.nodes[p.parts[last].from].Parent() == n.Parent() &&
		(kind == spacePart || p.parts[last].kind == kind) {
		p.parts[last].to = at + 1
		return
	}
	p.parts = append(p.parts, piece{kind, at, at + 1})
}

// kindOf is the kind of the part a text node begins.
func (p *wording) kindOf(n *ui.Node) string {
	if strings.TrimSpace(n.Text()) == "" {
		return spacePart
	}
	kind := plainPart
	for e := n.Parent(); e != nil && e != p.block; e = e.Parent() {
		switch e.Tag().String() {
		case codeTag, "kbd", "samp", "var":
			return codePart
		case strongTag, "b":
			kind = boldPart
		case "em", "i":
			if kind != boldPart {
				kind = slantPart
			}
		default:
			if kind == plainPart {
				kind = heldPart
			}
		}
	}
	return kind
}

// read reports whether the passage is one a reader reads: shown, of a
// block that keeps no white space as written, and with a letter that
// is not code.
func (p *wording) read(doc *ui.Document) bool {
	if _, shown := doc.BoxOf(p.block); !shown {
		return false
	}
	if kept, _ := doc.ComputedValue(p.block, "white-space"); strings.HasPrefix(kept, "pre") {
		return false
	}
	for _, part := range p.parts {
		if part.kind != codePart && strings.ContainsFunc(p.text(part), unicode.IsLetter) {
			return true
		}
	}
	return false
}

// text is the original text of a part.
func (p *wording) text(of piece) string { return strings.Join(p.original[of.from:of.to], "") }

// marked are the parts that are text, and whether more than one is:
// a passage of one part is given without a mark.
func (p *wording) marked() bool {
	texts := 0
	for _, part := range p.parts {
		if part.kind != spacePart {
			texts++
		}
	}
	return texts > 1
}

// closing is the mark that ends what kind begins.
func closing(kind string) string {
	if kind == heldPart {
		return "]"
	}
	return kind
}

// markdown is the passage as the model is given it: a line, its white
// space single, what stands in an element of the line between marks.
func (p *wording) markdown() string {
	var line strings.Builder
	marked := p.marked()
	for _, part := range p.parts {
		text := p.text(part)
		core := strings.TrimSpace(text)
		if !marked || part.kind == plainPart || part.kind == spacePart {
			line.WriteString(text)
			continue
		}
		lead, tail, _ := strings.Cut(text, core)
		line.WriteString(lead + part.kind + core + closing(part.kind) + tail)
	}
	return strings.Join(strings.Fields(line.String()), " ")
}

// spoken is a part of a translation: what stands between two marks of
// kind, or outside any.
type spoken struct{ kind, text string }

// saidIn reads a translation as its parts, by the marks of kinds: a
// mark of another kind, or one that is not closed, is text, and the
// address a model gives a link is dropped.
func saidIn(text string, kinds []string) []spoken {
	var out []spoken
	plain := ""
	for text != "" {
		kind, inside, rest := markAt(text, kinds)
		if kind == "" {
			plain += text[:1]
			text = text[1:]
			continue
		}
		out = append(out, spoken{plainPart, plain}, spoken{kind, inside})
		plain, text = "", rest
	}
	return append(out, spoken{plainPart, plain})
}

// markAt is the marked part text begins with: its kind, what is inside
// and what follows; no kind for text that begins with none.
func markAt(text string, kinds []string) (string, string, string) {
	for _, kind := range kinds {
		rest, begins := strings.CutPrefix(text, kind)
		if !begins {
			continue
		}
		inside, after, closed := strings.Cut(rest, closing(kind))
		if !closed || strings.TrimSpace(inside) == "" || inside != strings.TrimSpace(inside) && kind != heldPart {
			continue
		}
		if kind == heldPart {
			after = unlinked(after)
		}
		return kind, strings.TrimSpace(inside), after
	}
	return "", "", ""
}

// unlinked is what follows a link's text without the address between
// brackets a model wrote after it.
func unlinked(after string) string {
	if !strings.HasPrefix(after, "(") {
		return after
	}
	depth := 0
	for i, r := range after {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			return after[i+1:]
		}
	}
	return after
}

// kinds are the marks of the passage's parts, the longest first so
// that bold is read before slanted.
func (p *wording) kinds() []string {
	var out []string
	for _, kind := range []string{boldPart, slantPart, codePart, heldPart} {
		for _, part := range p.parts {
			if part.kind == kind && p.marked() {
				out = append(out, kind)
				break
			}
		}
	}
	return out
}

// write puts a translation in the place of the passage's text: part by
// part when it has the passage's shape, whole in its first text
// otherwise (see wording).
func (p *wording) write(translation string) {
	translation = strings.Join(strings.Fields(translation), " ")
	whole := p.text(piece{from: 0, to: len(p.nodes)})
	core := strings.TrimSpace(whole)
	lead, tail, _ := strings.Cut(whole, core)
	parts := saidIn(translation, p.kinds())
	texts, fits := p.fitted(parts)
	if !fits {
		texts = p.whole(parts)
	}
	if p.kindAt(0) != spacePart {
		texts[0] = lead + strings.TrimLeft(texts[0], " ")
	}
	if last := len(texts) - 1; p.kindAt(last) != spacePart {
		texts[last] = strings.TrimRight(texts[last], " ") + tail
	}
	p.set(texts)
}

// kindAt is the kind of the part the node i is of.
func (p *wording) kindAt(i int) string {
	for _, part := range p.parts {
		if i >= part.from && i < part.to {
			return part.kind
		}
	}
	return plainPart
}

// fitted is the text each node takes of a translation of the passage's
// shape, and whether it has that shape: the same marks in the same
// order, and no text where the passage has no text node of its own.
func (p *wording) fitted(parts []spoken) ([]string, bool) {
	texts := make([]string, len(p.nodes))
	at := 0
	placed := false
	for _, part := range p.parts {
		switch part.kind {
		case spacePart:
			copy(texts[part.from:part.to], p.original[part.from:part.to])
			continue
		case plainPart:
			if placed {
				continue
			}
			texts[part.from], placed = parts[at].text, true
			continue
		}
		if !placed && strings.TrimSpace(parts[at].text) != "" || at+1 >= len(parts) || parts[at+1].kind != part.kind {
			return nil, false
		}
		at, placed = at+2, false
		texts[part.from] = parts[at-1].text
		if part.kind == codePart {
			copy(texts[part.from:part.to], p.original[part.from:part.to])
		}
	}
	return texts, at == len(parts)-1 && (placed || strings.TrimSpace(parts[at].text) == "")
}

// whole is a translation without its marks in the passage's first text
// of the block itself, its first text when it has none, and nothing in
// the others.
func (p *wording) whole(parts []spoken) []string {
	texts := make([]string, len(p.nodes))
	into := -1
	for _, part := range p.parts {
		if part.kind == plainPart || into < 0 && part.kind != spacePart {
			if into = part.from; part.kind == plainPart {
				break
			}
		}
	}
	if into < 0 {
		return texts
	}
	for _, part := range parts {
		texts[into] += part.text
	}
	return texts
}

// set writes texts in the passage's nodes, those still in the page.
func (p *wording) set(texts []string) {
	p.restore()
	p.wrote = texts
	for i, n := range p.nodes {
		if n.Parent() != nil && n.Text() != texts[i] {
			n.SetText(texts[i])
		}
	}
}

// restore puts the passage's original text back, exactly, but in a
// node something else wrote in since.
func (p *wording) restore() {
	for i, n := range p.nodes {
		if p.wrote != nil && n.Parent() != nil && n.Text() == p.wrote[i] && n.Text() != p.original[i] {
			n.SetText(p.original[i])
		}
	}
	p.wrote = nil
}
