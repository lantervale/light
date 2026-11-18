// Package web turns an address of the web into what light shows for it:
// the answer fetched, read by its kind, and for HTML built as a ui
// Document of its own, once adapted (adaptations.go).
package web

import (
	"image"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
)

// Page is what an address gave, or why it gave nothing.
type Page struct {
	// Address is where the answer came from, after its redirects.
	Address string
	Title   string
	// Status is the answer's HTTP status.
	Status int
	Kind   content.Kind
	// Media is the media type the answer is taken for.
	Media string
	// Doc is an HTML page built, nil for the other kinds.
	Doc *ui.Document
	// Source is the text the server sent, decoded: of an HTML page and of
	// text.
	Source string
	// Body is a picture as ui draws it.
	Body []byte
	// Size is a picture's own size, zero for one that scales (SVG).
	Size image.Point
	// FileName is the name the answer is saved under.
	FileName string
	// Scripts are the page's scripts in the order they run, and Tree what
	// they need of the page's builder: both nil unless the page was
	// loaded for scripts to run.
	Scripts []Script
	Tree    *Tree
	// Icon is the site's icon as a picture ui draws, nil for none.
	Icon []byte
	// Empty is an answer with nothing to show (204, 205, no body).
	Empty bool
	// Err is why there is no page; the other fields but Address are then
	// unset.
	Err error
}

// Links is what a page's links do: open an address, name it while the
// pointer is over one, and save it, under a name when the link gives
// one.
type Links struct {
	Open, Over func(address string)
	Save       func(address, name string)
}
