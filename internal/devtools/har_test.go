package devtools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/pkg/har"
	"github.com/lantervale/light/pkg/netlog"
)

func (s *shown) archive(id int, sensitive bool) har.Archive {
	s.t.Helper()
	data, err := s.panel.HAR(id, sensitive)
	if err != nil {
		s.t.Fatal(err)
	}
	if err = har.Validate(data); err != nil {
		s.t.Fatalf("not a HAR 1.2 file:\n%v", err)
	}
	var a har.Archive
	if err = json.Unmarshal(data, &a); err != nil {
		s.t.Fatal(err)
	}
	return a
}

func TestTheNetworkBarHasAButtonThatSavesAHARFile(t *testing.T) {
	s := show(t)
	s.click(devtools.ExportID)
	if s.exported != 1 {
		t.Fatalf("a click on the button asked the screen to save %d times", s.exported)
	}
	if s.named != "" {
		t.Fatalf("after a click the button still says %q under the pointer", s.named)
	}
	button := s.node(devtools.ExportID)
	if label, _ := button.Attr("aria-label"); label != "Save all as HAR" || button.Tag().String() != "button" ||
		!button.HasClass("dt-glyph") {
		t.Fatalf("the button is a %s named %q", button.Tag(), label)
	}
	bar := button.Parent()
	if !bar.HasClass("dt-bar") || bar.Children()[0].ID() != "dt-clear" || bar.Children()[1] != button {
		t.Fatal("the button is not after the bin in the Network bar")
	}
	s.in.SetMouse(s.node("dt-filter").Box().Center())
	s.frame()
	s.in.SetMouse(button.Box().Center())
	s.frame()
	s.frame()
	if !strings.Contains(s.named, "cookies and credentials redacted") || !strings.Contains(s.named, "a form's fields") {
		t.Fatalf("under the pointer the button says %q", s.named)
	}
	s.clean()
}

func TestThePanelsHARIsOfTheRequestsListed(t *testing.T) {
	s := show(t)
	s.log.KeepBodies(netlog.BodyCap, netlog.BodyBudget)
	s.load()
	s.log.Block("POST", s.srv.URL+"/beacon", "app.js:3", netlog.Type("beacon"), "refused")
	s.frame()
	all := s.archive(0, false)
	if len(all.Log.Entries) != 6 || len(all.Log.Pages) != 1 || len(s.panel.Listed()) != 6 {
		t.Fatalf("%d entries of %d pages, %d listed", len(all.Log.Entries), len(all.Log.Pages), len(s.panel.Listed()))
	}
	hop, last := all.Log.Entries[0], all.Log.Entries[5]
	if hop.Response.Status != 302 || hop.Response.RedirectURL != "/" || all.Log.Entries[1].Response.Content.Text !=
		"<p>\r\n\thello</p>" {
		t.Fatalf("the redirect is %+v then %+v", hop.Response, all.Log.Entries[1].Response)
	}
	if last.Kind != "beacon" || last.Blocked != "refused" || last.Response.Status != 0 {
		t.Fatalf("a blocked request of a kind the panel does not know is %+v", last)
	}

	s.click("dt-chip-stylesheet")
	sheets := s.archive(0, false)
	if len(sheets.Log.Entries) != 2 || sheets.Log.Entries[0].Kind != "stylesheet" || sheets.Log.Entries[0].Pageref !=
		all.Log.Pages[0].ID {
		t.Fatalf("with the sheets alone listed the file holds %+v", sheets.Log.Entries)
	}
	s.state.Sort = -9
	s.panel.Refresh()
	if again := s.archive(0, false); again.Log.Entries[0].Request.URL != sheets.Log.Entries[0].Request.URL {
		t.Fatal("the file follows the order of the table, not the one the requests were asked in")
	}
	picture := s.log.Rows()[4]
	one := s.archive(picture.ID, true)
	if len(one.Log.Entries) != 1 || one.Log.Entries[0].Request.URL != s.srv.URL+"/map.png" || one.Log.Comment != "" ||
		one.Log.Entries[0].Response.Content.Encoding != "base64" {
		t.Fatalf("one request's file holds %+v", one.Log.Entries)
	}
	s.clean()
}
