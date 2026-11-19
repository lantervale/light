//go:build !(v8 && cgo && (darwin || linux) && (amd64 || arm64))

package app //nolint:testpackage // the driver is checked against the screens it drives: their tabs, their windows

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/webdriver"
)

func TestWithoutAnEngineTheDriverRunsNoScriptAndFindsNoElement(t *testing.T) {
	srv, _ := crypt(t)
	d := drive(t)
	d.begin("{}")
	d.open(srv.URL + "/")
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/{s}/execute/sync", `{"script":"return 1","args":[]}`},
		{"POST", "/{s}/execute/async", `{"script":"arguments[0](1)","args":[]}`},
		{"POST", "/{s}/element", `{"using":"css selector","value":"p"}`},
		{"POST", "/{s}/elements", `{"using":"css selector","value":"p"}`},
		{"GET", "/{s}/element/active", ""},
		{"GET", "/{s}/element/e1/text", ""},
		{"POST", "/{s}/element/e1/click", "{}"},
		{"POST", "/{s}/element/e1/value", `{"text":"a"}`},
		{"POST", "/{s}/frame", `{"id":0}`},
	} {
		a := d.fails(webdriver.UnsupportedOperation, c.method, c.path, c.body)
		if !strings.Contains(a.message, "no JavaScript engine") || !strings.Contains(a.message, "-tags v8") {
			t.Errorf("%s %s says %q", c.method, c.path, a.message)
		}
	}
	if got := d.text("/{s}/source"); !strings.Contains(got, "The lower vaults stay wet.") {
		t.Fatalf("the source of the page is %q", got)
	}
	if got := d.text("/{s}/title"); got != "A crypt" {
		t.Fatalf("the title is %q", got)
	}
}
