// Package devtools is light's developer tools: a panel docked under a
// tab's page, drawn with ui elements, whose Network tab lists the tab's
// requests as its netlog.Log has them and whose Warnings tab lists what
// ui refused in the page's sheets.
package devtools

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/netlog"
)

// Sheet is the panel's look, for the Document it is added to.
//
//go:embed devtools.css
var Sheet string

// State is one tab's panel: what the player set in it, kept while another
// tab's is shown.
type State struct {
	Open bool
	// Warnings is whether the Warnings tab is shown, in place of Network.
	Warnings bool
	// Preserve keeps the rows over a navigation.
	Preserve bool
	// Filter and Kind choose the rows listed.
	Filter string
	Kind   netlog.Type
	// Selected is the row whose details show, 0 for none, and Response
	// whether they show its body in place of its headers.
	Selected int
	Response bool
}

// Host is what the panel asks of the screen it is part of.
type Host struct {
	// Point names an address while the pointer is over its row, "" after.
	Point func(address string)
	// Serve makes a picture drawable under name by the Document's img.
	Serve func(name string, picture []byte)
	// Closed is called once the panel's own button closed it.
	Closed func()
}

// rowID is the id of the element of a row, before its number.
const rowID = "dt-row-"

// Panel is the developer tools of the tab last given to Show.
type Panel struct {
	doc  *ui.Document
	host Host

	root, network, warned, filter, preserve, body, details, foot *ui.Node
	chips                                                        map[netlog.Type]*ui.Node

	state    *State
	log      *netlog.Log
	warnings func() []error
	// seen is the log's version as last listed.
	seen uint64
}

// New is a panel for doc, shown once Node is among its elements.
func New(doc *ui.Document, host Host) *Panel {
	p := &Panel{doc: doc, host: host, state: &State{}, log: netlog.New(0), chips: make(map[netlog.Type]*ui.Node),
		warnings: func() []error { return nil }}
	p.network = p.button("dt-tab-network", "Network", func() { p.state.Warnings = false })
	p.warned = p.button("dt-tab-warnings", "Warnings", func() { p.state.Warnings = true })
	p.filter = ui.Input(ui.ID("dt-filter"), ui.Placeholder("Filter"), ui.Attr("aria-label", "Filter requests"))
	ui.On(p.filter, ui.EventInput, func() {
		p.state.Filter = p.doc.Value(p.filter)
		p.list()
	})
	p.preserve = ui.Input(ui.ID("dt-preserve"), ui.Type("checkbox"))
	ui.On(p.preserve, ui.EventChange, func() { p.state.Preserve = p.doc.Checked(p.preserve) })
	bar := ui.Div(ui.Class("dt-bar"), p.network, p.warned, ui.Div(ui.Class("dt-rule")), p.filter)
	for _, chip := range []struct {
		name string
		kind netlog.Type
	}{{"All", netlog.Any}, {"Doc", netlog.Document}, {"CSS", netlog.Stylesheet}, {"Img", netlog.Image}, {"Other",
		netlog.Other}} {
		id := "dt-chip-" + string(chip.kind)
		if chip.kind == netlog.Any {
			id = "dt-chip-all"
		}
		p.chips[chip.kind] = p.button(id, chip.name, func() { p.state.Kind = chip.kind })
		bar.Append(p.chips[chip.kind])
	}
	bar.Append(ui.Div(ui.Class("dt-rule")),
		ui.Label(ui.Class("dt-check"), p.preserve, ui.Text("Preserve log")),
		p.button("dt-clear", "Clear", func() {
			p.log.Clear()
			p.state.Selected = 0
		}),
		ui.Div(ui.Class("dt-space")),
		p.button("dt-close", "Close", func() {
			p.state.Open = false
			p.host.Closed()
		}))
	p.body = ui.Div(ui.Class("dt-body"))
	p.details = ui.Div(ui.ID("dt-details"))
	p.foot = ui.Div(ui.ID("dt-foot"))
	p.root = ui.Section(ui.ID("devtools"), ui.Attr("aria-label", "Developer tools"), bar, p.body, p.foot)
	return p
}

// button is a button of the panel: it does what it does, then the panel
// shows itself again.
func (p *Panel) button(id, text string, do func()) *ui.Node {
	return ui.Button(ui.ID(id), ui.Text(text), ui.OnClick(func() {
		do()
		p.show()
	}))
}

func (p *Panel) Node() *ui.Node { return p.root }

// Show shows a tab's panel: its state, its requests and the warnings of
// its page, asked for each time they are listed.
func (p *Panel) Show(state *State, log *netlog.Log, warnings func() []error) {
	p.state, p.log, p.warnings = state, log, warnings
	p.doc.SetValue(p.filter, state.Filter)
	p.doc.SetChecked(p.preserve, state.Preserve)
	p.show()
}

// Update lists the requests again once one changed.
func (p *Panel) Update() {
	if p.state.Open && !p.state.Warnings && p.log.Version() != p.seen {
		p.list()
	}
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

func (p *Panel) show() {
	toggle(p.root, "shown", p.state.Open)
	toggle(p.network, "chosen", !p.state.Warnings)
	toggle(p.warned, "chosen", p.state.Warnings)
	for kind, chip := range p.chips {
		toggle(chip, "chosen", kind == p.state.Kind)
	}
	warnings := p.warnings()
	p.warned.SetText(fmt.Sprintf("Warnings (%d)", len(warnings)))
	if !p.state.Open {
		return
	}
	if p.state.Warnings {
		p.warn(warnings)
		return
	}
	p.list()
}

// warn fills the body with the page's warnings.
func (p *Panel) warn(warnings []error) {
	list := ui.Div(ui.ID("dt-warnings"))
	for _, w := range warnings {
		list.Append(ui.Div(ui.Class("dt-warning"), ui.Text(w.Error())))
	}
	if len(warnings) == 0 {
		list.Append(ui.Div(ui.Class("dt-none"), ui.Text("ui read this tab's page without a warning.")))
	}
	fill(p.body, list)
	p.foot.SetText("What ui refused in the page's sheets and pictures")
}

// list fills the body with the requests the filters keep, the details of
// the selected one beside them, and sums them up in the footer.
func (p *Panel) list() {
	p.seen = p.log.Version()
	all := p.log.Rows()
	kept := netlog.Filter(all, p.state.Filter, p.state.Kind)
	head := line("Name", "Status", "Method", "Type", "Initiator", "Size", "Time")
	head.AddClass("dt-head")
	rows := ui.Div(ui.ID("dt-rows"), head)
	for _, r := range kept {
		rows.Append(p.row(r))
	}
	fill(p.body, rows, p.details)
	p.detail(all)
	p.foot.SetText(summary(len(all), kept, p.log.Dropped()))
}

// line is one line of the table: a cell for each text.
func line(texts ...string) *ui.Node {
	row := ui.Div(ui.Class("dt-row"))
	for _, text := range texts {
		row.Append(ui.Span(ui.Text(text)))
	}
	return row
}

func (p *Panel) row(r netlog.Row) *ui.Node {
	row := line(r.Name(), status(r), r.Method, string(r.Type), r.Initiator, size(r.Size), took(r))
	row.SetID(rowID + strconv.Itoa(r.ID))
	ui.TabIndex(0).Apply(row)
	ui.OnClick(func() {
		p.state.Selected = r.ID
		p.list()
	}).Apply(row)
	ui.OnPointerEnter(func() { p.host.Point(r.Address) }).Apply(row)
	ui.OnPointerLeave(func() { p.host.Point("") }).Apply(row)
	toggle(row, "failed", r.Failed())
	toggle(row, "redirect", !r.Failed() && r.Status >= 300)
	toggle(row, "selected", r.ID == p.state.Selected)
	return row
}

// detail fills the details with the selected row's headers or response,
// and hides them when no row of all is selected.
func (p *Panel) detail(all []netlog.Row) {
	at := slices.IndexFunc(all, func(r netlog.Row) bool { return r.ID == p.state.Selected })
	toggle(p.details, "shown", at >= 0)
	if at < 0 {
		p.state.Selected = 0
		return
	}
	r := all[at]
	headers := p.button("dt-headers", "Headers", func() { p.state.Response = false })
	response := p.button("dt-response", "Response", func() { p.state.Response = true })
	toggle(headers, "chosen", !p.state.Response)
	toggle(response, "chosen", p.state.Response)
	content := ui.Div(ui.ID("dt-detail"))
	if p.state.Response {
		content.Append(p.response(r)...)
	} else {
		content.Append(headersOf(r, netlog.Chain(all, r.ID))...)
	}
	fill(p.details,
		ui.Div(ui.Class("dt-bar"), headers, response, ui.Div(ui.Class("dt-space")),
			p.button("dt-details-close", "×", func() { p.state.Selected = 0 })),
		content)
}

func headersOf(r netlog.Row, chain []netlog.Row) []*ui.Node {
	out := []*ui.Node{ui.H4(ui.Text("General")), pair("Address", r.Address), pair("Method", r.Method),
		pair("Status", strings.TrimSpace(status(r)+" "+r.Reason))}
	if r.Err != "" {
		out = append(out, pair("Error", r.Err))
	}
	if r.Remote != "" {
		out = append(out, pair("Remote address", r.Remote))
	}
	for _, hop := range chain {
		if hop.ID < r.ID {
			out = append(out, pair("Redirected from", hop.Address))
		}
	}
	out = append(out, ui.H4(ui.Text("Response headers")))
	out = append(out, pairs(r.Response)...)
	out = append(out, ui.H4(ui.Text("Request headers")))
	return append(out, pairs(r.Request)...)
}

func pairs(header map[string][]string) []*ui.Node {
	var out []*ui.Node
	for _, name := range slices.Sorted(maps.Keys(header)) {
		out = append(out, pair(name, strings.Join(header[name], ", ")))
	}
	return out
}

func pair(name, value string) *ui.Node {
	return ui.Div(ui.Class("dt-pair"), ui.Span(ui.Class("dt-name"), ui.Text(name)),
		ui.Span(ui.Class("dt-value"), ui.Text(value)))
}

// response is what a row keeps of its body: text as it was sent, a
// picture drawn, and for the rest what it is.
func (p *Panel) response(r netlog.Row) []*ui.Node {
	preview := ui.Pre(ui.ID("dt-preview"))
	out := []*ui.Node{preview}
	switch kind, media := content.Of(r.Response.Get("Content-Type"), r.Preview, r.Address); {
	case len(r.Preview) == 0:
		out = []*ui.Node{ui.Div(ui.Class("dt-none"), ui.Text("This request has no body to show."))}
	case kind == content.Picture && r.Done && !r.Truncated:
		name := "netlog:" + strconv.Itoa(r.ID)
		p.host.Serve(name, content.Drawable(r.Preview))
		preview.Append(ui.Img(ui.Attr("src", name), ui.Attr("alt", r.Name())))
	case kind == content.Page || kind == content.Text:
		preview.SetText(content.Spaced(string(r.Preview)))
	default:
		out = []*ui.Node{ui.Div(ui.Class("dt-none"), ui.Text("No preview of "+media+"."))}
	}
	if r.Truncated {
		out = append(out, ui.Div(ui.Class("dt-none"),
			ui.Text(fmt.Sprintf("Cut at %s: a row keeps no more of a body.", size(netlog.PreviewCap)))))
	}
	return out
}

func status(r netlog.Row) string {
	switch {
	case r.Err == "cancelled":
		return "(cancelled)"
	case r.Err != "":
		return "(failed)"
	case r.Status == 0:
		return "(pending)"
	}
	return strconv.Itoa(r.Status)
}

func size(bytes int64) string {
	switch {
	case bytes < 1000:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1000*1000:
		return fmt.Sprintf("%.1f kB", float64(bytes)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1000*1000))
}

func took(r netlog.Row) string {
	if ms := r.Took.Milliseconds(); ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.2f s", r.Took.Seconds())
}

// summary is the footer: how many requests are listed, of how many, their
// bytes and the time to the last answer, and how many went past the cap.
func summary(all int, kept []netlog.Row, dropped int) string {
	if all == 0 && dropped == 0 {
		return "No requests"
	}
	total := netlog.Totals(kept)
	count := fmt.Sprintf("%d requests", all)
	if len(kept) != all {
		count = fmt.Sprintf("%d of %d requests", len(kept), all)
	}
	text := fmt.Sprintf("%s · %s · %s", count, size(total.Bytes), took(netlog.Row{Took: total.Took}))
	if dropped > 0 {
		text += fmt.Sprintf(" · %d older requests dropped", dropped)
	}
	return text
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
