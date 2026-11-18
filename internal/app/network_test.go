package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/tabs"
)

// requests are the shown tab's requests, each as its status, its path
// and what asked for it.
func (s *shown) requests() []string {
	var out []string
	for _, r := range s.session().log.Rows() {
		path := r.Address
		if _, rest, ok := strings.Cut(strings.TrimPrefix(r.Address, "http://"), "/"); ok {
			path = "/" + rest
		}
		out = append(out, http.StatusText(r.Status)+" "+path+" by "+r.Initiator)
	}
	return out
}

func (s *shown) toolsKeys() {
	if runtime.GOOS == "darwin" {
		s.in.SetKey(input.KeyLeftAlt, true)
		s.command(input.KeyI)
		s.in.SetKey(input.KeyLeftAlt, false)
	} else {
		s.press(input.KeyF12)
	}
	s.frame()
}

func (s *shown) toolsShown() bool {
	_, ok := s.doc.BoxOf(s.node("devtools"))
	return ok
}

func TestARedirectedPageIsAtTheAddressThatAnswered(t *testing.T) {
	srv, _ := crypt(t)
	s := showWith(t, Options{Now: clock()})
	s.visit(srv.URL + "/")
	s.visit(srv.URL + "/old")
	s.at(srv.URL + "/vaults")
	if got := s.title(srv.URL + "/vaults"); got != "Vaults" {
		t.Fatalf("the tab is named %q", got)
	}
	s.click("back")
	s.at(srv.URL + "/")
	if s.disabled("forward") {
		t.Fatal("the redirected page is not ahead")
	}
	s.click("forward")
	s.at(srv.URL + "/vaults")
	moves := slices.DeleteFunc(s.requests(), func(r string) bool { return !strings.HasSuffix(r, "by navigation") })
	if got, want := strings.Join(moves, ", "),
		"Found /old by navigation, OK /vaults by navigation"; got != want {
		t.Fatalf("the requests are %s, want %s", got, want)
	}
	s.Open(historyPage)
	s.frame()
	if got := s.visits(); len(got) != 2 || !strings.HasSuffix(got[0], "Vaults | "+srv.URL+"/vaults") {
		t.Fatalf("the history lists %v", got)
	}
}

func TestAnAnswerWithNothingToShowLeavesTheTabWhereItWas(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/vaults")
	s.enter(srv.URL + "/nothing")
	s.until("the answer landed", func() bool { return s.status.Text() != "" && !s.loading() })
	s.at(srv.URL + "/vaults")
	if got := s.status.Text(); got != "127.0.0.1 answered 204 No Content: nothing to show" {
		t.Fatalf("the bubble says %q", got)
	}
	if s.host == nil || !s.disabled("forward") {
		t.Fatal("the page it was on is not shown, or the empty one is ahead")
	}

	s.OpenInNewTab(srv.URL + "/nothing")
	s.until("a first load landed", func() bool { return s.node("page").HasClass("blank") })
	s.at(srv.URL + "/nothing")
	s.clean()
}

func TestAnAnswerThatAsksToSignInSaysItIsNotDone(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.enter(srv.URL + "/sign-in")
	s.until("the page says why", func() bool {
		text := s.node("page").Text()
		return strings.Contains(text, "can't be reached") && strings.Contains(text, "401") && strings.Contains(text,
			"sign")
	})
	s.clean()
}

func TestARedirectLoopIsAnErrorPage(t *testing.T) {
	srv, seen := answers(t, map[string]answer{"/a": moved("/b"), "/b": moved("/a")})
	s := show(t)
	s.enter(srv.URL + "/a")
	s.until("the page says why", func() bool { return strings.Contains(s.node("page").Text(), "too many redirects") })
	if n := seen.count("/a") + seen.count("/b"); n > 12 {
		t.Fatalf("%d requests in a loop", n)
	}
}

func TestStopEndsALoadThatIsBeingRedirected(t *testing.T) {
	held := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-held:
			case <-r.Context().Done():
			}
			return
		}
		http.Redirect(w, r, "/slow", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(held) })
	s := show(t)
	s.enter(srv.URL + "/")
	s.until("the redirect was followed", func() bool { return len(s.session().log.Rows()) == 2 })
	s.click("reload")
	if page := s.session().pages[srv.URL+"/"]; page == nil || !errors.Is(page.Err, errStopped) {
		t.Fatalf("the page is %+v", page)
	}
	s.until("the request is cancelled", func() bool {
		rows := s.session().log.Rows()
		return rows[1].Done && rows[1].Failed()
	})
	s.at(srv.URL + "/")
}

func TestTheDeveloperToolsAreEachTabsAndToggledByTheirKeys(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	if s.toolsShown() {
		t.Fatal("the tools show before they are asked for")
	}
	s.toolsKeys()
	if !s.toolsShown() {
		t.Fatal("the keys do not open the tools")
	}
	tools, stage := s.node("devtools").Box(), s.box("stage")
	if tools.H != 280 || tools.Y != stage.Y+stage.H || tools.Y+tools.H != s.win.Size().Y {
		t.Fatalf("the tools at %v are not docked under the page %v", tools, stage)
	}
	s.clean()
	doc := s.visit(srv.URL + "/")
	s.until("the map is drawn", func() bool { return box(t, doc, "map").W == 30 })
	got := strings.Join(s.requests(), ", ")
	for _, want := range []string{"OK / by navigation", "OK /site.css by document", "OK /map.png by picture",
		"OK /favicon.ico by document"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the requests are %s, without %s", got, want)
		}
	}
	if s.doc.ByID(s.requestRow("/site.css")) == nil {
		t.Fatal("the sheet's request has no row")
	}
	s.clean()

	s.command(input.KeyT)
	if s.toolsShown() || len(s.requests()) != 0 {
		t.Fatal("a new tab shows the tools, or the requests, of another")
	}
	s.click("tab-0")
	if !s.toolsShown() || len(s.requests()) == 0 {
		t.Fatal("the first tab lost its tools or its requests")
	}
	s.click("dt-close")
	s.frame()
	if s.toolsShown() {
		t.Fatal("the close button leaves the tools open")
	}
	s.rightClickAt(s.inPage("space"))
	s.choose("Developer tools")
	if !s.toolsShown() {
		t.Fatal("the menu does not open the tools")
	}
}

func TestTheRequestsOfAPageGoAtTheNextUnlessPreserved(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.toggleTools()
	s.visit(srv.URL + "/vaults")
	s.visit(srv.URL + "/notes.txt")
	if got := strings.Join(s.requests(), ", "); got != "OK /notes.txt by navigation" {
		t.Fatalf("after a second page the requests are %s", got)
	}
	s.click("dt-preserve")
	s.visit(srv.URL + "/app.js")
	if got := strings.Join(s.requests(), ", "); got != "OK /notes.txt by navigation, OK /app.js by navigation" {
		t.Fatalf("with the log preserved the requests are %s", got)
	}
	s.Open(tabs.Home)
	s.frame()
	s.clean()
}

func TestF12OpensAndClosesTheDeveloperToolsOnEverySystem(t *testing.T) {
	s := showWith(t, Options{Now: clock()})
	s.press(input.KeyF12)
	s.frame()
	if !s.toolsShown() {
		t.Fatal("F12 opened no tools")
	}
	s.press(input.KeyF12)
	s.frame()
	if s.toolsShown() {
		t.Fatal("F12 left the tools open")
	}
}
