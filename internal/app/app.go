// Package app is light's screen: the tab strip that is the window's
// title bar, the navigation row, the shown page with the status bubble
// over it, the context menu and the developer tools, all ui elements of
// one Document. It reads the input, calls the rules (pkg/…) and shows
// the result; a page of the web is internal/web's, drawn in a canvas.
package app

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"sync/atomic"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	_ "github.com/lantervale/ember/ui/svg" // the glyphs, the logo and the web's SVG pictures

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/internal/devtools/inspector"
	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/menu"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/localnet"
	"github.com/lantervale/light/pkg/tabs"
)

//go:embed app.css
var sheet string

const (
	// historyFile is the history's file in the folder light is given.
	historyFile = "history.json"
)

// Options are what the screen is given from outside.
type Options struct {
	// Transport makes the requests, http.DefaultTransport when nil.
	Transport http.RoundTripper
	// Space is the space of an address, for tests whose servers are all
	// of this machine; nil for each address's own (localnet.Of).
	Space func(netip.AddrPort) localnet.Space
	// Dir is the folder the history and the bookmarks are kept in between
	// runs; with none, they are kept in memory only.
	Dir string
	// Downloads is the folder downloads are saved to whatever the
	// settings say, and DefaultDownloads the one they are saved to when
	// neither names one.
	Downloads, DefaultDownloads string
	// Files opens a downloaded file and shows it in its folder; with
	// none, light offers neither.
	Files Files
	// Now tells the time of a visit, time.Now when nil.
	Now func() time.Time
	// ScriptBudget is how long one script of a page may run, the
	// engine's own limit when 0.
	ScriptBudget time.Duration
	// Patient has a script that runs long go on while the window does,
	// its user told and asked whether to stop it, in place of being
	// stopped at ScriptBudget: for a light with somebody at its window.
	// Patience is how long it waits, the browser's own times when zero.
	Patient  bool
	Patience dom.Patience
	// Version is light's, which the HAR files it saves name, and its
	// User-Agent when a build was given one ("" and "dev" are a build
	// made by hand).
	Version string
	// Languages are the languages the system's user prefers, most
	// wanted first, and Driven whether the run is driven by something
	// else than a person at its window: what light tells a site
	// (identity.go).
	Languages []string
	Driven    bool
	// ExactText draws the pages' text at its exact coverage, without
	// stem darkening, unless a page says how its own is smoothed: for a
	// light a test runner compares the pictures of.
	ExactText bool
}

// Screen is a window's screen, redrawn from the browser after each
// move. What it shares with the other windows' is its Profile's.
type Screen struct {
	*Profile

	doc     *ui.Document
	win     *platform.Window
	browser *tabs.Browser
	loader  atomic.Pointer[web.Loader]
	viewed  atomic.Uint64

	// fields are what shows each setting on the shown page again after
	// a change, and relist what lists the history again on the page
	// that shows it, which listed it when it had changed visitsSeen
	// times.
	fields     []func()
	relist     func()
	visitsSeen uint64
	// relistMarks lists the bookmarks again on the page that shows them,
	// and marksSeen is how many times they had changed when the star and
	// that page were last shown.
	relistMarks func()
	marksSeen   uint64
	// star is the address field's star and panel the card it opens; more
	// is the navigation row's last button and choices the menu it opens,
	// whose zoom reads zoomed and whose entry that saves the page is
	// keeps. again is the entry the menu opens on again once chosen.
	star, more, choices, zoomed, keeps, again *ui.Node
	panel                                     bookmarkPanel

	// sought shows the search engine's mark in the address field while
	// words are typed there.
	sought *ui.Node

	tabs, back, forward, reload, lock, field, address, view, status, list *ui.Node
	strip                                                                 []stripTab
	menu                                                                  *menu.Menu
	tools                                                                 *devtools.Panel
	inspect                                                               *inspector.Inspector

	// sessions is what each open tab holds beside its addresses.
	sessions map[*tabs.Tab]*session
	// landed brings what ended off the frame to the screen's frame,
	// which runs it.
	landed chan func()

	// later is what scripts asked of the tab, done once they returned,
	// and from the page a navigation is being opened by, "" for the
	// player's own.
	later []func()
	from  string
	// unasked is whether that navigation is a script's doing, not an
	// act of the player's.
	unasked bool

	// savesSeen is the downloads' version last shown, and announced the
	// state each download was last said to be in.
	savesSeen uint64
	announced map[int]downloads.State
	// archived are the downloads that are HAR files of the developer
	// tools, each with whether it holds the sensitive data; telling is
	// whether the status bubble says what came of one over the address
	// under the pointer, which was at told then.
	archived map[int]bool
	telling  bool
	told     geom.Vec2
	// downloads is the navigation row's button, recent its list, and
	// listed the list of the downloads page while it is shown.
	downloads, progress, recent, listed *ui.Node
	recentShown, listedShown            listing

	// line is the line that says how far the shown tab's load is, lineAt
	// the share of it filled, turn what the spinners of the tabs that
	// load are turned by, and dimAfter how long a page is held as it is.
	line     *ui.Node
	lineAt   float32
	turn     int
	dimAfter time.Duration

	// host is the canvas the shown web page draws in, nil for a page of
	// light's own.
	host *ui.Node
	// alone is the picture the shown tab shows alone, nil for another
	// kind of page.
	alone *alone
	// area is the title bar the header was last laid out around, size the
	// window's then, editing whether the address field had the focus, and
	// over the address named under the pointer.
	area    platform.TitleBarArea
	size    geom.Vec2
	editing bool
	over    string
	// aimed is the link the middle button went down on.
	aimed string
	// picked counts the times a page was asked what is under the pointer
	// for its scripts, for tests.
	picked int
}

// New is the screen of win, on the start page, with a Profile of its
// own: light with one window.
func New(win *platform.Window, opts Options) *Screen { return NewProfile(opts).Screen(win) }

// screen is the screen of win, a window of light's, on address.
func (p *Profile) screen(win *platform.Window, address string) *Screen {
	s := &Screen{Profile: p, win: win, sessions: map[*tabs.Tab]*session{}, landed: make(chan func(), 64),
		announced: map[int]downloads.State{}, archived: map[int]bool{}, dimAfter: dimAfter}
	s.browser = tabs.NewBrowser(address)
	for _, item := range p.saves.Items() {
		s.announced[item.ID] = item.State
	}
	s.savesSeen = p.saves.Version()
	s.apply()
	s.doc = ui.NewDocument(ui.CSS(sheet), ui.CSS(menu.Sheet), ui.CSS(devtools.Sheet), ui.Images(icons()),
		ui.FetchImages(s.picture))
	s.build()
	s.header()
	s.visit(address)
	s.show()
	s.showProgress(p.saves.Items())
	p.screens = append(p.screens, s)
	return s
}

// shut ends what the screen's tabs hold, as its window closes: their
// loads, their scripts and the frames of their pages.
func (s *Screen) shut() {
	s.menu.Close()
	s.doc.SetFrame(s.host, nil)
	s.browser.Tabs = nil
	s.forget()
}

// Document is the screen's Document, for its window.
func (s *Screen) Document() *ui.Document { return s.doc }

// build makes the screen's elements, once.
func (s *Screen) build() {
	s.tabs = ui.Nav(ui.ID("tabs"))
	s.view = ui.Main(ui.ID("view"))
	s.status = ui.Div(ui.ID("status"), ui.Attr("aria-hidden", "true"))
	s.list = ui.Div(ui.ID("tab-list"))
	s.menu = menu.New(s.doc)
	s.inspect = devtools.Inspect(s.doc, s.inspectorHost())
	s.tools = devtools.New(s.doc, devtools.Host{
		Point:      s.point,
		Serve:      func(name string, picture []byte) { s.pictures.Store(name, picture) },
		Closed:     s.show,
		Evaluate:   s.evaluate,
		Contexts:   s.contexts,
		EvaluateIn: s.evaluateIn,
		Unscripted: s.unscripted,
		Pointer: func() (geom.Vec2, bool) {
			in := s.win.Input()
			return in.Mouse(), in.Down(input.MouseLeft)
		},
		Resized: func(height int) {
			s.config.ToolsHeight = height
			s.settings.touch()
		},
		Source:  func(address string, _ int) { s.openBehind(tabs.SourceOf(address)) },
		Open:    s.openBehind,
		Storage: s.storedBy,
		Export:  func() { s.saveHAR(0, false) },
		Title:   s.titled,
		Version: s.opts.Version,
		Timing:  s.timingOf,
	})
	s.tools.SetHeight(s.config.ToolsHeight)
	s.doc.Body().Append(
		s.tabs,
		s.navigation(),
		ui.Div(ui.ID("split"), ui.Div(ui.ID("stage"), s.view, s.loadline(), s.inspect.Overlay(), s.status, s.list,
			s.recent, s.buildPanel(), s.choices),
			s.tools.Node()),
		s.shortcuts(),
		s.menu.Node())
	ui.OnContextMenu(s.contextMenu).Apply(s.doc.Body())
}

// Open shows address in the shown tab: at once for a page light has,
// and once it is ready for one that is fetched, the tab holding the
// page it shows till then (loading.go).
func (s *Screen) Open(address string) {
	tab := s.browser.Tab()
	s.session().note = ""
	s.depart(address, false, func() {
		tab.Visit(address)
		s.visit(address)
	})
}

// OpenInNewTab shows address in a tab of its own.
func (s *Screen) OpenInNewTab(address string) {
	s.browser.Open(address)
	s.visit(address)
	s.show()
}

// move goes through the shown tab's history as step does, past the
// steps of frames its page has no more.
func (s *Screen) move(step func(*tabs.Tab)) {
	tab := s.browser.Tab()
	ahead := *tab
	for {
		before := ahead.At()
		step(&ahead)
		if ahead.At() == before || ahead.Since(before, s.framed).Moved() {
			break
		}
	}
	s.cross(ahead.Since(tab.At(), s.framed))
}

// tick runs what ended off the frame since the last one, and follows
// the window, the pointer, the requests and the frames of the shown
// page.
func (s *Screen) tick() {
	s.header()
	s.closeUnderMiddle()
	s.followUnderMiddle()
	s.closeMenuWhenMoved()
	s.follow()
	s.sizePicture()
	s.tools.Update()
	s.watchDownloads()
	s.watchHistory()
	s.watchBookmarks()
	s.followChoices()
	s.measure()
	s.drew()
	s.stopByKey()
	s.followPanel()
	s.runScripts()
	if s.host != nil {
		page := s.session().pages[s.browser.Tab().Current()]
		s.broken(page)
		if page != nil && page.Err != nil {
			s.show()
			return
		}
		s.frames()
	}
	for {
		select {
		case land := <-s.landed:
			land()
		default:
			s.progressing()
			s.tellRequests()
			return
		}
	}
}

// overlaid reports whether a menu or a list of light's is open over the
// page: a press or a key is then not the page's scripts' to hear.
func (s *Screen) overlaid() bool {
	return s.menu.IsOpen() || s.list.HasClass("shown") || s.recent.HasClass("shown") || s.inspect.Picking() ||
		s.panelOpen() || s.choosing()
}

// show redraws the screen from the browser: the tabs, the shown tab's
// page, its address and its tools.
func (s *Screen) show() {
	s.forget()
	s.menu.Close()
	s.recent.RemoveClass("shown")
	s.closePanel(false)
	s.closeChoices()
	tab, session := s.browser.Tab(), s.session()
	empty(s.view)
	s.host, s.alone, s.listed, s.fields, s.relist, s.relistMarks = nil, nil, nil, nil, nil, nil
	s.view.Append(s.page(tab.Current()))
	s.showTabs()
	s.leave(tab, session)
	s.quitFrames(tab, session)
	s.showNavigation()
	s.tools.SetDark(s.config.DarkMode)
	s.keepBodies(session)
	s.tools.Show(&session.tools, session.log, session.said, s.warnings)
	s.over = ""
	s.say()
}

// warnings are what ui refused in the shown page of the web, and in
// the documents of its frames.
func (s *Screen) warnings() []error {
	session := s.session()
	page := session.pages[s.browser.Tab().Current()]
	if page == nil || page.Doc == nil {
		return nil
	}
	all := slices.Clone(page.Doc.Warnings())
	for _, f := range session.frames[page].frameList() {
		if f.gone || f.page == nil || f.page.Doc == nil {
			continue
		}
		for _, warned := range f.page.Doc.Warnings() {
			all = append(all, fmt.Errorf("in the frame %s: %w", f.label(), warned))
		}
	}
	return all
}

// picture gives the Document a picture stored under name.
func (s *Screen) picture(name string) ([]byte, error) {
	stored, _ := s.pictures.Load(name)
	if data, ok := stored.([]byte); ok {
		return data, nil
	}
	return nil, fmt.Errorf("no picture named %s", name)
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
