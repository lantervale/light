package web //nolint:testpackage // uses the package's unexported test helpers (answers, show)

import "testing"

func TestAPagesContentLanguageIsWhatLangReadsWhereNoLangAttributeSaysOne(t *testing.T) {
	const look = `<style>p { width: 10px } p:lang(fr) { width: 100px } p:lang(de) { width: 200px }</style>`
	said := func(language, body string) answer {
		a := typed("text/html", body)
		a.header["Content-Language"] = language
		return a
	}
	srv, _ := answers(t, map[string]answer{
		"/one":    said(" fr-BE ", look+`<p id="page">page</p><div lang="de"><p id="own">own</p></div>`),
		"/two":    said("fr, de", look+`<p id="page">page</p>`),
		"/rooted": said("fr", `<html lang="de">`+look+`<p id="page">page</p></html>`),
		"/none":   typed("text/html", look+`<p id="page">page</p>`),
	})
	for _, scripts := range []bool{false, true} {
		t.Run(map[bool]string{false: "without scripts", true: "with scripts"}[scripts], func(t *testing.T) {
			s := show(t)
			s.scripts = scripts
			for _, c := range []struct {
				path, id string
				width    float32
			}{
				{"/one", "page", 100}, {"/one", "own", 200}, {"/two", "page", 10}, {"/rooted", "page", 200},
				{"/none", "page", 10},
			} {
				doc := s.visit(srv.URL + c.path)
				if got := box(t, doc, c.id).W; got != c.width {
					t.Errorf("%s: #%s is %v wide, want %v", c.path, c.id, got, c.width)
				}
			}
		})
	}
}
