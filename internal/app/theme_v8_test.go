//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import "testing"

func TestAScriptTogglingTheRootsClassSwitchesEveryColourItsCustomPropertiesGive(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Switch</title><style>
			:root { --bg: #f7f4ee; --card: #ffffff } :root.dark { --bg: #0b0c0f; --card: #15171b }
			body { margin: 0; background: var(--bg) } .card { background: var(--card) }
			button { display: block; width: 80px; height: 40px }</style>
			<div class="card" id="card"><button id="switch">switch</button><p id="said">none</p></div>
			<script>
			const root = document.documentElement, said = document.getElementById("said");
			function shown() {
				said.textContent = root.className + " " + getComputedStyle(document.body).backgroundColor + " " +
					getComputedStyle(document.getElementById("card")).backgroundColor;
			}
			document.getElementById("switch").addEventListener("click", () => {
				root.classList.toggle("dark", !root.classList.contains("dark"));
				shown();
			});
			shown();</script>`),
	})
	s := show(t)
	s.visit(srv.URL + "/")
	light := " rgb(247, 244, 238) rgb(255, 255, 255)"
	s.until("its script ran", func() bool { return s.pageText("said") == light })
	page := s.session().pages[s.browser.Tab().Current()].Doc
	for _, c := range []struct{ read, drawn string }{
		{"dark rgb(11, 12, 15) rgb(21, 23, 27)", "#0b0c0f #15171b"},
		{light, "#f7f4ee #ffffff"},
	} {
		s.clickAt(s.inPage("switch"))
		if got := s.pageText("said"); got != c.read {
			t.Errorf("the script read %q at once, want %q", got, c.read)
		}
		body, _ := page.ComputedValue(page.Body(), "background-color")
		card, _ := page.ComputedValue(page.ByID("card"), "background-color")
		if got := body + " " + card; got != c.drawn {
			t.Errorf("the page is %s, want %s", got, c.drawn)
		}
	}
	s.clean()
}
