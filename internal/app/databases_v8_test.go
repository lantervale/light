//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"path/filepath"
	"testing"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/idb"
	"github.com/lantervale/light/pkg/tabs"
)

// counts is a document that adds a record to a database of its site,
// then notes how many it holds.
const counts = `<script>
	const open = indexedDB.open("visits", 1);
	open.onupgradeneeded = () => open.result.createObjectStore("seen", { autoIncrement: true });
	open.onsuccess = () => {
		const db = open.result, tx = db.transaction("seen", "readwrite");
		tx.objectStore("seen").add({ at: location.href, bytes: new Blob(["kept"]) });
		tx.oncomplete = () => {
			db.transaction("seen").objectStore("seen").getAll().onsuccess = async (e) => {
				window.count = e.target.result.length + " " + await e.target.result[0].bytes.text();
			};
		};
	};
</script>`

func TestDatabasesAreKeptForTheNextRun(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{"site.test/": html(`<p>counts</p>` + counts)})
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.becomes("", "count", "1 kept")
	s.visit("https://site.test/?again")
	s.becomes("", "count", "2 kept")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, idb.Folder, "*.idb")); len(files) != 1 {
		t.Fatalf("the site's databases are kept in %v", files)
	}
	again := showWith(t, Options{Dir: dir, Transport: transport, Downloads: t.TempDir()})
	again.visit("https://site.test/")
	again.becomes("", "count", "3 kept")

	memory := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	memory.visit("https://site.test/")
	memory.becomes("", "count", "1 kept")
}

// whose is a document that notes the name the database of its site
// holds, then keeps its own there.
const whose = `<script>
	const name = location.search.slice(1);
	try {
		const open = indexedDB.open("who", 1);
		open.onupgradeneeded = () => open.result.createObjectStore("s");
		open.onsuccess = () => {
			const db = open.result, tx = db.transaction("s", "readwrite"), s = tx.objectStore("s");
			s.get("who").onsuccess = (e) => {
				s.put(name, "who");
				tx.oncomplete = () => { db.close(); window.found = String(e.target.result); };
			};
		};
	} catch (e) { window.found = e.name; }
</script>`

func TestTheDatabasesOfASiteAreItsOwnAndApartInAFrameOfAnother(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/":   html(`<iframe id="w" src="https://widget.test/w?under-site"></iframe>` + noted),
		"blog.test/":   html(`<iframe id="w" src="https://widget.test/w?under-blog"></iframe>` + noted),
		"widget.test/": html(`<iframe id="w" src="/w?in-itself"></iframe>` + noted),
		"site.test/boxed": html(`<iframe id="w" sandbox="allow-scripts" src="https://widget.test/w?boxed"></iframe>` +
			noted),
		"site.test/own":      html(`<p>own</p>` + whose),
		"widget.test/w":      html(`<p>widget</p>` + whose),
		"widget.test/direct": html(`<p>direct</p>` + whose),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.configure(func(c *config.Config) { c.ThirdPartyCookies = true })
	found := func(top, frame, want string) {
		t.Helper()
		s.visit(top)
		s.becomes(frame, "found", want)
		s.Open(tabs.Home)
		s.frame()
	}
	found("https://site.test/", "w", "undefined")
	found("https://widget.test/direct?first", "", "undefined")
	found("https://blog.test/", "w", "undefined")
	found("https://site.test/own?own", "", "undefined")
	found("https://site.test/", "w", "under-site")
	found("https://widget.test/direct?first", "", "first")
	found("https://widget.test/", "w", "first")
	found("https://blog.test/", "w", "under-blog")
	found("https://site.test/boxed", "w", "SecurityError")
	found("https://site.test/own?own", "", "own")
	for key, want := range map[string]int{
		"https://widget.test": 1, "https://site.test": 1, "https://site.test\x00https://widget.test": 1,
		"https://blog.test\x00https://widget.test": 1, "https://blog.test": 0,
	} {
		if got := len(s.bases.Site(key).Databases()); got != want {
			t.Errorf("%q has %d databases, want %d", key, got, want)
		}
	}
}

// versioned is a page that opens a database of its site at the version
// its address asks for, and notes what it is told.
const versioned = `<script>
	window.told = [];
	const open = indexedDB.open("shared", Number(location.search.slice(1)));
	open.onupgradeneeded = (e) => {
		told.push("upgrade " + e.oldVersion + " to " + e.newVersion);
		if (e.oldVersion === 0) open.result.createObjectStore("s");
		open.transaction.objectStore("s").put(location.search, "by");
	};
	open.onblocked = (e) => told.push("blocked " + e.oldVersion + " to " + e.newVersion);
	open.onsuccess = () => {
		window.db = open.result;
		told.push("open " + db.version);
		db.onversionchange = (e) => told.push("versionchange " + e.oldVersion + " to " + e.newVersion);
		db.onclose = () => told.push("closed");
		db.transaction("s").objectStore("s").get("by").onsuccess = (e) => told.push("by " + e.target.result);
	};
</script>`

func TestTwoWindowsShareTheDatabasesOfASite(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{"site.test/": html(`<p>versioned</p>` + versioned)})
	one := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	win := Window(nil)
	screen := one.Profile.Screen(win)
	win.Add(screen.Document())
	one.world.AddEntity(win)
	two := &shown{Screen: screen, t: t, win: win, in: win.Input(), frame: one.frame, world: one.world}
	two.frame()

	one.visit("https://site.test/?1")
	one.becomes("", "told.join()", "upgrade 0 to 1,open 1,by ?1")
	two.visit("https://site.test/?2")
	two.becomes("", "told.join()", "blocked 1 to 2")
	one.becomes("", "told.join()", "upgrade 0 to 1,open 1,by ?1,versionchange 1 to 2")
	one.does("", "db.close()")
	two.becomes("", "told.join()", "blocked 1 to 2,upgrade 1 to 2,open 2,by ?2")

	one.settings()
	one.click("set-clear-databases")
	two.becomes("", "told.join()", "blocked 1 to 2,upgrade 1 to 2,open 2,by ?2,closed")
	if got := two.within("", `db.transaction("s")`); got != "throws InvalidStateError" {
		t.Errorf("a database that was cleared gives a transaction: %s", got)
	}
	if got := one.bases.Site("https://site.test").Databases(); len(got) != 0 {
		t.Errorf("cleared, the site has %+v", got)
	}
}
