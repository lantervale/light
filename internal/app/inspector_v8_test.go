//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import "testing"

func TestTheConsoleNamesTheInspectedElement(t *testing.T) {
	srv, _ := scripted(t)
	s := showWith(t, Options{Downloads: t.TempDir()})
	doc := s.visit(srv.URL + "/")
	s.until("the page's scripts ran", func() bool { return s.pageText("count") == "0 clicks" })
	if got, err := s.evaluate("$0"); err != nil || got != "null" {
		t.Fatalf("with nothing selected $0 is %s, %v", got, err)
	}
	s.inspector()
	s.inspect.Select(doc.ByID("count"))
	s.frame()
	if got, err := s.evaluate("$0.id + ': ' + $0.textContent"); err != nil || got != `"count: 0 clicks"` {
		t.Fatalf("$0 is %s, %v", got, err)
	}
	if head := s.session().pages[srv.URL+"/"].Tree.Head(); s.inspect.RowOf(head) == nil {
		t.Fatal("the tree of a page loaded for its scripts has no row for its head")
	}
	s.inspect.Select(doc.ByID("more"))
	if got, _ := s.evaluate("$0.id"); got != `"more"` {
		t.Fatalf("after another selection $0 is %s", got)
	}
	s.clean()
}
