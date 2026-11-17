package main

import "testing"

func TestVisitingAfterBackDropsTheForwardPages(t *testing.T) {
	h := NewHistory("a")
	h.Visit("b")
	h.Visit("c")
	h.Back()
	h.Back()
	h.Back()
	if h.Current() != "a" || h.CanBack() || !h.CanForward() {
		t.Fatalf("at %q, back %v, forward %v", h.Current(), h.CanBack(), h.CanForward())
	}
	h.Forward()
	h.Visit("d")
	if h.Current() != "d" || h.CanForward() {
		t.Fatalf("at %q, forward %v", h.Current(), h.CanForward())
	}
	h.Back()
	if h.Current() != "b" {
		t.Fatalf("back from d is %q", h.Current())
	}
}

func TestVisitingTheSamePageAddsNothing(t *testing.T) {
	h := NewHistory("a")
	h.Visit("a")
	if h.CanBack() {
		t.Fatal("a reload became a history entry")
	}
}

func TestResolve(t *testing.T) {
	for typed, want := range map[string]string{
		"":                 home,
		"  ember://guide ": "ember://guide",
		"example.com":      "https://example.com",
		"tile maps":        "ember://search?q=tile+maps",
		"guide":            "ember://search?q=guide",
		"what is 1.5 zoom": "ember://search?q=what+is+1.5+zoom",
	} {
		if got := Resolve(typed); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", typed, got, want)
		}
	}
}

func TestClosingTabsKeepsTheShownOne(t *testing.T) {
	b := NewBrowser()
	b.Open("b")
	b.Open("c")
	b.Select(1)
	b.Close(0)
	if b.Tab().Current() != "b" {
		t.Fatalf("closing a tab before the shown one shows %q", b.Tab().Current())
	}
	b.Close(1)
	if b.Tab().Current() != "b" || len(b.Tabs) != 1 {
		t.Fatalf("shows %q of %d tabs", b.Tab().Current(), len(b.Tabs))
	}
	b.Close(0)
	if len(b.Tabs) != 1 || b.Tab().Current() != home {
		t.Fatalf("closing the last tab leaves %q of %d tabs", b.Tab().Current(), len(b.Tabs))
	}
}
