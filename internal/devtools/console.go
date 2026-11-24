package devtools

import (
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/netlog"
)

// levels are the chips that leave a level of the console out or in,
// each alone.
//
//nolint:gochecknoglobals // a table
var levels = []struct {
	name  string
	level console.Level
}{
	{"Errors", console.Error}, {"Warnings", console.Warn}, {"Info", console.Info}, {"Logs", console.Log},
	{"Debug", console.Debug},
}

// requested is the level of a line that is a request, unanswered of one
// that failed, and refused the one of a line that is a warning of ui's
// about the page's sheets.
const (
	requested  console.Level = "net"
	unanswered console.Level = "net-failed"
	refused    console.Level = "css"
)

// marks are the pictures a line of each level begins with.
//
//nolint:gochecknoglobals,exhaustive // a table, of the levels that have a mark
var marks = map[console.Level]string{
	console.Info: pictureInfo, console.Warn: pictureWarn, console.Error: pictureError, console.Input: pictureInput,
	console.Result: "result", refused: pictureWarn,
}

const (
	// indent is how far a group of the console moves its lines, and
	// deepest how many groups move them.
	indent  = 12
	deepest = 8
)

// spoke is the Console tab: what the page's scripts said and what
// nothing caught, by level and by text, the requests among them when
// asked for, and the line that evaluates what is typed in it.
type spoke struct {
	p *Panel

	filter, clear, scroll, lines, eval *ui.Node
	// context is the button that names the document the line is
	// evaluated in, and chooses the next; in is what it last read.
	context            *ui.Node
	in                 string
	chips              map[console.Level]*ui.Node
	css, xhr, requests *ui.Node

	// state is the one last shown, heard the console's version as last
	// listed and seen the requests'.
	state       *State
	heard, seen uint64
	// recalled is the line of the history the console's line shows, past
	// the last for the draft, what was typed before the history was
	// walked.
	recalled int
	draft    string
	// follow shows the console's last line the next time it is listed,
	// wherever it is scrolled to.
	follow bool
}

func newConsole(p *Panel) Tool {
	c := &spoke{p: p, chips: make(map[console.Level]*ui.Node)}
	c.eval = ui.Input(ui.ID("dt-eval"), ui.Placeholder("Evaluate in the page"), ui.Attr("aria-label", "Evaluate"),
		ui.Attr("autocomplete", "off"))
	ui.On(c.eval, ui.EventSubmit, c.evaluate)
	c.context = p.Button("dt-context", "top", c.next)
	c.context.AddClass("dt-chip")
	c.lines = ui.Div(ui.ID("dt-lines"))
	c.scroll = ui.Div(ui.ID("dt-console"))
	c.filter = ui.Input(ui.ID("dt-console-filter"), ui.Placeholder("Filter Output"),
		ui.Attr("aria-label", "Filter Output"))
	ui.On(c.filter, ui.EventInput, func() {
		p.state.Said = p.doc.Value(c.filter)
		c.say()
	})
	c.clear = ui.Button(ui.ID("dt-console-clear"), ui.Class("dt-glyph"), ui.Attr("aria-label", "Clear the console"),
		p.Icon("clear"), ui.OnClick(func() {
			p.console.Clear()
			c.say()
		}))
	bar := ui.Div(ui.Class("dt-bar"), c.clear, ui.Label(ui.Class("dt-field"), p.Icon("filter"), c.filter),
		ui.Div(ui.Class("dt-space")))
	for _, chip := range levels {
		c.chips[chip.level] = c.chip("dt-level-"+string(chip.level), chip.name, func() {
			if p.state.Muted == nil {
				p.state.Muted = make(map[console.Level]bool)
			}
			p.state.Muted[chip.level] = !p.state.Muted[chip.level]
		})
		bar.Append(c.chips[chip.level])
	}
	c.css = c.chip("dt-show-css", "CSS", func() { p.state.CSS = !p.state.CSS })
	c.xhr = c.chip("dt-show-xhr", "XHR", func() { p.state.XHR = !p.state.XHR })
	c.requests = c.chip("dt-show-requests", "Requests", func() { p.state.Requests = !p.state.Requests })
	bar.Append(ui.Div(ui.Class("dt-rule")), c.css, c.xhr, c.requests)
	body := ui.Div(ui.ID("dt-said"), c.scroll,
		ui.Div(ui.ID("dt-prompt"), p.Icon("input"), c.eval, c.context), c.keys())
	return Tool{Bar: bar, Body: body, Show: c.show, Update: func(shown bool) {
		if !shown {
			return
		}
		c.within()
		lists := p.state.XHR || p.state.Requests
		if p.console.Version() != c.heard || lists && p.log.Version() != c.seen {
			c.say()
		}
	}}
}

func (c *spoke) chip(id, name string, do func()) *ui.Node {
	chip := c.p.Button(id, name, do)
	chip.AddClass("dt-chip")
	return chip
}

// keys are the keys that walk what was typed in the console, as a menu
// never shown: Alt with the up or down arrow. The arrows alone are the
// line's own, which a ui text field tells nobody of.
func (c *spoke) keys() *ui.Node {
	menu := ui.Div(ui.ID("dt-keys"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Console history"))
	previous, next := historyKeys(runtime.GOOS)
	for _, item := range []struct {
		name, keys string
		step       int
	}{{"Previous line", previous, -1}, {"Next line", next, 1}} {
		menu.Append(ui.Div(ui.Attr("role", "menuitem"), ui.Attr("aria-keyshortcuts", item.keys), ui.Text(item.name),
			ui.OnClick(func() {
				if c.p.state.Open && c.p.state.Shown() == consoleTab && c.p.doc.Focused(c.eval) {
					c.recall(item.step)
				}
			})))
	}
	return menu
}

// historyKeys are the keys that walk the console's history on system, as
// runtime.GOOS names it: Alt with the up or down arrow, and on macOS
// Control with P or N too, as Chrome has them. Elsewhere Control is the
// shortcuts' key, and Control with N opens a window.
func historyKeys(system string) (string, string) {
	if system == "darwin" {
		return "Alt+ArrowUp Control+P", "Alt+ArrowDown Control+N"
	}
	return "Alt+ArrowUp", "Alt+ArrowDown"
}

func (c *spoke) show() {
	state := c.p.state
	if state != c.state {
		c.state, c.recalled, c.draft, c.follow = state, len(state.History), "", true
	}
	for level, chip := range c.chips {
		toggle(chip, "chosen", !state.Muted[level])
	}
	toggle(c.css, "chosen", state.CSS)
	toggle(c.xhr, "chosen", state.XHR)
	toggle(c.requests, "chosen", state.Requests)
	if c.p.doc.Value(c.filter) != state.Said {
		c.p.doc.SetValue(c.filter, state.Said)
	}
	c.within()
	c.say()
}

// listed is the console's lines and, among them by their time, what the
// state adds: the requests of scripts or all of them, a line each, and
// first what ui refused in the page's sheets.
func (c *spoke) listed() []console.Line {
	p := c.p
	all := p.console.Lines()
	if p.state.XHR || p.state.Requests {
		for _, r := range p.log.Rows() {
			if p.state.Requests || r.Type == netlog.XHR {
				all = append(all, asked(r))
			}
		}
		slices.SortStableFunc(all, func(a, b console.Line) int { return a.At.Compare(b.At) })
	}
	if !p.state.CSS {
		return all
	}
	var sheets []console.Line
	for _, w := range p.warnings() {
		sheets = append(sheets, console.Line{Level: refused, Text: w.Error()})
	}
	return append(sheets, all...)
}

// asked is a request as a line of the console: its method, its address
// and what it came to, as a browser's console lists it.
func asked(r netlog.Row) console.Line {
	text := r.Method + " " + r.Address
	if r.Type == netlog.XHR {
		text = "XHR " + text
	}
	switch {
	case r.Blocked:
		text += " [Blocked: " + r.Err + "]"
	case r.Err != "":
		text += " [" + r.Err + "]"
	case r.Status != 0:
		text += fmt.Sprintf(" [HTTP %d %s %s]", r.Status, r.Reason, took(r))
	}
	level := requested
	if r.Failed() {
		level = unanswered
	}
	return console.Line{Level: level, Text: text, At: r.Started}
}

// say fills the body with the console: the lines its filters keep, and
// the line that evaluates what is typed in it. It shows the last line
// when the one before was shown, and leaves a console scrolled up where
// it is.
func (c *spoke) say() {
	p := c.p
	c.heard, c.seen = p.console.Version(), p.log.Version()
	all := c.listed()
	kept := console.Filter(all, p.state.Said, p.state.Muted)
	follow := c.follow || c.followed()
	c.follow = false
	fill(c.lines)
	for _, l := range kept {
		c.lines.Append(c.spoken(l))
	}
	fill(c.scroll)
	unscripted := ""
	if p.host.Unscripted != nil {
		unscripted = p.host.Unscripted()
	}
	if unscripted != "" {
		c.scroll.Append(ui.Div(ui.ID("dt-unscripted"), ui.Class("dt-none"), ui.Text(unscripted)))
		c.eval.SetAttr("disabled", "")
	} else {
		c.eval.RemoveAttr("disabled")
	}
	c.scroll.Append(c.lines)
	switch {
	case len(all) == 0 && unscripted == "":
		c.scroll.Append(ui.Div(ui.Class("dt-none"), ui.Text("This tab's page said nothing.")))
	case len(all) > 0 && len(kept) == 0:
		c.scroll.Append(ui.Div(ui.Class("dt-none"), ui.Text("No line of the console passes the filters.")))
	}
	if follow && len(kept) > 0 {
		p.doc.ScrollIntoView(c.lines.Children()[len(kept)-1])
	}
	p.SetFoot(heard(p.console.Said(), p.console.Count(console.Error), p.console.Count(console.Warn),
		len(all)-len(kept), p.console.Dropped()))
}

// followed reports whether the console shows its last line, as it does
// before it lists any.
func (c *spoke) followed() bool {
	lines := c.lines.Children()
	if len(lines) == 0 {
		return true
	}
	view, shown := c.p.doc.BoxOf(c.scroll)
	last, laid := c.p.doc.BoxOf(lines[len(lines)-1])
	return !shown || !laid || last.Y+last.H/2 <= view.Y+view.H
}

// spoken is one line of the console: the picture of its level, how many
// times it was said when more than once, its text over the calls that
// led to it once unfolded, and where it is from, which leads to its
// source.
func (c *spoke) spoken(l console.Line) *ui.Node {
	p := c.p
	line := ui.Div(ui.Class("dt-line"), ui.Class("dt-"+string(l.Level)))
	if mark, ok := marks[l.Level]; ok {
		line.Append(p.Icon(mark))
	} else {
		line.Append(ui.Span(ui.Class("dt-mark")))
	}
	if depth := min(l.Depth, deepest); depth > 0 {
		ui.Style(fmt.Sprintf("padding-left: %dpx", 8+indent*depth)).Apply(line)
	}
	said := ui.Div(ui.Class("dt-said"), ui.Div(append([]ui.Arg{ui.Class("dt-text")}, c.linked(l.Text)...)...))
	if l.Stack != "" {
		key := l.Text + "\n" + l.Stack
		open, glyph := p.state.Unfolded[key], "folded"
		if open {
			glyph = "open-fold"
			said.Append(ui.Div(ui.Class("dt-stack"), ui.Text(l.Stack)))
		}
		line.Append(ui.Button(ui.Class("dt-fold"), ui.Attr("aria-label", "The calls that led to it"), p.Icon(glyph),
			ui.OnClick(func() {
				if p.state.Unfolded == nil {
					p.state.Unfolded = map[string]bool{}
				}
				p.state.Unfolded[key] = !open
				c.say()
			})))
	}
	if l.Times() > 1 {
		line.Append(ui.Span(ui.Class("dt-times"), ui.Text(strconv.Itoa(l.Times()))))
	}
	where := ui.Span(ui.Class("dt-where"), ui.Text(l.Where()))
	if l.Source != "" && p.host.Source != nil {
		ui.Attr("role", "link").Apply(where)
		ui.TabIndex(0).Apply(where)
		ui.OnClick(func() { p.host.Source(l.Source, l.Line) }).Apply(where)
	}
	if l.Frame != "" {
		line.Append(ui.Span(ui.Class("dt-frame"), ui.Attr("title", "Said by a frame of the page"), ui.Text(l.Frame)))
	}
	line.Append(said, where)
	return line
}

// addressed finds the addresses of the web in a line.
var addressed = regexp.MustCompile(`https?://[^\s"'<>\[\]()]+`)

// linked is a line's text, the addresses in it links that open them.
func (c *spoke) linked(text string) []ui.Arg {
	if c.p.host.Open == nil {
		return []ui.Arg{ui.Text(text)}
	}
	var out []ui.Arg
	at := 0
	for _, found := range addressed.FindAllStringIndex(text, -1) {
		address := strings.TrimRight(text[found[0]:found[1]], ".,;:!?")
		if found[0] > at {
			out = append(out, ui.Text(text[at:found[0]]))
		}
		out = append(out, ui.Span(ui.Class("dt-link"), ui.Attr("role", "link"), ui.TabIndex(0), ui.Text(address),
			ui.OnClick(func() { c.p.host.Open(address) })))
		at = found[0] + len(address)
	}
	if at < len(text) || len(out) == 0 {
		out = append(out, ui.Text(text[at:]))
	}
	return out
}

// heard sums the console up.
func heard(said, errors, warnings, hidden, dropped int) string {
	text := fmt.Sprintf("%s · %s · %s", counted(said, "message"), counted(errors, "error"), counted(warnings, "warning"))
	if hidden > 0 {
		text += fmt.Sprintf(" · %d hidden by the filters", hidden)
	}
	if dropped > 0 {
		text += fmt.Sprintf(" · %d older lines dropped", dropped)
	}
	return text
}

// evaluate runs what the console's line holds in the page, and lists
// it with what it gave.
func (c *spoke) evaluate() {
	p := c.p
	source := strings.TrimSpace(p.doc.Value(c.eval))
	if source == "" || p.host.Evaluate == nil {
		return
	}
	p.doc.SetValue(c.eval, "")
	if last := len(p.state.History) - 1; last < 0 || p.state.History[last] != source {
		p.state.History = append(p.state.History, source)
	}
	c.recalled, c.draft, c.follow = len(p.state.History), "", true
	in := c.within()
	frame := ""
	if in.ID != 0 {
		frame = in.Name
	}
	p.console.Add(console.Line{Level: console.Input, Text: source, Frame: frame})
	var (
		value string
		err   error
	)
	if in.ID != 0 {
		value, err = p.host.EvaluateIn(in.ID, source)
	} else {
		value, err = p.host.Evaluate(source)
	}
	if err != nil {
		p.console.Add(console.Line{Level: console.Error, Text: err.Error(), Frame: frame})
		return
	}
	p.console.Add(console.Line{Level: console.Result, Text: value, Frame: frame})
}

// contexts are the documents the line may be evaluated in: the page's
// alone when the screen names no other.
func (c *spoke) contexts() []Context {
	if c.p.host.Contexts != nil && c.p.host.EvaluateIn != nil {
		if all := c.p.host.Contexts(); len(all) > 0 {
			return all
		}
	}
	return []Context{{Name: "top"}}
}

// within is the document the line is evaluated in, named on its
// button: the page's when the one chosen is no more. The button shows
// only while the page has frames to choose.
func (c *spoke) within() Context {
	all := c.contexts()
	at := slices.IndexFunc(all, func(in Context) bool { return in.ID == c.p.state.Context })
	if at < 0 {
		at, c.p.state.Context = 0, all[0].ID
	}
	in := all[at]
	named := in.Name
	if in.Origin != "" {
		named += " · " + in.Origin
	}
	if len(all) < 2 {
		named = ""
	}
	if named != c.in {
		c.in = named
		toggle(c.context, "shown", named != "")
		c.context.SetText(named)
		c.context.SetAttr("title", "Evaluate in "+named+": press for the next document of the page")
	}
	return in
}

// next has the line evaluated in the document after the one it is.
func (c *spoke) next() {
	all := c.contexts()
	at := slices.IndexFunc(all, func(in Context) bool { return in.ID == c.p.state.Context })
	c.p.state.Context = all[(at+1)%len(all)].ID
	c.within()
}

// recall shows in the console's line what was typed step lines later:
// before, for a step below 0, down to the first, and after, up to what
// was being typed when the history was first walked. The line leaves
// the focus for as long as it takes: a field being typed in keeps its
// text.
func (c *spoke) recall(step int) {
	p := c.p
	history := p.state.History
	c.recalled = min(c.recalled, len(history))
	to := min(max(c.recalled+step, 0), len(history))
	if to == c.recalled {
		return
	}
	if c.recalled == len(history) {
		c.draft = p.doc.Value(c.eval)
	}
	c.recalled = to
	line := c.draft
	if to < len(history) {
		line = history[to]
	}
	p.doc.Focus(c.clear)
	p.doc.SetValue(c.eval, line)
	p.doc.Focus(c.eval)
	p.doc.SetSelectionRange(c.eval, utf8.RuneCountInString(line), utf8.RuneCountInString(line))
}
