//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/ember/input"
)

// searchSite is a site whose page is a search form holding field, its
// script run once the form is in the page, with q the field and form
// the form; /search answers with what it was asked for.
func searchSite(t *testing.T, field, script string) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu    sync.Mutex
		asked []string
	)
	page := `<title>Search</title><style>body { margin: 0 } textarea, input { display: block; height: 24px }</style>
		<form id="form" action="/search">` + field + `</form><p id="said">none</p><script>
		const q = document.getElementById("q"), form = document.getElementById("form");
		const said = document.getElementById("said");
		` + script + `</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path != "/search" {
			_, _ = w.Write([]byte(page))
			return
		}
		mu.Lock()
		asked = append(asked, r.URL.RawQuery)
		mu.Unlock()
		_, _ = w.Write([]byte(`<title>Results</title><p>found</p>`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
}

// typeCats types "cats" in the page's field q, clicked first.
func (s *shown) typeCats() {
	s.clickAt(s.inPage("q"))
	for _, r := range "cats" {
		s.in.AddText(r)
	}
	s.frame()
	s.frame()
}

// settle runs frames for what a page's scripts do after a key.
func (s *shown) settle() {
	for range 20 {
		s.frame()
		time.Sleep(2 * time.Millisecond)
	}
}

func TestEnterInASearchFieldSendsItsForm(t *testing.T) {
	const submits = `if (e.key === "Enter" && e.keyCode === 13 && !e.shiftKey) { e.preventDefault(); form.submit(); }`
	for _, c := range []struct{ name, field, script string }{
		{"an input alone in its form", `<input id="q" name="q">`, ""},
		{"an input beside its button", `<input id="q" name="q"><button>Search</button>`, ""},
		{"a textarea whose keydown submits", `<textarea id="q" name="q"></textarea>`,
			`q.addEventListener("keydown", (e) => { ` + submits + ` });`},
		{"a textarea whose keydown asks to submit", `<textarea id="q" name="q"></textarea>`,
			`q.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); form.requestSubmit(); } });`},
		{"a textarea whose keydown clicks a hidden button",
			`<textarea id="q" name="q"></textarea><button id="go" style="display: none">Search</button>`,
			`q.addEventListener("keydown", (e) => {
				if (e.key === "Enter") { e.preventDefault(); document.getElementById("go").click(); }
			});`},
		{"a textarea whose form hears its keydown", `<textarea id="q" name="q"></textarea>`,
			`form.addEventListener("keydown", (e) => { if (e.target === q) { ` + submits + ` } });`},
		{"a textarea whose document hears its keydown", `<textarea id="q" name="q"></textarea>`,
			`document.addEventListener("keydown", (e) => { ` + submits + ` });`},
		{"a textarea whose keypress submits", `<textarea id="q" name="q"></textarea>`,
			`q.addEventListener("keypress", (e) => { ` + submits + ` });`},
		{"a textarea whose beforeinput submits", `<textarea id="q" name="q"></textarea>`,
			`q.addEventListener("beforeinput", (e) => {
				if (e.inputType === "insertLineBreak") { e.preventDefault(); form.submit(); }
			});`},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, asked := searchSite(t, c.field, c.script)
			s := show(t)
			s.visit(srv.URL + "/")
			s.until("the page's scripts run", func() bool { return s.session().scripts != nil })
			s.typeCats()
			s.press(input.KeyEnter)
			s.until("the search was sent", func() bool { return len(asked()) > 0 })
			s.until("the results show", func() bool { return s.title(srv.URL+"/search?q=cats") == "Results" })
			if got := asked(); !slices.Equal(got, []string{"q=cats"}) {
				t.Errorf("the site was asked %q", got)
			}
			if said := s.said(); said != "" {
				t.Errorf("the console says %s", said)
			}
		})
	}
}

func TestEnterInATextareaBreaksItsLineUnlessAScriptPreventsIt(t *testing.T) {
	for _, c := range []struct{ name, script, value, said string }{
		{"heard by no script", "", "cats\n", "none"},
		{"heard by a script that lets it be",
			`q.addEventListener("keydown", (e) => { said.textContent = e.type + " " + e.key + " " + e.keyCode; });
			q.addEventListener("input", (e) => { said.textContent += ", " + e.type + " " + e.inputType; });`,
			"cats\n", "keydown Enter 13, input insertLineBreak"},
		{"prevented at its keydown", `q.addEventListener("keydown", (e) => { if (e.key === "Enter") e.preventDefault(); });`,
			"cats", "none"},
		{"prevented at its keypress", `q.addEventListener("keypress", (e) => {
				said.textContent = e.type + " " + e.charCode; e.preventDefault(); });`, "cats", "keypress 13"},
		{"prevented at its beforeinput", `q.addEventListener("beforeinput", (e) => {
				said.textContent = e.type + " " + e.inputType; e.preventDefault(); });`, "cats", "beforeinput insertLineBreak"},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, asked := searchSite(t, `<textarea id="q" name="q"></textarea>`, c.script)
			s := show(t)
			s.visit(srv.URL + "/")
			s.until("the page's scripts run", func() bool { return s.session().scripts != nil })
			s.typeCats()
			s.press(input.KeyEnter)
			s.settle()
			page := s.session().pages[srv.URL+"/"]
			if got := page.Doc.Value(page.Doc.ByID("q")); got != c.value {
				t.Errorf("the textarea holds %q, want %q", got, c.value)
			}
			if got := s.pageText("said"); got != c.said {
				t.Errorf("the page says %q, want %q", got, c.said)
			}
			if got := asked(); len(got) != 0 {
				t.Errorf("the site was asked %q", got)
			}
			if said := s.said(); said != "" {
				t.Errorf("the console says %s", said)
			}
		})
	}
}

func TestEnterTellsScriptsTheModifiersHeldWithIt(t *testing.T) {
	const script = `q.addEventListener("keydown", (e) => {
		said.textContent = [e.key, e.shiftKey, e.altKey, e.ctrlKey, e.metaKey].join(" ");
		if (e.key === "Enter" && !e.shiftKey) e.preventDefault();
	});`
	for _, c := range []struct {
		name  string
		held  input.Key
		value string
		said  string
	}{
		{"Shift, which the script lets add a line", input.KeyLeftShift, "cats\n", "Enter true false false false"},
		{"Alt, which it keeps from that", input.KeyLeftAlt, "cats", "Enter false true false false"},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := searchSite(t, `<textarea id="q" name="q"></textarea>`, script)
			s := show(t)
			s.visit(srv.URL + "/")
			s.until("the page's scripts run", func() bool { return s.session().scripts != nil })
			s.typeCats()
			s.in.SetKey(c.held, true)
			s.frame()
			s.press(input.KeyEnter)
			s.in.SetKey(c.held, false)
			s.settle()
			page := s.session().pages[srv.URL+"/"]
			if got := page.Doc.Value(page.Doc.ByID("q")); got != c.value {
				t.Errorf("the textarea holds %q, want %q", got, c.value)
			}
			if got := s.pageText("said"); got != c.said {
				t.Errorf("the page says %q, want %q", got, c.said)
			}
		})
	}
}

func TestTheShortcutEnterInATextareaSubmitsItsFormWithItsKeys(t *testing.T) {
	command, name := input.KeyLeftControl, "ctrlKey"
	if runtime.GOOS == "darwin" {
		command, name = input.KeyLeftSuper, "metaKey"
	}
	for _, c := range []struct {
		name, script, said string
		asked              []string
	}{
		{"heard by no script", "", "none", []string{"q=cats"}},
		{"prevented by a script that reads its keys", `q.addEventListener("keydown", (e) => {
				if (e.key === "Enter" && e.` + name + `) { said.textContent = "kept"; e.preventDefault(); }
			});`, "kept", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, asked := searchSite(t, `<textarea id="q" name="q"></textarea>`, c.script)
			s := show(t)
			s.visit(srv.URL + "/")
			s.until("the page's scripts run", func() bool { return s.session().scripts != nil })
			s.typeCats()
			s.in.SetKey(command, true)
			s.frame()
			s.press(input.KeyEnter)
			s.in.SetKey(command, false)
			s.settle()
			if c.asked != nil {
				s.until("the search was sent", func() bool { return len(asked()) > 0 })
				s.settle()
			} else if got := s.pageText("said"); got != c.said {
				t.Errorf("the page says %q, want %q", got, c.said)
			}
			if got := asked(); !slices.Equal(got, c.asked) {
				t.Errorf("the site was asked %q, want %q", got, c.asked)
			}
		})
	}
}
