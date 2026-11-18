package dom

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// errSelector is a selector that is none.
var errSelector = errors.New("is not a valid selector")

// matcher is what matching asks of the page beside its tree.
type matcher struct {
	checked, focused func(*html.Node) bool
	// scope is the element :scope stands for.
	scope *html.Node
}

// selectors is a selector list: an element matches when one of them
// does.
type selectors []complexSelector

// complexSelector is compounds joined by combinators, read left to
// right: combinators[i] joins compounds[i] and compounds[i+1].
type complexSelector struct {
	compounds   []compound
	combinators []byte
}

// compound is the tests one element passes: its tag, "" for any, and
// the rest.
type compound struct {
	tag   string
	tests []func(n *html.Node, m matcher) bool
}

func (s selectors) matches(n *html.Node, m matcher) bool {
	for _, c := range s {
		if c.matches(n, m, len(c.compounds)-1) {
			return true
		}
	}
	return false
}

// matches reports whether n passes the compound at and what is left of
// it does, through the combinator between.
func (c complexSelector) matches(n *html.Node, m matcher, at int) bool {
	if n == nil || n.Type != html.ElementNode || !c.compounds[at].matches(n, m) {
		return false
	}
	if at == 0 {
		return true
	}
	switch c.combinators[at-1] {
	case '>':
		return c.matches(n.Parent, m, at-1)
	case '+':
		return c.matches(previous(n), m, at-1)
	case '~':
		for p := previous(n); p != nil; p = previous(p) {
			if c.matches(p, m, at-1) {
				return true
			}
		}
		return false
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if c.matches(p, m, at-1) {
			return true
		}
	}
	return false
}

func (c compound) matches(n *html.Node, m matcher) bool {
	if c.tag != "" && n.Data != c.tag && (n.Namespace == "" || !strings.EqualFold(n.Data, c.tag)) {
		return false
	}
	for _, test := range c.tests {
		if !test(n, m) {
			return false
		}
	}
	return true
}

// previous is the element before n among its parent's.
func previous(n *html.Node) *html.Node {
	for c := n.PrevSibling; c != nil; c = c.PrevSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

// idOf is n's id, "" for none.
func idOf(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "id" {
			return a.Val
		}
	}
	return ""
}

// attribute is n's attribute name, and whether it has one.
func attribute(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// classed reports whether class is one of n's.
func classed(n *html.Node, class string) bool {
	all, _ := attribute(n, "class")
	for at := strings.Index(all, class); at >= 0; {
		end := at + len(class)
		if (at == 0 || space(all[at-1])) && (end == len(all) || space(all[end])) {
			return true
		}
		next := strings.Index(all[end:], class)
		if next < 0 {
			return false
		}
		at = end + next
	}
	return false
}

func space(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// reader reads a selector.
type reader struct {
	src string
	at  int
}

// parseSelectors reads a selector list such as "nav > .item, #main".
func parseSelectors(src string) (selectors, error) {
	r := &reader{src: strings.TrimSpace(src)}
	list, err := r.list()
	if err == nil && r.at < len(r.src) {
		err = errSelector
	}
	if err != nil {
		return nil, fmt.Errorf("%q %w", src, errSelector)
	}
	return list, nil
}

func (r *reader) peek() byte {
	if r.at < len(r.src) {
		return r.src[r.at]
	}
	return 0
}

func (r *reader) space() bool {
	start := r.at
	for r.at < len(r.src) && unicode.IsSpace(rune(r.src[r.at])) {
		r.at++
	}
	return r.at > start
}

// list reads selectors up to the end or a closing parenthesis.
func (r *reader) list() (selectors, error) {
	var list selectors
	for {
		r.space()
		c, err := r.complex()
		if err != nil {
			return nil, err
		}
		list = append(list, c)
		r.space()
		if r.peek() != ',' {
			return list, nil
		}
		r.at++
	}
}

func (r *reader) complex() (complexSelector, error) {
	var c complexSelector
	for {
		one, err := r.compound()
		if err != nil {
			return c, err
		}
		c.compounds = append(c.compounds, one)
		spaced := r.space()
		switch next := r.peek(); {
		case next == '>' || next == '+' || next == '~':
			r.at++
			r.space()
			c.combinators = append(c.combinators, next)
		case spaced && next != 0 && next != ',' && next != ')':
			c.combinators = append(c.combinators, ' ')
		default:
			return c, nil
		}
	}
}

func (r *reader) compound() (compound, error) {
	var c compound
	start := r.at
	if r.peek() == '*' {
		r.at++
	} else {
		c.tag = strings.ToLower(r.name())
	}
	for {
		test, err := r.simple()
		if err != nil {
			return c, err
		}
		if test == nil {
			break
		}
		c.tests = append(c.tests, test)
	}
	if r.at == start {
		return c, errSelector
	}
	return c, nil
}

// name reads an identifier, its escapes undone.
func (r *reader) name() string {
	var b strings.Builder
	for r.at < len(r.src) {
		c := r.src[r.at]
		switch {
		case c == '\\' && r.at+1 < len(r.src):
			b.WriteByte(r.src[r.at+1])
			r.at += 2
		case c == '-' || c == '_' || c >= 0x80 || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)):
			b.WriteByte(c)
			r.at++
		default:
			return b.String()
		}
	}
	return b.String()
}

// simple reads one test of a compound, nil at its end.
func (r *reader) simple() (func(*html.Node, matcher) bool, error) {
	switch r.peek() {
	case '#':
		r.at++
		id := r.name()
		if id == "" {
			return nil, errSelector
		}
		return func(n *html.Node, _ matcher) bool { return idOf(n) == id }, nil
	case '.':
		r.at++
		class := r.name()
		if class == "" {
			return nil, errSelector
		}
		return func(n *html.Node, _ matcher) bool { return classed(n, class) }, nil
	case '[':
		r.at++
		return r.attribute()
	case ':':
		r.at++
		return r.pseudo()
	}
	return nil, nil //nolint:nilnil // no test left to read is no error
}

// attribute reads what follows "[" up to its "]".
func (r *reader) attribute() (func(*html.Node, matcher) bool, error) {
	r.space()
	name := strings.ToLower(r.name())
	r.space()
	op := ""
	for strings.IndexByte("~|^$*=", r.peek()) >= 0 && r.peek() != 0 {
		op += string(r.peek())
		r.at++
	}
	value, fold := "", false
	if op != "" {
		r.space()
		value = r.value()
		r.space()
		if c := r.peek(); c == 'i' || c == 'I' {
			fold = true
			r.at++
			r.space()
		}
	}
	if name == "" || r.peek() != ']' {
		return nil, errSelector
	}
	r.at++
	compare, known := comparisons()[op]
	if !known {
		return nil, errSelector
	}
	return func(n *html.Node, _ matcher) bool {
		got, has := attribute(n, name)
		if fold {
			got, value = strings.ToLower(got), strings.ToLower(value)
		}
		return has && compare(got, value)
	}, nil
}

// comparisons are the attribute operators.
func comparisons() map[string]func(got, want string) bool {
	return map[string]func(got, want string) bool{
		"":   func(string, string) bool { return true },
		"=":  func(got, want string) bool { return got == want },
		"^=": func(got, want string) bool { return want != "" && strings.HasPrefix(got, want) },
		"$=": func(got, want string) bool { return want != "" && strings.HasSuffix(got, want) },
		"*=": func(got, want string) bool { return want != "" && strings.Contains(got, want) },
		"~=": func(got, want string) bool {
			for _, word := range strings.Fields(got) {
				if word == want {
					return true
				}
			}
			return false
		},
		"|=": func(got, want string) bool { return got == want || strings.HasPrefix(got, want+"-") },
	}
}

// value reads an attribute's value: quoted, or a name.
func (r *reader) value() string {
	quote := r.peek()
	if quote != '"' && quote != '\'' {
		return r.name()
	}
	r.at++
	var b strings.Builder
	for r.at < len(r.src) && r.src[r.at] != quote {
		if r.src[r.at] == '\\' && r.at+1 < len(r.src) {
			r.at++
		}
		b.WriteByte(r.src[r.at])
		r.at++
	}
	r.at++
	return b.String()
}

// pseudo reads a pseudo-class after its colon.
func (r *reader) pseudo() (func(*html.Node, matcher) bool, error) {
	name := strings.ToLower(r.name())
	if r.peek() != '(' {
		if test, known := states()[name]; known {
			return test, nil
		}
		return nil, errSelector
	}
	r.at++
	var test func(*html.Node, matcher) bool
	var err error
	switch name {
	case "not", "is", "where", "matches":
		test, err = r.nested(name == "not")
	case "has":
		test, err = r.has()
	case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
		test, err = r.nth(strings.Contains(name, "last"), strings.Contains(name, "type"))
	default:
		err = errSelector
	}
	r.space()
	if err != nil || r.peek() != ')' {
		return nil, errSelector
	}
	r.at++
	return test, nil
}

func (r *reader) nested(negated bool) (func(*html.Node, matcher) bool, error) {
	list, err := r.list()
	if err != nil {
		return nil, err
	}
	return func(n *html.Node, m matcher) bool { return list.matches(n, m) != negated }, nil
}

// has reads :has()'s relative selectors: a descendant, or with a
// leading ">" a child.
func (r *reader) has() (func(*html.Node, matcher) bool, error) {
	r.space()
	child := r.peek() == '>'
	if child {
		r.at++
	}
	list, err := r.list()
	if err != nil {
		return nil, err
	}
	return func(n *html.Node, m matcher) bool {
		if child {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if list.matches(c, m) {
					return true
				}
			}
			return false
		}
		return !walk(n, func(d *html.Node) bool { return !list.matches(d, m) })
	}, nil
}

// nth reads an+b, odd or even, and counts among the siblings, those of
// the element's tag only when typed, from the last when last.
func (r *reader) nth(last, typed bool) (func(*html.Node, matcher) bool, error) {
	end := strings.IndexByte(r.src[r.at:], ')')
	if end < 0 {
		return nil, errSelector
	}
	a, b, ok := step(strings.ToLower(strings.Join(strings.Fields(r.src[r.at:r.at+end]), "")))
	if !ok {
		return nil, errSelector
	}
	r.at += end
	return func(n *html.Node, _ matcher) bool {
		at := position(n, last, typed)
		if a == 0 {
			return at == b
		}
		return (at-b)%a == 0 && (at-b)/a >= 0
	}, nil
}

// step reads an+b.
func step(text string) (int, int, bool) {
	switch text {
	case "odd":
		return 2, 1, true
	case "even":
		return 2, 0, true
	}
	before, after, stepped := strings.Cut(text, "n")
	if !stepped {
		b, err := strconv.Atoi(text)
		return 0, b, err == nil
	}
	a := 1
	switch before {
	case "", "+":
	case "-":
		a = -1
	default:
		parsed, err := strconv.Atoi(before)
		if err != nil {
			return 0, 0, false
		}
		a = parsed
	}
	if after == "" {
		return a, 0, true
	}
	b, err := strconv.Atoi(after)
	return a, b, err == nil
}

// position is n's place from 1 among its siblings.
func position(n *html.Node, last, typed bool) int {
	step := func(c *html.Node) *html.Node { return c.PrevSibling }
	if last {
		step = func(c *html.Node) *html.Node { return c.NextSibling }
	}
	at := 1
	for c := step(n); c != nil; c = step(c) {
		if c.Type == html.ElementNode && (!typed || c.Data == n.Data) {
			at++
		}
	}
	return at
}

// states are the pseudo-classes without arguments.
func states() map[string]func(*html.Node, matcher) bool {
	never := func(*html.Node, matcher) bool { return false }
	disabled := func(n *html.Node, _ matcher) bool { _, on := attribute(n, "disabled"); return on }
	link := func(n *html.Node, _ matcher) bool { _, has := attribute(n, "href"); return has && n.Data == "a" }
	nth := func(last, typed bool) func(*html.Node, matcher) bool {
		return func(n *html.Node, _ matcher) bool { return position(n, last, typed) == 1 }
	}
	only := func(typed bool) func(*html.Node, matcher) bool {
		return func(n *html.Node, _ matcher) bool {
			return position(n, false, typed) == 1 && position(n, true, typed) == 1
		}
	}
	empty := func(n *html.Node, _ matcher) bool {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode || c.Type == html.TextNode && c.Data != "" {
				return false
			}
		}
		return true
	}
	return map[string]func(*html.Node, matcher) bool{
		"first-child": nth(false, false), "last-child": nth(true, false), "only-child": only(false),
		"first-of-type": nth(false, true), "last-of-type": nth(true, true), "only-of-type": only(true),
		"empty":    empty,
		"root":     func(n *html.Node, _ matcher) bool { return n.Parent == nil || n.Parent.Type != html.ElementNode },
		"scope":    func(n *html.Node, m matcher) bool { return n == m.scope },
		checked:    func(n *html.Node, m matcher) bool { return m.checked != nil && m.checked(n) },
		"focus":    func(n *html.Node, m matcher) bool { return m.focused != nil && m.focused(n) },
		"disabled": disabled,
		"enabled":  func(n *html.Node, m matcher) bool { return control(n.Data) && !disabled(n, m) },
		"link":     link, "any-link": link,
		"hover": never, "active": never, "visited": never, "target": never, "focus-visible": never,
		"focus-within": never,
	}
}

// control reports whether tag is a form control's.
func control(tag string) bool {
	switch tag {
	case input, button, selects, "textarea", "option", "fieldset":
		return true
	}
	return false
}
