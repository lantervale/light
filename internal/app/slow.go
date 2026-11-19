package app

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/tabs"
)

// tardy is a script the user is told runs long: the document it was
// entered in, and the bar that asks whether to wait for it.
type tardy struct {
	live *running
	who  string
	node *ui.Node
}

// laggard names the document of a slow script as the bar does: the host
// of its page.
func laggard(address string, framed bool) string {
	host := ""
	if u, err := url.Parse(address); err == nil {
		host = u.Host
	}
	switch {
	case host == "":
		return "This page"
	case framed:
		return "An embedded page at " + host
	}
	return host
}

// slowed is what a document's scripts tell the browser when one runs
// long, nil for a light with nobody to ask: its scripts are then
// stopped at their budget.
func (s *Screen) slowed(tab *tabs.Tab, live *running, who func() string) func(time.Duration) {
	if !s.opts.Patient {
		return nil
	}
	return func(time.Duration) {
		a := s.modalOf(tab)
		if a.slow != nil {
			s.unslow(a)
		}
		a.slow = &tardy{live: live, who: who()}
	}
}

// pageSlowed and frameSlowed are slowed for a tab's page and for a
// frame's document.
func (s *Screen) pageSlowed(tab *tabs.Tab, live *running) func(time.Duration) {
	return s.slowed(tab, live, func() string { return laggard(live.page.Address, false) })
}

func (s *Screen) frameSlowed(f *frame, live *running) func(time.Duration) {
	return s.slowed(f.set.tab, live, func() string { return laggard(f.origin, true) })
}

// slowness shows the bar of the shown tab's slow script and takes away
// those of the scripts that ended, were stopped or are waited for, and
// of the tabs that are not shown; it reports whether a's script still
// runs, which its tab's name shows.
func (s *Screen) slowness(a *modal, shown bool) bool {
	if a == nil || a.slow == nil {
		return false
	}
	late, told := a.slow.live.rt.Slow()
	switch {
	case !late:
		s.unslow(a)
		a.slow = nil
		return false
	case !shown || !told:
		s.unslow(a)
	case a.slow.node == nil:
		s.slow(a)
	}
	return true
}

// slow shows the bar of a's slow script under the header: the page
// goes on under it. Its Wait button takes the focus, so Enter waits and
// Tab reaches Stop, unless a text is being typed somewhere in the
// window: Tab reaches the bar from there.
func (s *Screen) slow(a *modal) {
	if !s.modals.styled[s.doc] {
		s.modals.styled[s.doc] = true
		s.doc.AddSheet(askingSheet)
	}
	t := a.slow
	chosen := func(act func()) ui.Arg {
		return ui.OnClick(func() {
			act()
			s.unslow(a)
		})
	}
	wait := ui.Button(ui.ID("slow-wait"), ui.Text("Wait"), chosen(t.live.rt.Wait))
	t.node = ui.Div(ui.ID("slow"), ui.Attr("role", "alert"), ui.Attr("aria-label", t.who+" is slow"),
		ui.Span(ui.ID("slow-says"), ui.Text(t.who+" is slow: a script of it has been running for a while.")),
		wait, ui.Button(ui.ID("slow-stop"), ui.Text("Stop it"), chosen(t.live.rt.Stop)))
	toggle(t.node, "dark", s.dark())
	s.status.Parent().Append(t.node)
	if !typing(s.doc) {
		s.doc.Focus(wait)
	}
}

// typing reports whether the focus of doc, or of a Document it frames,
// is where a text is typed: a text field or a text area. Nothing else
// of a page takes typed text in light.
func typing(doc *ui.Document) bool {
	var within func(n *ui.Node) bool
	within = func(n *ui.Node) bool {
		if doc.Focused(n) && takesText(n) {
			return true
		}
		if framed := doc.Frame(n); framed != nil && typing(framed) {
			return true
		}
		return slices.ContainsFunc(n.Children(), within)
	}
	return within(doc.Body())
}

// takesText reports whether n is a field a text is typed in.
func takesText(n *ui.Node) bool {
	switch n.Tag().String() {
	case "textarea":
		return true
	case "input":
		switch kind, _ := n.Attr("type"); strings.ToLower(kind) {
		case "checkbox", "radio", "button", "submit", "reset", "range", "color", "file", "image", "hidden":
			return false
		}
		return true
	}
	return false
}

// unslow takes the bar of a's slow script away.
func (s *Screen) unslow(a *modal) {
	if t := a.slow; t != nil && t.node != nil {
		if parent := t.node.Parent(); parent != nil {
			parent.Remove(t.node)
		}
		t.node = nil
	}
}

// patience is how long the documents of a patient light are waited
// for, the browser's own times unless the run was given others.
func (s *Screen) patience() dom.Patience { return s.opts.Patience }
