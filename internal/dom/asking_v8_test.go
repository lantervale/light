//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
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

// asking is a page whose questions are kept for the test to answer.
type asking struct {
	rt        *dom.Runtime
	page      *web.Page
	questions []dom.Question
	said      []string
}

func questioned(t *testing.T, body string, shown bool) *asking {
	t.Helper()
	a := &asking{}
	a.page = (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address, body,
		web.Links{Open: func(string) {}, Over: func(string) {}})
	if a.page.Err != nil {
		t.Fatal(a.page.Err)
	}
	rt, err := dom.New(a.page, dom.Host{
		Budget:   300 * time.Millisecond,
		Viewport: func() (float32, float32) { return 800, 600 },
		Console:  func(m dom.Message) { a.said = append(a.said, m.Level+": "+m.Text) },
		Ask: func(q dom.Question) bool {
			a.questions = append(a.questions, q)
			return shown
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	a.rt = rt
	a.page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	rt.Start()
	return a
}

func value(t *testing.T, rt *dom.Runtime, source string) string {
	t.Helper()
	got, err := rt.Evaluate(source)
	if err != nil {
		t.Fatalf("%s: %v", source, err)
	}
	return got
}

func TestAScriptWaitsForTheAnswerItAskedFor(t *testing.T) {
	a := questioned(t, `<p id="p">before</p><script>
		window.got = [];
		got.push(alert("hello"));
		got.push(confirm("sure?"));
		got.push(confirm());
		got.push(prompt("name?", "nobody"));
		got.push(prompt("again?"));
		document.getElementById("p").textContent = "after";
		</script><script>window.second = true</script>`, true)
	rt, questions := a.rt, &a.questions
	if !rt.Asking() || len(*questions) != 1 || (*questions)[0] != (dom.Question{Kind: dom.Alert, Message: "hello"}) {
		t.Fatalf("the page asks %v, waiting %v", *questions, rt.Asking())
	}
	if _, err := rt.Evaluate("1"); err != dom.ErrAsking { //nolint:errorlint // the very error
		t.Fatalf("a script run while the page waits ends with %v", err)
	}
	if !rt.Frame() || rt.Fire(nil, "resize", nil) {
		t.Fatal("a frame of a page that waits says nothing is left")
	}
	time.Sleep(400 * time.Millisecond)
	for i, a := range []dom.Answer{{OK: true}, {OK: true}, {}, {OK: true, Text: "somebody"}, {Text: "ignored"}} {
		if !rt.Asking() || len(*questions) != i+1 {
			t.Fatalf("before answer %d the page asked %v", i, *questions)
		}
		rt.Answer(a)
	}
	if rt.Asking() {
		t.Fatal("the page still waits")
	}
	want := []dom.Question{{Kind: dom.Alert, Message: "hello"}, {Kind: dom.Confirm, Message: "sure?"}, {Kind: dom.Confirm},
		{Kind: dom.Prompt, Message: "name?", Default: "nobody"}, {Kind: dom.Prompt, Message: "again?"}}
	for i, q := range want {
		if (*questions)[i] != q {
			t.Errorf("question %d is %+v, want %+v", i, (*questions)[i], q)
		}
	}
	if got := value(t, rt, `JSON.stringify(got) + document.getElementById("p").textContent + window.second`); got !=
		`"[null,true,false,\"somebody\",null]aftertrue"` {
		t.Fatalf("the script got %s", got)
	}
}

func TestAQuestionThatIsNotShownAnswersAsIfNoneWasAsked(t *testing.T) {
	a := questioned(t, `<script>window.got = [alert("a"), confirm("b"), prompt("c", "d")]</script>`, false)
	rt, questions, said := a.rt, &a.questions, &a.said
	if rt.Asking() || len(*questions) != 3 {
		t.Fatalf("the page asked %v and waits: %v", *questions, rt.Asking())
	}
	if got := value(t, rt, `JSON.stringify(got)`); got != `"[null,false,null]"` || len(*said) != 0 {
		t.Fatalf("the script got %s, the console %v", got, *said)
	}
}

func TestWhatComesWhileAPageWaitsRunsOnceItIsAnswered(t *testing.T) {
	a := questioned(t, `<a id="b" href="/next">b</a><script>
		window.heard = [];
		addEventListener("resize", () => heard.push("resize"));
		document.getElementById("b").addEventListener("click", (e) => {
			if (!confirm("go?")) e.preventDefault();
			heard.push("clicked");
		});
		setTimeout(() => heard.push("timer"), 0);
		</script>`, true)
	rt, link, followed := a.rt, a.page.Doc.ByID("b"), 0
	for _, c := range []struct {
		answer dom.Answer
		want   int
	}{{dom.Answer{}, 0}, {dom.Answer{OK: true}, 1}} {
		if rt.Click(link, 1, 1, dom.Keys{}) || !rt.Asking() {
			t.Fatal("a click whose listener asks did not wait")
		}
		a.page.Tree.Activate(link, func() { followed++ })
		rt.Fire(nil, "resize", nil)
		rt.Frame()
		if followed != 0 {
			t.Fatal("the link was followed before the answer")
		}
		rt.Answer(c.answer)
		if followed != c.want {
			t.Fatalf("after the answer %+v the link was followed %d times", c.answer, followed)
		}
		rt.Frame()
	}
	if got := value(t, rt, `heard.join()`); got != `"clicked,resize,timer,clicked,resize"` {
		t.Fatalf("the page heard %s", got)
	}
}

func TestTheTimeAQuestionWaitsIsNotTheScripts(t *testing.T) {
	a := questioned(t, `<script>alert("one"); window.ran = 1; alert("two"); for (;;) {}</script>
		<script>window.next = true</script>`, true)
	rt, said := a.rt, &a.said
	time.Sleep(500 * time.Millisecond)
	rt.Answer(dom.Answer{OK: true})
	if got := len(*said); got != 0 || !rt.Asking() {
		t.Fatalf("after a long wait the console says %v", *said)
	}
	began := time.Now()
	rt.Answer(dom.Answer{OK: true})
	if took := time.Since(began); took > 2*time.Second || len(*said) != 1 || !strings.Contains((*said)[0], "too long") {
		t.Fatalf("a script that loops after its question was stopped after %s: %v", took, *said)
	}
	if got := value(t, rt, `String(window.ran) + window.next`); got != `"1true"` {
		t.Fatalf("after the loop the page has %s", got)
	}
}

func TestAPageClosedWhileItAsksEndsItsScript(t *testing.T) {
	before, engines := runtime.NumGoroutine(), script.Live()
	for range 5 {
		a := questioned(t, `<script>try { confirm("stay?") } finally { window.after = true }</script>
			<script>window.second = true</script>`, true)
		rt, said := a.rt, &a.said
		if !rt.Asking() {
			t.Fatal("the page does not wait")
		}
		rt.Close()
		if rt.Asking() || !rt.Closed() || len(*said) != 0 {
			t.Fatalf("a page closed while it asks still waits, or says %v", *said)
		}
		rt.Answer(dom.Answer{OK: true})
		rt.Close()
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before || script.Live() != engines {
		t.Fatalf("%d goroutines and %d engines are left, of %d and %d", got, script.Live(), before, engines)
	}
}
