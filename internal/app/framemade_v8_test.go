//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import "testing"

// probe is what a library does to learn how an element is displayed
// by default: a frame with no address made and put under the root
// element, its document written to at once, an element measured in
// it, and the frame taken out.
const probe = `(where) => {
	const frame = document.createElement("iframe");
	frame.setAttribute("frameborder", "0");
	frame.width = 0;
	frame.height = 0;
	where.appendChild(frame);
	const steps = [];
	try {
		const inner = (frame.contentWindow || frame.contentDocument).document;
		steps.push(inner === frame.contentDocument);
		inner.write("<!doctype html><html><body>");
		inner.close();
		const made = inner.createElement("span");
		inner.body.appendChild(made);
		steps.push(frame.contentWindow.getComputedStyle(made).display, getComputedStyle(made).display);
		const block = inner.body.appendChild(inner.createElement("div"));
		steps.push(getComputedStyle(block).display);
	} catch (e) {
		steps.push("throws " + e);
	}
	where.removeChild(frame);
	return steps.join();
}`

func TestAFrameAScriptMakesHasItsBlankDocumentAtOnce(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><script>var probe = ` + probe + `;
			window.early = probe(document.documentElement);</script><body><p>text</p>
			<script>window.parsing = probe(document.documentElement) + " " + probe(document.body);</script>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("")
	const want = "true,inline,inline,block"
	s.gives("", map[string]string{
		`early`:                           want,
		`parsing`:                         want + " " + want,
		`probe(document.documentElement)`: want,
		`probe(document.body)`:            want,
		`document.querySelectorAll("iframe").length`: "0",
	})
}
