//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
)

// searchForm is a search engine's form, and the handler of its second
// button as such a page writes it.
const searchForm = `<form name="f" id="form" action="/search"><input name="hl" type="hidden" value="fr">
	<input name="q" id="q" value="hi"><input type="radio" name="size" value="s">
	<input type="radio" name="size" value="l" checked>
	<input type="submit" name="btnG" id="go" value="Search"><input type="submit" name="btnI" id="lucky" value="Lucky">
	<input type="hidden" name="iflsig" value="sig" disabled><img name="logo"></form>
	<form id="login" method="post" action="/login"><input name="user"><input type="submit" id="enter" name="in"
	value="yes"></form>
	<script>var seen = [];
	lucky.onclick = function () {
		if (this.form.q.value) { this.checked = 1; if (this.form.iflsig) this.form.iflsig.disabled = false; }
		else top.location = "/doodles/";
	};</script>`

func TestAFormAndTheDocumentNameWhatTheyHold(t *testing.T) {
	s := run(t, searchForm)
	for _, c := range []struct{ script, want string }{
		{`document.f.id + document.forms.f.id + document.forms.form.id + document.forms.length`, "formformform2"},
		{`document.forms[0].q.value + form.q.id + form.elements.q.value + form.elements["hl"].value`, "hiqhifr"},
		{`form.elements.namedItem("q").id + form.elements.length + form.length`, "q77"},
		{`[form.size.length, form.size.value, form.elements.size instanceof RadioNodeList].join()`, "2,l,true"},
		{`[lucky.form === form, q.form.btnG.value, form.logo.tagName, document.logo.tagName].join()`,
			"true,Search,IMG,IMG"},
		{`[form.nothing, document.nothing, form.elements.nothing, document.forms.nothing].join()`, ",,,"},
		{`[form.id, form.method, form.action, form.elements instanceof HTMLCollection, document.title].join()`,
			"form,get,https://site.test/search,true,"},
		{`form.own = 3; document.own = 4; [form.own, document.own, Object.keys(form.elements).length].join()`, "3,4,7"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s is %q, want %q", c.script, got, c.want)
		}
	}
	if said := s.said(); said != "" {
		t.Errorf("the console says %s", said)
	}
}

func TestAFormIsSentUnlessAScriptHoldsIt(t *testing.T) {
	const to = "https://site.test/search?hl=fr&q=hi&size=l&"
	const login = "https://site.test/login application/x-www-form-urlencoded "
	for _, c := range []struct {
		name, script string
		do           func(s *scripted)
		opened, went []string
	}{
		{"a click on its button", "", func(s *scripted) { s.press("go") }, []string{to + "btnG=Search"}, nil},
		{"a click its handler changes the form on", "", func(s *scripted) { s.press("lucky") },
			[]string{to + "btnI=Lucky&iflsig=sig"}, nil},
		{"a click its handler sends elsewhere", `q.value = ""`, func(s *scripted) { s.press("lucky") },
			[]string{"https://site.test/search?hl=fr&q=&size=l&btnI=Lucky"}, []string{"https://site.test/doodles/"}},
		{"a prevented click", `go.addEventListener("click", (e) => e.preventDefault())`,
			func(s *scripted) { s.press("go") }, nil, nil},
		{"a prevented submit", `form.addEventListener("submit", (e) => { seen.push(e.submitter.id); e.preventDefault() })`,
			func(s *scripted) { s.press("go") }, nil, nil},
		{"the keyboard on its button", "", func(s *scripted) {
			s.page.Tree.Submit(s.byID("form"), s.byID("go"))
			s.rt.Frame()
		}, []string{to + "btnG=Search"}, nil},
		{"Enter in its field", `addEventListener("submit", (e) => seen.push(e.submitter.id))`, func(s *scripted) {
			s.page.Tree.Heard(s.byID("q"), "submit")
			s.rt.Frame()
		}, []string{to + "btnG=Search"}, nil},
		{"Enter a script holds", `form.onsubmit = () => false`, func(s *scripted) {
			s.page.Tree.Heard(s.byID("q"), "submit")
			s.rt.Frame()
		}, nil, nil},
		{"a script's submit", `form.onsubmit = () => seen.push("told"); form.submit()`, func(*scripted) {}, nil,
			[]string{strings.TrimSuffix(to, "&")}},
		{"a script's requestSubmit", `form.onsubmit = (e) => { seen.push("told " + e.submitter.id) };
			form.requestSubmit(lucky)`, func(*scripted) {}, nil, []string{to + "btnI=Lucky"}},
		{"a POST by its button", "", func(s *scripted) { s.press("enter") }, nil, nil},
		{"a POST whose click is prevented", `enter.onclick = () => false`, func(s *scripted) { s.press("enter") }, nil,
			nil},
		{"a POST a script holds", `login.addEventListener("submit", (e) => e.preventDefault())`,
			func(s *scripted) { s.press("enter") }, nil, nil},
		{"a script's submit of a POST", `login.onsubmit = () => false; login.user.value = "é"; login.submit()`,
			func(*scripted) {}, nil, nil},
		{"a script's requestSubmit of a POST", `login.requestSubmit(enter)`, func(*scripted) {}, nil, nil},
		{"a script's requestSubmit a script holds", `login.onsubmit = () => false; login.requestSubmit()`,
			func(*scripted) {}, nil, nil},
		{"a form with a file's type", `login.setAttribute("enctype", "multipart/form-data"); login.submit()`,
			func(*scripted) {}, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, searchForm+"<script>"+c.script+"</script>")
			s.t = t
			c.do(s)
			if !slices.Equal(s.opened, c.opened) || !slices.Equal(s.went, c.went) {
				t.Errorf("the tab opened %q and scripts sent it to %q, want %q and %q", s.opened, s.went, c.opened, c.went)
			}
			posted := map[string][]string{"a POST by its button": {login + "user=&in=yes"},
				"a script's submit of a POST":        {"script " + login + "user=%C3%A9"},
				"a script's requestSubmit of a POST": {"script " + login + "user=&in=yes"}}[c.name]
			if !slices.Equal(s.posted, posted) {
				t.Errorf("the tab was sent %q with a POST, want %q", s.posted, posted)
			}
			told := map[string]string{"a prevented submit": "go", "Enter in its field": "go",
				"a script's requestSubmit": "told lucky"}[c.name]
			if got := s.eval(`seen.join()`); got != told {
				t.Errorf("the page was told %q, want %q", got, told)
			}
			warned := strings.Contains(s.said(),
				"A form was not sent: light sends no form as multipart/form-data, and so no file.")
			if unsent := c.name == "a form with a file's type"; warned != unsent {
				t.Errorf("the console says %s", s.said())
			}
		})
	}
}

func TestAListenerAddedBareIsTheWindows(t *testing.T) {
	s := run(t, `<script>var seen = [];
		const heard = (e) => seen.push(e.type + " " + (e.currentTarget === window));
		addEventListener("load", heard);
		addEventListener("gone", heard);
		removeEventListener("gone", heard);
		addEventListener("made", heard);
		dispatchEvent(new Event("gone"));
		dispatchEvent(new Event("made"));</script>`)
	if got := s.eval(`seen.join()`); got != "made true,load true" {
		t.Errorf("the page heard %q", got)
	}
	if said := s.said(); said != "" {
		t.Errorf("the console says %s", said)
	}
}

// press is a click on a button of the page's form: scripts are told of
// it, then ui clicks the button.
func (s *scripted) press(id string) {
	s.t.Helper()
	n := s.byID(id)
	s.rt.Click(n, 1, 1, dom.Keys{})
	s.page.Tree.Submit(web.FormOf(n), n)
}
