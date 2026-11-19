package devtools_test

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/netlog"
)

// clickOn clicks the first element of class under the element named id.
func (s *shown) clickOn(id, class string) {
	s.t.Helper()
	var found *ui.Node
	s.node(id).Walk(func(n *ui.Node) bool {
		if found == nil && n.HasClass(class) {
			found = n
		}
		return true
	})
	if found == nil {
		s.t.Fatalf("nothing of class %q in %q", class, id)
	}
	box, _ := s.doc.BoxOf(found)
	s.in.SetMouse(box.Center())
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

// drag presses the handle on the panel's edge, moves the pointer by and
// lets go.
func (s *shown) drag(by float32) {
	s.t.Helper()
	handle, ok := s.doc.BoxOf(s.node("dt-handle"))
	if !ok {
		s.t.Fatal("the handle is not shown")
	}
	s.in.SetMouse(handle.Center())
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouse(handle.Center().Add(geom.V2(40, by/2)))
	s.frame()
	s.in.SetMouse(handle.Center().Add(geom.V2(40, by)))
	s.frame()
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
}

func (s *shown) height() float32 {
	return s.panel.Node().Box().H
}

func TestTheHandleOnThePanelsEdgeSizesIt(t *testing.T) {
	s := show(t)
	if s.height() != 280 || s.panel.Height() != 0 {
		t.Fatalf("the panel starts %v high, given %d", s.height(), s.panel.Height())
	}
	s.drag(-100)
	if box := s.panel.Node().Box(); box.H != 380 || box.Y+box.H != 680 || s.panel.Height() != 380 {
		t.Fatalf("dragged 100 up the panel is at %v", box)
	}
	if len(s.resized) != 1 || s.resized[0] != 380 {
		t.Fatalf("the screen was told %v", s.resized)
	}
	s.clean()
	s.drag(-1000)
	if s.height() != 600 {
		t.Fatalf("dragged past the top the panel is %v high, want the window less 80", s.height())
	}
	s.drag(2000)
	if s.height() != 120 {
		t.Fatalf("dragged past the bottom the panel is %v high, want 120", s.height())
	}
	s.clean()
	if got := s.resized; len(got) != 3 || got[1] != 600 || got[2] != 120 {
		t.Fatalf("the screen was told %v", got)
	}
	handle, _ := s.doc.BoxOf(s.node("dt-handle"))
	s.in.SetMouse(handle.Center())
	s.frame()
	for range 2 {
		s.in.SetMouseButton(input.MouseLeft, true)
		s.frame()
		s.in.SetMouseButton(input.MouseLeft, false)
		s.frame()
	}
	s.frame()
	if s.height() != 280 || s.panel.Height() != 0 || s.resized[len(s.resized)-1] != 0 {
		t.Fatalf("after a double click on the handle the panel is %v high, the screen told %v", s.height(), s.resized)
	}
}

func TestAPressElsewhereDraggedOverTheHandleSizesNothing(t *testing.T) {
	s := show(t)
	handle, _ := s.doc.BoxOf(s.node("dt-handle"))
	s.in.SetMouse(handle.Center().Add(geom.V2(0, -60)))
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouse(handle.Center())
	s.frame()
	s.in.SetMouse(handle.Center().Add(geom.V2(0, -90)))
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	if s.height() != 280 || len(s.resized) != 0 {
		t.Fatalf("the panel is %v high, the screen told %v", s.height(), s.resized)
	}
}

func TestAHeightGivenIsKeptWithinTheWindow(t *testing.T) {
	s := show(t)
	s.panel.SetHeight(5000)
	s.frame()
	if s.height() != 600 {
		t.Fatalf("given 5000 the panel is %v high", s.height())
	}
	s.panel.SetHeight(340)
	s.frame()
	if s.height() != 340 {
		t.Fatalf("given 340 the panel is %v high", s.height())
	}
	s.panel.SetHeight(0)
	s.frame()
	if s.height() != 280 {
		t.Fatalf("given its own height back the panel is %v high", s.height())
	}
}

func TestAScriptsRequestsHaveAChipAndARefusedOneIsABlockedRow(t *testing.T) {
	s := show(t)
	s.load()
	s.get("/app.js", "app.js:12", netlog.XHR)
	s.log.Block("POST", s.srv.URL+"/collect", "app.js:14", netlog.XHR, "light sends no POST for a script")
	s.frame()
	s.click("dt-chip-xhr")
	host := strings.TrimPrefix(s.srv.URL, "http://")
	rows := s.rows()
	if len(rows) != 2 || !strings.HasPrefix(rows[0], "200|GET|"+host+"|app.js|app.js:12|js|") ||
		rows[1] != "|POST|"+host+"|collect|app.js:14||Blocked|0 B|0 ms" {
		t.Fatalf("XHR keeps %q", rows)
	}
	list := s.node("dt-rows").Children()
	if !list[2].HasClass("blocked") || !list[2].HasClass("failed") || list[1].HasClass("failed") {
		t.Error("the blocked row is not marked, or the answered one is")
	}
	sign, _ := list[2].Children()[0].Children()[0].Attr("src")
	if sign != "dt-blocked.svg" {
		t.Errorf("the blocked row's status shows %q", sign)
	}
	s.click("dt-row-7")
	if text := s.node("dt-details").Text(); !strings.Contains(text, "Status(blocked)") ||
		!strings.Contains(text, "Errorlight sends no POST for a script") || !strings.Contains(text, "Initiatorapp.js:14") {
		t.Errorf("the blocked row's details read %q", text)
	}
	s.clean()
	for _, chip := range []string{"font", "media", "other"} {
		s.click("dt-chip-" + chip)
		if rows = s.rows(); len(rows) != 0 {
			t.Errorf("%s keeps %q", chip, rows)
		}
	}
	if s.doc.ByID("dt-chip-ws") != nil {
		t.Error("there is a chip for WebSockets, which light never opens")
	}
}

func TestAColumnsNameSortsTheRows(t *testing.T) {
	s := show(t)
	s.load()
	files := func() string { return strings.Join(s.column("file"), ",") }
	asked := files()
	s.click("dt-column-file")
	host := strings.TrimPrefix(s.srv.URL, "http://")
	if got, want := files(), host+",gone.css,map.png,old,site.css"; got != want || s.state.Sort != 4 {
		t.Fatalf("by their file the rows are %s, want %s", got, want)
	}
	if len(s.node("dt-column-file").Children()) != 2 {
		t.Error("the column sorted by shows no arrow")
	}
	s.clean()
	s.click("dt-column-file")
	if got, want := files(), "site.css,old,map.png,gone.css,"+host; got != want {
		t.Fatalf("the other way the rows are %s, want %s", got, want)
	}
	s.click("dt-column-file")
	if got := files(); got != asked || s.state.Sort != 0 {
		t.Fatalf("after a third click the rows are %s, want %s", got, asked)
	}
	s.click("dt-column-status")
	if got := strings.Join(s.column("status"), ","); got != "200,200,200,302,404" {
		t.Fatalf("by their status the rows are %s", got)
	}
}

func TestAnHTTPSRowShowsALock(t *testing.T) {
	s := show(t)
	s.log.Block("POST", "https://secure.test/collect", "app.js:1", netlog.XHR, "refused")
	s.get("/", "navigation", netlog.Document)
	s.frame()
	s.frame()
	locks := 0
	for _, row := range s.node("dt-rows").Children()[1:] {
		row.Children()[2].Walk(func(n *ui.Node) bool {
			if src, _ := n.Attr("src"); src == "dt-lock.svg" {
				locks++
			}
			return true
		})
	}
	if got := s.column("domain"); locks != 1 || got[0] != "secure.test" {
		t.Fatalf("%d locks for the sites %q", locks, got)
	}
}

func TestTheConsoleListsRequestsWhenAskedTo(t *testing.T) {
	s := show(t)
	s.said.Add(console.Line{Level: console.Log, Text: "before"})
	s.load()
	s.get("/app.js", "app.js:12", netlog.XHR)
	s.log.Block("POST", s.srv.URL+"/collect", "app.js:14", netlog.XHR, "light sends no POST")
	s.said.Add(console.Line{Level: console.Log, Text: "after"})
	s.click("dt-tab-console")
	if got := s.lines(); len(got) != 2 {
		t.Fatalf("before it is asked to the console lists %q", got)
	}
	s.click("dt-show-xhr")
	got := s.lines()
	if len(got) != 4 || got[0] != "log|before|" || got[3] != "log|after|" ||
		!strings.HasPrefix(got[1], "net|XHR GET "+s.srv.URL+"/app.js [HTTP 200 OK ") ||
		got[2] != "net-failed|XHR POST "+s.srv.URL+"/collect [Blocked: light sends no POST]|" {
		t.Fatalf("with XHR the console lists %q", got)
	}
	if !s.node("dt-show-xhr").HasClass("chosen") || !s.state.XHR {
		t.Error("the XHR chip is not marked")
	}
	s.clean()
	s.get("/app.js", "app.js:20", netlog.XHR)
	s.frame()
	s.frame()
	if got = s.lines(); len(got) != 5 {
		t.Fatalf("a request made while the console is shown is not listed: %q", got)
	}
	s.click("dt-show-requests")
	if got = s.lines(); len(got) != 10 || !strings.HasPrefix(got[1], "net|GET "+s.srv.URL+"/old [HTTP 302 Found ") {
		t.Fatalf("with every request the console lists %q", got)
	}
	s.click("dt-show-requests")
	s.click("dt-show-xhr")
	if got = s.lines(); len(got) != 2 {
		t.Fatalf("with both off again the console lists %q", got)
	}
}

func TestTheConsoleListsWhatUIRefusedWhenAskedTo(t *testing.T) {
	s := show(t)
	s.warns = []error{testError("site.css:3: animation is not read")}
	s.said.Add(console.Line{Level: console.Log, Text: "ready"})
	s.click("dt-tab-console")
	s.click("dt-show-css")
	if got := s.lines(); len(got) != 2 || got[0] != "css|site.css:3: animation is not read|" {
		t.Fatalf("with CSS the console lists %q", got)
	}
	s.clean()
}

type testError string

func (e testError) Error() string { return string(e) }

func TestWhereALineIsFromLeadsToItsSourceAndAnAddressInItOpens(t *testing.T) {
	s := show(t)
	s.said.Add(console.Line{Level: console.Warn, Text: "see https://docs.test/a?b=1, then (http://x.test/y).",
		Source: s.srv.URL + "/js/app.js", Line: 12})
	s.click("dt-tab-console")
	if got := s.lines(); len(got) != 1 || got[0] != "warn|see https://docs.test/a?b=1, then (http://x.test/y).|app.js:12" {
		t.Fatalf("the console lists %q", got)
	}
	s.clickOn("dt-lines", "dt-where")
	if got := strings.Join(s.sources, ","); got != s.srv.URL+"/js/app.js#12" {
		t.Fatalf("the screen was asked for the source of %q", got)
	}
	s.clickOn("dt-lines", "dt-link")
	if got := strings.Join(s.opened, ","); got != "https://docs.test/a?b=1" {
		t.Fatalf("the screen was asked to open %q", got)
	}
	s.clean()
}

func TestThePanelIsLightOnALightScreen(t *testing.T) {
	s := show(t)
	s.load()
	s.panel.SetDark(false)
	s.frame()
	if !s.panel.Node().HasClass("light") {
		t.Fatal("the panel is not marked light")
	}
	s.clean()
	s.panel.SetDark(true)
	if s.panel.Node().HasClass("light") {
		t.Fatal("the panel stays light")
	}
}

func TestATabRegisteredIsInTheToolBarAndShowsItsTool(t *testing.T) {
	shows, updates := 0, 0
	devtools.Register(devtools.Tab{ID: "probe", Label: "Probe", Order: 10, Icon: `<path d="M3 8h10"/>`,
		Build: func(p *devtools.Panel) devtools.Tool {
			body := ui.Div(ui.ID("probe-body"))
			return devtools.Tool{Body: body, Show: func() {
				shows++
				body.SetText("probing " + p.State().Shown())
				p.SetFoot("")
			}, Update: func(shown bool) {
				if shown {
					updates++
				}
			}}
		}})
	s := show(t)
	if s.visible("probe-body") || shows != 0 {
		t.Fatal("the tab's tool shows before it is chosen")
	}
	bar := s.node("dt-tab-probe").Parent().Children()
	if bar[0].ID() != "dt-tab-probe" || bar[1].ID() != "dt-tab-inspector" || bar[2].ID() != "dt-tab-console" {
		t.Fatalf("the tool bar starts with %s, %s, %s", bar[0].ID(), bar[1].ID(), bar[2].ID())
	}
	s.click("dt-tab-probe")
	if !s.visible("probe-body") || s.node("probe-body").Text() != "probing probe" || s.visible("dt-rows") ||
		s.visible("dt-foot") || updates == 0 || !s.node("dt-tab-probe").HasClass("chosen") {
		t.Fatalf("the tab chosen shows %q, updated %d times", s.node("probe-body").Text(), updates)
	}
	s.clean()
	s.click("dt-tab-network")
	if s.visible("probe-body") || !s.visible("dt-rows") || !s.visible("dt-foot") {
		t.Fatal("the Network tab does not come back")
	}
}
