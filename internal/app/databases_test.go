package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/idb"
)

// based makes a database of that site as a page's script would: one
// store with an index, holding one record.
func (s *shown) based(site, name string) {
	s.t.Helper()
	var told []idb.Event
	c := s.databasesIn("", site).Client(func(e idb.Event) { told = append(told, e) })
	defer c.Close()
	c.Open(name, 2)
	if len(told) == 0 || told[0].Kind != idb.Upgrade {
		s.t.Fatalf("opening %s told %+v", name, told)
	}
	tx := c.Tx(told[0].Tx)
	pages, err := tx.CreateStore("pages", `"id"`, false)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err = tx.CreateIndex(pages, "by-title", `"title"`, false, false); err != nil {
		s.t.Fatal(err)
	}
	if _, err = tx.Put(pages, idb.Number(1), []byte("a page"), nil, false); err != nil {
		s.t.Fatal(err)
	}
	tx.Commit()
}

func TestTheStorageTabListsTheDatabasesOfThePagesSite(t *testing.T) {
	n := cookied(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.based("http://site.test", "notes")
	s.based("http://other.test", "elsewhere")
	s.toggleTools()
	s.click("dt-tab-storage")
	s.visit("http://site.test/next")
	text := s.node("dt-storage").Text()
	for _, want := range []string{"IndexedDB", "notes", "version 2", "pages", "1 record", "by-title"} {
		if !strings.Contains(text, want) {
			t.Errorf("the tab lacks %q in %q", want, text)
		}
	}
	if strings.Contains(text, "elsewhere") {
		t.Error("the tab lists the databases of another site")
	}
	s.clean()
}

func TestClearingTheDatabasesForgetsThemAndTheirFiles(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.based("http://site.test", "notes")
	s.based("http://site.test\x00http://widget.test", "framed")
	if err := s.bases.Flush(); err != nil {
		t.Fatal(err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, idb.Folder, "*.idb")); len(files) != 2 {
		t.Fatalf("two sites' databases are kept in %v", files)
	}
	s.settings()
	s.click("set-clear-databases")
	if got := s.databasesIn("", "http://site.test").Databases(); len(got) != 0 {
		t.Errorf("cleared, the site has %+v", got)
	}
	if err := s.bases.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, idb.Folder)); !os.IsNotExist(err) {
		t.Errorf("cleared, the folder of the databases is still there: %v", err)
	}
	s.based("http://site.test", "after")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if got := again.databasesIn("", "http://site.test").Databases(); len(got) != 1 || got[0].Name != "after" {
		t.Errorf("the next run has %+v", got)
	}
}

func TestWithNoFolderTheDatabasesAreKeptNowhere(t *testing.T) {
	t.Chdir(t.TempDir())
	s := show(t)
	s.based("http://site.test", "notes")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir("."); len(left) != 0 {
		t.Errorf("a run with no folder left %v", left)
	}
	if got := show(t).databasesIn("", "http://site.test").Databases(); len(got) != 0 {
		t.Errorf("another run with no folder has %+v", got)
	}
}
