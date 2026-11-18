package web //nolint:testpackage // the builder's tree and the caps tested are unexported

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

// scripted is the page at address of a site serving by, loaded with
// scripts on, and the requests it made.
func scripted(t *testing.T, by map[string]answer, path string) (*Page, *netlog.Log) {
	t.Helper()
	srv, _ := answers(t, by)
	log := netlog.New(100)
	loader := &Loader{Client: fetch.New(&netlog.Transport{}), Scripts: true}
	nothing := func(string) {}
	page := loader.Load(netlog.With(t.Context(), log, "navigation", netlog.Document), srv.URL+path,
		Links{Open: nothing, Over: nothing})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	return page, log
}

// byID is the element of that id in a page's tree, nil for none.
func (t *Tree) byID(id string) *html.Node {
	var find func(n *html.Node) *html.Node
	find = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && attr(n, "id") == id {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := find(c); found != nil {
				return found
			}
		}
		return nil
	}
	return find(t.root)
}

func markup(body string) answer {
	return answer{header: map[string]string{"Content-Type": "text/html"}, body: body}
}

func source(body string) answer {
	return answer{header: map[string]string{"Content-Type": "text/javascript"}, body: body}
}

func TestAPagesScriptsAreKeptInTheOrderTheyRun(t *testing.T) {
	page, log := scripted(t, map[string]answer{
		"/": markup(`<head><script>first()</script><script src="/late.js" defer></script>
			<script src="lib/second.js"></script><script type="module">import "x"</script>
			<script type="text/template">not a script</script><script src="/gone.js"></script>
			<script nomodule>fallback()</script><script async src="/async.js"></script></head>
			<body><p>text</p><script type="text/javascript">third()</script><script type="application/ld+json">{}</script>`),
		"/late.js": source("late()"), "/lib/second.js": source("second()"), "/async.js": source("whenever()"),
	}, "/")
	var ran, skipped []string
	for _, s := range page.Scripts {
		if s.Skipped != "" {
			skipped = append(skipped, s.Skipped)
			continue
		}
		ran = append(ran, s.Source+"@"+s.Address[strings.LastIndex(s.Address, "/"):])
	}
	want := "first()@/ second()@/second.js fallback()@/ third()@/ late()@/late.js whenever()@/async.js"
	if got := strings.Join(ran, " "); got != want {
		t.Errorf("the scripts run as %s\nwant %s", got, want)
	}
	if len(skipped) != 2 || !strings.Contains(skipped[0], "module") || !strings.Contains(skipped[1], "404") {
		t.Errorf("the scripts skipped are %q", skipped)
	}
	scripts := 0
	for _, row := range log.Rows() {
		if row.Initiator == "document" && (row.Type == netlog.Script || strings.HasSuffix(row.Address, "/gone.js")) {
			scripts++
		}
	}
	if scripts != 4 {
		t.Errorf("%d requests of scripts are in the log, want 4: %+v", scripts, log.Rows())
	}
}

func TestAScriptAndASheetAreReadAsUTF8WhateverTheirFirstBytes(t *testing.T) {
	late := strings.Repeat(" ", 2048)
	page, _ := scripted(t, map[string]answer{
		"/":         markup(`<link rel="stylesheet" href="/site.css"><script src="/app.js"></script><p id="p">text</p>`),
		"/app.js":   source(late + "var r = {À: 'A'}"),
		"/site.css": typed("text/css", late+"#p::before { content: 'é'; }"),
	}, "/")
	if len(page.Scripts) != 1 || !strings.HasSuffix(page.Scripts[0].Source, "{À: 'A'}") {
		t.Errorf("the script reads %q", strings.TrimSpace(page.Scripts[0].Source))
	}
	if sheet := strings.TrimSpace(page.Tree.b.sheets[0].text); sheet != "#p::before { content: 'é'; }" {
		t.Errorf("the sheet reads %q", sheet)
	}
}

func TestWithoutScriptsAPageKeepsNone(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/": markup(`<script src="/app.js"></script><p id="p" onclick="go()">text</p>`), "/app.js": source("go()"),
	})
	page := load(t, srv.URL+"/")
	if page.Scripts != nil || page.Tree != nil || seen.has("/app.js") {
		t.Fatalf("a page loaded without scripts has %d, a tree %v, its script fetched %v", len(page.Scripts),
			page.Tree != nil, seen.has("/app.js"))
	}
}

func TestWithScriptsWhatTheyReadIsKeptOutOfSight(t *testing.T) {
	page, _ := scripted(t, map[string]answer{"/": markup(`<html lang="en"><head><title>Vault</title>
		<meta name="depth" content="3"><link rel="canonical" href="/vault"><style>p { margin: 0 }</style>
		<script id="app" data-mode="x">run()</script></head>
		<body onload="start()"><p id="shown" onclick="tap()" tabindex="2">shown</p><script id="inline">more()</script>
		<noscript><p id="without">without</p></noscript>
		<template id="row"><li class="item">one</li></template><crypt-map id="map"></crypt-map>`)}, "/")
	tree, doc := page.Tree, page.Doc
	var tags []string
	var head []*ui.Node
	for _, n := range tree.Head().Children() {
		if !n.IsText() {
			head = append(head, n)
			tags = append(tags, tree.Tag(n))
		}
	}
	if got := strings.Join(tags, " "); got != "title meta link style script script" {
		t.Errorf("out of sight are %s", got)
	}
	if doc.ByID("app") != nil || doc.ByID("inline") != nil || doc.ByID("without") != nil ||
		strings.TrimSpace(doc.Body().Text()) != "shown" {
		t.Errorf("the page shows %q, its scripts among its elements", doc.Body().Text())
	}
	meta := head[1]
	if content, _ := meta.Attr("content"); content != "3" || head[0].Text() != "Vault" {
		t.Errorf("the meta's content is %q, the title %q", content, head[0].Text())
	}
	if href, _ := head[2].Attr("href"); href != "/vault" {
		t.Errorf("the link's href is %q", href)
	}
	if tree.Node(page.Scripts[0].Node) != head[4] || attr(page.Scripts[1].Node, "id") != "inline" {
		t.Error("the scripts are not their elements")
	}
	shown := doc.ByID("shown")
	if handler, _ := shown.Attr("onclick"); handler != "tap()" {
		t.Errorf("onclick is %q", handler)
	}
	if _, has := shown.Attr("tabindex"); has {
		t.Error("tabindex is kept")
	}
	if handler, _ := doc.Body().Attr("onload"); handler != "start()" {
		t.Errorf("the body's onload is %q", handler)
	}
	theTemplateAndTheNoscriptAreKept(t, tree, doc)
}

// theTemplateAndTheNoscriptAreKept checks what the tree keeps of the
// elements the page does not show, and the tags it tells.
func theTemplateAndTheNoscriptAreKept(t *testing.T, tree *Tree, doc *ui.Document) {
	t.Helper()
	row := tree.byID("row")
	if row == nil || row.FirstChild != nil || doc.ByID("row") != nil || tree.Node(row) != nil {
		t.Fatalf("the template is %v, shown as %v", row, doc.ByID("row"))
	}
	content := tree.Content(row)
	if content == nil || content.FirstChild == nil || content.FirstChild != content.LastChild ||
		attr(content.FirstChild, "class") != "item" || content.FirstChild.Parent != content {
		t.Errorf("the template's content is %v", content)
	}
	without := tree.byID("shown").NextSibling.NextSibling.NextSibling
	if without.Data != "noscript" || without.FirstChild.Type != html.TextNode ||
		without.FirstChild.Data != `<p id="without">without</p>` {
		t.Errorf("the noscript is %v holding %v", without, without.FirstChild)
	}
	shown := doc.ByID("shown")
	if tree.Tag(doc.ByID("map")) != "crypt-map" || tree.Tag(shown) != "p" || tree.Tag(doc.Body()) != "body" {
		t.Errorf("the tags are %s, %s, %s", tree.Tag(doc.ByID("map")), tree.Tag(shown), tree.Tag(doc.Body()))
	}
}

func TestATreeMakesWhatScriptsAskFor(t *testing.T) {
	page, _ := scripted(t, map[string]answer{"/vault/": markup(`<p id="p">text</p>`)}, "/vault/")
	tree := page.Tree
	if n := tree.Make(tree.Element("section", "")); n.Tag().String() != "section" || tree.Tag(n) != "section" {
		t.Errorf("a section is a %s", n.Tag())
	}
	if n := tree.Make(tree.Element("x-gauge", "")); n.Tag().String() != "div" || tree.Tag(n) != "x-gauge" {
		t.Errorf("a custom element is a %s named %s", n.Tag(), tree.Tag(n))
	}
	if path := tree.Element("path", "svg"); tree.Make(path) != nil || path.Namespace != "svg" {
		t.Errorf("ui has %v of an SVG path", tree.Make(path))
	}
	nodes := tree.Fragment(`lead <b class="k" onclick="x()">bold</b><!--c--><script>never()</script><td>cell</td>`, "div")
	if len(nodes) != 5 || nodes[0].Type != html.TextNode || nodes[0].Data != "lead " || attr(nodes[1], "class") != "k" ||
		nodes[2].Type != html.CommentNode || nodes[3].Data != "script" || nodes[3].Parent != nil ||
		nodes[4].Data != "cell" {
		t.Fatalf("the fragment is %d nodes: %v", len(nodes), nodes)
	}
	if len(page.Scripts) != 0 {
		t.Error("a fragment's script is among the page's")
	}
	rows := tree.Fragment(`<tr><td>a</td><td>b</td></tr>`, "tbody")
	if len(rows) != 1 || rows[0].Data != "tr" || rows[0].FirstChild.NextSibling != rows[0].LastChild {
		t.Errorf("a row read in a tbody is %v", rows)
	}
}

func TestALinkScriptsMadeOpensWhereItSaysThen(t *testing.T) {
	srv, _ := answers(t, map[string]answer{"/vault/": markup(`<style>a { display: block; height: 20px; }</style>
		<a id="a" href="one">one</a><div id="into"></div>`)})
	s := show(t)
	s.scripts = true
	doc := s.visit(srv.URL + "/vault/")
	a := doc.ByID("a")
	a.SetAttr("href", "two")
	s.click(a)
	s.at(srv.URL + "/vault/two")
	tree := s.page.Tree
	link := tree.Element("a", "")
	tree.SetAttr(link, "href", "/three.csv")
	tree.SetAttr(link, "download", "t.csv")
	for _, n := range tree.Fragment("save", "a") {
		tree.Insert(link, n, nil)
	}
	tree.Insert(tree.Of(doc.ByID("into")), link, nil)
	made := tree.Node(link)
	if made == nil || made.Parent() != doc.ByID("into") || made.Text() != "save" {
		t.Fatalf("the link made shows as %v", made)
	}
	s.frame()
	s.click(made)
	if s.saved != srv.URL+"/three.csv as t.csv" {
		t.Errorf("the link saved %q", s.saved)
	}
	var asked *ui.Node
	s.page.Tree.Activate = func(link *ui.Node, _ func()) { asked = link }
	a.SetAttr("href", "four")
	s.click(a)
	if asked != a {
		t.Errorf("a tree that activates links itself was asked %v", asked)
	}
	s.at(srv.URL + "/vault/two")
}

func TestAPageOfLightsOwnIsBuiltFromItsText(t *testing.T) {
	loader := &Loader{Client: fetch.New(nil), Scripts: true}
	page := loader.Local("light://scripts", `<title>Scripts</title><p id="p">text</p><script>go()</script>`,
		Links{Open: func(string) {}, Over: func(string) {}})
	if page.Err != nil || page.Title != "Scripts" || page.Doc.ByID("p") == nil || len(page.Scripts) != 1 ||
		page.Scripts[0].Address != "light://scripts" || page.Address != "light://scripts" {
		t.Fatalf("the page is %+v", page)
	}
}

func scriptsAreKeptOnlyWhenTheyRun(t *testing.T) {
	by := map[string]answer{
		"/": markup(`<title>T</title><meta name="kind" content="test"><script src="/app.js"></script>
			<p id="p" onclick="go()">text</p><a id="a" href="/next">next</a><template id="t"><b>held</b></template>`),
		"/app.js": source("go()"),
	}
	page, _ := scripted(t, by, "/")
	if len(page.Scripts) != 1 || page.Scripts[0].Source != "go()" || !page.Scripted() {
		t.Fatalf("the page keeps %+v", page.Scripts)
	}
	var kept []string
	for _, n := range page.Tree.Head().Children() {
		if !n.IsText() {
			kept = append(kept, page.Tree.Tag(n))
		}
	}
	if got := strings.Join(kept, ","); got != "title,meta,script" {
		t.Errorf("out of sight the page has %s", got)
	}
	if handler, _ := page.Doc.ByID("p").Attr("onclick"); handler != "go()" {
		t.Errorf("the handler kept is %q", handler)
	}
	if href, _ := page.Doc.ByID("a").Attr("href"); href != "/next" {
		t.Errorf("the link's href is %q", href)
	}
	if content := page.Tree.Content(page.Tree.byID("t")); content == nil || content.FirstChild.Data != "b" {
		t.Error("the template's content is not kept apart")
	}
	srv, seen := answers(t, by)
	plain := load(t, srv.URL+"/")
	if plain.Scripted() || seen.has("/app.js") || plain.Doc.ByID("t") != nil {
		t.Error("without scripts the page keeps one, fetched it, or built its template")
	}
	if _, kept := plain.Doc.ByID("p").Attr("onclick"); kept {
		t.Error("without scripts a handler is kept")
	}
}

func TestARefreshInANoscriptIsFollowedOnlyWithoutScripts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/plain" {
			_, _ = w.Write([]byte(`<title>Plain</title><p>no script</p>`))
			return
		}
		_, _ = w.Write([]byte(`<head><title>App</title><noscript><meta http-equiv="refresh" ` +
			`content="0; url=&quot;/plain&quot;"><style>body { display: none }</style></noscript></head><p>app</p>`))
	}))
	t.Cleanup(srv.Close)
	nothing := func(string) {}
	for _, c := range []struct {
		scripts bool
		title   string
	}{{false, "Plain"}, {true, "App"}} {
		loader := &Loader{Client: fetch.New(nil), Scripts: c.scripts}
		if page := loader.Load(t.Context(), srv.URL+"/", Links{Open: nothing, Over: nothing}); page.Title != c.title {
			t.Errorf("with scripts %v the page is %q at %s, want %q: %v", c.scripts, page.Title, page.Address, c.title, page.Err)
		}
	}
}
