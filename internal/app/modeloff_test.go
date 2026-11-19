package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"
)

// seen reports whether n takes room on the screen.
func (s *shown) seen(n *ui.Node) bool {
	r, ok := s.doc.BoxOf(n)
	return ok && r.W > 0 && r.H > 0
}

// nothingHelped fails when the screen shows anything of what a model
// helps with: a robot, a card, a label, a section of theirs.
func (s *shown) nothingHelped(where string) {
	s.t.Helper()
	s.doc.Body().Walk(func(n *ui.Node) bool {
		if n.IsText() || !s.seen(n) {
			return !n.IsText()
		}
		id := n.ID()
		src, _ := n.Attr("src")
		switch {
		case n.HasClass("robot-toggle"), strings.Contains(src, "robot") && n.Parent().ID() != "menu-model-on",
			strings.HasPrefix(id, "category"), strings.HasPrefix(id, "answer"), strings.HasPrefix(id, "picked"),
			strings.HasPrefix(id, "history-ask"), strings.HasPrefix(id, "find-pass"), strings.HasPrefix(id, "set-recall"),
			id == "robot", id == "menu-ask", id == "menu-sorted", id == "menu-model", id == "find-robot",
			id == "continue-robot", id == "bookmark-robot":
			s.t.Errorf("%s: without a model the screen shows <%s id=%q src=%q>", where, n.Tag(), id, src)
		}
		return true
	})
	text := s.doc.Body().Text()
	for _, f := range features() {
		if f.id != findFeature && strings.Contains(text, f.label) {
			s.t.Errorf("%s: without a model the screen says %q", where, f.label)
		}
	}
	for _, said := range []string{"by your categories", "by meaning", "what pages said", "on this device, nothing sent"} {
		if strings.Contains(text, said) {
			s.t.Errorf("%s: without a model the screen says %q", where, said)
		}
	}
}

func TestWithoutAModelNothingOfWhatItHelpsWithShowsAnywhere(t *testing.T) {
	srv := harbour(t)
	for name, build := range map[string]func() *shown{
		"no model": func() *shown { return showWith(t, Options{Transport: &fenced{}, Downloads: t.TempDir()}) },
		"the model off": func() *shown {
			s, _ := modelled(t, Options{Transport: &fenced{}})
			return s
		},
	} {
		s := build()
		page := s.visit(srv.URL + "/")
		s.selects(page, "cost")
		s.nothingHelped(name + ", a page with a selection")
		s.command(input.KeyD)
		s.frame()
		s.nothingHelped(name + ", the bookmark panel")
		s.press(input.KeyEscape)
		s.command(input.KeyF)
		s.frame()
		s.types("ferry")
		s.nothingHelped(name + ", find in page")
		s.press(input.KeyEscape)
		s.click("menu")
		s.nothingHelped(name + ", the menu")
		s.press(input.KeyEscape)
		for _, address := range []string{s.config.StartPage, historyPage, bookmarksPage, configPage} {
			s.Open(address)
			s.frame()
			s.frame()
			s.nothingHelped(name + ", " + address)
		}
		s.clean()
	}
}

func TestTheCardOfAModelFitsTheWindowWithAllItHelpsWith(t *testing.T) {
	for i, size := range []geom.Vec2{geom.V2(1200, 760), geom.V2(760, 480), geom.V2(1200, 760), geom.V2(760, 480)} {
		s, fake := modelOn(t)
		if i > 1 {
			unembedded(t, fake)
		}
		s.win.SetSize(size)
		s.frame()
		s.frame()
		s.click("robot")
		if got := len(s.texts("model-helps", "folder")); got != len(features()) {
			t.Fatalf("at %v the card lists %d of the %d things the model helps with", size, got, len(features()))
		}
		if card := s.box("model"); card.Y+card.H > size.Y || card.X < 0 || card.X+card.W > size.X {
			t.Errorf("at %v the card stands at %v", size, card)
		}
		s.clean()
	}
}
