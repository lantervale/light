package bookmarks_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/bookmarks"
)

func at(hour int) time.Time { return time.Date(2026, time.November, 16, hour, 0, 0, 0, time.UTC) }

func names(marks []bookmarks.Bookmark) string {
	out := make([]string, len(marks))
	for i, b := range marks {
		out[i] = b.Name
	}
	return strings.Join(out, ", ")
}

func TestAPageIsBookmarkedOnceAndFoundByItsAddress(t *testing.T) {
	l := bookmarks.New()
	if _, ok := l.Lookup("https://a.test/"); ok || l.Len() != 0 {
		t.Fatal("an empty list holds a bookmark")
	}
	first := l.Add("https://a.test/", "A", "", at(9))
	again := l.Add("https://a.test/", "A, again", "Work", at(10))
	if l.Len() != 1 || again != first {
		t.Fatalf("bookmarked twice, the list holds %d and gives %+v", l.Len(), again)
	}
	got, ok := l.Lookup("https://a.test/")
	if want := (bookmarks.Bookmark{Name: "A", Address: "https://a.test/", Added: at(9)}); !ok || got != want {
		t.Fatalf("found %+v, want %+v", got, want)
	}
	if _, ok = l.Lookup("https://a.test/other"); ok {
		t.Fatal("another address of the site is found too")
	}
}

func TestABookmarkWithNoNameIsNamedByItsAddress(t *testing.T) {
	l := bookmarks.New()
	if b := l.Add("https://a.test/page", "  ", "", at(9)); b.Name != "https://a.test/page" {
		t.Fatalf("it is named %q", b.Name)
	}
}

func TestABookmarkIsRenamedMovedAndRemoved(t *testing.T) {
	l := bookmarks.New()
	l.Add("https://a.test/", "A", "", at(9))
	l.Add("https://b.test/", "B", "", at(10))
	if !l.Rename("https://a.test/", " The first ") || l.Rename("https://none.test/", "None") {
		t.Fatal("renaming answers wrongly")
	}
	if l.Rename("https://a.test/", " ") {
		t.Fatal("a bookmark took an empty name")
	}
	if !l.Move("https://a.test/", "Reading list") || l.Move("https://none.test/", "Work") {
		t.Fatal("moving answers wrongly")
	}
	got, _ := l.Lookup("https://a.test/")
	if got.Name != "The first" || got.Folder != "Reading list" {
		t.Fatalf("the bookmark is %+v", got)
	}
	if folders := fmt.Sprint(l.Folders()); folders != "[Reading list]" {
		t.Fatalf("moving to a new folder made %s", folders)
	}
	l.Move("https://a.test/", "")
	if got, _ = l.Lookup("https://a.test/"); got.Folder != "" || len(l.Folders()) != 1 {
		t.Fatalf("out of its folder the bookmark is %+v, the folders %v", got, l.Folders())
	}
	if !l.Remove("https://a.test/") || l.Remove("https://a.test/") {
		t.Fatal("removing answers wrongly")
	}
	if got := names(l.All()); got != "B" {
		t.Fatalf("the list holds %s", got)
	}
}

func TestFoldersAreMadeRenamedAndRemoved(t *testing.T) {
	l := bookmarks.New()
	if !l.AddFolder(" Work ") || l.AddFolder("Work") || l.AddFolder("  ") {
		t.Fatal("making a folder answers wrongly")
	}
	l.AddFolder("Reading list")
	l.Add("https://a.test/", "A", "Work", at(9))
	l.Add("https://b.test/", "B", "Reading list", at(10))
	if !l.RenameFolder("Work", "Office") || l.RenameFolder("Office", "Reading list") || l.RenameFolder("None", "X") {
		t.Fatal("renaming a folder answers wrongly")
	}
	if got, _ := l.Lookup("https://a.test/"); got.Folder != "Office" {
		t.Fatalf("the bookmark of a renamed folder is in %q", got.Folder)
	}
	if folders := fmt.Sprint(l.Folders()); folders != "[Office Reading list]" {
		t.Fatalf("the folders are %s", folders)
	}
	if !l.RemoveFolder("Office") || l.RemoveFolder("Office") {
		t.Fatal("removing a folder answers wrongly")
	}
	if got, ok := l.Lookup("https://a.test/"); !ok || got.Folder != "" {
		t.Fatalf("the bookmark of a removed folder is %+v, %v", got, ok)
	}
}

func TestBookmarksAreListedByFolderInTheOrderAdded(t *testing.T) {
	l := bookmarks.New()
	l.AddFolder("Empty")
	l.Add("https://a.test/", "A", "Work", at(9))
	l.Add("https://b.test/", "B", "", at(10))
	l.Add("https://c.test/", "C", "Work", at(11))
	l.Add("https://d.test/", "D", "", at(12))
	var got []string
	for _, g := range l.Groups() {
		got = append(got, g.Folder+": "+names(g.Bookmarks))
	}
	if want := ": B, D | Empty:  | Work: A, C"; strings.Join(got, " | ") != want {
		t.Fatalf("the groups are %q, want %q", strings.Join(got, " | "), want)
	}
	if got := names(l.In("Work")); got != "A, C" {
		t.Fatalf("Work holds %s", got)
	}
	l.Remove("https://b.test/")
	l.Remove("https://d.test/")
	if groups := l.Groups(); len(groups) != 2 || groups[0].Folder != "Empty" {
		t.Fatalf("with every bookmark in a folder the groups are %+v", groups)
	}
}

func TestTheUsualFolderIsTheOneOfTheLastBookmarkFiled(t *testing.T) {
	l := bookmarks.New()
	if l.Usual() != "" {
		t.Fatalf("an empty list has the usual folder %q", l.Usual())
	}
	l.Add("https://a.test/", "A", "Work", at(9))
	l.Add("https://b.test/", "B", "", at(10))
	l.Move("https://b.test/", "Reading list")
	if l.Usual() != "Reading list" {
		t.Fatalf("the usual folder is %q", l.Usual())
	}
	l.RemoveFolder("Reading list")
	if l.Usual() != "" {
		t.Fatalf("a removed folder stays the usual one: %q", l.Usual())
	}
}

func TestAListIsWrittenAndReadBack(t *testing.T) {
	l := bookmarks.New()
	l.AddFolder("Empty")
	l.Add("https://a.test/", "A “quoted”", "Work", at(9))
	l.Add("https://b.test/", "B", "", at(10))
	data, err := l.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := bookmarks.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(back.Groups(), back.Folders(), back.Usual()), fmt.Sprint(l.Groups(), l.Folders(),
		l.Usual()); got != want {
		t.Fatalf("read back %s, want %s", got, want)
	}
}

func TestACorruptListIsAnEmptyOneAndAnError(t *testing.T) {
	for _, data := range []string{"", "{", `{"bookmarks": 3}`, "\x00\x01"} {
		l, err := bookmarks.Decode([]byte(data))
		if err == nil || l == nil || l.Len() != 0 {
			t.Errorf("%q: list %v, err %v", data, l, err)
		}
		l.Add("https://a.test/", "A", "", at(9))
	}
}

func TestAListReadHoldsEachAddressOnceAndTheFoldersOfItsBookmarks(t *testing.T) {
	l, err := bookmarks.Decode([]byte(`{"bookmarks": [
		{"name": "A", "address": "https://a.test/", "folder": "Work", "added": "2026-11-16T09:00:00Z"},
		{"name": "A again", "address": "https://a.test/", "added": "2026-11-16T10:00:00Z"},
		{"name": "No address", "added": "2026-11-16T10:00:00Z"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(l.All()); got != "A" || fmt.Sprint(l.Folders()) != "[Work]" {
		t.Fatalf("the list holds %s in the folders %v", got, l.Folders())
	}
}
