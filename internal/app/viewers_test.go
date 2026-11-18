package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember/input"
)

func TestAPictureIsShownAloneFittedToTheView(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/map.png")
	if got := s.title(srv.URL + "/map.png"); got != "map.png (30×20)" {
		t.Fatalf("the tab is named %q", got)
	}
	s.until("the picture is drawn at its own size", func() bool {
		r, ok := s.doc.BoxOf(s.node("shown-picture"))
		return ok && r.W == 30 && r.H == 20
	})
	view, _ := s.doc.BoxOf(s.view)
	pic, _ := s.doc.BoxOf(s.node("shown-picture"))
	if pic.Center().Sub(view.Center()).Len() > 1 {
		t.Fatalf("the picture at %v is not in the middle of the view %v", pic, view)
	}
	s.clean()

	s.visit(srv.URL + "/wide.png")
	s.until("a picture wider than the view is fitted to it", func() bool {
		r, ok := s.doc.BoxOf(s.node("shown-picture"))
		return ok && r.W == view.W && r.H == view.W/10
	})
	s.click("picture")
	s.until("a click shows it at its own size", func() bool {
		r, _ := s.doc.BoxOf(s.node("shown-picture"))
		return r.W == 3000 && r.H == 300
	})
	s.clickAt(view.Center())
	s.until("another fits it again", func() bool {
		r, _ := s.doc.BoxOf(s.node("shown-picture"))
		return r.W == view.W
	})

	s.visit(srv.URL + "/tall.png")
	s.until("a picture taller than the view is fitted to it", func() bool {
		r, ok := s.doc.BoxOf(s.node("shown-picture"))
		return ok && r.H == view.H && r.W == view.H/10 && r.Y == view.Y
	})
	if page := s.box("page"); page.W != view.W || page.H != view.H {
		t.Fatalf("the dark page %v does not fill the view %v", page, view)
	}
	s.clean()
}

func TestTextIsShownAsItWasSentUnwrappedAndSelectable(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/app.js")
	text := s.node("text")
	if got := text.Text(); !strings.HasPrefix(got, "let a = 1;\nxxxx") {
		t.Fatalf("the text reads %q…", got[:20])
	}
	view, _ := s.doc.BoxOf(s.view)
	if box := text.Box(); box.W <= view.W || box.H > 80 {
		t.Fatalf("a line of 4000 letters is laid out %v wide and %v high in a view %v wide: it wrapped", box.W,
			box.H, view.W)
	}
	if !s.doc.SelectContents(text) || !strings.HasPrefix(s.doc.Selection().Text, "let a = 1;") {
		t.Fatalf("the text takes no selection: %q", s.doc.Selection().Text)
	}
	if got := s.title(srv.URL + "/app.js"); got != "app.js" {
		t.Errorf("the tab is named %q", got)
	}
	s.clean()
}

func TestTabsInTextAreShownAsSpacesToTheNextStopOfEight(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/tabs.txt")
	if got, want := s.node("text").Text(), "a       b\n        indented\n12345678        end"; got != want {
		t.Fatalf("the text reads %q, want %q", got, want)
	}
	s.enter("view-source:" + srv.URL + "/tabs.txt")
	s.until("the source landed", func() bool { return s.doc.ByID("source") != nil })
	if got := s.node("source").Text(); !strings.HasPrefix(got, "a       b\n") {
		t.Fatalf("the source reads %q", got)
	}
}

func TestLinesEndedTheWindowsWayShowNoStrayCharacter(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/dos.css")
	if got, want := s.node("text").Text(), "a {\n        color: red;\n}\n"; got != want {
		t.Fatalf("the text reads %q, want %q", got, want)
	}
	s.enter("view-source:" + srv.URL + "/dos.css")
	s.until("the source landed", func() bool { return s.doc.ByID("source") != nil })
	if got := s.node("source").Text(); strings.Contains(got, "\r") {
		t.Fatalf("the source reads %q", got)
	}
}

// sourceKeys presses the keys Chrome shows a page's source by.
func (s *shown) sourceKeys() {
	if runtime.GOOS == "darwin" {
		s.in.SetKey(input.KeyLeftAlt, true)
		s.command(input.KeyU)
		s.in.SetKey(input.KeyLeftAlt, false)
	} else {
		s.chord(input.KeyLeftControl, input.KeyU)
	}
	s.frame()
	s.frame()
}

func TestAPagesSourceShowsInANewTabFromTheBytesAlreadyFetched(t *testing.T) {
	srv, seen := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.sourceKeys()
	if len(s.browser.Tabs) != 2 || s.browser.Active != 1 {
		t.Fatalf("%d tabs, tab %d shown", len(s.browser.Tabs), s.browser.Active)
	}
	s.at("view-source:" + srv.URL + "/")
	if got := s.title("view-source:" + srv.URL + "/"); got != "view-source:"+srv.URL+"/" {
		t.Errorf("the tab is named %q", got)
	}
	source := s.node("source").Text()
	if !strings.HasPrefix(source, "<title>A crypt</title>") || !strings.Contains(source,
		`<a id="vault" href="/vaults">`) {
		t.Fatalf("the source reads %q", source)
	}
	if seen.count("/") != 1 {
		t.Fatalf("the page was fetched %d times", seen.count("/"))
	}
	lines := strings.Count(source, "\n") + 1
	if got := strings.Fields(s.node("gutter").Text()); len(got) != lines || got[0] != "1" ||
		got[lines-1] != strconv.Itoa(lines) {
		t.Fatalf("the gutter numbers %v for %d lines", got, lines)
	}
	gutter, text := s.node("gutter").Box(), s.node("source").Box()
	if gutter.H != text.H || gutter.X+gutter.W > text.X {
		t.Fatalf("the gutter %v does not run beside the source %v", gutter, text)
	}
	if !s.doc.SelectAll() || strings.Contains(s.doc.Selection().Text, "1\n2\n") ||
		!strings.Contains(s.doc.Selection().Text, "A crypt") {
		t.Fatalf("select all takes %q", s.doc.Selection().Text)
	}
	s.clean()
}

func TestATypedViewSourceAddressFetchesThePage(t *testing.T) {
	srv, seen := crypt(t)
	s := show(t)
	s.enter("view-source:" + srv.URL + "/notes.txt")
	s.until("the source landed", func() bool { return s.doc.ByID("source") != nil })
	if got := s.node("source").Text(); got != "line one\nline two\n" || s.node("gutter").Text() != "1\n2\n" {
		t.Fatalf("the source reads %q beside %q", got, s.node("gutter").Text())
	}
	if seen.count("/notes.txt") != 1 {
		t.Fatalf("%d fetches", seen.count("/notes.txt"))
	}
	s.rightClick("source")
	s.choose("View page source")
	s.at("view-source:" + srv.URL + "/notes.txt")
	if len(s.browser.Tabs) != 2 {
		t.Fatalf("%d tabs", len(s.browser.Tabs))
	}
}

func TestWhatHasNoSourceSaysSo(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.Open("light://guide")
	s.rightClickAt(s.node("page").Children()[0].Box().Center())
	s.choose("View page source")
	s.at("view-source:light://guide")
	if text := s.node("page").Text(); !strings.Contains(text, "has no source") || !strings.Contains(text,
		"builds its own pages in Go") {
		t.Fatalf("the page reads %q", text)
	}
	s.enter("view-source:" + srv.URL + "/map.png")
	s.until("the picture landed", func() bool { return strings.Contains(s.node("page").Text(), "image/png, not text") })
	s.enter("view-source:" + srv.URL + "/missing")
	s.until("the failure landed", func() bool { return strings.Contains(s.node("page").Text(), "404 Not Found") })
	s.clean()
}
