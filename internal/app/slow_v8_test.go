//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the bar is checked with the screens that show it and the scripts that run long

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/script"
)

// spinsPage is a page whose scripts run for as long as a test tells
// them to, touching the page all the while.
const spinsPage = `<title>Spins</title><body style="margin:0"><button id="more" style="display:block;height:24px">More
	</button><p id="out">none</p><input id="field" style="display:block"><textarea id="area"></textarea>
	<input id="tick" type="checkbox" style="display:block">
	<script>
	const out = document.getElementById("out");
	window.clicks = 0;
	document.getElementById("more").addEventListener("click", () => window.clicks++);
	window.spin = (ms) => {
		const end = Date.now() + ms;
		for (let n = 0; Date.now() < end; n++) out.textContent = "spinning " + n;
		out.textContent = "ended";
	};
	window.forever = () => { for (;;) out.textContent = "forever"; };
	window.after = () => { out.textContent = "the next task ran"; };
	</script>`

// hasty is a browser that waits little, for tests that wait less.
func hasty() dom.Patience {
	return dom.Patience{After: 30 * time.Millisecond, Notice: 90 * time.Millisecond, Again: 150 * time.Millisecond}
}

func spinningPage(t *testing.T) (*shown, string) {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
		"/":      typed("text/html", spinsPage),
		"/other": typed("text/html", `<title>Other</title><p id="here">other</p>`),
	})
	s := showWith(t, Options{Downloads: t.TempDir(), Patient: true, Patience: hasty()})
	s.visit(srv.URL + "/")
	return s, srv.URL
}

// spins has the shown page's scripts run statements at their next
// frame, and waits for the bar that says they run long.
func (s *shown) spins(statements string) {
	s.t.Helper()
	s.until("the page runs its scripts", func() bool { return s.session().scripts != nil })
	s.until("the page takes a script", func() bool {
		_, err := s.session().scripts.rt.Evaluate(`setTimeout(() => { ` + statements + ` }, 0), 0`)
		if err != nil && !errors.Is(err, dom.ErrBusy) {
			s.t.Fatal(err)
		}
		return err == nil
	})
	s.until("told the page is slow", func() bool { return s.doc.ByID("slow") != nil })
}

func (s *shown) marked(i int) bool { return s.doc.ByID(tabID(i)).HasClass("asking") }

func TestAPageThatIsSlowIsWaitedForOrStopped(t *testing.T) {
	s, _ := spinningPage(t)
	s.spins(`setTimeout(after, 0); forever()`)
	got := s.doc.ByID("slow-says").Text()
	if !strings.HasPrefix(got, "127.0.0.1:") || !strings.Contains(got, " is slow") ||
		!s.marked(0) || !s.consoleSays(console.Warn, "A script of this page has run for") {
		t.Fatalf("the bar says %q, the tab marked %v; the console says\n%s", got, s.marked(0), s.said())
	}
	s.clean()
	if _, err := s.session().scripts.rt.Evaluate("1"); err == nil || err.Error() != dom.ErrBusy.Error() {
		t.Fatalf("the console's expression runs in a page whose script still runs: %v", err)
	}
	s.clickAt(s.inPage("more"))
	s.click("slow-wait")
	if s.doc.ByID("slow") != nil || !s.marked(0) {
		t.Fatalf("after Wait the bar stays (%v) or the tab is unmarked", s.doc.ByID("slow") != nil)
	}
	began := time.Now()
	s.until("told again", func() bool { return s.doc.ByID("slow") != nil })
	if waited := time.Since(began); waited < hasty().Again/2 {
		t.Fatalf("the user is asked again after %s", waited)
	}
	s.click("slow-stop")
	s.until("on to the next task", func() bool { return s.pageText("out") == "the next task ran" })
	s.frame()
	if s.doc.ByID("slow") != nil || s.marked(0) || !s.consoleSays(console.Error, script.ErrInterrupted.Error()) {
		t.Fatalf("after Stop the bar stays (%v), the tab marked %v; the console says\n%s", s.doc.ByID("slow") != nil,
			s.marked(0), s.said())
	}
	if heard, err := s.session().scripts.rt.Evaluate("clicks"); err != nil || heard != "1" {
		t.Fatalf("the click made while the script ran was heard %s times: %v", heard, err)
	}
}

func TestTheBarOfASlowPageIsReachedByTheKeys(t *testing.T) {
	s, _ := spinningPage(t)
	s.spins(`spin(500)`)
	s.frame()
	if !s.doc.Focused(s.doc.ByID("slow-wait")) {
		t.Fatal("the bar's Wait does not hold the focus")
	}
	s.press(input.KeyEnter)
	if s.doc.ByID("slow") != nil {
		t.Fatal("Enter on Wait leaves the bar")
	}
	s.until("the script ended", func() bool { return s.pageText("out") == "ended" })

	s.spins(`setTimeout(after, 0); forever()`)
	s.frame()
	s.press(input.KeyTab)
	if !s.doc.Focused(s.doc.ByID("slow-stop")) {
		t.Fatal("Tab does not reach the bar's Stop")
	}
	s.press(input.KeyEnter)
	s.until("on to the next task", func() bool { return s.pageText("out") == "the next task ran" })
}

// tabsTo presses Tab until the element id holds the focus.
func (s *shown) tabsTo(from, id string) {
	s.t.Helper()
	for presses := range 150 {
		if s.doc.Focused(s.doc.ByID(id)) {
			s.t.Logf("from %s Tab reaches %s in %d presses", from, id, presses)
			return
		}
		s.press(input.KeyTab)
	}
	s.t.Fatalf("from %s Tab never reaches %s", from, id)
}

func TestTheBarOfASlowPageTakesNoFocusFromATextBeingTyped(t *testing.T) {
	s, _ := spinningPage(t)
	typed := func(what string) {
		t.Helper()
		s.spins(`setTimeout(after, 0); forever()`)
		s.frame()
		if s.doc.Focused(s.doc.ByID("slow-wait")) {
			t.Fatalf("the bar takes the focus of %s", what)
		}
		s.tabsTo(what, "slow-stop")
		s.press(input.KeyEnter)
		s.until("the script was stopped", func() bool {
			return s.pageText("out") == "the next task ran" && s.doc.ByID("slow") == nil
		})
	}
	s.click("address")
	typed("the address being typed")
	if s.doc.Focused(s.address) {
		t.Fatal("Tab left the focus in the address")
	}
	for _, id := range []string{"field", "area"} {
		s.clickAt(s.inPage(id))
		typed("the page's " + id)
	}
	s.toolsKeys()
	s.until("the tools show", s.toolsShown)
	for _, id := range []string{"dt-eval", "dt-console-filter"} {
		s.click("dt-tab-console")
		s.click(id)
		if !s.doc.Focused(s.doc.ByID(id)) {
			t.Fatalf("a click on %s does not focus it", id)
		}
		typed("the tools' " + id)
	}

	s.clickAt(s.inPage("tick"))
	s.spins(`spin(300)`)
	s.frame()
	if !s.doc.Focused(s.doc.ByID("slow-wait")) {
		t.Fatal("the bar leaves the focus to a checkbox of the page, where nothing is typed")
	}
}

func TestTheBarOfASlowPageIsItsTabsAndItsWindows(t *testing.T) {
	s, base := spinningPage(t)
	s.frame()
	goroutines, engines := runtime.NumGoroutine(), script.Live()
	settled := func(what string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for (runtime.NumGoroutine() > goroutines || script.Live() > engines) && time.Now().Before(deadline) {
			s.frame()
			time.Sleep(5 * time.Millisecond)
		}
		if runtime.NumGoroutine() > goroutines || script.Live() > engines || s.doc.ByID("slow") != nil {
			t.Fatalf("after %s %d goroutines and %d engines are left of %d and %d, bar shown: %v", what,
				runtime.NumGoroutine(), script.Live(), goroutines, engines, s.doc.ByID("slow") != nil)
		}
	}
	s.spins(`forever()`)
	s.browser.Open(base + "/other")
	s.show()
	s.until("the second tab's page shows", func() bool { return s.session().pages[base+"/other"] != nil })
	if s.doc.ByID("slow") != nil || !s.marked(0) || s.marked(1) {
		t.Fatalf("in another tab the bar shows (%v); the tabs are marked %v %v", s.doc.ByID("slow") != nil, s.marked(0),
			s.marked(1))
	}
	s.selectTab(0)
	s.until("the bar is back with its tab", func() bool { return s.doc.ByID("slow") != nil })
	s.selectTab(1)
	s.frame()
	s.closeTab(0)
	s.frame()
	settled("its tab was closed")

	s.visit(base + "/")
	goroutines, engines = runtime.NumGoroutine(), script.Live()
	s.spins(`forever()`)
	s.Open(base + "/other")
	s.until("the other page shows", func() bool { return s.session().pages[base+"/other"] != nil })
	settled("the page was left")
	if s.consoleSays(console.Error, "too long") {
		t.Fatalf("leaving a slow page tells the console\n%s", s.said())
	}

	s.visit(base + "/")
	s.newWindow(base + "/")
	s.frame()
	other := s.window(1)
	other.until("the second window's page runs", func() bool { return other.session().scripts != nil })
	other.spins(`forever()`)
	if s.doc.ByID("slow") != nil || s.marked(0) {
		t.Fatal("a window shows the bar of another's slow page")
	}
	s.spins(`setTimeout(after, 0); forever()`)
	s.click("slow-stop")
	s.until("on to the next task", func() bool { return s.pageText("out") == "the next task ran" })
	if other.doc.ByID("slow") == nil {
		t.Fatal("stopping a window's script stopped the other's")
	}
	other.closeWindow()
	s.frame()
	settled("its window was closed")
}

func TestTheBarOfASlowPageFollowsTheDarkMode(t *testing.T) {
	s, _ := spinningPage(t)
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	s.spins(`spin(300)`)
	if !s.doc.ByID("slow").HasClass("dark") {
		t.Fatal("in dark mode the bar is light")
	}
	s.clean()
	s.until("the script ended", func() bool { return s.pageText("out") == "ended" })
	s.until("the bar goes with its script", func() bool { return s.doc.ByID("slow") == nil && !s.marked(0) })
}

func TestAFramesSlowScriptIsNamedAsItsOwn(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><body style="margin:0"><iframe id="same" src="/spins"></iframe>
			<iframe id="other" src="https://other.test/spins"></iframe>`),
		"site.test/spins":  html(spinsPage),
		"other.test/spins": html(spinsPage),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir(), Patient: true, Patience: hasty()})
	s.visit("https://site.test/")
	s.running("same", "other")
	for id, who := range map[string]string{"same": "site.test", "other": "other.test"} {
		s.until("the frame takes a script", func() bool {
			_, err := s.frameNamed(id).run.rt.Evaluate(`setTimeout(() => spin(300), 0), 0`)
			if err != nil && !errors.Is(err, dom.ErrBusy) {
				t.Fatal(err)
			}
			return err == nil
		})
		s.until("told the frame is slow", func() bool { return s.doc.ByID("slow") != nil })
		if label, _ := s.doc.ByID("slow").Attr("aria-label"); label != "An embedded page at "+who+" is slow" {
			t.Fatalf("the slow script of the frame %s is named %q", id, label)
		}
		s.clean()
		s.until("the frame's script ended", func() bool { return s.framedText(id) != "" && s.doc.ByID("slow") == nil })
	}
}

func TestALightWithNobodyToAskStopsASlowScriptAtItsBudget(t *testing.T) {
	srv, _ := answers(t, map[string]answer{"/": typed("text/html", spinsPage)})
	s := showWith(t, Options{Downloads: t.TempDir(), ScriptBudget: 400 * time.Millisecond, Patience: hasty()})
	s.visit(srv.URL + "/")
	if _, err := s.session().scripts.rt.Evaluate(`setTimeout(forever, 0), setTimeout(after, 0), 0`); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	s.until("on to the next task", func() bool { return s.pageText("out") == "the next task ran" })
	if took := time.Since(began); took < 400*time.Millisecond || s.doc.ByID("slow") != nil ||
		!s.consoleSays(console.Error, script.ErrInterrupted.Error()) || s.consoleSays(console.Warn, "A script of this page") {
		t.Fatalf("the script was stopped after %s, bar shown %v; the console says\n%s", took, s.doc.ByID("slow") != nil,
			s.said())
	}
}
