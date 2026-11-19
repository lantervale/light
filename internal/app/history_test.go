package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// clock tells a time an hour later each time it is asked, from the
// evening of a day, so some visits fall on the next.
func clock() func() time.Time {
	at := time.Date(2026, time.November, 16, 21, 30, 0, 0, time.UTC)
	return func() time.Time {
		at = at.Add(time.Hour)
		return at
	}
}

// visits are the rows of the shown history page, each as its time, its
// title and its address.
func (s *shown) visits() []string {
	s.t.Helper()
	var out []string
	s.node("history-list").Walk(func(n *ui.Node) bool {
		if n.HasClass("visit") {
			var cells []string
			for _, c := range n.Children() {
				if !c.HasClass("icon") && !c.HasClass("remove") {
					cells = append(cells, c.Text())
				}
			}
			out = append(out, strings.Join(cells, " | "))
			return false
		}
		return true
	})
	return out
}

func TestTheHistoryListsEveryPageVisitedNewestFirstByDay(t *testing.T) {
	srv, _ := crypt(t)
	s := showWith(t, Options{Now: clock()})
	s.Open("light://guide")
	s.visit(srv.URL + "/")
	s.visit(srv.URL + "/old")
	s.Open(tabs.Home)
	s.Open(historyPage)
	s.frame()
	want := []string{
		"00:30 | Vaults | " + srv.URL + "/vaults",
		"23:30 | A crypt | " + srv.URL + "/",
		"22:30 | The ui guide | light://guide",
	}
	got := s.visits()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the history lists\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var days []string
	for _, n := range s.node("history-list").Children() {
		if !n.HasClass("visit") {
			days = append(days, n.Text())
		}
	}
	if strings.Join(days, ", ") != "Tuesday 17 November 2026, Monday 16 November 2026" {
		t.Fatalf("the days are %v", days)
	}
	if icon := s.node("history-list").Children()[1].Children()[1]; !icon.HasClass("icon") ||
		src(icon) != faviconOf(srv.URL+"/vaults") {
		t.Errorf("a visit to a site shows %q before its title", src(icon))
	}
	s.clean()
}

func TestAVisitToASiteWithNoIconKeepsItsTitleInLineWithTheOthers(t *testing.T) {
	srv, _ := answers(t, map[string]answer{"/": typed("text/html", "<title>Bare</title><p>no icon</p>")})
	s := show(t)
	s.Open("light://guide")
	s.visit(srv.URL + "/")
	s.Open(historyPage)
	s.frame()
	rows := s.node("history-list").Children()
	bare, guide := rows[1].Children(), rows[2].Children()
	if bare[2].Text() != "Bare" || bare[2].Box().X != guide[2].Box().X {
		t.Fatalf("%q starts at %v, %q at %v", bare[2].Text(), bare[2].Box().X, guide[2].Text(), guide[2].Box().X)
	}
	s.clean()
}

func TestAVisitOpensAndIsRemovedAndTheHistoryIsFilteredAndCleared(t *testing.T) {
	s := showWith(t, Options{Now: clock()})
	for _, page := range []string{"guide", "examples", "about"} {
		s.Open("light://" + page)
	}
	s.Open(historyPage)
	s.frame()
	s.click("history-filter")
	for _, r := range "exam" {
		s.in.AddText(r)
	}
	s.frame()
	s.frame()
	if got := s.visits(); len(got) != 1 || !strings.Contains(got[0], "Examples") {
		t.Fatalf("“exam” keeps %v", got)
	}
	if !s.doc.Focused(s.node("history-filter")) {
		t.Fatal("typing took the focus off the filter")
	}
	for range 4 {
		s.press(input.KeyBackspace)
	}
	if got := s.visits(); len(got) != 3 {
		t.Fatalf("an emptied filter keeps %v", got)
	}
	remove := s.node("history-list").Children()[1].Children()
	r, _ := s.doc.BoxOf(remove[len(remove)-1])
	s.clickAt(r.Center())
	if got := s.visits(); len(got) != 2 || strings.Contains(got[0], "About") {
		t.Fatalf("after removing the newest: %v", got)
	}
	var link *ui.Node
	for _, c := range s.node("history-list").Children()[1].Children() {
		if c.HasClass("link") {
			link = c
		}
	}
	r, _ = s.doc.BoxOf(link)
	s.clickAt(r.Center())
	s.at("light://examples")
	s.Open(historyPage)
	s.frame()
	s.click("history-clear")
	if got := s.visits(); len(got) != 0 || s.doc.ByID("history-none") == nil {
		t.Fatalf("after Clear: %v", got)
	}
	s.clean()
}

func TestTheHistoryIsReachedByTheMenuAndItsKeys(t *testing.T) {
	s := show(t)
	s.click("menu")
	s.click("menu-history")
	s.at(historyPage)
	if got := s.title(historyPage); got != "History" {
		t.Fatalf("the tab is named %q", got)
	}
	s.Open(tabs.Home)
	s.Open("light://search?q=nothing")
	s.frame()
	key := input.KeyH
	if runtime.GOOS == "darwin" {
		key = input.KeyY
	}
	s.command(key)
	s.at(historyPage)
	if got := s.visits(); len(got) != 1 || !strings.Contains(got[0], "nothing - Search") {
		t.Fatalf("the start page or the history is a visit, or a search is none: %v", got)
	}
}

func TestTheHistoryIsKeptBetweenRunsInTheFolderGiven(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Now: clock()})
	s.Open("light://guide")
	s.frame()
	file := filepath.Join(dir, "history.json")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("a visit is not written: %v", err)
	}
	s.Open("light://about")
	s.frame()
	if data, _ := os.ReadFile(file); strings.Contains(string(data), "about") {
		t.Fatal("the history is written at each change, not held back")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again := showWith(t, Options{Dir: dir, Now: clock()})
	again.Open(historyPage)
	again.frame()
	if got := again.visits(); len(got) != 2 || !strings.Contains(got[0], "About") {
		t.Fatalf("a new run lists %v", got)
	}
}

func TestAHistoryThatCannotBeReadIsEmptyAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := showWith(t, Options{Dir: dir})
	s.Open(historyPage)
	s.frame()
	if got := s.visits(); len(got) != 0 {
		t.Fatalf("a broken file lists %v", got)
	}
	if text := s.node("history-problem").Text(); !strings.Contains(text, "reading the history") {
		t.Fatalf("the page says %q", text)
	}
	s.clean()
}

func TestWithNoFolderTheHistoryStaysInMemory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	s := show(t)
	s.Open("light://guide")
	s.frame()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("with no folder given, %d files were written", len(entries))
	}
}
