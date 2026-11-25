//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

func TestAScriptSeesThePageAsFarAsItWasRead(t *testing.T) {
	s := run(t, `<!DOCTYPE html><html><head><script>var seen = [document.body === null,
		document.scripts.length].join();</script><title>t</title></head>
		<body><p id="one">one</p><script>seen += " " + [document.getElementById("one") !== null,
		document.getElementById("two"), document.body.lastChild.nodeName].join();
		document.body.appendChild(document.createElement("hr"));</script>
		<p id="two">two<script>seen += " " + document.getElementById("two").firstChild.data</script> after</p>
		</body></html>`)
	if got, want := s.eval(`seen`), "true,1 true,,SCRIPT two"; got != want {
		t.Errorf("the scripts saw %q, want %q", got, want)
	}
	if got, want := s.eval(`[...document.body.children].map((e) => e.nodeName).join()`),
		"P,SCRIPT,HR,P"; got != want {
		t.Errorf("the page holds %s, want %s", got, want)
	}
	if got := s.eval(`document.getElementById("two").lastChild.data`); got != " after" {
		t.Errorf("the paragraph says %q", got)
	}
	if two := s.page.Doc.ByID("two"); two == nil || s.page.Doc.Body().Parent() != s.page.Doc.HTML() {
		t.Errorf("ui shows the paragraph as %v, and its body under %v", two, s.page.Doc.Body().Parent())
	}
}
