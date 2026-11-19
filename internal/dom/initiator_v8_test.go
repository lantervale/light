//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/lantervale/light/internal/dom"
)

func TestARequestOfAScriptSaysWhereItWasAskedFrom(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start(`<script>var done = 0;
		fetch("/data.json").then(() => done++);
		const xhr = new XMLHttpRequest(); xhr.open("GET", "/list.json"); xhr.onloadend = () => done++;
		xhr.send();
		fetch("/collect", { method: "POST", body: "x" }).then(() => done++);</script>`, dom.Host{})
	s.frames("the answers", func() bool { return s.eval(`done`) == "3" })
	var asked []string
	for _, q := range s.fetched {
		asked = append(asked, fmt.Sprintf("%s %s from %s:%d", q.Method, q.Address, q.Source, q.Line))
	}
	slices.Sort(asked)
	want := []string{
		"GET https://site.test/data.json from " + address + ":2",
		"GET https://site.test/list.json from " + address + ":4",
		"POST https://site.test/collect from " + address + ":5",
	}
	if !slices.Equal(asked, want) {
		t.Errorf("the page asked %q, want %q", asked, want)
	}
}
