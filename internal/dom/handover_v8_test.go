//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"bytes"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
)

// here names the goroutine that runs.
func here() string {
	var stack [64]byte
	return string(bytes.Fields(stack[:runtime.Stack(stack[:], false)])[1])
}

// kept is a Storage and the Cookies of a test, which note where they
// are called.
type kept struct {
	values map[string]string
	where  func(string)
}

func (k kept) Get(key string) (string, bool) {
	k.where("storage get")
	value, has := k.values[key]
	return value, has
}
func (k kept) Set(key, value string) { k.where("storage set"); k.values[key] = value }
func (k kept) Remove(key string)     { k.where("storage remove"); delete(k.values, key) }
func (k kept) Clear()                { k.where("storage clear"); clear(k.values) }
func (k kept) Keys() []string {
	k.where("storage keys")
	keys := []string{}
	for key := range k.values {
		keys = append(keys, key)
	}
	return keys
}

type jar struct{ where func(string) }

func (j jar) Get() string   { j.where("cookies get"); return "a=1" }
func (j jar) Set(string)    { j.where("cookies set") }
func (j jar) Enabled() bool { j.where("cookies enabled"); return true }

// hosted is a page whose Host is the test's.
func hosted(t *testing.T, body string, host dom.Host) *dom.Runtime {
	t.Helper()
	page := (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address, body,
		web.Links{Open: func(string) {}, Over: func(string) {}})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	if host.Viewport == nil {
		host.Viewport = func() (float32, float32) { return 800, 600 }
	}
	rt, err := dom.New(page, host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	rt.Start()
	return rt
}

func TestWhatTheBrowserAnswersAScriptRunsOnTheFramesGoroutine(t *testing.T) {
	frame, called := here(), map[string]string{}
	where := func(what string) { called[what] = here() }
	stored := kept{values: map[string]string{}, where: where}
	var stop func()
	rt := hosted(t, `<title>One</title><p id="p">x</p><script>
		window.wide = innerWidth + outerWidth + screenX;
		console.log("said");
		console.clear();
		localStorage.setItem("k", "v");
		window.read = localStorage.getItem("k") + localStorage.length;
		localStorage.removeItem("k");
		localStorage.clear();
		document.cookie = "b=2";
		window.cookies = document.cookie + navigator.cookieEnabled;
		document.title = "Two";
		history.pushState(null, "", "/pushed");
		window.steps = history.length;
		const aborted = new AbortController();
		fetch("/data", { signal: aborted.signal }).catch(() => {});
		aborted.abort();
		window.asked = confirm("sure?");
		</script>`, dom.Host{
		Ask:      func(dom.Question) bool { where("ask"); return false },
		Console:  func(dom.Message) { where("console") },
		Cleared:  func() { where("cleared") },
		Viewport: func() (float32, float32) { where("viewport"); return 800, 600 },
		Window:   func() dom.Window { where("window"); return dom.Window{Width: 900, Height: 700, X: 10, Y: 20, Ratio: 2} },
		Titled:   func(string) { where("titled") },
		Moved:    func(string, string) { where("moved") },
		Steps:    func() int { where("steps"); return 2 },
		Storage:  func(string) dom.Storage { where("storage"); return stored },
		Cookies:  jar{where},
		Fetch: func(dom.Request, func(dom.Response)) func() {
			where("fetch")
			return func() { where("fetch cancelled") }
		},
	})
	rt.Frame()
	_ = stop
	for _, what := range []string{"ask", "console", "cleared", "viewport", "window", "titled", "moved", "steps", "storage",
		"storage get", "storage set", "storage remove", "storage clear", "storage keys", "cookies get", "cookies set",
		"cookies enabled", "fetch", "fetch cancelled"} {
		if at, was := called[what]; !was || at != frame {
			t.Errorf("%s is answered on goroutine %q (called: %v), the frame's being %s", what, at, was, frame)
		}
	}
	if got := value(t, rt, `[wide, read, cookies, steps, asked].join()`); got != `"1710,v1,a=1true,2,false"` {
		t.Fatalf("the script was answered %s", got)
	}
}

func TestEveryKindOfCallOfTheHostIsHandedToTheFrame(t *testing.T) {
	host := reflect.TypeFor[dom.Host]()
	handed := map[string]bool{"Cookies": true, "Windows": true}
	plain := map[string]bool{"Navigator": true, "Sandboxed": true, "Budget": true, "Hold": true, "Kin": true,
		"Patience": true, "Self": true}
	for i := range host.NumField() {
		field := host.Field(i)
		switch {
		case handed[field.Name] || plain[field.Name]:
		case field.Type.Kind() != reflect.Func:
			t.Errorf("Host.%s is a %s: say in framed (framed.go) how its calls reach the frame's goroutine", field.Name,
				field.Type)
		default:
			for o := range field.Type.NumOut() {
				out := field.Type.Out(o)
				if kind := out.Kind(); kind == reflect.Interface && out != reflect.TypeFor[dom.Storage]() &&
					out != reflect.TypeFor[error]() || kind == reflect.Chan || kind == reflect.Map {
					t.Errorf("Host.%s returns a %s: say in framedFunc (framed.go) how its calls reach the frame", field.Name, out)
				}
			}
		}
	}
}

func TestAScriptEnteredFromACallOfTheBrowserRunsWithItsPage(t *testing.T) {
	var rt *dom.Runtime
	var inner []string
	var said []string
	rt = hosted(t, `<title>One</title><script>window.n = 20;</script>`, dom.Host{
		Ask:     func(dom.Question) bool { t.Error("a question asked under a call of the browser is shown"); return true },
		Console: func(m dom.Message) { said = append(said, m.Level+": "+m.Text) },
		Titled: func(title string) {
			got, err := rt.Evaluate(`window.n += 1, [window.n, confirm("under a call?")].join()`)
			inner = append(inner, title+" "+got, here())
			if err != nil {
				t.Error(err)
			}
		},
	})
	value(t, rt, `document.title = "Two"`)
	rt.Frame()
	if got := value(t, rt, `window.n`); got != "21" {
		t.Fatalf("after a script entered under a call the page holds %s", got)
	}
	if len(inner) < 2 || inner[0] != `Two "21,false"` || inner[1] != here() || rt.Asking() {
		t.Fatalf("a script entered from a call of the browser gave %q", inner)
	}
	if text := strings.Join(said, "\n"); !strings.Contains(text, "error: Ignored call to 'confirm()'. It was made while") {
		t.Fatalf("the console says %q", text)
	}
}

func TestTheDocumentsOfATabWaitTogether(t *testing.T) {
	hold := &dom.Hold{}
	asks := hosted(t, `<script>window.got = "none"; setTimeout(() => { window.got = confirm("sure?"); }, 0);</script>`,
		dom.Host{Ask: func(dom.Question) bool { return true }, Hold: hold})
	other := hosted(t, `<script>window.heard = []; addEventListener("ping", () => heard.push("ping"));</script>`,
		dom.Host{Ask: func(dom.Question) bool { return true }, Hold: hold})
	deadline := time.Now().Add(2 * time.Second)
	for !asks.Asking() && time.Now().Before(deadline) {
		asks.Frame()
		time.Sleep(time.Millisecond)
	}
	if hold.Asking() != asks {
		t.Fatal("the page did not ask")
	}
	if _, err := other.Evaluate("1"); !errors.Is(err, dom.ErrAsking) {
		t.Fatalf("while a document of its tab asks another runs a script: %v", err)
	}
	other.Fire(nil, "ping", nil)
	if more := other.Frame(); !more {
		t.Fatal("a document held by another's question has nothing left to run")
	}
	other.Close()
	third := hosted(t, `<script>window.heard = [];</script>`, dom.Host{Ask: func(dom.Question) bool { return true },
		Hold: hold})
	if _, err := third.Evaluate("1"); !errors.Is(err, dom.ErrAsking) {
		t.Fatalf("a document that came while another asks runs a script: %v", err)
	}
	asks.Answer(dom.Answer{OK: true})
	third.Frame()
	got := value(t, asks, "String(got)") + value(t, third, "String(heard.length)")
	if got != `"true""0"` || hold.Asking() != nil {
		t.Fatalf("after the answer the documents say %s", got)
	}
}

func benchmarked(b *testing.B, asks bool, source string) {
	b.Helper()
	host := dom.Host{Viewport: func() (float32, float32) { return 800, 600 }}
	if asks {
		host.Ask = func(dom.Question) bool { return true }
	}
	page := (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address, `<p id="p">x</p>`,
		web.Links{Open: func(string) {}, Over: func(string) {}})
	rt, err := dom.New(page, host)
	if err != nil {
		b.Fatal(err)
	}
	defer rt.Close()
	page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	rt.Start()
	b.ResetTimer()
	for range b.N {
		if _, err = rt.Evaluate(source); err != nil {
			b.Fatal(err)
		}
	}
}

const (
	browserCalls = `for (let i = 0; i < 1000; i++) innerWidth;`
	treeCalls    = `for (let i = 0; i < 1000; i++) document.getElementById("p").getAttribute("id");`
)

func BenchmarkAThousandCallsOfTheBrowserInline(b *testing.B) { benchmarked(b, false, browserCalls) }
func BenchmarkAThousandCallsOfTheBrowserHanded(b *testing.B) { benchmarked(b, true, browserCalls) }
func BenchmarkAThousandCallsOfTheTreeInline(b *testing.B)    { benchmarked(b, false, treeCalls) }
func BenchmarkAThousandCallsOfTheTreeHanded(b *testing.B)    { benchmarked(b, true, treeCalls) }
func BenchmarkAnEntryIntoThePagesScriptsInline(b *testing.B) { benchmarked(b, false, `1`) }
func BenchmarkAnEntryIntoThePagesScriptsHanded(b *testing.B) { benchmarked(b, true, `1`) }
