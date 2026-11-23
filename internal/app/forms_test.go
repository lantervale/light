package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"testing"

	"github.com/lantervale/ember/input"
)

// searching is a search engine's site: its form as such a page writes
// it, the handler of its second button and a listener of the window's
// added bare included, and the results every search finds.
func searching(t *testing.T) (string, *asked) {
	t.Helper()
	srv, seen := answers(t, map[string]answer{
		"/": typed("text/html; charset=ISO-8859-1", `<title>Search</title><style>body { margin: 0 }
			input { display: block; height: 24px }</style><form action="/search" name="f">
			<input name="hl" type="hidden" value="fr"><input name="q" id="q">
			<input type="submit" name="btnG" id="go" value="Search"><input type="submit" name="btnI" id="lucky" value="Lucky">
			<script>document.getElementById("lucky").onclick = function () {
				if (this.form.q.value) { this.checked = 1; if (this.form.iflsig) this.form.iflsig.disabled = false; }
				else top.location = "/doodles/";
			};
			addEventListener("submit", function (e) {
				if (e.target.getAttribute("data-submitfalse") === "q" && !e.target.elements.q.value) e.preventDefault();
			}, true);</script>
			<input type="hidden" name="iflsig" value="sig" disabled></form>`),
		"/search": typed("text/html", `<title>Results</title><p id="found">found</p>`),
	})
	return srv.URL, seen
}

func TestASearchIsSentByItsButtonsAndByEnter(t *testing.T) {
	for _, c := range []struct {
		name string
		send func(s *shown)
		// want is what the search asks for, scripted where the page's scripts change it.
		want, scripted string
	}{
		{"its button", func(s *shown) { s.clickAt(s.inPage("go")) }, "/search?hl=fr&q=hello&btnG=Search", ""},
		{"Enter in its field", func(s *shown) { s.press(input.KeyEnter) }, "/search?hl=fr&q=hello&btnG=Search", ""},
		{"a button with a handler", func(s *shown) { s.clickAt(s.inPage("lucky")) }, "/search?hl=fr&q=hello&btnI=Lucky",
			"/search?hl=fr&q=hello&btnI=Lucky&iflsig=sig"},
	} {
		t.Run(c.name, func(t *testing.T) {
			site, seen := searching(t)
			s := show(t)
			s.visit(site + "/")
			scripts := s.session().scripts != nil
			s.clickAt(s.inPage("q"))
			for _, r := range "hello" {
				s.in.AddText(r)
				s.frame()
			}
			c.send(s)
			want := site + c.want
			if scripts && c.scripted != "" {
				want = site + c.scripted
			}
			s.until("the search was sent", func() bool { return s.session().pages[want] != nil })
			s.at(want)
			for range 10 {
				s.frame()
			}
			if n := seen.count("/search"); n != 1 {
				t.Errorf("the search was sent %d times", n)
			}
			for _, l := range s.session().said.Lines() {
				t.Errorf("the console says %s: %s", l.Level, l.Text)
			}
		})
	}
}

func TestEnterSendsAFormAsHTMLsImplicitSubmissionAndItsFieldsAllow(t *testing.T) {
	for _, c := range []struct {
		name, form string
		sent       bool
	}{
		{"one field and no button", `<form action="/found" method="post"><input name="q" id="q"></form>`, true},
		{"two fields and no button", `<form action="/found" method="post"><input name="q" id="q">
			<input name="r"></form>`, false},
		{"a required field left empty", `<form action="/found" method="post"><input name="q" id="q">
			<input name="r" id="r" required><button>Go</button></form>`, false},
		{"a required field left empty in a form of novalidate", `<form action="/found" method="post" novalidate>
			<input name="q" id="q"><input name="r" required><button>Go</button></form>`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, seen := answers(t, map[string]answer{
				"/":      typed("text/html", `<style>input { display: block; height: 24px }</style>`+c.form),
				"/found": typed("text/html", `<p>found</p>`),
			})
			s := show(t)
			page := s.visit(srv.URL + "/")
			if c.name != "one field and no button" && s.session().scripts != nil {
				t.Skip("with scripts, the page's scripts hear Enter and light sends what they leave")
			}
			s.clickAt(s.inPage("q"))
			s.in.AddText('x')
			s.frame()
			s.press(input.KeyEnter)
			if c.sent {
				s.until("the form was sent", func() bool { return seen.count("/found") > 0 })
			}
			for range 10 {
				s.frame()
			}
			if want, n := map[bool]int{true: 1}[c.sent], seen.count("/found"); n != want {
				t.Fatalf("the form was sent %d times, want %d", n, want)
			}
			if r := page.ByID("r"); r != nil && !page.Focused(r) {
				t.Error("the required field left empty does not take the focus")
			}
		})
	}
}
