package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"testing"

	"github.com/lantervale/ember/input"
)

// searching is a search engine's site: its form as such a page writes
// it, the handler of its second button and a listener of the window's
// added bare included, and the results every search finds.
func searching(t *testing.T) string {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
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
	return srv.URL
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
			site := searching(t)
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
			for _, l := range s.session().said.Lines() {
				t.Errorf("the console says %s: %s", l.Level, l.Text)
			}
		})
	}
}
