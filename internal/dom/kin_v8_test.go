//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"testing"
	"time"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/script"
)

// family is a page, window 1, with the frames 2, 3… in it, all kin.
type family struct {
	*tab
	frames int
}

func (*family) Parent(int) int { return 1 }
func (*family) Top(int) int    { return 1 }
func (f *family) Length(of int) int {
	if of == 1 {
		return f.frames
	}
	return 0
}
func (f *family) Child(of, index int) int {
	if of == 1 && index >= 0 && index < f.frames {
		return index + 2
	}
	return dom.NoWindow
}
func (*family) Kin(int) bool { return true }

// member is a document of a family: its window as itself asks for it,
// and what it asked and said.
type member struct {
	*family
	self int
	rt   *dom.Runtime
	page *web.Page
	said []string
	on   func(string)
}

func (m *member) own(of int) int {
	if of == dom.OwnWindow {
		return m.self
	}
	return of
}

// named is a window of the family as m's scripts name it.
func (m *member) named(window int) int {
	if window == m.self {
		return dom.OwnWindow
	}
	return window
}
func (m *member) Parent(of int) int       { return m.named(m.family.Parent(m.own(of))) }
func (m *member) Top(of int) int          { return m.named(m.family.Top(m.own(of))) }
func (m *member) Length(of int) int       { return m.family.Length(m.own(of)) }
func (m *member) Child(of, index int) int { return m.named(m.family.Child(m.own(of), index)) }

// joins makes a document of the family and of kin, at that window.
func (f *family) joins(t *testing.T, kin *dom.Kin, hold *dom.Hold, self int, body string) *member {
	t.Helper()
	m := &member{family: f, self: self}
	page := (&web.Loader{Client: fetch.New(nil), Scripts: true}).Local(address, body,
		web.Links{Open: func(string) {}, Over: func(string) {}})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	rt, err := dom.New(page, dom.Host{
		Budget:   300 * time.Millisecond,
		Viewport: func() (float32, float32) { return 800, 600 },
		Console: func(said dom.Message) {
			m.said = append(m.said, said.Text)
			if m.on != nil {
				m.on(said.Text)
			}
		},
		Ask:  func(dom.Question) bool { return true },
		Hold: hold, Kin: kin, Self: self, Windows: m,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	m.rt, m.page = rt, page
	page.Doc.Layout(geom.Vec2{X: 800, Y: 600})
	rt.Start()
	return m
}

func TestKinShareAnEngineAndCloseOnceNoScriptHoldsIt(t *testing.T) {
	engines, documents := script.Live(), script.Contexts()
	counts := func(what string, kin *dom.Kin, members, live, held int) {
		t.Helper()
		if kin.Members() != members || script.Live()-engines != live || script.Contexts()-documents != held {
			t.Fatalf("%s: %d kin in %d engines and %d documents, want %d, %d and %d", what, kin.Members(),
				script.Live()-engines, script.Contexts()-documents, members, live, held)
		}
	}
	kin, hold := dom.NewKin("https://site.test"), &dom.Hold{}
	f := &family{tab: &tab{closed: map[int]bool{}}, frames: 2}
	top := f.joins(t, kin, hold, 1, `<script>window.mark = "top";</script>`)
	counts("the page alone", kin, 1, 1, 1)
	asker := f.joins(t, kin, hold, 2, `<script>window.mark = "asker";
		window.ask = () => { window.answer = confirm("sure?"); };</script>`)
	other := f.joins(t, kin, hold, 3, `<script>window.mark = "other";</script>`)
	counts("three kin", kin, 3, 1, 3)
	for expression, want := range map[string]string{
		`frames[0].mark + " " + frames[1].mark`:                  `"asker other"`,
		`String(frames[0].Object !== Object)`:                    `"true"`,
		`typeof kin + typeof join + typeof leave + typeof light`: `"undefinedundefinedundefinedundefined"`,
	} {
		if got := value(t, top.rt, expression); got != want {
			t.Errorf("in the page, %s is %s, want %s", expression, got, want)
		}
	}
	if got := value(t, other.rt, `parent.mark + " " + parent.frames[0].mark`); got != `"top asker"` {
		t.Errorf("a frame reads its kin as %s", got)
	}

	if _, err := asker.rt.Evaluate(`setTimeout(ask, 0)`); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		if asker.rt.Frame(); asker.rt.Asking() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !asker.rt.Asking() || !kin.Held() || top.rt.Asking() || other.rt.Asking() {
		t.Fatal("the kin that asks does not hold the engine, or another is said to ask")
	}
	other.rt.Close()
	if !other.rt.Closed() {
		t.Fatal("a kin closed while another asks is not closed for the browser")
	}
	counts("a kin closed while another asks", kin, 3, 1, 3)
	asker.rt.Answer(dom.Answer{OK: true})
	counts("the answer came", kin, 2, 1, 2)
	if got := value(t, asker.rt, `String(answer)`); got != `"true"` || kin.Held() {
		t.Fatalf("the script that asked went on to %s, the engine held: %t", got, kin.Held())
	}

	third := f.joins(t, kin, hold, 3, `<script>window.mark = "third"; window.kept = () => mark;</script>`)
	counts("a kin came in place of the one that left", kin, 3, 1, 3)
	during := ""
	top.on = func(string) {
		third.rt.Close()
		during = "closed: " + map[bool]string{true: "yes", false: "no"}[third.rt.Closed()] + ", kin: " +
			map[bool]string{true: "all", false: "fewer"}[kin.Members() == 3]
	}
	got := value(t, top.rt, `(() => { const kept = frames[1].kept; console.log("close"); return kept(); })()`)
	if during != "closed: yes, kin: all" || got != `"third"` {
		t.Fatalf("a kin closed while a script of the page ran was %q, and what the script held of it gave %s", during, got)
	}
	counts("the script ended", kin, 2, 1, 2)
	top.on = nil

	asker.rt.Close()
	top.rt.Close()
	counts("every kin closed", kin, 0, 0, 0)
	again := f.joins(t, kin, hold, 1, `<script>window.mark = "again";</script>`)
	counts("one came back", kin, 1, 1, 1)
	again.rt.Close()
	counts("and left", kin, 0, 0, 0)
}
