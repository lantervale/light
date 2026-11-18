package web

import (
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

//nolint:gochecknoglobals // how XML escapes text and attribute values
var (
	xmlText  = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	xmlValue = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\t", "&#9;", "\n", "&#10;",
		"\r", "&#13;")
)

// MarkupXML is n written as XML, or only what it holds, as the DOM
// says a node is serialised: every element closed, one that holds
// nothing in its start tag unless it is HTML's, and each element and
// attribute of a namespace under a prefix declared for it around it,
// the one it was written with where that still names its namespace,
// else one made for it (ns1, ns2…). What is written stands alone, so
// it starts in no namespace whatever n is in. prefix is the prefix an
// element was written or made with.
func MarkupXML(n *html.Node, outer bool, prefix func(*html.Node) string) string {
	w := xmlWriter{prefix: prefix, made: 1}
	declared := declarations{xmlAddress: {xmlPrefix}}
	if outer {
		w.node(n, "", declared)
	} else {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.node(c, "", declared)
		}
	}
	return w.out.String()
}

type xmlWriter struct {
	out    strings.Builder
	prefix func(*html.Node) string
	// made numbers the next prefix made for a namespace none names.
	made int
}

// declarations are the prefixes declared for each namespace where an
// element is written, the nearest last.
type declarations map[string][]string

func (d declarations) copy() declarations {
	out := make(declarations, len(d))
	for space, prefixes := range d {
		out[space] = slices.Clone(prefixes)
	}
	return out
}

// preferred is the prefix to write a name of a namespace with: the one
// it has when that is declared for it, else the nearest declared.
func (d declarations) preferred(space, prefix string) (string, bool) {
	declared := d[space]
	if len(declared) == 0 {
		return "", false
	}
	if slices.Contains(declared, prefix) {
		return prefix, true
	}
	return declared[len(declared)-1], true
}

// generate makes a prefix for a namespace and declares it.
func (w *xmlWriter) generate(d declarations, space string) string {
	prefix := "ns" + strconv.Itoa(w.made)
	w.made++
	d[space] = append(d[space], prefix)
	return prefix
}

func (w *xmlWriter) node(n *html.Node, around string, declared declarations) {
	switch n.Type { //nolint:exhaustive // the tree has no other
	case html.TextNode:
		if n.Namespace == CDATASection {
			w.out.WriteString(cdataOpen + n.Data + "]]>")
			return
		}
		w.out.WriteString(xmlText.Replace(n.Data))
	case html.CommentNode:
		if n.Namespace == Instruction {
			w.out.WriteString("<?" + InstructionTarget(n))
			if n.Data != "" {
				w.out.WriteString(" " + n.Data)
			}
			w.out.WriteString("?>")
			return
		}
		w.out.WriteString("<!--" + n.Data + "-->")
	case html.DoctypeNode:
		w.out.WriteString("<!DOCTYPE " + n.Data)
		public, system := attr(n, doctypePublic), attr(n, doctypeSystem)
		switch {
		case public != "":
			w.out.WriteString(` PUBLIC "` + public + `" "` + system + `"`)
		case system != "":
			w.out.WriteString(` SYSTEM "` + system + `"`)
		}
		w.out.WriteString(">")
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.node(c, around, declared)
		}
	case html.ElementNode:
		w.element(n, around, declared)
	}
}

// declares reports whether a declares the namespace of the names
// written without a prefix: browsers take any attribute called xmlns
// for it, whatever its own namespace.
func declares(space, prefix, local string) bool {
	return prefix == "" && local == xmlnsPrefix && (space == xmlnsAddress || space == "")
}

// record takes the prefixes n declares into declared and local, and
// tells the namespace n declares for names without a prefix.
func record(n *html.Node, declared declarations, local map[string]string) (string, bool) {
	fallback, has := "", false
	for _, a := range n.Attr {
		space, prefix, name := AttrName(a)
		switch {
		case declares(space, prefix, name):
			fallback, has = a.Val, true
		case space != xmlnsAddress || a.Val == xmlAddress || slices.Contains(declared[a.Val], name):
		default:
			declared[a.Val] = append(declared[a.Val], name)
			local[name] = a.Val
		}
	}
	return fallback, has
}

func (w *xmlWriter) element(n *html.Node, around string, declared declarations) {
	declared = declared.copy()
	local := map[string]string{}
	fallback, has := record(n, declared, local)
	space, name, ignore := NamespaceOf(n.Namespace), n.Data, has
	if around == space {
		if space == xmlAddress {
			name = xmlPrefix + ":" + n.Data
		}
		w.out.WriteString("<" + name)
	} else {
		name, around, ignore = w.start(n, around, declared, local, fallback, has)
	}
	w.attributes(n, declared, local, ignore)
	switch {
	case space == xhtmlNamespace && n.FirstChild == nil && voidElements[n.Data]:
		w.out.WriteString(" />")
		return
	case space != xhtmlNamespace && n.FirstChild == nil:
		w.out.WriteString("/>")
		return
	}
	w.out.WriteString(">")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c, around, declared)
	}
	w.out.WriteString("</" + name + ">")
}

// start writes the start of an element whose namespace is not the one
// around it: its name as it is written, the namespace what it holds is
// in, and whether its own declaration of one is left out.
func (w *xmlWriter) start(n *html.Node, around string, declared declarations, local map[string]string,
	fallback string, has bool) (string, string, bool) {
	space, prefix, name, ignore := NamespaceOf(n.Namespace), w.prefix(n), n.Data, false
	candidate, found := declared.preferred(space, prefix)
	if prefix == xmlnsPrefix {
		candidate, found = xmlnsPrefix, true
	}
	switch {
	case found:
		name = candidate + ":" + n.Data
		if has && fallback != xmlAddress {
			around = fallback
		}
		w.out.WriteString("<" + name)
	case prefix != "":
		if _, taken := local[prefix]; taken {
			prefix = w.generate(declared, space)
		} else {
			declared[space] = append(declared[space], prefix)
		}
		name = prefix + ":" + n.Data
		w.out.WriteString("<" + name + " xmlns:" + prefix + `="` + xmlValue.Replace(space) + `"`)
		if has {
			around = fallback
		}
	case !has || fallback != space:
		around, ignore = space, true
		w.out.WriteString("<" + name + ` xmlns="` + xmlValue.Replace(space) + `"`)
	default:
		around = space
		w.out.WriteString("<" + name)
	}
	return name, around, ignore
}

func (w *xmlWriter) attributes(n *html.Node, declared declarations, local map[string]string, ignore bool) {
	for _, a := range n.Attr {
		space, prefix, name := AttrName(a)
		candidate := ""
		switch {
		case declares(space, prefix, name):
			if ignore || a.Val == xmlAddress {
				continue
			}
		case space == xmlnsAddress:
			if kept, here := local[name]; a.Val == xmlAddress || (!here || kept != a.Val) &&
				slices.Contains(declared[a.Val], name) {
				continue
			}
			candidate = declared.candidate(space, prefix)
		case space != "":
			var found bool
			if candidate, found = declared.preferred(space, prefix); !found {
				candidate = w.generate(declared, space)
				w.out.WriteString(" xmlns:" + candidate + `="` + xmlValue.Replace(space) + `"`)
			}
		}
		w.out.WriteString(" ")
		if candidate != "" {
			w.out.WriteString(candidate + ":")
		}
		w.out.WriteString(name + `="` + xmlValue.Replace(a.Val) + `"`)
	}
}

// candidate is the prefix a declaration of a prefix is written with.
func (d declarations) candidate(space, prefix string) string {
	if prefix == xmlnsPrefix {
		return prefix
	}
	candidate, _ := d.preferred(space, prefix)
	return candidate
}

//nolint:gochecknoglobals // a constant set
var voidElements = map[string]bool{
	"area": true, "base": true, "basefont": true, "bgsound": true, "br": true, "col": true, "embed": true,
	"frame": true, "hr": true, "img": true, "input": true, "keygen": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}

// InstructionTarget is the target of a processing instruction.
func InstructionTarget(n *html.Node) string {
	if len(n.Attr) == 0 {
		return ""
	}
	return n.Attr[0].Key
}
