//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"runtime"
	"testing"

	"github.com/lantervale/ember/input"
)

func TestAScriptHearsACommandClickAndMayPreventIt(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Routed</title><style>body { margin: 0 } a { display: block; height: 24px }
			</style><p id="heard">none</p><a id="plain" href="/next">plain</a><a id="held" href="/next">held</a>
			<a id="routed" href="/routed">routed</a><script>
			const heard = document.getElementById("heard");
			document.addEventListener("click", (e) => {
				heard.textContent = e.target.id + (e.metaKey ? " meta" : "") + (e.shiftKey ? " shift" : "") +
					(e.ctrlKey ? " ctrl" : "") + " button " + e.button;
			});
			document.getElementById("held").addEventListener("click", (e) => e.preventDefault());
			document.getElementById("routed").addEventListener("click", (e) => {
				if (e.metaKey || e.ctrlKey) return;
				e.preventDefault();
				history.pushState({}, "", "/routed");
			});</script>`),
		"/next":   typed("text/html", `<title>Next</title><p>There.</p>`),
		"/routed": typed("text/html", `<title>Routed, fetched</title><p>There.</p>`),
	})
	s := show(t)
	s.system = "darwin"
	s.visit(srv.URL + "/")
	s.until("its scripts ran", func() bool { return s.session().scripts != nil })

	s.clickHolding(s.inPage("held"), input.KeyLeftSuper)
	if got := s.pageText("heard"); got != "held meta button 0" {
		t.Fatalf("the page heard %q", got)
	}
	if got, want := s.opened(), "["+srv.URL+"/]"; got != want {
		t.Fatalf("a prevented click left the tabs %s, want %s", got, want)
	}

	s.clickHolding(s.inPage("plain"), input.KeyLeftSuper, input.KeyLeftShift)
	if got, want := s.opened(), srv.URL+"/ ["+srv.URL+"/next]"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	s.click("tab-0")
	if got := s.pageText("heard"); got != "plain meta shift button 0" {
		t.Fatalf("the page heard %q", got)
	}

	s.clickHolding(s.inPage("routed"), input.KeyLeftSuper)
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/next "+srv.URL+"/routed"; got != want {
		t.Fatalf("a router that leaves a Command click alone left the tabs %s, want %s", got, want)
	}
	if s.session().scripts == nil || s.pageText("heard") != "routed meta button 0" {
		t.Fatalf("the shown page lost its scripts, or heard %q", s.pageText("heard"))
	}

	s.middleClickAt(s.inPage("held"))
	if got, want := s.opened(), "["+srv.URL+"/] "+srv.URL+"/next "+srv.URL+"/routed "+srv.URL+"/next"; got != want {
		t.Fatalf("after a middle click the tabs are %s, want %s", got, want)
	}
	if got := s.pageText("heard"); got != "routed meta button 0" {
		t.Fatalf("the page heard a click of the middle button: %q", got)
	}

	if runtime.GOOS == macOS {
		return // a Control click is a context click on a Mac, whatever light's system says (ember's decision 0142)
	}
	s.system = "linux"
	s.clickHolding(s.inPage("routed"), input.KeyLeftControl)
	if got := s.pageText("heard"); got != "routed ctrl button 0" || len(s.browser.Tabs) != 5 {
		t.Fatalf("off macOS the page heard %q, the tabs are %s", got, s.opened())
	}
	s.at(srv.URL + "/")
}

func TestAScriptHearsAShiftClickAndTheWindowItOpensRunsItsOwnScripts(t *testing.T) {
	srv, _ := scripted(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.until("its scripts ran", func() bool { return s.pageText("count") == "0 clicks" })
	s.clickHolding(s.inPage("held"), input.KeyLeftShift)
	if len(s.screens) != 1 {
		t.Fatalf("a prevented Shift click opened a window: light has %d", len(s.screens))
	}
	s.clickHolding(s.inPage("next"), input.KeyLeftShift)
	if len(s.screens) != 2 {
		t.Fatalf("light has %d windows after a Shift click", len(s.screens))
	}
	s.at(srv.URL + "/")
	second := s.window(1)
	second.until("the new window's scripts ran", func() bool {
		return second.session().scripts != nil && second.pageText("visits") == "1 visits"
	})
	s.clickAt(s.inPage("more"))
	if got := s.pageText("count"); got != "1 clicks" || second.pageText("visits") != "1 visits" {
		t.Fatalf("after a click in the first window it reads %q and the second %q", got, second.pageText("visits"))
	}
	live, sessions := second.session().scripts, second.sessions
	second.closeWindow()
	if len(sessions) != 0 || len(s.screens) != 1 {
		t.Fatalf("the closed window holds %d sessions, light %d windows", len(sessions), len(s.screens))
	}
	if _, err := live.rt.Evaluate("1 + 1"); err == nil {
		t.Error("the closed window's scripts still run")
	}
	s.clickAt(s.inPage("more"))
	if got := s.pageText("count"); got != "2 clicks" {
		t.Fatalf("after the other window closed the first reads %q", got)
	}
}
