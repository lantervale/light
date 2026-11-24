//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the driver is checked against the screens it drives: their tabs, their windows

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/tabs"
	"github.com/lantervale/light/pkg/webdriver"
)

func TestTheDriverAnswersWhatAPageAsks(t *testing.T) {
	d, _ := drivenAt(t, "/")
	if got := d.run(`window.answered = [alert("hello")]; return "after"`); got != `null` || d.doc.ByID("asking") == nil {
		t.Fatalf("a script that asks is answered %s, dialog shown: %v", got, d.doc.ByID("asking") != nil)
	}
	if got := d.text("/{s}/alert/text"); got != "hello" {
		t.Fatalf("the alert's text is %q", got)
	}
	d.fails(webdriver.ElementNotInteractable, "POST", "/{s}/alert/text", `{"text":"typed"}`)
	d.fails(webdriver.InvalidArgument, "POST", "/{s}/alert/text", `{"text":5}`)
	d.ok("POST", "/{s}/alert/accept", "{}")
	if d.doc.ByID("asking") != nil {
		t.Fatal("an accepted alert still shows")
	}
	d.fails(webdriver.NoSuchAlert, "POST", "/{s}/alert/accept", "{}")
	d.fails(webdriver.NoSuchAlert, "POST", "/{s}/alert/dismiss", "{}")
	d.fails(webdriver.NoSuchAlert, "GET", "/{s}/alert/text", "")
	d.fails(webdriver.NoSuchAlert, "POST", "/{s}/alert/text", `{"text":"a"}`)
	for _, c := range []struct{ asks, act, send, want string }{
		{`confirm("sure?")`, "accept", "", `true`},
		{`confirm("sure?")`, "dismiss", "", `false`},
		{`prompt("name?", "nobody")`, "accept", "", `"nobody"`},
		{`prompt("name?", "nobody")`, "accept", "Aldric", `"Aldric"`},
		{`prompt("name?", "nobody")`, "dismiss", "Aldric", `null`},
	} {
		if got := d.run(`window.answered = ` + c.asks); got != `null` {
			t.Fatalf("%s is answered %s before its prompt is", c.asks, got)
		}
		d.fails(webdriver.UnexpectedAlertOpen, "POST", "/{s}/execute/sync", `{"script":"return 1","args":[]}`)
		if d.doc.ByID("asking") != nil {
			t.Fatal("a command that met a prompt left it open, with nothing asked of the session")
		}
		d.fails(webdriver.NoSuchAlert, "GET", "/{s}/alert/text", "")
		d.run(`window.answered = ` + c.asks)
		if c.send != "" {
			d.ok("POST", "/{s}/alert/text", `{"text":"`+c.send+`"}`)
			if got := d.doc.Value(d.doc.ByID("asking-answer")); got != c.send {
				t.Fatalf("the prompt's field holds %q", got)
			}
		}
		d.ok("POST", "/{s}/alert/"+c.act, "{}")
		if got := d.run(`return window.answered`); got != c.want {
			t.Errorf("%s then %s (%q sent) answers the page %s, want %s", c.asks, c.act, c.send, got, c.want)
		}
	}
	async := d.ok("POST", "/{s}/execute/async", `{"script":"const done = arguments[0]; setTimeout(() => {`+
		` window.late = confirm('late?'); done('ended'); }, 30)","args":[]}`)
	if async != `null` || d.text("/{s}/alert/text") != "late?" {
		t.Fatalf("an async script that asks is answered %s", async)
	}
	d.ok("POST", "/{s}/alert/accept", "{}")
	if got := d.run(`return window.late`); got != `true` {
		t.Fatalf("the async script was answered %s", got)
	}
	d.clean()
}

func TestAPromptNobodyExpectedIsHandledAsTheSessionAsked(t *testing.T) {
	for _, c := range []struct {
		asked        string
		fails, stays bool
		answered     string
	}{
		{``, true, false, `false`},
		{`"dismiss"`, false, false, `false`},
		{`"accept"`, false, false, `true`},
		{`"dismiss and notify"`, true, false, `false`},
		{`"accept and notify"`, true, false, `true`},
		{`"ignore"`, true, true, `true`},
		{`{"confirm":"accept","default":"dismiss"}`, false, false, `true`},
		{`{"alert":"accept","default":"dismiss and notify"}`, true, false, `false`},
	} {
		srv := drivable(t)
		d := drive(t)
		capabilities := `{}`
		if c.asked != "" {
			capabilities = `{"alwaysMatch":{"unhandledPromptBehavior":` + c.asked + `}}`
		}
		d.begin(capabilities)
		d.open(srv.URL + "/")
		d.run(`window.answered = confirm("sure?")`)
		a := d.ask("GET", "/{s}/title", "")
		switch {
		case c.fails && (a.code != string(webdriver.UnexpectedAlertOpen) ||
			!strings.Contains(a.text, `"data":{"text":"sure?"}`)):
			t.Errorf("with %s a command that meets a confirm is answered %s", c.asked, a.text)
		case !c.fails && a.text != `"Driven"`:
			t.Errorf("with %s a command that meets a confirm is answered %s", c.asked, a.text)
		}
		if open := d.doc.ByID("asking") != nil; open != c.stays {
			t.Errorf("with %s the confirm is left open: %v", c.asked, open)
		}
		if c.stays {
			d.ok("POST", "/{s}/alert/accept", "{}")
		}
		if got := d.run(`return window.answered`); got != c.answered {
			t.Errorf("with %s the page is answered %s, want %s", c.asked, got, c.answered)
		}
	}
}

func TestAPageThatAsksAsItLoadsIsDrivenOn(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Asks</title><p id="got">none</p><script>
			document.getElementById("got").textContent = String(confirm("come in?"));</script>`),
	})
	d := drive(t)
	d.begin(`{"alwaysMatch":{"unhandledPromptBehavior":"ignore"}}`)
	d.open(srv.URL + "/")
	if got := d.text("/{s}/alert/text"); got != "come in?" {
		t.Fatalf("the page asks %q as it loads", got)
	}
	d.ok("POST", "/{s}/alert/accept", "{}")
	if got := d.run(`return document.getElementById("got").textContent`); got != `"true"` {
		t.Fatalf("the page was answered %s", got)
	}
	d.open(srv.URL + "/")
	d.ok("DELETE", "/{s}", "")
	d.frame()
	d.frame()
	if d.doc.ByID("asking") != nil || d.browser.Tab().Current() != tabs.Home {
		t.Fatalf("after its session light still asks, at %s", d.browser.Tab().Current())
	}
	d.clean()
}
