package devtools

import (
	"sync"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools/inspector"
)

// InspectorTab is the ID of the Inspector's tab: the page as a tree of
// markup, with the rules, the computed style and the box of the element
// selected in it (package inspector).
const InspectorTab = "inspector"

// inspectors are the Inspectors made by Inspect, by their Document,
// until the panel of that Document takes its own.
//
//nolint:gochecknoglobals // what Inspect hands to the tab's Build
var inspectors sync.Map

//nolint:gochecknoinits // the registration of the Inspector's tab
func init() {
	Register(Tab{ID: InspectorTab, Label: "Inspector", Order: 50, Build: newInspector,
		Icon: `<path d="M12.5 7V3.5h-9v8H7M8.5 8.5l1.8 5 1-2.2 2.2-1z"/>`})
	Sheet += inspector.Sheet
}

// Inspect makes the Inspector of the panel made next for doc, which
// asks host for the page: the screen keeps it, to put its Overlay over
// the page and to select in it. A panel made for a Document without
// one has an Inspector that shows no page.
func Inspect(doc *ui.Document, host inspector.Host) *inspector.Inspector {
	in := inspector.New(doc, host)
	inspectors.Store(doc, in)
	return in
}

func newInspector(p *Panel) Tool {
	var in *inspector.Inspector
	if made, ok := inspectors.LoadAndDelete(p.Doc()); ok {
		in, _ = made.(*inspector.Inspector)
	}
	if in == nil {
		in = inspector.New(p.Doc(), inspector.Host{})
	}
	return Tool{
		Body: in.Body(),
		Show: func() {
			state := p.State()
			if state.Tool == nil {
				state.Tool = map[string]any{}
			}
			kept, _ := state.Tool[InspectorTab].(*inspector.State)
			if kept == nil {
				kept = &inspector.State{}
				state.Tool[InspectorTab] = kept
			}
			in.Show(kept)
			p.SetFoot("")
		},
		Update: in.Update,
	}
}
