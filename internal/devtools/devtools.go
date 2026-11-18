// Package devtools is light's developer tools: a panel docked under a
// tab's page, drawn with ui elements. Its frame is a tool bar of tabs,
// the chosen tab's tool under it and a handle on its top edge that
// sizes it; its tools are the tabs registered with Register: Console
// (console.go), Network (network.go) and Warnings (warnings.go) here.
package devtools

import (
	_ "embed"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/netlog"
)

// Sheet is the panel's look, for the Document it is added to.
//
//go:embed devtools.css
var Sheet string

// Tab is one tab of the panel's tool bar and the tool it shows. A file
// adds its own with Register, from an init or a package variable.
type Tab struct {
	// ID names the tab: its button is dt-tab-<ID>, its tool is in
	// dt-tool-<ID>, and State.Tab holds it while it is the one shown.
	ID, Label string
	// Icon is the tab's picture: what an svg element of 16 by 16 holds,
	// drawn in strokes (see pictures).
	Icon string
	// Order places the tab in the tool bar, the lowest first.
	Order int
	// Build makes the tab's tool for a panel, once, when the panel is
	// made.
	Build func(p *Panel) Tool
}

// Tool is what a tab shows under the tool bar.
type Tool struct {
	// Bar is the tool's own row of buttons, nil for none, and Body what
	// fills the panel under it. Both are made once.
	Bar, Body *ui.Node
	// Show fills them from the panel's state: it runs each time the
	// panel is shown again while the tab is the chosen one.
	Show func()
	// Update runs every frame, shown telling whether the panel is open
	// on this tab. It may be nil.
	Update func(shown bool)
}

//nolint:gochecknoglobals // the tabs every panel is made with
var registered []Tab

// Register adds a tab to the panels made after it.
func Register(tab Tab) {
	registered = append(registered, tab)
	slices.SortStableFunc(registered, func(a, b Tab) int { return a.Order - b.Order })
}

// The tabs of this package, and where they stand in the tool bar: a tab
// registered with an Order under 100 comes before them.
const (
	consoleTab  = "console"
	networkTab  = "network"
	warningsTab = "warnings"
)

//nolint:gochecknoinits // the registration of this package's own tabs
func init() {
	Register(Tab{ID: consoleTab, Label: "Console", Order: 100, Build: newConsole,
		Icon: `<path d="M2.5 3.5h11v9h-11zM5 6.5 7 8 5 9.5M8.5 10h2.5"/>`})
	Register(Tab{ID: networkTab, Label: "Network", Order: 200, Build: newNetwork,
		Icon: `<path d="M5.5 13V3.5M3 6l2.5-2.5L8 6M10.5 3v9.5M8 10l2.5 2.5L13 10"/>`})
	Register(Tab{ID: warningsTab, Label: "Warnings", Order: 300, Build: newWarnings,
		Icon: `<path d="M8 2.5 14 13H2zM8 6.5v3M8 11.2v.2"/>`})
}

// State is one tab's panel: what the player set in it, kept while another
// tab's is shown.
type State struct {
	Open bool
	// Tab is the ID of the tab shown, "" for Network.
	Tab string
	// Preserve keeps the rows and the console over a navigation.
	Preserve bool
	// Filter and Kind choose the requests listed, Sort the column they
	// are in the order of (0 for the order they were asked in, below 0
	// for the last first), Selected the one whose details show, 0 for
	// none, and Response whether they show its body in place of its
	// headers.
	Filter   string
	Kind     netlog.Type
	Sort     int
	Selected int
	Response bool
	// Said and Muted choose the lines of the console: the levels it
	// leaves out. XHR lists the requests of scripts among them, Requests
	// every request and CSS what ui refused in the page's sheets.
	Said               string
	Muted              map[console.Level]bool
	XHR, Requests, CSS bool
	// History is what was typed in the console, the oldest first, and
	// Unfolded the lines whose calls show.
	History  []string
	Unfolded map[string]bool
	// Tool is what the tabs registered elsewhere keep, by their ID.
	Tool map[string]any
}

// Shown is the ID of the tab shown.
func (s *State) Shown() string {
	if s.Tab == "" {
		return networkTab
	}
	return s.Tab
}

// Host is what the panel asks of the screen it is part of. Point, Serve
// and Closed are needed; the rest may be nil.
type Host struct {
	// Point names an address while the pointer is over its row, "" after.
	Point func(address string)
	// Serve makes a picture drawable under name by the Document's img.
	Serve func(name string, picture []byte)
	// Closed is called once the panel's own button closed it.
	Closed func()
	// Evaluate runs what was typed in the console in the tab's page, and
	// gives its value as the console prints it.
	Evaluate func(source string) (string, error)
	// Unscripted is why the tab's page runs no script, "" when it does.
	Unscripted func() string
	// Pointer is where the pointer is in the window and whether its main
	// button is down: the handle on the panel's edge follows it.
	Pointer func() (at geom.Vec2, down bool)
	// Resized is told the height the player gave the panel, 0 for the
	// one it has by itself.
	Resized func(height int)
	// Source shows the source of address, at line when it is not 0.
	Source func(address string, line int)
	// Open shows address in a tab of its own.
	Open func(address string)
	// Storage is what the site of the tab's page keeps (storage.go).
	Storage func() Stored
}

// rowID is the id of the element of a row, before its number.
const rowID = "dt-row-"

const (
	// minHeight is the least the panel is high, and margin what it
	// leaves of what it is docked in.
	minHeight = 120
	margin    = 80
	// twice is how soon a second click on the handle is a double click.
	twice = 400 * time.Millisecond
)

// tab is a Tab in a panel: its button, the text of it and its tool.
type tab struct {
	Tab
	tool          Tool
	button, label *ui.Node
	frame         *ui.Node
}

// Panel is the developer tools of the tab last given to Show.
type Panel struct {
	doc  *ui.Document
	host Host

	root, handle, failed, errors, preserve, foot *ui.Node
	tabs                                         []*tab

	state    *State
	log      *netlog.Log
	console  *console.Lines
	warnings func() []error
	// heard is the console's version as the tool bar last counted it.
	heard uint64

	// height is the one the player gave the panel, 0 for its own.
	// dragging is whether the handle follows the pointer, from where it
	// was grabbed and the height the panel had then, moved whether the
	// pointer left that place since; held is whether the pointer's
	// button was down the frame before.
	height                int
	dragging, moved, held bool
	// tapped is when the handle was last pressed and let go where it
	// was: a second time soon after gives the panel its own height.
	tapped            time.Time
	grabbed, grabbedH float32
}

// New is a panel for doc, shown once Node is among its elements.
func New(doc *ui.Document, host Host) *Panel {
	p := &Panel{doc: doc, host: host, state: &State{}, log: netlog.New(0), console: console.New(0),
		warnings: func() []error { return nil }}
	for name, paths := range pictures {
		host.Serve(pictureName(name), picture(paths, pictureInk[name]))
	}
	bar := ui.Div(ui.Class("dt-tools"))
	p.root = ui.Section(ui.ID("devtools"), ui.Attr("aria-label", "Developer tools"))
	p.handle = ui.Div(ui.ID("dt-handle"))
	p.root.Append(p.handle, bar)
	for _, registered := range registered {
		t := &tab{Tab: registered, label: ui.Span(ui.Text(registered.Label))}
		host.Serve(pictureName("tab-"+t.ID), picture(t.Icon, ""))
		t.button = ui.Button(ui.ID("dt-tab-"+t.ID), ui.Class("dt-tab"), p.Icon("tab-"+t.ID), t.label,
			ui.OnClick(func() {
				p.state.Tab = t.ID
				p.Refresh()
			}))
		t.tool = t.Build(p)
		t.frame = ui.Div(ui.ID("dt-tool-"+t.ID), ui.Class("dt-tool"))
		if t.tool.Bar != nil {
			t.frame.Append(t.tool.Bar)
		}
		t.frame.Append(t.tool.Body)
		bar.Append(t.button)
		p.root.Append(t.frame)
		p.tabs = append(p.tabs, t)
	}
	p.preserve = ui.Input(ui.ID("dt-preserve"), ui.Type("checkbox"))
	ui.On(p.preserve, ui.EventChange, func() { p.state.Preserve = p.doc.Checked(p.preserve) })
	p.errors = ui.Span()
	p.failed = ui.Button(ui.ID("dt-console-errors"), ui.Attr("aria-label", "Errors"),
		ui.Span(ui.Class("dt-pill"), p.Icon("failed"), p.errors), ui.OnClick(func() {
			p.state.Tab = consoleTab
			p.Refresh()
		}))
	p.foot = ui.Div(ui.ID("dt-foot"))
	bar.Append(ui.Div(ui.Class("dt-space")),
		ui.Label(ui.Class("dt-check"), p.preserve, ui.Text("Preserve log")),
		p.failed,
		ui.Button(ui.ID("dt-close"), ui.Attr("aria-label", "Close the developer tools"), p.Icon("close"),
			ui.OnClick(func() {
				p.state.Open = false
				p.Refresh()
				p.host.Closed()
			})))
	p.root.Append(p.foot)
	return p
}

func (p *Panel) Node() *ui.Node { return p.root }

// Doc is the Document the panel is part of.
func (p *Panel) Doc() *ui.Document { return p.doc }

// State is the state of the tab the panel shows.
func (p *Panel) State() *State { return p.state }

// Host is what the panel was given to ask its screen.
func (p *Panel) Host() Host { return p.host }

// Icon is one of the panel's pictures, by its name in pictures, or a
// tab's as "tab-<ID>".
func (p *Panel) Icon(name string) *ui.Node {
	return ui.Img(ui.Class("dt-icon"), ui.Attr("src", pictureName(name)), ui.Attr("alt", ""))
}

// Button is a button of the panel: it does what it does, then the panel
// shows itself again.
func (p *Panel) Button(id, text string, do func()) *ui.Node {
	return ui.Button(ui.ID(id), ui.Text(text), ui.OnClick(func() {
		do()
		p.Refresh()
	}))
}

// SetLabel changes what the button of the tab named id reads.
func (p *Panel) SetLabel(id, label string) {
	for _, t := range p.tabs {
		if t.ID == id && t.label.Text() != label {
			t.label.SetText(label)
		}
	}
}

// SetFoot changes the line under the tool, which "" hides.
func (p *Panel) SetFoot(text string) {
	if p.foot.Text() != text {
		p.foot.SetText(text)
	}
	toggle(p.foot, "away", text == "")
}

// SetDark draws the panel dark, or light as a light page is.
func (p *Panel) SetDark(dark bool) { toggle(p.root, "light", !dark) }

// Height is the height the player gave the panel, 0 for its own.
func (p *Panel) Height() int { return p.height }

// SetHeight gives the panel a height, kept within what it is docked in;
// 0 gives it back its own.
func (p *Panel) SetHeight(height int) {
	if height <= 0 {
		p.height = 0
		p.root.SetStyle("")
		return
	}
	p.height = p.fit(height)
	p.root.SetStyle("height: " + strconv.Itoa(p.height) + "px")
}

// fit is height kept between the least the panel is high and what it is
// docked in less a margin.
func (p *Panel) fit(height int) int {
	if parent := p.root.Parent(); parent != nil {
		if around, ok := p.doc.BoxOf(parent); ok && around.H > 0 {
			height = min(height, int(around.H)-margin)
		}
	}
	return max(height, minHeight)
}

// resize is SetHeight by the player: the screen is told.
func (p *Panel) resize(height int) {
	before := p.height
	p.SetHeight(height)
	if p.height != before && p.host.Resized != nil {
		p.host.Resized(p.height)
	}
}

// follow has the panel's top edge follow the pointer from a press on
// its handle to the release, gives the panel its own height back at a
// double click on it, and keeps a panel given a height within a window
// made smaller. ui tells an element of no press and of no pointer that
// left it, so the pointer is read from the screen each frame.
func (p *Panel) follow() {
	if p.host.Pointer == nil || !p.state.Open {
		p.dragging, p.held = false, false
		return
	}
	at, down := p.host.Pointer()
	switch {
	case !down && p.dragging:
		p.dragging = false
		p.root.RemoveClass("sizing")
		switch {
		case p.moved:
			p.tapped = time.Time{}
			if p.host.Resized != nil {
				p.host.Resized(p.height)
			}
		case time.Since(p.tapped) < twice:
			p.tapped = time.Time{}
			p.resize(0)
		default:
			p.tapped = time.Now()
		}
	case down && p.dragging:
		if at.Y != p.grabbed || p.moved {
			p.moved = true
			p.SetHeight(max(minHeight, int(math.Round(float64(p.grabbedH+p.grabbed-at.Y)))))
		}
	case down && !p.held && p.doc.ElementAt(at) == p.handle:
		p.dragging, p.moved, p.grabbed, p.grabbedH = true, false, at.Y, p.root.Box().H
		p.root.AddClass("sizing")
	case p.height > 0 && p.fit(p.height) != p.height:
		p.SetHeight(p.height)
	}
	p.held = down
}

// Show shows a tab's panel: its state, its requests, what its page's
// scripts said and the warnings of its page, asked for each time they
// are listed.
func (p *Panel) Show(state *State, log *netlog.Log, said *console.Lines, warnings func() []error) {
	p.state, p.log, p.console, p.warnings = state, log, said, warnings
	p.doc.SetChecked(p.preserve, state.Preserve)
	p.Refresh()
}

// Update follows the pointer on the handle, counts the console's errors
// again once something was said, and has each tool follow what it
// lists.
func (p *Panel) Update() {
	p.follow()
	if p.console.Version() != p.heard {
		p.count()
	}
	for _, t := range p.tabs {
		if t.tool.Update != nil {
			t.tool.Update(p.state.Open && p.state.Shown() == t.ID)
		}
	}
}

// Refresh shows the panel again from its state: the chosen tab and its
// tool.
func (p *Panel) Refresh() {
	toggle(p.root, "shown", p.state.Open)
	shown := p.state.Shown()
	if !slices.ContainsFunc(p.tabs, func(t *tab) bool { return t.ID == shown }) {
		shown = networkTab
	}
	p.count()
	for _, t := range p.tabs {
		toggle(t.button, "chosen", t.ID == shown)
		toggle(t.frame, "shown", t.ID == shown)
	}
	if !p.state.Open {
		return
	}
	for _, t := range p.tabs {
		if t.ID == shown && t.tool.Show != nil {
			t.tool.Show()
		}
	}
}

// count says at the right of the tool bar how many errors the console
// holds.
func (p *Panel) count() {
	p.heard = p.console.Version()
	errors := p.console.Count(console.Error)
	if p.errors.Text() != strconv.Itoa(errors) {
		p.errors.SetText(strconv.Itoa(errors))
	}
	toggle(p.failed, "away", errors == 0)
}

// RowAt is the request whose row n is part of.
func (p *Panel) RowAt(n *ui.Node) (netlog.Row, bool) {
	for ; n != nil; n = n.Parent() {
		if number, ok := strings.CutPrefix(n.ID(), rowID); ok {
			id, err := strconv.Atoi(number)
			if err != nil {
				return netlog.Row{}, false
			}
			return p.log.Row(id)
		}
	}
	return netlog.Row{}, false
}

func counted(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func fill(n *ui.Node, children ...*ui.Node) {
	for len(n.Children()) > 0 {
		n.Remove(n.Children()[0])
	}
	n.Append(children...)
}

func toggle(n *ui.Node, class string, on bool) {
	if on {
		n.AddClass(class)
		return
	}
	n.RemoveClass(class)
}
