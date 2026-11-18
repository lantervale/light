package web

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html/charset"

	"github.com/lantervale/ember/ui"
)

const (
	tagForm, tagInput, tagButton = "form", "input", "button"
	typeSubmit                   = "submit"
)

// ErrNotSent is why a form that is not sent with a GET goes nowhere.
var ErrNotSent = errors.New("light sends a form with a GET only")

// encoderOf writes text in the encoding a page's came in, which its
// forms are sent in: what its Content-Type says, else what its HTML
// does. A character the encoding has none for is a character reference.
func encoderOf(body []byte, contentType string) func(text string) string {
	enc, _, _ := charset.DetermineEncoding(body, contentType)
	return func(text string) string {
		var out strings.Builder
		for _, r := range text {
			if in, err := enc.NewEncoder().String(string(r)); err == nil {
				out.WriteString(in)
				continue
			}
			out.WriteString("&#" + strconv.Itoa(int(r)) + ";")
		}
		return out.String()
	}
}

// submits makes control, an input or a button, send its form: a click
// on one that submits, and Enter in a field. The page's scripts, when
// they run, are asked first (Tree.Submit) and hear the field's Enter
// themselves (Tree.Heard).
func (b *builder) submits(control *ui.Node, tag string) {
	ui.OnClick(func() {
		form := submittedBy(control)
		switch {
		case form == nil:
		case b.tree != nil && b.tree.Submit != nil:
			b.tree.Submit(form, control)
		default:
			_ = b.send(form, control)
		}
	}).Apply(control)
	if tag != tagInput {
		return
	}
	ui.On(control, ui.EventSubmit, func() {
		if form := FormOf(control); form != nil && (b.tree == nil || b.tree.Submit == nil) {
			_ = b.send(form, Submitter(form))
		}
	})
}

// Send sends form as a browser does, submitter the button that was
// pressed for it, nil for none: the tab opens the address it makes.
func (t *Tree) Send(form, submitter *ui.Node) error { return t.b.send(form, submitter) }

// Submission is the address Send opens.
func (t *Tree) Submission(form, submitter *ui.Node) (string, error) {
	return t.b.submission(form, submitter)
}

// Submitter is the button Enter in a field of form presses: its first
// that submits, nil for a form without one.
func Submitter(form *ui.Node) *ui.Node {
	var first *ui.Node
	form.Walk(func(n *ui.Node) bool {
		if first == nil && !n.IsText() && n != form && submittedBy(n) == form {
			first = n
		}
		return first == nil
	})
	return first
}

func (b *builder) send(form, submitter *ui.Node) error {
	to, err := b.submission(form, submitter)
	if err == nil {
		b.links.Open(to)
	}
	return err
}

// submittedBy is the form a click on control sends, nil for a control
// that sends none: a button or an input that submits, enabled, in a form.
func submittedBy(control *ui.Node) *ui.Node {
	kind := kindOf(control)
	_, disabled := control.Attr("disabled")
	switch tag := control.Tag().String(); {
	case disabled:
	case tag == tagButton && (kind == "" || kind == typeSubmit),
		tag == tagInput && (kind == typeSubmit || kind == "image"):
		return FormOf(control)
	}
	return nil
}

func kindOf(control *ui.Node) string {
	kind, _ := control.Attr("type")
	return strings.ToLower(strings.TrimSpace(kind))
}

// FormOf is the form control is in, nil for none.
func FormOf(control *ui.Node) *ui.Node {
	for at := control.Parent(); at != nil; at = at.Parent() {
		if !at.IsText() && at.Tag().String() == tagForm {
			return at
		}
	}
	return nil
}

// submission is the address a form is sent to with a GET: its action,
// from the page's address, with the names and values of its controls as
// its query, in the page's encoding.
func (b *builder) submission(form, submitter *ui.Node) (string, error) {
	if method, _ := form.Attr("method"); method != "" && !strings.EqualFold(strings.TrimSpace(method), "get") {
		return "", ErrNotSent
	}
	action, _ := form.Attr("action")
	to, err := url.Parse(b.resolve(action))
	if err != nil || to.Host == "" {
		return "", ErrNotSent
	}
	var query strings.Builder
	for _, pair := range b.entries(form, submitter) {
		if query.Len() > 0 {
			query.WriteByte('&')
		}
		query.WriteString(b.escaped(pair[0]) + "=" + b.escaped(pair[1]))
	}
	to.RawQuery = query.String()
	return to.String(), nil
}

// escaped is text as a form writes it in a query: in the page's
// encoding, what that has no character for as a character reference.
func (b *builder) escaped(text string) string {
	if b.encode != nil {
		text = b.encode(text)
	}
	return url.QueryEscape(text)
}

// entries are the names and values a form sends, in the page's order:
// those of its controls that have a name and are not disabled, a box
// only when checked, a button only when it is the submitter.
func (b *builder) entries(form, submitter *ui.Node) [][2]string {
	var out [][2]string
	doc := b.doc
	form.Walk(func(n *ui.Node) bool {
		if n.IsText() || n == form {
			return true
		}
		name, _ := n.Attr("name")
		_, disabled := n.Attr("disabled")
		if name == "" || disabled {
			return true
		}
		value, _ := n.Attr("value")
		switch tag := n.Tag().String(); {
		case tag == tagButton && n == submitter:
			out = append(out, [2]string{name, value})
		case tag == "select", tag == "textarea":
			out = append(out, [2]string{name, doc.Value(n)})
		case tag == tagInput:
			switch kindOf(n) {
			case typeSubmit, "image":
				if n == submitter {
					out = append(out, [2]string{name, value})
				}
			case tagButton, "reset", "file":
			case "hidden":
				out = append(out, [2]string{name, value})
			case "checkbox", "radio":
				if doc.Checked(n) {
					if _, set := n.Attr("value"); !set {
						value = "on"
					}
					out = append(out, [2]string{name, value})
				}
			default:
				out = append(out, [2]string{name, doc.Value(n)})
			}
		}
		return true
	})
	return out
}
