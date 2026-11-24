//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its tabs, its sessions, its frames

import "testing"

func TestAScriptOfASandboxedFrameSendsNoFormOutOfItsSandbox(t *testing.T) {
	f, s := escaping(t, "allow-scripts", `<button id="script">submit()</button>
		<script>document.getElementById("script").onclick = () => document.getElementById("post").submit();</script>`)
	s.refused(f, "script")
	s.clickAt(s.inFrame(s.frameAt("free"), "script"))
	const sent = "https://other.test/sent"
	if got, want := s.opened(), "https://site.test/ ["+sent+"]"; got != want {
		t.Fatalf("a script of a frame allowed to escape left the tabs as %s, want %s", got, want)
	}
}
