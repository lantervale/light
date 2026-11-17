package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"sync"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/world"
)

const sheet = `html, body { height: 100%; }
body { margin: 0; display: flex; flex-direction: column; background: linear-gradient(to right, #302b42, #422625);
	color: #e8e3ee; font-size: 13px; }
button { border: none; padding: 0; background: transparent; cursor: pointer; }
button img, .tab img, #lock { display: block; }

#tabs { display: flex; align-items: center; gap: 4px; height: 44px; padding: 0 8px; overflow: hidden;
	app-region: drag; }
.tab { flex: 1 1 0; box-sizing: border-box; min-width: 64px; max-width: 232px; height: 34px; padding: 1px;
	border-radius: 13px; color: #bbb6c6; cursor: pointer; }
.face { display: flex; align-items: center; gap: 8px; box-sizing: border-box; height: 100%; padding: 0 6px 0 12px;
	border-radius: 12px; }
.tab:hover .face { background: rgba(255, 255, 255, 0.07); }
.tab.shown { background: linear-gradient(to right, #b3a1f4, #d49bb4 50%, #f39b6b); color: #ffffff; }
.tab.shown .face { background: #171519; }
.tab .icon { flex: none; width: 16px; height: 16px; }
.tab .name { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.tab button, #new-tab, #all-tabs { flex: none; width: 24px; height: 24px; border-radius: 12px; }
.tab button img { width: 14px; height: 14px; margin: 5px; }
#new-tab img, #all-tabs img { width: 18px; height: 18px; margin: 3px; }
.tab button:hover, #new-tab:hover, #all-tabs:hover { background: rgba(255, 255, 255, 0.14); }
#new-tab { margin-left: 10px; }
#all-tabs { margin-left: auto; }

#bar { display: flex; align-items: center; gap: 4px; box-sizing: border-box; height: 41px; padding: 0 10px;
	border-bottom: 1px solid rgba(255, 255, 255, 0.09); background: rgba(0, 0, 0, 0.2); }
#bar button { flex: none; width: 32px; height: 32px; border-radius: 16px; }
#bar button img { width: 18px; height: 18px; margin: 7px; }
#bar button:hover { background: rgba(255, 255, 255, 0.12); }
#bar button:disabled { background: transparent; cursor: default; }
#bar button:disabled img { opacity: 0.35; }
#field { display: flex; flex: 1; align-items: center; gap: 10px; min-width: 0; height: 32px; margin: 0 4px 0 10px;
	padding: 0 14px; border-radius: 16px; background: #1f1c2a; box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.1); }
#field.editing { box-shadow: 0 0 0 1px #b3a1f4; }
#lock { flex: none; width: 18px; height: 18px; }
#address { flex: 1; min-width: 0; height: 32px; padding: 0; border: none; outline: none; background: transparent;
	color: #f4f1f8; font-size: 14px; }

#tab-list { display: none; position: absolute; top: 4px; right: 8px; z-index: 2; box-sizing: border-box;
	width: 280px; max-height: 80%; padding: 6px; overflow-y: auto; border: 1px solid rgba(255, 255, 255, 0.14);
	border-radius: 10px; background: #2a2533; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4); }
#tab-list.shown { display: block; }
.listed { padding: 6px 10px; border-radius: 6px; color: #d9d4e2; white-space: nowrap; overflow: hidden;
	text-overflow: ellipsis; cursor: pointer; }
.listed:hover { background: rgba(255, 255, 255, 0.1); }
.listed.shown { color: #ffffff; font-weight: bold; }

#keys { display: none; }

#stage { position: relative; display: flex; flex: 1; flex-direction: column; min-height: 0; }
#view { flex: 1; min-height: 0; overflow-y: auto; background: #ffffff; color: #202124; }
#page { width: 640px; margin: 0 auto; padding: 24px 0 48px; font-size: 15px; line-height: 1.6; }
#page h1 { font-size: 28px; margin: 12px 0 8px; }
#page h2 { font-size: 19px; margin: 24px 0 4px; }
#page p { margin: 8px 0; color: #3c4043; }
#list div { padding: 6px 0; border-bottom: 1px solid #f1f3f4; }
#page.hosted { width: 100%; height: 100%; margin: 0; padding: 0; }
#web { display: block; width: 100%; height: 100%; }
#page pre { padding: 12px; border-radius: 6px; background: #f1f3f4; font-size: 13px; overflow-x: auto; }
#page hr { margin: 16px 0; border: none; border-top: 1px solid #dadce0; }
.item { padding: 2px 0 2px 16px; }
.quote { margin: 8px 0; padding-left: 14px; border-left: 3px solid #dadce0; color: #5f6368; }
#page code { padding: 1px 5px; border-radius: 4px; background: #f1f3f4; }
.link { color: #1a0dab; cursor: pointer; }
.link:hover { color: #d93025; }

#page.home { display: flex; flex-direction: column; align-items: center; padding-top: 72px; }
#wordmark { font-size: 56px; font-weight: bold; letter-spacing: -2px; color: #1a73e8; margin-bottom: 24px; }
#search { width: 520px; height: 44px; padding: 0 20px; border: 1px solid #dfe1e5; border-radius: 22px; outline: none;
	color: #202124; font-size: 16px; }
#search:focus { box-shadow: 0 1px 6px rgba(32, 33, 36, 0.28); }
#tiles { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; width: 100%; margin-top: 36px; }
.tile { display: flex; flex-direction: column; gap: 6px; padding: 14px; border: 1px solid #dadce0; border-radius: 10px;
	text-align: left; font-size: 12px; line-height: 1.4; color: #5f6368; }
.tile:hover { border-color: #1a73e8; background: #f8faff; }
.tile strong { font-size: 14px; color: #202124; }

.result { margin: 18px 0; font-size: 18px; }
.result .address { font-size: 12px; color: #188038; }
.result p { font-size: 14px; }

#status { display: none; position: absolute; left: -1px; bottom: -1px; box-sizing: border-box; max-width: 50%;
	height: 23px; padding: 0 8px 0 9px; border: 1px solid #5b5763; border-radius: 0 6px 0 0;
	background: #2b2831; color: #f3eff7; font-size: 12px; line-height: 21px; white-space: nowrap; overflow: hidden;
	text-overflow: ellipsis; pointer-events: none; }
#status.shown { display: block; }
#status.away { left: auto; right: -1px; padding: 0 9px 0 8px; border-radius: 6px 0 0 0; }`

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

var errStopped = errors.New("its loading was stopped")

// load is a page on its way: cancel stops what of it can be, the page
// and its sheets.
type load struct{ cancel context.CancelFunc }

// explorer is the window's screen: the tab strip that is its title bar,
// the navigation row, the shown page and the status bubble over it,
// redrawn from the browser after each move.
type explorer struct {
	doc     *ui.Document
	win     *platform.Window
	browser *Browser

	tabs, back, forward, reload, lock, field, address, view, status, list *ui.Node
	// remotes are the web pages fetched so far by address, loads those on
	// their way; loaded brings each from its fetch to Update.
	remotes map[string]*remote
	loads   map[string]*load
	loaded  chan fetched
	// host is the canvas the shown web page draws in, nil for a page of
	// the explorer's own.
	host *ui.Node
	// area is the title bar the header was last laid out around, editing
	// whether the address field had the focus then, and over the address
	// of the link under the pointer.
	area    platform.TitleBarArea
	editing bool
	over    string
	icons   sync.Map
}

func newExplorer(win *platform.Window) *explorer {
	e := &explorer{
		win: win, browser: NewBrowser(),
		remotes: map[string]*remote{}, loads: map[string]*load{}, loaded: make(chan fetched, 64),
	}
	e.doc = ui.NewDocument(ui.CSS(sheet), ui.Images(icons()), ui.FetchImages(e.favicon))
	e.tabs = ui.Nav(ui.ID("tabs"))
	e.back = e.button("back", "Back", "back", func() { e.move((*History).Back) })
	e.forward = e.button("forward", "Forward", "forward", func() { e.move((*History).Forward) })
	e.reload = e.button("reload", "Reload", "reload", e.reloadOrStop)
	e.lock = ui.Img(ui.ID("lock"), ui.Attr("alt", ""))
	e.address = ui.Input(ui.ID("address"), ui.Attr("aria-label", "Address"),
		ui.OnSubmit(func() { e.open(Resolve(e.doc.Value(e.address))) }))
	e.field = ui.Div(ui.ID("field"), e.lock, e.address)
	e.view = ui.Main(ui.ID("view"))
	e.status = ui.Div(ui.ID("status"), ui.Attr("aria-hidden", "true"))
	e.list = ui.Div(ui.ID("tab-list"))
	e.doc.Body().Append(
		e.tabs,
		ui.Header(ui.ID("bar"), e.back, e.forward, e.reload,
			e.button("home", "Home", "home", func() { e.open(home) }), e.field),
		ui.Div(ui.ID("stage"), e.view, e.status, e.list),
		e.keys())
	e.header()
	e.show()
	return e
}

func (e *explorer) button(id, label, icon string, click func()) *ui.Node {
	return ui.Button(ui.ID(id), ui.Attr("aria-label", label), glyph(icon), ui.OnClick(click))
}

func glyph(name string) *ui.Node { return ui.Img(ui.Attr("src", name+".svg"), ui.Attr("alt", "")) }

func (e *explorer) open(address string) {
	e.browser.Tab().Visit(address)
	e.show()
}

func (e *explorer) move(step func(*History)) {
	step(e.browser.Tab())
	e.show()
}

func (e *explorer) loading() bool { return e.loads[e.browser.Tab().Current()] != nil }

// reloadOrStop is the one button: it stops the shown page while it loads,
// and fetches it again once it has.
func (e *explorer) reloadOrStop() {
	address := e.browser.Tab().Current()
	if l := e.loads[address]; l != nil {
		l.cancel()
		delete(e.loads, address)
		e.remotes[address] = &remote{err: errStopped}
	} else {
		delete(e.remotes, address)
	}
	e.show()
}

// Update shows the web pages whose fetch has ended since the last frame,
// has the shown one drawn again (its Document lives by being drawn), and
// follows the window and the pointer.
func (e *explorer) Update(world.Frame) {
	e.header()
	e.closeUnderMiddle()
	e.follow()
	if e.host != nil {
		if page := e.remotes[e.browser.Tab().Current()]; page != nil && page.err != nil {
			e.show()
			return
		}
		e.doc.Invalidate(e.host)
	}
	for {
		select {
		case f := <-e.loaded:
			if e.loads[f.address] != f.load {
				continue
			}
			delete(e.loads, f.address)
			e.remotes[f.address] = f.page
			if f.page.icon != nil {
				e.icons.Store(f.address, f.page.icon)
			}
			e.show()
		default:
			return
		}
	}
}

// header keeps the tab strip clear of the title bar the system draws
// over it: as high as it, and starting past the window's buttons.
func (e *explorer) header() {
	area := e.win.TitleBarArea()
	if area == e.area && e.tabs.Style() != "" {
		return
	}
	e.area = area
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
	e.tabs.SetStyle(fmt.Sprintf("height: %gpx; padding-left: %gpx; padding-right: %gpx", height, left, right))
}

// keys is the explorer's shortcuts as a menu never shown, which ui
// chooses from by its items' keys, a text field focused or not: Command
// on macOS and Control elsewhere with T (a new tab), W (close the tab),
// L (the address), R (reload or stop) and [ or ] (back, forward), and off
// macOS, where they move no caret, Alt with the left or right arrow.
func (e *explorer) keys() *ui.Node {
	menu := ui.Div(ui.ID("keys"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Shortcuts"))
	back, forward := "Cmd+[", "Cmd+]"
	if runtime.GOOS != "darwin" {
		back, forward = back+" Alt+ArrowLeft", forward+" Alt+ArrowRight"
	}
	for _, item := range []struct {
		name, keys string
		do         func()
	}{
		{"New tab", "Cmd+T", func() { e.browser.Open(home); e.show() }},
		{"Close tab", "Cmd+W", func() { e.browser.Close(e.browser.Active); e.show() }},
		{"Address", "Cmd+L", func() { e.doc.Focus(e.address) }},
		{"Reload", "Cmd+R", e.reloadOrStop},
		{"Back", back, func() { e.move((*History).Back) }},
		{"Forward", forward, func() { e.move((*History).Forward) }},
	} {
		menu.Append(ui.Div(ui.Attr("role", "menuitem"), ui.Attr("aria-keyshortcuts", item.keys), ui.Text(item.name),
			ui.OnClick(item.do)))
	}
	return menu
}

// closeUnderMiddle closes the tab a middle click is on.
func (e *explorer) closeUnderMiddle() {
	in := e.win.Input()
	if !in.Pressed(input.MouseMiddle) {
		return
	}
	if i := e.tabAt(in.Mouse()); i >= 0 {
		e.browser.Close(i)
		e.show()
	}
}

// tabAt is the tab under at, -1 for none.
func (e *explorer) tabAt(at geom.Vec2) int {
	for i := range e.browser.Tabs {
		if r, ok := e.doc.BoxOf(e.tabs.Children()[i]); ok && r.Contains(at) {
			return i
		}
	}
	return -1
}

// The status bubble's left and bottom borders lie a point outside the
// window, so it shows as a box stuck to the corner.
const bubbleOut = 1

// follow shows the whole address while its field has the focus, and
// moves the status bubble to the right while the pointer is where it
// stands on the left.
func (e *explorer) follow() {
	if editing := e.doc.Focused(e.address); editing != e.editing {
		e.editing = editing
		e.locate()
	}
	stage, _ := e.doc.BoxOf(e.status.Parent())
	bubble, shown := e.doc.BoxOf(e.status)
	at := e.win.Input().Mouse()
	under := shown && at.X >= stage.X && at.X < stage.X+bubble.W+16 && at.Y >= stage.Y+stage.H-bubble.H-8 &&
		at.Y < stage.Y+stage.H
	if under != e.status.HasClass("away") {
		toggle(e.status, "away", under)
	}
}

// locate fills the address field: the whole address while it is edited,
// else as shown, with a lock beside one fetched over https.
func (e *explorer) locate() {
	address := e.browser.Tab().Current()
	icon := "open.svg"
	if strings.HasPrefix(address, "https://") {
		icon = "lock.svg"
	}
	e.lock.SetAttr("src", icon)
	toggle(e.field, "editing", e.editing)
	if !e.editing {
		address = shownAs(address)
	}
	e.doc.SetValue(e.address, address)
}

// shownAs is an address as the address field and the status bubble show
// it: an https one without its scheme, any other whole.
func shownAs(address string) string { return strings.TrimPrefix(address, "https://") }

// point names the address of the link under the pointer, "" for none.
func (e *explorer) point(address string) {
	e.over = address
	e.say()
}

// say fills the status bubble: the link under the pointer, else the host
// the shown page is waited for from, else nothing, and no bubble.
func (e *explorer) say() {
	text := shownAs(e.over)
	if text == "" && e.loading() {
		if u, err := url.Parse(e.browser.Tab().Current()); err == nil {
			text = "Waiting for " + u.Hostname() + "…"
		}
	}
	if text != e.status.Text() {
		e.status.SetText(text)
	}
	toggle(e.status, "shown", text != "")
}

// icon is the picture a tab shows before its name: its site's icon, the
// explorer's glyph for a page of its own, and none for a site without one
// the explorer can draw.
func (e *explorer) icon(address string) *ui.Node {
	name := ""
	if _, ok := e.icons.Load(address); ok {
		name = "favicon:" + address
	} else if site, _ := locate(address); site != "" {
		name = "page.svg"
	}
	if name == "" {
		return nil
	}
	return ui.Img(ui.Class("icon"), ui.Attr("src", name), ui.Attr("alt", ""))
}

func (e *explorer) show() {
	empty(e.tabs)
	empty(e.list)
	e.list.RemoveClass("shown")
	for i, tab := range e.browser.Tabs {
		face := ui.Div(ui.Class("face"))
		if icon := e.icon(tab.Current()); icon != nil {
			face.Append(icon)
		}
		face.Append(ui.Span(ui.Class("name"), ui.Text(e.title(tab.Current()))),
			e.button(fmt.Sprintf("close-%d", i), "Close tab", "close", func() { e.browser.Close(i); e.show() }))
		choose := func() { e.browser.Select(i); e.show() }
		t := ui.Div(ui.ID(fmt.Sprintf("tab-%d", i)), ui.Class("tab"), ui.TabIndex(0), face, ui.OnClick(choose))
		listed := ui.Div(ui.ID(fmt.Sprintf("listed-%d", i)), ui.Class("listed"), ui.TabIndex(0),
			ui.Text(e.title(tab.Current())), ui.OnClick(choose))
		if i == e.browser.Active {
			t.AddClass("shown")
			listed.AddClass("shown")
		}
		e.tabs.Append(t)
		e.list.Append(listed)
	}
	e.tabs.Append(
		e.button("new-tab", "New tab", "plus", func() { e.browser.Open(home); e.show() }),
		e.button("all-tabs", "List all tabs", "tabs", func() { toggle(e.list, "shown", !e.list.HasClass("shown")) }))

	tab := e.browser.Tab()
	empty(e.view)
	e.host = nil
	e.view.Append(e.page(tab.Current()))
	e.locate()
	enable(e.back, tab.CanBack())
	enable(e.forward, tab.CanForward())
	label, icon := "Reload", "reload.svg"
	if e.loading() {
		label, icon = "Stop", "stop.svg"
	}
	e.reload.SetAttr("aria-label", label)
	e.reload.Children()[0].SetAttr("src", icon)
	e.over = ""
	e.say()
}

// favicon gives the explorer's Document the icon of the page at the
// address a tab's picture names.
func (e *explorer) favicon(name string) ([]byte, error) {
	if icon, ok := e.icons.Load(strings.TrimPrefix(name, "favicon:")); ok {
		return icon.([]byte), nil //nolint:forcetypeassert // only Update stores, and only bytes
	}
	return nil, fmt.Errorf("no icon for %s", name)
}

func empty(n *ui.Node) {
	for len(n.Children()) > 0 {
		n.Remove(n.Children()[0])
	}
}

func enable(n *ui.Node, on bool) {
	if on {
		n.RemoveAttr("disabled")
		return
	}
	n.SetAttr("disabled", "")
}

func toggle(n *ui.Node, class string, on bool) {
	if on {
		n.AddClass(class)
		return
	}
	n.RemoveClass(class)
}
