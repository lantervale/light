package dom

import (
	"strings"

	"golang.org/x/net/html"
)

// validity is what the page's own rules said of the controls of a
// tree, the last time a selector asked: light's selectors match
// :valid, :invalid, :in-range and :out-of-range from it, since a
// control's pattern is the page's engine's to read.
type validity struct {
	invalid, valid, outOfRange, inRange map[*html.Node]bool
}

// validityTold takes the controls of each state, by their numbers.
func (r *Runtime) validityTold(a arguments) (any, error) {
	set := func(i int) map[*html.Node]bool {
		out := map[*html.Node]bool{}
		refs, _ := a[i].([]any)
		for _, ref := range refs {
			if number, ok := ref.(float64); ok {
				if n := r.nodes[int(number)]; n != nil {
					out[n] = true
				}
			}
		}
		return out
	}
	if len(a) < 4 {
		return nothing()
	}
	r.validity = validity{invalid: set(0), valid: set(1), outOfRange: set(2), inRange: set(3)}
	return nothing()
}

// holds reports whether an element of a set is under n.
func holds(n *html.Node, set map[*html.Node]bool) bool {
	found := false
	walk(n, func(c *html.Node) bool {
		found = found || set[c]
		return !found
	})
	return found
}

func has(n *html.Node, name string) bool {
	_, on := attribute(n, name)
	return on
}

// textInput reports whether n is an input a line of text is typed in:
// the ones readonly and placeholder are for.
func textInput(n *html.Node) bool {
	kind, _ := attribute(n, "type")
	return n.Data == input && !untyped[strings.ToLower(kind)]
}

const (
	fieldset = "fieldset"
	option   = "option"
	optgroup = "optgroup"
	checkbox = "checkbox"
	image    = "image"
	textarea = "textarea"
	radios   = "radio"
	reset    = "reset"
)

//nolint:gochecknoglobals // a constant set
var untyped = map[string]bool{
	"hidden": true, checkbox: true, radios: true, "file": true, submit: true, image: true, reset: true,
	button: true, "range": true, "color": true,
}

// disabled reports whether n is a control that is off: by its own
// attribute, by a fieldset round it that is, unless it is in that
// fieldset's first legend, or by its group for an option.
func disabled(n *html.Node, _ matcher) bool {
	switch n.Data {
	case optgroup:
		return has(n, "disabled")
	case option:
		return has(n, "disabled") || n.Parent != nil && n.Parent.Data == optgroup && has(n.Parent, "disabled")
	case input, button, selects, textarea, fieldset:
	default:
		return false
	}
	if has(n, "disabled") {
		return true
	}
	for in, at := n, n.Parent; at != nil; in, at = at, at.Parent {
		if at.Data != fieldset || !has(at, "disabled") {
			continue
		}
		if legend := firstNamed(at, "legend"); legend == nil || legend != in {
			return true
		}
	}
	return false
}

// firstNamed is the first child of n of a tag.
func firstNamed(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func enabled(n *html.Node, m matcher) bool {
	switch n.Data {
	case input, button, selects, textarea, fieldset, optgroup, option:
		return !disabled(n, m)
	}
	return false
}

// editable reports whether n is in what contenteditable makes
// editable.
func editable(n *html.Node) bool {
	for at := n; at != nil && at.Type == html.ElementNode; at = at.Parent {
		if said, on := attribute(at, "contenteditable"); on {
			switch strings.ToLower(said) {
			case "", "true", "plaintext-only":
				return true
			case "false":
				return false
			}
		}
	}
	return false
}

func readWrite(n *html.Node, m matcher) bool {
	if textInput(n) || n.Data == textarea {
		return !has(n, "readonly") && !disabled(n, m)
	}
	return editable(n)
}

// formOf is the form n is in, nil for none.
func formOf(n *html.Node) *html.Node {
	for at := n.Parent; at != nil; at = at.Parent {
		if at.Type == html.ElementNode && at.Data == "form" {
			return at
		}
	}
	return nil
}

// submits reports whether n is a button that sends its form.
func submits(n *html.Node) bool {
	kind, _ := attribute(n, "type")
	kind = strings.ToLower(kind)
	if n.Data == button {
		return kind != "reset" && kind != button
	}
	return n.Data == input && (kind == "submit" || kind == image)
}

// byDefault reports whether n is what its form takes when nothing
// says otherwise: its first button that sends it, a box or an option
// written as chosen.
func byDefault(n *html.Node, _ matcher) bool {
	switch {
	case n.Data == option:
		return has(n, "selected")
	case n.Data == input && boxed(n):
		return has(n, checked)
	case !submits(n):
		return false
	}
	form := formOf(n)
	if form == nil {
		return false
	}
	var first *html.Node
	walk(form, func(c *html.Node) bool {
		if first == nil && c.Type == html.ElementNode && submits(c) {
			first = c
		}
		return first == nil
	})
	return first == n
}

// boxed reports whether n is a checkbox or a radio button, and radio
// whether it is the second.
func boxed(n *html.Node) bool {
	kind, _ := attribute(n, "type")
	kind = strings.ToLower(kind)
	return n.Data == input && (kind == checkbox || kind == radios)
}

func radio(n *html.Node) bool {
	kind, _ := attribute(n, "type")
	return n.Data == input && strings.EqualFold(kind, radios)
}

// undecided reports whether n shows neither state: a radio button of a
// group none of which is chosen, a progress bar that has no value.
func undecided(n *html.Node, m matcher) bool {
	if n.Data == "progress" {
		return !has(n, "value")
	}
	if !radio(n) || m.checked == nil {
		return false
	}
	name, _ := attribute(n, "name")
	if name == "" {
		return !m.checked(n)
	}
	form, top := formOf(n), n
	for top.Parent != nil {
		top = top.Parent
	}
	if form != nil {
		top = form
	}
	chosen := false
	walk(top, func(c *html.Node) bool {
		if other, _ := attribute(c, "name"); c.Type == html.ElementNode && radio(c) && other == name && formOf(c) == form {
			chosen = chosen || m.checked(c)
		}
		return !chosen
	})
	return !chosen
}

// controlStates are the pseudo-classes of form controls and of what
// can be edited.
func controlStates() map[string]func(*html.Node, matcher) bool {
	field := func(n *html.Node) bool { return n.Data == input || n.Data == selects || n.Data == textarea }
	grouped := func(n *html.Node) bool { return n.Data == "form" || n.Data == fieldset }
	link := func(n *html.Node, _ matcher) bool { return (n.Data == "a" || n.Data == "area") && has(n, "href") }
	return map[string]func(*html.Node, matcher) bool{
		"disabled": disabled, "enabled": enabled,
		"required":   func(n *html.Node, _ matcher) bool { return field(n) && has(n, "required") },
		"optional":   func(n *html.Node, _ matcher) bool { return field(n) && !has(n, "required") },
		"read-write": readWrite,
		"read-only":  func(n *html.Node, m matcher) bool { return !readWrite(n, m) },
		"placeholder-shown": func(n *html.Node, m matcher) bool {
			return (textInput(n) || n.Data == textarea) && has(n, "placeholder") && m.value != nil && m.value(n) == ""
		},
		"default": byDefault, "indeterminate": undecided,
		checked: func(n *html.Node, m matcher) bool {
			if n.Data == option {
				return has(n, "selected")
			}
			return m.checked != nil && m.checked(n)
		},
		"valid": func(n *html.Node, m matcher) bool {
			return m.said.valid[n] || grouped(n) && !holds(n, m.said.invalid)
		},
		"invalid": func(n *html.Node, m matcher) bool {
			return m.said.invalid[n] || grouped(n) && holds(n, m.said.invalid)
		},
		"in-range":     func(n *html.Node, m matcher) bool { return m.said.inRange[n] },
		"out-of-range": func(n *html.Node, m matcher) bool { return m.said.outOfRange[n] },
		"defined":      func(n *html.Node, _ matcher) bool { return n.Namespace != "" || !strings.Contains(n.Data, "-") },
		"link":         link, "any-link": link,
	}
}
