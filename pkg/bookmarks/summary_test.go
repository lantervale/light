package bookmarks_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/bookmarks"
)

func TestABookmarkKeepsWhatItsPageSaysAndIsReadBackWithIt(t *testing.T) {
	l := bookmarks.New()
	l.Add("https://a.test/", "A", "", at(9))
	if l.Summarise("https://b.test/", "Nothing.") {
		t.Fatal("an address that is not kept was summed up")
	}
	if !l.Summarise("https://a.test/", "  The council chose a ferry.\n") {
		t.Fatal("the bookmark was not summed up")
	}
	if got, _ := l.Lookup("https://a.test/"); got.Summary != "The council chose a ferry." {
		t.Fatalf("the bookmark says %q", got.Summary)
	}
	data, err := l.Encode()
	if err != nil || !strings.Contains(string(data), `"summary":"The council chose a ferry."`) {
		t.Fatalf("written as %s, %v", data, err)
	}
	back, err := bookmarks.Decode(data)
	if got, _ := back.Lookup("https://a.test/"); err != nil || got.Summary != "The council chose a ferry." {
		t.Fatalf("read back it says %q, %v", got.Summary, err)
	}
	l.Summarise("https://a.test/", "")
	if data, _ = l.Encode(); strings.Contains(string(data), "summary") {
		t.Fatalf("a bookmark that says nothing is written as %s", data)
	}
}

func TestABookmarkPlacedInAFolderLeavesTheUsualOneAsItWas(t *testing.T) {
	l := bookmarks.New()
	l.Add("https://a.test/", "A", "Reading list", at(9))
	l.Add("https://b.test/", "B", "", at(10))
	if l.Place("https://c.test/", "News") || fmt.Sprint(l.Folders()) != "[Reading list]" {
		t.Fatalf("an address that is not kept was placed, the folders now %v", l.Folders())
	}
	if !l.Place("https://b.test/", " News ") {
		t.Fatal("the bookmark was not placed")
	}
	if got, _ := l.Lookup("https://b.test/"); got.Folder != "News" || l.Usual() != "Reading list" ||
		fmt.Sprint(l.Folders()) != "[Reading list News]" {
		t.Fatalf("placed in %q, the usual folder %q of %v", got.Folder, l.Usual(), l.Folders())
	}
	l.Place("https://b.test/", "")
	if got, _ := l.Lookup("https://b.test/"); got.Folder != "" || l.Usual() != "Reading list" {
		t.Fatalf("taken out it is in %q, the usual folder %q", got.Folder, l.Usual())
	}
}
