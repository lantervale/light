package devtools

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/netlog"
)

// columns are the table's, in their order: a row's cell of each is
// dt-<name>.
//
//nolint:gochecknoglobals // a table
var columns = []string{"Status", "Method", "Domain", "File", "Initiator", "Type", "Transferred", "Size", "Duration"}

// kinds are the chips that choose the requests listed by what they are.
// There is none for WebSockets: light opens none.
//
//nolint:gochecknoglobals // a table
var kinds = []struct {
	name string
	kind netlog.Type
}{
	{"All", netlog.Any}, {"HTML", netlog.Document}, {"CSS", netlog.Stylesheet}, {"JS", netlog.Script},
	{"XHR", netlog.XHR}, {"Fonts", netlog.Font}, {"Images", netlog.Image}, {"Media", netlog.Media},
	{"Other", netlog.Other},
}

// network is the Network tab: the tab's requests as its netlog.Log has
// them, a row each, and the details of the selected one beside them.
type network struct {
	p                     *Panel
	filter, rows, details *ui.Node
	chips                 map[netlog.Type]*ui.Node
	// seen is the log's version as last listed.
	seen uint64
}

func newNetwork(p *Panel) Tool {
	n := &network{p: p, chips: make(map[netlog.Type]*ui.Node), rows: ui.Div(ui.ID("dt-rows")),
		details: ui.Div(ui.ID("dt-details"))}
	n.filter = ui.Input(ui.ID("dt-filter"), ui.Placeholder("Filter URLs"), ui.Attr("aria-label", "Filter URLs"))
	ui.On(n.filter, ui.EventInput, func() {
		p.state.Filter = p.doc.Value(n.filter)
		n.list()
	})
	wipe := ui.Button(ui.ID("dt-clear"), ui.Class("dt-glyph"), ui.Attr("aria-label", "Clear"), p.Icon("clear"),
		ui.OnClick(func() {
			p.log.Clear()
			p.state.Selected = 0
			n.list()
		}))
	bar := ui.Div(ui.Class("dt-bar"), wipe, n.export(), ui.Label(ui.Class("dt-field"), p.Icon("filter"), n.filter),
		ui.Div(ui.Class("dt-rule")))
	for _, chip := range kinds {
		id := "dt-chip-" + string(chip.kind)
		if chip.kind == netlog.Any {
			id = "dt-chip-all"
		}
		n.chips[chip.kind] = p.Button(id, chip.name, func() { p.state.Kind = chip.kind })
		n.chips[chip.kind].AddClass("dt-chip")
		bar.Append(n.chips[chip.kind])
	}
	return Tool{Bar: bar, Body: ui.Div(ui.Class("dt-body"), n.rows, n.details), Show: n.show,
		Update: func(shown bool) {
			if shown && p.log.Version() != n.seen {
				n.list()
			}
		}}
}

func (n *network) show() {
	for kind, chip := range n.chips {
		toggle(chip, "chosen", kind == n.p.state.Kind)
	}
	if n.p.doc.Value(n.filter) != n.p.state.Filter {
		n.p.doc.SetValue(n.filter, n.p.state.Filter)
	}
	n.list()
}

// list fills the table with the requests the filters keep, in the order
// of the column chosen, the details of the selected one beside them,
// and sums them up in the footer.
func (n *network) list() {
	p := n.p
	n.seen = p.log.Version()
	all := p.log.Rows()
	kept := netlog.Filter(all, p.state.Filter, p.state.Kind)
	sorted(kept, p.state.Sort)
	fill(n.rows, n.head())
	for _, r := range kept {
		n.rows.Append(n.row(r))
	}
	n.detail(all)
	p.SetFoot(summary(len(all), kept, p.log.Dropped()))
}

// head is the table's first line: the name of each column, which a
// click sorts the rows by, a second the other way, and a third no
// more.
func (n *network) head() *ui.Node {
	head := ui.Div(ui.Class("dt-row"), ui.Class("dt-head"))
	for i, name := range columns {
		column := i + 1
		cell := ui.Span(ui.ID("dt-column-"+strings.ToLower(name)), ui.Class("dt-"+strings.ToLower(name)),
			ui.Attr("role", "button"), ui.TabIndex(0), ui.Span(ui.Text(name)), ui.OnClick(func() {
				switch n.p.state.Sort {
				case column:
					n.p.state.Sort = -column
				case -column:
					n.p.state.Sort = 0
				default:
					n.p.state.Sort = column
				}
				n.list()
			}))
		switch n.p.state.Sort {
		case column:
			cell.Append(n.p.Icon("up"))
		case -column:
			cell.Append(n.p.Icon("down"))
		}
		head.Append(cell)
	}
	return head
}

// sorted puts rows in the order of a column, numbered from 1, the last
// first for one below 0; 0 leaves them as they were asked.
func sorted(rows []netlog.Row, column int) {
	if column == 0 {
		return
	}
	by := []func(a, b netlog.Row) int{
		func(a, b netlog.Row) int { return cmp.Compare(a.Status, b.Status) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Method, b.Method) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Host(), b.Host()) },
		func(a, b netlog.Row) int { return cmp.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name())) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Initiator, b.Initiator) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Kind(), b.Kind()) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Transferred, b.Transferred) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Size, b.Size) },
		func(a, b netlog.Row) int { return cmp.Compare(a.Took, b.Took) },
	}[max(column, -column)-1]
	slices.SortStableFunc(rows, func(a, b netlog.Row) int {
		if column < 0 {
			return by(b, a)
		}
		return by(a, b)
	})
}

func (n *network) row(r netlog.Row) *ui.Node {
	p := n.p
	domain := ui.Span(ui.Class("dt-domain"))
	if r.Secure() {
		domain.Append(p.Icon("lock"))
	}
	domain.Append(ui.Span(ui.Text(r.Host())))
	row := ui.Div(ui.Class("dt-row"), ui.ID(rowID+strconv.Itoa(r.ID)), ui.TabIndex(0), n.status(r),
		ui.Span(ui.Class("dt-method"), ui.Text(r.Method)), domain,
		ui.Span(ui.Class("dt-file"), ui.Text(r.Name())),
		ui.Span(ui.Class("dt-initiator"), ui.Text(r.Initiator)),
		ui.Span(ui.Class("dt-type"), ui.Text(r.Kind())),
		ui.Span(ui.Class("dt-transferred"), ui.Text(transferred(r))),
		ui.Span(ui.Class("dt-size"), ui.Text(size(r.Size))),
		ui.Span(ui.Class("dt-duration"), ui.Text(took(r))),
		ui.OnClick(func() {
			p.state.Selected = r.ID
			n.list()
		}),
		ui.OnPointerEnter(func() { p.host.Point(r.Address) }),
		ui.OnPointerLeave(func() { p.host.Point("") }))
	toggle(row, "failed", r.Failed())
	toggle(row, "blocked", r.Blocked)
	toggle(row, "redirect", !r.Failed() && r.Status >= 300)
	toggle(row, "selected", r.ID == p.state.Selected)
	return row
}

// status is a row's first cell: the answer's code on the colour of its
// kind, a sign for a request that was blocked or got no answer, and
// nothing until one comes.
func (n *network) status(r netlog.Row) *ui.Node {
	cell := ui.Span(ui.Class("dt-status"))
	switch {
	case r.Blocked:
		cell.Append(n.p.Icon("blocked"))
	case r.Err != "":
		cell.Append(n.p.Icon("error"))
	case r.Status != 0:
		badge := ui.Span(ui.Class("dt-code"), ui.Text(strconv.Itoa(r.Status)))
		toggle(badge, "dt-moved", r.Status >= 300 && r.Status < 400)
		toggle(badge, "dt-bad", r.Status >= 400)
		cell.Append(badge)
	}
	return cell
}

// detail fills the details with the selected row's headers or response,
// the table beside them keeping its first columns only, and hides them
// when no row of all is selected.
func (n *network) detail(all []netlog.Row) {
	p := n.p
	at := slices.IndexFunc(all, func(r netlog.Row) bool { return r.ID == p.state.Selected })
	toggle(n.details, "shown", at >= 0)
	toggle(n.rows, "beside", at >= 0)
	if at < 0 {
		p.state.Selected = 0
		fill(n.details)
		return
	}
	r := all[at]
	headers := p.Button("dt-headers", "Headers", func() { p.state.Response = false })
	response := p.Button("dt-response", "Response", func() { p.state.Response = true })
	headers.AddClass("dt-chip")
	response.AddClass("dt-chip")
	toggle(headers, "chosen", !p.state.Response)
	toggle(response, "chosen", p.state.Response)
	shown := ui.Div(ui.ID("dt-detail"))
	if p.state.Response {
		shown.Append(n.response(r)...)
	} else {
		shown.Append(headersOf(r, netlog.Chain(all, r.ID))...)
	}
	fill(n.details,
		ui.Div(ui.Class("dt-bar"), headers, response, ui.Div(ui.Class("dt-space")),
			ui.Button(ui.ID("dt-details-close"), ui.Class("dt-glyph"), ui.Attr("aria-label", "Close the details"),
				p.Icon("close"), ui.OnClick(func() {
					p.state.Selected = 0
					n.list()
				}))),
		shown)
}

func headersOf(r netlog.Row, chain []netlog.Row) []*ui.Node {
	out := []*ui.Node{ui.H4(ui.Text("General")), pair("Address", r.Address), pair("Method", r.Method),
		pair("Status", strings.TrimSpace(status(r)+" "+r.Reason)), pair("Initiator", r.Initiator)}
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
func (n *network) response(r netlog.Row) []*ui.Node {
	preview := ui.Pre(ui.ID("dt-preview"))
	out := []*ui.Node{preview}
	switch kind, media := content.Of(r.Response.Get("Content-Type"), r.Preview, r.Address); {
	case len(r.Preview) == 0:
		out = []*ui.Node{ui.Div(ui.Class("dt-none"), ui.Text("This request has no body to show."))}
	case kind == content.Picture && r.Done && !r.Truncated:
		name := "netlog:" + strconv.Itoa(r.ID)
		n.p.host.Serve(name, content.Drawable(r.Preview))
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
	case r.Blocked:
		return "(blocked)"
	case r.Err == "cancelled":
		return "(cancelled)"
	case r.Err != "":
		return "(failed)"
	case r.Status == 0:
		return "(pending)"
	}
	return strconv.Itoa(r.Status)
}

// transferred is what the server sent of a row, or why it sent nothing.
func transferred(r netlog.Row) string {
	switch {
	case r.Blocked:
		return "Blocked"
	case r.Err == "cancelled":
		return "Cancelled"
	case r.Err != "" && r.Status == 0:
		return "Failed"
	}
	return size(r.Transferred)
}

func size(bytes int64) string {
	switch {
	case bytes < 1000:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1000*1000:
		return fmt.Sprintf("%.2f kB", float64(bytes)/1000)
	}
	return fmt.Sprintf("%.2f MB", float64(bytes)/(1000*1000))
}

func took(r netlog.Row) string {
	if ms := r.Took.Milliseconds(); ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.2f s", r.Took.Seconds())
}

// summary is the footer: how many requests are listed, of how many, the
// bytes sent of those read and the time to the last answer, and how
// many went past the cap.
func summary(all int, kept []netlog.Row, dropped int) string {
	if all == 0 && dropped == 0 {
		return "No requests"
	}
	total := netlog.Totals(kept)
	count := counted(all, "request")
	if len(kept) != all {
		count = fmt.Sprintf("%d of %s", len(kept), count)
	}
	text := fmt.Sprintf("%s · %s / %s transferred · Finish: %s", count, size(total.Bytes), size(total.Transferred),
		took(netlog.Row{Took: total.Took}))
	if dropped > 0 {
		text += fmt.Sprintf(" · %d older requests dropped", dropped)
	}
	return text
}
