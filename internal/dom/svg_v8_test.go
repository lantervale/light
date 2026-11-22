//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
)

func (s *scripted) picture(id string) string {
	s.t.Helper()
	address, _ := ui.PictureOf(s.byID(id))
	text, err := url.PathUnescape(address)
	if err != nil {
		s.t.Fatal(err)
	}
	return text
}

func TestAnSvgAScriptChangesIsDrawnAgain(t *testing.T) {
	s := &scripted{t: t}
	s.start(`<body style="margin: 0"><svg id="icon" viewBox="0 0 24 24" width="24"><path id="shape" d="M0 0h24v24z"/></svg>
		<div id="host"></div>`, dom.Host{})
	if got := s.picture("icon"); !strings.Contains(got, `d="M0 0h24v24z"`) {
		t.Fatalf("the page's own svg is %s", got)
	}
	s.eval(`shape.setAttribute("d", "M0 0h12v12z");
		const dot = document.createElementNS("http://www.w3.org/2000/svg", "circle");
		dot.setAttribute("r", "5");
		icon.appendChild(dot);
		icon.setAttribute("viewBox", "0 0 12 12");
		icon.style.width = "60px"`)
	s.frames("the svg drawn again", func() bool {
		got := s.picture("icon")
		return strings.Contains(got, `d="M0 0h12v12z"`) && strings.Contains(got, `<circle r="5">`) &&
			strings.Contains(got, `viewBox="0 0 12 12"`)
	})
	s.layout()
	if box := s.byID("icon").Box(); box.W != 60 || box.H != 60 {
		t.Errorf("the svg is %v by %v, want 60 by 60", box.W, box.H)
	}
	s.eval(`shape.remove();
		host.innerHTML = '<svg id="made" width="16" height="16" viewBox="0 0 16 16"><rect width="16" height="16"/></svg>'`)
	s.frames("the shape gone", func() bool { return !strings.Contains(s.picture("icon"), "<path") })
	s.layout()
	if box := s.byID("made").Box(); box.W != 16 || box.H != 16 || !strings.Contains(s.picture("made"), "<rect") {
		t.Errorf("the svg a script wrote is %v by %v: %s", box.W, box.H, s.picture("made"))
	}
	if got := s.eval(
		`[made.getBoundingClientRect().width, made.firstChild.tagName, icon.children.length].join()`,
	); got != "16,rect,1" {
		t.Errorf("the script reads %s", got)
	}
	s.quiet()
}

func TestAScriptReadsAndChangesThePictureAnImgChose(t *testing.T) {
	s := &scripted{t: t}
	s.start(`<img id="pic" src="small.png" srcset="mid.png 600w, big.png 1200w" sizes="40vw"><img id="plain" src="a.png">`,
		dom.Host{})
	if got := s.eval(`[pic.currentSrc, pic.src, plain.currentSrc].join()`); got != "https://site.test/dir/mid.png,"+
		"https://site.test/dir/small.png,https://site.test/dir/a.png" {
		t.Errorf("the script reads %s", got)
	}
	s.eval(`pic.sizes = "100vw"; plain.srcset = "b.png 1x"`)
	s.frames("the imgs choosing again", func() bool {
		return s.eval(`[pic.currentSrc, plain.currentSrc, pic.getAttribute("src")].join()`) ==
			"https://site.test/dir/big.png,https://site.test/dir/b.png,small.png"
	})
	s.eval(`pic.removeAttribute("srcset")`)
	s.frames("the img showing its src without its srcset", func() bool {
		got, _ := ui.PictureOf(s.byID("pic"))
		return got == "small.png"
	})
	s.quiet()
}
