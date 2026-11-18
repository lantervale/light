package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/storage"
)

func TestEachSiteHasItsOwnArea(t *testing.T) {
	store := storage.New()
	site, other := store.Area("https://site.test"), store.Area("https://other.test")
	site.Set("name", "Ann")
	site.Set("count", "2")
	other.Set("name", "Bob")
	if got, ok := site.Get("name"); !ok || got != "Ann" {
		t.Errorf("the site reads %q, %v", got, ok)
	}
	if _, ok := other.Get("count"); ok {
		t.Error("a site reads what another stored")
	}
	if got := strings.Join(site.Keys(), ","); got != "count,name" {
		t.Errorf("the site's keys are %q", got)
	}
	if again := store.Area("https://site.test"); len(again.Keys()) != 2 {
		t.Error("the same site asked again has another area")
	}
	site.Remove("count")
	other.Clear()
	if len(site.Keys()) != 1 || len(other.Keys()) != 0 {
		t.Errorf("after a removal and a clear the areas hold %v and %v", site.Keys(), other.Keys())
	}
}

func TestAnAreaIsCapped(t *testing.T) {
	area := storage.New().Area("https://site.test")
	area.Set("big", strings.Repeat("x", storage.MaxBytes+1))
	if _, ok := area.Get("big"); ok {
		t.Fatal("a value over the cap was stored")
	}
	half := strings.Repeat("x", storage.MaxBytes/2)
	area.Set("a", half)
	area.Set("b", half)
	if _, ok := area.Get("b"); ok {
		t.Fatal("two values over the cap together were stored")
	}
	area.Set("a", "small")
	area.Set("b", half)
	if _, ok := area.Get("b"); !ok {
		t.Fatal("a value that fits once another shrank was refused")
	}
}

func TestWhatIsStoredIsKeptInAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "light", "storage.json")
	store, err := storage.Load(path)
	if err != nil || store.Changed() {
		t.Fatalf("no file reads as %v, changed %v", err, store.Changed())
	}
	store.Area("https://site.test").Set("name", "Ann")
	store.Area("https://empty.test").Clear()
	if !store.Changed() {
		t.Fatal("a set did not change the store")
	}
	if err = store.Save(path); err != nil || store.Changed() {
		t.Fatalf("saving: %v, still changed %v", err, store.Changed())
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "empty.test") {
		t.Errorf("an empty area is written: %s", data)
	}
	again, err := storage.Load(path)
	if got, _ := again.Area("https://site.test").Get("name"); err != nil || got != "Ann" {
		t.Fatalf("read back %q, %v", got, err)
	}
	if err = os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt, err := storage.Load(path)
	if err == nil || len(corrupt.Area("https://site.test").Keys()) != 0 {
		t.Fatalf("a corrupt file reads as %v", err)
	}
}

func TestASiteFramedByAnotherKeepsItsTextsApart(t *testing.T) {
	store := storage.New()
	own := store.Area("https://widget.test")
	underNews := store.Framed("https://news.test", "https://widget.test")
	underBlog := store.Framed("https://blog.test", "https://widget.test")
	own.Set("who", "first party")
	underNews.Set("who", "under news")
	underBlog.Set("who", "under blog")
	areas := map[string]*storage.Area{"first party": own, "under news": underNews, "under blog": underBlog}
	for name, area := range areas {
		if got, _ := area.Get("who"); got != name || len(area.Keys()) != 1 {
			t.Errorf("the area %s reads %q among %v", name, got, area.Keys())
		}
	}
	underNews.Clear()
	if _, ok := own.Get("who"); !ok {
		t.Error("clearing a framed area cleared the site's own")
	}
	underNews.Set("who", "under news")

	path := filepath.Join(t.TempDir(), "storage.json")
	if err := store.Save(path); err != nil {
		t.Fatal(err)
	}
	again, err := storage.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := again.Framed("https://news.test", "https://widget.test").Get("who"); got != "under news" {
		t.Errorf("a framed area came back as %q", got)
	}
	if got, _ := again.Area("https://widget.test").Get("who"); got != "first party" {
		t.Errorf("the site's own area came back as %q", got)
	}
}

func TestAFileOfTheFormerFormKeepsWhatSitesStored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")
	const former = `{"version": 1, "sites": {"https://site.test": {"name": "Ann"}}}`
	if err := os.WriteFile(path, []byte(former), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Area("https://site.test").Get("name"); got != "Ann" {
		t.Fatalf("what a site stored before is read as %q", got)
	}
	if len(store.Framed("https://site.test", "https://site.test").Keys()) != 0 {
		t.Error("a framed area reads what the site's own pages stored")
	}
	store.Area("https://site.test").Set("more", "1")
	if err = store.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"version": 2`) || !strings.Contains(string(data), `"name": "Ann"`) {
		t.Errorf("the file written again is %s", data)
	}
}
