//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/lantervale/light/internal/dom"
)

func TestARequestOfAScriptSaysWhereItWasAskedFrom(t *testing.T) {
	var refused []dom.Request
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start(`<script>var done = 0;
		fetch("/data.json").then(() => done++);
		const xhr = new XMLHttpRequest(); xhr.open("GET", "/list.json"); xhr.onloadend = () => done++;
		xhr.send();
		fetch("/collect", { method: "POST", body: "x" }).catch(() => done++);</script>`,
		dom.Host{Refused: func(q dom.Request) { refused = append(refused, q) }})
	s.frames("the answers", func() bool { return s.eval(`done`) == "3" })
	var asked []string
	for _, q := range s.fetched {
		asked = append(asked, fmt.Sprintf("%s %s from %s:%d", q.Method, q.Address, q.Source, q.Line))
	}
	slices.Sort(asked)
	want := []string{
		"GET https://site.test/data.json from " + address + ":2",
		"GET https://site.test/list.json from " + address + ":4",
	}
	if !slices.Equal(asked, want) {
		t.Errorf("the page asked %q, want %q", asked, want)
	}
	if len(refused) != 1 || refused[0].Method != http.MethodPost || refused[0].Address != "https://site.test/collect" ||
		refused[0].Source != address || refused[0].Line != 5 {
		t.Errorf("light was told it refused %+v", refused)
	}
}
