package app //nolint:testpackage // the dialogs are checked with the screens that show them

import (
	"testing"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/script"
)

// unasked dismisses what the shown page asked as it loaded, where its
// scripts run.
func (s *shown) unasked() {
	s.t.Helper()
	for range 3 {
		s.frame()
	}
	if s.doc.ByID("asking") != nil {
		s.press(input.KeyEscape)
	}
}

func TestAPageThatAsksAsItLoadsShowsItsQuestionWhereScriptsRun(t *testing.T) {
	srv := serve(t)
	s := show(t)
	s.visit(srv.URL + "/")
	for range 3 {
		s.frame()
	}
	dialog := s.doc.ByID("asking")
	if shown := dialog != nil; shown != script.Available() {
		t.Fatalf("a page whose script calls alert shows a dialog: %v, with an engine: %v", shown, script.Available())
	}
	if dialog == nil {
		return
	}
	if got := s.doc.ByID("asking-message").Text(); got != "1" {
		t.Fatalf("the alert says %q", got)
	}
	s.clean()
	s.unasked()
	if s.doc.ByID("asking") != nil {
		t.Fatal("Escape left the alert open")
	}
}
