package devtools_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/align"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/netlog"
)

// glide is how many frames a scroll by the wheel takes to land, as
// Chrome glides it: 12 at most, and one to start.
const glide = 14

type shown struct {
	t     *testing.T
	doc   *ui.Document
	panel *devtools.Panel
	state *devtools.State
	log   *netlog.Log
	said  *console.Lines
	asked []string
	// contexts are the documents the console's line may be evaluated
	// in, and askedIn where each line was.
	contexts []devtools.Context
	askedIn  []int
	srv      *httptest.Server
	in       *input.State
	frame    func()
	named    string
	served   map[string][]byte
	closed   int
	// exported counts the times the screen was asked to save a HAR file.
	exported int
	warns    []error
	// unscripted is why the page runs no script, "" when it does.
	unscripted string
	// resized are the heights the screen was told, sources and opened
	// what it was asked to show.
	resized         []int
	sources, opened []string
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
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, "run()")
	})
	mux.HandleFunc("/gone.css", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func show(t *testing.T) *shown {
	t.Helper()
	s := &shown{t: t, state: &devtools.State{Open: true}, log: netlog.New(100), srv: site(t),
		served: map[string][]byte{}, said: console.New(100)}
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
		Evaluate: func(source string) (string, error) {
			s.asked = append(s.asked, source)
			if source == "boom" {
				return "", errors.New("ReferenceError: boom is not defined")
			}
			return "value of " + source, nil
		},
		Contexts: func() []devtools.Context { return s.contexts },
		EvaluateIn: func(context int, source string) (string, error) {
			s.askedIn = append(s.askedIn, context)
			return source + " in " + strconv.Itoa(context), nil
		},
		Unscripted: func() string { return s.unscripted },
		Pointer:    func() (geom.Vec2, bool) { return s.in.Mouse(), s.in.Down(input.MouseLeft) },
		Resized:    func(height int) { s.resized = append(s.resized, height) },
		Source: func(address string, line int) {
			s.sources = append(s.sources, address+"#"+strconv.Itoa(line))
		},
		Open:    func(address string) { s.opened = append(s.opened, address) },
		Export:  func() { s.exported++ },
		Version: "1.2.3",
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
	s.panel.Show(s.state, s.log, s.said, func() []error { return s.warns })
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

// column is what the rows show in the column named.
func (s *shown) column(name string) []string {
	var out []string
	for _, row := range s.node("dt-rows").Children()[1:] {
		for _, cell := range row.Children() {
			if cell.HasClass("dt-" + name) {
				out = append(out, cell.Text())
			}
		}
	}
	return out
}

// visible reports whether the element named id is laid out.
func (s *shown) visible(id string) bool {
	n := s.doc.ByID(id)
	if n == nil {
		return false
	}
	_, ok := s.doc.BoxOf(n)
	return ok
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
	host := strings.TrimPrefix(s.srv.URL, "http://")
	for i, want := range []string{
		"302|GET|" + host + "|old|navigation|html|", "200|GET|" + host + "|" + host + "|navigation|html|",
		"200|GET|" + host + "|site.css|document|css|1.70 kB|1.70 kB|", "404|GET|" + host + "|gone.css|document|plain|",
		"200|GET|" + host + "|map.png|picture|png|",
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
		" kB transferred · Finish: ") {
		t.Errorf("the footer reads %q", text)
	}
	s.clean()
}

func TestTheFooterCountsTheRowsDroppedPastTheCap(t *testing.T) {
	s := show(t)
	s.log = netlog.New(2)
	s.panel.Show(s.state, s.log, s.said, func() []error { return nil })
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
	file, _ := s.doc.BoxOf(s.node("dt-row-2").Children()[3])
	if _, shown := s.doc.BoxOf(s.node("dt-row-2").Children()[7]); shown || file.W < 150 {
		t.Errorf("beside the details the table keeps its last columns, or its file column is %v wide", file.W)
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
	if rows := s.column("file"); len(rows) != 1 || rows[0] != "gone.css" || s.state.Filter != "gone" {
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
	s.panel.Show(other, s.log, s.said, func() []error { return nil })
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
	s.panel.Show(s.state, s.log, s.said, func() []error { return nil })
	s.frame()
	if _, shown := s.doc.BoxOf(s.panel.Node()); shown {
		t.Fatal("a closed panel is laid out")
	}
}

func TestTheWarningsTabListsWhatUIRefusedInThePage(t *testing.T) {
	s := show(t)
	s.warns = []error{errors.New("site.css:3: animation is not read"), errors.New("image /lost.png: 404")}
	s.panel.Show(s.state, s.log, s.said, func() []error { return s.warns })
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
	if s.visible("dt-warnings") || !s.visible("dt-rows") {
		t.Fatal("the Network tab does not come back")
	}
}

// lines are the console's lines as shown: level, text, where from, and
// how many times when said more than once.
func (s *shown) lines() []string {
	var out []string
	for _, line := range s.node("dt-lines").Children() {
		cells := []string{strings.TrimPrefix(line.Classes()[1], "dt-")}
		for _, class := range []string{"dt-text", "dt-where", "dt-times"} {
			line.Walk(func(n *ui.Node) bool {
				if n.HasClass(class) && (class != "dt-times" || n.Text() != "") {
					cells = append(cells, n.Text())
				}
				return true
			})
		}
		out = append(out, strings.Join(cells, "|"))
	}
	return out
}

func (s *shown) press(k input.Key) {
	s.in.SetKey(k, true)
	s.frame()
	s.in.SetKey(k, false)
	s.frame()
	s.frame()
}

func (s *shown) enter(typed string) {
	for _, r := range typed {
		s.in.AddText(r)
	}
	s.frame()
	s.press(input.KeyEnter)
}

func TestTheConsoleTabListsWhatThePagesScriptsSaid(t *testing.T) {
	s := show(t)
	s.said.Add(console.Line{Level: console.Log, Text: "ready", Source: s.srv.URL + "/js/app.js", Line: 12})
	s.said.Add(console.Line{Level: console.Error, Text: "Uncaught TypeError: x is not a function", Source: s.srv.URL + "/",
		Line: 3})
	s.frame()
	if text := s.node("dt-tab-console").Text(); text != "Console" || s.node("dt-console-errors").Text() != "1" {
		t.Fatalf("the tab reads %q", text)
	}
	s.click("dt-tab-console")
	host := strings.TrimPrefix(s.srv.URL, "http://")
	want := []string{"log|ready|app.js:12", "error|Uncaught TypeError: x is not a function|" + host + ":3"}
	if got := s.lines(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the console lists %q, want %q", got, want)
	}
	if !s.node("dt-tab-console").HasClass("chosen") || s.visible("dt-rows") || s.state.Tab != "console" {
		t.Fatal("the Console tab is not the one shown")
	}
	s.clean()
	s.said.Add(console.Line{Level: console.Warn, Text: "late"})
	s.frame()
	s.frame()
	if got := s.lines(); len(got) != 3 || got[2] != "warn|late|" {
		t.Fatalf("a line said while the tab is shown is not listed: %q", got)
	}
	s.click("dt-tab-warnings")
	if s.visible("dt-lines") || s.state.Tab != "warnings" {
		t.Fatal("the Warnings tab leaves the console shown")
	}
	s.click("dt-tab-console")
	s.click("dt-tab-network")
	if s.visible("dt-lines") || !s.visible("dt-rows") {
		t.Fatal("the Network tab does not come back")
	}
}

func TestClearOnTheConsoleEmptiesItAlone(t *testing.T) {
	s := show(t)
	s.load()
	s.said.Add(console.Line{Level: console.Log, Text: "kept until cleared"})
	s.click("dt-tab-console")
	s.click("dt-console-clear")
	if len(s.said.Lines()) != 0 || len(s.lines()) != 0 || len(s.log.Rows()) == 0 {
		t.Fatalf("after Clear the console keeps %d lines, the network %d rows", len(s.said.Lines()), len(s.log.Rows()))
	}
	if text := s.node("dt-lines").Parent().Text(); !strings.Contains(text, "said nothing") {
		t.Errorf("an empty console reads %q", text)
	}
	s.clean()
}

func TestTheConsolesLineEvaluatesInThePage(t *testing.T) {
	s := show(t)
	s.click("dt-tab-console")
	s.click("dt-eval")
	for _, typed := range []string{"1 + 1", "boom"} {
		s.enter(typed)
	}
	want := []string{"input|1 + 1|", "result|value of 1 + 1|", "input|boom|", "error|ReferenceError: boom is not defined|"}
	if got := s.lines(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the console lists %q, want %q", got, want)
	}
	if strings.Join(s.asked, ",") != "1 + 1,boom" || s.doc.Value(s.node("dt-eval")) != "" {
		t.Fatalf("the page was asked %q, the line left with %q", s.asked, s.doc.Value(s.node("dt-eval")))
	}
	if !s.doc.Focused(s.node("dt-eval")) {
		t.Error("evaluating took the focus off the line")
	}
	s.clean()
}

func TestScriptsHaveAChip(t *testing.T) {
	s := show(t)
	s.load()
	s.get("/app.js", "document", netlog.Script)
	s.frame()
	s.click("dt-chip-script")
	if rows := s.column("file"); len(rows) != 1 || rows[0] != "app.js" {
		t.Fatalf("JS keeps %v", rows)
	}
}

func TestALineOfTheConsoleShowsItsLevelItsCountItsGroupAndItsStack(t *testing.T) {
	s := show(t)
	for range 3 {
		s.said.Add(console.Line{Level: console.Info, Text: "tick", Source: s.srv.URL + "/app.js", Line: 4})
	}
	s.said.Add(console.Line{Level: console.Log, Text: "in a group", Depth: 2})
	s.said.Add(console.Line{Level: console.Error, Text: "Uncaught TypeError: x is not a function",
		Stack: "deep (app.js:2:5)\nlater (app.js:9:1)"})
	s.said.Add(console.Line{Level: console.Debug, Text: "quiet"})
	s.click("dt-tab-console")
	want := []string{"info|tick|app.js:4|3", "log|in a group|", "error|Uncaught TypeError: x is not a function|",
		"debug|quiet|"}
	if got := s.lines(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the console lists %q, want %q", got, want)
	}
	lines := s.node("dt-lines").Children()
	var marks []string
	for _, line := range lines {
		mark, _ := line.Children()[0].Attr("src")
		marks = append(marks, mark)
	}
	if got := strings.Join(marks, ","); got != "dt-info.svg,,dt-error.svg," {
		t.Errorf("the lines are marked %q", got)
	}
	plain, _ := s.doc.BoxOf(lines[0].Children()[0])
	grouped, _ := s.doc.BoxOf(lines[1].Children()[0])
	if grouped.X-plain.X != 24 {
		t.Errorf("a line two groups deep starts %v further, want 24", grouped.X-plain.X)
	}
	stack := func() (string, *ui.Node) {
		var text string
		var fold *ui.Node
		s.node("dt-lines").Children()[2].Walk(func(n *ui.Node) bool {
			if n.HasClass("dt-stack") {
				text = n.Text()
			}
			if n.HasClass("dt-fold") {
				fold = n
			}
			return true
		})
		return text, fold
	}
	text, fold := stack()
	if text != "" || fold == nil {
		t.Fatalf("before its triangle is pressed the error shows the stack %q", text)
	}
	for _, want := range []string{"deep (app.js:2:5)\nlater (app.js:9:1)", ""} {
		_, fold = stack()
		box, _ := s.doc.BoxOf(fold)
		s.in.SetMouse(box.Center())
		s.frame()
		s.in.SetMouseButton(input.MouseLeft, true)
		s.frame()
		s.in.SetMouseButton(input.MouseLeft, false)
		s.frame()
		s.frame()
		if text, _ = stack(); text != want {
			t.Errorf("after its triangle the error shows the stack %q, want %q", text, want)
		}
	}
	if foot := s.node("dt-foot").Text(); foot != "6 messages · 1 error · 0 warnings" {
		t.Errorf("the footer reads %q", foot)
	}
	s.clean()
}

func TestTheConsoleTabCountsWhatWasSaidAndItsErrors(t *testing.T) {
	s := show(t)
	s.said.Add(console.Line{Level: console.Log, Text: "ready"})
	s.said.Add(console.Line{Level: console.Log, Text: "ready"})
	s.said.Add(console.Line{Level: console.Input, Text: "1"})
	s.said.Add(console.Line{Level: console.Result, Text: "1"})
	s.frame()
	if _, shown := s.doc.BoxOf(s.node("dt-console-errors")); shown {
		t.Fatal("without an error the tab shows a count of errors")
	}
	s.said.Add(console.Line{Level: console.Error, Text: "Uncaught"})
	s.said.Add(console.Line{Level: console.Error, Text: "Uncaught"})
	s.frame()
	s.frame()
	if text := s.node("dt-console-errors").Text(); text != "2" {
		t.Fatalf("with two errors the tool bar counts %q", text)
	}
	if _, shown := s.doc.BoxOf(s.node("dt-console-errors")); !shown {
		t.Fatal("the count of errors is not shown")
	}
	s.click("dt-console-errors")
	if s.state.Tab != "console" || !s.visible("dt-lines") {
		t.Fatal("the count of errors does not open the console")
	}
	s.clean()
}

func TestTheConsoleIsFilteredByLevelAndByText(t *testing.T) {
	s := show(t)
	s.load()
	s.said.Add(console.Line{Level: console.Log, Text: "ready"})
	s.said.Add(console.Line{Level: console.Warn, Text: "slow to get ready"})
	s.said.Add(console.Line{Level: console.Error, Text: "Uncaught", Source: s.srv.URL + "/app.js", Line: 3})
	s.said.Add(console.Line{Level: console.Debug, Text: "quiet"})
	if _, shown := s.doc.BoxOf(s.node("dt-level-error")); shown {
		t.Fatal("the Network tab shows the console's levels")
	}
	s.click("dt-tab-console")
	if _, shown := s.doc.BoxOf(s.node("dt-chip-script")); shown {
		t.Fatal("the Console tab shows the kinds of requests")
	}
	s.said.Add(console.Line{Level: console.Info, Text: "told"})
	texts := func() string {
		var out []string
		for _, line := range s.lines() {
			out = append(out, strings.Split(line, "|")[1])
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct{ chip, want string }{
		{"dt-level-error", "ready,slow to get ready,quiet,told"}, {"dt-level-warn", "ready,quiet,told"},
		{"dt-level-info", "ready,quiet"}, {"dt-level-debug", "ready"}, {"dt-level-error", "ready,Uncaught"},
		{"dt-level-log", "Uncaught"},
	} {
		s.click(c.chip)
		if got := texts(); got != c.want {
			t.Errorf("after %s the console keeps %q, want %q", c.chip, got, c.want)
		}
	}
	if !s.node("dt-level-error").HasClass("chosen") || s.node("dt-level-log").HasClass("chosen") {
		t.Error("the chip of a level listed is not marked, or the one of a level left out is")
	}
	for _, chip := range []string{"dt-level-warn", "dt-level-info", "dt-level-debug", "dt-level-log"} {
		s.click(chip)
	}
	s.click("dt-console-filter")
	for _, r := range "READY" {
		s.in.AddText(r)
	}
	s.frame()
	s.frame()
	if got := texts(); got != "ready,slow to get ready" || s.state.Said != "READY" || s.state.Filter != "" {
		t.Fatalf("the filter keeps %q; the console's is %q, the requests' %q", got, s.state.Said, s.state.Filter)
	}
	if foot := s.node("dt-foot").Text(); foot != "5 messages · 1 error · 1 warning · 3 hidden by the filters" {
		t.Errorf("the footer reads %q", foot)
	}
	s.click("dt-level-log")
	s.click("dt-level-warn")
	if text := s.node("dt-lines").Parent().Text(); !strings.Contains(text, "No line of the console passes the filters.") {
		t.Errorf("with every line filtered out the console reads %q", text)
	}
	s.click("dt-tab-network")
	if got := s.doc.Value(s.node("dt-filter")); got != "" || len(s.rows()) != 5 {
		t.Fatalf("back on Network the filter holds %q and %d rows are listed", got, len(s.rows()))
	}
	s.click("dt-tab-console")
	if got := s.doc.Value(s.node("dt-console-filter")); got != "READY" {
		t.Fatalf("back on Console the filter holds %q", got)
	}
	s.clean()
}

func TestKeysWalkWhatWasTypedInTheConsole(t *testing.T) {
	s := show(t)
	s.click("dt-tab-console")
	s.click("dt-eval")
	s.enter("first")
	s.enter("second")
	s.enter("second")
	for _, r := range "draft" {
		s.in.AddText(r)
	}
	s.frame()
	line := s.node("dt-eval")
	previous, next := input.KeyLeftAlt, input.KeyLeftAlt
	up, down := input.KeyUp, input.KeyDown
	if runtime.GOOS == "darwin" {
		previous, next, up, down = input.KeyLeftControl, input.KeyLeftControl, input.KeyP, input.KeyN
	}
	for _, c := range []struct {
		with, key input.Key
		want      string
	}{
		{input.KeyLeftAlt, input.KeyUp, "second"}, {input.KeyLeftAlt, input.KeyUp, "first"},
		{previous, up, "first"},
		{input.KeyLeftAlt, input.KeyDown, "second"}, {next, down, "draft"},
		{input.KeyLeftAlt, input.KeyDown, "draft"}, {previous, up, "second"},
	} {
		s.in.SetKey(c.with, true)
		s.frame()
		s.press(c.key)
		s.in.SetKey(c.with, false)
		s.frame()
		if got := s.doc.Value(line); got != c.want {
			t.Fatalf("after keys %v and %v the line holds %q, want %q", c.with, c.key, got, c.want)
		}
	}
	s.enter(";")
	if got := strings.Join(s.asked, ","); got != "first,second,second,second;" {
		t.Fatalf("the page was asked %q", got)
	}
	if got := strings.Join(s.state.History, ","); got != "first,second,second;" {
		t.Errorf("the tab remembers %q", got)
	}
	s.clean()
}

func TestTheConsoleFollowsItsLastLineUnlessScrolledUp(t *testing.T) {
	s := show(t)
	s.click("dt-tab-console")
	say := func(from, to int) {
		for i := from; i < to; i++ {
			s.said.Add(console.Line{Level: console.Log, Text: "line " + strconv.Itoa(i)})
		}
		s.frame()
		s.frame()
		s.frame()
	}
	seen := func(n int) bool {
		view, _ := s.doc.BoxOf(s.node("dt-console"))
		row, ok := s.doc.BoxOf(s.node("dt-lines").Children()[n])
		return ok && row.Y >= view.Y-1 && row.Y+row.H <= view.Y+view.H+1
	}
	say(0, 40)
	if !seen(39) || seen(0) {
		t.Fatal("the console does not show its last line")
	}
	say(40, 41)
	if !seen(40) {
		t.Fatal("the console did not follow a line said later")
	}
	view, _ := s.doc.BoxOf(s.node("dt-console"))
	s.in.SetMouse(view.Center())
	s.frame()
	s.in.AddWheel(geom.Vec2{Y: -100})
	for range glide {
		s.frame()
	}
	if !seen(0) {
		t.Fatal("the wheel did not scroll the console up")
	}
	say(41, 43)
	if !seen(0) || seen(42) {
		t.Fatal("a line said while the console is scrolled up took it back down")
	}
	s.click("dt-eval")
	s.enter("1 + 1")
	s.frame()
	if last := len(s.node("dt-lines").Children()) - 1; !seen(last) {
		t.Fatal("what the line gave is not shown")
	}
	s.clean()
}

func TestTheConsoleSaysWhyNoScriptRuns(t *testing.T) {
	s := show(t)
	s.unscripted = "This build of light has no JavaScript engine."
	s.click("dt-tab-console")
	if text := s.node("dt-unscripted").Text(); text != s.unscripted {
		t.Fatalf("the console says %q", text)
	}
	if _, disabled := s.node("dt-eval").Attr("disabled"); !disabled {
		t.Error("the line still takes what is typed")
	}
	s.clean()
	s.unscripted = ""
	s.click("dt-tab-console")
	if s.doc.ByID("dt-unscripted") != nil {
		t.Fatal("the console still says no script runs")
	}
	if _, disabled := s.node("dt-eval").Attr("disabled"); disabled {
		t.Error("the line takes nothing")
	}
	s.click("dt-eval")
	s.enter("1")
	if len(s.asked) != 1 {
		t.Fatalf("the page was asked %q", s.asked)
	}
}

func TestTheConsolesTextIsSelected(t *testing.T) {
	s := show(t)
	s.said.Add(console.Line{Level: console.Warn, Text: "copy me", Source: s.srv.URL + "/app.js", Line: 2})
	s.click("dt-tab-console")
	if !s.doc.SelectContents(s.node("dt-lines")) {
		t.Fatal("the console's lines take no selection")
	}
	s.frame()
	if got := s.doc.Selection().Text; !strings.Contains(got, "copy me") || !strings.Contains(got, "app.js:2") {
		t.Fatalf("the selection reads %q", got)
	}
}

func TestTheConsolesLineIsEvaluatedInTheDocumentChosen(t *testing.T) {
	s := show(t)
	s.click("dt-tab-console")
	if _, shown := s.doc.BoxOf(s.node("dt-context")); shown {
		t.Fatal("a page without frames has documents to choose")
	}
	s.contexts = []devtools.Context{{Name: "top", Origin: "https://site.test"},
		{ID: 4, Name: "other.test", Origin: "https://other.test"}, {ID: 7, Name: "srcdoc", Origin: "null"}}
	s.frame()
	s.frame()
	if got := s.node("dt-context").Text(); got != "top · https://site.test" {
		t.Fatalf("the line is evaluated in %q", got)
	}
	s.click("dt-eval")
	s.enter("a")
	s.click("dt-context")
	if got := s.node("dt-context").Text(); got != "other.test · https://other.test" {
		t.Fatalf("after a press the line is evaluated in %q", got)
	}
	s.click("dt-eval")
	s.enter("b")
	s.click("dt-context")
	s.click("dt-eval")
	s.enter("c")
	if got := s.node("dt-context").Text(); got != "srcdoc · null" || s.state.Context != 7 {
		t.Fatalf("after two presses the line is evaluated in %q", got)
	}
	s.contexts = s.contexts[:2]
	s.frame()
	s.frame()
	if got := s.node("dt-context").Text(); got != "top · https://site.test" || s.state.Context != 0 {
		t.Fatalf("once its frame went away the line is evaluated in %q", got)
	}
	s.enter("d")
	s.click("dt-context")
	s.click("dt-context")
	if s.state.Context != 0 {
		t.Fatalf("past the last document the line is evaluated in %d", s.state.Context)
	}
	if strings.Join(s.asked, ",") != "a,d" || !slices.Equal(s.askedIn, []int{4, 7}) {
		t.Fatalf("the page was asked %q and its frames %v", s.asked, s.askedIn)
	}
	want := []string{"input|a|", "result|value of a|", "input|b|", "result|b in 4|", "input|c|", "result|c in 7|",
		"input|d|", "result|value of d|"}
	if got := s.lines(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the console lists %q, want %q", got, want)
	}
	var tagged []string
	s.node("dt-lines").Walk(func(n *ui.Node) bool {
		if n.HasClass("dt-frame") {
			tagged = append(tagged, n.Text())
		}
		return true
	})
	if strings.Join(tagged, ",") != "other.test,other.test,srcdoc,srcdoc" {
		t.Fatalf("the lines of frames are tagged %q", tagged)
	}
	s.contexts = nil
	s.frame()
	s.frame()
	if _, shown := s.doc.BoxOf(s.node("dt-context")); shown {
		t.Fatal("a page whose frames went away has documents to choose")
	}
	s.clean()
}
