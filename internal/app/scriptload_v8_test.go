//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import "testing"

func TestAScriptThePageNamesTellsItsElementItRanOrCouldNot(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><head><script>window.log = [];
			window.came = function (name) { log.push(name + " load " + typeof window[name]) };</script>
			<script async src="https://other.test/late.js" onload="came('late')" onerror="log.push('late error')"></script>
			<script src="/now.js" onload="came('now')"></script>
			<script src="/missing.js" onload="log.push('missing load')" onerror="log.push('missing error')"></script>
			<script defer id="last" src="/last.js"></script>
			<script>document.getElementById("last").addEventListener("load", (e) =>
				log.push("last load " + (e.target.id === "last") + e.bubbles));
				log.push("inline");</script></head><body><p>text</p>`),
		"other.test/late.js": typed("text/javascript", `window.late = 1; log.push("late ran");`),
		"site.test/now.js":   typed("text/javascript", `window.now = 1; log.push("now ran");`),
		"site.test/last.js":  typed("text/javascript", `log.push("last ran");`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("")
	s.gives("", map[string]string{`log.join(", ")`: "now ran, now load number, missing error, inline, " +
		"late ran, late load number, last ran, last load truefalse"})
}
