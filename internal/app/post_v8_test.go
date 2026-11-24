//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"net/http"
	"slices"
	"testing"
)

func TestAPostIsHeldChangedAndSentByThePagesScripts(t *testing.T) {
	const page, echo = "http://consent.site.test/scripted", "http://consent.site.test/echo"
	const sent = "/echo | application/x-www-form-urlencoded | http://consent.site.test | " + page + " |  | "
	for _, c := range []struct{ name, button, carried string }{
		{"a button", "go", "a=seen&by=go"},
		{"a button whose click is prevented", "held", ""},
		{"a button whose submit is prevented", "checked", ""},
		{"a script's submit", "script", "a=1"},
		{"a script's requestSubmit", "request", "a=seen&by=go"},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, got := consenting(t, http.StatusSeeOther)
			s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
			s.visit(page)
			s.until("the page's scripts run", func() bool { return s.session().scripts != nil })
			s.clickAt(s.inPage(c.button))
			if c.carried == "" {
				for range 10 {
					s.frame()
				}
				s.at(page)
				if posts := got.lines(); len(posts) != 0 {
					t.Fatalf("the site was sent %q", posts)
				}
				return
			}
			s.until("the form was answered", func() bool { return s.title(echo) == "POST echo" })
			s.at(echo)
			if posts := got.lines(); !slices.Equal(posts, []string{sent + c.carried}) {
				t.Fatalf("the site was sent %q, want %q", posts, sent+c.carried)
			}
			for _, l := range s.session().said.Lines() {
				t.Errorf("the console says %s: %s", l.Level, l.Text)
			}
		})
	}
}
