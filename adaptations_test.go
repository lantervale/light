package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"
)

// tested is the test of each adaptation, by its name.
var tested = map[string]func(t *testing.T){
	"browser sheets":      aSitesSheetIsReadAsABrowserReadsIt,
	"sheet addresses":     aSheetsPicturesComeFromItsAddress,
	"scrolling viewport":  theViewportScrolls,
	"overlay scrollbars":  theScrollbarTakesNoWidth,
	"system fonts":        theMachinesFontsAreNamed,
	"svg pictures":        svgPicturesAreDrawn,
	"linked sheets":       onlyScreenSheetsAreFetched,
	"elements left out":   scriptsAndTheLikeBuildNothing,
	"unknown elements":    anUnknownElementIsADiv,
	"links":               linksOpenFromThePagesAddress,
	"attributes left out": handlersAndTheLikeAreNotCopied,
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
	page := fetch(t.Context(), srv.URL+"/", linker{})
	if page.err != nil {
		t.Fatal(page.err)
	}
	return page.doc, seen
}

// written is a sheet as sites write them, which the explorer hands to ui
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
	if !doc.BrowserSheets() || ui.NewDocument(options(true)...).BrowserSheets() {
		t.Error("the page's sheets are read as a game's, or a plain-text page's as a browser's")
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
		star  = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="18" viewBox="0 0 4 3"><path d="M0 0h4v3z"/></svg>`
		wide  = `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 1"><rect width="4" height="1"/></svg>`
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notes.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("line\n", 400)))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<style>body { margin: 0; background: #fff url(/tree.png) no-repeat bottom right; }
			#bar { position: fixed; top: 0; left: 0; width: 50px; height: 20px; }</style>
			<div id="bar"></div><div id="tall" style="height: 4000px"></div><p id="last" style="margin: 0">The end</p>`))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	if text := s.visit(srv.URL + "/notes.txt"); !text.ScrollingViewport() {
		t.Error("a plain-text page's viewport doesn't scroll")
	}
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notes.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("line\n", 400)))
			return
		}
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
	text := s.visit(srv.URL + "/notes.txt")
	if pre := text.Body().Children()[0].Box(); pre.W != host.W-2*8 {
		t.Errorf("plain text is %v wide in a view of %v, less body's margins", pre.W, host.W)
	}
}

func theMachinesFontsAreNamed(t *testing.T) {
	doc, _ := built(t, `<p>Georgia or not</p>`, nil)
	if !doc.SystemFonts() {
		t.Error("the page's font-family names only the engine's faces")
	}
	if ui.NewDocument(options(true)...).SystemFonts() {
		t.Error("a plain-text page scans the machine's fonts for the one face it shows")
	}
}

func onlyScreenSheetsAreFetched(t *testing.T) {
	var page strings.Builder
	page.WriteString(`<link rel="StyleSheet" href="/screen.css"><link rel="stylesheet" media="print" href="/print.css">
		<link rel="alternate" href="/feed.css"><link rel="stylesheet" href="/gone.css"><p id="p">text</p>`)
	sheets := map[string]string{"/screen.css": "p { top: 0 }", "/print.css": "p { top: 0 }", "/feed.css": "p { top: 0 }"}
	for i := range maxSheets {
		path := "/more-" + string(rune('a'+i)) + ".css"
		page.WriteString(`<link rel="stylesheet" href="` + path + `">`)
		sheets[path] = "p { top: 0 }"
	}
	doc, seen := built(t, page.String(), sheets)
	for path, want := range map[string]bool{
		"/screen.css": true, "/print.css": false, "/feed.css": false, "/gone.css": true,
		"/more-a.css": true, "/more-n.css": true, "/more-o.css": false, "/more-p.css": false,
	} {
		if seen.has(path) != want {
			t.Errorf("%s fetched: %v, want %v", path, seen.has(path), want)
		}
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("a sheet that can't be fetched warns: %v", warns)
	}
}

func scriptsAndTheLikeBuildNothing(t *testing.T) {
	markup := map[string]string{
		"head": `<head id="gone-head"><title>lost</title></head><body><p id="kept">kept</p>`,
		"meta": `<meta id="gone-meta" name="lost">`, "link": `<link id="gone-link" rel="lost" href="/lost">`,
		"source": `<source id="gone-source" src="lost.ogg">`, "embed": `<embed id="gone-embed" src="lost.swf">`,
		"style": `<style id="gone-style">p { top: 0 }</style>`, "script": `<script id="gone-script">lost()</script>`,
		"svg": `<svg id="gone-svg"><text id="in-svg">lost</text></svg>`,
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
	if len(droppedNames()) != 16 {
		t.Errorf("%d elements are left out: %v", len(droppedNames()), droppedNames())
	}
	for _, tag := range droppedNames() {
		if doc.ByID("gone-"+tag) != nil {
			t.Errorf("a %s was built", tag)
		}
	}
	if text := doc.Body().Text(); text != "kept" || doc.ByID("in-svg") != nil || doc.ByID("shown") == nil {
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
	if custom == nil || custom.Tag().String() != "div" || !custom.HasClass("wide") || strings.TrimSpace(custom.Text()) != "The lower vault" {
		t.Fatalf("a custom element is built as %v", custom)
	}
	if floor, _ := custom.Attr("data-floor"); floor != "2" {
		t.Errorf("it lost its attributes: data-floor is %q", floor)
	}
	if doc.ByID("nobr").Tag().String() != "div" || doc.ByID("known").Tag().String() != "section" {
		t.Errorf("nobr is a %s and section a %s", doc.ByID("nobr").Tag(), doc.ByID("known").Tag())
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
		if r.URL.Path == "/big" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("x", maxBytes+100)))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("<p>line</p>", maxElements+50)))
	}))
	t.Cleanup(srv.Close)
	if n := len(fetch(t.Context(), srv.URL, linker{}).doc.Body().Children()); n != maxElements {
		t.Errorf("%d elements built, want %d", n, maxElements)
	}
	if n := len(fetch(t.Context(), srv.URL+"/big", linker{}).doc.Body().Text()); n != maxBytes {
		t.Errorf("%d bytes read, want %d", n, maxBytes)
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
			<a id="turned" href="/vaults" style="-webkit-transition: all 0.4s ease; -o-transition: all 0.4s ease">v</a>`))
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
