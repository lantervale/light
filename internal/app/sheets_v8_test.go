//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/netlog"
)

func TestASheetAScriptLinksToIsFetchedAppliedAndListedAsAStylesheet(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Styled</title><style>#said { display: block; width: 100px }</style>
			<p id="said">none</p><script src="/app.js"></script>`),
		"/app.js": typed("text/javascript", `const link = document.createElement("link");
			link.rel = "stylesheet";
			link.href = "/late.css";
			link.onload = () => {
				const style = document.createElement("style");
				style.textContent = "#said { height: 40px }";
				document.head.appendChild(style);
				style.sheet.insertRule("#said { margin-left: 7px }", 1);
				document.getElementById("said").textContent = "loaded " + document.styleSheets.length;
			};
			document.head.appendChild(link);`),
		"/late.css": typed("text/css", `#said { width: 240px }`),
	})
	s := show(t)
	s.toggleTools()
	s.visit(srv.URL + "/")
	s.until("the linked sheet loaded", func() bool { return s.pageText("said") == "loaded 3" })
	s.frame()
	s.frame()
	if box := s.session().pages[s.browser.Tab().Current()].Doc.ByID("said").Box(); box.W != 240 || box.H != 40 {
		t.Errorf("the paragraph is %v by %v, want 240 by 40", box.W, box.H)
	}
	var sheet netlog.Row
	for _, r := range s.session().log.Rows() {
		if strings.HasSuffix(r.Address, "/late.css") {
			sheet = r
		}
	}
	if sheet.Type != netlog.Stylesheet || sheet.Initiator != "script" || sheet.Kind() != "css" || sheet.Status != 200 ||
		seen.count("/late.css") != 1 {
		t.Errorf("the sheet's request is %s by %s, a %s answered %d, asked %d times", sheet.Type, sheet.Initiator,
			sheet.Kind(), sheet.Status, seen.count("/late.css"))
	}
	s.clean()
}
