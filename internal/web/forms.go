package web

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html/charset"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/identity"
)

const (
	tagForm, tagInput, tagButton = "form", "input", "button"
	tagSelect, tagTextarea       = "select", "textarea"
	typeSubmit                   = "submit"
)

// Why a form goes nowhere.
var (
	// ErrMultipart is a form sent as multipart/form-data, the one way
	// to send a file.
	ErrMultipart = errors.New("light sends no form as multipart/form-data, and so no file")
	// ErrDialog is a form that closes the dialog it is in.
	ErrDialog = errors.New("its method is dialog, and light closes no dialog with a form")
	// ErrNoAddress is a form whose action is not of the web.
	ErrNoAddress = errors.New("its action is no http or https address")
)

// The types a form's fields are written in.
const (
	urlencoded = "application/x-www-form-urlencoded"
	multipart  = "multipart/form-data"
	plainText  = "text/plain"
)

// Submission is a form as it is sent: where it goes and, with a POST,
// what it carries.
type Submission struct {
	Address string
	// Form is what a POST carries, nil for a GET, whose fields are in
	// its address.
	Form *fetch.Form
	// Referrer is the address a POST says it comes from.
	Referrer string
}

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
		form := SubmittedBy(control)
		switch {
		case form == nil:
		case b.tree != nil && b.tree.Submit != nil:
			b.tree.Submit(form, control)
		default:
			b.sendOrSay(form, control)
		}
	}).Apply(control)
	if tag != tagInput {
		return
	}
	ui.On(control, ui.EventSubmit, func() {
		if form := FormOf(control); form != nil && (b.tree == nil || b.tree.Submit == nil) {
			b.sendOrSay(form, Submitter(form))
		}
	})
}

// sendOrSay sends form, and has the page's links told of one that is
// not sent.
func (b *builder) sendOrSay(form, submitter *ui.Node) {
	if err := b.send(form, submitter); err != nil && b.links.Unsent != nil {
		b.links.Unsent(err)
	}
}

// Send sends form as a browser does, submitter the button that was
// pressed for it, nil for none: the tab opens the address it makes, or
// is sent there with what a POST carries.
func (t *Tree) Send(form, submitter *ui.Node) error { return t.b.send(form, submitter) }

// Submission is what Send sends.
func (t *Tree) Submission(form, submitter *ui.Node) (Submission, error) {
	return t.b.submission(form, submitter)
}

// Submitter is the button Enter in a field of form presses: its first
// that submits, nil for a form without one.
func Submitter(form *ui.Node) *ui.Node {
	var first *ui.Node
	top(form).Walk(func(n *ui.Node) bool {
		if first == nil && !n.IsText() && n != form && SubmittedBy(n) == form {
			first = n
		}
		return first == nil
	})
	return first
}

func (b *builder) send(form, submitter *ui.Node) error {
	sent, err := b.submission(form, submitter)
	switch {
	case err != nil:
		return err
	case sent.Form != nil && b.links.Post != nil:
		b.links.Post(sent)
	case sent.Form == nil:
		b.links.Open(sent.Address)
	}
	return nil
}

// SubmittedBy is the form a click on control sends, nil for a control
// that sends none: a button or an input that submits, enabled, of a form.
func SubmittedBy(control *ui.Node) *ui.Node {
	if control.IsText() {
		return nil
	}
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

// FormOf is the form control is of, nil for none: the one its form
// attribute names by its id, else the one it is in.
func FormOf(control *ui.Node) *ui.Node {
	if id, named := control.Attr("form"); named {
		var form *ui.Node
		top(control).Walk(func(n *ui.Node) bool {
			if of, _ := n.Attr("id"); form == nil && !n.IsText() && of == id && n.Tag().String() == tagForm {
				form = n
			}
			return form == nil
		})
		return form
	}
	for at := control.Parent(); at != nil; at = at.Parent() {
		if !at.IsText() && at.Tag().String() == tagForm {
			return at
		}
	}
	return nil
}

// top is the element n's page hangs from.
func top(n *ui.Node) *ui.Node {
	for n.Parent() != nil {
		n = n.Parent()
	}
	return n
}

// stated is what the form says of how it is sent under name, which its
// submitter says for it under "form"+name when it does.
func stated(form, submitter *ui.Node, name string) string {
	value, _ := form.Attr(name)
	if submitter != nil {
		if own, says := submitter.Attr("form" + name); says {
			value = own
		}
	}
	return strings.ToLower(strings.TrimSpace(value))
}

// submission is how a form is sent: to its action, from the page's
// address, with the names and values of its controls in the page's
// encoding, as the address's query for a GET and as what a POST carries
// for one, in the type the form asks for.
func (b *builder) submission(form, submitter *ui.Node) (Submission, error) {
	method, kind := stated(form, submitter, "method"), stated(form, submitter, "enctype")
	switch {
	case method == "dialog":
		return Submission{}, ErrDialog
	case method == "post" && kind == multipart:
		return Submission{}, ErrMultipart
	}
	action, _ := form.Attr("action")
	if submitter != nil {
		if own, says := submitter.Attr("formaction"); says {
			action = own
		}
	}
	to, err := url.Parse(b.resolve(action))
	if err != nil || to.Host == "" {
		return Submission{}, ErrNoAddress
	}
	entries := b.entries(form, submitter)
	if method != "post" {
		to.RawQuery = b.query(entries)
		return Submission{Address: to.String()}, nil
	}
	sent := &fetch.Form{Type: urlencoded, Body: []byte(b.query(entries)), Origin: b.base.Scheme + "://" + b.base.Host}
	if kind == plainText {
		var lines strings.Builder
		for _, pair := range entries {
			lines.WriteString(b.encoded(pair[0]) + "=" + b.encoded(pair[1]) + "\r\n")
		}
		sent.Type, sent.Body = plainText, []byte(lines.String())
	}
	return Submission{Address: to.String(), Form: sent, Referrer: identity.Referrer(b.base, to)}, nil
}

// query is the names and values of a form as a query writes them.
func (b *builder) query(entries [][2]string) string {
	var query strings.Builder
	for _, pair := range entries {
		if query.Len() > 0 {
			query.WriteByte('&')
		}
		query.WriteString(url.QueryEscape(b.encoded(pair[0])) + "=" + url.QueryEscape(b.encoded(pair[1])))
	}
	return query.String()
}

// encoded is text in the page's encoding, what that has no character
// for as a character reference.
func (b *builder) encoded(text string) string {
	if b.encode != nil {
		return b.encode(text)
	}
	return text
}

// lines is a text of several lines as a form sends it, each ended as
// the network ends them.
func lines(text string) string {
	return strings.NewReplacer("\r\n", "\r\n", "\r", "\r\n", "\n", "\r\n").Replace(text)
}

// entries are the names and values a form sends, in the page's order:
// those of its controls that have a name and are not disabled, a box
// only when checked, a button only when it is the submitter, a file
// field with no file.
func (b *builder) entries(form, submitter *ui.Node) [][2]string {
	var out [][2]string
	doc := b.doc
	top(form).Walk(func(n *ui.Node) bool {
		if n.IsText() || n == form {
			return true
		}
		name, _ := n.Attr("name")
		_, disabled := n.Attr("disabled")
		tag := n.Tag().String()
		if name == "" || disabled || (tag != tagButton && tag != tagInput && tag != tagSelect && tag != tagTextarea) ||
			FormOf(n) != form {
			return true
		}
		value, _ := n.Attr("value")
		switch {
		case tag == tagButton && n == submitter:
			out = append(out, [2]string{name, value})
		case tag == tagSelect:
			out = append(out, [2]string{name, doc.Value(n)})
		case tag == tagTextarea:
			out = append(out, [2]string{name, lines(doc.Value(n))})
		case tag == tagInput:
			switch kindOf(n) {
			case typeSubmit, "image":
				if n == submitter {
					out = append(out, [2]string{name, value})
				}
			case tagButton, "reset":
			case "file":
				out = append(out, [2]string{name, ""})
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
