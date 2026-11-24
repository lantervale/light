//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/script"
)

// slow is a page of a browser that asks, with little patience, and
// what it was told.
type slow struct {
	t       *testing.T
	rt      *dom.Runtime
	page    *web.Page
	said    []string
	noticed []time.Duration
	asked   []dom.Question
	viewed  map[string]int
	waits   dom.Patience
}

// impatient is a browser that waits little, for tests that wait less.
func impatient() dom.Patience {
	return dom.Patience{After: 40 * time.Millisecond, Notice: 120 * time.Millisecond, Again: 150 * time.Millisecond}
}

func (s *slow) host() dom.Host {
	return dom.Host{
		Ask:      func(q dom.Question) bool { s.asked = append(s.asked, q); return true },
		Console:  func(m dom.Message) { s.said = append(s.said, m.Level+": "+m.Text) },
		Viewport: func() (float32, float32) { s.viewed[here()]++; return 800, 600 },
		Slow:     func(ran time.Duration) { s.noticed = append(s.noticed, ran) },
		Patience: s.waits,
	}
}

func slowly(t *testing.T, body string) *slow {
	t.Helper()
	return slowlyWith(t, body, impatient())
}

func slowlyWith(t *testing.T, body string, waits dom.Patience) *slow {
	t.Helper()
	s := &slow{t: t, viewed: map[string]int{}, waits: waits}
	s.page = (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address, body,
		web.Links{Open: func(string) {}, Over: func(string) {}})
	if s.page.Err != nil {
		t.Fatal(s.page.Err)
	}
	rt, err := dom.New(s.page, s.host())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	s.rt = rt
	s.page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	rt.Start()
	for late, _ := rt.Slow(); late; late, _ = rt.Slow() {
		rt.Frame()
	}
	s.said, s.noticed = nil, nil
	return s
}

// frames runs frames, laying the page out at each as the window does,
// until held says so; it reports how many it ran and the longest.
func (s *slow) frames(what string, held func() bool) (int, time.Duration) {
	s.t.Helper()
	count, longest := 0, time.Duration(0)
	for deadline := time.Now().Add(5 * time.Second); !held(); count++ {
		if time.Now().After(deadline) {
			s.t.Fatalf("never %s; the console says\n%s", what, strings.Join(s.said, "\n"))
		}
		began := time.Now()
		s.rt.Frame()
		s.page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
		_ = s.page.Doc.Body().Text()
		longest = max(longest, time.Since(began))
		time.Sleep(time.Millisecond)
	}
	return count, longest
}

func (s *slow) late() bool {
	_, err := s.rt.Evaluate("1")
	return errors.Is(err, dom.ErrBusy)
}

func (s *slow) says(text string) int {
	return strings.Count(strings.Join(s.said, "\n"), text)
}

func TestTheFramesGoOnWhileAScriptRunsLong(t *testing.T) {
	s := slowly(t, `<p id="p">none</p><script>
		window.heard = [];
		addEventListener("first", () => heard.push("first"));
		addEventListener("second", () => heard.push("second"));
		setTimeout(() => heard.push("timer"), 0);
		setTimeout(() => {
			const end = Date.now() + 500;
			let n = 0;
			while (Date.now() < end) {
				document.getElementById("p").textContent = "at " + (n++) + " of " + innerWidth;
				const added = document.body.appendChild(document.createElement("i"));
				if (n % 3) added.remove();
			}
			heard.push("ended");
		}, 0);
		</script>`)
	frame := here()
	s.frames("late", s.late)
	tellsOf := func() bool { _, on := s.rt.Slow(); return on }
	if _, on := s.rt.Slow(); on || len(s.noticed) != 0 || s.says("warn: A script of this page has run for") != 1 {
		t.Fatalf("a script late for %s is told of as %v, the console saying %q", impatient().After, s.noticed, s.said)
	}
	before := s.viewed[frame]
	s.rt.Fire(nil, "first", nil)
	count, longest := s.frames("told of", tellsOf)
	if count < 3 || longest > 100*time.Millisecond || s.viewed[frame] == before || len(s.viewed) != 1 {
		t.Fatalf("while the script ran %d frames went by, the longest %s, the browser answering %d calls on %v", count,
			longest, s.viewed[frame]-before, s.viewed)
	}
	if len(s.noticed) != 1 || s.noticed[0] < impatient().Notice {
		t.Fatalf("the user is told at %v", s.noticed)
	}
	s.rt.Fire(nil, "second", nil)
	if text := s.page.Doc.ByID("p").Text(); !strings.HasPrefix(text, "at ") || !strings.HasSuffix(text, " of 800") {
		t.Fatalf("the page the frames show reads %q", text)
	}
	s.rt.Wait()
	s.frames("ended", func() bool { return !s.late() })
	if got := value(t, s.rt, `heard.join()`); got != `"timer,ended,first,second"` {
		t.Fatalf("what came while the script ran was heard as %s", got)
	}
	if _, on := s.rt.Slow(); on || len(s.noticed) != 2 || s.noticed[1] < s.noticed[0]+impatient().Again {
		t.Fatalf("after waiting the user is told at %v (still: %v)", s.noticed, on)
	}
	if s.says("warn: A script") != 1 || s.says("error") != 0 {
		t.Fatalf("the console says %q", s.said)
	}
}

func TestAScriptThatRunsLongIsStoppedWhenTheUserSaysSo(t *testing.T) {
	s := slowly(t, `<script>
		window.stage = "none";
		setTimeout(() => { window.stage = "looping"; for (;;) {} }, 0);
		setTimeout(() => { window.stage = "the next task ran"; }, 0);
		</script>`)
	s.frames("told of", func() bool { _, on := s.rt.Slow(); return on })
	s.rt.Stop()
	s.frames("stopped", func() bool { return !s.late() })
	s.frames("on to its next task", func() bool { return value(t, s.rt, "stage") == `"the next task ran"` })
	if s.says("error: "+script.ErrInterrupted.Error()) != 1 {
		t.Fatalf("the console says %q", s.said)
	}
}

func TestAQuestionOfAScriptThatRunsLongIsWaitedForAsAnyOther(t *testing.T) {
	waits := dom.Patience{After: 40 * time.Millisecond, Notice: 600 * time.Millisecond, Again: time.Second}
	s := slowlyWith(t, `<script>
		const spin = (ms) => { const end = Date.now() + ms; while (Date.now() < end) {} };
		setTimeout(() => {
			spin(200);
			window.answer = confirm("still there?");
			spin(100);
			window.ended = true;
		}, 0);
		</script>`, waits)
	s.frames("late", s.late)
	s.frames("asking", s.rt.Asking)
	if s.late() || len(s.asked) != 1 {
		t.Fatalf("a late script that asks is still late, or asked %v", s.asked)
	}
	time.Sleep(waits.Notice)
	s.rt.Frame()
	if _, on := s.rt.Slow(); on || len(s.noticed) != 0 {
		t.Fatalf("the time a question is open is counted: told at %v", s.noticed)
	}
	s.rt.Answer(dom.Answer{OK: true})
	s.frames("ended", func() bool {
		got, err := s.rt.Evaluate(`String(window.ended) + window.answer`)
		return err == nil && got == `"truetrue"`
	})
}

func TestAPageClosedWhileItsScriptRunsLongEndsIt(t *testing.T) {
	s := slowly(t, `<script>window.n = 0; setTimeout(() => { for (;;) { document.title = "t" + (n++); } }, 0);</script>`)
	goroutines, engines := runtime.NumGoroutine(), script.Live()
	s.frames("late", s.late)
	began := time.Now()
	s.rt.Close()
	if took := time.Since(began); took > time.Second || script.Live() != engines-1 {
		t.Fatalf("closing took %s and left %d engines of %d", took, script.Live(), engines)
	}
	for deadline := time.Now().Add(2 * time.Second); runtime.NumGoroutine() > goroutines && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if runtime.NumGoroutine() > goroutines || s.says("error") != 0 {
		t.Fatalf("%d goroutines are left of %d; the console says %q", runtime.NumGoroutine(), goroutines, s.said)
	}
}

func TestALateScriptHoldsItsKinAndItsTab(t *testing.T) {
	kin, hold := dom.NewKin("https://site.test"), &dom.Hold{}
	f := &family{tab: &tab{closed: map[int]bool{}}, frames: 1}
	var noticed []string
	joined := func(self int, body string) *dom.Runtime {
		page := (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address, body,
			web.Links{Open: func(string) {}, Over: func(string) {}})
		rt, err := dom.New(page, dom.Host{
			Ask:      func(dom.Question) bool { return true },
			Viewport: func() (float32, float32) { return 800, 600 },
			Slow:     func(time.Duration) { noticed = append(noticed, body[:12]) },
			Patience: impatient(), Hold: hold, Kin: kin, Self: self, Windows: &member{family: f, self: self},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rt.Close)
		page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
		rt.Start()
		return rt
	}
	top := joined(1, `<p id="top"></p><script>window.heard = [];
		addEventListener("ping", () => heard.push("ping"));</script>`)
	framed := joined(2, `<p id="frm"></p><script>
		window.spin = () => { const end = Date.now() + 300; while (Date.now() < end) { parent.heard.length; } };</script>`)
	apart := hosted(t, `<script>window.own = 1;</script>`,
		dom.Host{Ask: func(dom.Question) bool { return true }, Hold: hold})
	if _, err := framed.Evaluate(`setTimeout(spin, 0)`); err != nil {
		t.Fatal(err)
	}
	busy := func(rt *dom.Runtime) bool {
		_, err := rt.Evaluate("1")
		return err != nil
	}
	for deadline := time.Now().Add(3 * time.Second); !busy(framed) && time.Now().Before(deadline); {
		framed.Frame()
		time.Sleep(time.Millisecond)
	}
	if !busy(framed) || !busy(top) || !busy(apart) || !kin.Held() {
		t.Fatalf("while a frame's script runs long its kin and its tab's other documents run scripts: %v %v %v",
			busy(framed), busy(top), busy(apart))
	}
	top.Fire(nil, "ping", nil)
	on := false
	for deadline := time.Now().Add(3 * time.Second); busy(framed) && time.Now().Before(deadline); {
		top.Frame()
		framed.Frame()
		apart.Frame()
		if _, told := framed.Slow(); told {
			on = true
		}
		if _, told := top.Slow(); told {
			t.Fatal("the page is told slow for its frame's script")
		}
		time.Sleep(time.Millisecond)
	}
	top.Frame()
	if got := value(t, top, `heard.join()`) + value(t, apart, `own`); got != `"ping"1` || !on || len(noticed) != 1 ||
		noticed[0] != `<p id="frm">` {
		t.Fatalf("after the frame's script the page heard %s; told %v of %q", got, on, noticed)
	}
}

func TestABrowserThatDoesNotAskStopsAScriptAtItsBudget(t *testing.T) {
	var said []string
	rt := hosted(t, `<script>setTimeout(() => { for (;;) {} }, 0);</script>`, dom.Host{
		Budget:  80 * time.Millisecond,
		Ask:     func(dom.Question) bool { return true },
		Console: func(m dom.Message) { said = append(said, m.Level+": "+m.Text) },
	})
	began := time.Now()
	rt.Frame()
	if took := time.Since(began); took < 80*time.Millisecond || took > time.Second ||
		strings.Join(said, "\n") != "error: "+script.ErrInterrupted.Error() {
		t.Fatalf("the frame took %s and the console says %q", took, said)
	}
	if got := value(t, rt, "1 + 1"); got != "2" {
		t.Fatalf("after the stopped script the page gives %s", got)
	}
}

func TestAScriptRunUnderAnotherOnesCallIsStoppedWhenTheUserWouldBeAsked(t *testing.T) {
	s := &slow{t: t, viewed: map[string]int{}, waits: impatient()}
	s.page = (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address,
		`<script>window.width = () => { window.got = innerWidth; };</script>`,
		web.Links{Open: func(string) {}, Over: func(string) {}})
	host, under := s.host(), error(nil)
	host.Viewport = func() (float32, float32) {
		if s.rt != nil && under == nil {
			_, under = s.rt.Evaluate(`for (;;) {}`)
		}
		return 800, 600
	}
	rt, err := dom.New(s.page, host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	s.page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	rt.Start()
	for late, _ := rt.Slow(); late; late, _ = rt.Slow() {
		rt.Frame()
	}
	s.rt = rt
	began := time.Now()
	if _, err = rt.Evaluate(`width()`); err != nil && !errors.Is(err, dom.ErrBusy) {
		t.Fatal(err)
	}
	s.frames("ended", func() bool { return !s.late() })
	if took := time.Since(began); under == nil || !strings.Contains(under.Error(), script.ErrInterrupted.Error()) ||
		took < impatient().Notice || took > 2*time.Second {
		t.Fatalf("the script under a call ended after %s with %v", took, under)
	}
	if got := value(t, rt, "got"); got != "800" {
		t.Fatalf("the call it ran under gave %s", got)
	}
}

func TestAnExpressionThatRunsLongSaysItStillRuns(t *testing.T) {
	s := slowly(t, `<script>window.spin = (ms) => { const end = Date.now() + ms; while (Date.now() < end) {} };</script>`)
	if _, err := s.rt.Evaluate(`spin(100), window.spun = true`); !errors.Is(err, dom.ErrBusy) {
		t.Fatalf("an expression that runs long gave %v", err)
	}
	s.frames("ended", func() bool { return !s.late() })
	if got := value(t, s.rt, "spun"); got != "true" {
		t.Fatalf("the expression went on to %s", got)
	}
}
