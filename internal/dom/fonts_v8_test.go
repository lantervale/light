//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
)

func TestDocumentFontsTellsScriptsOfThePagesFontsAsTheyLand(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		if r.URL.Path == "/heavy.ttf" {
			_, _ = w.Write(gobold.TTF)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	s := run(t, `<style>
		@font-face { font-family: Heavy; src: url(`+srv.URL+`/heavy.ttf); }
		@font-face { font-family: Gone; src: url(`+srv.URL+`/gone.woff2); }
		p { font-family: Heavy, sans-serif; }</style><p><span id="text">text</span></p><script>
		window.log = [];
		document.fonts.addEventListener("loading", () => log.push("loading"));
		document.fonts.onloadingdone = () => log.push("done");
		document.fonts.ready.then((set) => log.push("ready " + set.status + " " + (set === document.fonts)));
		document.fonts.load("16px Heavy").then((faces) => log.push("loaded " + faces.length));
		document.fonts.load("bold 16px Gone", "abc").then(() => log.push("gone"));
	</script>`)
	if got := s.eval(`[document.fonts.status, document.fonts.check("16px Heavy"), document.fonts.check("16px serif"),
		document.fonts instanceof FontFaceSet].join()`); got != "loading,false,true,true" {
		t.Errorf("while the font is on its way: %s", got)
	}
	plain := s.byID("text").Box().W
	close(release)
	s.frames("the fonts' end", func() bool {
		told := s.eval(`log.join()`)
		return strings.Contains(told, "ready") && strings.Contains(told, "gone")
	})
	if got := s.eval(`log.filter((told) => told !== "gone").join()`); got != "loading,done,loaded 0,ready loaded true" {
		t.Errorf("the page was told %s", got)
	}
	if got := s.eval(
		`[document.fonts.status, document.fonts.check("16px Heavy"), document.fonts.check("16px Gone")].join()`,
	); got != "loaded,true,true" {
		t.Errorf("once the fonts are there: %s", got)
	}
	if heavy := s.byID("text").Box().W; heavy <= plain {
		t.Errorf("the text is %v wide in the page's font, %v before", heavy, plain)
	}
	if got := s.eval(
		`let threw = ""; try { document.fonts.check("Heavy") } catch (e) { threw = e.name } threw`,
	); got != "SyntaxError" {
		t.Errorf("a font that is no font value throws %q", got)
	}
	if got := s.eval(`let later = "no"; document.fonts.ready.then(() => { later = "yes" }); later`); got != "no" {
		t.Errorf("ready answered within the script: %s", got)
	}
	s.frames("ready of a page whose fonts are there", func() bool { return s.eval(`later`) == "yes" })
}

func TestFontsReadyAskedWhileAFontCameBetweenTwoFramesIsAnswered(t *testing.T) {
	release, came := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer close(came)
		<-release
		_, _ = w.Write(gobold.TTF)
	}))
	t.Cleanup(srv.Close)
	s := run(t, `<style>@font-face { font-family: Heavy; src: url(`+srv.URL+`/heavy.ttf); }
		p { font-family: Heavy; }</style><p>text</p>`)
	s.layout()
	asked := `window.ready = "no"; document.fonts.ready.then(() => { ready = "yes" }); document.fonts.status`
	if got := s.eval(asked); got != "loading" {
		t.Fatalf("the font is %s when ready is asked", got)
	}
	close(release)
	<-came
	for end := time.Now().Add(time.Second); s.eval(`document.fonts.status`) != "loaded" && time.Now().Before(end); {
		time.Sleep(time.Millisecond)
		s.layout()
	}
	s.frames("ready, the font landed before a frame saw it on its way", func() bool { return s.eval(`ready`) == "yes" })
}
