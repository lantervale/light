//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the dialogs are checked with the screens that show them and the scripts that wait

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/script"
)

// busyPage asks while timers, fetches, messages with its frame, a
// transaction of its database and the load of a module are under way,
// each going on for as long as the page does.
const busyPage = `<title>Busy</title><body style="margin:0"><iframe id="in" src="/inner"></iframe><p id="out">none</p>
	<script>
	window.seen = { ticks: 0, fetched: 0, pings: 0, stored: 0, modules: 0 };
	setInterval(() => seen.ticks++, 1);
	const pull = () => fetch("/data").then((r) => r.text()).then(() => { seen.fetched++; pull(); });
	pull();
	pull();
	addEventListener("message", (e) => { seen.pings++; e.source.postMessage("ping", "*"); });
	const open = indexedDB.open("busy", 1);
	open.onupgradeneeded = () => open.result.createObjectStore("s");
	open.onsuccess = () => {
		const put = () => {
			const tx = open.result.transaction("s", "readwrite");
			tx.objectStore("s").put(seen.stored, "k");
			tx.oncomplete = () => { seen.stored++; put(); };
		};
		put();
	};
	const load = () => import("/mod.js?" + seen.modules).then(() => { seen.modules++; load(); });
	load();
	setTimeout(() => {
		document.getElementById("out").textContent = "asking";
		document.getElementById("out").textContent = "answered " + confirm("busy?");
	}, 40);
	</script>`

func TestAPageThatAsksWhileAllOfItIsUnderWayGoesOnOrEndsWhole(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/":       typed("text/html", busyPage),
		"/data":   typed("text/plain", "data"),
		"/mod.js": typed("text/javascript", `export const made = 1;`),
		"/inner": typed("text/html", `<script>parent.postMessage("ping", "*");
			addEventListener("message", () => parent.postMessage("ping", "*"));</script>`),
		"/other": typed("text/html", `<title>Other</title><p>other</p>`),
	})
	s := showWith(t, Options{Downloads: t.TempDir()})
	s.visit(srv.URL + "/other")
	for range 5 {
		s.frame()
	}
	engines := script.Live()
	seen := func(of *shown) []int {
		t.Helper()
		got, err := of.session().scripts.rt.Evaluate(`Object.values(seen).join()`)
		if err != nil {
			t.Fatal(err)
		}
		var counts []int
		for _, n := range strings.Split(strings.Trim(got, `"`), ",") {
			count, _ := strconv.Atoi(n)
			counts = append(counts, count)
		}
		return counts
	}
	asks := func(of *shown, what string) {
		t.Helper()
		of.until(what+" asked", func() bool { return of.doc.ByID("asking") != nil })
		for range 8 {
			of.frame()
			time.Sleep(time.Millisecond)
		}
		if !of.session().scripts.rt.Asking() || of.pageText("out") != "asking" || waiting() == 0 {
			t.Fatalf("%s shows a dialog its scripts do not wait behind", what)
		}
	}
	for i := range 50 {
		what := fmt.Sprintf("round %d", i)
		switch i % 5 {
		case 0:
			s.visit(srv.URL + "/?" + strconv.Itoa(i))
			asks(s, what)
			s.click("asking-ok")
			before := seen(s)
			s.until(what+": everything went on", func() bool {
				for k, count := range seen(s) {
					if count <= before[k]+1 {
						return false
					}
				}
				return true
			})
			if got := s.pageText("out"); got != "answered true" {
				t.Fatalf("%s: the script went on to %q", what, got)
			}
		case 1:
			s.visit(srv.URL + "/?" + strconv.Itoa(i))
			asks(s, what)
			s.Open(srv.URL + "/other")
		case 2:
			s.browser.Open(srv.URL + "/?" + strconv.Itoa(i))
			s.show()
			s.until(what+": the tab's page runs", func() bool { return s.session().scripts != nil })
			asks(s, what)
			s.closeTab(1)
		case 3:
			s.newWindow(srv.URL + "/?" + strconv.Itoa(i))
			s.frame()
			other := s.window(1)
			other.until(what+": the window's page runs", func() bool { return other.session().scripts != nil })
			asks(other, what)
			other.closeWindow()
		case 4:
			s.visit(srv.URL + "/?" + strconv.Itoa(i))
			asks(s, what)
			s.reloadOrStop()
			s.until(what+": the page was loaded again", func() bool { return s.doc.ByID("asking") == nil })
			asks(s, what)
			s.press(input.KeyEscape)
			s.until(what+": the script went on", func() bool { return s.pageText("out") == "answered false" })
		}
		s.visit(srv.URL + "/other")
		if s.doc.ByID("asking") != nil {
			t.Fatalf("%s leaves a dialog", what)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for (waiting() > 0 || script.Live() > engines) && time.Now().Before(deadline) {
		s.frame()
		time.Sleep(5 * time.Millisecond)
	}
	if waiting() > 0 || script.Live() > engines {
		t.Fatalf("after fifty pages that asked %d of their scripts still wait and %d engines are left of %d", waiting(),
			script.Live(), engines)
	}
}

// waiting counts the goroutines of scripts: those that run one, and
// those that wait for an answer.
func waiting() int {
	stacks := make([]byte, 1<<20)
	stacks = stacks[:runtime.Stack(stacks, true)]
	count := 0
	for _, stack := range strings.Split(string(stacks), "\n\n") {
		if strings.Contains(stack, "internal/dom.(*Runtime).enterScript.func") {
			count++
		}
	}
	return count
}

// groupPage is busyPage with the asking left to a frame of its own
// origin, which shares its engine: the page and another such frame go
// on with their timers, fetches, messages, database and modules, each
// reaching into the others, while that frame asks.
const groupPage = `<title>Group</title><body style="margin:0"><iframe id="asker" src="/asker"></iframe>
	<iframe id="kin" src="/kin"></iframe><p id="out">none</p><p id="beat">0</p>
	<script>
	window.seen = { ticks: 0, fetched: 0, pings: 0, stored: 0, modules: 0, reached: 0, kin: 0 };
	setInterval(() => {
		seen.ticks++;
		document.getElementById("beat").textContent = seen.ticks;
		const kin = document.getElementById("kin").contentWindow;
		if (kin && typeof kin.count === "number") { seen.reached++; seen.kin = kin.count; }
	}, 1);
	const pull = () => fetch("/data").then((r) => r.text()).then(() => { seen.fetched++; pull(); });
	pull();
	pull();
	addEventListener("message", (e) => { seen.pings++; e.source.postMessage("ping", "*"); });
	const open = indexedDB.open("group", 1);
	open.onupgradeneeded = () => open.result.createObjectStore("s");
	open.onsuccess = () => {
		const put = () => {
			const tx = open.result.transaction("s", "readwrite");
			tx.objectStore("s").put(seen.stored, "k");
			tx.oncomplete = () => { seen.stored++; put(); };
		};
		put();
	};
	const load = () => import("/mod.js?" + seen.modules).then(() => { seen.modules++; load(); });
	load();
	</script>`

// group is a window shown for the pages whose frame asks, and the
// engines and documents that ran before any.
type group struct {
	*shown
	base               string
	engines, documents int
}

// seen are the counts the page keeps of what goes on in it.
func (g *group) seen() []int {
	g.t.Helper()
	got, err := g.session().scripts.rt.Evaluate(`Object.values(seen).join()`)
	if err != nil {
		g.t.Fatal(err)
	}
	var counts []int
	for _, n := range strings.Split(strings.Trim(got, `"`), ",") {
		count, _ := strconv.Atoi(n)
		counts = append(counts, count)
	}
	return counts
}

// asks waits for the frame of the page of shows its question, and
// checks that it holds the engine of its kin: nothing of them runs.
func (g *group) asks(of *shown, what string) {
	g.t.Helper()
	of.until(what+" asked", func() bool { return of.doc.ByID("asking") != nil })
	beat := of.pageText("beat")
	for range 8 {
		of.frame()
		time.Sleep(time.Millisecond)
	}
	asker, kin := of.frameNamed("asker"), of.frameNamed("kin")
	switch {
	case asker == nil || asker.run == nil || !asker.run.rt.Asking() || of.session().scripts.rt.Asking():
		g.t.Fatalf("%s shows a dialog the frame's scripts do not wait behind", what)
	case !asker.run.kin.Held() || asker.run.kin != of.session().scripts.kin || kin == nil || kin.run == nil ||
		kin.run.kin != asker.run.kin:
		g.t.Fatalf("%s: the frame that asks does not hold the engine of its kin", what)
	case of.pageText("out") != "asking" || of.pageText("beat") != beat || waiting() == 0:
		g.t.Fatalf("%s: the page went on from beat %s to %s while its frame asked", what, beat, of.pageText("beat"))
	}
}

// goesOn waits for everything of the page to go on: its messages with
// its other frame and what it reads there too, while it has it.
func (g *group) goesOn(what string, kin bool) {
	g.t.Helper()
	before := g.seen()
	g.until(what+": everything went on", func() bool {
		for k, count := range g.seen() {
			if count <= before[k]+1 && (kin || k != 2 && k < 5) {
				return false
			}
		}
		return true
	})
}

// visits shows the page, and waits for its frame to ask.
func (g *group) visits(what string, i int) {
	g.t.Helper()
	g.visit(g.base + "/?" + strconv.Itoa(i))
	g.asks(g.shown, what)
}

// went waits for the script that asked to go on with that answer.
func (g *group) went(what, answer string) {
	g.t.Helper()
	g.until(what+": the script went on", func() bool {
		return strings.HasPrefix(g.pageText("out"), "answered "+answer+" ")
	})
}

// removed takes the frame of that id out of the page as it asks, and
// checks how many documents then run.
func (g *group) removed(what, id string, documents int) {
	g.t.Helper()
	f := g.frameNamed(id)
	f.node.Parent().Remove(f.node)
	for range 5 {
		g.frame()
	}
	if got := script.Contexts() - g.documents; got != documents || f.run != nil {
		g.t.Fatalf("%s: %d documents run once the frame %s was removed (left: %t), want %d", what, got, id, f.run == nil,
			documents)
	}
}

// round has a frame ask, and ends its question one of seven ways.
func (g *group) round(i int) {
	g.t.Helper()
	what := fmt.Sprintf("round %d", i)
	switch i % 7 {
	case 0:
		g.visits(what, i)
		g.click("asking-ok")
		g.went(what, "true")
		g.goesOn(what, true)
	case 1:
		g.visits(what, i)
		for range 30 {
			g.frame()
			time.Sleep(2 * time.Millisecond)
		}
		g.asks(g.shown, what+", later")
		g.press(input.KeyEscape)
		g.went(what, "false")
		g.goesOn(what, true)
	case 2:
		g.visits(what, i)
		g.Open(g.base + "/other")
	case 3:
		g.visits(what, i)
		g.removed(what, "asker", 2)
		if g.doc.ByID("asking") != nil || g.pageText("out") != "asking" {
			g.t.Fatalf("%s: the frame that was removed left its dialog, or its script went on to %q", what, g.pageText("out"))
		}
		g.goesOn(what, true)
	case 4:
		g.visits(what, i)
		g.removed(what, "kin", 3)
		g.click("asking-ok")
		g.went(what, "true")
		g.goesOn(what, false)
		if got := script.Contexts() - g.documents; got != 2 {
			g.t.Fatalf("%s: %d documents run once the frame that was removed could close, want 2", what, got)
		}
	case 5:
		g.browser.Open(g.base + "/?" + strconv.Itoa(i))
		g.show()
		g.until(what+": the tab's page runs", func() bool { return g.session().scripts != nil })
		g.asks(g.shown, what)
		g.closeTab(1)
	case 6:
		g.newWindow(g.base + "/?" + strconv.Itoa(i))
		g.frame()
		other := g.window(1)
		other.until(what+": the window's page runs", func() bool { return other.session().scripts != nil })
		g.asks(other, what)
		other.closeWindow()
	}
	g.visit(g.base + "/other")
	if g.doc.ByID("asking") != nil {
		g.t.Fatalf("%s leaves a dialog", what)
	}
}

func TestAFrameThatAsksWhileItsKinAreUnderWayGoesOnOrEndsWhole(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/":       typed("text/html", groupPage),
		"/data":   typed("text/plain", "data"),
		"/mod.js": typed("text/javascript", `export const made = 1;`),
		"/asker": typed("text/html", `<script>setTimeout(() => {
			const out = parent.document.getElementById("out");
			out.textContent = "asking";
			out.textContent = "answered " + confirm("busy?") + " " + parent.seen.ticks;
		}, 40);</script>`),
		"/kin": typed("text/html", `<script>window.count = 0;
			setInterval(() => { count++; parent.seen.ticks; }, 1);
			const pull = () => fetch("/data").then((r) => r.text()).then(() => { count++; pull(); });
			pull();
			parent.postMessage("ping", "*");
			addEventListener("message", () => parent.postMessage("ping", "*"));</script>`),
		"/other": typed("text/html", `<title>Other</title><p>other</p>`),
	})
	s := showWith(t, Options{Downloads: t.TempDir()})
	s.visit(srv.URL + "/other")
	for range 5 {
		s.frame()
	}
	g := &group{shown: s, base: srv.URL, engines: script.Live(), documents: script.Contexts()}
	for i := range 50 {
		g.round(i)
	}
	deadline := time.Now().Add(5 * time.Second)
	left := func() bool { return waiting() > 0 || script.Live() > g.engines || script.Contexts() > g.documents }
	for left() && time.Now().Before(deadline) {
		s.frame()
		time.Sleep(5 * time.Millisecond)
	}
	if left() {
		t.Fatalf("after fifty pages whose frame asked %d of their scripts still wait, and %d engines and %d documents "+
			"are left of %d and %d", waiting(), script.Live(), script.Contexts(), g.engines, g.documents)
	}
}
