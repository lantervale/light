package dom

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
)

// put sets an attribute of n, and what of n's state it stands for.
func (r *Runtime) put(n *html.Node, name, value string) {
	r.tree.SetAttr(n, name, value)
	r.stated(n, name, value)
}

func (r *Runtime) attr(n *html.Node, a arguments) (any, error) {
	if value, ok := r.tree.Attr(n, a.text(1)); ok {
		return value, nil
	}
	return nothing()
}

func (r *Runtime) setAttr(n *html.Node, a arguments) (any, error) {
	r.put(n, a.text(1), a.text(2))
	return nothing()
}

func (r *Runtime) removeAttr(n *html.Node, a arguments) (any, error) {
	name := a.text(1)
	if u := r.tree.Node(n); u != nil && name == checked {
		r.doc.SetChecked(u, false)
	}
	r.tree.RemoveAttr(n, name)
	return nothing()
}

// attrs are the attributes of n in the order its markup wrote them.
func (r *Runtime) attrs(n *html.Node, _ arguments) (any, error) {
	out := make([][2]string, len(n.Attr))
	for i, a := range n.Attr {
		out[i] = [2]string{web.QualifiedAttr(a), a.Val}
	}
	return out, nil
}

func (r *Runtime) value(n *ui.Node, _ arguments) (any, error) {
	if r.takesText(n) {
		return r.doc.Value(n), nil
	}
	value, set := n.Attr("value")
	if kind, _ := n.Attr("type"); !set && (kind == "checkbox" || kind == "radio") {
		return "on", nil
	}
	return value, nil
}

// takesText reports whether n's value is what was typed in it.
func (r *Runtime) takesText(n *ui.Node) bool {
	switch r.tagOf(n) {
	case "textarea", selects:
		return true
	case input:
		kind, _ := n.Attr("type")
		switch strings.ToLower(kind) {
		case "checkbox", "radio", button, submit, "reset", "hidden", "image":
			return false
		}
		return true
	}
	return false
}

func (r *Runtime) setValue(n *ui.Node, a arguments) (any, error) {
	if r.takesText(n) {
		r.doc.SetValue(n, a.text(1))
	} else {
		n.SetAttr("value", a.text(1))
	}
	return nothing()
}

func (r *Runtime) checked(n *ui.Node, _ arguments) (any, error) { return r.doc.Checked(n), nil }

func (r *Runtime) setChecked(n *ui.Node, a arguments) (any, error) {
	r.doc.SetChecked(n, a.flag(1))
	return nothing()
}

func (r *Runtime) selected(n *ui.Node, _ arguments) (any, error) { return r.doc.Selected(n), nil }

func (r *Runtime) setSelected(n *ui.Node, a arguments) (any, error) {
	r.doc.SetSelected(n, int(a.number(1)))
	return nothing()
}

func (r *Runtime) selectedText(arguments) (any, error) { return r.doc.Selection().Text, nil }

func (r *Runtime) selection(n *ui.Node, _ arguments) (any, error) {
	from, to := r.doc.SelectionRange(n)
	return []int{from, to}, nil
}

// selectRange selects a field's text between two places, to its end
// for -1.
func (r *Runtime) selectRange(n *ui.Node, a arguments) (any, error) {
	from, to := int(a.number(1)), int(a.number(2))
	if to < 0 {
		to = len([]rune(r.doc.Value(n)))
	}
	r.doc.SetSelectionRange(n, from, to)
	return nothing()
}

func (r *Runtime) focus(n *ui.Node, _ arguments) (any, error) {
	if r.connected(n) {
		r.doc.Focus(n)
	}
	return nothing()
}

// blur is asked to take the focus off n, which ui has no way to do
// but by giving it to another element.
func (r *Runtime) blur(*ui.Node, arguments) (any, error) {
	return nothing()
}

// holder is the element of the page that has the focus, nil for none.
func (r *Runtime) holder() *ui.Node {
	if r.focused != nil && r.doc.Focused(r.focused) {
		return r.focused
	}
	var found *ui.Node
	r.doc.Body().Walk(func(n *ui.Node) bool {
		if found == nil && !n.IsText() && r.doc.Focused(n) {
			found = n
		}
		return found == nil
	})
	return found
}

func (r *Runtime) active(arguments) (any, error) { return r.refOf(r.holder()), nil }

// box is where n is in the page's view: left, top, width and height.
func (r *Runtime) box(n *ui.Node, _ arguments) (any, error) {
	b := n.Box()
	return []float32{b.X, b.Y, b.W, b.H}, nil
}

// laid reports whether n is laid out: shown, whatever its size.
func (r *Runtime) laid(n *ui.Node, _ arguments) (any, error) {
	_, shown := r.doc.BoxOf(n)
	return shown, nil
}

func (r *Runtime) reveal(n *ui.Node, _ arguments) (any, error) {
	if r.connected(n) {
		r.doc.ScrollIntoView(n)
	}
	return nothing()
}

func (r *Runtime) at(a arguments) (any, error) {
	at := geom.Vec2{X: float32(a.number(0)), Y: float32(a.number(1))}
	return r.refOf(r.doc.ElementAt(at.Scale(r.doc.Zoom()))), nil
}

// void are the elements written without an end.
//
//nolint:gochecknoglobals // a constant set
var void = map[string]bool{
	"area": true, "base": true, "basefont": true, "bgsound": true, "br": true, "col": true, "embed": true,
	"frame": true, "hr": true, "img": true, input: true, "keygen": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}

// raw are the elements whose text is written as it is.
//
//nolint:gochecknoglobals // a constant set
var raw = map[string]bool{
	"script": true, style: true, "xmp": true, "iframe": true, "noembed": true, "noframes": true, "plaintext": true,
	"noscript": true,
}

// html is the markup of what n holds, of n too when asked for its
// outer markup, as a browser writes it.
func (r *Runtime) html(n *html.Node, a arguments) (any, error) {
	if r.xmlOf(n) {
		return web.MarkupXML(n, a.flag(1), r.prefixOf), nil
	}
	var out strings.Builder
	if a.flag(1) {
		r.write(&out, n)
	} else {
		r.inner(&out, n)
	}
	return out.String(), nil
}

func (r *Runtime) inner(out *strings.Builder, n *html.Node) {
	holder := n
	if content := r.tree.Content(n); content != nil {
		holder = content
	}
	for c := holder.FirstChild; c != nil; c = c.NextSibling {
		r.write(out, c)
	}
}

func (r *Runtime) write(out *strings.Builder, n *html.Node) {
	switch n.Type { //nolint:exhaustive // the tree has no other
	case html.TextNode:
		if p := n.Parent; p != nil && p.Type == html.ElementNode && p.Namespace == "" && raw[p.Data] {
			out.WriteString(n.Data)
		} else {
			out.WriteString(textEscaper.Replace(n.Data))
		}
	case html.CommentNode:
		if n.Namespace == web.Instruction {
			out.WriteString("<?" + web.InstructionTarget(n) + " " + n.Data + ">")
			return
		}
		out.WriteString("<!--" + n.Data + "-->")
	case html.DoctypeNode:
		out.WriteString("<!DOCTYPE " + n.Data + ">")
	case html.DocumentNode:
		r.inner(out, n)
	case html.ElementNode:
		name := r.written(n)
		out.WriteString("<" + name)
		for _, attribute := range n.Attr {
			out.WriteString(" " + web.QualifiedAttr(attribute) + `="` + valueEscaper.Replace(attribute.Val) + `"`)
		}
		out.WriteString(">")
		if n.Namespace == "" && void[n.Data] {
			return
		}
		r.inner(out, n)
		out.WriteString("</" + name + ">")
	}
}

//nolint:gochecknoglobals // how markup escapes text and attribute values
var (
	textEscaper  = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\u00a0", "&nbsp;")
	valueEscaper = strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;", "\u00a0", "&nbsp;")
)
