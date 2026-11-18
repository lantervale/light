package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/tabs"
)

// set gives the setting's field id a value, as typed and entered.
func (s *shown) set(id, value string) {
	s.t.Helper()
	s.click(id)
	s.doc.SetValue(s.node(id), value)
	s.press(input.KeyEnter)
}

// settings opens the settings' page.
func (s *shown) settings() {
	s.t.Helper()
	s.Open(configPage)
	s.frame()
}

func TestTheSettingsAreReachedByTheirTileTheirKeysAndTheMenu(t *testing.T) {
	s := show(t)
	s.click("go-config")
	s.at(configPage)
	s.Open(tabs.Page(guideName))
	s.frame()
	s.command(input.KeyComma)
	s.at(configPage)
	s.Open(tabs.Page(guideName))
	s.frame()
	s.rightClickAt(s.node("page").Children()[0].Box().Center())
	s.choose("Settings")
	s.at(configPage)
}

func TestASettingIsAppliedAtOnceAndKeptBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.settings()
	if s.doc.ByID("set-save") != nil {
		t.Fatal("the page has a Save button: a setting is applied as it changes")
	}
	s.set("set-start-page", "light://guide")
	s.click("new-tab")
	s.at(tabs.Page(guideName))
	s.Open(tabs.Page(aboutName))
	s.click("home")
	s.at(tabs.Page(guideName))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	kept, err := config.Load(filepath.Join(dir, configFile))
	if err != nil || kept.StartPage != "light://guide" {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	again.at(tabs.Page(guideName))
	again.settings()
	if got := again.doc.Value(again.node("set-start-page")); got != "light://guide" {
		t.Fatalf("the field shows %q", got)
	}
	again.set("set-start-page", "example.test")
	if got := again.config.StartPage; got != "https://example.test" {
		t.Fatalf("a typed host name is kept as %q", got)
	}
	again.click("set-reset")
	if again.config != config.Default() || again.doc.Value(again.node("set-start-page")) != tabs.Home {
		t.Fatalf("reset left %+v, the field %q", again.config, again.doc.Value(again.node("set-start-page")))
	}
	again.clean()
}

func TestABadValueIsBroughtBackAndACorruptFileIsLeftUntilASettingChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFile)
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.settings()
	if s.config != config.Default() || !strings.Contains(s.node("config-problem").Text(), "not one light wrote") {
		t.Fatalf("a corrupt file gives %+v", s.config)
	}
	s.set("set-user-agent", "")
	if got := s.doc.Value(s.node("set-user-agent")); got != config.Default().UserAgent {
		t.Fatalf("an empty User-Agent is shown as %q", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{broken" {
		t.Fatalf("the corrupt file was rewritten with nothing changed: %q", data)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	again.settings()
	again.set("set-timeout", "100000")
	if got := again.doc.Value(again.node("set-timeout")); again.config.Timeout != config.MaxTimeout || got != "300" {
		t.Fatalf("a timeout past its bounds is kept as %d, shown as %q", again.config.Timeout, got)
	}
	again.set("set-timeout", "12")
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	if kept, err := config.Load(path); err != nil || kept.Timeout != 12 {
		t.Fatalf("after a change the file holds %+v, %v", kept, err)
	}
}

func TestTheDownloadsFolderIsTheSettingUnlessTheRunIsGivenOne(t *testing.T) {
	srv, _ := crypt(t)
	usual, chosen := t.TempDir(), t.TempDir()
	s := showWith(t, Options{DefaultDownloads: usual})
	s.saves.Start(t.Context(), srv.URL+"/paper.pdf", "")
	s.settled()
	s.settings()
	s.set("set-downloads", chosen)
	s.saves.Start(t.Context(), srv.URL+"/paper.pdf", "")
	s.settled()
	for _, dir := range []string{usual, chosen} {
		if _, err := os.Stat(filepath.Join(dir, "paper.pdf")); err != nil {
			t.Errorf("no file in %s: %v", dir, err)
		}
	}
	given := t.TempDir()
	run := showWith(t, Options{Downloads: given, DefaultDownloads: usual})
	run.settings()
	run.set("set-downloads", chosen)
	run.saves.Start(t.Context(), srv.URL+"/notes.txt", "")
	run.settled()
	if run.downloaded("notes.txt") == "" || !strings.Contains(run.node("downloads-given").Text(), given) {
		t.Fatalf("a run given a folder saved elsewhere, or does not say so: %q", run.node("page").Text())
	}
}

func TestTheHistoryIsKeptOrNotCappedAndCleared(t *testing.T) {
	s := show(t)
	for _, name := range []string{guideName, examplesName, aboutName} {
		s.Open(tabs.Page(name))
	}
	s.settings()
	s.set("set-history-limit", "100")
	if s.Screen.visits.Len() != 4 {
		t.Fatalf("%d visits", s.Screen.visits.Len())
	}
	s.click("set-keep-history")
	s.Open(tabs.Page(guideName))
	if s.config.KeepHistory || s.Screen.visits.Len() != 4 {
		t.Fatalf("with the history off a visit was noted: %d", s.Screen.visits.Len())
	}
	s.settings()
	s.click("set-clear-history")
	if s.Screen.visits.Len() != 0 {
		t.Fatalf("cleared, %d visits are left", s.Screen.visits.Len())
	}
	s.click("set-keep-history")
	s.Open(tabs.Page(guideName))
	if s.Screen.visits.Len() != 1 {
		t.Fatalf("with the history on again %d visits", s.Screen.visits.Len())
	}
}

func TestTheZoomIsTheSettingAndEachTabsOwnByItsKeys(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	doc := s.visit(srv.URL + "/vaults")
	s.OpenInNewTab(configPage)
	s.frame()
	s.click("set-zoom")
	s.press(input.KeyRight)
	if got := s.node("zoom-shown").Text(); doc.Zoom() != 1.5 || got != "150 %" {
		t.Fatalf("a page already loaded is at %v, the setting reads %q", doc.Zoom(), got)
	}
	if other := s.visit(srv.URL + "/"); other.Zoom() != 1.5 {
		t.Fatalf("a page loaded after is at %v", other.Zoom())
	}
	s.selectTab(0)
	s.frame()
	s.doc.Focus(nil)
	s.command(input.KeyEqual)
	if doc.Zoom() != 1.6 || s.status.Text() != "Zoom 160 %" {
		t.Fatalf("zoomed in, the page is at %v, the bubble says %q", doc.Zoom(), s.status.Text())
	}
	s.command(input.KeyMinus)
	s.command(input.KeyMinus)
	if doc.Zoom() != 1.4 {
		t.Fatalf("zoomed out twice, the page is at %v", doc.Zoom())
	}
	if again := s.visit(srv.URL + "/notes.txt"); again == nil && s.session().zoom != 140 {
		t.Fatalf("the tab forgot its zoom: %d", s.session().zoom)
	}
	s.visit(srv.URL + "/vaults")
	s.command(input.Key0)
	if doc.Zoom() != 1.5 {
		t.Fatalf("reset, the page is at %v, not the setting", doc.Zoom())
	}
	s.clean()
}

func TestThePageSettingsHoldForThePagesLoadedAfter(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/":        typed("text/html", `<title>Here</title><img src="/map.png" width="30" height="20">`),
		"/go":      typed("text/html", `<title>Go</title><meta http-equiv="refresh" content="0; url=/">`),
		"/old":     moved("/"),
		"/map.png": typed("image/png", string(picture(t, 30, 20))),
	})
	s := show(t)
	s.settings()
	s.click("set-pictures")
	s.click("set-meta-refresh")
	s.set("set-redirects", "0")
	s.visit(srv.URL + "/")
	for range 20 {
		s.frame()
	}
	if seen.count("/map.png") != 0 {
		t.Fatal("a picture was asked for with pictures off")
	}
	s.visit(srv.URL + "/go")
	s.at(srv.URL + "/go")
	s.visit(srv.URL + "/old")
	if text := s.node("page").Text(); !strings.Contains(text, "too many redirects") {
		t.Fatalf("a redirect with none allowed shows %q", text)
	}
}

func TestRequestsAreSentAsSetAndGivenUpWhenTheyLast(t *testing.T) {
	var agent atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent.Store(r.UserAgent())
		if r.URL.Path == "/never" {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("<title>Here</title>"))
	}))
	t.Cleanup(srv.Close)
	s := show(t)
	s.settings()
	s.set("set-user-agent", "Probe/2.0")
	s.set("set-timeout", "1")
	s.visit(srv.URL + "/")
	if got := agent.Load(); got != "Probe/2.0" {
		t.Fatalf("the server was asked by %q", got)
	}
	s.visit(srv.URL + "/never")
	if text := s.node("page").Text(); !strings.Contains(text, "can't be reached") {
		t.Fatalf("a page that never answers shows %q", text)
	}
}

func TestTheDeveloperToolsStartAsSet(t *testing.T) {
	s := show(t)
	s.settings()
	s.click("set-preserve-log")
	s.set("set-requests", "100")
	if got := s.session().log.Limit(); got != 100 || s.session().tools.Preserve {
		t.Fatalf("the open tab keeps %d requests, preserving %v", got, s.session().tools.Preserve)
	}
	s.click("new-tab")
	if got := s.session().log.Limit(); got != 100 || !s.session().tools.Preserve {
		t.Fatalf("a new tab keeps %d requests, preserving %v", got, s.session().tools.Preserve)
	}
}

func TestTheAboutPageSaysWhereTheFilesAreKept(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.Open(tabs.Page(aboutName))
	s.frame()
	text := s.node("kept").Text()
	for _, name := range []string{historyFile, downloadsFile, configFile} {
		if !strings.Contains(text, filepath.Join(dir, name)) {
			t.Errorf("%s is not named: %q", name, text)
		}
	}
	none := show(t)
	none.Open(tabs.Page(aboutName))
	none.frame()
	if text = none.node("kept").Text(); !strings.Contains(text, "Nothing is kept") {
		t.Fatalf("with no folder the page says %q", text)
	}
}
