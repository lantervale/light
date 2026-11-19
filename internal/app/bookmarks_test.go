package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// starred reports whether the address field's star is filled.
func (s *shown) starred() bool {
	return src(s.star.Children()[0]) == "star-on.svg" && s.star.HasClass("on")
}

// panelShown reports whether the bookmark's panel is open.
func (s *shown) panelShown() bool { return s.panel.root.HasClass("shown") }

// chips are the folders the open panel offers, a ✓ before the chosen one.
func (s *shown) chips() string {
	var out []string
	for _, chip := range s.node("bookmark-folders").Children() {
		label := chip.Text()
		if pressed, _ := chip.Attr("aria-pressed"); pressed == "true" {
			if len(chip.Children()) == 0 || src(chip.Children()[0]) != "check.svg" {
				s.t.Errorf("the chosen folder %q shows no check", label)
			}
			label = "✓" + label
		}
		out = append(out, label)
	}
	return strings.Join(out, " | ")
}

// bookmarks are the rows of the shown bookmarks page: a folder's heading in
// brackets, then each of its bookmarks as its name and its address.
func (s *shown) bookmarks() string {
	s.t.Helper()
	var out []string
	s.node("bookmarks-list").Walk(func(n *ui.Node) bool {
		switch {
		case n.IsText():
		case n.Tag().String() == "h2":
			out = append(out, "["+n.Text()+"]")
		case n.HasClass("visit"):
			out = append(out, n.Children()[1].Text()+" "+n.Children()[2].Text())
			return false
		}
		return true
	})
	return strings.Join(out, " | ")
}

func TestTheStarBookmarksThePageAtOnceAndOpensThePanel(t *testing.T) {
	srv, _ := crypt(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Now: clock()})
	s.visit(srv.URL + "/")
	if _, hidden := s.star.Attr("hidden"); hidden || s.starred() || src(s.star.Children()[0]) != "star.svg" {
		t.Fatalf("on a page of the web the star is hidden %v, its picture %q", hidden, src(s.star.Children()[0]))
	}
	field, star := s.box("field"), s.box("star")
	if star.X+star.W > field.X+field.W || star.X < s.box("address").X+s.box("address").W {
		t.Fatalf("the star at %v is not at the right end of the field %v", star, field)
	}
	s.click("star")
	kept, ok := s.marks.Lookup(srv.URL + "/")
	if !ok || kept.Name != "A crypt" || kept.Folder != "" || kept.Added.IsZero() {
		t.Fatalf("a click on the star kept %+v, %v", kept, ok)
	}
	if !s.starred() || !s.panelShown() {
		t.Fatalf("after the click the star is filled %v, the panel open %v", s.starred(), s.panelShown())
	}
	if name := s.node("bookmark-name"); s.doc.Value(name) != "A crypt" || !s.doc.Focused(name) {
		t.Fatalf("the panel names the bookmark %q, focused %v", s.doc.Value(name), s.doc.Focused(name))
	}
	if site := s.node("bookmark-site").Text(); site != siteOf(srv.URL) {
		t.Fatalf("the panel names the site %q", site)
	}
	if icon := s.node("bookmark-tile").Children()[0].Children()[0]; src(icon) != faviconOf(srv.URL+"/") {
		t.Fatalf("the tile of a site whose icon light holds shows %q", src(icon))
	}
	panel, notch := s.box("bookmark"), s.panel.notch.Box()
	if centre := notch.Center().X; centre < star.X || centre > star.X+star.W || panel.Y < star.Y+star.H {
		t.Fatalf("the panel %v, its notch %v, does not hang from the star %v", panel, notch, star)
	}
	s.clean()
}

func TestASiteLightHoldsNoIconOfShowsALetter(t *testing.T) {
	srv, _ := answers(t, map[string]answer{"/": typed("text/html", "<title>Bare</title><p>no icon</p>")})
	s := show(t)
	s.visit(srv.URL + "/")
	s.click("star")
	tile := s.node("bookmark-tile").Children()[0]
	if tile.Text() != "1" || tile.Children()[0].Tag().String() == "img" || !strings.Contains(tile.Style(), "background") {
		t.Fatalf("the tile of 127.0.0.1 shows %q, styled %q", tile.Text(), tile.Style())
	}
	s.clean()
}

func TestTheStarIsNotShownOnLightsOwnPages(t *testing.T) {
	s := show(t)
	for _, address := range []string{tabs.Home, tabs.Page(guideName), tabs.Search("tabs")} {
		s.Open(address)
		s.frame()
		if _, hidden := s.star.Attr("hidden"); !hidden {
			t.Errorf("%s shows the star", address)
		}
		if _, shown := s.doc.BoxOf(s.star); shown {
			t.Errorf("%s gives the star a box", address)
		}
		s.command(input.KeyD)
		if s.marks.Len() != 0 || s.panelShown() {
			t.Errorf("%s was bookmarked by the keys", address)
		}
	}
	s.clean()
}

func TestTheKeysBookmarkThePageAndOpenThePanelOfOneKept(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/vaults")
	s.command(input.KeyD)
	if _, ok := s.marks.Lookup(srv.URL + "/vaults"); !ok || !s.panelShown() || !s.starred() {
		t.Fatalf("the keys kept %v, the panel open %v, the star filled %v", ok, s.panelShown(), s.starred())
	}
	s.click("bookmark-done")
	if s.panelShown() || !s.starred() {
		t.Fatalf("after Done the panel is open %v, the star filled %v", s.panelShown(), s.starred())
	}
	s.command(input.KeyD)
	if s.marks.Len() != 1 || !s.panelShown() {
		t.Fatalf("the keys on a page kept: %d bookmarks, the panel open %v", s.marks.Len(), s.panelShown())
	}
	s.visit(srv.URL + "/")
	if s.panelShown() || s.starred() {
		t.Fatalf("on another page the panel is open %v, the star filled %v", s.panelShown(), s.starred())
	}
	s.click("back")
	if !s.starred() {
		t.Fatal("back on the page kept, the star is not filled")
	}
}

func TestThePanelSavesTheNameAndTheFolderAsTheyAreEdited(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	address := srv.URL + "/"
	s.marks.AddFolder("Reading list")
	s.visit(address)
	s.click("star")
	if got := s.chips(); got != "Reading list | + New" {
		t.Fatalf("the folders offered are %q", got)
	}
	s.types(", kept")
	if kept, _ := s.marks.Lookup(address); kept.Name != "A crypt, kept" {
		t.Fatalf("typing named the bookmark %q", kept.Name)
	}
	s.click("bookmark-new")
	field := s.node("bookmark-folder")
	if !s.doc.Focused(field) {
		t.Fatal("the new folder's field has no focus")
	}
	s.types("Work")
	s.press(input.KeyEscape)
	if !s.panelShown() || s.doc.ByID("bookmark-folder") != nil || len(s.marks.Folders()) != 1 {
		t.Fatalf("Escape in the folder's field: the panel open %v, the folders %v", s.panelShown(), s.marks.Folders())
	}
	s.click("bookmark-new")
	s.types("Work")
	s.press(input.KeyEnter)
	if kept, _ := s.marks.Lookup(address); kept.Folder != "Work" {
		t.Fatalf("a new folder left the bookmark in %q", kept.Folder)
	}
	if got := s.chips(); got != "Reading list | ✓Work | + New" {
		t.Fatalf("the folders offered are %q", got)
	}
	s.clickAt(s.node("bookmark-folders").Children()[0].Box().Center())
	if kept, _ := s.marks.Lookup(address); kept.Folder != "Reading list" || s.chips() != "✓Reading list | Work | + New" {
		t.Fatalf("a click on a folder left the bookmark in %q, the folders %q", kept.Folder, s.chips())
	}
	s.clickAt(s.node("bookmark-folders").Children()[0].Box().Center())
	if kept, _ := s.marks.Lookup(address); kept.Folder != "" || s.chips() != "Reading list | Work | + New" {
		t.Fatalf("a click on its folder left the bookmark in %q, the folders %q", kept.Folder, s.chips())
	}
	s.clean()
}

func TestThePanelIsClosedByEscapeDoneAndAPressElsewhereAndRemovesTheBookmark(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	address := srv.URL + "/"
	s.visit(address)
	s.click("star")
	s.press(input.KeyEscape)
	if s.panelShown() || !s.doc.Focused(s.star) || !s.starred() {
		t.Fatalf("after Escape the panel is open %v, the star focused %v and filled %v", s.panelShown(),
			s.doc.Focused(s.star), s.starred())
	}
	s.click("star")
	if !s.panelShown() || s.marks.Len() != 1 {
		t.Fatalf("the star of a page kept: the panel open %v, %d bookmarks", s.panelShown(), s.marks.Len())
	}
	s.press(input.KeyEnter)
	if s.panelShown() {
		t.Fatal("Enter in the name left the panel open")
	}
	s.click("star")
	s.clickAt(s.inPage("words"))
	if s.panelShown() || !s.starred() {
		t.Fatalf("after a press on the page the panel is open %v, the star filled %v", s.panelShown(), s.starred())
	}
	s.click("star")
	s.click("bookmark-remove")
	if s.panelShown() || s.starred() || s.marks.Len() != 0 {
		t.Fatalf("after Remove the panel is open %v, the star filled %v, %d bookmarks", s.panelShown(), s.starred(),
			s.marks.Len())
	}
	s.clean()
}

func TestTheBookmarksPageListsThemByFolderAndOpensAndRemovesOne(t *testing.T) {
	s := show(t)
	s.Open(bookmarksPage)
	s.frame()
	if s.doc.ByID("bookmarks-none") == nil || s.title(bookmarksPage) != "Bookmarks" {
		t.Fatalf("with none kept the page says nothing, or the tab is named %q", s.title(bookmarksPage))
	}
	at := time.Date(2026, time.November, 16, 9, 0, 0, 0, time.UTC)
	s.marks.Add("https://gazette.test/ferry", "Ferry vote", "Local news", at)
	s.marks.Add("https://go.test/doc", "Go documentation", "", at)
	s.marks.Add("light://guide", "The guide", "Local news", at)
	s.remark()
	s.frame()
	s.frame()
	want := "Go documentation go.test/doc | [Local news] | Ferry vote gazette.test/ferry | The guide light://guide"
	if got := s.bookmarks(); got != want {
		t.Fatalf("the page lists %q, want %q", got, want)
	}
	s.clean()
	rows := s.node("bookmarks-list").Children()
	remove := rows[0].Children()
	s.clickAt(remove[len(remove)-1].Box().Center())
	if _, ok := s.marks.Lookup("https://go.test/doc"); ok || strings.Contains(s.bookmarks(), "go.test") {
		t.Fatalf("after removing one the page lists %q", s.bookmarks())
	}
	rows = s.node("bookmarks-list").Children()
	s.clickAt(rows[len(rows)-1].Children()[1].Box().Center())
	s.at("light://guide")
	s.clean()
}

func TestTheBookmarksAreKeptBetweenRunsInTheFolderGiven(t *testing.T) {
	srv, _ := crypt(t)
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Now: clock()})
	s.visit(srv.URL + "/")
	s.click("star")
	s.click("bookmark-new")
	s.types("Work")
	s.press(input.KeyEnter)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, bookmarksFile)); err != nil {
		t.Fatalf("the bookmarks are not written: %v", err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	again.Open(bookmarksPage)
	again.frame()
	if got, want := again.bookmarks(), "[Work] | A crypt "+shownAs(srv.URL+"/"); got != want {
		t.Fatalf("a new run lists %q, want %q", got, want)
	}
}

func TestBookmarksThatCannotBeReadAreNoneAndThePageSaysSo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, bookmarksFile), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.Open(bookmarksPage)
	s.frame()
	if text := s.node("bookmarks-problem").Text(); s.marks.Len() != 0 || !strings.Contains(text, "reading the bookmarks") {
		t.Fatalf("a broken file gives %d bookmarks, and the page says %q", s.marks.Len(), text)
	}
	s.clean()
}

func TestWithNoFolderTheBookmarksStayInMemory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	s := show(t)
	s.marks.Add("https://a.test/", "A", "Work", time.Now())
	s.remark()
	s.frame()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("with no folder given, %d files were written", len(entries))
	}
}

func TestABookmarkMadeInOneWindowShowsInTheOthers(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.newWindow(bookmarksPage)
	s.frame()
	s.frame()
	second := s.screens[1]
	s.click("star")
	s.frame()
	if list := second.doc.ByID("bookmarks-list"); list == nil || !strings.Contains(list.Text(), "A crypt") {
		t.Fatal("the second window's bookmarks page does not list the page the first kept")
	}
	second.marks.Remove(srv.URL + "/")
	second.remark()
	s.frame()
	if s.starred() {
		t.Fatal("the first window's star stays filled once the second removed the bookmark")
	}
}
