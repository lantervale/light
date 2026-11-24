//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"
)

const feedXML = `<?xml version="1.0"?><feed xmlns:a="urn:a"><a:entry id="e">one</a:entry><entry>two</entry></feed>`

func TestAFrameOfXMLIsADocumentItsKinRead(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Top</title><iframe id="x" src="/feed.xml" onload="window.loaded = 1"></iframe>
			<iframe id="atom" src="/feed.atom"></iframe><iframe id="broken" src="/broken.xml"></iframe>
			<iframe id="text" src="/notes.txt"></iframe><iframe id="far" src="https://other.test/feed.xml"></iframe>
			<iframe id="boxed" sandbox="allow-scripts" src="/feed.xml"></iframe><script>window.mark = "top";</script>`),
		"site.test/feed.xml":   typed("application/xml", feedXML),
		"site.test/feed.atom":  typed("application/atom+xml", feedXML),
		"site.test/broken.xml": typed("text/xml", `<feed><entry></feed>`),
		"site.test/notes.txt":  typed("text/plain", `notes`),
		"other.test/feed.xml":  typed("application/xml", feedXML),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("")
	for _, id := range []string{"x", "atom", "broken", "text", "far", "boxed"} {
		s.framedText(id)
	}
	s.becomes("", `String(window.loaded)`, "1")
	const at = "https://site.test/feed.xml"
	s.does("", `window.xd = x.contentDocument; window.xw = x.contentWindow; window.ad = atom.contentDocument;`)
	s.gives("", map[string]string{
		`xd.documentElement.textContent`:                               "onetwo",
		`xd.documentElement.localName + xd.contentType`:                "feedapplication/xml",
		`xd instanceof xw.XMLDocument`:                                 "true",
		`xd.URL + " " + xw.location.href`:                              at + " " + at,
		`xd.defaultView === xw`:                                        "true",
		`xw.frameElement === x && xw.parent === window`:                "true",
		`String(xd.body) + String(xd.head)`:                            "nullnull",
		`xd.getElementById("e").namespaceURI`:                          "urn:a",
		`xd.getElementById("e").prefix + xd.title`:                     "a",
		`String(xd.createElement("P").namespaceURI)`:                   "null",
		`xd.createElement("P").localName`:                              "P",
		`xd.querySelectorAll("entry").length`:                          "2",
		`xd.compatMode + xd.characterSet`:                              "CSS1CompatUTF-8",
		`new xw.XMLSerializer().serializeToString(xd.documentElement)`: feedXML[len(`<?xml version="1.0"?>`):],
		`ad.contentType + ad.firstChild.nodeName`:                      "application/atom+xmlfeed",
		`String(text.contentDocument) + String(far.contentDocument)`:   "nullnull",
		`String(boxed.contentDocument)`:                                "null",
		`far.contentWindow.document`:                                   "throws SecurityError",
	})
	s.does("", `const d = x.contentDocument;
		d.documentElement.appendChild(d.createElement("more")).textContent = "three";
		d.getElementById("e").setAttribute("seen", "yes");`)
	s.gives("", map[string]string{
		`x.contentDocument.documentElement.lastChild.nodeName + x.contentDocument.documentElement.childNodes.length`: "more3",
		`x.contentDocument.querySelector("[seen]").textContent`:                                                      "one",
	})
	if got := s.framedText("x"); !strings.Contains(got, "<a:entry") || strings.Contains(got, "three") {
		t.Errorf("the frame shows %q, want its text as it was sent", got)
	}
	if got := s.framedText("broken"); !strings.Contains(got, "<feed><entry></feed>") {
		t.Errorf("the frame of XML that is not well-formed shows %q", got)
	}
	if s.frameNamed("broken").run != nil || s.frameNamed("text").run != nil || s.frameNamed("boxed").run != nil {
		t.Error("a frame with no document for scripts got an engine")
	}
}

func TestATabOfXMLKeepsItsTextAndHasADocument(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{"site.test/feed.xml": typed("application/xml", feedXML)})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/feed.xml")
	if got, err := s.evaluate(`document.documentElement.nodeName + " " + document.contentType + " " +
		(document instanceof XMLDocument) + " " + document.querySelectorAll("entry").length`); err != nil ||
		got != `"feed application/xml true 2"` {
		t.Errorf("the console's line gives %s, %v", got, err)
	}
	if page := s.session().pages[s.browser.Tab().Current()]; page == nil || page.Doc != nil || page.Source != feedXML {
		t.Errorf("the tab shows something else than the text it was sent")
	}
}
