package app

import (
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/idb"
)

// databasesIn is the IndexedDB databases of the documents of an
// origin, kept apart as their localStorage is (storageIn): top is ""
// for the site's own, and the site of the tab's page for what a frame
// of another site, or in one, keeps under it. A document with no site
// of its own has none but its own, gone with it.
func (s *Screen) databasesIn(top, origin string) *idb.Site {
	switch {
	case origin == "" || origin == opaqueOrigin:
		return nil
	case top == "":
		return s.bases.Site(origin)
	}
	return s.bases.Site(top + "\x00" + origin)
}

// pageDatabases gives the databases of a tab's page, once a script of
// it asks for them.
func (s *Screen) pageDatabases(live *running) func() *idb.Site {
	return func() *idb.Site { return s.databasesIn("", originOf(live.page.Address)) }
}

// frameDatabases is pageDatabases for the document a frame shows.
func (s *Screen) frameDatabases(f *frame, origin string) func() *idb.Site {
	return func() *idb.Site {
		top := ""
		if f.crossed() {
			top = frames.Site(f.set.top.Base)
		}
		return s.databasesIn(top, origin)
	}
}

// clearDatabases forgets every site's databases; the pages that have
// one open are told it closed.
func (s *Screen) clearDatabases() { s.bases.Clear() }

// clearDatabasesSetting is the button that forgets every database.
func (s *Screen) clearDatabasesSetting() *ui.Node {
	return ui.Div(ui.Class("setting"), ui.Button(ui.ID("set-clear-databases"), ui.Text("Clear databases"),
		ui.OnClick(s.clearDatabases)))
}
