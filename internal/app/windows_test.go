package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"weak"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/tabs"
)

// window is light's window i, the first being 0, driven as the first is.
func (s *shown) window(i int) *shown {
	s.t.Helper()
	if i >= len(s.screens) {
		s.t.Fatalf("light has %d windows, none numbered %d", len(s.screens), i)
	}
	screen := s.screens[i]
	return &shown{Screen: screen, t: s.t, win: screen.win, in: screen.win.Input(), frame: s.frame, world: s.world}
}

// closeWindow asks the window to close, as its close button does.
func (s *shown) closeWindow() {
	s.win.RequestClose()
	s.frame()
	s.frame()
}

func TestAShiftClickOpensALinkInAWindowOfItsOwn(t *testing.T) {
	srv, seen := crypt(t)
	s := show(t)
	s.win.SetPosition(geom.V2(100, 80))
	s.win.SetSize(geom.V2(900, 600))
	s.visit(srv.URL + "/")
	s.clickHolding(s.inPage("vault"), input.KeyLeftShift)
	if len(s.screens) != 2 {
		t.Fatalf("light has %d windows after a Shift click", len(s.screens))
	}
	if got, want := s.opened(), "["+srv.URL+"/]"; got != want {
		t.Fatalf("the first window's tabs are %s, want %s", got, want)
	}
	second := s.window(1)
	second.until("the new window's page loaded", func() bool { return second.session().pages[srv.URL+"/vaults"] != nil })
	second.frame()
	if got, want := second.opened(), "["+srv.URL+"/vaults]"; got != want {
		t.Fatalf("the new window's tabs are %s, want %s", got, want)
	}
	second.at(srv.URL + "/vaults")
	if second.browser.Tab().CanBack() || seen.count("/vaults") != 1 {
		t.Errorf("the new window's tab can go back, or its page was fetched %d times", seen.count("/vaults"))
	}
	if got := second.tabNames(); got != "Vaults" {
		t.Errorf("the new window's tab is named %q", got)
	}
	win := second.win
	if win.Size() != geom.V2(900, 600) || win.Position() != geom.V2(100+cascade, 80+cascade) {
		t.Errorf("the new window is %v at %v, want the first's size, %d points down and right of it", win.Size(),
			win.Position(), cascade)
	}
	if !win.OwnTitleBar() || win.MinSize() != s.win.MinSize() || win.Background() != s.win.Background() ||
		win.Closing() != platform.CloseQuitsIfLast || s.win.Closing() != platform.CloseQuitsIfLast {
		t.Error("the new window is not made as the first")
	}
	second.clean()
	s.clean()
}

func TestEachWindowHearsItsOwnInput(t *testing.T) {
	s := show(t)
	s.newWindow(s.config.StartPage)
	s.frame()
	second := s.window(1)
	second.enter("light://guide")
	second.at("light://guide")
	s.at(tabs.Home)
	second.command(input.KeyT)
	if len(second.browser.Tabs) != 2 || len(s.browser.Tabs) != 1 {
		t.Fatalf("after a new tab's keys in the second window, the windows have %d and %d tabs", len(s.browser.Tabs),
			len(second.browser.Tabs))
	}
	s.enter("light://about")
	s.at("light://about")
	second.at(tabs.Home)
	second.rightClick("page")
	if !second.menu.IsOpen() || s.menu.IsOpen() {
		t.Fatal("a right click in the second window opened no menu there, or one in the first")
	}
	second.clean()
}

func TestTheNewWindowKeysOpenOneOnTheStartPage(t *testing.T) {
	s := show(t)
	s.configure(func(c *config.Config) { c.StartPage = "light://about" })
	s.command(input.KeyN)
	if len(s.screens) != 2 {
		t.Fatalf("light has %d windows", len(s.screens))
	}
	second := s.window(1)
	second.at("light://about")
	if len(second.browser.Tabs) != 1 || len(s.browser.Tabs) != 1 {
		t.Fatalf("the windows have %d and %d tabs", len(s.browser.Tabs), len(second.browser.Tabs))
	}
	second.command(input.KeyN)
	if len(s.screens) != 3 || s.window(2).win.Position() != second.win.Position().Add(geom.V2(cascade, cascade)) {
		t.Fatalf("light has %d windows after the keys in the second", len(s.screens))
	}
	second.clean()
}

func TestClosingAWindowLeavesTheOthersAndTheLastEndsLight(t *testing.T) {
	for _, first := range []int{0, 1} {
		srv, _ := crypt(t)
		s := show(t)
		s.visit(srv.URL + "/")
		s.newWindow(srv.URL + "/vaults")
		s.frame()
		closed, left := s.window(first), s.window(1-first)
		left.until("the page loaded", func() bool { return left.session().pages[left.browser.Tab().Current()] != nil })
		closed.closeWindow()
		if len(s.screens) != 1 || s.screens[0] != left.Screen || s.world.ShouldQuit() {
			t.Fatalf("after closing window %d light has %d windows, quitting %v", first, len(s.screens),
				s.world.ShouldQuit())
		}
		if len(closed.sessions) != 0 {
			t.Errorf("the closed window still holds %d tabs' sessions", len(closed.sessions))
		}
		left.Open("light://guide")
		left.frame()
		left.click("back")
		left.until("the window left still works", func() bool { return left.host != nil })
		left.clean()
		left.closeWindow()
		if !s.world.ShouldQuit() {
			t.Fatalf("closing the last window, after window %d, did not end light", first)
		}
	}
}

func TestTheQuitKeysEndLightWhateverItsWindows(t *testing.T) {
	s := show(t)
	s.newWindow(s.config.StartPage)
	s.frame()
	s.window(1).command(input.KeyQ)
	if !s.world.ShouldQuit() {
		t.Fatal("the quit keys did not end light")
	}
}

func TestClosingAWindowsLastTabClosesTheWindowAndTheLastWindowEndsLight(t *testing.T) {
	for name, closeLast := range map[string]func(*shown){
		"the keys":          func(s *shown) { s.command(input.KeyW) },
		"its button":        func(s *shown) { s.click("close-0") },
		"a middle click":    func(s *shown) { s.middleClickAt(s.middle("tab-0")) },
		"the menu of a tab": func(s *shown) { s.rightClick("tab-0"); s.choose("Close tab") },
	} {
		t.Run(name, func(t *testing.T) {
			s := show(t)
			s.newWindow("light://about")
			s.frame()
			second := s.window(1)
			second.newTab()
			second.frame()
			closeLast(second)
			second.frame()
			if len(s.screens) != 2 || len(second.browser.Tabs) != 1 || s.world.ShouldQuit() {
				t.Fatalf("after closing one of two tabs light has %d windows, the second %d tabs, quitting %v",
					len(s.screens), len(second.browser.Tabs), s.world.ShouldQuit())
			}
			closeLast(second)
			second.frame()
			if len(s.screens) != 1 || s.screens[0] != s.Screen || s.world.ShouldQuit() {
				t.Fatalf("after closing the second window's last tab light has %d windows, quitting %v",
					len(s.screens), s.world.ShouldQuit())
			}
			if len(second.sessions) != 0 {
				t.Errorf("the closed window still holds %d tabs' sessions", len(second.sessions))
			}
			s.at(tabs.Home)
			closeLast(s)
			s.frame()
			if !s.world.ShouldQuit() {
				t.Fatal("closing the last tab of the last window did not end light")
			}
		})
	}
}

func TestTheSettingsAreTheSameInEveryWindow(t *testing.T) {
	s := showWith(t, Options{Dir: t.TempDir(), Downloads: t.TempDir()})
	s.newWindow(configPage)
	s.frame()
	second := s.window(1)
	s.click("logo")
	if !s.dark() || !second.dark() || !second.doc.Checked(second.node("set-dark-mode")) {
		t.Fatalf("after the first window went dark: the first %v, the second %v, its setting %v", s.dark(),
			second.dark(), second.doc.Checked(second.node("set-dark-mode")))
	}
	second.click("set-dark-mode")
	if s.dark() || second.dark() || src(s.node("logo")) != logo {
		t.Fatalf("after the second window's setting: the first dark %v with logo %q, the second %v", s.dark(),
			src(s.node("logo")), second.dark())
	}
	second.set("set-start-page", "light://guide")
	s.click("new-tab")
	s.at("light://guide")
	second.configure(func(c *config.Config) { c.Zoom = 150 })
	if s.current().Zoom != 1.5 || second.current().Zoom != 1.5 {
		t.Fatalf("pages load at %v in the first window and %v in the second", s.current().Zoom, second.current().Zoom)
	}
}

func TestTheHistoryAndTheDownloadsAreTheSameInEveryWindow(t *testing.T) {
	srv, _ := crypt(t)
	s := showWith(t, Options{Now: clock(), Downloads: t.TempDir()})
	s.newWindow(historyPage)
	s.frame()
	second := s.window(1)
	if got := second.visits(); len(got) != 0 {
		t.Fatalf("the history starts with %v", got)
	}
	s.visit(srv.URL + "/vaults")
	s.frame()
	if got := strings.Join(second.visits(), "\n"); !strings.Contains(got, "Vaults | "+srv.URL+"/vaults") {
		t.Fatalf("the second window's history lists %q after a visit in the first", got)
	}
	if second.node("page").Children()[0].Text() != visited {
		t.Fatal("the second window left its history page")
	}
	second.click("history-clear")
	s.Open(historyPage)
	s.frame()
	if got := s.visits(); len(got) != 0 {
		t.Fatalf("the first window's history lists %v after the second cleared it", got)
	}

	second.Open(downloadsPage)
	second.frame()
	s.enter(srv.URL + "/paper.pdf")
	s.saved("paper.pdf")
	second.settled()
	if text := second.node("page").Text(); !strings.Contains(text, "paper.pdf") {
		t.Fatalf("the second window's downloads page reads %q", text)
	}
	if got := second.status.Text(); !strings.Contains(got, "paper.pdf") {
		t.Errorf("the second window's bubble says %q of a download of the first", got)
	}
	s.newWindow(downloadsPage)
	s.frame()
	third := s.window(2)
	third.frame()
	if text := third.node("page").Text(); !strings.Contains(text, "paper.pdf") || third.status.Text() != "" {
		t.Fatalf("a window opened later lists %q and says %q of a download that ended before it", text,
			third.status.Text())
	}
	third.clean()
}

func TestAFaviconFetchedInOneWindowShowsInTheOthers(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.until("the icon came", func() bool { return s.icon(srv.URL+"/") != "" })
	s.newWindow(historyPage)
	s.frame()
	second := s.window(1)
	second.frame()
	found := false
	second.node("history-list").Walk(func(n *ui.Node) bool {
		found = found || src(n) == faviconOf(srv.URL+"/")
		return true
	})
	if !found {
		t.Fatal("the second window's history shows no icon for a page the first fetched")
	}
	second.clean()
}

// stalled is a site whose pages never come.
func stalled(t *testing.T) *httptest.Server {
	t.Helper()
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(done) })
	return srv
}

func TestWindowsOpenedAndClosedAreLetGo(t *testing.T) {
	srv, _ := crypt(t)
	held := stalled(t)
	s := show(t)
	var gone []weak.Pointer[ui.Document]
	var screens []weak.Pointer[Screen]
	for range 6 {
		s.newWindow(srv.URL + "/")
		s.frame()
		w := s.window(1)
		w.until("the page loaded", func() bool { return w.session().pages[srv.URL+"/"] != nil })
		w.clickHolding(w.inPage("vault"), input.KeyLeftSuper)
		w.OpenInNewTab(held.URL + "/slow")
		w.frame()
		loading := w.session().loads[held.URL+"/slow"]
		if loading == nil {
			t.Fatal("the slow page is not on its way")
		}
		gone = append(gone, weak.Make(w.doc), weak.Make(w.sessionOf(w.browser.Tabs[0]).pages[srv.URL+"/"].Doc))
		screens = append(screens, weak.Make(w.Screen))
		sessions := w.sessions
		w.closeWindow()
		if len(s.screens) != 1 || len(sessions) != 0 {
			t.Fatalf("after a close light has %d windows, the closed one %d sessions", len(s.screens), len(sessions))
		}
	}
	s.frame()
	left := func() int {
		n := 0
		for _, p := range gone {
			if p.Value() != nil {
				n++
			}
		}
		for _, p := range screens {
			if p.Value() != nil {
				n++
			}
		}
		return n
	}
	for range 40 {
		runtime.GC()
		if left() == 0 {
			break
		}
		s.frame()
		time.Sleep(5 * time.Millisecond)
	}
	if n := left(); n != 0 {
		t.Errorf("%d of the %d screens and Documents of closed windows are still held", n, len(gone)+len(screens))
	}
	s.clean()
}

func TestAClosedWindowsLoadsAreStopped(t *testing.T) {
	held := stalled(t)
	s := show(t)
	s.newWindow(held.URL + "/slow")
	s.frame()
	second := s.window(1)
	second.frame()
	loading := second.session().loads[held.URL+"/slow"]
	if loading == nil {
		t.Fatal("the slow page is not on its way")
	}
	stopped := false
	cancel := loading.cancel
	loading.cancel = func() {
		stopped = true
		cancel()
	}
	second.closeWindow()
	if !stopped {
		t.Fatal("the closed window's load was not stopped")
	}
}

func TestLightSavesOnceWhenItEnds(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.newWindow("light://guide")
	s.frame()
	s.window(1).Open("light://about")
	s.window(1).closeWindow()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, historyFile)
	saved, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(saved), "light://about") {
		t.Fatalf("the history kept is %q: %v", saved, err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); err == nil {
		t.Fatal("light saved a second time")
	}
}

func TestAShiftMiddleClickShowsItsTabWhileAFieldHasTheKeyboard(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.click("address")
	s.in.SetKey(input.KeyLeftShift, true)
	s.middleClickAt(s.inPage("vault"))
	s.in.SetKey(input.KeyLeftShift, false)
	s.frame()
	if got, want := s.opened(), srv.URL+"/ ["+srv.URL+"/vaults]"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
}
