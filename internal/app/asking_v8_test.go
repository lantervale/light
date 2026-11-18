//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the dialogs are checked with the screens that show them and the scripts that wait

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/script"
)

// asksPage is a page whose scripts ask what a test tells them to, and
// write down each answer.
const asksPage = `<title>Asks</title><body style="margin:0"><button id="more" style="display:block;height:24px">More
	</button><p id="out">none</p><script>
	window.ticks = 0;
	window.clicks = 0;
	setInterval(() => window.ticks++, 5);
	document.getElementById("more").addEventListener("click", () => window.clicks++);
	window.say = (value) => {
		document.getElementById("out").textContent = value === undefined ? "undefined" : JSON.stringify(value);
	};
	</script>`

func askingPage(t *testing.T) (*shown, string) {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
		"/":      typed("text/html", asksPage),
		"/other": typed("text/html", `<title>Other</title><p id="here">other</p><script>window.other = true</script>`),
	})
	s := showWith(t, Options{Downloads: t.TempDir(), ScriptBudget: 400 * time.Millisecond})
	s.visit(srv.URL + "/")
	return s, srv.URL
}

// has has the shown page's scripts run statements at their next frame,
// and waits for the dialog of what they ask.
func (s *shown) has(statements string) *ui.Node {
	s.t.Helper()
	if _, err := s.session().scripts.rt.Evaluate(`setTimeout(() => { ` + statements + ` }, 0), 0`); err != nil {
		s.t.Fatal(err)
	}
	s.until("the page asked", func() bool { return s.doc.ByID("asking") != nil })
	return s.doc.ByID("asking")
}

// told waits for the answer the page's scripts wrote down.
func (s *shown) told(want string) {
	s.t.Helper()
	for range 20 {
		s.frame()
	}
	if got := s.pageText("out"); got != want || s.doc.ByID("asking") != nil {
		s.t.Fatalf("the script was answered %s, want %s (dialog shown: %v)", got, want, s.doc.ByID("asking") != nil)
	}
}

func TestAPageAsksAndItsScriptsWaitForTheAnswer(t *testing.T) {
	s, base := askingPage(t)
	dialog := s.has(`say(confirm("Are you <b>sure</b>?\nReally?"))`)
	label, _ := dialog.Attr("aria-label")
	if want := strings.TrimPrefix(base, "http://") + " says"; label != want {
		t.Fatalf("the dialog is named %q, want %q", label, want)
	}
	message := s.doc.ByID("asking-message")
	if message.Text() != "Are you <b>sure</b>?\nReally?" {
		t.Fatalf("the message is shown as %q", message.Text())
	}
	if s.doc.ByID("asking-cancel") == nil || s.doc.ByID("asking-answer") != nil || s.doc.ByID("asking-stop") != nil ||
		!s.doc.Focused(s.doc.ByID("asking-ok")) {
		t.Fatal("a confirm has no Cancel, a field, a box to stop it, or its OK lacks the focus")
	}
	s.clean()
	_, err := s.session().scripts.rt.Evaluate("1")
	if err == nil || !s.session().scripts.rt.Asking() {
		t.Fatal("the page's scripts run while it asks")
	}
	s.clickAt(s.inPage("more"))
	for range 10 {
		s.frame()
		time.Sleep(2 * time.Millisecond)
	}
	s.click("asking-ok")
	s.told("true")
	if got := s.within("", "window.clicks"); got != "0" {
		t.Fatalf("the page under the dialog heard %s clicks", got)
	}
	before := s.within("", "window.ticks")
	for range 5 {
		s.frame()
		time.Sleep(6 * time.Millisecond)
	}
	if after := s.within("", "window.ticks"); after == before {
		t.Fatalf("after the answer the page's timers stand at %s", after)
	}
	for _, c := range []struct {
		asks string
		act  func()
		want string
	}{
		{`say(confirm("sure?"))`, func() { s.click("asking-cancel") }, "false"},
		{`say(confirm("sure?"))`, func() { s.press(input.KeyEscape) }, "false"},
		{`say(confirm("sure?"))`, func() { s.press(input.KeyEnter) }, "true"},
		{`say(confirm())`, func() { s.click("asking-ok") }, "true"},
		{`say(alert("hello"))`, func() { s.click("asking-ok") }, "undefined"},
		{`say(alert("hello"))`, func() { s.press(input.KeyEscape) }, "undefined"},
		{`say(prompt("name?", "nobody"))`, func() { s.click("asking-ok") }, `"nobody"`},
		{`say(prompt("name?", "nobody"))`, func() { s.click("asking-cancel") }, "null"},
		{`say(prompt("name?", "nobody"))`, func() { s.press(input.KeyEscape) }, "null"},
		{`say(prompt("name?"))`, func() { s.click("asking-ok") }, `""`},
		{`say(prompt("name?"))`, func() {
			s.doc.SetValue(s.doc.ByID("asking-answer"), "Aldric")
			s.press(input.KeyEnter)
		}, `"Aldric"`},
		{`say([alert("one"), confirm("two")])`, func() {
			s.click("asking-ok")
			s.until("the page asked again", func() bool {
				n := s.doc.ByID("asking-message")
				return n != nil && n.Text() == "two"
			})
			s.click("asking-ok")
		}, `[null,true]`},
	} {
		s.has(c.asks)
		kind := strings.Contains(c.asks, "alert")
		if (s.doc.ByID("asking-cancel") == nil) != kind && !strings.Contains(c.asks, "one") {
			t.Fatalf("%s shows a Cancel: %v", c.asks, s.doc.ByID("asking-cancel") != nil)
		}
		if field := s.doc.ByID("asking-answer"); field != nil && !s.doc.Focused(field) {
			t.Fatalf("the field of %s lacks the focus", c.asks)
		}
		s.clean()
		c.act()
		s.told(c.want)
	}
	long := s.has(`say(alert("x".repeat(5000)))`)
	if got := len([]rune(s.doc.ByID("asking-message").Text())); got != askedMost+1 || long == nil {
		t.Fatalf("a message of 5000 characters is shown as %d", got)
	}
	s.clean()
	s.click("asking-ok")
	s.told("undefined")
}

func TestAPageThatKeepsAskingIsStoppedByItsUser(t *testing.T) {
	s, base := askingPage(t)
	s.has(`window.n = 0; for (;;) { alert("again"); window.n++; }`)
	if s.doc.ByID("asking-stop") != nil {
		t.Fatal("the first question of a page offers to stop it")
	}
	s.click("asking-ok")
	s.until("the page asked again", func() bool { return s.doc.ByID("asking-stop") != nil })
	s.clean()
	s.click("asking-stop")
	s.click("asking-ok")
	s.until("the loop was stopped", func() bool { return s.consoleSays(console.Error, "too long") })
	if s.doc.ByID("asking") != nil || !s.consoleSays(console.Error, "Ignored call to 'alert()'. The page was stopped") {
		t.Fatalf("a page stopped from asking still asks, or the console says\n%s", s.said())
	}
	if got := s.within("", `[alert("more"), confirm("more"), prompt("more")].join()`); got != ",false," {
		t.Fatalf("a page stopped from asking is answered %q", got)
	}
	s.visit(base + "/other")
	s.visit(base + "/")
	s.has(`say(confirm("anew"))`)
	s.click("asking-ok")
	s.told("true")
}

func TestAQuestionIsItsTabs(t *testing.T) {
	s, base := askingPage(t)
	s.has(`say(prompt("name?", "nobody"))`)
	s.doc.SetValue(s.doc.ByID("asking-answer"), "half typed")
	s.browser.Open(base + "/other")
	s.show()
	s.until("the other tab's page shows", func() bool { return s.session().pages[base+"/other"] != nil })
	s.frame()
	if s.doc.ByID("asking") != nil || !s.doc.ByID(tabID(0)).HasClass("asking") || s.doc.ByID(tabID(1)).HasClass("asking") {
		t.Fatal("the question of a tab shows over another, or its tab is not marked")
	}
	if got := s.within("", "window.other"); got != "true" {
		t.Fatalf("the other tab's scripts say %s", got)
	}
	s.clean()
	s.selectTab(0)
	s.until("the question shows again", func() bool { return s.doc.ByID("asking") != nil })
	if got := s.doc.Value(s.doc.ByID("asking-answer")); got != "half typed" {
		t.Fatalf("back in its tab the prompt holds %q", got)
	}
	s.click("asking-ok")
	s.told(`"half typed"`)
	if s.doc.ByID(tabID(0)).HasClass("asking") {
		t.Fatal("an answered tab is still marked")
	}
}

func TestAQuestionEndsWithItsPageItsTabAndItsWindow(t *testing.T) {
	s, base := askingPage(t)
	s.frame()
	goroutines, engines := runtime.NumGoroutine(), script.Live()
	settled := func(what string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for (runtime.NumGoroutine() > goroutines || script.Live() > engines) && time.Now().Before(deadline) {
			s.frame()
			time.Sleep(5 * time.Millisecond)
		}
		if runtime.NumGoroutine() > goroutines || script.Live() > engines || s.doc.ByID("asking") != nil {
			t.Fatalf("after %s %d goroutines and %d engines are left of %d and %d, dialog shown: %v", what,
				runtime.NumGoroutine(), script.Live(), goroutines, engines, s.doc.ByID("asking") != nil)
		}
	}
	s.has(`try { confirm("stay?") } finally { say("went on") }`)
	s.Open(base + "/other")
	s.until("the other page shows", func() bool { return s.session().pages[base+"/other"] != nil })
	settled("the page was left")
	if s.consoleSays(console.Error, "too long") || s.consoleSays(console.Error, "closed") {
		t.Fatalf("leaving a page that asks tells the console\n%s", s.said())
	}
	s.visit(base + "/")
	s.browser.Open(base + "/")
	s.show()
	s.until("the second tab's page shows", func() bool { return s.session().scripts != nil })
	s.has(`alert("in the second tab")`)
	s.closeTab(1)
	s.frame()
	s.frame()
	settled("its tab was closed")
	s.newWindow(base + "/")
	s.frame()
	other := s.window(1)
	other.until("the second window's page runs", func() bool { return other.session().scripts != nil })
	other.has(`alert("in the second window")`)
	s.has(`say(confirm("in the first"))`)
	if other.doc.ByID("asking-message").Text() != "in the second window" ||
		s.doc.ByID("asking-message").Text() != "in the first" {
		t.Fatal("two windows do not each show their own question")
	}
	s.click("asking-ok")
	s.told("true")
	if other.doc.ByID("asking") == nil {
		t.Fatal("answering a window's question answered the other's")
	}
	other.closeWindow()
	s.frame()
	settled("its window was closed")
}

func TestOnlyADocumentOfThePagesOriginAsks(t *testing.T) {
	asks := `<body style="margin:0"><p id="got">none</p><script>
		window.go = () => { document.getElementById("got").textContent = String(confirm("from a frame")); };
		</script>`
	web, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><body style="margin:0">
			<iframe id="same" src="/asks"></iframe><iframe id="other" src="https://other.test/asks"></iframe>
			<iframe id="boxed" sandbox="allow-scripts" src="/asks"></iframe>
			<iframe id="modal" sandbox="allow-scripts allow-modals" src="/asks"></iframe>
			<script>window.ticks = 0; setInterval(() => window.ticks++, 5);</script>`),
		"site.test/asks":  html(asks),
		"other.test/asks": html(asks),
	})
	_ = web
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("same", "other", "boxed", "modal")
	for id, why := range map[string]string{
		"other": "A document of another origin than its tab's page shows no dialog.",
		"boxed": "the 'allow-modals' keyword is not set",
		"modal": "A document of another origin than its tab's page shows no dialog.",
	} {
		s.does(id, `go()`)
		s.frame()
		if got := s.framedText(id); s.doc.ByID("asking") != nil || !strings.Contains(got, "false") ||
			!s.consoleSays(console.Error, why) {
			t.Fatalf("the frame %s asked (dialog %v) and got %q; the console says\n%s", id, s.doc.ByID("asking") != nil, got,
				s.said())
		}
	}
	if _, err := s.frameNamed("same").run.rt.Evaluate(`setTimeout(go, 0), 0`); err != nil {
		t.Fatal(err)
	}
	s.until("the frame asked", func() bool { return s.doc.ByID("asking") != nil })
	if label, _ := s.doc.ByID("asking").Attr("aria-label"); label != "An embedded page at site.test says" {
		t.Fatalf("a frame's question is named %q", label)
	}
	s.clean()
	before := s.framedText("modal")
	for _, held := range []*dom.Runtime{s.session().scripts.rt, s.frameNamed("other").run.rt} {
		if _, err := held.Evaluate("window.ticks = -1"); !errors.Is(err, dom.ErrAsking) {
			t.Fatalf("while a frame asks another document of its tab runs a script: %v", err)
		}
	}
	for range 5 {
		s.frame()
		time.Sleep(6 * time.Millisecond)
	}
	s.click("asking-ok")
	s.until("the frame was answered", func() bool { return strings.Contains(s.framedText("same"), "true") })
	ticks := s.within("", "window.ticks")
	if ticks == "0" || ticks == "-1" || s.framedText("modal") != before {
		t.Fatalf("after the answer the page's timers stand at %s", ticks)
	}
}

func TestTheDialogFollowsTheDarkMode(t *testing.T) {
	s, _ := askingPage(t)
	s.configure(func(c *config.Config) { c.DarkMode = true })
	dialog := s.has(`say(prompt("dark?", "yes"))`)
	if !dialog.HasClass("dark") {
		t.Fatal("in dark mode the dialog is light")
	}
	s.clean()
	s.click("asking-ok")
	s.told(`"yes"`)
}
