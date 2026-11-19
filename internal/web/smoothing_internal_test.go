package web

import (
	"testing"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/pkg/fetch"
)

func smoothing(t *testing.T, loader *Loader, source string) string {
	t.Helper()
	page := loader.Local("https://site.test/", source, Links{})
	if page.Err != nil || page.Doc == nil {
		t.Fatalf("the page is %v", page.Err)
	}
	page.Doc.Layout(geom.V2(800, 600))
	got, ok := page.Doc.ComputedValue(page.Doc.ByID("p"), "-webkit-font-smoothing")
	if !ok {
		t.Fatal("the page's text has no smoothing")
	}
	if warns := page.Doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page warns: %v", warns)
	}
	return got
}

func TestADrivenLightDrawsAPagesTextExact(t *testing.T) {
	const plain = `<p id="p">text</p>`
	for _, scripts := range []bool{false, true} {
		person := &Loader{Client: fetch.New(nil), Scripts: scripts}
		driven := &Loader{Client: fetch.New(nil), Scripts: scripts, Exact: true}
		if got := smoothing(t, person, plain); got != "auto" {
			t.Errorf("scripts %v: a person's light smooths a page's text as %q", scripts, got)
		}
		if got := smoothing(t, driven, plain); got != "antialiased" {
			t.Errorf("scripts %v: a driven light smooths a page's text as %q", scripts, got)
		}
		for _, sheet := range []string{
			`html { -webkit-font-smoothing: auto }`, `:root { -webkit-font-smoothing: auto }`,
			`* { -webkit-font-smoothing: auto }`, `body { -webkit-font-smoothing: auto }`,
		} {
			if got := smoothing(t, driven, `<style>`+sheet+`</style>`+plain); got != "auto" {
				t.Errorf("scripts %v: under the page's %s a driven light smooths its text as %q", scripts, sheet, got)
			}
		}
		if got := smoothing(t, driven, `<p id="p" style="-webkit-font-smoothing: none">text</p>`); got != "none" {
			t.Errorf("scripts %v: under the element's own style a driven light smooths its text as %q", scripts, got)
		}
	}
}
