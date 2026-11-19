// Package web turns an address of the web into what light shows for it:
// the answer fetched, read by its kind, and for HTML built as a ui
// Document of its own, once adapted (adaptations.go).
package web

import (
	"html"
	"image"
	"net/http"
	"net/url"
	"sync"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/localnet"
)

// Page is what an address gave, or why it gave nothing.
type Page struct {
	// Address is where the answer came from, after its redirects.
	Address string
	Title   string
	// Status is the answer's HTTP status.
	Status int
	// Space is how far from the Internet the address that answered is:
	// what the page's own requests may reach. A page of light's is of
	// this machine.
	Space localnet.Space
	Kind  content.Kind
	// Media is the media type the answer is taken for.
	Media string
	// Encoding is the name of the encoding an HTML page came in, as the
	// DOM spells it, "" where none was read.
	Encoding string
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
	// Base is the address what the page links to is read from.
	Base *url.URL
	// Frames are the iframes the page's Document was built with, and
	// those its scripts make later: nil for a page that is no Document.
	Frames *Frames
	// Icon is the site's icon as a picture ui draws, nil for none.
	Icon []byte
	// Timing is when each step of the load ended, nil for a load that
	// was given none.
	Timing *Timing
	// Empty is an answer with nothing to show (204, 205, no body).
	Empty bool
	// Err is why there is no page; the other fields but Address are then
	// unset.
	Err error
}

// Links is what a page's links and forms do: open an address, name it
// while the pointer is over one, save it, under a name when the link
// gives one, send a form with a POST, and tell why a form was not sent.
type Links struct {
	Open, Over func(address string)
	Save       func(address, name string)
	Post       func(sent Submission)
	Unsent     func(why error)
	// Aimed, when set, is asked to open an address a link or a form
	// with a target leads to, and reports whether it did: Open is left
	// what it did not.
	Aimed func(address, target string) bool
}

// Frames are the iframe elements of a page as they are built, for
// whoever shows a document in each: built off the frame as the page
// loads, then on it as its scripts make more.
type Frames struct {
	mu    sync.Mutex
	fresh []*ui.Node
}

func (f *Frames) add(n *ui.Node) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fresh = append(f.fresh, n)
}

// Take is the iframe elements built since it was last asked.
func (f *Frames) Take() []*ui.Node {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	fresh := f.fresh
	f.fresh = nil
	return fresh
}

// Frame is what a Loader is told of the frame it loads documents for.
type Frame struct {
	// Top is the address of the tab's page, which the frame's requests
	// are a third party's or not by, and Chain those of the documents
	// the frame is in, the tab's page aside, nil for one with no origin:
	// a frame that is, or is in, a document of another site than Top's
	// keeps its cookies under Top's site (cookies.Framed).
	Top   *url.URL
	Chain []*url.URL
	// Admit is why a document answered with those headers at that
	// address is not shown in the frame, asked before anything of it is
	// read; nil when it is.
	Admit func(header http.Header, at *url.URL) error
	// Opaque is a frame whose documents have an origin nothing shares:
	// its requests carry and keep no cookie, and its forms say they
	// come from "null". NoForms is one whose forms are not sent.
	Opaque, NoForms bool
}

// Notice is a small document of light's own that says text, for a
// frame that shows nothing of the web.
func Notice(text string) *ui.Document {
	return said(`<body style="margin:0;padding:8px;background:#f1f3f4;color:#5f6368;font:12px/16px sans-serif">` +
		html.EscapeString(text))
}

// Text is a text of the web as a frame shows it, as it was sent.
func Text(source string) *ui.Document {
	return said(`<body><pre style="margin:0;white-space:pre-wrap">` + html.EscapeString(source))
}

func said(markup string) *ui.Document {
	return (&Loader{}).Local("about:blank", markup, Links{}).Doc
}
