package web

import (
	"strings"

	"golang.org/x/net/html"
)

//nolint:gochecknoglobals // how XML escapes text and attribute values
var (
	xmlText  = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	xmlValue = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\t", "&#9;", "\n", "&#10;",
		"\r", "&#13;")
)

// MarkupXML is n written as XML, or only what it holds: every element
// closed, one that holds nothing in its start tag, and each element
// whose namespace is not the one around it declaring its own. What is
// written stands alone, so its outermost elements declare theirs
// whatever n is in. Elements are written without a prefix.
func MarkupXML(n *html.Node, outer bool) string {
	var out strings.Builder
	if outer {
		writeXML(&out, n, NoNamespace)
	} else {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeXML(&out, c, NoNamespace)
		}
	}
	return out.String()
}

func writeXML(out *strings.Builder, n *html.Node, around string) {
	switch n.Type { //nolint:exhaustive // the tree has no other
	case html.TextNode:
		if n.Namespace == CDATASection {
			out.WriteString(cdataOpen + n.Data + "]]>")
			return
		}
		out.WriteString(xmlText.Replace(n.Data))
	case html.CommentNode:
		if n.Namespace == Instruction {
			out.WriteString("<?" + InstructionTarget(n))
			if n.Data != "" {
				out.WriteString(" " + n.Data)
			}
			out.WriteString("?>")
			return
		}
		out.WriteString("<!--" + n.Data + "-->")
	case html.DoctypeNode:
		out.WriteString("<!DOCTYPE " + n.Data)
		public, system := attr(n, doctypePublic), attr(n, doctypeSystem)
		switch {
		case public != "":
			out.WriteString(` PUBLIC "` + public + `" "` + system + `"`)
		case system != "":
			out.WriteString(` SYSTEM "` + system + `"`)
		}
		out.WriteString(">")
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeXML(out, c, NoNamespace)
		}
	case html.ElementNode:
		writeXMLElement(out, n, around)
	}
}

func writeXMLElement(out *strings.Builder, n *html.Node, around string) {
	out.WriteString("<" + n.Data)
	if n.Namespace != around {
		out.WriteString(` xmlns="` + xmlValue.Replace(NamespaceOf(n.Namespace)) + `"`)
	}
	for _, a := range n.Attr {
		name := a.Key
		switch {
		case a.Namespace == "" && a.Key == xmlnsPrefix:
			continue
		case a.Namespace != "":
			name = a.Namespace + ":" + a.Key
		}
		out.WriteString(" " + name + `="` + xmlValue.Replace(a.Val) + `"`)
	}
	if n.FirstChild == nil {
		out.WriteString("/>")
		return
	}
	out.WriteString(">")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeXML(out, c, n.Namespace)
	}
	out.WriteString("</" + n.Data + ">")
}

// InstructionTarget is the target of a processing instruction.
func InstructionTarget(n *html.Node) string {
	if len(n.Attr) == 0 {
		return ""
	}
	return n.Attr[0].Key
}
