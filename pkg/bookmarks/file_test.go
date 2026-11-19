package bookmarks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lantervale/light/pkg/bookmarks"
)

func TestAListIsSavedToAFileAndLoadedBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "light", "bookmarks.json")
	l := bookmarks.New()
	l.Add("https://a.test/", "A", "Work", at(9))
	if err := bookmarks.Save(path, l); err != nil {
		t.Fatal(err)
	}
	l.Add("https://b.test/", "B", "", at(10))
	if err := bookmarks.Save(path, l); err != nil {
		t.Fatal(err)
	}
	back, err := bookmarks.Load(path)
	if err != nil || back.Len() != 2 {
		t.Fatalf("loaded %d bookmarks, %v", back.Len(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("saving left %d files beside the bookmarks", len(entries)-1)
	}
}

func TestAMissingFileIsAnEmptyListAndACorruptOneSaysSo(t *testing.T) {
	dir := t.TempDir()
	l, err := bookmarks.Load(filepath.Join(dir, "none.json"))
	if err != nil || l == nil || l.Len() != 0 {
		t.Fatalf("a missing file gives %v, %v", l, err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err = os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err = bookmarks.Load(bad)
	if err == nil || l == nil || l.Len() != 0 {
		t.Fatalf("a corrupt file gives %v, %v", l, err)
	}
}
