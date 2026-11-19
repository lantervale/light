package webdriver_test

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/webdriver"
)

func TestAPromptThatIsOpenIsHandledAsTheSessionAsked(t *testing.T) {
	for _, c := range []struct {
		asked, kind string
		did         string
		fails       bool
	}{
		{``, "alert", "DismissAlert", true},
		{`"dismiss"`, "confirm", "DismissAlert", false},
		{`"accept"`, "confirm", "AcceptAlert", false},
		{`"dismiss and notify"`, "prompt", "DismissAlert", true},
		{`"accept and notify"`, "prompt", "AcceptAlert", true},
		{`"ignore"`, "alert", "", true},
		{`{"alert":"accept","default":"ignore"}`, "alert", "AcceptAlert", false},
		{`{"alert":"accept","default":"ignore"}`, "confirm", "", true},
		{`{"confirm":"dismiss"}`, "prompt", "DismissAlert", true},
	} {
		d := drive(t)
		capabilities := `{}`
		if c.asked != "" {
			capabilities = `{"alwaysMatch":{"unhandledPromptBehavior":` + c.asked + `}}`
		}
		d.begin(capabilities)
		d.browser.prompt = &webdriver.Prompt{Kind: c.kind, Text: "sure?"}
		d.browser.calls = nil
		a := d.ask("GET", "/session/{s}/title", "")
		did := strings.Join(d.browser.calls, ",")
		if c.fails {
			if a.code != "unexpected alert open" || a.status != 500 || !strings.Contains(a.text, `"data":{"text":"sure?"}`) ||
				did != c.did {
				t.Errorf("with %s a command that meets a %s is answered %d %s after %q", c.asked, c.kind, a.status, a.text, did)
			}
			if open := d.browser.prompt != nil; open != (c.did == "") {
				t.Errorf("with %s the %s is left open: %v", c.asked, c.kind, open)
			}
			continue
		}
		if a.status != 200 || did != c.did+",Title" {
			t.Errorf("with %s a command that meets a %s is answered %d %s after %q", c.asked, c.kind, a.status, a.text, did)
		}
	}
}

func TestSomeCommandsLeaveAPromptAlone(t *testing.T) {
	d := drive(t)
	d.begin(`{}`)
	d.browser.prompt = &webdriver.Prompt{Kind: "prompt", Text: "name?"}
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/status", ""},
		{"GET", "/session/{s}/timeouts", ""},
		{"POST", "/session/{s}/timeouts", `{"script":1000}`},
		{"GET", "/session/{s}/window", ""},
		{"GET", "/session/{s}/window/handles", ""},
		{"POST", "/session/{s}/window", `{"handle":"w1"}`},
		{"GET", "/session/{s}/alert/text", ""},
		{"POST", "/session/{s}/alert/text", `{"text":"typed"}`},
	} {
		if a := d.ask(c.method, c.path, c.body); a.status != 200 || d.browser.prompt == nil {
			t.Fatalf("%s %s with a prompt open is answered %d %s", c.method, c.path, a.status, a.text)
		}
	}
	if a := d.ask("GET", "/session/{s}/alert/text", ""); a.text != `"name?"` {
		t.Fatalf("the prompt's text is %s", a.text)
	}
	if a := d.ask("POST", "/session/{s}/alert/text", `{"text":5}`); a.code != "invalid argument" {
		t.Fatalf("a text that is no string is answered %s", a.text)
	}
	if a := d.ask("POST", "/session/{s}/alert/accept", `{}`); a.status != 200 || d.browser.prompt != nil {
		t.Fatalf("accepting the prompt is answered %s", a.text)
	}
	if a := d.ask("POST", "/session/{s}/alert/accept", `{}`); a.code != "no such alert" {
		t.Fatalf("accepting no prompt is answered %s", a.text)
	}
	d.browser.prompt = &webdriver.Prompt{Kind: "alert", Text: "bye"}
	if a := d.ask("DELETE", "/session/{s}", ""); a.status != 200 {
		t.Fatalf("ending the session with a prompt open is answered %s", a.text)
	}
}
