package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/tabs"
)

func TestTheToolsEdgeIsDraggedAndTheirHeightIsKeptBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.toggleTools()
	s.frame()
	handle, _ := s.doc.BoxOf(s.node("dt-handle"))
	s.in.SetMouse(handle.Center())
	s.frame()
	s.frame()
	if got := s.win.Cursor(); got != platform.CursorResizeNS {
		t.Errorf("over the edge the pointer is %v", got)
	}
	s.in.SetMouseButton(input.MouseLeft, true)
	s.frame()
	s.in.SetMouse(handle.Center().Add(geom.V2(0, -120)))
	s.frame()
	s.frame()
	if got := s.win.Cursor(); got != platform.CursorResizeNS {
		t.Errorf("while the edge is dragged the pointer is %v", got)
	}
	s.in.SetMouseButton(input.MouseLeft, false)
	s.frame()
	s.frame()
	tools, stage := s.box("devtools"), s.box("stage")
	if tools.H != 400 || tools.Y != stage.Y+stage.H || s.config.ToolsHeight != 400 {
		t.Fatalf("the tools dragged 120 up are at %v under %v, the settings keeping %d", tools, stage,
			s.config.ToolsHeight)
	}
	s.clean()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	again.toggleTools()
	again.frame()
	again.frame()
	if tools = again.box("devtools"); tools.H != 400 {
		t.Fatalf("the next run's tools are %v high", tools.H)
	}
}

func TestTheToolsFollowTheDarkMode(t *testing.T) {
	s := show(t)
	s.toggleTools()
	s.frame()
	if !s.node("devtools").HasClass("light") {
		t.Fatal("with the dark mode off the tools are dark")
	}
	s.clean()
	s.configure(func(c *config.Config) { c.DarkMode = true })
	s.frame()
	if s.node("devtools").HasClass("light") {
		t.Fatal("with the dark mode on the tools are light")
	}
	s.clean()
}

func TestWhereALineOfTheConsoleIsFromOpensItsSource(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.toggleTools()
	s.session().said.Add(console.Line{Level: console.Log, Text: "see " + srv.URL + "/vaults.", Source: srv.URL + "/app.js",
		Line: 2})
	s.click("dt-tab-console")
	find := func(class string) geom.Vec2 {
		var at geom.Vec2
		s.node("dt-lines").Walk(func(n *ui.Node) bool {
			if n.HasClass(class) {
				box, _ := s.doc.BoxOf(n)
				at = box.Center()
			}
			return true
		})
		return at
	}
	s.newTab()
	s.click("tab-0")
	source, linked := tabs.SourceOf(srv.URL+"/app.js"), srv.URL+"/vaults"
	s.clickAt(find("dt-where"))
	if got, want := s.opened(), "["+srv.URL+"/] "+source+" "+tabs.Home; got != want {
		t.Fatalf("after a click on where the line is from the tabs are %s, want %s", got, want)
	}
	s.clickAt(find("dt-link"))
	if got, want := s.opened(), "["+srv.URL+"/] "+source+" "+linked+" "+tabs.Home; got != want {
		t.Fatalf("after a click on the address in the line the tabs are %s, want %s", got, want)
	}
	s.click("tab-1")
	s.at(source)
}
