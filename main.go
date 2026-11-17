// A web explorer: tabs in the window's own title bar, a navigation row
// and an address field, a status bubble over the page, back and forward
// over a few pages built in Go, all plain ui elements and one stylesheet. The rules
// (a tab's history, what a typed address means) are plain Go beside it.
// Its own pages are ember://home, guide, examples and about; an http or
// https address is fetched with its stylesheets and pictures and built as
// a Document of its own, drawn in a canvas of the explorer's (web.go),
// once adapted for ui: every adaptation is listed in adaptations.go.
//
//	go run .
package main

import (
	"github.com/lantervale/ember"
	"github.com/lantervale/ember/color"
	"github.com/lantervale/ember/platform"
)

func main() {
	win := platform.NewWindow(
		platform.Title("ember · web explorer"),
		platform.Size(1024, 680),
		platform.MinSize(760, 480),
		platform.OwnTitleBar(),
		platform.Background(color.Hex("#2a2530")),
	)
	e := newExplorer(win)
	win.Add(e.doc)
	ember.New().AddEntities(win).AddSystems(e).MustRun()
}
