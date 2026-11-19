package dom

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/lantervale/light/internal/web"
)

// escape reads what follows a backslash: up to six hexadecimal digits
// and one space after them, the character of that number, else the
// character itself.
func (r *reader) escape() string {
	digits := 0
	for digits < 6 && r.at+digits < len(r.src) && hexadecimal(r.src[r.at+digits]) {
		digits++
	}
	if digits == 0 {
		_, size := utf8.DecodeRuneInString(r.src[r.at:])
		r.at += size
		return r.src[r.at-size : r.at]
	}
	code, _ := strconv.ParseUint(r.src[r.at:r.at+digits], 16, 32)
	r.at += digits
	if r.at < len(r.src) && space(r.src[r.at]) {
		r.at++
	}
	if code == 0 || code > utf8.MaxRune || code >= 0xD800 && code <= 0xDFFF {
		return string(utf8.RuneError)
	}
	return string(rune(code))
}

func hexadecimal(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// ident reads an identifier, "" where none starts: a digit starts
// none, nor a hyphen before one.
func (r *reader) ident() string {
	c := r.peek()
	if c == '-' && r.at+1 < len(r.src) {
		c = r.src[r.at+1]
		if c == '-' {
			return r.name()
		}
	}
	if c >= '0' && c <= '9' {
		return ""
	}
	return r.name()
}

// ended closes what a selector left open at its end, as CSS does: a
// bracket, a parenthesis.
func (r *reader) ended(closing byte) bool {
	r.space()
	if r.at >= len(r.src) {
		return true
	}
	if r.peek() != closing {
		return false
	}
	r.at++
	return true
}

// spaced reads the namespace before a name, after the name or the star
// that may have been one: any reports a name of any namespace, none of
// no namespace, and a prefix is refused, since a selector a script
// gives declares none.
func (r *reader) spaced(before string, star bool) (bool, bool, error) {
	if r.peek() != '|' || r.at+1 < len(r.src) && r.src[r.at+1] == '=' {
		return false, false, nil
	}
	r.at++
	switch {
	case star:
		return true, false, nil
	case before == "":
		return false, true, nil
	}
	return false, false, errSelector
}

// unspaced reports whether n is an element of no namespace.
func unspaced(n *html.Node, _ matcher) bool { return n.Namespace == web.NoNamespace }

// anyAttribute is the attribute of n of a local name in any namespace,
// and whether it has one.
func anyAttribute(n *html.Node, local string) (string, bool) {
	for _, a := range n.Attr {
		if _, _, name := web.AttrName(a); name == local {
			return a.Val, true
		}
	}
	return "", false
}

// pseudoElements are the pseudo-elements a selector may name: none is
// an element, so none matches, and the old four take one colon.
func pseudoElements() map[string]bool {
	return map[string]bool{
		"before": true, "after": true, "first-line": true, "first-letter": true, "marker": false,
		"placeholder": false, "selection": false, "backdrop": false, "file-selector-button": false,
	}
}

// pseudoElement reads a pseudo-element after its two colons.
func (r *reader) pseudoElement() (func(*html.Node, matcher) bool, error) {
	name := strings.ToLower(r.ident())
	never := func(*html.Node, matcher) bool { return false }
	if r.peek() != '(' {
		if _, known := pseudoElements()[name]; !known {
			return nil, errSelector
		}
		return never, nil
	}
	r.at++
	if name != "slotted" && name != "part" && name != "highlight" {
		return nil, errSelector
	}
	if name == "slotted" {
		if _, err := r.list(); err != nil {
			return nil, err
		}
	} else if r.space(); r.ident() == "" {
		return nil, errSelector
	}
	if !r.ended(')') {
		return nil, errSelector
	}
	return never, nil
}

// lang reads :lang()'s languages: an element matches one that its own
// language is, or begins with before a hyphen.
func (r *reader) lang() (func(*html.Node, matcher) bool, error) {
	var ranges []string
	for {
		r.space()
		one := strings.ToLower(r.value())
		if one == "" {
			return nil, errSelector
		}
		ranges = append(ranges, one)
		r.space()
		if r.peek() != ',' {
			break
		}
		r.at++
	}
	return func(n *html.Node, _ matcher) bool {
		spoken := strings.ToLower(language(n))
		for _, one := range ranges {
			if spoken == one || strings.HasPrefix(spoken, one+"-") {
				return true
			}
		}
		return false
	}, nil
}

// language is the language of n: the nearest lang it or what it is in
// says, XML's before HTML's.
func language(n *html.Node) string {
	for at := n; at != nil; at = at.Parent {
		spoken, said := "", false
		for _, a := range at.Attr {
			switch {
			case a.Key != "lang":
			case a.Namespace == "xml":
				return a.Val
			case a.Namespace == "":
				spoken, said = a.Val, true
			}
		}
		if said {
			return spoken
		}
	}
	return ""
}

// targeted reports whether n is the element the page's address names
// after its "#".
func (r *Runtime) targeted(n *html.Node) bool {
	address := r.here
	if address == "" {
		address = r.page.Address
	}
	_, fragment, found := strings.Cut(address, "#")
	if !found || fragment == "" {
		return false
	}
	id := idOf(n)
	if id == "" || id != fragment && id != unescaped(fragment) {
		return false
	}
	var first *html.Node
	walk(r.tree.Root(), func(c *html.Node) bool {
		if c.Type == html.ElementNode && idOf(c) == id {
			first = c
		}
		return first == nil
	})
	return first == n
}

func unescaped(fragment string) string {
	if plain, err := url.PathUnescape(fragment); err == nil {
		return plain
	}
	return fragment
}

// forgiving reads the list of :is() and :where(), which leaves out the
// selectors it cannot read where another list would be refused.
func (r *reader) forgiving() func(*html.Node, matcher) bool {
	var list selectors
	for {
		r.space()
		start := r.at
		c, err := r.complex()
		r.space()
		if next := r.peek(); err != nil || next != ',' && next != ')' && next != 0 {
			r.at = start
			r.skipped()
		} else {
			list = append(list, c)
		}
		if r.peek() != ',' {
			break
		}
		r.at++
	}
	return func(n *html.Node, m matcher) bool { return list.matches(n, m) }
}

// skipped passes what is left of one selector of a list: up to the
// comma or the parenthesis that ends it, over what brackets, other
// parentheses and strings hold.
func (r *reader) skipped() {
	depth, quote := 0, byte(0)
	for ; r.at < len(r.src); r.at++ {
		switch c := r.src[r.at]; {
		case quote != 0:
			switch c {
			case '\\':
				r.at++
			case quote:
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '\\':
			r.at++
		case c == '(' || c == '[':
			depth++
		case (c == ')' || c == ',') && depth == 0:
			return
		case c == ')' || c == ']':
			depth--
		}
	}
}
