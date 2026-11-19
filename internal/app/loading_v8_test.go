//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/script"
)

// The document a tab holds runs until the next one takes its place: its
// timers, its listeners, and what it asks of the tab.
func TestTheScriptsOfThePageHeldRunUntilTheNextPageTakesItsPlace(t *testing.T) {
	open := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/held":
			_, _ = w.Write([]byte(`<title>Held</title><a id="go" href="/slow">go</a><button id="b">b</button>
				<p id="n">0</p><script>
				let n = 0; setInterval(() => { document.getElementById("n").textContent = ++n; }, 1);
				document.getElementById("b").onclick = () => { document.title = "Clicked"; };</script>`))
		case "/slow":
			select {
			case <-open:
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`<title>Next</title><p id="from"></p><script>
				document.getElementById("from").textContent = "` + r.Header.Get("Referer") + `|" + history.length;</script>`))
		default:
			_, _ = w.Write([]byte(`<title>Other</title><p>other</p>`))
		}
	}))
	t.Cleanup(srv.Close)
	engines, documents := script.Live(), script.Contexts()
	s := show(t)
	held, slow := srv.URL+"/held", srv.URL+"/slow"
	s.visit(held)
	s.until("the page's scripts run", func() bool { return s.pageText("n") != "0" })
	routines := runtime.NumGoroutine()
	s.clickAt(s.inPage("go"))
	if s.session().pending == nil || s.browser.Tab().Current() != held {
		t.Fatal("the script's move is not waited for on its page")
	}
	s.at(held)
	was := s.pageText("n")
	s.until("the page held goes on", func() bool { return s.pageText("n") != was })
	s.clickAt(s.inPage("b"))
	if got := s.within("", `document.title + "|" + history.length`); got != "Clicked|2" {
		t.Errorf("the page held says %s", got)
	}
	if got, docs := script.Live()-engines, script.Contexts()-documents; got != 1 || docs != 1 {
		t.Errorf("%d engines run %d documents while the next page loads, want the page's alone", got, docs)
	}
	close(open)
	s.until("the next page shows", func() bool { return s.browser.Tab().Current() == slow && s.pageText("from") != "" })
	if got := s.pageText("from"); got != held+"|3" {
		t.Errorf("the next page was asked for from %s", got)
	}
	if got, docs := script.Live()-engines, script.Contexts()-documents; got != 1 || docs != 1 {
		t.Errorf("%d engines run %d documents once the next page shows", got, docs)
	}

	for _, how := range []string{"stopped", "failed", "replaced"} {
		s.visit(held)
		s.until("the page's scripts run", func() bool { return s.pageText("n") != "0" })
		switch how {
		case "stopped":
			open = make(chan struct{})
			s.Open(slow + "?again")
			s.frame()
			s.stop()
		case "failed":
			s.Open("http://127.0.0.1:1/nothing")
			s.until("the failure shows", func() bool { return !s.loading() })
		case "replaced":
			s.Open("http://127.0.0.1:1/nothing")
			s.visit(srv.URL + "/other")
		}
		for range 5 {
			s.frame()
		}
		want := 1
		if how == "failed" || how == "replaced" {
			want = 0
		}
		if got, docs := script.Live()-engines, script.Contexts()-documents; got != want || docs != want {
			t.Errorf("%s: %d engines run %d documents, want %d", how, got, docs, want)
		}
		if len(s.session().loads) != 0 || s.session().pending != nil {
			t.Errorf("%s: the tab still waits", how)
		}
	}
	s.until("the goroutines of the loads end", func() bool { return runtime.NumGoroutine() <= routines+4 })
	if said := s.said(); strings.Contains(said, "error:") {
		t.Errorf("the console says %s", said)
	}
	s.shut()
	if script.Live() != engines || script.Contexts() != documents {
		t.Errorf("%d engines and %d documents are left", script.Live()-engines, script.Contexts()-documents)
	}
}
