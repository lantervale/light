//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/script"
)

// gives checks what expressions are in the document of the frame from.
func (s *shown) gives(from string, wants map[string]string) {
	s.t.Helper()
	for expression, want := range wants {
		if got := s.within(from, expression); got != want {
			s.t.Errorf("in %q, %s is %s, want %s", from, expression, got, want)
		}
	}
}

func TestADocumentReachesTheDocumentsOfItsOriginInItsFrames(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><iframe id="one" name="first" src="/inner"></iframe>
			<iframe id="quiet" src="/quiet"></iframe><iframe id="blank"></iframe>
			<iframe id="written" srcdoc="<p id=w>written</p>"></iframe><iframe id="shut" sandbox src="/quiet"></iframe>
			<iframe id="boxed" sandbox="allow-scripts" src="/quiet"></iframe>
			<iframe id="far" src="https://other.test/far"></iframe><script>window.mark = "top";</script>`),
		"site.test/inner": html(`<p id="p">inner</p><script>window.mark = "inner";
			window.seen = [frameElement && frameElement.id, parent.mark, top.document.title,
				parent.document === top.document].join();</script>`),
		"site.test/quiet":  html(`<p id="p">quiet</p>`),
		"site.test/second": html(`<p id="p">second</p>`),
		"other.test/far":   html(`<script>window.mark = "far"; window.seen = String(frameElement);</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "one", "far")
	for _, id := range []string{"quiet", "written", "shut", "boxed"} {
		s.framedText(id)
	}
	if s.frameNamed("quiet").run != nil {
		t.Fatal("a document with no script runs some before its kin reached for it")
	}
	s.gives("", map[string]string{
		`one.contentDocument.getElementById("p").textContent + " " + one.contentWindow.mark`:   "inner inner",
		`one.contentWindow.document === one.contentDocument`:                                   "true",
		`frames[0].document === one.contentDocument && first.document === one.contentDocument`: "true",
		`frames["first"] === one.contentWindow && one.contentWindow.frameElement === one`:      "true",
		`frameElement === null && one.contentWindow.Element !== Element`:                       "true",
		`one.contentDocument instanceof one.contentWindow.Document`:                            "true",
		`one.contentDocument.defaultView === one.contentWindow`:                                "true",
		`document.defaultView === window`:                                                      "true",
		`quiet.contentDocument.getElementById("p").textContent`:                                "quiet",
		`quiet.contentWindow.Element !== Element`:                                              "true",
		`quiet.contentWindow.document === quiet.contentDocument`:                               "true",
		`blank.contentDocument.body.childNodes.length`:                                         "0",
		`blank.contentDocument.URL + " " + blank.contentWindow.location.href`:                  "about:blank about:blank",
		`blank.contentWindow.origin`:                                                           "https://site.test",
		`blank.contentDocument === blank.contentDocument`:                                      "true",
		`blank.contentWindow.frameElement === blank`:                                           "true",
		`written.contentDocument.getElementById("w").textContent`:                              "written",
		`String(shut.contentDocument) + String(boxed.contentDocument)`:                         "nullnull",
		`String(far.contentDocument)`:                                                          "null",
		`shut.contentWindow.document`:                                                          "throws SecurityError",
		`boxed.contentWindow.document`:                                                         "throws SecurityError",
		`far.contentWindow.document`:                                                           "throws SecurityError",
		`far.contentWindow.mark`:                                                               "throws SecurityError",
	})
	s.does("", `blank.contentDocument.body.innerHTML = "<b>x</b>";`)
	s.gives("one", map[string]string{`seen`: "one,top,Top,true",
		`parent.frames[1].document.getElementById("p").textContent`: "quiet",
		`parent.frames[2].document.querySelector("b").textContent`:  "x"})
	s.gives("far", map[string]string{`seen`: "null", `parent.document`: "throws SecurityError"})
	if s.frameNamed("quiet").run == nil || s.frameNamed("shut").run != nil {
		t.Error("the document with no script got none for its kin to reach it, or the one that may run none got some")
	}

	s.does("", `window.made = document.createElement("iframe"); made.id = "made"; made.src = "/quiet";
		made.onload = () => { window.loaded = made.contentDocument.getElementById("p").textContent; };
		document.body.append(made);
		window.early = made.contentDocument.URL + " " + made.contentDocument.body.childNodes.length;
		window.first = made.contentDocument;`)
	s.becomes("", `String(window.loaded)`, "quiet")
	s.gives("", map[string]string{`early`: "about:blank 0", `made.contentDocument !== first`: "true"})

	s.does("", `one.src = "/second";`)
	s.framedTextOf("one", "https://site.test/second")
	s.gives("", map[string]string{`one.contentDocument.getElementById("p").textContent`: "second",
		`String(one.contentWindow.mark)`: "undefined"})
	if said := s.said(); strings.Contains(said, "error:") {
		t.Errorf("the console says %s", said)
	}
}

func TestWhatAScriptHeldOfAFrameFollowsWhatTheFrameShows(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="one" src="/inner"></iframe><script>window.mark = "top";</script>`),
		"site.test/inner": html(`<title>Inner</title><p id="p">inner</p><script>window.mark = "inner";
			window.fn = () => "fn of " + document.title; window.plain = { kept: "plain" };</script>`),
		"other.test/far": html(`<p>far</p><script>window.mark = "far";</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "one")
	s.does("", `window.w = frames[0]; window.d = w.document; window.n = d.getElementById("p"); window.f = w.fn;
		window.o = w.plain; window.place = w.location;`)
	s.gives("", map[string]string{`w.mark + " " + d.title + " " + n.textContent + " " + f() + " " + place.pathname`: "" +
		"inner Inner inner fn of Inner /inner"})
	engines, documents := script.Live(), script.Contexts()

	s.does("", `one.src = "https://other.test/far";`)
	s.framedTextOf("one", "https://other.test/far")
	s.running("one")
	gone := "throws Error"
	s.gives("", map[string]string{
		`w === frames[0] && w === one.contentWindow`: "true",
		`w.mark`:                      "throws SecurityError",
		`w.document`:                  "throws SecurityError",
		`w.fn`:                        "throws SecurityError",
		`w.location.href`:             "throws SecurityError",
		`place.href`:                  "throws SecurityError",
		`place === w.location`:        "false",
		`String(one.contentDocument)`: "null",
		`w.postMessage("x", "*")`:     "undefined",
		`Object.getPrototypeOf(w)`:    "null",
		`d.title`:                     gone,
		`d.createElement("p")`:        gone,
		`n.textContent`:               gone,
		`n.remove()`:                  gone,
		`f()`:                         gone,
		`o.kept`:                      "plain",
	})
	if script.Live() != engines+1 || script.Contexts() != documents {
		t.Errorf("a frame gone to another origin leaves %d engines and %d documents, of %d and %d", script.Live(),
			script.Contexts(), engines, documents)
	}

	s.does("", `one.src = "/inner?again";`)
	s.framedTextOf("one", "https://site.test/inner?again")
	s.running("one")
	s.gives("", map[string]string{
		`w === frames[0] && w.mark`:            "inner",
		`w.document !== d && w.document.title`: "Inner",
		`w.fn !== f && w.fn()`:                 "fn of Inner",
		`place.search`:                         "?again",
		`d.title`:                              gone,
		`f()`:                                  gone,
	})
	if script.Live() != engines || script.Contexts() != documents {
		t.Errorf("a frame back at its page's origin leaves %d engines and %d documents, of %d and %d", script.Live(),
			script.Contexts(), engines, documents)
	}
}

func TestFramesOfAPagesOriginComeAndGoAndLeaveNothing(t *testing.T) {
	engines, documents := script.Live(), script.Contexts()
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<p id="said"></p><script>
			window.kept = [];
			window.add = (n) => {
				const frame = document.createElement("iframe");
				frame.id = "made";
				frame.src = "/inner?" + n;
				frame.onload = () => {
					kept.push(frame.contentWindow, frame.contentDocument, frame.contentWindow.tick);
					document.getElementById("said").textContent = frame.contentDocument.title + " " + frame.contentWindow.n;
				};
				document.body.append(frame);
			};
			window.drop = () => { document.getElementById("made").remove(); kept.length = 0; };</script>`),
		"site.test/inner": html(`<title>ready</title><iframe id="deep" src="/deep"></iframe><iframe id="empty"></iframe>
			<script>window.n = location.search.slice(1); window.tick = () => n; setInterval(tick, 5);
			fetch("/slow").catch(() => {}); window.under = document.getElementById("empty").contentDocument;</script>`),
		"site.test/deep": html(`<script>setInterval(() => parent.tick(), 5); window.up = top.document;</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("")
	kin := s.session().scripts.kin
	cycle := func(n int) {
		s.does("", `add(`+itoa(n)+`)`)
		s.until("the frame was reached", func() bool { return s.pageText("said") == "ready "+itoa(n) })
		s.running("made", "deep")
		if got, held := script.Live()-engines, script.Contexts()-documents; got != 1 || held != 4 || kin.Members() != 4 {
			t.Fatalf("round %d: %d engines run %d documents, %d of them kin, want 1, 4 and 4", n, got, held, kin.Members())
		}
		s.does("", `drop()`)
		s.frame()
		s.frame()
		if got, held := script.Live()-engines, script.Contexts()-documents; got != 1 || held != 1 || kin.Members() != 1 {
			t.Fatalf("round %d: %d engines run %d documents, %d of them kin, once the frame was removed: want the page's "+
				"alone", n, got, held, kin.Members())
		}
	}
	cycle(0)
	settle := func() int {
		for range 20 {
			s.frame()
			time.Sleep(5 * time.Millisecond)
		}
		return runtime.NumGoroutine()
	}
	routines := settle()
	for n := 1; n <= 50; n++ {
		cycle(n)
	}
	if after := settle(); after > routines+4 {
		t.Errorf("%d goroutines run after 50 frames came and went, %d before", after, routines)
	}
	if set := s.framesOfPage(); len(set.all) > idleFrames || len(set.mail) != 0 {
		t.Errorf("the page still holds %d frames and %d messages", len(set.all), len(set.mail))
	}
	if said := s.said(); strings.Contains(said, "error:") {
		t.Errorf("the console says %s", said)
	}
	s.shut()
	if script.Live() != engines || script.Contexts() != documents {
		t.Errorf("%d engines and %d documents are left once the window is shut, of %d and %d", script.Live(),
			script.Contexts(), engines, documents)
	}
}

// A browser ends the documents under an iframe the moment the element
// leaves its document: no timer of theirs runs after it.
func TestNoScriptOfAFrameRunsOnceItsElementLeftTheDocument(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><iframe id="made" src="/inner"></iframe>
			<script>window.ticks = 0;</script>`),
		"site.test/inner": html(`<iframe id="deep" src="/deep"></iframe>
			<script>window.tick = () => { top.ticks++; }; setInterval(tick, 0);</script>`),
		"site.test/deep": html(`<script>setInterval(() => parent.tick(), 0);</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "made", "deep")
	s.until("the frames tick", func() bool { return s.within("", `ticks > 2`) == "true" })
	s.does("", `document.getElementById("made").remove(); window.ticks = 0;`)
	for range 5 {
		time.Sleep(2 * time.Millisecond)
		s.frame()
	}
	if got := s.within("", `ticks`); got != "0" {
		t.Errorf("the frames ticked %s times once their element was removed", got)
	}
	if said := s.said(); strings.Contains(said, "error:") {
		t.Errorf("the console says %s", said)
	}
}

func TestAScriptThatNeverEndsIsStoppedWhicheverKinItRunsIn(t *testing.T) {
	const ticking = `<script>window.ticks = 0; setInterval(() => { ticks++; }, 5);</script>`
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="spins" src="/spins"></iframe><iframe id="fine" src="/fine"></iframe>` + ticking),
		"site.test/spins": html(`<script>window.spun = 0; window.spin = () => { spun++; for (;;) {} };
			setTimeout(spin, 30);</script>` + ticking),
		"site.test/fine": html(ticking),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir(), ScriptBudget: 100 * time.Millisecond})
	s.visit("https://site.test/")
	s.running("", "spins", "fine")
	s.until("the frame that spins was stopped", func() bool { return s.consoleSays(console.Error, "ran too long") })
	told := func(frame string) int {
		count := 0
		for _, line := range s.session().said.Lines() {
			if strings.Contains(line.Text, "ran too long") && line.Frame == frame {
				count++
			}
		}
		return count
	}
	if told("site.test") != 1 || told("") != 0 {
		t.Errorf("the console tells of the script that was stopped %d times for its frame and %d for the page: %s",
			told("site.test"), told(""), s.said())
	}
	going := func() {
		t.Helper()
		before := map[string]string{}
		for _, id := range []string{"", "spins", "fine"} {
			before[id] = s.within(id, `String(ticks)`)
		}
		for _, id := range []string{"", "spins", "fine"} {
			s.becomes(id, `ticks > `+before[id]+` + 3`, "true")
		}
	}
	going()
	s.does("", `setTimeout(() => frames[0].spin(), 0);`)
	s.until("the page's script was stopped", func() bool { return told("") == 1 })
	if got := s.within("spins", `String(spun)`); got != "2" {
		t.Errorf("the function that never ends ran %s times, want 2", got)
	}
	going()
}
