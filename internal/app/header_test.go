package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"testing"

	"github.com/lantervale/ember/input"
)

func (s *shown) pressOn(id string) {
	s.t.Helper()
	r, ok := s.doc.BoxOf(s.node(id))
	if !ok {
		s.t.Fatalf("%q has no box", id)
	}
	s.in.SetMouse(r.Center())
	s.frame()
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
}

func (s *shown) release() {
	s.t.Helper()
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
}

func TestATabPressedWhileAnotherLandsIsStillChosen(t *testing.T) {
	s := show(t)
	s.newTab()
	s.frame()
	if s.browser.Active != 1 {
		t.Fatalf("the new tab is not shown: %d", s.browser.Active)
	}
	s.pressOn(tabID(0))
	s.showTabs()
	s.frame()
	s.release()
	if s.browser.Active != 0 {
		t.Fatalf("the press was lost: tab %d is shown", s.browser.Active)
	}
}

func TestATabClosedByItsButtonPressedWhileAnotherLandsIsClosed(t *testing.T) {
	s := show(t)
	s.newTab()
	s.frame()
	s.pressOn("close-0")
	s.showTabs()
	s.frame()
	s.release()
	if len(s.browser.Tabs) != 1 {
		t.Fatalf("the press was lost: %d tabs", len(s.browser.Tabs))
	}
}

func TestTheStripFollowsATabsNameAndIcon(t *testing.T) {
	s := show(t)
	s.newTab()
	s.frame()
	tab := s.node(tabID(1))
	s.browser.Tabs[1].Visit("light://history")
	s.showTabs()
	s.frame()
	if s.node(tabID(1)) != tab {
		t.Fatal("the tab was made again")
	}
	if got, want := s.tabNames(), s.title(s.browser.Tabs[0].Current())+" | "+s.title("light://history"); got != want {
		t.Fatalf("the strip names %q, want %q", got, want)
	}
	s.clean()
}
