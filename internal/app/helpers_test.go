package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/align"
)

func picture(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// answer is what a test server sends for a path.
type answer struct {
	status int
	header map[string]string
	body   string
}

func typed(media, body string) answer {
	return answer{header: map[string]string{"Content-Type": media}, body: body}
}

func moved(to string) answer {
	return answer{status: http.StatusFound, header: map[string]string{"Location": to}}
}

// answers serves each path with its answer, a body-less 404 for the
// rest, and counts the requests of each path.
func answers(t *testing.T, by map[string]answer) (*httptest.Server, *asked) {
	t.Helper()
	seen := &asked{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Lock()
		seen.paths = append(seen.paths, r.URL.Path)
		seen.Unlock()
		a, ok := by[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for name, value := range a.header {
			w.Header().Set(name, value)
		}
		if a.status != 0 {
			w.WriteHeader(a.status)
		}
		_, _ = w.Write([]byte(a.body))
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func (a *asked) count(path string) int {
	a.Lock()
	defer a.Unlock()
	n := 0
	for _, p := range a.paths {
		if p == path {
			n++
		}
	}
	return n
}

// crypt is a small site: a page with a link and a picture, a sheet, and
// one file of each kind light shows.
func crypt(t *testing.T) (*httptest.Server, *asked) {
	t.Helper()
	return answers(t, map[string]answer{
		"/": typed("text/html", `<title>A crypt</title><link rel="stylesheet" href="/site.css"><body>
			<p id="words">The lower vaults stay wet.</p>
			<a id="vault" href="/vaults">the vault page</a><br>
			<img id="map" src="/map.png" alt="The map">
			<p><a id="framed" href="/vaults"><img id="door" src="/map.png" alt="A door"></a></p>
			<div id="space" style="height: 200px"></div></body>`),
		"/site.css":  typed("text/css", "body { margin: 0; } a { display: inline-block; }\n#map { display: block; }\n"),
		"/vaults":    typed("text/html", `<title>Vaults</title><p>The lower vaults stay wet.</p>`),
		"/map.png":   typed("image/png", string(picture(t, 30, 20))),
		"/wide.png":  typed("image/png", string(picture(t, 3000, 300))),
		"/tall.png":  typed("image/png", string(picture(t, 300, 3000))),
		"/notes.txt": typed("text/plain; charset=utf-8", "line one\nline two\n"),
		"/tabs.txt":  typed("text/plain", "a\tb\n\tindented\n12345678\tend"),
		"/dos.css":   typed("text/css", "a {\r\n\tcolor: red;\r\n}\r\n"),
		"/app.js":    typed("text/javascript", "let a = 1;\n"+strings.Repeat("x", 4000)+"\n"),
		"/paper.pdf": typed("application/pdf", "%PDF-1.7 a paper"),
		"/report": {header: map[string]string{"Content-Type": "text/plain",
			"Content-Disposition": `attachment; filename="../q3.txt"`}, body: "figures"},
		"/files": typed("text/html", `<title>Files</title><style>a { display: block; height: 20px; }</style>
			<a id="named" href="/notes.txt" download="kept notes.txt">named</a>
			<a id="paper" href="/paper.pdf">a paper</a>`),
		"/old":         moved("/vaults"),
		"/nothing":     {status: http.StatusNoContent},
		"/sign-in":     {status: http.StatusUnauthorized, body: "who?"},
		"/favicon.ico": typed("image/png", string(picture(t, 16, 16))),
	})
}

// resize sizes the window as its system would at a pixel a point: SetSize
// alone keeps the framebuffer the window was made with, and a page then
// lays out at a fraction of a device pixel a point, where its line
// heights floor to that pixel's layout unit.
func (s *shown) resize(size geom.Vec2) {
	s.win.SetSize(size)
	s.win.Sync(size, size, s.win.Focused())
}

func (s *shown) press(k input.Key) {
	s.in.SetKey(k, true)
	s.frame()
	s.in.SetKey(k, false)
	s.frame()
	s.frame()
}

func (s *shown) clickAt(at geom.Vec2) {
	s.in.SetMouse(at)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

func (s *shown) rightClickAt(at geom.Vec2) {
	s.in.SetMouse(at)
	s.frame()
	s.in.SetMouseButton(input.MouseRight, true)
	s.frame()
	s.in.SetMouseButton(input.MouseRight, false)
	s.frame()
	s.frame()
}

// rightClick right-clicks the middle of the screen's element id.
func (s *shown) rightClick(id string) {
	s.t.Helper()
	r, ok := s.doc.BoxOf(s.node(id))
	if !ok {
		s.t.Fatalf("%q is not shown", id)
	}
	s.rightClickAt(r.Center())
}

// inPage is the middle of the shown web page's element id, in the window.
func (s *shown) inPage(id string) geom.Vec2 {
	s.t.Helper()
	page := s.session().pages[s.browser.Tab().Current()]
	if page == nil || page.Doc == nil || s.host == nil {
		s.t.Fatal("no page of the web is shown")
	}
	host, _ := s.doc.BoxOf(s.host)
	return box(s.t, page.Doc, id).Center().Add(geom.V2(host.X, host.Y))
}

func (s *shown) node(id string) *ui.Node {
	s.t.Helper()
	n := s.doc.ByID(id)
	if n == nil {
		s.t.Fatalf("no element %q", id)
	}
	return n
}

// entries are the labels of the open menu's entries, "-" for a line
// between groups, and a "(off)" after one that cannot be chosen.
func (s *shown) entries() []string {
	s.t.Helper()
	if !s.menu.IsOpen() {
		s.t.Fatal("no menu is open")
	}
	var out []string
	for _, item := range s.menu.Node().Children() {
		label := item.Text()
		if role, _ := item.Attr("role"); role == "separator" {
			label = "-"
		}
		if off, _ := item.Attr("aria-disabled"); off == "true" {
			label += " (off)"
		}
		out = append(out, label)
	}
	return out
}

// choose clicks the open menu's entry reading label.
func (s *shown) choose(label string) {
	s.t.Helper()
	for _, item := range s.menu.Node().Children() {
		if item.Text() == label {
			s.clickAt(item.Box().Center())
			s.frame()
			return
		}
	}
	s.t.Fatalf("the menu has no entry %q: %v", label, s.entries())
}

func (s *shown) clean() {
	s.t.Helper()
	if faults := align.Check(s.doc); len(faults) > 0 {
		s.t.Errorf("layout faults: %v", faults)
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		s.t.Errorf("CSS warnings: %v", warns)
	}
}

// requestRow is the id of the developer tools' row of the shown tab's
// request for path.
func (s *shown) requestRow(path string) string {
	s.t.Helper()
	for _, r := range s.session().log.Rows() {
		if u, err := url.Parse(r.Address); err == nil && u.Path == path {
			return "dt-row-" + strconv.Itoa(r.ID)
		}
	}
	s.t.Fatalf("no request for %s", path)
	return ""
}

// downloaded is what the file name holds in the downloads folder, "" for
// no such file.
func (s *shown) downloaded(name string) string {
	data, err := os.ReadFile(filepath.Join(s.opts.Downloads, name))
	if err != nil {
		return ""
	}
	return string(data)
}
