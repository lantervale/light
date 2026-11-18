package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/frames"
)

// framing is a web of sites by their names, over http and https alike:
// each answers what its host and path are given, and what it was asked
// with is kept.
type framing struct {
	*hosts
	mu    sync.Mutex
	pages map[string]answer
	asked map[string]http.Header
	// held are the paths answered only once the test lets go of them.
	held chan struct{}
}

func framingOf(t *testing.T, pages map[string]answer) (*framing, http.RoundTripper) {
	t.Helper()
	f := &framing{pages: pages, asked: map[string]http.Header{}, held: make(chan struct{})}
	f.hosts = names(t, func(w http.ResponseWriter, r *http.Request) {
		at := r.Host + r.URL.Path
		f.mu.Lock()
		f.asked[at] = r.Header.Clone()
		a, ok := f.pages[at]
		f.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/slow") {
			select {
			case <-f.held:
			case <-r.Context().Done():
				return
			}
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for name, value := range a.header {
			for _, v := range strings.Split(value, "\n") {
				w.Header().Add(name, v)
			}
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		if a.status != 0 {
			w.WriteHeader(a.status)
		}
		_, _ = w.Write([]byte(a.body))
	})
	t.Cleanup(func() { close(f.held) })
	return f, roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" {
			return f.transport.RoundTrip(r)
		}
		plain := r.Clone(r.Context())
		plain.URL.Scheme = "http"
		resp, err := f.transport.RoundTrip(plain)
		if err == nil {
			resp.Request = r
		}
		return resp, err
	})
}

// header is what the last request for that host and path said under
// name, "(none)" for a request that never came.
func (f *framing) header(to, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if asked, came := f.asked[to]; came {
		return asked.Get(name)
	}
	return "(none)"
}

func html(body string) answer { return answer{body: body} }

func with(a answer, name, value string) answer {
	header := map[string]string{name: value}
	for n, v := range a.header {
		header[n] = v
	}
	a.header = header
	return a
}

// framesOfPage are the frames of the shown page.
func (s *shown) framesOfPage() *framed {
	s.t.Helper()
	set := s.session().frames[s.session().pages[s.browser.Tab().Current()]]
	if set == nil {
		s.t.Fatal("the shown page has no frames")
	}
	return set
}

// frameAt is the frame of the shown page's documents whose element has
// that id.
func (s *shown) frameAt(id string) *frame {
	s.t.Helper()
	for _, f := range s.framesOfPage().all {
		if f.node.ID() == id {
			return f
		}
	}
	s.t.Fatalf("no frame %q", id)
	return nil
}

// framedText waits for the frame id to show a document, and gives its
// text.
func (s *shown) framedText(id string) string {
	s.t.Helper()
	var f *frame
	s.until("the frame "+id+" showed a document", func() bool {
		for _, at := range s.session().frames[s.session().pages[s.browser.Tab().Current()]].frameList() {
			if at.node.ID() == id && at.page != nil && at.load == nil {
				f = at
				return true
			}
		}
		return false
	})
	s.frame()
	s.frame()
	return strings.Join(strings.Fields(f.page.Doc.Body().Text()), " ")
}

func (set *framed) frameList() []*frame {
	if set == nil {
		return nil
	}
	return set.all
}

// quiet waits for every frame of the shown page to have loaded.
func (s *shown) quiet() {
	s.t.Helper()
	stable := 0
	s.until("the frames loaded", func() bool {
		stable++
		for _, f := range s.session().frames[s.session().pages[s.browser.Tab().Current()]].frameList() {
			if f.load != nil {
				stable = 0
			}
		}
		return stable > 3
	})
}

// inFrame is the middle of the element id of the document the frame
// shows, in the window.
func (s *shown) inFrame(f *frame, id string) geom.Vec2 {
	s.t.Helper()
	at := box(s.t, f.page.Doc, id).Center()
	for ; f != nil; f = f.parent {
		r, ok := f.in.Doc.BoxOf(f.node)
		if !ok {
			s.t.Fatal("the frame is not laid out")
		}
		at = at.Add(geom.V2(r.X, r.Y))
	}
	host, _ := s.doc.BoxOf(s.host)
	return at.Add(geom.V2(host.X, host.Y))
}

// consoleSays reports whether the shown tab's console has a line of
// that level holding text.
func (s *shown) consoleSays(level console.Level, text string) bool {
	for _, line := range s.session().said.Lines() {
		if line.Level == level && strings.Contains(line.Text, text) {
			return true
		}
	}
	return false
}

func TestAPageShowsTheDocumentsOfItsFrames(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Framing</title><body style="margin:0">
			<iframe id="same" src="/inner?x=1"></iframe>
			<iframe id="cross" src="https://other.test/box/page" width="200" height="100"></iframe>
			<iframe id="inline" src="/never"
				srcdoc="&lt;p id=said&gt;written in place &lt;img src=inline.png alt=x&gt;"></iframe>
			<iframe id="blank" src="about:blank"></iframe><iframe id="none"></iframe>
			<iframe id="script" src="javascript:alert(1)"></iframe>`),
		"site.test/inner":         html(`<body><p id="in">an inner page</p><iframe id="deep" src="deeper"></iframe>`),
		"site.test/deeper":        html(`<p>a deeper page</p>`),
		"other.test/box/page":     html(`<link rel="stylesheet" href="look.css"><p>another site <img src="pic.png" alt="p">`),
		"other.test/box/look.css": typed("text/css", "p { color: red }"),
		"other.test/box/pic.png":  typed("image/png", string(picture(t, 4, 4))),
		"site.test/inline.png":    typed("image/png", string(picture(t, 4, 4))),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	doc := s.visit("https://site.test/")
	for id, want := range map[string]string{"same": "an inner page", "cross": "another site", "inline": "written in place",
		"deep": "a deeper page"} {
		if got := s.framedText(id); got != want {
			t.Errorf("the frame %s shows %q, want %q", id, got, want)
		}
	}
	s.quiet()
	for _, id := range []string{"blank", "none", "script"} {
		if doc.Frame(doc.ByID(id)) != nil {
			t.Errorf("the frame %s shows a document", id)
		}
	}
	if f.came("other.test/favicon.ico") {
		t.Error("the icon of a frame's site was asked for")
	}
	if f.came("site.test/never") {
		t.Error("the src of a frame that has a srcdoc was asked for")
	}
	if !s.consoleSays(console.Warn, frames.ErrScheme.Error()) {
		t.Errorf("the console does not say why a javascript: frame is empty: %v", s.session().said.Lines())
	}
	for _, c := range []struct{ to, name, want string }{
		{"site.test/inner", "Sec-Fetch-Dest", "iframe"}, {"site.test/inner", "Sec-Fetch-Mode", "navigate"},
		{"site.test/inner", "Sec-Fetch-Site", "same-origin"}, {"site.test/inner", "Sec-Fetch-User", ""},
		{"site.test/inner", "Referer", "https://site.test/"},
		{"other.test/box/page", "Sec-Fetch-Dest", "iframe"}, {"other.test/box/page", "Sec-Fetch-Site", "cross-site"},
		{"other.test/box/page", "Referer", "https://site.test/"}, {"other.test/box/page", "Sec-Fetch-User", ""},
		{"site.test/deeper", "Referer", "https://site.test/inner?x=1"},
		{"other.test/box/look.css", "Referer", "https://other.test/box/page"},
		{"other.test/box/look.css", "Sec-Fetch-Dest", "style"},
		{"other.test/box/look.css", "Sec-Fetch-Site", "same-origin"},
		{"other.test/box/pic.png", "Sec-Fetch-Dest", "image"},
		{"site.test/inline.png", "Referer", "https://site.test/"},
	} {
		if got := f.header(c.to, c.name); got != c.want {
			t.Errorf("the request for %s says %s: %q, want %q", c.to, c.name, got, c.want)
		}
	}
	cross := s.frameAt("cross")
	if r, _ := doc.BoxOf(cross.node); r.W != 200 || r.H != 100 {
		t.Errorf("the frame's box is %v, want its width and height attributes", r)
	}
	initiators := map[string]string{}
	for _, row := range s.session().log.Rows() {
		at, _ := url.Parse(row.Address)
		initiators[at.Host+at.Path] = row.Initiator
	}
	for to, want := range map[string]string{"site.test/": "navigation", "site.test/inner": "iframe",
		"other.test/box/page": "iframe", "other.test/box/look.css": "frame", "other.test/box/pic.png": "frame",
		"site.test/deeper": "iframe"} {
		if initiators[to] != want {
			t.Errorf("the request for %s is listed as asked by %q, want %q", to, initiators[to], want)
		}
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page warns: %v", warns)
	}
	s.toggleTools()
	s.frame()
	archive, ok := s.archive(0, false)
	if !ok || strings.Count(string(archive), `"name": "iframe"`)+strings.Count(string(archive), `"name":"iframe"`) != 3 ||
		!strings.Contains(string(archive), "https://other.test/box/look.css") {
		t.Errorf("the HAR file does not list the frames' requests as theirs: %s", archive)
	}
}

func TestALinkInAFrameOpensWhereItsTargetSays(t *testing.T) {
	inner := `<body style="margin:0"><a id="self" href="/next">here</a> <a id="top" href="/away" target="_top">top</a>
		<a id="parent" href="/up" target="_parent">parent</a> <a id="named" href="/side" target="side">side</a>
		<a id="new" href="/next" target="_blank">new</a>
		<form action="/found" target="_self"><input name="q" value="a b"><button id="find">find</button></form>
		<form action="/sent" method="post"><input name="p" value="1"><button id="send">send</button></form>`
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<body style="margin:0"><iframe id="one" src="https://other.test/links" width="500"></iframe>
			<iframe id="two" name="side" src="/empty"></iframe><iframe id="outer" src="/outer" width="500"></iframe>
			<a id="aim" href="/aimed" target="side" style="display:block">aim</a>`),
		"site.test/outer":  html(`<body style="margin:0"><iframe id="nested" src="https://other.test/links" width="480">`),
		"site.test/empty":  html(`<p>empty</p>`),
		"site.test/aimed":  html(`<p>aimed at</p>`),
		"other.test/links": html(inner),
		"other.test/next":  html(`<p>the next page</p>`),
		"other.test/up":    html(`<p>the parent's page</p>`),
		"other.test/side":  html(`<p>the side page</p>`),
		"other.test/found": html(`<p>found it</p>`),
		"other.test/sent":  html(`<p>it was sent</p>`),
		"other.test/away":  html(`<title>Away</title><p>the whole tab</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.framedText("one")
	s.framedText("nested")
	s.quiet()

	before := s.Screen.visits.Len()
	s.clickAt(s.inFrame(s.frameAt("one"), "self"))
	s.until("the frame showed the page its link leads to", func() bool {
		return s.frameAt("one").page != nil && s.frameAt("one").page.Address == "https://other.test/next"
	})
	if got := s.browser.Tab().Current(); got != "https://site.test/" || s.browser.Tab().CanForward() ||
		s.Screen.visits.Len() != before {
		t.Errorf("a link in a frame took the tab to %s, or left an entry in its history", got)
	}
	for name, want := range map[string]string{"Sec-Fetch-Dest": "iframe", "Sec-Fetch-User": "?1",
		"Sec-Fetch-Site": "same-origin", "Referer": "https://other.test/links"} {
		if got := f.header("other.test/next", name); got != want {
			t.Errorf("the frame's navigation says %s: %q, want %q", name, got, want)
		}
	}

	nested := s.frameAt("nested")
	s.clickAt(s.inFrame(nested, "named"))
	if got := s.framedTextOf("two", "https://other.test/side"); got != "the side page" {
		t.Errorf("the frame a link names shows %q", got)
	}
	s.clickAt(s.inFrame(nested, "find"))
	if got := s.framedTextOf("nested", "https://other.test/found?q=a+b"); got != "found it" {
		t.Errorf("a form sent with a GET in a frame shows %q", got)
	}
	s.frameAt("nested").stop()
	s.navigate(s.frameAt("nested"), wanted{from: s.frameAt("outer").page, ref: "https://other.test/links"})
	s.framedTextOf("nested", "https://other.test/links")
	s.clickAt(s.inFrame(s.frameAt("nested"), "send"))
	if got := s.framedTextOf("nested", "https://other.test/sent"); got != "it was sent" {
		t.Errorf("a form sent with a POST in a frame shows %q", got)
	}
	if got := f.header("other.test/sent", "Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Errorf("the form was sent as %q", got)
	}
	s.navigate(s.frameAt("nested"), wanted{from: s.frameAt("outer").page, ref: "https://other.test/links"})
	s.framedTextOf("nested", "https://other.test/links")
	s.clickAt(s.inFrame(s.frameAt("nested"), "parent"))
	if got := s.framedTextOf("outer", "https://other.test/up"); got != "the parent's page" {
		t.Errorf("a link to its parent shows %q in the frame around it", got)
	}

	s.clickAt(s.inPage("aim"))
	if got := s.framedTextOf("two", "https://site.test/aimed"); got != "aimed at" {
		t.Errorf("a link of the page that names a frame shows %q in it", got)
	}
	if got := s.browser.Tab().Current(); got != "https://site.test/" {
		t.Fatalf("the tab is at %s after links that stay in frames", got)
	}

	s.navigate(s.frameAt("one"), wanted{from: s.frameAt("one").in, ref: "https://other.test/links"})
	s.framedTextOf("one", "https://other.test/links")
	s.clickAt(s.inFrame(s.frameAt("one"), "top"))
	s.until("the tab went where a link to the top leads", func() bool {
		page := s.session().pages["https://other.test/away"]
		return page != nil && s.browser.Tab().Current() == "https://other.test/away"
	})
	if got := f.header("other.test/away", "Referer"); got != "https://other.test/links" {
		t.Errorf("the tab's navigation says it comes from %q, want the frame's page", got)
	}
	if got := f.header("other.test/away", "Sec-Fetch-Dest"); got != "document" {
		t.Errorf("the tab's navigation is sent as a %q", got)
	}
}

// framedTextOf waits for the frame id to show the document at address,
// and gives its text.
func (s *shown) framedTextOf(id, address string) string {
	s.t.Helper()
	s.until("the frame "+id+" showed "+address, func() bool {
		for _, f := range s.session().frames[s.session().pages[s.browser.Tab().Current()]].frameList() {
			if f.node.ID() == id && f.page != nil && f.load == nil && f.page.Address == address {
				return true
			}
		}
		return false
	})
	return s.framedText(id)
}

func TestFramesAreASetting(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/":      html(`<iframe id="one" src="/inner"></iframe>`),
		"site.test/inner": html(`<p>an inner page</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.configure(func(c *config.Config) { c.Frames = false })
	doc := s.visit("https://site.test/")
	for range 20 {
		s.frame()
	}
	if f.came("site.test/inner") || doc.Frame(doc.ByID("one")) != nil {
		t.Fatal("a frame was loaded with the setting off")
	}
	if r, ok := doc.BoxOf(doc.ByID("one")); !ok || r.W != 300 || r.H != 150 {
		t.Errorf("the empty frame's box is %v, want an iframe's own", r)
	}
	s.configure(func(c *config.Config) { c.Frames = true })
	if got := s.framedText("one"); got != "an inner page" {
		t.Errorf("with the setting back on the frame shows %q", got)
	}
	s.settings()
	if box := s.doc.ByID("set-frames"); box == nil || !s.doc.Checked(box) {
		t.Fatal("the settings page has no checked box for frames")
	}
	s.click("set-frames")
	if s.config.Frames {
		t.Error("the box of the settings page does not turn frames off")
	}
}
