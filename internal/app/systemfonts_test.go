package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"math"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
)

// fallenBack is how wide text is in the font of like, laid out by a
// Document that falls back to the machine's faces for the characters
// its own lack. It skips t where the machine has no face for them: the
// width stays the one of a Document without the fallback.
func fallenBack(t *testing.T, doc *ui.Document, like *ui.Node, text string) float32 {
	t.Helper()
	look := ""
	for _, name := range []string{"font-family", "font-size", "font-weight", "font-style"} {
		value, _ := doc.ComputedValue(like, name)
		look += name + ": " + value + "; "
	}
	width := func(opts ...ui.DocumentOption) func() float32 {
		d := ui.NewDocument(opts...)
		span := ui.Span(ui.Text(text), ui.Style(look))
		if lang, ok := like.Attr("lang"); ok {
			span.SetAttr("lang", lang)
		}
		d.Body().Append(span)
		return func() float32 { d.Layout(geom.V2(800, 200)); return span.Box().W }
	}
	alone := width()()
	fallen := width(ui.SystemFonts())
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if w := fallen(); w != alone {
			return w
		}
	}
	t.Skipf("this machine has no face drawing %q", text)
	return 0
}

func TestATabNamedInAScriptOfNoEngineFaceDrawsItsLetters(t *testing.T) {
	for _, c := range []struct{ name, lang, title string }{
		{"Arabic", "ar", "الصفحة الرئيسية"},
		{"Japanese", "ja", "日本語のメインページ"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := show(t)
			srv, _ := answers(t, map[string]answer{"/": typed("text/html",
				`<html lang="`+c.lang+`"><title>`+c.title+`</title><p>`+c.title)})
			s.visit(srv.URL + "/")
			if got := s.tabNames(); got != c.title {
				t.Fatalf("the tab is named %q, want %q", got, c.title)
			}
			name := s.strip[s.browser.Active].name
			s.doc.AddSheet(".tab .name { flex: none; overflow: visible; }")
			want := fallenBack(t, s.doc, name, c.title)
			var got float32
			for range 600 {
				s.frame()
				if got = name.Box().W; math.Abs(float64(got-want)) < 0.5 {
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
			t.Errorf("the tab's name is %v wide, where the machine's faces draw it %v wide: its letters are missing",
				got, want)
		})
	}
}

func TestATabsNameIsInItsPagesLanguage(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/ja": typed("text/html", `<html lang="ja"><title>漢字</title>`),
		"/zh": {header: map[string]string{"Content-Type": "text/html", "Content-Language": "zh-Hans"},
			body: `<title>汉字</title>`},
		"/none": typed("text/html", `<title>漢字</title>`),
	})
	s := show(t)
	for _, c := range []struct{ path, lang string }{{"/ja", "ja"}, {"/zh", "zh-Hans"}, {"/none", ""}} {
		s.visit(srv.URL + c.path)
		for what, n := range map[string]*ui.Node{"strip": s.strip[s.browser.Active].name,
			"list": s.strip[s.browser.Active].listed} {
			if got, _ := n.Attr("lang"); got != c.lang {
				t.Errorf("in the %s the name of %s is in %q, want %q", what, c.path, got, c.lang)
			}
		}
	}
}
