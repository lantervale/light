//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/tabs"
)

// scripted is a small site whose pages run scripts.
func scripted(t *testing.T) (*httptest.Server, *asked) {
	t.Helper()
	return answers(t, map[string]answer{
		"/": typed("text/html", `<title>Counter</title><style>body { margin: 0 }
			button, a, input { display: block; height: 24px }</style><p id="count">none</p>
			<button id="more">More</button><a id="next" href="/next">next</a>
			<a id="held" href="/next">held</a><a id="pushed" href="/pushed">pushed</a><input id="field">
			<script src="/app.js"></script>`),
		"/app.js": typed("text/javascript", `let n = 0;
			const count = document.getElementById("count");
			count.textContent = "0 clicks";
			document.getElementById("more").addEventListener("click", () => {
				count.textContent = ++n + " clicks";
				document.title = "Counter " + n;
				console.log("clicked", n);
			});
			document.getElementById("held").addEventListener("click", (e) => e.preventDefault());
			document.getElementById("pushed").addEventListener("click", (e) => {
				e.preventDefault();
				history.pushState({ n }, "", "/pushed");
			});
			window.addEventListener("popstate", (e) => { count.textContent = "popped " + JSON.stringify(e.state) });
			document.getElementById("field").addEventListener("keydown", (e) => { count.textContent = "key " + e.key });
			localStorage.setItem("visits", String(Number(localStorage.getItem("visits") || 0) + 1));`),
		"/next": typed("text/html", `<title>Next</title><p id="visits">?</p><script>
			document.getElementById("visits").textContent = localStorage.getItem("visits") + " visits";
			fetch("/data.json").then((r) => r.json()).then((d) => { document.title = d.title });</script>`),
		"/data.json": typed("application/json", strings.Repeat(" ", 2048)+`{"title": "Fetché"}`),
		"/go":        typed("text/html", `<title>Go</title><script>location.href = "/next"</script>`),
		"/forever": typed("text/html", `<title>Forever</title><p id="p">before</p><script>for (;;) {}</script>
			<script>document.getElementById("p").textContent = "after"</script>`),
	})
}

func (s *shown) pageText(id string) string {
	s.t.Helper()
	page := s.session().pages[s.browser.Tab().Current()]
	if page == nil || page.Doc == nil || page.Doc.ByID(id) == nil {
		s.t.Fatalf("the shown page has no #%s", id)
	}
	return page.Doc.ByID(id).Text()
}

func (s *shown) said() string {
	var lines []string
	for _, l := range s.session().said.Lines() {
		lines = append(lines, string(l.Level)+": "+l.Text)
	}
	return strings.Join(lines, "\n")
}

func TestAPagesScriptsRunAndHearItsClicks(t *testing.T) {
	srv, _ := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.until("its scripts ran", func() bool { return s.pageText("count") == "0 clicks" })
	s.clickAt(s.inPage("more"))
	s.clickAt(s.inPage("more"))
	s.frame()
	if got := s.pageText("count"); got != "2 clicks" {
		t.Fatalf("after two clicks the page reads %q; the console says %s", got, s.said())
	}
	if got := s.title(srv.URL + "/"); got != "Counter 2" {
		t.Errorf("the tab is named %q", got)
	}
	lines := s.session().said.Lines()
	if len(lines) != 2 || lines[1].Text != "clicked 2" || lines[1].Where() != "app.js:7" || lines[1].Level != console.Log {
		t.Errorf("the console holds %+v", lines)
	}
	if rows := strings.Join(s.requests(), ", "); !strings.Contains(rows, "OK /app.js by document") {
		t.Errorf("the script's request is not listed: %s", rows)
	}
	s.clean()
}

func TestThePointerOverAScriptedPageIsLookedUpOnlyAtAClick(t *testing.T) {
	srv, _ := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.until("its scripts ran", func() bool { return s.pageText("count") == "0 clicks" })
	more, next := s.inPage("more"), s.inPage("next")
	before := s.picked
	for i := range 20 {
		s.in.SetMouse(more.Lerp(next, float32(i)/20))
		s.frame()
	}
	if got := s.picked - before; got != 0 {
		t.Errorf("the pointer moving over the page had it looked up %d times, with no button", got)
	}
	s.clickAt(more)
	if got := s.picked - before; got != 2 {
		t.Errorf("a click had the page looked up %d times, not at its press and its release", got)
	}
	if got := s.pageText("count"); got != "1 clicks" {
		t.Errorf("after a click the page reads %q", got)
	}
	s.clean()
}

func TestAScriptDecidesWhetherALinkIsFollowed(t *testing.T) {
	srv, _ := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("held"))
	s.frame()
	s.at(srv.URL + "/")
	s.clickAt(s.inPage("pushed"))
	s.frame()
	s.at(srv.URL + "/pushed")
	if s.session().scripts == nil || s.pageText("count") != "0 clicks" || s.disabled("back") {
		t.Fatalf("after pushState the page reads %q, back disabled %v", s.pageText("count"), s.disabled("back"))
	}
	s.click("back")
	s.frame()
	s.at(srv.URL + "/")
	s.click("forward")
	s.frame()
	s.frame()
	if got := s.pageText("count"); got != `popped {"n":0}` {
		t.Fatalf("back at what it pushed the page reads %q", got)
	}
	s.clickAt(s.inPage("next"))
	s.until("the link was followed", func() bool { return s.session().pages[srv.URL+"/next"] != nil })
	s.at(srv.URL + "/next")
}

func TestLeavingAPageEndsItsScripts(t *testing.T) {
	srv, seen := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	first := s.session().scripts
	if first == nil || first.rt.Closed() {
		t.Fatal("the shown page runs no script")
	}
	s.visit(srv.URL + "/next")
	s.frame()
	if !first.rt.Closed() || s.session().scripts == nil || s.session().scripts == first {
		t.Fatal("the page left still runs its scripts, or the new one runs none")
	}
	if s.session().pages[srv.URL+"/"] != nil {
		t.Fatal("the page left is kept")
	}
	s.click("back")
	s.until("the page left is fetched again", func() bool { return seen.count("/") == 2 && s.session().scripts != nil })
	s.until("its scripts ran again", func() bool { return s.pageText("count") == "0 clicks" })
	again := s.session().scripts
	s.OpenInNewTab(tabs.Home)
	s.frame()
	if again.rt.Closed() {
		t.Fatal("a tab left in the background lost its scripts")
	}
	s.closeTab(0)
	s.frame()
	if !again.rt.Closed() {
		t.Fatal("a closed tab still runs its scripts")
	}
}

func TestScriptsNavigateFetchAndStore(t *testing.T) {
	srv, _ := scripted(t)
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.visit(srv.URL + "/")
	s.visit(srv.URL + "/go")
	s.until("the script navigated", func() bool { return s.session().pages[srv.URL+"/next"] != nil })
	s.at(srv.URL + "/next")
	s.until("the script fetched", func() bool { return s.title(srv.URL+"/next") == "Fetché" })
	if got := s.pageText("visits"); got != "1 visits" {
		t.Errorf("the page reads %q of what another stored", got)
	}
	if rows := strings.Join(s.requests(), ", "); !strings.Contains(rows, "OK /data.json by next:3") {
		t.Errorf("the script's fetch is not listed: %s", rows)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, storageFile))
	if err != nil || !strings.Contains(string(data), `"visits": "1"`) || !strings.Contains(string(data), srv.URL) {
		t.Fatalf("the file holds %s, %v", data, err)
	}
}

func TestARunawayScriptLeavesLightResponsive(t *testing.T) {
	srv, _ := scripted(t)
	s := showWith(t, Options{Downloads: t.TempDir(), ScriptBudget: 50 * time.Millisecond})
	began := time.Now()
	s.visit(srv.URL + "/forever")
	s.until("the script was stopped", func() bool { return strings.Contains(s.said(), "too long") })
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("light was held %v", took)
	}
	if got := s.pageText("p"); got != "after" {
		t.Errorf("the script after the runaway one left %q", got)
	}
	s.click("new-tab")
	if len(s.browser.Tabs) != 2 {
		t.Fatal("light no longer answers a click")
	}
}

func TestKeysTypedInAPageReachItsScripts(t *testing.T) {
	srv, _ := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.clickAt(s.inPage("field"))
	s.in.AddText('x')
	s.press(input.KeyX)
	s.frame()
	if got := s.pageText("count"); got != "key x" {
		t.Fatalf("after a key the page reads %q", got)
	}
}

func TestJavaScriptIsASetting(t *testing.T) {
	srv, seen := scripted(t)
	s := show(t)
	s.settings()
	if !s.doc.Checked(s.node("set-javascript")) || !strings.Contains(s.node("javascript-engine").Text(), "V8") {
		t.Fatalf("the setting is off, or names no engine: %q", s.node("javascript-engine").Text())
	}
	s.click("set-javascript")
	s.visit(srv.URL + "/")
	for range 10 {
		s.frame()
	}
	if s.pageText("count") != "none" || seen.count("/app.js") != 0 || s.session().scripts != nil {
		t.Fatalf("with JavaScript off the page reads %q, its script asked for %d times", s.pageText("count"),
			seen.count("/app.js"))
	}
	s.clean()
}

func TestTheConsoleEvaluatesInTheShownPage(t *testing.T) {
	srv, _ := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.toolsKeys()
	s.click("dt-tab-console")
	s.click("dt-eval")
	for _, r := range "n + 40" {
		s.in.AddText(r)
	}
	s.frame()
	s.press(input.KeyEnter)
	s.frame()
	if got := s.said(); got != "input: n + 40\nresult: 40" {
		t.Fatalf("the console holds %q", got)
	}
	if lines := s.node("dt-lines").Children(); len(lines) != 2 || !lines[1].HasClass("dt-result") {
		t.Fatalf("the Console tab lists %d lines", len(lines))
	}
	s.clean()
}

func TestTheScriptsPageRuns(t *testing.T) {
	s := show(t)
	s.Open(scriptsPage)
	s.until("its scripts ran", func() bool { return s.session().scripts != nil && s.pageText("count") == "0" })
	page := s.session().pages[scriptsPage]
	if page.Doc.ByID("none") != nil {
		t.Fatal("the page still says no script ran")
	}
	s.clickAt(s.inPage("more"))
	s.clickAt(s.inPage("more"))
	s.clickAt(s.inPage("less"))
	if got := s.pageText("count") + " " + s.pageText("parity"); got != "1 odd" {
		t.Fatalf("the counter reads %q; the console says %s", got, s.said())
	}
	s.clickAt(s.inPage("todo"))
	for _, r := range "feed the cat" {
		s.in.AddText(r)
	}
	s.frame()
	s.press(input.KeyEnter)
	s.frame()
	s.frame()
	if got := page.Doc.ByID("todos").Text(); !strings.Contains(got, "feed the cat") {
		t.Fatalf("the list holds %q, the problem %q", got, s.pageText("problem"))
	}
	s.until("the clock counts", func() bool { return s.pageText("clock") != "0.0" })
	if strings.Contains(s.said(), "error") || strings.Contains(s.said(), "warn") {
		t.Errorf("the console says %s", s.said())
	}
	if warns := page.Doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page has warnings: %v", warns)
	}
	s.clean()
}

// consoled is a site whose pages speak to the console.
func consoled(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Loud</title><p id="p">loud</p><script>
			console.group("Loading");
			console.warn("slow");
			console.groupEnd();
			function inner() { console.trace("traced"); }
			(function outer() { inner(); })();
			setTimeout(function later() { null.x; }, 0);</script>`),
		"/plain": typed("text/html", `<title>Plain</title><p id="p">no script here</p>`),
	})
	return srv
}

func TestTheConsoleKeepsTheGroupsAndTheStacksOfWhatWasSaid(t *testing.T) {
	srv := consoled(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.until("the timer ran", func() bool { return strings.Contains(s.said(), "Uncaught TypeError") })
	lines := s.session().said.Lines()
	if len(lines) != 4 || lines[0].Depth != 0 || lines[1].Depth != 1 || lines[1].Level != console.Warn {
		t.Fatalf("the console holds %+v", lines)
	}
	if trace := lines[2]; trace.Text != "traced" || !strings.HasPrefix(trace.Stack, "inner (") ||
		!strings.Contains(trace.Stack, "\nouter (") {
		t.Errorf("the trace is %+v", trace)
	}
	if thrown := lines[3]; thrown.Level != console.Error || thrown.Where() != strings.TrimPrefix(srv.URL, "http://")+":7" {
		t.Errorf("the exception is %s from %s", thrown.Level, thrown.Where())
	}
	s.toolsKeys()
	s.click("dt-tab-console")
	if got := s.node("dt-console-errors").Text(); got != "1" {
		t.Errorf("the tab counts %s errors", got)
	}
	s.clean()
}

func TestTheConsoleGoesAtTheNextPageUnlessPreserved(t *testing.T) {
	srv := consoled(t)
	s := show(t)
	s.toolsKeys()
	s.click("dt-tab-console")
	s.visit(srv.URL + "/")
	if !strings.Contains(s.said(), "warn: slow") {
		t.Fatalf("the console says %s", s.said())
	}
	s.visit(srv.URL + "/plain")
	if got := s.said(); got != "" {
		t.Fatalf("after another page the console still says %s", got)
	}
	s.click("dt-preserve")
	s.visit(srv.URL + "/")
	s.visit(srv.URL + "/plain")
	if !strings.Contains(s.said(), "warn: slow") {
		t.Fatalf("with the log preserved the console says %q", s.said())
	}
	s.clean()
}

func TestTheConsoleEvaluatesInAPageWithoutAScript(t *testing.T) {
	srv := consoled(t)
	s := show(t)
	s.visit(srv.URL + "/plain")
	if s.session().scripts != nil {
		t.Fatal("a page without a script runs some")
	}
	s.toolsKeys()
	s.click("dt-tab-console")
	if s.doc.ByID("dt-unscripted") != nil {
		t.Fatalf("the console says %q", s.node("dt-unscripted").Text())
	}
	s.click("dt-eval")
	for _, line := range []string{`document.getElementById("p").textContent`, `document.title = "Changed"`, `nope()`} {
		for _, r := range line {
			s.in.AddText(r)
		}
		s.frame()
		s.press(input.KeyEnter)
		s.frame()
	}
	want := "input: document.getElementById(\"p\").textContent\nresult: \"no script here\"\n" +
		"input: document.title = \"Changed\"\nresult: \"Changed\"\ninput: nope()\n" +
		"error: Uncaught ReferenceError: nope is not defined"
	if got := s.said(); got != want {
		t.Fatalf("the console holds %q, want %q", got, want)
	}
	s.until("the page is titled", func() bool { return s.session().pages[s.browser.Tab().Current()].Title == "Changed" })
	s.clean()
}

func TestTheConsoleSaysJavaScriptIsOff(t *testing.T) {
	srv := consoled(t)
	s := show(t)
	s.config.JavaScript = false
	s.apply()
	s.visit(srv.URL + "/")
	s.toolsKeys()
	s.click("dt-tab-console")
	if text := s.node("dt-unscripted").Text(); !strings.Contains(text, "JavaScript is off") {
		t.Fatalf("the console says %q", text)
	}
	if !s.disabled("dt-eval") {
		t.Error("the console's line still takes what is typed")
	}
	s.clean()
}

func TestTheScriptsPageSpeaksToTheConsole(t *testing.T) {
	s := show(t)
	s.Open(scriptsPage)
	s.until("its scripts ran", func() bool { return s.session().scripts != nil && s.pageText("count") == "0" })
	for _, id := range []string{"speak", "fail"} {
		if _, err := s.evaluate(`document.getElementById("` + id + `").click()`); err != nil {
			t.Fatal(err)
		}
	}
	s.frame()
	said := s.said()
	for _, want := range []string{
		"log: Said 1", `log: a log with values {said: 1, list: [1, "two", null]} <body>`, "info: info with a style left out",
		"warn: a warning", "error: an error", "debug: a debug line", "log: (index)  name   kind",
		"error: Uncaught RangeError: thrown by a click",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the console does not say %q:\n%s", want, said)
		}
	}
	if lines := s.session().said.Lines(); len(lines) < 3 || lines[2].Depth != 1 || lines[2].Where() != "scripts:"+
		strconv.Itoa(lines[2].Line) || lines[2].Line == 0 {
		t.Errorf("a line of the group is %+v", lines)
	}
	s.clean()
}
