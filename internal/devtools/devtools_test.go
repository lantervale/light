package devtools_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/align"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/netlog"
)

type shown struct {
	t      *testing.T
	doc    *ui.Document
	panel  *devtools.Panel
	state  *devtools.State
	log    *netlog.Log
	srv    *httptest.Server
	in     *input.State
	frame  func()
	named  string
	served map[string][]byte
	closed int
	warns  []error
}

func site(t *testing.T) *httptest.Server {
	t.Helper()
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewNRGBA(image.Rect(0, 0, 30, 20))); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Crypt", "flooded")
		_, _ = io.WriteString(w, "<p>\r\n\thello</p>")
	})
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusFound) })
	mux.HandleFunc("/css/site.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, strings.Repeat("a { color: red }\n", 100))
	})
	mux.HandleFunc("/map.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pic.Bytes())
	})
	mux.HandleFunc("/gone.css", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func show(t *testing.T) *shown {
	t.Helper()
	s := &shown{t: t, state: &devtools.State{Open: true}, log: netlog.New(100), srv: site(t),
		served: map[string][]byte{}}
	s.doc = ui.NewDocument(ui.CSS("html, body { height: 100%; } body { margin: 0; display: flex; flex-direction: "+
		"column; }"+
		" main { flex: 1; min-height: 0; }"), ui.CSS(devtools.Sheet),
		ui.FetchImages(func(name string) ([]byte, error) {
			if data, ok := s.served[name]; ok {
				return data, nil
			}
			return nil, errors.New("not served")
		}))
	s.panel = devtools.New(s.doc, devtools.Host{
		Point:  func(address string) { s.named = address },
		Serve:  func(name string, picture []byte) { s.served[name] = picture },
		Closed: func() { s.closed++ },
	})
	s.doc.Body().Append(ui.Main(), s.panel.Node())
	win := platform.NewWindow(platform.Size(1024, 680))
	win.Add(s.doc)
	w := ember.New().AddEntities(win)
	s.in = win.Input()
	s.frame = func() {
		s.panel.Update()
		w.RunFrames(1)
	}
	s.panel.Show(s.state, s.log, func() []error { return s.warns })
	s.frame()
	return s
}

func (s *shown) get(path, initiator string, wanted netlog.Type) {
	s.t.Helper()
	req, err := http.NewRequestWithContext(netlog.With(s.t.Context(), s.log, initiator, wanted), http.MethodGet,
		s.srv.URL+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// load makes the requests of a page: a redirect to it, a sheet, one that
// is not there and a picture.
func (s *shown) load() {
	s.get("/old", "navigation", netlog.Document)
	s.get("/css/site.css", "document", netlog.Stylesheet)
	s.get("/gone.css", "document", netlog.Stylesheet)
	s.get("/map.png", "picture", netlog.Image)
	s.frame()
	s.frame()
}

func (s *shown) node(id string) *ui.Node {
	s.t.Helper()
	n := s.doc.ByID(id)
	if n == nil {
		s.t.Fatalf("no element %q", id)
	}
	return n
}

func (s *shown) click(id string) {
	s.t.Helper()
	r, ok := s.doc.BoxOf(s.node(id))
	if !ok {
		s.t.Fatalf("%q is not shown", id)
	}
	s.in.SetMouse(r.Center())
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

func (s *shown) rows() []string {
	var out []string
	for _, row := range s.node("dt-rows").Children()[1:] {
		var cells []string
		for _, cell := range row.Children() {
			cells = append(cells, cell.Text())
		}
		out = append(out, strings.Join(cells, "|"))
	}
	return out
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

func TestEachRequestIsARowNewestLast(t *testing.T) {
	s := show(t)
	if text := s.node("dt-foot").Text(); text != "No requests" {
		t.Fatalf("an empty log's footer reads %q", text)
	}
	s.load()
	rows := s.rows()
	if len(rows) != 5 {
		t.Fatalf("%d rows: %v", len(rows), rows)
	}
	for i, want := range []string{
		"old|302|GET|document|navigation|", "127.0.0.1", "site.css|200|GET|stylesheet|document|1.7 kB|",
		"gone.css|404|GET|stylesheet|document|", "map.png|200|GET|image|picture|",
	} {
		if !strings.HasPrefix(rows[i], want) || !strings.HasSuffix(rows[i], " ms") {
			t.Errorf("row %d reads %q, want it to begin %q and end with its time", i, rows[i], want)
		}
	}
	list := s.node("dt-rows").Children()
	if !list[1].HasClass("redirect") || !list[4].HasClass("failed") || list[3].HasClass("failed") {
		t.Error("a redirect is not dimmed, or a 404 not marked failed, or a 200 is")
	}
	if text := s.node("dt-foot").Text(); !strings.HasPrefix(text, "5 requests · ") || !strings.Contains(text,
		" kB · ") {
		t.Errorf("the footer reads %q", text)
	}
	s.clean()
}

func TestTheFooterCountsTheRowsDroppedPastTheCap(t *testing.T) {
	s := show(t)
	s.log = netlog.New(2)
	s.panel.Show(s.state, s.log, func() []error { return nil })
	s.load()
	if text := s.node("dt-foot").Text(); !strings.HasPrefix(text, "2 requests · ") || !strings.HasSuffix(text,
		" · 3 older requests dropped") {
		t.Errorf("the footer reads %q", text)
	}
}

func TestARowUnderThePointerNamesItsAddress(t *testing.T) {
	s := show(t)
	s.load()
	r, _ := s.doc.BoxOf(s.node("dt-row-3"))
	s.in.SetMouse(r.Center())
	s.frame()
	s.frame()
	if s.named != s.srv.URL+"/css/site.css" {
		t.Fatalf("the row names %q", s.named)
	}
	if row, ok := s.panel.RowAt(s.node("dt-row-3").Children()[0]); !ok || row.Address != s.srv.URL+"/css/site.css" {
		t.Fatalf("the row at its name cell is %v, %v", row.Address, ok)
	}
	if _, ok := s.panel.RowAt(s.node("dt-foot")); ok {
		t.Fatal("the footer is a row")
	}
}

func TestASelectedRowShowsItsHeadersAndItsResponse(t *testing.T) {
	s := show(t)
	if _, shown := s.doc.BoxOf(s.node("dt-details")); shown {
		t.Fatal("details show with no row selected")
	}
	s.load()
	s.click("dt-row-2")
	text := s.node("dt-details").Text()
	for _, want := range []string{"Address" + s.srv.URL + "/", "MethodGET", "Status200 OK", "Remote address127.0.0.1:",
		"Redirected from" + s.srv.URL + "/old", "Response headers", "X-Cryptflooded", "Content-Typetext/html",
		"Request headers"} {
		if !strings.Contains(text, want) {
			t.Errorf("the details lack %q in %q", want, text)
		}
	}
	if !s.node("dt-row-2").HasClass("selected") || s.state.Selected != 2 {
		t.Error("the row is not marked selected")
	}
	s.click("dt-response")
	if body := s.node("dt-preview").Text(); body != "<p>\n        hello</p>" {
		t.Errorf("the response reads %q", body)
	}
	s.clean()

	s.click("dt-row-5")
	img := s.node("dt-preview").Children()[0]
	name, _ := img.Attr("src")
	if !bytes.HasPrefix(s.served[name], []byte("\x89PNG")) {
		t.Fatalf("the picture %q is not served: %d bytes", name, len(s.served[name]))
	}
	s.click("dt-row-4")
	if body := s.node("dt-preview").Text(); !strings.Contains(body, "404 page not found") {
		t.Errorf("a 404's response reads %q", body)
	}
	s.click("dt-headers")
	if headers := s.node("dt-details").Text(); !strings.Contains(headers, "Status404 Not Found") {
		t.Errorf("back on its headers: %q", headers)
	}
	s.click("dt-details-close")
	if _, shown := s.doc.BoxOf(s.node("dt-details")); shown || s.state.Selected != 0 {
		t.Fatal("the details stay after their close button")
	}
}

func TestRowsAreFilteredByTheFieldAndTheChips(t *testing.T) {
	s := show(t)
	s.load()
	s.click("dt-chip-stylesheet")
	if rows := s.rows(); len(rows) != 2 || !s.node("dt-chip-stylesheet").HasClass("chosen") {
		t.Fatalf("CSS keeps %v", rows)
	}
	s.click("dt-filter")
	for _, r := range "gone" {
		s.in.AddText(r)
	}
	s.frame()
	s.frame()
	if rows := s.rows(); len(rows) != 1 || !strings.HasPrefix(rows[0], "gone.css") || s.state.Filter != "gone" {
		t.Fatalf("CSS and “gone” keep %v", rows)
	}
	if !s.doc.Focused(s.node("dt-filter")) {
		t.Fatal("typing took the focus off the filter")
	}
	s.click("dt-chip-all")
	if rows := s.rows(); len(rows) != 1 {
		t.Fatalf("All and “gone” keep %v", rows)
	}
	if text := s.node("dt-foot").Text(); !strings.HasPrefix(text, "1 of 5 requests · ") {
		t.Errorf("the footer reads %q", text)
	}
}

func TestClearEmptiesTheLogAndPreserveIsKept(t *testing.T) {
	s := show(t)
	s.load()
	s.click("dt-row-2")
	s.click("dt-preserve")
	if !s.state.Preserve {
		t.Fatal("the checkbox leaves Preserve off")
	}
	s.click("dt-clear")
	if len(s.log.Rows()) != 0 || len(s.rows()) != 0 || s.state.Selected != 0 {
		t.Fatalf("after Clear: %d rows logged, %d shown, row %d selected", len(s.log.Rows()), len(s.rows()),
			s.state.Selected)
	}
	other := &devtools.State{Open: true}
	s.panel.Show(other, s.log, func() []error { return nil })
	s.frame()
	if s.doc.Checked(s.node("dt-preserve")) {
		t.Fatal("another tab's panel shows this one's Preserve")
	}
}

func TestThePanelShowsOnlyWhileOpenAndItsButtonClosesIt(t *testing.T) {
	s := show(t)
	if box, shown := s.doc.BoxOf(s.panel.Node()); !shown || box.H < 200 || box.Y+box.H != 680 {
		t.Fatalf("the open panel is at %v", box)
	}
	s.click("dt-close")
	if s.state.Open || s.closed != 1 {
		t.Fatalf("after the close button: open %v, told %d times", s.state.Open, s.closed)
	}
	s.panel.Show(s.state, s.log, func() []error { return nil })
	s.frame()
	if _, shown := s.doc.BoxOf(s.panel.Node()); shown {
		t.Fatal("a closed panel is laid out")
	}
}

func TestTheWarningsTabListsWhatUIRefusedInThePage(t *testing.T) {
	s := show(t)
	s.warns = []error{errors.New("site.css:3: animation is not read"), errors.New("image /lost.png: 404")}
	s.panel.Show(s.state, s.log, func() []error { return s.warns })
	s.frame()
	if text := s.node("dt-tab-warnings").Text(); text != "Warnings ("+strconv.Itoa(2)+")" {
		t.Fatalf("the tab reads %q", text)
	}
	s.click("dt-tab-warnings")
	list := s.node("dt-warnings").Children()
	if len(list) != 2 || list[0].Text() != "site.css:3: animation is not read" {
		t.Fatalf("%d warnings listed", len(list))
	}
	s.clean()
	s.click("dt-tab-network")
	if s.doc.ByID("dt-warnings") != nil || s.doc.ByID("dt-rows") == nil {
		t.Fatal("the Network tab does not come back")
	}
}
