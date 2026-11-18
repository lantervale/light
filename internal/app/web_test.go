package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"
)

const article = `<!doctype html><html class="site"><head><title> A  crypt </title>
<link rel="stylesheet" href="/css/site.css">
<style>p.note { height: 50px } ul li { height: 31px; }</style></head><body id="top">
<h1>The crypt</h1>
<p class="note">It floods <b>every</b> spring, says <a id="vault" href="/vaults?x=1">the vault page</a>.</p>
<script>alert(1)</script>
<ul><li id="torch">Torch</li><li><a id="jump" href="#top">Top</a></li></ul>
<img id="map" src="map.png" alt="The map"><img id="lost" src="/lost.png">
</body></html>`

const siteSheet = `/* a { height: 99px } */
body { margin: 0; }
#vault { display: inline-block; width: 120px; }
a:link { height: 20px; }
@media print { h1 { display: none; } }
h1 { height: 40px; margin: 0; background: url(../art/wall.png); }`

// asked are the paths the test server was asked for.
type asked struct {
	sync.Mutex
	paths []string
}

func (a *asked) has(path string) bool {
	a.Lock()
	defer a.Unlock()
	return slices.Contains(a.paths, path)
}

func serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := serveAsked(t)
	return srv
}

func serveAsked(t *testing.T) (*httptest.Server, *asked) {
	t.Helper()
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 30, 20))); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(article))
	})
	mux.HandleFunc("/css/site.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte(siteSheet))
	})
	for _, path := range []string{"/map.png", "/art/wall.png"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(picture.Bytes())
		})
	}
	mux.HandleFunc("/vaults", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Vaults</title><p>The lower vaults stay wet.</p>`))
	})
	mux.HandleFunc("/notes.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("line one\nline two"))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/lost", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<title>Lost</title><h1 id="own">The crypt has no such room</h1>`))
	})
	seen := &asked{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Lock()
		seen.paths = append(seen.paths, r.URL.Path)
		seen.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
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

// visit opens address and runs frames until its fetch has landed and its
// page is laid out; it gives the page's Document, nil for one that failed.
func (s *shown) visit(address string) *ui.Document {
	s.t.Helper()
	s.Open(address)
	for range 600 {
		s.frame()
		if page := s.session().pages[address]; page != nil {
			s.frame()
			s.frame()
			return page.Doc
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.t.Fatalf("%s never loaded", address)
	return nil
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

func TestAnExternalPageIsBuiltWithItsSheetsAndPictures(t *testing.T) {
	srv, seen := serveAsked(t)
	s := show(t)
	s.Open(srv.URL + "/")
	s.frame()
	if s.doc.ByID("loading") == nil {
		t.Fatal("no loading page while the fetch runs")
	}
	doc := s.visit(srv.URL + "/")
	if got := s.title(srv.URL + "/"); got != "A crypt" {
		t.Errorf("the tab is named %q", got)
	}
	if text := doc.Body().Text(); strings.Contains(text, "alert") || !strings.Contains(text, "It floods every spring") {
		t.Errorf("the page reads %q", text)
	}
	if !doc.HTML().HasClass("site") || doc.Body().ID() != "top" {
		t.Error("html and body lost their attributes")
	}
	host, _ := s.doc.BoxOf(s.host)
	if view, _ := s.doc.BoxOf(s.view); host != view || host.H < 500 {
		t.Errorf("the page has %v of the view's %v", host, view)
	}
	s.until("the map's picture landed", func() bool { return box(t, doc, "map").W == 30 })
	s.until("the h1's background was asked for, from its sheet's folder", func() bool {
		return seen.has("/art/wall.png")
	})
	for id, want := range map[string]geom.Vec2{
		"vault": geom.V2(120, 20), // the linked sheet, its a:link rule
		"torch": geom.V2(0, 31),   // the style element, its ul li rule
		"map":   geom.V2(30, 20),  // the picture's own size
	} {
		r := box(t, doc, id)
		if (want.X != 0 && r.W != want.X) || r.H != want.Y {
			t.Errorf("#%s is %v by %v, want %v", id, r.W, r.H, want)
		}
	}
	if h1 := doc.Body().Children()[1]; h1.Box().H != 40 || h1.Box().Y != 0 {
		t.Errorf("the h1 is at %v: the sheet's body and h1 rules, its @media print left out", h1.Box())
	}
	s.until("the picture that is none is a warning", func() bool {
		return slices.ContainsFunc(doc.Warnings(), func(err error) bool {
			return strings.Contains(err.Error(), "image /lost.png: ")
		})
	})
	if href, _ := doc.ByID("jump").Attr("href"); href != "#top" {
		t.Errorf("a link to the page's own #top has the href %q", href)
	}
	host, _ = s.doc.BoxOf(s.host)
	s.in.SetMouse(box(t, doc, "jump").Center().Add(geom.V2(host.X, host.Y)))
	s.frame()
	s.frame()
	if got := s.status.Text(); got != "" {
		t.Errorf("a link to the page's own #top names %q to open", got)
	}
}

func TestALinkOfAnExternalPageOpensRelativeToIt(t *testing.T) {
	srv := serve(t)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	host, _ := s.doc.BoxOf(s.host)
	at := box(t, doc, "vault").Center().Add(geom.V2(host.X, host.Y))
	s.in.SetMouse(at)
	s.frame()
	s.frame()
	if got := s.status.Text(); got != srv.URL+"/vaults?x=1" {
		t.Errorf("the status line names %q", got)
	}
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
	s.at(srv.URL + "/vaults?x=1")
	if text := s.visit(srv.URL + "/vaults?x=1").Body().Text(); !strings.Contains(text, "lower vaults") {
		t.Fatalf("the linked page is %q", text)
	}
	s.click("back")
	s.at(srv.URL + "/")
	s.until("back shows the page, fetched again when its scripts ran", func() bool { return s.host != nil })
}

func TestLightsOwnSheetStaysOutOfAPage(t *testing.T) {
	srv := serve(t)
	s := show(t)
	before := box(t, s.doc, "bar")
	s.visit(srv.URL + "/")
	if bar := box(t, s.doc, "bar"); bar != before {
		t.Errorf("the page's sheets moved the bar from %v to %v", before, bar)
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		t.Errorf("light's sheet warns: %v", warns)
	}
}

func TestPlainTextAndFailuresHaveTheirPage(t *testing.T) {
	srv := serve(t)
	s := show(t)
	s.visit(srv.URL + "/notes.txt")
	if text := s.doc.ByID("text").Text(); text != "line one\nline two" {
		t.Errorf("plain text shows %q", text)
	}
	s.visit(srv.URL + "/gone")
	if text := s.doc.ByID("page").Text(); !strings.Contains(text, "can't be reached") || !strings.Contains(text,
		"404 Not Found") {
		t.Errorf("a missing page shows %q", text)
	}
	if own := s.visit(srv.URL + "/lost"); own == nil || own.ByID("own") == nil || s.title(srv.URL+"/lost") != "Lost" {
		t.Error("a site's own 404 page is not shown as the page it is")
	}
}

func TestReloadFetchesAgain(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			http.NotFound(w, r)
			return
		}
		hits++
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>hello</p>"))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	s.visit(srv.URL)
	s.click("reload")
	s.visit(srv.URL)
	if hits != 2 {
		t.Fatalf("%d fetches after a reload, want 2", hits)
	}
}
