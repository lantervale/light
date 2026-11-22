package web //nolint:testpackage // uses the package's unexported test helpers (answers, show)

import (
	"testing"

	"github.com/lantervale/ember/ui"
)

// pictures is a site whose page offers its pictures through srcset,
// sizes and a picture's sources, each served as a PNG.
func pictures(t *testing.T) (string, *asked) {
	t.Helper()
	by := map[string]answer{"/": typed("text/html", `<style>body { margin: 0 }</style>
		<picture id="frame"><source srcset="new.avif" type="image/avif">
			<source media="(max-width: 500px)" srcset="narrow.png">
			<source srcset="wide-600.png 600w, wide-1400.png 1400w" sizes="50vw" type="image/png">
			<img id="art" src="old.png" alt="art"></picture>
		<img id="set" src="small.png" srcset="mid.png 300w, big.png 1200w" sizes="25vw" alt="set">`)}
	for _, name := range []string{"old", "narrow", "wide-600", "wide-1400", "small", "mid", "big", "other"} {
		by["/"+name+".png"] = typed("image/png", string(picture(t, 8, 8)))
	}
	srv, seen := answers(t, by)
	return srv.URL, seen
}

func TestAnImgChoosesItsPictureInUIAmongItsSrcsetAndItsPicturesSources(t *testing.T) {
	for _, scripts := range []bool{false, true} {
		t.Run(map[bool]string{false: "without scripts", true: "with scripts"}[scripts], func(t *testing.T) {
			site, seen := pictures(t)
			s := show(t)
			s.scripts = scripts
			doc := s.visit(site + "/")
			if frame := doc.ByID("frame"); frame == nil || frame.Tag().String() != "picture" {
				t.Fatalf("the picture is built as %v", frame)
			}
			var sources []string
			doc.ByID("frame").Walk(func(n *ui.Node) bool {
				if !n.IsText() && n.Tag().String() == "source" {
					srcset, _ := n.Attr("srcset")
					sources = append(sources, srcset)
				}
				return true
			})
			if len(sources) != 3 {
				t.Fatalf("the picture's sources are built as %q", sources)
			}
			for id, want := range map[string]string{"art": "wide-600.png", "set": "mid.png"} {
				s.until("#"+id+" shows "+want, func() bool {
					got, _ := ui.PictureOf(doc.ByID(id))
					return got == want && seen.has("/"+want)
				})
			}
			for _, path := range []string{"/old.png", "/new.avif", "/narrow.png", "/wide-1400.png", "/small.png",
				"/big.png"} {
				if seen.has(path) {
					t.Errorf("%s was asked for: no img shows it", path)
				}
			}
			if !scripts {
				return
			}
			s.page.Tree.SetAttr(s.page.Tree.byID("set"), "srcset", "other.png 300w")
			s.until("the img shows the srcset a script gave it", func() bool {
				got, _ := ui.PictureOf(doc.ByID("set"))
				return got == "other.png" && seen.has("/other.png")
			})
			s.page.Tree.RemoveAttr(s.page.Tree.byID("set"), "srcset")
			s.until("the img shows its src once a script took its srcset off", func() bool {
				got, _ := ui.PictureOf(doc.ByID("set"))
				return got == "small.png"
			})
			if warns := doc.Warnings(); len(warns) > 0 {
				t.Errorf("the page warns: %v", warns)
			}
		})
	}
}
