package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/twod"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/tabs"
)

func breaking() *ui.Node { return ui.Canvas(func(*twod.Canvas) { panic("drawn wrong") }) }

func TestAPageThatBreaksLightLeavesItsNoticeAndTheOtherTabs(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/":      html(`<p>a page</p>`),
		"site.test/other": html(`<p id="fine">another page</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/").Body().Append(breaking())
	for range 4 {
		s.frame()
	}
	text := s.node("page").Text()
	if !s.node("page").HasClass("unreachable") || !strings.Contains(text, "the page broke light: drawn wrong") {
		t.Fatalf("a page whose drawing panicked shows %q", text)
	}
	if !s.consoleSays(console.Error, "drawn wrong") {
		t.Error("the console does not hold the panic")
	}

	s.OpenInNewTab(tabs.Home)
	other := s.visit("https://site.test/other")
	if other == nil || s.host == nil || !s.doc.FrameShown(s.host) || other.ByID("fine") == nil {
		t.Fatal("another tab does not show its page after one broke light")
	}
	s.browser.Select(0)
	s.show()
	s.frame()
	if back := s.node("page").Text(); !strings.Contains(back, "the page broke light") {
		t.Fatalf("back on its tab, the page that broke light shows %q", back)
	}
}

func TestAFrameThatBreaksLightLeavesItsNoticeAndItsPage(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/":      html(`<p id="around">around</p><iframe id="one" src="/inner"></iframe>`),
		"site.test/inner": html(`<p>inner</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	doc := s.visit("https://site.test/")
	s.framedText("one")
	f := s.frameAt("one")
	f.page.Doc.Body().Append(breaking())
	for range 4 {
		s.frame()
	}
	if got := s.framedText("one"); !strings.Contains(got, "This frame is not shown: its page broke light") {
		t.Fatalf("a frame whose drawing panicked shows %q", got)
	}
	if !s.consoleSays(console.Error, "drawn wrong") {
		t.Error("the console does not hold the panic of the frame")
	}
	if s.host == nil || !s.doc.FrameShown(s.host) || doc.ByID("around") == nil ||
		s.session().pages["https://site.test/"].Err != nil {
		t.Fatal("the page around a frame that broke light is not shown any more")
	}
}
