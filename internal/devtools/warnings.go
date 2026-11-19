package devtools

import (
	"fmt"

	"github.com/lantervale/ember/ui"
)

// newWarnings is the Warnings tab: what ui refused in the page's sheets
// and pictures.
func newWarnings(p *Panel) Tool {
	list := ui.Div(ui.ID("dt-warnings"))
	label := func() []error {
		warnings := p.warnings()
		p.SetLabel(warningsTab, fmt.Sprintf("Warnings (%d)", len(warnings)))
		return warnings
	}
	return Tool{Body: ui.Div(ui.Class("dt-body"), list),
		Show: func() {
			fill(list)
			warnings := label()
			for _, w := range warnings {
				list.Append(ui.Div(ui.Class("dt-warning"), p.Icon("warn"), ui.Span(ui.Text(w.Error()))))
			}
			if len(warnings) == 0 {
				list.Append(ui.Div(ui.Class("dt-none"), ui.Text("ui read this tab's page without a warning.")))
			}
			p.SetFoot("What ui refused in the page's sheets and pictures")
		},
		Update: func(bool) { label() }}
}
