package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/tabs"
)

// box is the border box of the screen's element id.
func (s *shown) box(id string) geom.Rect {
	s.t.Helper()
	n := s.doc.ByID(id)
	if n == nil {
		s.t.Fatalf("no element %q", id)
	}
	return n.Box()
}

func (s *shown) command(k input.Key) {
	mod := input.KeyLeftControl
	if runtime.GOOS == "darwin" {
		mod = input.KeyLeftSuper
	}
	s.chord(mod, k)
	s.frame()
}

func src(n *ui.Node) string {
	v, _ := n.Attr("src")
	return v
}

func TestTheTabStripKeepsClearOfTheTitleBar(t *testing.T) {
	s := show(t)
	if tabs, tab := s.box("tabs"), s.box("tab-0"); tabs.Y != 0 || tabs.H != headerHeight || tab.X != headerInset {
		t.Fatalf("with no title bar over it the strip is %v and its first tab at %v", tabs, tab.X)
	}
	s.win.SyncTitleBar(platform.TitleBarArea{Left: 84, Height: 40})
	s.frame()
	s.frame()
	if tabs, tab := s.box("tabs"), s.box("tab-0"); tabs.H != 40 || tab.X != 84+headerGap {
		t.Fatalf("under a 40 point title bar with 84 of buttons the strip is %v high, its first tab at %v", tabs.H,
			tab.X)
	}
	if tab := s.box("tab-0"); tab.Y+tab.H/2 != 20 {
		t.Fatalf("the tab is centred at %v, the window's buttons at 20", tab.Y+tab.H/2)
	}
	for range 20 {
		s.browser.Open(tabs.Home)
	}
	s.show()
	s.frame()
	if tab := s.box("tab-0"); tab.X != 84+headerGap {
		t.Fatalf("with 21 tabs the first is at %v, under the window's buttons", tab.X)
	}
	s.win.SyncTitleBar(platform.TitleBarArea{})
	s.frame()
	s.frame()
	if tabs := s.box("tabs"); tabs.H != headerHeight {
		t.Fatalf("fullscreen, the strip is %v high", tabs.H)
	}
}

func TestTabsShrinkAsTheyMultiply(t *testing.T) {
	s := show(t)
	if one := s.box("tab-0"); one.W != 232 || one.H != 34 {
		t.Fatalf("one tab is %v by %v, want its most, 232 by 34", one.W, one.H)
	}
	for range 9 {
		s.click("new-tab")
	}
	if len(s.browser.Tabs) != 10 || !s.doc.ByID("tab-9").HasClass("shown") || s.doc.ByID("tab-0").HasClass("shown") {
		t.Fatal("ten tabs, the newest shown alone")
	}
	first, last, add := s.box("tab-0"), s.box("tab-9"), s.box("new-tab")
	if first.W >= 232 || first.W < 64 || first.W != last.W {
		t.Fatalf("ten tabs are %v and %v wide, want one width between 64 and 232", first.W, last.W)
	}
	all := s.box("all-tabs")
	if add.X < last.X+last.W || add.X+add.W > all.X || all.X+all.W > s.win.Size().X {
		t.Fatalf("the + is at %v, want after the last tab (%v) and before the tab list's button (%v)", add, last, all)
	}
	face := s.doc.ByID("tab-0").Children()[0].Children()
	name, closer := face[len(face)-2].Box(), s.box("close-0")
	if name.X+name.W > closer.X || closer.X+closer.W > first.X+first.W {
		t.Fatalf("the tab's name runs to %v, its × stands at %v, in a tab ending at %v", name.X+name.W, closer,
			first.X+first.W)
	}
}

func TestTheTabListChoosesATab(t *testing.T) {
	s := show(t)
	s.click("go-guide")
	s.click("new-tab")
	if _, laid := s.doc.BoxOf(s.list); laid {
		t.Fatal("the tab list shows before its button is pressed")
	}
	s.click("all-tabs")
	rows := s.list.Children()
	if len(rows) != 2 || rows[0].Text() != "The ui guide" || !rows[1].HasClass("shown") {
		t.Fatalf("the list holds %d rows", len(rows))
	}
	if r := s.box("tab-list"); r.X+r.W > s.win.Size().X || r.Y < s.box("stage").Y {
		t.Fatalf("the list is at %v", r)
	}
	s.click("listed-0")
	s.at("light://guide")
	if _, laid := s.doc.BoxOf(s.list); laid || s.browser.Active != 0 {
		t.Fatal("choosing a row leaves the list open, or the tab unchosen")
	}
}

func TestAMiddleClickClosesItsTab(t *testing.T) {
	s := show(t)
	s.click("go-guide")
	s.click("new-tab")
	s.in.SetMouse(s.box("tab-0").Center())
	s.frame()
	s.in.SetMouseButton(input.MouseMiddle, true)
	s.frame()
	s.in.SetMouseButton(input.MouseMiddle, false)
	s.frame()
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("%d tabs after a middle click on the first of two", len(s.browser.Tabs))
	}
	s.at(tabs.Home)
}

func TestTheEmptyTabStripDragsTheWindow(t *testing.T) {
	s := show(t)
	b := &dragged{}
	s.win.Attach(b)
	tabs := s.box("tabs")
	s.in.SetMouse(geom.V2(tabs.X+tabs.W-40, tabs.Y+tabs.H/2))
	s.frame()
	for range 2 {
		s.in.SetMouseButton(input.MouseLeft, true)
		s.frame()
		s.in.SetMouseButton(input.MouseLeft, false)
		s.frame()
	}
	if b.drags != 1 || b.doubles != 1 {
		t.Fatalf("two presses on the empty strip: %d drags and %d double clicks, want one of each", b.drags, b.doubles)
	}
	s.click("new-tab")
	s.click("tab-0")
	if b.drags != 1 || len(s.browser.Tabs) != 2 || s.browser.Active != 0 {
		t.Fatalf("the + and a tab: %d drags, %d tabs, tab %d shown", b.drags, len(s.browser.Tabs), s.browser.Active)
	}
}

// dragged is a window backend counting what the header asks of it.
type dragged struct {
	platform.Backend
	drags, doubles int
}

func (d *dragged) StartDrag()              { d.drags++ }
func (d *dragged) DoubleClickTitleBar()    { d.doubles++ }
func (*dragged) SetCursor(platform.Cursor) {}
func (*dragged) Clipboard() string         { return "" }
func (*dragged) SetClipboard(string)       {}

func TestReloadIsStopWhileThePageLoadsAndStopCancelsIt(t *testing.T) {
	asked, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			return
		}
		asked <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	if label, _ := s.reload.Attr("aria-label"); label != "Reload" || src(s.reload.Children()[0]) != "reload.svg" {
		t.Fatalf("an idle tab's button is %q", label)
	}
	before := s.box("view")
	s.Open(srv.URL + "/")
	<-asked
	s.frame()
	if label, _ := s.reload.Attr("aria-label"); label != "Stop" || src(s.reload.Children()[0]) != "stop.svg" {
		t.Fatalf("while the page loads the button is %q", label)
	}
	if got := s.status.Text(); got != "Waiting for 127.0.0.1…" || !s.status.HasClass("shown") {
		t.Fatalf("while the page loads the bubble says %q", got)
	}
	if view := s.box("view"); view != before {
		t.Fatalf("the bubble moved the view from %v to %v", before, view)
	}
	s.click("reload")
	<-cancelled
	if label, _ := s.reload.Attr("aria-label"); label != "Reload" {
		t.Fatalf("once stopped the button is %q", label)
	}
	if text := s.doc.ByID("page").Text(); !strings.Contains(text, "stopped") {
		t.Fatalf("the stopped page reads %q", text)
	}
	if got := s.status.Text(); got != "" || s.status.HasClass("shown") {
		t.Fatalf("once stopped the bubble says %q", got)
	}
	for range 5 {
		s.frame()
	}
	if page := s.session().pages[srv.URL+"/"]; page == nil || !errors.Is(page.Err, errStopped) {
		t.Fatalf("the cancelled fetch landed over the stop: %v", page)
	}
}

func TestTheAddressShowsWithoutHTTPSUntilItIsEdited(t *testing.T) {
	s := show(t)
	if src(s.lock) != "open.svg" {
		t.Fatalf("a page of light's own shows %q beside its address", src(s.lock))
	}
	s.session().pages["https://example.test/crypt"] = &web.Page{Err: errors.New("no such host")}
	s.Open("https://example.test/crypt")
	s.frame()
	if got := s.doc.Value(s.address); got != "example.test/crypt" || src(s.lock) != "lock.svg" {
		t.Fatalf("an https address shows as %q beside %q", got, src(s.lock))
	}
	s.click("address")
	if got := s.doc.Value(s.address); got != "https://example.test/crypt" || !s.field.HasClass("editing") {
		t.Fatalf("the focused field holds %q", got)
	}
	s.click("view")
	if got := s.doc.Value(s.address); got != "example.test/crypt" || s.field.HasClass("editing") {
		t.Fatalf("once left the field shows %q", got)
	}
	s.Open("http://example.test/")
	if got := s.doc.Value(s.address); got != "http://example.test/" || src(s.lock) != "open.svg" {
		t.Fatalf("an http address shows as %q beside %q", got, src(s.lock))
	}
}

func TestTheStatusBubbleShowsOverThePageOnlyWhenItHasSomethingToSay(t *testing.T) {
	s := show(t)
	s.Open("light://guide")
	s.frame()
	view := s.box("view")
	if _, laid := s.doc.BoxOf(s.status); laid || s.status.Text() != "" {
		t.Fatal("an idle screen shows a bubble")
	}
	if size := s.win.Size(); view.Y+view.H != size.Y || view.W != size.X {
		t.Fatalf("the view is %v: the page stops short of the window's bottom edge (%v)", view, size)
	}
	var link *ui.Node
	s.doc.ByID("page").Walk(func(n *ui.Node) bool {
		if n.HasClass("link") && link == nil {
			link = n
		}
		return true
	})
	at, _ := s.doc.BoxOf(link)
	s.in.SetMouse(at.Center())
	s.frame()
	s.frame()
	bubble := s.box("status")
	if got := s.status.Text(); got != "light://examples" {
		t.Fatalf("over a link the bubble says %q", got)
	}
	if bubble.X != 0 || bubble.Y+bubble.H != s.win.Size().Y || bubble.W > 200 {
		t.Fatalf("the bubble is %v, want at the bottom-left corner and as wide as its text", bubble)
	}
	if after := s.box("view"); after != view {
		t.Fatalf("the bubble moved the view from %v to %v", view, after)
	}
	s.in.SetMouse(geom.V2(500, 300))
	s.frame()
	s.frame()
	if _, laid := s.doc.BoxOf(s.status); laid || s.status.Text() != "" {
		t.Fatalf("off the link the bubble still says %q", s.status.Text())
	}
}

func TestTheStatusBubbleTakesNoPointerAndMovesAwayFromIt(t *testing.T) {
	long := "/" + strings.Repeat("vaults/", 60)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<body style="margin: 0"><a id="low" href="%s" style="position: fixed; left: 0; `+
			`bottom: 0; width: 300px; height: 60px">down</a>`, long)
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	s.visit(srv.URL + "/")
	size := s.win.Size()
	s.in.SetMouse(geom.V2(150, size.Y-50))
	for range 4 {
		s.frame()
	}
	bubble := s.box("status")
	if got := s.status.Text(); got != srv.URL+long {
		t.Fatalf("over the link the bubble says %q", got)
	}
	if bubble.W != size.X/2 || bubble.X != 0 || s.status.HasClass("away") {
		t.Fatalf("a long address makes the bubble %v, want half the window from its left", bubble)
	}
	s.in.SetMouse(geom.V2(150, size.Y-10))
	for range 6 {
		s.frame()
	}
	bubble = s.box("status")
	if got := s.status.Text(); got != srv.URL+long {
		t.Fatalf("with the pointer where the bubble stood, the link under it lost the pointer: the bubble says %q", got)
	}
	if !s.status.HasClass("away") || bubble.X+bubble.W != size.X || bubble.Y+bubble.H != size.Y {
		t.Fatalf("the bubble is %v, want moved to the bottom-right corner", bubble)
	}
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
	if got := s.browser.Tab().Current(); got != srv.URL+long {
		t.Fatalf("a click where the bubble stood opened %q, want the link under it", got)
	}
}

func TestAWebPageReachesTheWindowsBottomEdge(t *testing.T) {
	srv := serve(t)
	s := show(t)
	s.visit(srv.URL + "/")
	host, _ := s.doc.BoxOf(s.host)
	if size := s.win.Size(); host.Y+host.H != size.Y || host.X != 0 || host.W != size.X {
		t.Fatalf("the page is drawn in %v of a %v window", host, size)
	}
}

func TestTheUsualKeysDriveLight(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			hits++
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>hello</p>"))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	s.click("go-guide")
	s.command(input.KeyLeftBracket)
	s.at(tabs.Home)
	if !s.doc.Focused(s.doc.ByID("search")) {
		t.Fatal("the start page's search field has no focus: the next keys are to work from a text field")
	}
	s.command(input.KeyRightBracket)
	s.at("light://guide")
	if runtime.GOOS != "darwin" {
		s.chord(input.KeyLeftAlt, input.KeyLeft)
		s.frame()
		s.at(tabs.Home)
		s.chord(input.KeyLeftAlt, input.KeyRight)
		s.frame()
		s.at("light://guide")
	}
	s.command(input.KeyT)
	if len(s.browser.Tabs) != 2 || s.browser.Active != 1 {
		t.Fatalf("T: %d tabs, tab %d shown", len(s.browser.Tabs), s.browser.Active)
	}
	s.command(input.KeyL)
	if !s.doc.Focused(s.address) {
		t.Fatal("L left the focus off the address field")
	}
	s.command(input.KeyW)
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("W: %d tabs", len(s.browser.Tabs))
	}
	s.at("light://guide")
	s.visit(srv.URL + "/")
	s.command(input.KeyR)
	s.visit(srv.URL + "/")
	if hits != 2 {
		t.Fatalf("R: %d fetches, want 2", hits)
	}
}

func ico(pictures ...[]byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(pictures))})
	at := 6 + 16*len(pictures)
	for i, p := range pictures {
		out.Write([]byte{byte(16 * (i + 1)), byte(16 * (i + 1)), 0, 0, 1, 0, 32, 0})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(p)), uint32(at)})
		at += len(p)
	}
	for _, p := range pictures {
		out.Write(p)
	}
	return out.Bytes()
}

func TestATabShowsItsSitesIconElseLightsLogo(t *testing.T) {
	var icon bytes.Buffer
	_ = png.Encode(&icon, image.NewNRGBA(image.Rect(0, 0, 32, 32)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/art/icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(icon.Bytes())
		case "/favicon.ico":
			_, _ = w.Write(ico(icon.Bytes()))
		case "/linked":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="mask-icon" href="/mask.svg"><link rel="shortcut icon" ` +
				`href="art/icon.png"><p>a</p>`))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<p>b</p>`))
		}
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	picture := func() *ui.Node {
		if first := s.doc.ByID("tab-0").Children()[0].Children()[0]; first.HasClass("icon") {
			return first
		}
		return nil
	}
	if src(picture()) != logo {
		t.Fatalf("a page of light's own shows %q", src(picture()))
	}
	s.session().pages["https://example.test/"] = &web.Page{Err: errors.New("gone")}
	s.Open("https://example.test/")
	if picture() != nil {
		t.Fatalf("a site without an icon shows %q before its name", src(picture()))
	}
	for _, path := range []string{"/linked", "/plain"} {
		s.visit(srv.URL + path)
		if got := src(picture()); got != "favicon:"+srv.URL+path {
			t.Fatalf("%s: the tab shows %q, want the site's icon", path, got)
		}
		s.until("the icon is drawn at the tab's size", func() bool {
			r, ok := s.doc.BoxOf(picture())
			_, known := s.doc.ImageSize(src(picture()))
			return ok && known && r.W == 16 && r.H == 16
		})
	}
	if warns := s.doc.Warnings(); len(warns) > 0 {
		t.Fatalf("the icons warn: %v", warns)
	}
}
