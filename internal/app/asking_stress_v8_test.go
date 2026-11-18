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
