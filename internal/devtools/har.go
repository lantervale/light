package devtools

import (
	"bytes"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/har"
	"github.com/lantervale/light/pkg/netlog"
)

// ExportID is the id of the Network bar's button that saves a HAR file.
const ExportID = "dt-har"

// Exported is what a HAR file saved without its sensitive data lacks,
// and what it still holds.
const Exported = "cookies and credentials redacted; addresses and bodies, a form's fields among them, as sent"

// export is the bar's button that saves the requests listed as a HAR
// file, which the screen writes. The pointer over it says what the file
// holds, until a click, which leaves the screen to say what came of it.
func (n *network) export() *ui.Node {
	p := n.p
	return ui.Button(ui.ID(ExportID), ui.Class("dt-glyph"), ui.Attr("aria-label", "Save all as HAR"), p.Icon("save"),
		ui.OnClick(func() {
			p.host.Point("")
			if p.host.Export != nil {
				p.host.Export()
			}
		}),
		ui.OnPointerEnter(func() { p.host.Point("Save all as HAR (" + Exported + ")") }),
		ui.OnPointerLeave(func() { p.host.Point("") }))
}

// Listed are the requests the Network tab lists, by its filters, in the
// order they were asked.
func (p *Panel) Listed() []netlog.Row {
	return netlog.Filter(p.log.Rows(), p.state.Filter, p.state.Kind)
}

// HAR is the HAR file of the requests the Network tab lists, or of the
// request numbered id alone when it is not 0: sanitized as har.Sanitize
// does unless sensitive.
func (p *Panel) HAR(id int, sensitive bool) ([]byte, error) {
	kept := map[int]bool{}
	for _, r := range p.Listed() {
		kept[r.ID] = id == 0 || r.ID == id
	}
	archive := har.From(p.log.Rows(), har.Options{Version: p.host.Version, Title: p.host.Title,
		Keep: func(r netlog.Row) bool { return kept[r.ID] || r.ID == id }})
	if !sensitive {
		har.Sanitize(archive)
	}
	var out bytes.Buffer
	err := har.Encode(&out, archive)
	return out.Bytes(), err
}
