//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/console"
)

func tooled(t *testing.T) *shown {
	t.Helper()
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>top</title><body style="margin:0"><p id="here">top</p>
			<iframe id="a" src="https://other.test/kid" width="300" height="80" style="border:0;display:block"></iframe>
			<iframe id="b" sandbox="allow-scripts" src="/shut" width="300" height="40" style="border:0;display:block"></iframe>
			<script>window.secret = "of top";</script>`),
		"other.test/kid": html(`<title>kid</title><style>#in { margin: 0; height: 20px; width: 100px; colour: red; }</style>
			<body style="margin:0"><p id="in">in the frame</p><iframe id="n" src="/leaf" width="200" height="30"
			style="border:0;display:block"></iframe><script>window.secret = "of kid";</script>`),
		"other.test/leaf": html(`<body style="margin:0"><p id="leaf" style="margin:0">leaf</p>
			<script>window.secret = "of leaf";</script>`),
		"site.test/shut": html(`<p>shut</p><script>window.secret = "of shut";</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "a", "n", "b")
	s.inspector()
	return s
}

func TestTheInspectorGoesIntoFrames(t *testing.T) {
	s := tooled(t)
	inner := s.frameNamed("a").page.Doc.ByID("in")
	s.inspect.Select(inner)
	for range 5 {
		s.frame()
	}
	rows := strings.Join(s.markup(), "\n")
	for _, want := range []string{`<iframe id="a"`, `<p id="in">in the frame</p>`, `<iframe id="n"`,
		"<title>kid</title>"} {
		if strings.Contains(want, "title") == strings.Contains(rows, want) {
			t.Errorf("the tree with an element of a frame selected: %q in\n%s", want, rows)
		}
	}
	if !s.inspect.RowOf(inner).HasClass("selected") {
		t.Errorf("the element of the frame is not selected in the tree")
	}
	if rules := s.node("dt-insp-rules").Text(); !strings.Contains(rules, "#in") || !strings.Contains(rules, "100px") {
		t.Errorf("the rules of an element of a frame read %q", rules)
	}
	if box := s.node("dt-insp-box").Text(); !strings.Contains(box, "100 × 20") {
		t.Errorf("the box of an element of a frame reads %q", box)
	}
	s.in.SetMouse(s.inspect.RowOf(inner).Box().Center())
	s.frame()
	s.frame()
	at := s.inFrame(s.frameNamed("a"), "in")
	got, shown := s.inspect.Highlighted()
	if off := got.Center().Sub(at); !shown || got.W != 100 || got.H != 20 || off.Dot(off) > 1 {
		t.Errorf("the element of the frame is outlined at %v, %t, want around %v", got, shown, at)
	}
	if got := s.within("a", `$0 && $0.id`) + "|" + s.within("", `String($0)`) + "|" + s.within("n", `String($0)`); got !=
		"in|null|null" {
		t.Errorf("$0 reads %s in the frame, the page and the frame in it", got)
	}
	leaf := s.frameNamed("n").page.Doc.ByID("leaf")
	s.click("dt-insp-pick")
	s.clickAt(s.inFrame(s.frameNamed("n"), "leaf"))
	if s.inspect.Selected() != leaf || s.inspect.Picking() {
		t.Fatalf("picking in a frame of a frame selected %v", s.inspect.Selected())
	}
	if got := s.within("n", `$0 && $0.id`) + "|" + s.within("a", `String($0)`); got != "leaf|null" {
		t.Errorf("after picking $0 reads %s", got)
	}
	s.does("", `document.getElementById("a").remove();`)
	for range 60 {
		s.frame()
	}
	if rows = strings.Join(s.markup(), "\n"); strings.Contains(rows, "leaf") || s.inspect.Selected() == leaf {
		t.Errorf("the tree keeps the frame that went away:\n%s", rows)
	}
}

func TestInspectElementSelectsTheElementOfAFrame(t *testing.T) {
	s := tooled(t)
	s.toggleTools()
	s.frame()
	s.rightClickAt(s.inFrame(s.frameNamed("a"), "in"))
	s.choose("Inspect element")
	for range 5 {
		s.frame()
	}
	if got := s.inspect.Selected(); got != s.frameNamed("a").page.Doc.ByID("in") {
		t.Fatalf("Inspect element in a frame selected %v", got)
	}
}

func TestTheWarningsOfAFrameAreListedAsItsOwn(t *testing.T) {
	s := tooled(t)
	var warned []string
	for _, w := range s.warnings() {
		warned = append(warned, w.Error())
	}
	got := strings.Join(warned, "\n")
	if !strings.Contains(got, "in the frame other.test: ") || !strings.Contains(got, "colour") {
		t.Errorf("the warnings are\n%s", got)
	}
}

func TestTheConsolesLineIsEvaluatedInTheFrameChosen(t *testing.T) {
	s := tooled(t)
	s.click("dt-tab-console")
	s.frame()
	names := func() string {
		var out []string
		for _, in := range s.contexts() {
			out = append(out, in.Name+" "+in.Origin)
		}
		return strings.Join(out, ", ")
	}
	if got := names(); got != "top https://site.test, other.test https://other.test, site.test null, other.test "+
		"https://other.test" {
		t.Fatalf("the console's line may be evaluated in %s", got)
	}
	evaluate := func(source string) {
		s.click("dt-eval")
		for _, r := range source {
			s.in.AddText(r)
		}
		s.frame()
		s.press(input.KeyEnter)
		s.frame()
	}
	evaluate("secret")
	s.click("dt-context")
	evaluate("secret")
	s.click("dt-context")
	evaluate("secret + origin")
	lines := s.session().said.Lines()
	var got []string
	for _, l := range lines[len(lines)-6:] {
		if l.Level == console.Result {
			got = append(got, l.Frame+": "+l.Text)
		}
	}
	if strings.Join(got, ", ") != `: "of top", other.test: "of kid", site.test: "of shutnull"` {
		t.Fatalf("the lines gave %q", got)
	}
	s.does("", `document.getElementById("b").remove();`)
	for range 10 {
		s.frame()
	}
	if s.session().tools.Context != 0 || s.node("dt-context").Text() != "top · https://site.test" {
		t.Errorf("once its frame went away the line is evaluated in %q", s.node("dt-context").Text())
	}
	evaluate("secret")
	if last := s.session().said.Lines(); last[len(last)-1].Text != `"of top"` {
		t.Errorf("the line gave %q", last[len(last)-1].Text)
	}
}
