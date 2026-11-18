package app //nolint:testpackage // uses the package's unexported test helpers (show, box)

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const xhtmlSource = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
 <head>
  <title>Borders</title>
  <style type="text/css"><![CDATA[
   body { margin: 0; }
   #wide { width: 300px; height: 40px; }
   div > p { margin: 0; height: 10px; }
  ]]></style>
 </head>
 <body>
  <div id="wide"/>
  <div><p id="after">after</p></div>
 </body>
</html>`

// xmlSite serves the same XHTML bytes as XHTML and as HTML, and an
// XHTML document that is not well-formed.
func xmlSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page.xht":
			w.Header().Set("Content-Type", "application/xhtml+xml")
			_, _ = io.WriteString(w, xhtmlSource)
		case "/page.html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, xhtmlSource)
		case "/broken.xht":
			w.Header().Set("Content-Type", "application/xhtml+xml")
			_, _ = io.WriteString(w, strings.Replace(xhtmlSource, `<div id="wide"/>`, `<div id="wide">`, 1))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPageSentAsXHTMLIsShownByXMLsRules(t *testing.T) {
	srv := xmlSite(t)
	s := show(t)
	page := s.visit(srv.URL + "/page.xht")
	if page == nil {
		t.Fatal("the XHTML page is not shown")
	}
	if got := s.title(srv.URL + "/page.xht"); got != "Borders" {
		t.Errorf("its title is %q", got)
	}
	if wide := box(t, page, "wide"); wide.W != 300 || wide.H != 40 {
		t.Errorf("the sheet in its CDATA section gives #wide %v by %v, want 300 by 40", wide.W, wide.H)
	}
	if wide, after := box(t, page, "wide"), box(t, page, "after"); after.Y != wide.Y+wide.H || after.H != 10 {
		t.Errorf("the paragraph is at %v, %v tall, want under the div closed in its start tag at %v, 10 tall", after.Y,
			after.H, wide.Y+wide.H)
	}

	page = s.visit(srv.URL + "/page.html")
	if page == nil {
		t.Fatal("the same bytes sent as text/html are not shown")
	}
	if wide := box(t, page, "wide"); wide.W == 300 {
		t.Error("the same bytes sent as text/html are read by XML's rules: the CDATA section's sheet applies")
	}
	if after := page.ByID("after"); after == nil || after.Parent().Parent() != page.ByID("wide") {
		t.Error("sent as text/html, the div closed in its start tag does not hold what follows it")
	}
}

func TestXHTMLThatIsNotWellFormedShowsWhereInsteadOfThePage(t *testing.T) {
	srv := xmlSite(t)
	s := show(t)
	s.visit(srv.URL + "/broken.xht")
	notice := s.doc.ByID("page")
	if notice == nil {
		t.Fatal("no notice is shown")
	}
	text := notice.Text()
	for _, said := range []string{"not well-formed", "line 15, column 9", "</body> closes the element div"} {
		if !strings.Contains(text, said) {
			t.Errorf("the notice %q does not say %q", text, said)
		}
	}
	if strings.Contains(text, "after") {
		t.Errorf("the notice shows what the document held: %q", text)
	}
}
