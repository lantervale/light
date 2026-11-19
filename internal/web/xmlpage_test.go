package web //nolint:testpackage // uses the package's unexported test helpers (answers, show, box)

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/content"
)

const xhtmlPage = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
 <head>
  <title>A &amp; B&nbsp;C</title>
  <style type="text/css"><![CDATA[
   body { margin: 0; }
   #wide { width: 300px; height: 40px; }
   p > b { display: block; width: 70px; height: 10px; }
  ]]></style>
 </head>
 <body>
  <div id="wide"/>
  <p id="after"><b id="bold">x</b></p>
 </body>
</html>`

func TestAPageSentAsXHTMLIsReadAsXML(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/page.xht":  typed("application/xhtml+xml", xhtmlPage),
		"/page.html": typed("text/html", xhtmlPage),
	})
	s := show(t)
	doc := s.visit(srv.URL + "/page.xht")
	if s.page.Title != "A & B C" || s.page.Media != "application/xhtml+xml" {
		t.Fatalf("the page is %q, %s", s.page.Title, s.page.Media)
	}
	if wide := box(t, doc, "wide"); wide.W != 300 || wide.H != 40 {
		t.Errorf("the sheet in a CDATA section gives #wide %v by %v", wide.W, wide.H)
	}
	if bold := box(t, doc, "bold"); bold.W != 70 || bold.H != 10 {
		t.Errorf("its rule with a > gives #bold %v by %v", bold.W, bold.H)
	}
	if wide, after := box(t, doc, "wide"), box(t, doc, "after"); after.Y < wide.Y+wide.H {
		t.Errorf("the paragraph is at %v, in the div closed in its start tag (%v to %v)", after.Y, wide.Y, wide.Y+wide.H)
	}

	doc = s.visit(srv.URL + "/page.html")
	if wide := box(t, doc, "wide"); wide.W == 300 {
		t.Errorf("the same bytes sent as text/html are read as XML: #wide is %v wide", wide.W)
	}
	if after := doc.ByID("after"); after == nil || after.Parent() != doc.ByID("wide") {
		t.Errorf("sent as text/html, the div closed in its start tag does not hold what follows")
	}
}

func TestAPageOfXMLThatIsNotWellFormedIsNotShown(t *testing.T) {
	srv, _ := answers(t, map[string]answer{
		"/broken.xht": typed("application/xhtml+xml",
			"<html xmlns=\"http://www.w3.org/1999/xhtml\"><body>\n<p>shown?<br></p></body></html>"),
	})
	page := testLoader().Load(t.Context(), srv.URL+"/broken.xht", Links{})
	if page.Err == nil || page.Doc != nil {
		t.Fatalf("the page is shown: %v", page.Err)
	}
	for _, said := range []string{"not well-formed", "line 2, column 18", "</p> closes the element br"} {
		if !strings.Contains(page.Err.Error(), said) {
			t.Errorf("%q does not say %q", page.Err, said)
		}
	}
}

func TestADocumentOfXMLThatIsNoPageKeepsItsTreeForScripts(t *testing.T) {
	const feed = `<?xml version="1.0"?><!-- before --><feed xmlns:a="urn:a"><a:entry id="e">one</a:entry></feed>`
	by := map[string]answer{
		"/feed.xml":   typed("application/xml", feed),
		"/text.xml":   typed("text/xml; charset=utf-8", feed),
		"/feed.atom":  typed("application/atom+xml", feed),
		"/broken.xml": typed("application/xml", `<feed><entry></feed>`),
		"/plain.txt":  typed("text/plain", feed),
	}
	for _, path := range []string{"/feed.xml", "/text.xml", "/feed.atom"} {
		page, _ := scripted(t, by, path)
		if page.Kind != content.Text || page.Source != feed || page.Doc != nil {
			t.Fatalf("%s is %v, shown as %q, with a Document: %t", path, page.Kind, page.Source, page.Doc != nil)
		}
		tree := page.Tree
		if tree == nil || !tree.XML() || tree.Document() == nil || page.Base == nil || page.Base.Path != path {
			t.Fatalf("%s has a tree read as XML for scripts: %t, from %v", path, tree != nil, page.Base)
		}
		entry := tree.byID("e")
		if entry == nil || entry.Data != "entry" || entry.Namespace != "urn:a" || entry.Parent.Data != "feed" ||
			entry.Parent.Parent != tree.Root() {
			t.Fatalf("%s: the tree holds %v", path, entry)
		}
		added := tree.Element("more", "")
		if entered := tree.Insert(entry.Parent, added, nil); len(entered) != 0 || tree.Node(added) != nil ||
			!tree.Connected(added) {
			t.Errorf("%s: an element put in the tree is shown, or is not in it", path)
		}
		tree.SetAttr(added, "id", "m")
		tree.Remove(added)
		if tree.Connected(added) || len(tree.Document().Body().Children()) != 0 {
			t.Errorf("%s: what scripts change in the tree shows somewhere", path)
		}
	}
	for _, path := range []string{"/broken.xml", "/plain.txt"} {
		if page, _ := scripted(t, by, path); page.Kind != content.Text || page.Tree != nil || page.Source == "" {
			t.Errorf("%s is %v with a tree: %t", path, page.Kind, page.Tree != nil)
		}
	}
	srv, _ := answers(t, by)
	if page := testLoader().Load(t.Context(), srv.URL+"/feed.xml", Links{}); page.Tree != nil {
		t.Error("a document loaded for no script has a tree")
	}
}
