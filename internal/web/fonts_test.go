package web //nolint:testpackage // uses the package's unexported test helpers (answers, show)

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

func TestAPagesFontsAreFetchedThroughTheLoaderAsTheirSheetAsksForThem(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/": typed("text/html", `<link rel="stylesheet" href="css/site.css">`+
			`<style>@font-face { font-family: Unused; src: url(fonts/unused.woff2); }</style>`+
			`<div><span id="text">wide wide wide</span></div><div><span id="plain">wide wide wide</span></div>`),
		"/css/site.css": typed("text/css", `
@font-face { font-family: Heavy; src: url(../fonts/heavy.eot), url(../fonts/heavy.ttf) format("truetype"); }
#text { font-family: Heavy, sans-serif; } #plain { font-family: sans-serif; }`),
		"/fonts/heavy.ttf": typed("font/ttf", string(gobold.TTF)),
	})
	log := netlog.New(100)
	loader := &Loader{Client: fetch.New(&netlog.Transport{})}
	s := show(t)
	s.page = loader.Load(netlog.With(t.Context(), log, "navigation", netlog.Document), srv.URL+"/",
		Links{Open: func(string) {}, Over: func(string) {}})
	if s.page.Err != nil {
		t.Fatal(s.page.Err)
	}
	doc := s.page.Doc
	s.until("the text is set in the page's font", func() bool {
		return doc.FontsLoading() == 0 && box(t, doc, "text").W > box(t, doc, "plain").W
	})
	var font *netlog.Row
	for _, r := range log.Rows() {
		if strings.HasSuffix(r.Address, "/fonts/heavy.ttf") {
			font = &r
		}
		if strings.Contains(r.Address, "unused") || strings.HasSuffix(r.Address, ".eot") {
			t.Errorf("%s was asked for: no text needs it, or light does not read it", r.Address)
		}
	}
	if font == nil {
		t.Fatalf("the font is not in the network log; the server saw %v", seen.paths)
	}
	if font.Initiator != "stylesheet" || font.Type != netlog.Font || font.Status != http.StatusOK {
		t.Errorf("the font is recorded as %q %q %d, want stylesheet font 200", font.Initiator, font.Type, font.Status)
	}
	if accept := font.Request.Get("Accept"); !strings.Contains(accept, "font/woff2") || !strings.Contains(accept, "*/*") {
		t.Errorf("the font was asked for with Accept: %s", accept)
	}
	if referer := font.Request.Get("Referer"); referer != srv.URL+"/css/site.css" {
		t.Errorf("the font was asked for with Referer: %q, want its sheet's address", referer)
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page warns: %v", warns)
	}
}

func TestAFontThatCannotBeFetchedIsAWarningOfThePage(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<style>@font-face { font-family: Gone; src: url(gone.woff2); }
			@font-face { font-family: Page; src: url(page.woff); }
			p { font-family: Gone, Page, sans-serif; }</style><p id="text">text</p>`),
		"/page.woff": typed("text/html", "<p>not a font</p>"),
	})
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.until("both fonts failed", func() bool { return doc.FontsLoading() == 0 && len(doc.Warnings()) == 2 })
	var said []string
	for _, w := range doc.Warnings() {
		said = append(said, w.Error())
	}
	all := strings.Join(said, "\n")
	for _, want := range []string{srv.URL + "/gone.woff2: the server answered 404 Not Found",
		srv.URL + "/page.woff: woff: not a font file"} {
		if !strings.Contains(all, want) {
			t.Errorf("the warnings lack %q:\n%s", want, all)
		}
	}
}

func TestAReferrerIsCutToItsOriginAcrossSites(t *testing.T) {
	for _, c := range []struct{ from, to, want string }{
		{"https://site.test/css/a.css?v=2#top", "https://site.test/fonts/a.woff2", "https://site.test/css/a.css?v=2"},
		{"https://fonts.test/css2?family=A", "https://static.test/a.woff2", "https://fonts.test/"},
		{"https://site.test/a.css", "http://site.test/a.woff2", ""},
		{"http://site.test/a.css", "https://cdn.test/a.woff2", "http://site.test/"},
		{"https://user:secret@site.test/a.css", "https://site.test/a.woff2", "https://site.test/a.css"},
		{"about:blank", "https://site.test/a.woff2", ""},
	} {
		from, _ := url.Parse(c.from)
		to, _ := url.Parse(c.to)
		if got := referrer(from, to); got != c.want {
			t.Errorf("from %s to %s: %q, want %q", c.from, c.to, got, c.want)
		}
	}
}
