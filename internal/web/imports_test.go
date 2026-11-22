package web //nolint:testpackage // uses the package's unexported test helpers (answers, show)

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

func TestAPagesImportedSheetsAreFetchedThroughTheLoaderAsTheirSheetAsksForThem(t *testing.T) {
	for _, scripts := range []bool{false, true} {
		t.Run(map[bool]string{false: "without scripts", true: "with scripts"}[scripts], func(t *testing.T) {
			srv, seen := answers(t, map[string]answer{
				"/": typed("text/html", `<link rel="stylesheet" href="css/site.css">
					<style>@import "css/inline.css"; #plain { width: 10px }</style>
					<div id="wide"></div><div id="tall"></div><div id="plain"></div>`),
				"/css/site.css":       typed("text/css", `@import url(parts/base.css) screen; #tall { width: 20px }`),
				"/css/parts/base.css": typed("text/css", `@import "deep.css"; #wide { width: 300px }`),
				"/css/parts/deep.css": typed("text/css; charset=utf-8", `#tall { height: 40px }`),
				"/css/inline.css":     typed("text/css", `@import "/css/notcss.css"; #plain { height: 50px }`),
				"/css/notcss.css":     typed("text/html", `#plain { width: 99px }`),
			})
			log := netlog.New(100)
			loader := &Loader{Client: fetch.New(&netlog.Transport{}), Scripts: scripts}
			s := show(t)
			s.page = loader.Load(netlog.With(t.Context(), log, "navigation", netlog.Document), srv.URL+"/",
				Links{Open: func(string) {}, Over: func(string) {}})
			if s.page.Err != nil {
				t.Fatal(s.page.Err)
			}
			doc := s.page.Doc
			s.until("the imported sheets style the page", func() bool {
				return doc.SheetsLoading() == 0 && box(t, doc, "wide").W == 300 && box(t, doc, "tall").H == 40 &&
					box(t, doc, "plain").H == 50
			})
			if w := box(t, doc, "plain").W; w != 10 {
				t.Errorf("#plain is %v wide: a sheet served as HTML was taken", w)
			}
			referrers := map[string]string{
				"/css/parts/base.css": srv.URL + "/css/site.css", "/css/parts/deep.css": srv.URL + "/css/parts/base.css",
				"/css/inline.css": srv.URL + "/",
			}
			for path, referrer := range referrers {
				var row *netlog.Row
				for _, r := range log.Rows() {
					if strings.HasSuffix(r.Address, path) {
						row = &r
					}
				}
				if row == nil {
					t.Errorf("%s is not in the network log; the server saw %v", path, seen.paths)
					continue
				}
				if row.Initiator != "stylesheet" || row.Type != netlog.Stylesheet || row.Status != http.StatusOK {
					t.Errorf("%s is recorded as %q %q %d, want stylesheet stylesheet 200", path, row.Initiator, row.Type,
						row.Status)
				}
				if accept := row.Request.Get("Accept"); accept != "text/css,*/*;q=0.1" {
					t.Errorf("%s was asked for with Accept: %s", path, accept)
				}
				if got := row.Request.Get("Referer"); got != referrer {
					t.Errorf("%s was asked for with Referer: %q, want %q", path, got, referrer)
				}
			}
			var warned bool
			for _, w := range doc.Warnings() {
				warned = warned || strings.Contains(w.Error(), srv.URL+"/css/notcss.css")
			}
			if !warned {
				t.Errorf("a sheet served as HTML is no warning: %v", doc.Warnings())
			}
		})
	}
}
