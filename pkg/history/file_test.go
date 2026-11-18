package history_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lantervale/light/pkg/history"
)

func TestALogIsSavedToAFileAndLoadedBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "light", "history.json")
	l := history.New(10)
	l.Add("https://a.test", "A", at(15, 9))
	if err := history.Save(path, l); err != nil {
		t.Fatal(err)
	}
	l.Add("https://b.test", "B", at(15, 10))
	if err := history.Save(path, l); err != nil {
		t.Fatal(err)
	}
	back, err := history.Load(path, 10)
	if err != nil || back.Len() != 2 {
		t.Fatalf("loaded %d visits, %v", back.Len(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("saving left %d files beside the history", len(entries)-1)
	}
}

func TestAMissingFileIsAnEmptyLogAndACorruptOneSaysSo(t *testing.T) {
	dir := t.TempDir()
	l, err := history.Load(filepath.Join(dir, "none.json"), 10)
	if err != nil || l == nil || l.Len() != 0 {
		t.Fatalf("a missing file gives %v, %v", l, err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err = os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err = history.Load(bad, 10)
	if err == nil || l == nil || l.Len() != 0 {
		t.Fatalf("a corrupt file gives %v, %v", l, err)
	}
}
