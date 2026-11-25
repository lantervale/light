package dom

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/light/internal/web"
)

// A script element a script puts in the page, or fills, or gives a src,
// is prepared as the HTML Standard says: one whose text is its own runs
// at once, one with a src once fetched, in the order they were put in
// the page unless it is async, and the page's load waits for them.

// scriptsOf are the dynamic scripts on their way: inOrder those that
// run in the order they were prepared, and waited how many of them and
// of the async ones the page's load waits for.
type scriptsOf struct {
	inOrder []*turn
	waited  int
	// forced are the script elements whose async is true while their
	// attribute says nothing: those a script made, until async is set.
	forced map[*html.Node]bool
	// running is the script element whose script runs.
	running *html.Node
}

// isScript reports whether n is a script element, of HTML or of SVG.
func isScript(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && (n.DataAtom == atom.Script && n.Namespace == "" ||
		n.Data == "script" && n.Namespace == svgSpace)
}

const svgSpace = "svg"

// srcOf is the address a script element names: an HTML one's src, an
// SVG one's href.
func srcOf(n *html.Node) (string, bool) {
	if n.Namespace == svgSpace {
		return attribute(n, "href")
	}
	return attribute(n, "src")
}

// childText is the text of the text nodes n holds, those of its
// children only.
func childText(n *html.Node) string {
	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			text.WriteString(c.Data)
		}
	}
	return text.String()
}

// prepare prepares the script element n: nothing for one that ran or
// was about to, one out of the page, one with nothing to run or of a
// type that is no script's.
func (r *Runtime) prepare(n *html.Node) {
	if r.closed || !isScript(n) || r.ran[n] {
		return
	}
	src, external := srcOf(n)
	text := childText(n)
	if !external && text == "" || !r.tree.Connected(n) {
		return
	}
	kind := web.ScriptKind(n)
	if kind == "" {
		return
	}
	r.ran[n] = true
	_, fallback := attribute(n, "nomodule")
	if kind == web.ClassicScript && fallback {
		return
	}
	_, async := attribute(n, "async")
	ordered := !async && !r.scripts.forced[n]
	address := ""
	if external {
		if address = r.resolve(src); strings.Trim(src, " \t\n\f\r") == "" || address == "" {
			r.waiting = append(r.waiting, func() { r.fire(n, "error", nil) })
			return
		}
	}
	switch {
	case kind == web.ImportMap && external:
		r.waiting = append(r.waiting, func() { r.fire(n, "error", nil) })
	case kind == web.ImportMap:
		r.mapImports(text)
	case kind == web.ModuleScript:
		next := r.awaited(ordered)
		r.load(n, text, address, external, func(e *moduleElement) {
			r.waiting = append(r.waiting, func() { r.came(next, func() { r.execute(e) }) })
		})
	case !external:
		r.run(text, r.page.Address, n)
	default:
		next := r.awaited(ordered)
		r.request(Request{Method: methodGet, Address: address, Script: true}, func(answer Response) {
			r.came(next, func() {
				if answer.Err != nil || answer.Status < 200 || answer.Status > 299 {
					r.say("warn", "A script was not run: "+failure(answer), address)
					r.fire(n, "error", nil)
					return
				}
				r.run(answer.Text, answer.Address, n)
				r.fire(n, "load", nil)
			})
		})
	}
}

// awaited is the turn of a dynamic script the load waits for: among
// those that run in order, or on its own.
func (r *Runtime) awaited(ordered bool) *turn {
	next := &turn{}
	r.scripts.waited++
	if ordered {
		r.scripts.inOrder = append(r.scripts.inOrder, next)
	}
	return next
}

// came runs a dynamic script whose turn has come with what it runs:
// an async one at once, one in order once those before it ran.
func (r *Runtime) came(next *turn, run func()) {
	next.ready, next.run = true, run
	ordered := false
	for _, t := range r.scripts.inOrder {
		ordered = ordered || t == next
	}
	if !ordered {
		r.ranDynamic(next)
	}
	for len(r.scripts.inOrder) > 0 && r.scripts.inOrder[0].ready && !r.closed {
		first := r.scripts.inOrder[0]
		r.scripts.inOrder = r.scripts.inOrder[1:]
		r.ranDynamic(first)
	}
	r.proceed()
}

func (r *Runtime) ranDynamic(t *turn) {
	if r.closed {
		return
	}
	t.run()
	r.scripts.waited--
}

// async answers a script element's async, or, set, forgets that it
// was forced.
func (r *Runtime) async(n *html.Node, a arguments) (any, error) {
	if a.flag(1) {
		delete(r.scripts.forced, n)
		return nothing()
	}
	_, set := attribute(n, "async")
	return set || r.scripts.forced[n], nil
}

// parsedScripts are the script elements the parser prepared: those that are
// scripts and have something to run. The others are as a script made
// them: async, and run once they have something and are scripts.
func (r *Runtime) parsedScripts() map[*html.Node]bool {
	prepared := map[*html.Node]bool{}
	for _, s := range r.page.Scripts {
		if s.Node == nil {
			continue
		}
		if _, external := srcOf(s.Node); external || childText(s.Node) != "" {
			prepared[s.Node] = true
		}
	}
	walk(r.tree.Root(), func(n *html.Node) bool {
		if isScript(n) && !prepared[n] {
			if _, async := attribute(n, "async"); !async {
				r.scripts.forced[n] = true
			}
		}
		return true
	})
	return prepared
}

// scriptAttr follows an attribute set on a script element: async is no
// longer forced, and a src it had none of has it prepared.
func (r *Runtime) scriptAttr(n *html.Node, name string, had bool) {
	switch {
	case !isScript(n):
	case name == "async":
		delete(r.scripts.forced, n)
	case !had && (name == "src" && n.Namespace == "" || name == "href" && n.Namespace == svgSpace):
		r.prepare(n)
	}
}
