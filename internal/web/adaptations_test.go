package web //nolint:testpackage // the adaptations and the caps tested are unexported

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/fetch"
)

// tested is the test of each adaptation, by its name.
//
//nolint:gochecknoglobals // the table TestEveryAdaptationIsNamedSaidAndTested walks
var tested = map[string]func(t *testing.T){
	"browser sheets":      aSitesSheetIsReadAsABrowserReadsIt,
	"sheet addresses":     aSheetsPicturesComeFromItsAddress,
	"scrolling viewport":  theViewportScrolls,
	"overlay scrollbars":  theScrollbarTakesNoWidth,
	"system fonts":        theMachinesFontsAreNamed,
	"page fonts":          TestAPagesFontsAreFetchedThroughTheLoaderAsTheirSheetAsksForThem,
	"svg pictures":        svgPicturesAreDrawn,
	"svg elements":        anSvgElementIsAPicture,
	"chosen pictures":     TestAnImgShowsTheCandidateOfItsSrcsetOrOfItsPicturesSource,
	"linked sheets":       onlyScreenSheetsAreFetched,
	"elements left out":   scriptsAndTheLikeBuildNothing,
	"scripts":             scriptsAreKeptOnlyWhenTheyRun,
	"noscript":            aNoscriptIsThePagesWithoutScripts,
	"unknown elements":    anUnknownElementIsADiv,
	"links":               linksOpenFromThePagesAddress,
	"download links":      aLinkWithDownloadIsSaved,
	"forms":               aFormIsSentWithAGet,
	"attributes left out": handlersAndTheLikeAreNotCopied,
	"refresh":             anImmediateRefreshIsFollowed,
	"limits":              aLongPageIsCut,
}

func TestEveryAdaptationIsNamedSaidAndTested(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range adaptations {
		if a.name == "" || a.what == "" || seen[a.name] {
			t.Errorf("the adaptation %q, %q has no name, says nothing or is listed twice", a.name, a.what)
		}
		seen[a.name] = true
		test, ok := tested[a.name]
		if !ok {
			t.Errorf("%s has no test", a.name)
			continue
		}
		t.Run(a.name, test)
	}
	for name := range tested {
		if !seen[name] {
			t.Errorf("the test of %q is of no adaptation", name)
		}
	}
}

// built is the Document of a page whose HTML is body, served with its
// sheets by path.
func built(t *testing.T, body string, sheets map[string]string) (*ui.Document, *asked) {
	t.Helper()
	seen := &asked{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Lock()
		seen.paths = append(seen.paths, r.URL.Path)
		seen.Unlock()
		if sheet, ok := sheets[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(sheet))
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	page := load(t, srv.URL+"/")
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	return page.Doc, seen
}

// written is a sheet as sites write them, which light hands to ui
// untouched.
const written = `/* #side { width: 1px }
@media print { */
@layer reset, site;
@layer site { #side { width: 130px; padding: 10px; } }
@layer reset { div#side { width: 40px; } }
#edge { width: 130px; padding: 10px; -webkit-box-sizing: border-box; -moz-box-sizing: content-box; }
button { width: 80px; padding: 10px; }
@supports (display: grid) { #side { height: 30px; } }
@supports (display: -ms-grid) { #side { height: 99px; } }
@media screen and (min-width: 640px) and (max-width: 80em) { #edge { height: 21px; } }
@media (max-width: 720px), print, (prefers-color-scheme: dark) { #edge { height: 99px; } }
@media screen and (-webkit-min-device-pixel-ratio: 0) { #go { height: 33px; } }
#go { display: -moz-box; display: -webkit-inline-flex; -o-transition: all 1s; -khtml-opacity: 0.5; }
button::-moz-focus-inner, #go:-ms-input-placeholder { height: 98px; }
#side::-webkit-scrollbar { width: 3px; }
@-moz-document url-prefix() { #side { width: 1px; } }
`

func aSitesSheetIsReadAsABrowserReadsIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/site.css" {
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(written))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<link rel="stylesheet" href="/site.css">
			<style>@media (width >= 2000px) { #side { margin-left: 50px } }</style><body style="margin: 0">
			<div id="side" style="-webkit-box-shadow: none; -ms-filter: 'progid:X(a=1; b=2)'"></div>
			<div id="edge"></div><button id="go">Go</button>`))
	}))
	t.Cleanup(srv.Close)
	doc := show(t).visit(srv.URL)
	if !doc.BrowserSheets() {
		t.Error("the page's sheets are read as a game's")
	}
	for id, want := range map[string]geom.Vec2{
		"side": geom.V2(150, 50), // the later layer's width, the content's, and the height ui supports
		"edge": geom.V2(130, 21), // its border's, by the -webkit- name; the view is 1024 wide
		"go":   geom.V2(80, 33),  // a button's border's, as in a browser
	} {
		if got := doc.ByID(id).Box(); geom.V2(got.W, got.H) != want {
			t.Errorf("#%s is %v by %v, want %v", id, got.W, got.H, want)
		}
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("what a browser reads or drops in silence warns: %v", warns)
	}
}

func aSheetsPicturesComeFromItsAddress(t *testing.T) {
	seen := &asked{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Lock()
		seen.paths = append(seen.paths, r.URL.Path)
		seen.Unlock()
		switch r.URL.Path {
		case "/css/site.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(`#a { background: url(../art/wall.png); } #b { background: url( 'door.png' ); }`))
		case "/crypt/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="stylesheet" href="/css/site.css">
				<style>div { height: 20px; } #c { background: url(art/own.png); }</style>
				<div id="a"></div><div id="b"></div><div id="c"></div>
				<div id="d" style="background: url(inline.png)"></div>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	s.visit(srv.URL + "/crypt/")
	for _, path := range []string{"/art/wall.png", "/css/door.png", "/crypt/art/own.png", "/crypt/inline.png"} {
		s.until(path+" was asked for", func() bool { return seen.has(path) })
	}
}

func svgPicturesAreDrawn(t *testing.T) {
	const (
		star = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="18" viewBox="0 0 4 3"><path d="M0 ` +
			`0h4v3z"/></svg>`
		wide = `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 1"><rect width="4" ` +
			`height="1"/></svg>`
		label = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="12"><text y="10">Abc</text></svg>`
	)
	seen := &asked{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Lock()
		seen.paths = append(seen.paths, r.URL.Path)
		seen.Unlock()
		files := map[string]string{"/art/star.svg": star, "/icon": star, "/wide": wide, "/label.svg": label}
		svg := files[r.URL.Path]
		switch {
		case svg != "":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svg))
		case r.URL.Path == "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<style>body { margin: 0; width: 200px; }
				#panel { height: 40px; background: url(art/star.svg) no-repeat left bottom; background-size: 101%; }</style>
				<div id="panel"></div><div><img id="icon" src="/icon"></div><div><img id="wide" src="/wide"></div>
				<img id="label" src="label.svg">`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	s.until("the icon, an SVG by its bytes alone, has the size its file asks for", func() bool {
		r := box(t, doc, "icon")
		return r.W == 24 && r.H == 18
	})
	s.until("a view box alone is as wide as its block", func() bool {
		r := box(t, doc, "wide")
		return r.W == 200 && r.H == 50
	})
	s.until("the background was asked for", func() bool { return seen.has("/art/star.svg") })
	s.until("the text of label.svg is reported", func() bool {
		return slices.ContainsFunc(doc.Warnings(), func(err error) bool {
			return strings.Contains(err.Error(), "label.svg: image/svg+xml is not drawn: text")
		})
	})
	if r := box(t, doc, "label"); r.W != 40 || r.H != 12 {
		t.Errorf("the label is %v by %v", r.W, r.H)
	}
	for range 5 {
		s.frame()
	}
	for _, err := range doc.Warnings() {
		if strings.Contains(err.Error(), "image ") && !strings.Contains(err.Error(), "label.svg") {
			t.Errorf("a picture warns: %v", err)
		}
	}
}

func theViewportScrolls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<style>body { margin: 0; background: #fff url(/tree.png) no-repeat bottom right; }
			#bar { position: fixed; top: 0; left: 0; width: 50px; height: 20px; }</style>
			<div id="bar"></div><div id="tall" style="height: 4000px"></div><p id="last"
				style="margin: 0">The end</p>`))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	host, _ := s.doc.BoxOf(s.host)
	if !doc.ScrollingViewport() {
		t.Fatal("the page's viewport doesn't scroll")
	}
	page := doc.HTML().Box()
	if last := box(t, doc, "last"); page.Y != 0 || page.H != last.Y+last.H || page.H <= 4000 {
		t.Fatalf("html is %v, as high as the view and not the page: its bottom right is where "+
			"body's picture goes, and #last ends at %v", page, last.Y+last.H)
	}
	bar := box(t, doc, "bar")
	s.in.SetMouse(geom.V2(host.X+host.W/2, host.Y+host.H/2))
	s.frame()
	s.in.AddWheel(geom.V2(0, 2))
	s.frame()
	s.frame()
	first := -doc.HTML().Box().Y
	if first <= 0 {
		t.Fatal("the wheel over the page doesn't scroll it")
	}
	if box(t, doc, "bar") != bar {
		t.Errorf("the fixed bar moved from %v to %v with the page", bar, box(t, doc, "bar"))
	}
	for range 400 {
		s.in.AddWheel(geom.V2(0, 10))
		s.frame()
	}
	s.frame()
	if end := doc.HTML().Box(); end.Y+end.H != host.H {
		t.Errorf("at its end the page's box is %v in a view %v high: its bottom, the picture's, isn't the view's",
			end, host.H)
	}
}

func theScrollbarTakesNoWidth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<body style="margin: 0"><div id="tall" style="height: 4000px"></div>`))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	host, _ := s.doc.BoxOf(s.host)
	if tall := box(t, doc, "tall"); !doc.OverlayScrollbars() || tall.W != host.W {
		t.Errorf("the page's content is %v wide in a view of %v", tall.W, host.W)
	}
}

func theMachinesFontsAreNamed(t *testing.T) {
	doc, _ := built(t, `<p>Georgia or not</p>`, nil)
	if !doc.SystemFonts() {
		t.Error("the page's font-family names only the engine's faces")
	}
}

func onlyScreenSheetsAreFetched(t *testing.T) {
	var page strings.Builder
	page.WriteString(`<style>p { left: 0 }</style><style>p { right: 0 }</style><style>p { bottom: 0 }</style>
		<link rel="StyleSheet" href="/screen.css"><link rel="stylesheet" media="print" href="/print.css">
		<link rel="alternate" href="/feed.css"><link rel="stylesheet" href="/gone.css"><p id="p">text</p>`)
	sheets := map[string]string{"/screen.css": "p { top: 0 }", "/print.css": "p { top: 0 }",
		"/feed.css": "p { top: 0 }"}
	more := func(i int) string { return "/more-" + strconv.Itoa(i) + ".css" }
	for i := range maxSheets {
		page.WriteString(`<link rel="stylesheet" href="` + more(i) + `">`)
		sheets[more(i)] = "p { top: 0 }"
	}
	doc, seen := built(t, page.String(), sheets)
	// A page of a site builder links some fifty sheets.
	for path, want := range map[string]bool{
		"/screen.css": true, "/print.css": false, "/feed.css": false, "/gone.css": true,
		more(0): true, more(47): true, more(maxSheets - 3): true, more(maxSheets - 2): false, more(maxSheets - 1): false,
	} {
		if seen.has(path) != want {
			t.Errorf("%s fetched: %v, want %v", path, seen.has(path), want)
		}
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("a sheet that can't be fetched warns: %v", warns)
	}
}

func aNoscriptIsThePagesWithoutScripts(t *testing.T) {
	const page = `<head><noscript><style>.cover { width: 50px; }</style></noscript></head>
		<p class="cover" id="cover">cover</p><noscript><p id="without">without</p></noscript>`
	doc, _ := built(t, page, nil)
	doc.Layout(geom.V2(400, 300))
	if doc.ByID("without") == nil || box(t, doc, "cover").W != 50 {
		t.Errorf("without scripts a noscript's elements and sheets are not the page's")
	}
	with, _ := scripted(t, map[string]answer{"/": markup(page)}, "/")
	with.Doc.Layout(geom.V2(400, 300))
	if with.Doc.ByID("without") != nil || strings.TrimSpace(with.Doc.Body().Text()) != "cover" ||
		box(t, with.Doc, "cover").W == 50 {
		t.Errorf("with scripts a noscript is shown: %q", with.Doc.Body().Text())
	}
}

func anSvgElementIsAPicture(t *testing.T) {
	doc, _ := built(t, `<style>body { margin: 0; color: #c00; } .bar svg { height: 30px; }
		svg[icon=search] { width: 20px; }
		</style><div class="bar"><svg id="logo" viewBox="0 0 112 32"><path id="in-svg" d="M0 0h112v32z"/><title>lost</title>
		</svg><svg id="find" icon="search" viewBox="0 0 32 32"><circle cx="16" cy="16" r="9" fill="currentColor"/></svg>
		</div><svg id="sized" width="3em" height="24"><rect width="9" height="9" xlink:href="#in-svg"/></svg>`, nil)
	doc.Layout(geom.V2(800, 600))
	for id, want := range map[string]geom.Vec2{
		"logo": geom.V2(105, 30), "find": geom.V2(20, 30), "sized": geom.V2(48, 24)} {
		if r := box(t, doc, id); r.W != want.X || r.H != want.Y {
			t.Errorf("#%s is %v by %v, want %v", id, r.W, r.H, want)
		}
	}
	if doc.ByID("in-svg") != nil || strings.Contains(doc.Body().Text(), "lost") {
		t.Error("what an svg holds was built as elements of the page")
	}
	find := doc.ByID("find")
	address, _ := ui.PictureOf(find)
	picture, _ := url.PathUnescape(address)
	for _, want := range []string{`color="rgba(204,0,0,1)"`,
		`<circle cx="16" cy="16" r="9" fill="currentColor"></circle>`} {
		if !strings.Contains(picture, want) {
			t.Errorf("the picture lacks %s: %s", want, picture)
		}
	}
	if markup := doc.ByID("sized").Markup(); !strings.Contains(markup, `xlink:href="#in-svg"`) {
		t.Errorf("the markup is %s", markup)
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("an svg element warns: %v", warns)
	}
}

func scriptsAndTheLikeBuildNothing(t *testing.T) {
	markup := map[string]string{
		"head": `<head id="gone-head"><title>lost</title></head><body><p id="kept">kept</p>`,
		"meta": `<meta id="gone-meta" name="lost">`, "link": `<link id="gone-link" rel="lost" href="/lost">`,
		"source": `<source id="gone-source" src="lost.ogg">`, "embed": `<embed id="gone-embed" src="lost.swf">`,
		"style": `<style id="gone-style">p { top: 0 }</style>`, "script": `<script id="gone-script">lost()</script>`,
	}
	page := markup["head"]
	for _, tag := range droppedNames() {
		if _, written := markup[tag]; !written {
			markup[tag] = "<" + tag + ` id="gone-` + tag + `">lost</` + tag + ">"
		}
		if tag != "head" {
			page += markup[tag]
		}
	}
	doc, _ := built(t, page+`<picture><source srcset="a.webp"><img id="shown" alt="shown"></picture>`, nil)
	if len(droppedNames()) != 13 {
		t.Errorf("%d elements are left out: %v", len(droppedNames()), droppedNames())
	}
	for _, tag := range droppedNames() {
		if doc.ByID("gone-"+tag) != nil {
			t.Errorf("a %s was built", tag)
		}
	}
	if text := doc.Body().Text(); text != "kept" || doc.ByID("shown") == nil {
		t.Errorf("the page reads %q, want only what it shows, and a picture's img", text)
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("what is left out warns: %v", warns)
	}
}

func anUnknownElementIsADiv(t *testing.T) {
	doc, _ := built(t, `<crypt-map id="map" class="wide" data-floor="2"><b>The <nobr id="nobr">lower vault</nobr></b>
		</crypt-map><section id="known"></section>`, nil)
	custom := doc.ByID("map")
	if custom == nil || custom.Tag().String() != "div" || !custom.HasClass("wide") ||
		strings.TrimSpace(custom.Text()) != "The lower vault" {
		t.Fatalf("a custom element is built as %v", custom)
	}
	if floor, _ := custom.Attr("data-floor"); floor != "2" {
		t.Errorf("it lost its attributes: data-floor is %q", floor)
	}
	if doc.ByID("nobr").Tag().String() != "div" || doc.ByID("known").Tag().String() != "section" {
		t.Errorf("nobr is a %s and section a %s", doc.ByID("nobr").Tag(), doc.ByID("known").Tag())
	}
}

func aLinkWithDownloadIsSaved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<style>a { display: block; height: 20px; }</style>
			<a id="named" href="files/q3.csv" download="figures.csv">named</a>
			<a id="bare" href="/map.png" download>bare</a><a id="place" href="#top" download>a place</a>`))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/crypt/")
	host, _ := s.doc.BoxOf(s.host)
	for id, want := range map[string]string{
		"named": srv.URL + "/crypt/files/q3.csv as figures.csv", "bare": srv.URL + "/map.png as ", "place": "",
	} {
		s.saved = ""
		s.in.SetMouse(doc.ByID(id).Box().Center().Add(geom.V2(host.X, host.Y)))
		s.frame()
		s.in.SetMouseButton(input.MouseLeft, true)
		s.frame()
		s.in.SetMouseButton(input.MouseLeft, false)
		s.frame()
		s.frame()
		if s.saved != want || s.address != srv.URL+"/crypt/" {
			t.Errorf("a click on %s saves %q and shows %s, want %q saved", id, s.saved, s.address, want)
		}
	}
}

func linksOpenFromThePagesAddress(t *testing.T) {
	hrefs := []string{"vaults?x=1", "/top", "//cdn.test/a", "http://other.test/", "#top", "mailto:a@site.test",
		"javascript:void(0)"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		page := `<style>a { display: block; height: 20px; } a:link { width: 120px; }</style><a id="none">no href</a>`
		for i, href := range hrefs {
			page += `<a id="a` + strconv.Itoa(i) + `" href="` + href + `">link</a>`
		}
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/crypt/index.html")
	host, _ := s.doc.BoxOf(s.host)
	if _, has := doc.ByID("none").Attr("href"); has || box(t, doc, "none").W == 120 {
		t.Error("an a without href is a link")
	}
	for i, want := range []string{srv.URL + "/crypt/vaults?x=1", srv.URL + "/top", "http://cdn.test/a",
		"http://other.test/", "", "", ""} {
		a := doc.ByID("a" + strconv.Itoa(i))
		if href, _ := a.Attr("href"); href != hrefs[i] || a.Box().W != 120 {
			t.Errorf("a link to %q has the href %q and is %v wide, want :link's 120", hrefs[i], href, a.Box().W)
		}
		s.in.SetMouse(a.Box().Center().Add(geom.V2(host.X, host.Y)))
		s.frame()
		s.frame()
		if got := s.status.Text(); got != want {
			t.Errorf("a link to %q names %q to open, want %q", hrefs[i], got, want)
		}
	}
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
	s.at(srv.URL + "/crypt/index.html")
	s.in.SetMouse(doc.ByID("a1").Box().Center().Add(geom.V2(host.X, host.Y)))
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
	s.at(srv.URL + "/top")
}

// searchForm is a search engine's form: hidden fields, a field, two
// buttons that submit, a box, a choice and what a form never sends.
const searchForm = `<form action="/search" id="form"><input name="ie" value="ISO-8859-1" type="hidden">
	<input id="q" name="q"><input id="go" name="btnG" value="Search it" type="submit">
	<input id="lucky" name="btnI" value="Lucky" type="submit"><input name="off" value="1" disabled>
	<input type="checkbox" name="safe" checked><input type="checkbox" name="images" value="1">
	<select name="lang"><option value="fr">French<option value="en" selected>English</select>
	<textarea name="note">a&amp;b</textarea><input type="button" name="plain" value="x"><input value="unnamed">
	<button id="more" name="more" value="yes">More</button></form>
	<form action="/login" method="post"><input name="user" value="ann"><button id="post">Log in</button></form>
	<form action="other?kept=no#part" id="relative"><input name="a" value="1"><button id="there">Go</button></form>`

func aFormIsSentWithAGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=ISO-8859-1")
		_, _ = w.Write([]byte(searchForm))
	}))
	t.Cleanup(srv.Close)
	const sent = "ie=ISO-8859-1&q=caf%E9+%80+%26%2326085%3B&"
	const rest = "safe=on&lang=en&note=a%26b"
	for _, c := range []struct {
		name string
		do   func(s *shown, doc *ui.Document)
		want string
	}{
		{"a click on a submit button", func(s *shown, doc *ui.Document) { s.click(doc.ByID("go")) },
			"/search?" + sent + "btnG=Search+it&" + rest},
		{"a click on another", func(s *shown, doc *ui.Document) { s.click(doc.ByID("lucky")) },
			"/search?" + sent + "btnI=Lucky&" + rest},
		{"a click on a button element", func(s *shown, doc *ui.Document) { s.click(doc.ByID("more")) },
			"/search?" + sent + rest + "&more=yes"},
		{"Enter in a field", func(s *shown, _ *ui.Document) {
			s.in.SetKey(input.KeyEnter, true)
			s.frame()
			s.in.SetKey(input.KeyEnter, false)
			s.frame()
			s.frame()
		}, "/search?" + sent + "btnG=Search+it&" + rest},
		{"a form with another method", func(s *shown, doc *ui.Document) { s.click(doc.ByID("post")) }, "/dir/page"},
		{"an action from the page's address", func(s *shown, doc *ui.Document) { s.click(doc.ByID("there")) },
			"/dir/other?a=1#part"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := show(t)
			s.t = t
			doc := s.visit(srv.URL + "/dir/page")
			s.click(doc.ByID("q"))
			for _, r := range "café € 日" {
				s.in.AddText(r)
				s.frame()
			}
			c.do(s, doc)
			s.at(srv.URL + c.want)
		})
	}
}

func handlersAndTheLikeAreNotCopied(t *testing.T) {
	doc, _ := built(t, `<body onload="go()"><div id="d" class="a b" title="kept" data-x="1" onclick="go()" tabindex="3"
		href="/nowhere" style="top: 1px"></div><input id="i" autofocus name="q"><img id="m" srcset="a.png 2x" alt="map">
		<a id="a" href="/vaults" onmouseover="go()">vaults</a>`, nil)
	for id, want := range map[string]map[string]bool{
		"d": {"title": true, "data-x": true, "onclick": false, "tabindex": false, "href": false},
		"i": {"name": true, "autofocus": false},
		"m": {"alt": true, "srcset": false},
		"a": {"href": true, "onmouseover": false},
	} {
		for name, kept := range want {
			if _, has := doc.ByID(id).Attr(name); has != kept {
				t.Errorf("#%s has %s: %v, want %v", id, name, has, kept)
			}
		}
	}
	if _, has := doc.Body().Attr("onload"); has || !doc.ByID("d").HasClass("b") {
		t.Error("body kept its onload, or the div lost its classes")
	}
}

func aLongPageIsCut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.Repeat("x", maxBytes+100)))
			return
		case "/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, maxPicture+1))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("<p>line</p>", maxElements+50)))
	}))
	t.Cleanup(srv.Close)
	if n := len(load(t, srv.URL).Doc.Body().Children()); n != maxElements {
		t.Errorf("%d elements built, want %d", n, maxElements)
	}
	if n := len(load(t, srv.URL+"/big").Doc.Body().Text()); n != maxBytes {
		t.Errorf("%d bytes read, want %d", n, maxBytes)
	}
	if _, err := testLoader().Picture(t.Context(), srv.URL+"/big.png"); err == nil || !strings.Contains(err.Error(),
		"8 MB") {
		t.Errorf("a picture over the cap gives %v", err)
	}
}

func TestAWebkitTransformTurnsItsElementAndNoPrefixWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/site.css" {
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(`#turned { position: absolute; left: 200px; top: 200px; width: 200px; height: 20px;
				-webkit-transform: rotate(-90deg); -moz-transform: rotate(-90deg); -ms-transform: rotate(-90deg);
				-o-transform: rotate(-90deg); -webkit-box-sizing: border-box; -moz-box-sizing: border-box; }`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<link rel="stylesheet" href="/site.css"><body style="margin: 0">
			<a id="turned" href="/vaults" style="-webkit-transition: all 0.4s ease; -o-transition: all 0.4s ease"
				>v</a>`))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	doc := s.visit(srv.URL + "/")
	if got := box(t, doc, "turned"); got.W != 200 || got.H != 20 {
		t.Fatalf("the link is laid out %v by %v, want 200 by 20 before it turns", got.W, got.H)
	}
	host, _ := s.doc.BoxOf(s.host)
	centre := geom.V2(host.X+300, host.Y+210)
	for at, want := range map[geom.Vec2]string{
		centre.Add(geom.V2(-80, 0)): "",                  // in the box as laid out, left of it once turned
		centre.Add(geom.V2(0, -80)): srv.URL + "/vaults", // above the box as laid out, in it once turned
	} {
		s.in.SetMouse(at)
		s.frame()
		s.frame()
		if got := s.status.Text(); got != want {
			t.Errorf("at %v from the link's centre the status line names %q, want %q: the link stands a quarter turn "+
				"from its box", at.Sub(centre), got, want)
		}
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page's prefixed declarations warn: %v", warns)
	}
}

func anImmediateRefreshIsFollowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/meta":
			_, _ = w.Write([]byte(`<head><meta http-equiv="Refresh" content="0; url=pages/next"></head><p>moving</p>`))
		case "/header":
			w.Header().Set("Refresh", "0; url=/pages/next")
			_, _ = w.Write([]byte(`<p>moving</p>`))
		case "/later":
			_, _ = w.Write([]byte(`<meta http-equiv="refresh" content="5; url=/pages/next"><title>Wait</title><p>in ` +
				`a while</p>`))
		case "/round":
			_, _ = w.Write([]byte(`<meta http-equiv="refresh" content="0; url=/round">`))
		case "/away":
			_, _ = w.Write([]byte(`<meta http-equiv="refresh" content="0; url=ftp://files.test/">`))
		case "/pages/next":
			_, _ = w.Write([]byte(`<title>Next</title><a id="on" href="more">on</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	for _, path := range []string{"/meta", "/header"} {
		page := load(t, srv.URL+path)
		if page.Err != nil || page.Address != srv.URL+"/pages/next" || page.Title != "Next" {
			t.Errorf("%s landed at %s, titled %q: %v", path, page.Address, page.Title, page.Err)
		}
	}
	if page := load(t, srv.URL+"/later"); page.Address != srv.URL+"/later" || page.Title != "Wait" {
		t.Errorf("a refresh that waits moved the page to %s", page.Address)
	}
	if page := load(t, srv.URL+"/round"); !errors.Is(page.Err, fetch.ErrTooManyRedirects) {
		t.Errorf("a page refreshing to itself gives %v", page.Err)
	}
	if page := load(t, srv.URL+"/away"); page.Err == nil || !strings.Contains(page.Err.Error(), "ftp://files.test/") {
		t.Errorf("a refresh to another scheme gives %v", page.Err)
	}
}
