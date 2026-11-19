//nolint:testpackage // tests reach the screen's unexported parts through *shown
package app

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/ui/align"
)

func (s *shown) warned() bool { return s.doc.ByID("preview") != nil }

func TestAFirstStartWarnsThatLightIsAnEarlyPreview(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Preview: true, Version: "0.2.0"})
	s.frame()
	if !s.warned() || !s.doc.Open(s.node("preview")) {
		t.Fatal("a first start shows no warning over the window")
	}
	text := s.node("preview").Text()
	for _, said := range []string{"very early preview", "not even an alpha", "0.2.0", "sensitive", "password",
		"not hardened"} {
		if !strings.Contains(text, said) {
			t.Errorf("the warning does not say %q:\n%s", said, text)
		}
	}
	if faults := align.Check(s.doc); len(faults) > 0 {
		t.Fatalf("layout faults: %v", faults)
	}
	if s.config.PreviewSeen != "" {
		t.Fatalf("the warning is noted as read, %q, before it is", s.config.PreviewSeen)
	}
	s.click("preview-ok")
	if s.warned() || s.config.PreviewSeen != "0.2.0" {
		t.Fatalf("once agreed to, the warning shows %v and %q is noted", s.warned(), s.config.PreviewSeen)
	}
}

func TestTheWarningComesBackWithEachRelease(t *testing.T) {
	dir := t.TempDir()
	starts := func(version string) *shown {
		s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Preview: true, Version: version})
		s.frame()
		return s
	}
	first := starts("0.2.0")
	first.click("preview-ok")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if again := starts("0.2.0"); again.warned() {
		t.Fatal("the same release warns twice")
	}
	if built := starts("0.2.0-3-gabc1234-dirty"); built.warned() {
		t.Fatal("a build made after a release warns again")
	}
	if next := starts("0.3.0"); !next.warned() {
		t.Fatal("a new release does not warn")
	}
}

func TestALightNobodySitsAtWarnsNothing(t *testing.T) {
	s := showWith(t, Options{Dir: t.TempDir(), Downloads: t.TempDir(), Version: "0.2.0"})
	s.frame()
	if s.warned() {
		t.Fatal("a light without Preview warns")
	}
}

func TestAHandMadeBuildWarnsOnce(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Preview: true})
	s.frame()
	if !s.warned() || !strings.Contains(s.node("preview").Text(), "this build") {
		t.Fatal("a build made by hand shows no warning")
	}
	s.click("preview-ok")
	if s.config.PreviewSeen != "dev" {
		t.Fatalf("a build made by hand notes %q", s.config.PreviewSeen)
	}
}
