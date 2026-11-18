package web //nolint:testpackage // the adaptations and the caps tested are unexported

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

func picture(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
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

// answers serves each path with its headers and body, and notes the
// paths asked for.
type answer struct {
	status int
	header map[string]string
	body   string
}

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

func typed(media, body string) answer {
	return answer{header: map[string]string{"Content-Type": media}, body: body}
}

func moved(status int, to string) answer {
	return answer{status: status, header: map[string]string{"Location": to}}
}

func TestARedirectedPageIsBuiltFromItsFinalAddress(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/old": moved(http.StatusMovedPermanently, "/crypt/"),
		"/crypt/": typed("text/html",
			`<title>The crypt</title><link rel="stylesheet" href="/css/old.css"><a id="on" href="vaults">on</a>`),
		"/css/old.css":         moved(http.StatusFound, "/skins/dark/site.css"),
		"/skins/dark/site.css": typed("text/css", `a { background: url(wall.png); display: block; height: 20px; }`),
	})
	s := show(t)
	doc := s.visit(srv.URL + "/old")
	if s.page.Address != srv.URL+"/crypt/" || s.page.Title != "The crypt" || s.page.Status != 200 {
		t.Fatalf("the page is %s, %q, %d", s.page.Address, s.page.Title, s.page.Status)
	}
	s.until("the sheet's picture was asked for from where the sheet was redirected to", func() bool {
		return seen.has("/skins/dark/wall.png")
	})
	host, _ := s.doc.BoxOf(s.host)
	s.in.SetMouse(box(t, doc, "on").Center().Add(geom.V2(host.X, host.Y)))
	s.frame()
	s.frame()
	if got := s.status.Text(); got != srv.URL+"/crypt/vaults" {
		t.Errorf("a link of the redirected page names %q", got)
	}
}

func TestAStatusIsShownAsABrowserShowsIt(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/own-404": {status: 404, header: map[string]string{"Content-Type": "text/html"},
			body: "<title>Lost</title><h1>Nothing here</h1>"},
		"/bare-500": {status: 500},
		"/sign-in":  {status: 401, header: map[string]string{"WWW-Authenticate": "Basic"}, body: "<p>who?</p>"},
		"/proxy":    {status: 407},
		"/busy": {status: 503, header: map[string]string{"Retry-After": "120", "Content-Type": "text/html"},
			body: "<p>Back soon</p>"},
		"/partial":    {status: 206, header: map[string]string{"Content-Type": "text/html"}, body: "<p>part</p>"},
		"/no-content": {status: 204},
		"/reset":      {status: 205},
		"/empty":      typed("text/html", ""),
	})
	for path, want := range map[string]struct {
		status int
		text   string
	}{"/own-404": {404, "Nothing here"}, "/busy": {503, "Back soon"}, "/partial": {206, "part"}} {
		page := load(t, srv.URL+path)
		if page.Err != nil || page.Doc == nil || page.Status != want.status ||
			!strings.Contains(page.Doc.Body().Text(), want.text) {
			t.Errorf("%s: status %d, err %v", path, page.Status, page.Err)
		}
	}
	for path, want := range map[string]string{
		"/bare-500": "500 Internal Server Error", "/missing": "404 Not Found",
		"/sign-in": "sign-in", "/proxy": "407 Proxy Authentication Required",
	} {
		if page := load(t, srv.URL+path); page.Err == nil || !strings.Contains(page.Err.Error(), want) {
			t.Errorf("%s: err %v, want one saying %q", path, page.Err, want)
		}
	}
	for _, path := range []string{"/no-content", "/reset", "/empty"} {
		if page := load(t, srv.URL+path); page.Err != nil || !page.Empty || page.Doc != nil {
			t.Errorf("%s: empty %v, err %v", path, page.Empty, page.Err)
		}
	}
}

func TestAnAnswerIsReadByItsKind(t *testing.T) {
	pic := picture(t, 30, 20)
	srv, _ := answers(t, map[string]answer{
		"/photo.webp":  typed("image/webp", string(testdata(t, "lossy.webp"))),
		"/shot":        typed("application/octet-stream", string(testdata(t, "lossless-alpha.webp"))),
		"/art/map.png": typed("image/png", string(pic)),
		"/favicon.ico": typed("image/x-icon", string(ico(pic))),
		"/logo.svg":    typed("image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 3"/>`),
		"/site.css":    typed("text/css", "a { color: red }"),
		"/app.js":      typed("application/javascript", "let a = 1"),
		"/data.json":   typed("application/json", `{"a": 1}`),
		"/latin.txt":   typed("text/plain; charset=iso-8859-1", "cl\xe9"),
		"/feed":        typed("application/atom+xml", "<feed/>"),
		"/blob":        typed("application/octet-stream", "\x00\x01\x02"),
		"/paper.pdf":   typed("application/pdf", "%PDF-1.7"),
		"/photo.avif":  typed("image/avif", "\x00\x00\x00\x1cftypavif"),
		"/report": {header: map[string]string{"Content-Type": "text/html",
			"Content-Disposition": `attachment; filename="../q3 report.html"`}, body: "<p>saved, not shown</p>"},
	})
	for path, want := range map[string]Page{
		"/art/map.png": {Kind: content.Picture, Media: "image/png", Title: "map.png (30×20)", Size: image.Pt(30, 20)},
		"/favicon.ico": {Kind: content.Picture, Media: "image/x-icon", Title: "favicon.ico (30×20)",
			Size: image.Pt(30, 20)},
		"/logo.svg":  {Kind: content.Picture, Media: "image/svg+xml", Title: "logo.svg"},
		"/site.css":  {Kind: content.Text, Media: "text/css", Title: "site.css", Source: "a { color: red }"},
		"/app.js":    {Kind: content.Text, Media: "application/javascript", Title: "app.js", Source: "let a = 1"},
		"/data.json": {Kind: content.Text, Media: "application/json", Title: "data.json", Source: `{"a": 1}`},
		"/latin.txt": {Kind: content.Text, Media: "text/plain", Title: "latin.txt", Source: "clé"},
		"/feed":      {Kind: content.Text, Media: "application/atom+xml", Title: "feed", Source: "<feed/>"},
		"/blob":      {Kind: content.Download, Media: "application/octet-stream", Title: "blob"},
		"/paper.pdf": {Kind: content.Download, Media: "application/pdf", Title: "paper.pdf"},
		"/photo.webp": {Kind: content.Picture, Media: "image/webp", Title: "photo.webp (30×20)",
			Size: image.Pt(30, 20)},
		"/shot":       {Kind: content.Picture, Media: "image/webp", Title: "shot (30×20)", Size: image.Pt(30, 20)},
		"/photo.avif": {Kind: content.Download, Media: "image/avif", Title: "photo.avif"},
		"/report":     {Kind: content.Download, Media: "text/html", Title: "q3 report.html"},
	} {
		page := load(t, srv.URL+path)
		if page.Err != nil {
			t.Errorf("%s: %v", path, page.Err)
			continue
		}
		if page.Kind != want.Kind || page.Media != want.Media || page.Title != want.Title || page.Size != want.Size ||
			page.Source != want.Source || page.FileName != strings.Split(want.Title, " (")[0] || page.Doc != nil {
			t.Errorf("%s is %v, %s, titled %q, %v, reading %q, saved as %q", path, page.Kind, page.Media, page.Title,
				page.Size, page.Source, page.FileName)
		}
		if held := page.Kind == content.Picture; held != (len(page.Body) > 0) {
			t.Errorf("%s holds %d bytes", path, len(page.Body))
		}
	}
	if page := load(t, srv.URL+"/favicon.ico"); !bytes.Equal(page.Body, pic) {
		t.Error("an ico is not shown by its PNG")
	}
}

func TestAPageKeepsTheTextItsServerSent(t *testing.T) {
	const sent = "<!doctype html>\n<title>T</title>\n<p>caf\xe9</p>\n"
	srv, _ := answers(t, map[string]answer{"/": typed("text/html; charset=windows-1252", sent)})
	if page := load(t, srv.URL+"/"); page.Source != "<!doctype html>\n<title>T</title>\n<p>café</p>\n" {
		t.Fatalf("the source is %q", page.Source)
	}
}

func TestWhatCannotBeLoadedSaysWhy(t *testing.T) {
	stuck := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/huge":
			_, _ = w.Write(bytes.Repeat([]byte("x"), 5000))
		case "/huge.bin":
			_, _ = w.Write(make([]byte, 5000))
		case "/stuck":
			select {
			case <-stuck:
			case <-r.Context().Done():
			}
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(stuck) })
	small := &Loader{Client: fetch.New(nil, fetch.Limit(1<<20))}
	nothing := Links{Open: func(string) {}, Over: func(string) {}}
	huge := &Loader{Client: fetch.New(nil, fetch.Limit(4000))}
	if page := huge.Load(t.Context(), srv.URL+"/huge", nothing); page.Err == nil ||
		!strings.Contains(page.Err.Error(), "more than is held in memory") {
		t.Errorf("an answer over the limit gives %v", page.Err)
	}
	if page := huge.Load(t.Context(), srv.URL+"/huge.bin", nothing); page.Err != nil || page.Kind != content.Download {
		t.Errorf("a file over the limit, which is saved and not held, gives %v, %v", page.Kind, page.Err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if page := small.Load(ctx, srv.URL+"/stuck", nothing); page.Err == nil || !strings.Contains(page.Err.Error(),
		"deadline") {
		t.Errorf("a server that never answers gives %v", page.Err)
	}
	srv.Close()
	if page := small.Load(t.Context(), srv.URL+"/", nothing); page.Err == nil || !strings.Contains(page.Err.Error(),
		"refused") {
		t.Errorf("a closed server gives %v", page.Err)
	}
}

func TestEveryRequestOfAPageIsRecordedWithWhoAsked(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/old": moved(http.StatusFound, "/"),
		"/": typed("text/html",
			`<link rel="stylesheet" href="site.css"><link rel="stylesheet" href="gone.css"><img id="map" `+
				`src="map.png">`),
		"/site.css":    typed("text/css", "img { display: block }"),
		"/map.png":     typed("image/png", string(picture(t, 30, 20))),
		"/favicon.ico": typed("image/x-icon", string(ico(picture(t, 16, 16)))),
	})
	log := netlog.New(100)
	loader := &Loader{Client: fetch.New(&netlog.Transport{})}
	s := show(t)
	s.page = loader.Load(netlog.With(t.Context(), log, "navigation", netlog.Document), srv.URL+"/old",
		Links{Open: func(string) {}, Over: func(string) {}})
	if s.page.Err != nil {
		t.Fatal(s.page.Err)
	}
	s.until("the picture is drawn", func() bool { return box(t, s.page.Doc, "map").W == 30 })
	got := map[string]string{}
	for _, r := range log.Rows() {
		got[strings.TrimPrefix(r.Address,
			srv.URL)] = r.Initiator + " " + string(r.Type) + " " + http.StatusText(r.Status)
	}
	for path, want := range map[string]string{
		"/old": "navigation document Found", "/": "navigation document OK", "/site.css": "document stylesheet OK",
		"/gone.css": "document stylesheet Not Found", "/favicon.ico": "document image OK",
		"/map.png": "picture image OK",
	} {
		if got[path] != want {
			t.Errorf("%s is recorded as %q, want %q", path, got[path], want)
		}
	}
	if len(got) != 6 {
		t.Errorf("%d rows: %v", len(got), got)
	}
	if warns := s.page.Doc.Warnings(); len(warns) > 0 {
		t.Errorf("a sheet that is not there warns: %v", warns)
	}
}

func TestASitesIconIsWhatLightCanDraw(t *testing.T) {
	small, large := picture(t, 16, 16), picture(t, 32, 32)
	bitmap := make([]byte, 40)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	for name, c := range map[string]struct {
		file, media string
		data, want  []byte
	}{
		"a png":                       {"/i.png", "image/png", small, small},
		"a webp":                      {"/i", "", testdata(t, "lossy.webp"), testdata(t, "lossy.webp")},
		"an svg by its name":          {"/i.svg?v=2", "", svg, svg},
		"an svg by its type":          {"/icon", "image/svg+xml", svg, svg},
		"an ico's largest png":        {"/favicon.ico", "image/x-icon", ico(small, large), large},
		"an ico's png past a bitmap":  {"/favicon.ico", "", ico(small, bitmap), small},
		"an ico of bitmaps":           {"/favicon.ico", "", ico(bitmap), nil},
		"a page in place of the icon": {"/favicon.ico", "text/html", []byte("<p>no</p>"), nil},
		"a cut ico":                   {"/favicon.ico", "", ico(small)[:30], nil},
	} {
		if got := favicon("https://s.test"+c.file, c.media, c.data); !bytes.Equal(got, c.want) {
			t.Errorf("%s: %d bytes, want %d", name, len(got), len(c.want))
		}
	}
}

func TestPicturesAreAskedForInTheFormatsLightDraws(t *testing.T) {
	var accepts sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepts.Store(r.URL.Path, r.Header.Get("Accept"))
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="icon" href="/icon"><img id="door" src="/door">`))
		default:
			_, _ = w.Write(testdata(t, "lossless-alpha.webp"))
		}
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.until("the WebP picture took its size", func() bool { return box(t, doc, "door").W == 30 })
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("a WebP picture warns: %v", warns)
	}
	if !bytes.Equal(s.page.Icon, testdata(t, "lossless-alpha.webp")) {
		t.Error("a WebP icon is not the site's")
	}
	for _, path := range []string{"/icon", "/door"} {
		accept, _ := accepts.Load(path)
		if got, _ := accept.(string); !strings.Contains(got, "image/webp") || strings.Contains(got, "avif") ||
			strings.Contains(got, "*") {
			t.Errorf("%s was asked for with Accept: %s", path, got)
		}
	}
}

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestALoaderIsSetUpToLeaveOutPicturesAndRefreshesAndToZoom(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Here</title><meta http-equiv="refresh" content="0; url=/next">`+
			`<img id="map" src="/map.png" width="30" height="20">`),
		"/next":    typed("text/html", `<title>Next</title>`),
		"/map.png": typed("image/png", string(picture(t, 30, 20))),
	})
	nothing := func(string) {}
	set := &Loader{Client: fetch.New(nil), NoPictures: true, NoRefresh: true, Zoom: 1.5}
	s := show(t)
	s.page = set.Load(t.Context(), srv.URL+"/", Links{Open: nothing, Over: nothing})
	if s.page.Err != nil || s.page.Title != "Here" || s.page.Doc.Zoom() != 1.5 {
		t.Fatalf("the page is %q at zoom %v: %v", s.page.Title, s.page.Doc.Zoom(), s.page.Err)
	}
	for range 20 {
		s.frame()
	}
	if seen.has("/map.png") || seen.has("/next") {
		t.Fatal("a picture or the refresh was asked for with both off")
	}
	if _, err := set.Picture(t.Context(), srv.URL+"/map.png"); err != nil {
		t.Fatalf("a picture asked for by the player is not fetched: %v", err)
	}
	if page := load(t, srv.URL+"/"); page.Title != "Next" || page.Doc.Zoom() != 1 {
		t.Fatalf("a loader not set up lands on %q at zoom %v", page.Title, page.Doc.Zoom())
	}
}
