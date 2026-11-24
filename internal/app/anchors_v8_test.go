//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http/httptest"
	"testing"
)

// heard is glossary's page with a script that writes down what it hears
// of its address changing, and links that scripts answer.
func heard(t *testing.T) (*httptest.Server, *asked) {
	t.Helper()
	return answers(t, map[string]answer{
		"/": typed("text/html", `<title>Heard</title>`+glossaryLook+`<p id="log">nothing</p>
			<a id="to-low" href="#low">low</a><a id="by-script" href="/gone">by script</a>
			<a id="push" href="/gone">push</a><a id="held" href="#low">held</a>
			<a id="clicker" href="/gone">clicker</a>
			<div class="gap"></div><h2 id="low">Low</h2>
			<div class="gap"></div><a id="by-name" name="named">Named</a><div class="gap"></div>
			<script>
			const log = [];
			const say = (what) => { log.push(what); document.getElementById("log").textContent = log.join(" "); };
			const on = (id, does) => document.getElementById(id).addEventListener("click", (e) => {
				e.preventDefault();
				does();
			});
			addEventListener("hashchange", (e) => {
				const target = document.querySelector(":target");
				say("hash " + new URL(e.oldURL).hash + ">" + location.hash + " target " + (target ? target.id : "none"));
			});
			addEventListener("popstate", () => say("pop"));
			on("by-script", () => { location.hash = "#named"; });
			on("push", () => { history.pushState(null, "", "#low"); });
			on("held", () => {});
			on("clicker", () => { document.getElementById("to-low").click(); });
			</script>`),
		"/plain": typed("text/html", `<title>Plain</title><p id="to-low">plain</p>`),
		"/framing": typed("text/html", `<title>Framing</title><body style="margin:0"><p id="log">nothing</p>
			<iframe id="inner" src="/" width="300" height="200"></iframe>
			<script>addEventListener("hashchange", () => { document.getElementById("log").textContent = "heard"; });
			</script>`),
	})
}

func TestAFragmentLinkTellsThePagesScripts(t *testing.T) {
	srv, seen := heard(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("to-low"))
	s.at(srv.URL + "/#low")
	if !s.shows("low") {
		t.Fatal("the link did not scroll to its place")
	}
	if got := s.pageText("log"); got != "pop hash >#low target low" {
		t.Fatalf("after the link the page heard %q", got)
	}
	s.click("back")
	s.at(srv.URL + "/")
	if got := s.pageText("log"); !s.shows("to-low") || got != "pop hash >#low target low pop hash #low> target none" {
		t.Fatalf("back shows the top %v, and the page heard %q", s.shows("to-low"), got)
	}
	if got := seen.count("/"); got != 1 || s.session().scripts == nil {
		t.Fatalf("the page was asked for %d times", got)
	}
}

func TestBackToAPageFetchedAgainReturnsToWhereItWasLeft(t *testing.T) {
	srv, seen := heard(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.scroll(700)
	s.visit(srv.URL + "/plain")
	s.click("back")
	s.until("the page was fetched again", func() bool {
		return seen.count("/") == 2 && s.session().pending == nil && s.session().pages[srv.URL+"/"] != nil
	})
	if got := s.scrolled(); got != 700 {
		t.Fatalf("fetched again the page is scrolled %v, want the 700 it was left at", got)
	}
}

func TestAScriptsHashScrollsAndItsPushStateDoesNot(t *testing.T) {
	srv, seen := heard(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("push"))
	s.at(srv.URL + "/#low")
	if got := s.pageText("log"); s.shows("low") || got != "nothing" {
		t.Fatalf("pushState scrolled to the place %v, and the page heard %q", s.shows("low"), got)
	}
	s.clickAt(s.inPage("by-script"))
	s.at(srv.URL + "/#named")
	if got := s.pageText("log"); !s.topmost("by-name") || got != "hash #low>#named target none" {
		t.Fatalf("location.hash shows its place %v, and the page heard %q", s.shows("by-name"), got)
	}
	if got := seen.count("/"); got != 1 || s.browser.Tab().Len() != 4 {
		t.Fatalf("the page was asked for %d times, the history has %d steps", got, s.browser.Tab().Len())
	}
}

func TestAScriptHoldsAFragmentLinkOrClicksIt(t *testing.T) {
	srv, _ := heard(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("held"))
	s.at(srv.URL + "/")
	if s.shows("low") {
		t.Fatal("a click a script prevented scrolled the page")
	}
	s.clickAt(s.inPage("clicker"))
	s.at(srv.URL + "/#low")
	if got := s.pageText("log"); !s.shows("low") || got != "pop hash >#low target low" {
		t.Fatalf("a script's click shows the place %v, and the page heard %q", s.shows("low"), got)
	}
}

func TestAFragmentLinkInAFrameTellsThatFramesScriptsAlone(t *testing.T) {
	srv, seen := heard(t)
	s := show(t)
	s.visit(srv.URL + "/framing")
	s.framedText("inner")
	s.quiet()
	f := s.frameAt("inner")
	s.clickAt(s.inFrame(f, "to-low"))
	for range 4 {
		s.frame()
	}
	if got, at := f.page.Doc.ByID("log").Text(), box(t, f.page.Doc, "low").Y; at < -0.5 || at > 0.5 ||
		got != "pop hash >#low target low" {
		t.Fatalf("the frame has the place %v from its top, and its scripts heard %q", at, got)
	}
	if got := s.pageText("log"); got != "nothing" || seen.count("/") != 1 {
		t.Fatalf("the page heard %q, and the frame's document was asked for %d times", got, seen.count("/"))
	}
	s.click("back")
	for range 4 {
		s.frame()
	}
	if got := f.page.Doc.ByID("log").Text(); !inView(t, f.page.Doc, "to-low", 200) || seen.count("/") != 1 ||
		got != "pop hash >#low target low pop hash #low> target none" {
		t.Fatalf("back shows the frame's top %v, its scripts heard %q, its document asked for %d times",
			inView(t, f.page.Doc, "to-low", 200), got, seen.count("/"))
	}
}
