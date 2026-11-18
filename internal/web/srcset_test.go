package web //nolint:testpackage // the choice among a srcset's candidates tested is unexported

import (
	"testing"

	"github.com/lantervale/light/pkg/fetch"
)

func TestTheCandidateOfASrcsetIsTheSmallestThatFillsItsSlot(t *testing.T) {
	const widths = "a-240.jpg 240w, a-480.jpg 480w,a-800.jpg 800w ,\n a-1536.jpg 1536w"
	for _, c := range []struct {
		name, srcset, sizes string
		view                float32
		want                string
	}{
		{"the whole view without sizes", widths, "", 700, "a-800.jpg"},
		{"the first size whose condition holds", widths, "(min-width: 768px) 50vw, 100vw", 900, "a-480.jpg"},
		{"the last size otherwise", widths, "(min-width: 768px) 50vw, 100vw", 400, "a-480.jpg"},
		{"a length in pixels", widths, "(max-width: 600px) 200px, 30em", 500, "a-240.jpg"},
		{"a length in em", widths, "(max-width: 600px) 200px, 30em", 900, "a-480.jpg"},
		{"the largest when none is wide enough", widths, "", 4000, "a-1536.jpg"},
		{"a condition light does not read does not hold", widths, "(orientation: portrait) 10px, 700px", 900, "a-800.jpg"},
		{"densities: the one for a pixel a point", "a.png, a@2x.png 2x, a@3x.png 3x", "", 900, "a.png"},
		{"densities without a plain one", "a@1.5x.png 1.5x, a@2x.png 2x", "", 900, "a@1.5x.png"},
		{"an address holding commas", "https://x.test/i,w_200/a.jpg 200w, https://x.test/i,w_900/a.jpg 900w", "", 800,
			"https://x.test/i,w_900/a.jpg"},
		{"nothing read", " , ", "", 800, ""},
	} {
		if got := candidate(c.srcset, c.sizes, c.view); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAnImgShowsTheCandidateOfItsSrcsetOrOfItsPicturesSource(t *testing.T) {
	loader := &Loader{Client: fetch.New(nil), NoPictures: true, Viewport: func() (float32, float32) { return 1000, 700 }}
	page := loader.Local("https://site.test/", `<img id="plain" src="small.png">
		<img id="set" src="small.png" srcset="mid.png 600w, big.png 1200w" sizes="50vw">
		<img id="bare" srcset="only.png 2x, one.png">
		<picture><source srcset="new.avif" type="image/avif"><source media="(max-width: 500px)" srcset="narrow.webp">
			<source srcset="wide-600.webp 600w, wide-1400.webp 1400w" type="image/webp"><img id="art" src="old.jpg"></picture>
		<picture><source media="(min-width: 800px)" srcset="desk.png"><img id="desk" src="phone.png"></picture>`, Links{})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	for id, want := range map[string]string{"plain": "small.png", "set": "mid.png", "bare": "one.png",
		"art": "wide-1400.webp", "desk": "desk.png"} {
		if got, _ := page.Doc.ByID(id).Attr("src"); got != want {
			t.Errorf("#%s shows %q, want %q", id, got, want)
		}
	}
}
