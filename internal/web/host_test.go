package web //nolint:testpackage // the adaptations and the caps tested are unexported

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/twod"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/fetch"
)

// glide is how many frames a scroll by the wheel takes to land, as
// Chrome glides it: 12 at most, and one to start.
const glide = 14

func testLoader() *Loader { return &Loader{Client: fetch.New(nil)} }

// load is the page at address, its links doing nothing.
func load(t *testing.T, address string) *Page {
	t.Helper()
	nothing := func(string) {}
	return testLoader().Load(t.Context(), address, Links{Open: nothing, Over: nothing})
}

// asked are the paths a test server was asked for.
type asked struct {
	sync.Mutex
	paths []string
}

func (a *asked) has(path string) bool {
	a.Lock()
	defer a.Unlock()
	return slices.Contains(a.paths, path)
}

// named is the address a page's links last named under the pointer.
type named struct{ address string }

func (n *named) Text() string { return n.address }

// shown is a window showing one page as light does: its Document drawn
// in a canvas of a Document of the window's, under a header.
type shown struct {
	t      *testing.T
	doc    *ui.Document
	host   *ui.Node
	in     *input.State
	frame  func()
	page   *Page
	status *named
	// address is the page shown, or the one a click on a link asked for,
	// saved what a click on a download link asked to save.
	address, saved string
	// posted is each address a form was posted to.
	posted []string
	// scripts loads the pages visited with their scripts.
	scripts bool
}

// click presses and releases the left button over n, an element of the
// page shown.
func (s *shown) click(n *ui.Node) {
	s.t.Helper()
	host, _ := s.doc.BoxOf(s.host)
	s.in.SetMouse(n.Box().Center().Add(geom.V2(host.X, host.Y)))
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

func show(t *testing.T) *shown {
	t.Helper()
	s := &shown{t: t, status: &named{}}
	win := platform.NewWindow(platform.Size(1024, 680))
	s.doc = ui.NewDocument(ui.CSS(`html, body { height: 100%; }
		body { margin: 0; display: flex; flex-direction: column; }
		header { height: 85px; } main { flex: 1; min-height: 0; } canvas { display: block; width: 100%; height: 100%; }`))
	s.host = ui.Canvas(func(c *twod.Canvas) {
		if s.page != nil && s.page.Doc != nil {
			s.page.Doc.Draw(c)
		}
	})
	s.doc.Body().Append(ui.Header(), ui.Main(s.host))
	win.Add(s.doc)
	w := ember.New().AddEntities(win)
	s.in = win.Input()
	s.frame = func() {
		s.doc.Invalidate(s.host)
		w.RunFrames(1)
	}
	s.frame()
	return s
}

// visit shows the page at address and gives its Document once laid out.
func (s *shown) visit(address string) *ui.Document {
	s.t.Helper()
	s.address = address
	loader := testLoader()
	loader.Scripts = s.scripts
	s.page = loader.Load(s.t.Context(), address, Links{
		Open: func(to string) { s.address = to },
		Over: func(over string) { s.status.address = over },
		Save: func(from, name string) { s.saved = from + " as " + name },
		Post: func(sent Submission) { s.posted = append(s.posted, sent.Address) },
	})
	if s.page.Err != nil {
		s.t.Fatalf("%s: %v", address, s.page.Err)
	}
	for range 3 {
		s.frame()
	}
	return s.page.Doc
}

// until runs frames until held holds, for what lands after a page shows.
func (s *shown) until(what string, held func() bool) {
	s.t.Helper()
	for range 600 {
		s.frame()
		if held() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.t.Fatalf("never %s", what)
}

func (s *shown) at(address string) {
	s.t.Helper()
	if s.address != address {
		s.t.Fatalf("the window is at %q, want %q", s.address, address)
	}
}

func box(t *testing.T, doc *ui.Document, id string) geom.Rect {
	t.Helper()
	n := doc.ByID(id)
	if n == nil {
		t.Fatalf("the page has no #%s", id)
	}
	r, ok := doc.BoxOf(n)
	if !ok {
		t.Fatalf("#%s has no box", id)
	}
	return r
}
