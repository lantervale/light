// Package inspector is the Inspector of light's developer tools: the
// shown page as a tree of markup, the rules and the computed style of
// the element selected in it, and that element outlined over the page.
// It asks its screen for the page (Host) and reads it through ui's
// inspection calls; it changes nothing of it.
package inspector

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"
)

// Sheet is the Inspector's look, for the Document it is added to.
//
//go:embed inspector.css
var Sheet string

// Page is the page the Inspector shows.
type Page struct {
	// Doc is the Document that styles and lays the page out, and Root
	// the element its tree starts at: Doc's html for a page of the web,
	// an element of the screen's own Document for a page of light's.
	Doc  *ui.Document
	Root *ui.Node
	// Head holds what the page keeps out of sight, shown as its head;
	// nil for none.
	Head *ui.Node
	// Tag names an element ui has no element for; nil when n.Tag() names
	// them all.
	Tag func(n *ui.Node) string
	// Doctype is whether the tree begins with a doctype.
	Doctype bool
	// Area is where the page shows in the Inspector's Document, in the
	// points BoxOf answers in: the origin of Doc's own points when Doc
	// is another Document, and what the highlight is cut to.
	Area func() (geom.Rect, bool)
}

// Host is what the Inspector asks of the screen it is part of. Page is
// needed; the rest may be nil.
type Host struct {
	// Page is the page shown now; one with no Root is no page.
	Page func() Page
	// Input is the window's input: the pointer over the tree and the
	// page, and the keys that walk the tree.
	Input func() *input.State
	// Selected is told the node selected, nil for none.
	Selected func(n *ui.Node)
	// Source shows the sheet at address, at line.
	Source func(address string, line int)
}

// State is what the Inspector keeps of one page: the node selected,
// what was opened and closed, and what its fields hold.
type State struct {
	root     *ui.Node
	selected *ui.Node
	// toggled are the elements opened or closed by hand, and listed
	// which of their children those that show others than the first few
	// show.
	toggled map[*ui.Node]bool
	listed  map[*ui.Node]window
	// Search, Rules and Computed are what the three fields hold, and
	// All whether the computed pane lists the initial values too.
	Search, Rules, Computed string
	All                     bool
}

const (
	// every is how many frames pass between two looks at the page for
	// what changed in it.
	every = 15
	// few is how many children an element shows before it is asked for
	// more.
	few = 200
)

// Inspector is the Inspector of one screen.
type Inspector struct {
	doc   *ui.Document
	host  Host
	state *State
	page  Page

	body, pick, search, count, tree, none *ui.Node
	failure                               *ui.Node
	rules, rulesFilter                    *ui.Node
	box, computed, computedFilter, all    *ui.Node
	overlay, clip, outline, label         *ui.Node
	edges                                 [4]*ui.Node
	items                                 map[*ui.Node]*item
	rows                                  map[*ui.Node]*item
	top                                   []*item
	shown, picking, keyboard, held        bool
	// guarding keeps the page from the press that picked until its
	// release.
	guarding bool
	// orphan is what the selected node was under when it left the tree.
	orphan  *item
	keys    map[input.Key]bool
	hovered *ui.Node
	frames  int
	styled  string
	matches []*ui.Node
	matched string
	// failed is what went wrong in the Inspector, empty while nothing
	// did.
	failed string
}

// New is an Inspector for doc: Body goes among the tools, Overlay over
// the page.
func New(doc *ui.Document, host Host) *Inspector {
	in := &Inspector{doc: doc, host: host, state: &State{}, items: map[*ui.Node]*item{}, rows: map[*ui.Node]*item{},
		keys: make(map[input.Key]bool)}
	in.pick = ui.Button(ui.ID("dt-insp-pick"), ui.Attr("aria-label", "Pick an element from the page"),
		ui.Span(ui.Class("dt-insp-pick-box")), ui.Span(ui.Class("dt-insp-pick-arrow")),
		ui.OnClick(func() { in.SetPicking(!in.picking) }))
	in.failure = ui.Div(ui.Class("dt-insp-none"))
	in.search = field("dt-insp-search", "Search HTML")
	ui.On(in.search, ui.EventInput, in.guarded(func() {
		in.state.Search = in.doc.Value(in.search)
		in.find(0)
	}))
	ui.On(in.search, ui.EventSubmit, in.guarded(func() { in.find(1) }))
	in.count = ui.Span(ui.ID("dt-insp-count"))
	in.tree = ui.Div(ui.ID("dt-insp-tree"))
	in.none = ui.Div(ui.Class("dt-insp-none"), ui.Text("This tab shows no page to inspect."))
	in.rulesFilter = field("dt-insp-rules-filter", "Filter Styles")
	ui.On(in.rulesFilter, ui.EventInput, in.guarded(func() {
		in.state.Rules = in.doc.Value(in.rulesFilter)
		in.style()
	}))
	in.rules = ui.Div(ui.ID("dt-insp-rules"))
	in.computedFilter = field("dt-insp-computed-filter", "Filter Styles")
	ui.On(in.computedFilter, ui.EventInput, in.guarded(func() {
		in.state.Computed = in.doc.Value(in.computedFilter)
		in.style()
	}))
	in.all = ui.Input(ui.ID("dt-insp-all"), ui.Type("checkbox"))
	ui.On(in.all, ui.EventChange, in.guarded(func() {
		in.state.All = in.doc.Checked(in.all)
		in.style()
	}))
	in.box = ui.Div(ui.ID("dt-insp-box"))
	in.computed = ui.Div(ui.ID("dt-insp-computed"))
	in.body = ui.Div(ui.ID("dt-insp"),
		ui.Div(ui.ID("dt-insp-markup"), ui.Class("dt-insp-pane"),
			ui.Div(ui.Class("dt-insp-bar"), in.pick, in.search, in.count), in.tree),
		ui.Div(ui.ID("dt-insp-styles"), ui.Class("dt-insp-pane"),
			ui.Div(ui.Class("dt-insp-bar"), in.rulesFilter), in.rules),
		ui.Div(ui.ID("dt-insp-side"), ui.Class("dt-insp-pane"),
			ui.Div(ui.Class("dt-insp-bar"), in.computedFilter,
				ui.Label(ui.Class("dt-insp-check"), in.all, ui.Text("Browser styles"))),
			ui.Div(ui.ID("dt-insp-layout"), in.box, in.computed)))
	in.overlays()
	return in
}

func field(id, placeholder string) *ui.Node {
	return ui.Input(ui.ID(id), ui.Class("dt-insp-field"), ui.Placeholder(placeholder),
		ui.Attr("aria-label", placeholder), ui.Attr("autocomplete", "off"))
}

// Body is the Inspector's three panes, for the tools.
func (in *Inspector) Body() *ui.Node { return in.body }

// Overlay is what outlines an element of the page: it goes over the
// page, in an element positioned as the page's area is.
func (in *Inspector) Overlay() *ui.Node { return in.overlay }

// Show shows the Inspector with the state of the tab shown now.
func (in *Inspector) Show(state *State) {
	defer in.survive()
	if state == nil {
		state = &State{}
	}
	if state != in.state {
		in.state = state
		in.page = Page{}
	}
	for field, text := range map[*ui.Node]string{in.search: state.Search, in.rulesFilter: state.Rules,
		in.computedFilter: state.Computed} {
		if in.doc.Value(field) != text {
			in.doc.SetValue(field, text)
		}
	}
	in.doc.SetChecked(in.all, state.All)
	in.follow()
}

// Update runs every frame, shown telling whether the Inspector is what
// the tools show: it follows the page, the pointer and the keys.
func (in *Inspector) Update(shown bool) {
	defer in.survive()
	if in.failed != "" {
		if in.frames++; in.frames < every {
			return
		}
		in.failed, in.frames = "", 0
	}
	if shown != in.shown {
		in.shown = shown
		if !shown {
			in.SetPicking(false)
			in.hovered, in.keyboard = nil, false
		}
	}
	if !shown {
		in.place()
		return
	}
	if !in.follow() {
		in.frames++
	}
	if in.frames >= every {
		in.frames = 0
		in.refresh()
	}
	in.point()
	in.place()
}

// follow rebuilds the tree when the page shown is another, and reports
// whether it did.
func (in *Inspector) follow() bool {
	page := Page{}
	if in.host.Page != nil && in.failed == "" {
		page = in.host.Page()
	}
	if page.Doc == nil || page.Root == nil {
		page = Page{}
	}
	if page.Root == in.page.Root && page.Doc == in.page.Doc && page.Head == in.page.Head {
		in.page = page
		return false
	}
	in.page = page
	if in.state.root != page.Root {
		keep := *in.state
		*in.state = State{root: page.Root, Search: keep.Search, Rules: keep.Rules, Computed: keep.Computed,
			All: keep.All}
	}
	in.hovered = nil
	in.rebuild()
	in.find(0)
	in.restyle()
	in.frames = every - 2
	return true
}

// Selected is the node selected, nil for none.
func (in *Inspector) Selected() *ui.Node { return in.state.selected }

// Picking reports whether the next click on the page selects what is
// under it.
func (in *Inspector) Picking() bool { return in.picking || in.guarding }

// SetPicking starts or ends picking an element from the page.
func (in *Inspector) SetPicking(on bool) {
	defer in.survive()
	if on && (!in.shown || in.page.Root == nil || in.failed != "") {
		on = false
	}
	if in.picking == on {
		return
	}
	in.picking = on
	if !on {
		in.hovered = nil
	}
	toggle(in.pick, "chosen", on)
	toggle(in.overlay, "picking", on || in.guarding)
}

// Select selects n, a node of the page, in the tree: its ancestors are
// opened, its row shown, and the page scrolled to it when the page is a
// Document of its own. A node that is not of the page selects
// nothing.
func (in *Inspector) Select(n *ui.Node) {
	defer in.survive()
	if in.failed != "" {
		return
	}
	in.follow()
	var it *item
	if n != nil {
		it = in.reveal(n)
	}
	in.selectItem(it)
}

func (in *Inspector) selectItem(it *item) {
	var n *ui.Node
	if it != nil {
		n = it.n
	}
	changed := n != in.state.selected
	if was := in.items[in.state.selected]; was != nil {
		was.row.RemoveClass("selected")
	}
	in.state.selected = n
	if it != nil {
		it.row.AddClass("selected")
		in.doc.ScrollIntoView(it.row)
		if changed && !n.IsText() && n != in.page.Head && in.page.Doc != nil && in.page.Doc != in.doc {
			in.page.Doc.ScrollIntoView(n)
		}
	}
	in.restyle()
	if changed && in.host.Selected != nil {
		in.host.Selected(n)
	}
}

// RowOf is the row of n in the tree, nil while the tree does not show
// it.
func (in *Inspector) RowOf(n *ui.Node) *ui.Node {
	if it := in.items[n]; it != nil {
		return it.row
	}
	return nil
}

// point follows the pointer and the keys: a press on a row selects it,
// one on its triangle opens or closes it, the pointer over a row
// outlines its element in the page, and while picking the element of
// the page under the pointer is outlined, and selected by a press.
func (in *Inspector) point() {
	if in.host.Input == nil {
		return
	}
	state := in.host.Input()
	if state == nil {
		return
	}
	at := state.Mouse()
	down := state.Down(input.MouseLeft)
	pressed := down && !in.held
	in.held = down
	if in.guarding && !down {
		in.guarding = false
		toggle(in.overlay, "picking", in.picking)
	}
	if in.picking {
		in.choose(at, pressed, in.key(state, input.KeyEscape))
		return
	}
	over := in.doc.ElementAt(at)
	it, part := in.rowAt(over)
	if it != nil && it.n == nil {
		it = nil
	}
	in.hovered = nil
	if it != nil && !it.n.IsText() {
		in.hovered = it.n
	}
	if pressed {
		in.keyboard = it != nil
		switch {
		case it == nil:
		case part != "":
			in.more(it, part == "dt-insp-more")
		case it.expandable && onTwist(it, at):
			in.toggle(it)
		default:
			in.selectItem(it)
		}
	}
	in.walk(state)
}

// choose is a frame of picking: the element of the page under the
// pointer is outlined, a press selects it, and Escape or a press
// elsewhere ends it.
func (in *Inspector) choose(at geom.Vec2, pressed, escape bool) {
	in.hovered = in.under(at)
	switch {
	case escape:
		in.SetPicking(false)
	case pressed && in.hovered != nil:
		n := in.hovered
		in.guarding = true
		in.SetPicking(false)
		in.Select(n)
		in.keyboard = true
	case pressed && in.doc.ElementAt(at) != in.pick:
		in.SetPicking(false)
	}
}

// onTwist reports whether at is over the triangle of an item's row.
func onTwist(it *item, at geom.Vec2) bool {
	twist := it.row.Children()[0].Box()
	return at.X >= twist.X-4 && at.X <= twist.X+twist.W+4
}

// key reports whether k went down since the last frame.
func (in *Inspector) key(state *input.State, k input.Key) bool {
	down := state.Down(k)
	pressed := down && !in.keys[k]
	in.keys[k] = down
	return pressed
}

// walk moves the selection by the arrows once a row was pressed and
// until something else is: up and down by a row, left to close an
// element or go to its parent, right to open it or go to its first
// child.
func (in *Inspector) walk(state *input.State) {
	up, down := in.key(state, input.KeyUp), in.key(state, input.KeyDown)
	left, right := in.key(state, input.KeyLeft), in.key(state, input.KeyRight)
	it := in.items[in.state.selected]
	if !in.keyboard || it == nil || in.typing() {
		return
	}
	switch {
	case up, down:
		rows := in.visible()
		for i, row := range rows {
			if row != it {
				continue
			}
			if to := i + step(down); to >= 0 && to < len(rows) {
				in.selectItem(rows[to])
			}
			break
		}
	case left && it.open:
		in.toggle(it)
	case left && it.parent != nil:
		in.selectItem(it.parent)
	case right && it.expandable && !it.open:
		in.toggle(it)
	case right && it.open && len(it.kids) > 0:
		in.selectItem(it.kids[0])
	}
}

func step(forward bool) int {
	if forward {
		return 1
	}
	return -1
}

// typing reports whether one of the Inspector's fields has the keys.
func (in *Inspector) typing() bool {
	return in.doc.Focused(in.search) || in.doc.Focused(in.rulesFilter) || in.doc.Focused(in.computedFilter)
}

// find lists the nodes of the page the search field names, and selects
// the one step matches after the selected one: 0 selects nothing.
func (in *Inspector) find(step int) {
	query := strings.ToLower(strings.TrimSpace(in.state.Search))
	if query != in.matched || step == 0 {
		in.matched, in.matches = query, nil
		if query != "" && in.page.Root != nil {
			in.each(in.page.Root, func(n *ui.Node) {
				if in.named(n, query) {
					in.matches = append(in.matches, n)
				}
			})
		}
	}
	at := -1
	if step != 0 && len(in.matches) > 0 {
		at = 0
		for i, n := range in.matches {
			if n == in.state.selected {
				at = (i + step + len(in.matches)) % len(in.matches)
			}
		}
		in.Select(in.matches[at])
	}
	text := ""
	switch {
	case query == "":
	case len(in.matches) == 0:
		text = "No match"
	case at >= 0:
		text = plural(at+1, len(in.matches))
	default:
		text = plural(0, len(in.matches))
	}
	if in.count.Text() != text {
		in.count.SetText(text)
	}
}

func toggle(n *ui.Node, class string, on bool) {
	if on {
		n.AddClass(class)
		return
	}
	n.RemoveClass(class)
}

func fill(n *ui.Node, children ...*ui.Node) {
	for kids := n.Children(); len(kids) > 0; kids = n.Children() {
		n.Remove(kids[len(kids)-1])
	}
	n.Append(children...)
}

// Failed is what went wrong in the Inspector, empty while nothing did:
// a failure is kept in its tab, which says it and looks at the page
// again a few frames later.
func (in *Inspector) Failed() string { return in.failed }

// guarded is do as a callback of an element, a failure of it kept in
// the Inspector.
func (in *Inspector) guarded(do func()) func() {
	return func() {
		defer in.survive()
		if in.failed == "" {
			do()
		}
	}
}

// survive ends what the Inspector was doing when it panics: the tools
// are no reason for the browser to stop. It lets go of the page, which
// may be what failed, and says what happened where the tree was.
func (in *Inspector) survive() {
	r := recover()
	if r == nil {
		return
	}
	defer func() { _ = recover() }()
	in.failed, in.frames = fmt.Sprint(r), 0
	in.page, in.top, in.hovered, in.orphan, in.matches = Page{}, nil, nil, nil, nil
	in.picking, in.guarding, in.keyboard, in.styled = false, false, false, ""
	clear(in.items)
	clear(in.rows)
	*in.state = State{Search: in.state.Search, Rules: in.state.Rules, Computed: in.state.Computed, All: in.state.All}
	for _, n := range []*ui.Node{in.overlay, in.pick} {
		for _, class := range []string{"shown", "picking", "chosen"} {
			n.RemoveClass(class)
		}
	}
	in.failure.SetText("The Inspector failed and looks at the page again: " + in.failed)
	fill(in.tree, in.failure)
	fill(in.rules)
	fill(in.computed)
}
