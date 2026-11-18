//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/netlog"
)

func TestWhatAPagesScriptAsksForIsListedAsXHRAndARefusedOneAsBlocked(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Feed</title><p id="said">none</p><script src="/app.js"></script>`),
		"/app.js": typed("text/javascript", `fetch("/items.json").then((r) => r.json()).then((d) => {
				document.getElementById("said").textContent = d.n + " items";
			});
			fetch("/collect", { method: "POST", body: "seen" }).catch(() => {});`),
		"/items.json": typed("application/json", `{"n": 3}`),
	})
	s := show(t)
	s.toggleTools()
	s.visit(srv.URL + "/")
	s.until("the script's answer came", func() bool { return s.pageText("said") == "3 items" })
	var asked, refused netlog.Row
	for _, r := range s.session().log.Rows() {
		switch {
		case strings.HasSuffix(r.Address, "/items.json"):
			asked = r
		case strings.HasSuffix(r.Address, "/collect"):
			refused = r
		}
	}
	if asked.Type != netlog.XHR || asked.Initiator != "app.js:1" || asked.Kind() != "json" || asked.Status != 200 {
		t.Errorf("the script's request is %s by %s, a %s answered %d", asked.Type, asked.Initiator, asked.Kind(),
			asked.Status)
	}
	if !refused.Blocked || refused.Method != http.MethodPost || refused.Type != netlog.XHR ||
		refused.Initiator != "app.js:4" || seen.count("/collect") != 0 {
		t.Errorf("the POST is %+v, sent %d times", refused, seen.count("/collect"))
	}
	s.click("dt-chip-xhr")
	rows := s.node("dt-rows").Children()
	if len(rows) != 3 || rows[1].HasClass("blocked") == rows[2].HasClass("blocked") {
		t.Fatalf("XHR keeps %d rows, or not one of them blocked", len(rows)-1)
	}
	s.clean()
	s.click("dt-tab-console")
	s.click("dt-show-xhr")
	text := s.node("dt-lines").Text()
	for _, want := range []string{"XHR GET " + srv.URL + "/items.json [HTTP 200 OK ", "XHR POST " + srv.URL +
		"/collect [Blocked: light sends no POST for a script]"} {
		if !strings.Contains(text, want) {
			t.Errorf("the console lacks %q in %q", want, text)
		}
	}
	s.clean()
}
