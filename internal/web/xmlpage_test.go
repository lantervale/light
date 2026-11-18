package web //nolint:testpackage // uses the package's unexported test helpers (answers, show, box)

import (
	"strings"
	"testing"
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
