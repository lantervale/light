//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"testing"

	"github.com/lantervale/light/pkg/console"
)

func TestAScriptReachesItsPagesIframesAndTheDocumentsOfItsOrigin(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Scripted</title><iframe id="one" name="first" src="/inner" width="200"></iframe>
			<iframe id="bad" src="javascript:alert(1)"></iframe><p id="heard"></p><script>
			const heard = document.getElementById("heard");
			for (const frame of document.querySelectorAll("iframe")) {
				frame.addEventListener("load", () => { heard.textContent += frame.id + ":load "; });
				frame.addEventListener("error", () => { heard.textContent += frame.id + ":error "; });
			}
			window.made = () => {
				const frame = document.createElement("iframe");
				frame.id = "made";
				frame.src = "/second";
				frame.addEventListener("load", () => { heard.textContent += "made:load "; });
				document.body.append(frame);
			};</script>`),
		"site.test/inner": html(`<p>inner</p><script>fetch("/ran"); parent.document.title = "taken";</script>
			<noscript><p>no script here</p></noscript>`),
		"site.test/second": html(`<p>second</p>`),
		"site.test/third":  html(`<p>third</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.until("its scripts ran", func() bool { return s.session().scripts != nil })
	if got := s.framedText("one"); got != "inner" {
		t.Fatalf("the frame shows %q, want its page as one that runs its scripts", got)
	}
	s.until("the page heard of its frames", func() bool { return s.pageText("heard") == "bad:error one:load " })
	for expression, want := range map[string]string{
		`one.contentWindow === frames[0] && one.contentWindow === first && bad.contentDocument.URL`: `"about:blank"`,
		`one.contentDocument === first.document && one.contentDocument !== document`:                "true",
		`one.contentDocument.querySelector("p").textContent + " " + first.frameElement.id`:          `"inner one"`,
		`frames.length + " " + (parent === window) + " " + (top === window) + " " + (frameElement === null)`: `"2 ` +
			`true true true"`,
		`one.src + " " + one.name + " " + one.width + " " + one.srcdoc + "|" + one.sandbox.length`: `"https://` +
			`site.test/inner first 200 |0"`,
		`one instanceof HTMLIFrameElement && document.title`: `"taken"`,
	} {
		if got, err := s.evaluate(expression); err != nil || got != want {
			t.Errorf("%s is %s (%v), want %s", expression, got, err, want)
		}
	}
	s.until("the frame's script asked for what it fetches", func() bool { return f.came("site.test/ran") })
	if s.consoleSays(console.Error, "SecurityError") || s.session().pages["https://site.test/"].Title != "taken" {
		t.Errorf("a frame of the page's own origin did not reach its document: the console says %s", s.said())
	}

	if _, err := s.evaluate(`one.src = "/second"`); err != nil {
		t.Fatal(err)
	}
	if got := s.framedTextOf("one", "https://site.test/second"); got != "second" {
		t.Errorf("a frame whose src a script set shows %q", got)
	}
	const set = `one.sandbox.add("allow-forms"); one.name = "renamed"; one.srcdoc = "<p>written</p>"`
	if _, err := s.evaluate(set); err != nil {
		t.Fatal(err)
	}
	s.until("the frame showed its srcdoc", func() bool {
		return s.frameAt("one").page != nil && s.frameAt("one").page.Address == "about:srcdoc" && s.frameAt("one").load == nil
	})
	if one := s.frameAt("one"); !one.sandbox.On || !one.sandbox.Forms || s.framesOfPage().named("renamed") != one {
		t.Errorf("the attributes a script set were not followed: %+v", one.sandbox)
	}
	if _, err := s.evaluate(`made()`); err != nil {
		t.Fatal(err)
	}
	if got := s.framedTextOf("made", "https://site.test/second"); got != "second" {
		t.Errorf("a frame a script made shows %q", got)
	}
	s.until("the page heard the frame it made load", func() bool {
		return s.pageText("heard") == "bad:error one:load one:load one:load made:load "
	})
	made := s.frameAt("made")
	if _, err := s.evaluate(`document.getElementById("made").remove()`); err != nil {
		t.Fatal(err)
	}
	s.frame()
	s.frame()
	if made.page != nil || made.live {
		t.Error("a frame a script removed still shows its document")
	}
}
