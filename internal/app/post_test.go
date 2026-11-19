package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/console"
)

// consentPage is a page that asks for a choice before a site, as such
// pages are written: no script, a form a button, each with its hidden
// fields and sent with a POST, twice over for wide and narrow windows,
// and the form of the page's language, whose fields a rule of the page
// would show.
const consentPage = `<!DOCTYPE html><title>Before you go on</title><style>body { margin: 0 }
.narrow { display: none }
.wide { display: inline-block; margin-top: 12px }
.button { border: none; background: #0b57d0; color: #fff; height: 40px; padding: 0 24px; margin: 4px }
.picker, .picker select, .picker input { font-size: 12px; display: inline-flex; margin: 2px }
.footer form { display: inline }</style>
<h1 id="title">Before you go on</h1>
<div class="wide"><form action="http://consent.site.test/save" method="POST" style="display:inline;">
<input type="hidden" name="gl" value="FR"><input type="hidden" name="continue" value="http://www.site.test/">
<input type="hidden" name="escs" value="AdYY5j-xyz_1"><input type="hidden" name="set_eom" value="true" id="eom">
<input type="submit" value="Tout refuser" class="button" id="no" aria-label="Tout refuser"/></form>
<form action="http://consent.site.test/save" method="POST" style="display:inline;">
<input type="hidden" name="gl" value="FR"><input type="hidden" name="continue" value="http://www.site.test/">
<input type="hidden" name="escs" value="AdYY5j-xyz_1"><input type="hidden" name="set_eom" value="false">
<input type="hidden" name="set_sc" value="true"><input type="hidden" name="set_aps" value="true">
<input type="submit" value="Tout accepter" class="button" id="yes" aria-label="Tout accepter"/></form></div>
<div class="narrow"><form action="http://consent.site.test/save" method="POST" style="display:block;">
<input type="hidden" name="set_eom" value="false"><input type="submit" value="Tout accepter" class="button"/></form>
</div>
<div class="footer"><form action="http://consent.site.test/ml" method="get" class="picker">
<select id="language" name="hl"><option value="en">English</option><option value="fr" selected>Français</option>
</select><input type="hidden" name="oldhl" value="fr" id="oldhl"><input type="HIDDEN" name="gl" value="FR" id="gl">
<input type="hidden" name="continue" value="http://www.site.test/" id="continue" style="display: block">
<input type="submit" value="Changer de langue" id="change"></form></div>`

// saved is what a site was sent with a POST, each a line: its path, its
// type, its origin, its referrer, its cookies and what it carried.
type saved struct {
	sync.Mutex
	posts []string
}

func (k *saved) lines() []string {
	k.Lock()
	defer k.Unlock()
	return slices.Clone(k.posts)
}

// consenting is a site that asks for a choice at consent.site.test/ml,
// takes it at /save, where it sets a cookie that says it and sends the
// tab on to www.site.test with the status redirect, and answers a POST
// at /echo and at /form with a page of its own; the form at /scripted
// is held, changed and sent by the page's scripts.
func consenting(t *testing.T, redirect int) (*hosts, *saved) {
	t.Helper()
	got := &saved{}
	return names(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			got.Lock()
			got.posts = append(got.posts, strings.Join([]string{r.URL.Path, r.Header.Get("Content-Type"),
				r.Header.Get("Origin"), r.Header.Get("Referer"), r.Header.Get("Cookie"), string(body)}, " | "))
			got.Unlock()
		}
		w.Header().Set("Content-Type", "text/html")
		switch r.Host + r.URL.Path {
		case "consent.site.test/ml":
			w.Header().Add("Set-Cookie", "seen=1; Path=/; Domain=site.test")
			_, _ = io.WriteString(w, consentPage)
		case "consent.site.test/save":
			choice := "refused"
			if strings.Contains(r.URL.RawQuery+"&"+got.lines()[len(got.lines())-1], "set_eom=false") {
				choice = "accepted"
			}
			w.Header().Add("Set-Cookie", "SOCS="+choice+"; Path=/; Domain=site.test; Max-Age=3600; SameSite=Lax")
			w.Header().Set("Location", "http://www.site.test/?ucbcb=1")
			w.WriteHeader(redirect)
		case "consent.site.test/form":
			_, _ = io.WriteString(w, `<title>`+r.Method+` form</title><style>body { margin: 0 }</style>
				<form method="post"><input name="a" value="1"><input type="submit" id="again" value="Again"></form>
				<form method="post" action="/echo" enctype="multipart/form-data"><input name="a" value="1">
				<input type="submit" id="file" value="File"></form>
				<form method="post" action="/echo"><input name="b" value="2"><input type="submit" id="echo"
				value="Echo"></form>`)
		case "consent.site.test/scripted":
			_, _ = io.WriteString(w, `<title>Scripted</title><style>body { margin: 0 } input, button { display: block;
				height: 24px }</style><form id="f" method="post" action="/echo"><input name="a" value="1">
				<input type="submit" id="go" name="by" value="go"><input type="submit" id="held" name="by" value="held">
				<input type="submit" id="checked" name="by" value="checked"></form>
				<button id="script">submit()</button><button id="request">requestSubmit()</button>
				<script>held.addEventListener("click", (e) => e.preventDefault());
				f.addEventListener("submit", (e) => {
					if (e.submitter === checked) e.preventDefault(); else f.a.value = "seen";
				});
				script.onclick = () => f.submit();
				request.onclick = () => f.requestSubmit(go);</script>`)
		case "consent.site.test/echo":
			_, _ = io.WriteString(w, `<title>`+r.Method+` echo</title><p id="echoed">echoed</p>`)
		default:
			_, _ = io.WriteString(w, `<title>The site</title><p id="in">in</p>`)
		}
	}), got
}

// cookies is the Cookie header the last request for that host and path
// came with.
func (n *hosts) cookies(to string) string {
	n.Lock()
	defer n.Unlock()
	return n.got[to]
}

const consentAddress = "http://consent.site.test/ml?continue=http://www.site.test/&gl=FR"

func TestAChoiceIsPostedByItsOwnButtonAndItsCookieKept(t *testing.T) {
	const common = "gl=FR&continue=http%3A%2F%2Fwww.site.test%2F&escs=AdYY5j-xyz_1&"
	for _, c := range []struct{ button, carried, cookie string }{
		{"no", common + "set_eom=true", "refused"},
		{"yes", common + "set_eom=false&set_sc=true&set_aps=true", "accepted"},
	} {
		t.Run(c.button, func(t *testing.T) {
			n, got := consenting(t, http.StatusSeeOther)
			s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
			s.visit(consentAddress)
			s.clickAt(s.inPage(c.button))
			const landed = "http://www.site.test/?ucbcb=1"
			s.until("the choice led to the site", func() bool { return s.session().pages[landed] != nil })
			s.at(landed)
			want := []string{"/save | application/x-www-form-urlencoded | http://consent.site.test | " +
				consentAddress + " | seen=1 | " + c.carried}
			if posts := got.lines(); !slices.Equal(posts, want) {
				t.Fatalf("the site was sent\n%s\nwant\n%s", strings.Join(posts, "\n"), strings.Join(want, "\n"))
			}
			if header := n.cookies("www.site.test/"); header != "seen=1; SOCS="+c.cookie {
				t.Errorf("the site was then asked with the cookies %q", header)
			}
			var rows []string
			for _, r := range s.session().log.Rows() {
				rows = append(rows, r.Method+" "+r.Address+" "+http.StatusText(r.Status))
			}
			if listed := []string{"POST http://consent.site.test/save See Other", "GET " + landed + " OK"}; len(rows) < 2 ||
				!slices.Equal(rows[:2], listed) {
				t.Errorf("the Network panel lists %q, want %q", rows, listed)
			}
			s.click("back")
			s.at(consentAddress)
			s.click("forward")
			s.at(landed)
			if posts := got.lines(); len(posts) != 1 {
				t.Errorf("going back and forward sent the choice %d times", len(posts))
			}
			for _, l := range s.session().said.Lines() {
				t.Errorf("the console says %s: %s", l.Level, l.Text)
			}
		})
	}
}

func TestAHiddenFieldOfAPageIsNeverShown(t *testing.T) {
	n, _ := consenting(t, http.StatusSeeOther)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	doc := s.visit(consentAddress)
	for _, id := range []string{"eom", "oldhl", "gl", "continue"} {
		if at, ok := doc.BoxOf(doc.ByID(id)); ok {
			t.Errorf("the hidden field #%s is shown at %v", id, at)
		}
	}
	for _, id := range []string{"language", "change", "yes", "no"} {
		box(t, doc, id)
	}
}

func TestAPostIsRedirectedAsItsStatusSays(t *testing.T) {
	for status, again := range map[int]bool{http.StatusMovedPermanently: false, http.StatusFound: false,
		http.StatusTemporaryRedirect: true, http.StatusPermanentRedirect: true} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			n, got := consenting(t, status)
			s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
			s.visit(consentAddress)
			s.clickAt(s.inPage("yes"))
			const landed = "http://www.site.test/?ucbcb=1"
			s.until("the choice led to the site", func() bool { return s.session().pages[landed] != nil })
			posts := got.lines()
			if sentOn := len(posts) == 2 && strings.HasPrefix(posts[1], "/ | application/x-www-form-urlencoded | null |") &&
				strings.HasSuffix(posts[1], "set_aps=true"); sentOn != again || len(posts) > 2 {
				t.Errorf("the site was sent\n%s", strings.Join(posts, "\n"))
			}
		})
	}
}

func TestAPostedPageIsFetchedAgainWithAGet(t *testing.T) {
	n, got := consenting(t, http.StatusSeeOther)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	const form, echo = "http://consent.site.test/form", "http://consent.site.test/echo"
	s.visit(form)
	s.clickAt(s.inPage("echo"))
	s.until("the form was answered", func() bool { return s.title(echo) == "POST echo" })
	s.at(echo)
	s.click("reload")
	s.until("the page was fetched again", func() bool { return s.title(echo) == "GET echo" })
	s.click("back")
	s.until("the form is back", func() bool { return s.title(form) == "GET form" })
	s.clickAt(s.inPage("again"))
	s.until("the form was sent to its own page", func() bool { return s.title(form) == "POST form" })
	s.at(form)
	if s.browser.Tab().Len() != 3 {
		t.Errorf("the tab went through %d pages, want the start page, the form's and the echo", s.browser.Tab().Len())
	}
	s.click("reload")
	s.until("the page was fetched again", func() bool { return s.title(form) == "GET form" })
	want := []string{"/echo | application/x-www-form-urlencoded | http://consent.site.test | " + form + " |  | b=2",
		"/form | application/x-www-form-urlencoded | http://consent.site.test | " + form + " |  | a=1"}
	if posts := got.lines(); !slices.Equal(posts, want) {
		t.Fatalf("the site was sent\n%s\nwant\n%s", strings.Join(posts, "\n"), strings.Join(want, "\n"))
	}
}

func TestAFormLightDoesNotSendIsSaidInTheConsole(t *testing.T) {
	n, got := consenting(t, http.StatusSeeOther)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	const form = "http://consent.site.test/form"
	s.visit(form)
	s.clickAt(s.inPage("file"))
	s.frame()
	s.at(form)
	lines := s.session().said.Lines()
	if len(lines) != 1 || lines[0].Level != console.Warn ||
		lines[0].Text != "A form was not sent: light sends no form as multipart/form-data, and so no file." {
		t.Errorf("the console says %v", lines)
	}
	if posts := got.lines(); len(posts) != 0 {
		t.Errorf("the site was sent %q", posts)
	}
}
