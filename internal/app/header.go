package app

import (
	"fmt"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/tabs"
)

// The header's height and its left padding where the system draws no
// title bar over it; over one, the header is as high as it and starts
// past its buttons.
const (
	headerHeight = 44
	headerInset  = 8
	// headerGap is the room kept between the window's buttons and the
	// first tab.
	headerGap = 6
)

// header keeps the tab strip clear of the title bar the system draws
// over it: as high as it, and starting past the window's buttons.
func (s *Screen) header() {
	area := s.win.TitleBarArea()
	if area == s.area && s.tabs.Style() != "" {
		return
	}
	s.area = area
	height, left, right := float32(headerHeight), float32(headerInset), float32(headerInset)
	if area.Height > 0 {
		height = area.Height
	}
	if area.Left > 0 {
		left = area.Left + headerGap
	}
	if area.Right > 0 {
		right = area.Right + headerGap
	}
	s.tabs.SetStyle(fmt.Sprintf("height: %gpx; padding-left: %gpx; padding-right: %gpx", height, left, right))
}

// showTabs fills the tab strip and the list of all tabs.
func (s *Screen) showTabs() {
	empty(s.tabs)
	empty(s.list)
	s.list.RemoveClass("shown")
	for i, tab := range s.browser.Tabs {
		title := s.title(tab.Current())
		face := ui.Div(ui.Class("face"))
		if icon := s.icon(tab.Current()); icon != nil {
			face.Append(icon)
		}
		face.Append(ui.Span(ui.Class("name"), ui.Text(title)),
			s.button(fmt.Sprintf("close-%d", i), "Close tab", "close", func() { s.closeTab(i) }))
		choose := func() { s.selectTab(i) }
		t := ui.Div(ui.ID(tabID(i)), ui.Class("tab"), ui.TabIndex(0), face, ui.OnClick(choose))
		listed := ui.Div(ui.ID(fmt.Sprintf("listed-%d", i)), ui.Class("listed"), ui.TabIndex(0), ui.Text(title),
			ui.OnClick(choose))
		toggle(t, "shown", i == s.browser.Active)
		toggle(listed, "shown", i == s.browser.Active)
		s.tabs.Append(t)
		s.list.Append(listed)
	}
	s.tabs.Append(
		s.button("new-tab", "New tab", "plus", s.newTab),
		s.button("all-tabs", "List all tabs", "tabs", func() { toggle(s.list, "shown", !s.list.HasClass("shown")) }))
}

func tabID(i int) string { return fmt.Sprintf("tab-%d", i) }

func (s *Screen) newTab() {
	s.browser.Open(s.config.StartPage)
	s.show()
}

func (s *Screen) closeTab(i int) {
	s.browser.Close(i)
	s.show()
}

func (s *Screen) selectTab(i int) {
	s.browser.Select(i)
	s.show()
}

// icon is the picture a tab shows before its name: its site's icon,
// light's logo for a page of its own, and none for a site without one
// light can draw.
func (s *Screen) icon(address string) *ui.Node {
	name := ""
	if _, ok := s.pictures.Load(faviconOf(address)); ok {
		name = faviconOf(address)
	} else if _, _, lights := tabs.Own(address); lights {
		name = logo
	}
	if name == "" {
		return nil
	}
	return ui.Img(ui.Class("icon"), ui.Attr("src", name), ui.Attr("alt", ""))
}

// closeUnderMiddle closes the tab a middle click is on.
func (s *Screen) closeUnderMiddle() {
	in := s.win.Input()
	if !in.Pressed(input.MouseMiddle) {
		return
	}
	if i := s.tabAt(in.Mouse()); i >= 0 {
		s.closeTab(i)
	}
}

// tabAt is the tab under at, -1 for none.
func (s *Screen) tabAt(at geom.Vec2) int {
	for i := range s.browser.Tabs {
		if r, ok := s.doc.BoxOf(s.tabs.Children()[i]); ok && r.Contains(at) {
			return i
		}
	}
	return -1
}
