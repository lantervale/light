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

// stripTab is a tab of the strip with its entry in the list of all tabs,
// and the icon it shows.
type stripTab struct {
	tab, listed, name, icon *ui.Node
	pictured                string
}

// showTabs brings the tab strip and the list of all tabs up to the open
// tabs. The tabs already there are kept and told their name, their icon
// and which one shows: an element made again would lose the press it is
// under, as when a page lands in another tab.
func (s *Screen) showTabs() {
	s.list.RemoveClass("shown")
	if len(s.strip) != len(s.browser.Tabs) {
		s.buildTabs()
	}
	for i, tab := range s.browser.Tabs {
		t := &s.strip[i]
		title := s.title(tab.Current())
		t.name.SetText(title)
		t.listed.SetText(title)
		if name := s.icon(tab.Current()); name != t.pictured {
			t.pictured = name
			depict(t.icon, name)
		}
		toggle(t.tab, "shown", i == s.browser.Active)
		toggle(t.listed, "shown", i == s.browser.Active)
	}
}

func (s *Screen) buildTabs() {
	empty(s.tabs)
	empty(s.list)
	s.strip = s.strip[:0]
	for i := range s.browser.Tabs {
		t := stripTab{
			name: ui.Span(ui.Class("name")),
			icon: ui.Img(ui.Class("icon"), ui.Attr("alt", ""), ui.Attr("hidden", "")),
		}
		face := ui.Div(ui.Class("face"), t.icon, t.name,
			s.button(fmt.Sprintf("close-%d", i), "Close tab", "close", func() { s.closeTab(i) }))
		choose := func() { s.selectTab(i) }
		t.tab = ui.Div(ui.ID(tabID(i)), ui.Class("tab"), ui.TabIndex(0), face, ui.OnClick(choose))
		t.listed = ui.Div(ui.ID(fmt.Sprintf("listed-%d", i)), ui.Class("listed"), ui.TabIndex(0), ui.OnClick(choose))
		s.tabs.Append(t.tab)
		s.list.Append(t.listed)
		s.strip = append(s.strip, t)
	}
	s.tabs.Append(
		s.button("new-tab", "New tab", "plus", s.newTab),
		s.button("all-tabs", "List all tabs", "tabs", func() { toggle(s.list, "shown", !s.list.HasClass("shown")) }))
}

// depict has a tab's icon show the picture named, or nothing.
func depict(icon *ui.Node, name string) {
	if name == "" {
		icon.RemoveAttr("src")
		icon.SetAttr("hidden", "")
		return
	}
	icon.SetAttr("src", name)
	icon.RemoveAttr("hidden")
}

func tabID(i int) string { return fmt.Sprintf("tab-%d", i) }

func (s *Screen) newTab() {
	s.browser.Open(s.config.StartPage)
	s.show()
}

// closeTab closes tab i, and the window with its last one.
func (s *Screen) closeTab(i int) {
	if len(s.browser.Tabs) == 1 {
		s.win.RequestClose()
		return
	}
	s.browser.Close(i)
	s.show()
}

func (s *Screen) selectTab(i int) {
	s.browser.Select(i)
	s.show()
}

// icon names the picture a tab shows before its name: its site's icon,
// light's logo for a page of its own, and none for a site without one
// light can draw.
func (s *Screen) icon(address string) string {
	if _, ok := s.pictures.Load(faviconOf(address)); ok {
		return faviconOf(address)
	}
	if _, _, lights := tabs.Own(address); lights {
		return logo
	}
	return ""
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
	for i := range s.strip {
		if r, ok := s.doc.BoxOf(s.strip[i].tab); ok && r.Contains(at) {
			return i
		}
	}
	return -1
}
