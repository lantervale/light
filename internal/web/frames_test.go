package web //nolint:testpackage // the builder's parts tested are unexported

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/fetch"
)

func TestAnIframeIsBuiltEmptyAndHandedToWhoeverShowsThePage(t *testing.T) {
	page := (&Loader{Frame: &Frame{}}).Local("https://site.test/", `<p>before</p>
		<iframe id="one" src="/inner" width="200" height="80" frameborder="0">not <b id="fallback">shown</b></iframe>
		<iframe id="two" srcdoc="<p>x</p>" sandbox name="side"></iframe>`, Links{})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	fresh := page.Frames.Take()
	if len(fresh) != 2 || fresh[0].ID() != "one" || fresh[1].ID() != "two" {
		t.Fatalf("the page's iframes are %v", fresh)
	}
	if again := page.Frames.Take(); len(again) != 0 {
		t.Errorf("the iframes were handed twice: %v", again)
	}
	one := page.Doc.ByID("one")
	if one != fresh[0] || one.Tag().String() != "iframe" || len(one.Children()) != 0 || page.Doc.ByID("fallback") != nil {
		t.Errorf("the iframe was built with what it holds for browsers that show none")
	}
	if page.Doc.Frame(one) != nil {
		t.Error("the iframe shows a document before it was given one")
	}
	for name, want := range map[string]string{"src": "/inner", "width": "200", "frameborder": "0"} {
		if got, _ := one.Attr(name); got != want {
			t.Errorf("the iframe's %s is %q, want %q", name, got, want)
		}
	}
	if _, sandboxed := fresh[1].Attr("sandbox"); !sandboxed {
		t.Error("the iframe lost its sandbox")
	}
	if page.Base == nil || page.Base.String() != "https://site.test/" {
		t.Errorf("the page's base is %v", page.Base)
	}
	if warns := page.Doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page warns: %v", warns)
	}
	var none *Frames
	if none.Take() != nil {
		t.Error("a page with no frames hands some")
	}
}

func TestAFramesDocumentIsAskedLeaveBeforeItIsRead(t *testing.T) {
	asked := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked[r.URL.Path]++
		switch r.URL.Path {
		case "/hop":
			http.Redirect(w, r, "/refusing", http.StatusFound)
		case "/refusing":
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Refresh", "0; url=/elsewhere")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="stylesheet" href="/look.css"><img src="/pic.png">`))
		case "/file":
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Disposition", "attachment")
		case "/open":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<p id="said">shown</p>`))
		}
	}))
	t.Cleanup(srv.Close)
	refuse := errors.New("refused")
	var seen []string
	loader := &Loader{Client: fetch.New(nil), Frame: &Frame{Admit: func(header http.Header, at *url.URL) error {
		seen = append(seen, at.Path)
		if header.Get("X-Frame-Options") != "" {
			return refuse
		}
		return nil
	}}}
	for _, path := range []string{"/hop", "/refusing", "/file"} {
		page := loader.Load(t.Context(), srv.URL+path, Links{})
		if !errors.Is(page.Err, refuse) || page.Doc != nil || page.FileName != "" {
			t.Errorf("%s: the page is %+v, want it refused", path, page)
		}
	}
	if asked["/look.css"]+asked["/pic.png"]+asked["/elsewhere"] != 0 {
		t.Errorf("a refused document was read, or its refresh followed: %v", asked)
	}
	page := loader.Load(t.Context(), srv.URL+"/open", Links{})
	if page.Err != nil || page.Doc == nil || page.Doc.ByID("said") == nil {
		t.Fatalf("a document that is let in was not built: %+v", page)
	}
	if got := strings.Join(seen, " "); got != "/refusing /refusing /file /open" {
		t.Errorf("leave was asked for %s", got)
	}
}

func TestASrcdocIsBuiltFromItsHoldersAddress(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path+" from "+r.Header.Get("Referer"))
		w.Header().Set("Content-Type", "text/css")
	}))
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL + "/dir/page.html")
	if err != nil {
		t.Fatal(err)
	}
	var opened []string
	loader := &Loader{Client: fetch.New(nil), Frame: &Frame{Top: base}}
	page := loader.Inline(t.Context(), base, 0, `<link rel="stylesheet" href="look.css"><a id="a" href="next">n</a>`,
		Links{Open: func(address string) { opened = append(opened, address) }, Over: func(string) {}})
	if page.Err != nil || page.Address != "about:srcdoc" || page.Base != base {
		t.Fatalf("the srcdoc's page is %+v", page)
	}
	if got := strings.Join(asked, ", "); got != "/dir/look.css from "+srv.URL+"/dir/page.html" {
		t.Errorf("the srcdoc asked for %s", got)
	}
	if href, _ := page.Doc.ByID("a").Attr("href"); href != "next" {
		t.Errorf("the link's href is %q", href)
	}
}

func TestALinkOrAFormWithATargetAsksWhereItOpens(t *testing.T) {
	var aimed, opened []string
	links := Links{
		Open: func(address string) { opened = append(opened, address) },
		Aimed: func(address, target string) bool {
			aimed = append(aimed, target+" "+address)
			return target != "nowhere"
		},
	}
	b := newBuilder(&Loader{}, mustParse(t, "https://site.test/"), links)
	b.open("https://site.test/a", "")
	b.open("https://site.test/b", "side")
	b.open("https://site.test/c", "nowhere")
	if got := strings.Join(opened, " "); got != "https://site.test/a https://site.test/c" {
		t.Errorf("opened in place: %s", got)
	}
	if got := strings.Join(aimed, ", "); got != "side https://site.test/b, nowhere https://site.test/c" {
		t.Errorf("asked where to open: %s", got)
	}

	forms := forms(t, `<form id="f" action="/find" target="side"><input name="q" value="1">
		<button id="b" formtarget="_top"></button></form><form id="p" action="/save" method="post" target="side"></form>`)
	for _, c := range []struct{ form, by, target string }{{"f", "", "side"}, {"f", "b", "_top"}, {"p", "", "side"}} {
		sent, err := forms.submission(forms.doc.ByID(c.form), forms.doc.ByID(c.by))
		if err != nil || sent.Target != c.target {
			t.Errorf("the form %s sent by %q aims at %q (%v), want %q", c.form, c.by, sent.Target, err, c.target)
		}
	}
}

func mustParse(t *testing.T, address string) *url.URL {
	t.Helper()
	at, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestAFormOfASandboxedFrameIsNotSentAndOneWithNoOriginSaysSo(t *testing.T) {
	const markup = `<form id="f" action="/save" method="post"><input name="a" value="1"></form>`
	doc, _ := built(t, markup, nil)
	var posted []Submission
	links := Links{Post: func(sent Submission) { posted = append(posted, sent) }}
	closed := &builder{doc: doc, base: mustParse(t, "https://site.test/page"), links: links,
		loader: &Loader{Frame: &Frame{NoForms: true, Opaque: true}}}
	var sandboxed *SandboxedError
	if err := closed.send(doc.ByID("f"), nil); !errors.As(err, &sandboxed) || len(posted) != 0 ||
		sandboxed.Address != "https://site.test/save" {
		t.Fatalf("a form of a frame that allows none: %v, sent %d", err, len(posted))
	}
	open := &builder{doc: doc, base: mustParse(t, "https://site.test/page"), links: links,
		loader: &Loader{Frame: &Frame{Opaque: true}}}
	if err := open.send(doc.ByID("f"), nil); err != nil || len(posted) != 1 || posted[0].Form.Origin != "null" {
		t.Fatalf("a form of a frame with no origin: %v, sent %+v", err, posted)
	}
	plain := &builder{doc: doc, base: mustParse(t, "https://site.test/page"), links: links, loader: &Loader{}}
	if err := plain.send(doc.ByID("f"), nil); err != nil || posted[1].Form.Origin != "https://site.test" {
		t.Fatalf("a form of a page: %v, sent %+v", err, posted)
	}
}
