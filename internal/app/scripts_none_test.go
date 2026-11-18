//go:build !(v8 && cgo && (darwin || linux) && (amd64 || arm64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"
)

func TestWithoutAnEngineNoScriptRunsAndLightSaysSo(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/":       typed("text/html", `<p id="p">as sent</p><script src="/app.js"></script>`),
		"/app.js": typed("text/javascript", `document.getElementById("p").textContent = "changed"`),
	})
	s := show(t)
	doc := s.visit(srv.URL + "/")
	if doc.ByID("p").Text() != "as sent" || seen.count("/app.js") != 0 || s.session().scripts != nil {
		t.Fatalf("without an engine the page reads %q, its script asked for %d times", doc.ByID("p").Text(),
			seen.count("/app.js"))
	}
	s.Open(scriptsPage)
	s.frame()
	s.frame()
	page := s.session().pages[scriptsPage]
	if page == nil || page.Doc == nil || !strings.Contains(page.Doc.ByID("none").Text(), "no JavaScript engine") {
		t.Fatal("the scripts page does not say no script ran")
	}
	s.settings()
	if text := s.node("javascript-engine").Text(); !strings.Contains(text, "no JavaScript engine") {
		t.Fatalf("the settings say %q", text)
	}
	s.visit(srv.URL + "/")
	s.toggleTools()
	s.click("dt-tab-console")
	if text := s.node("dt-unscripted").Text(); !strings.Contains(text, "no JavaScript engine") {
		t.Fatalf("the console says %q", text)
	}
	if !s.disabled("dt-eval") {
		t.Error("the console's line takes what no engine would run")
	}
	s.clean()
}
