// Package app is light's screen: the tab strip that is the window's
// title bar, the navigation row, the shown page with the status bubble
// over it, the context menu and the developer tools, all ui elements of
// one Document. It reads the input, calls the rules (pkg/…) and shows
// the result; a page of the web is internal/web's, drawn in a canvas.
package app

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	_ "github.com/lantervale/ember/ui/svg" // the glyphs, the logo and the web's SVG pictures
	"github.com/lantervale/ember/world"

	"github.com/lantervale/light/internal/devtools"
	"github.com/lantervale/light/internal/devtools/inspector"
	"github.com/lantervale/light/internal/menu"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/storage"
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
	// Dir is the folder the history is kept in between runs; with none,
	// it is kept in memory only.
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
}

// Screen is the window's screen, redrawn from the browser after each
// move.
type Screen struct {
	doc     *ui.Document
	win     *platform.Window
	browser *tabs.Browser
	loader  atomic.Pointer[web.Loader]
	viewed  atomic.Uint64
	opts    Options

	// config is the settings, misread why their file could not be read,
	// settings their file, and fields what shows each on the shown page
	// again after a change.
	config   config.Config
	misread  error
	settings *store
	fields   []func()

	tabs, back, forward, reload, lock, field, address, view, status, list *ui.Node
	menu                                                                  *menu.Menu
	tools                                                                 *devtools.Panel
	inspect                                                               *inspector.Inspector

	// sessions is what each open tab holds beside its addresses.
	sessions map[*tabs.Tab]*session
	// landed brings what ended off the frame to Update, which runs it.
	landed chan func()
	// pictures are what the Document's img elements draw that no file
	// holds, by name: ui asks for them off the frame.
	pictures sync.Map

	visits *history.Log
	// unreadable is why the history's file could not be read.
	unreadable error
	// history and saved are the files the visits and the downloads are
	// kept in.
	history, saved *store

	// sites is what sites store through their scripts, stored its file,
	// and later what scripts asked of the tab, done once they returned.
	sites  *storage.Store
	stored *store
	later  []func()
	// jar is the cookies every request shares, jarFile its file, and
	// from the page a navigation is being opened by, "" for the
	// player's own.
	jar     *cookies.Jar
	jarFile *store
	from    string

	// saves runs the downloads; savesSeen is its version last shown, and
	// announced the state each download was last said to be in.
	saves     *downloads.Manager
	savesSeen uint64
	announced map[int]downloads.State
	// downloads is the navigation row's button, recent its list, and
	// listed the list of the downloads page while it is shown.
	downloads, progress, recent, listed *ui.Node
	recentShown, listedShown            listing

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
	// picked counts the times a page was asked what is under the pointer
	// for its scripts, for tests.
	picked int
}

// New is the screen of win, on the start page.
func New(win *platform.Window, opts Options) *Screen {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Screen{win: win, opts: opts, sessions: map[*tabs.Tab]*session{}, landed: make(chan func(), 64)}
	s.restore()
	s.browser = tabs.NewBrowser(s.config.StartPage)
	s.restoreDownloads()
	s.apply()
	s.doc = ui.NewDocument(ui.CSS(sheet), ui.CSS(menu.Sheet), ui.CSS(devtools.Sheet), ui.Images(icons()),
		ui.FetchImages(s.picture))
	s.build()
	s.header()
	s.show()
	return s
}

// restore reads the settings and the history from the folder light was
// given; with none, they are the defaults and empty.
func (s *Screen) restore() {
	s.config, s.visits = config.Default(), history.New(config.Default().HistoryLimit)
	s.settings = &store{name: configFile, write: func(path string) error { return config.Save(path, s.config) }}
	s.history = &store{name: historyFile, write: func(path string) error { return history.Save(path, s.visits) }}
	s.sites = storage.New()
	s.stored = &store{name: storageFile, write: func(path string) error { return s.sites.Save(path) }}
	s.jar = cookies.New()
	s.jarFile = &store{name: cookiesFile, write: func(path string) error { return s.jar.Save(path) }}
	if s.opts.Dir == "" {
		return
	}
	s.config, s.misread = config.Load(s.settings.path(s.opts.Dir))
	s.visits, s.unreadable = history.Load(s.history.path(s.opts.Dir), s.config.HistoryLimit)
	var unread error
	if s.sites, unread = storage.Load(s.stored.path(s.opts.Dir)); s.unreadable == nil {
		s.unreadable = unread
	}
	if s.jar, unread = cookies.Load(s.jarFile.path(s.opts.Dir)); s.unreadable == nil {
		s.unreadable = unread
	}
}

// keep writes the files that changed and were not written lately.
func (s *Screen) keep() {
	if s.sites.Changed() {
		s.stored.touch()
	}
	if s.jar.Changed() {
		s.jarFile.touch()
	}
	for _, file := range []*store{s.settings, s.history, s.saved, s.stored, s.jarFile} {
		if err := file.keep(s.opts.Dir); err != nil {
			s.unreadable = err
		}
	}
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
		Unscripted: s.unscripted,
		Pointer: func() (geom.Vec2, bool) {
			in := s.win.Input()
			return in.Mouse(), in.Down(input.MouseLeft)
		},
		Resized: func(height int) {
			s.config.ToolsHeight = height
			s.settings.touch()
		},
		Source:  func(address string, _ int) { s.OpenInNewTab(tabs.SourceOf(address)) },
		Open:    s.OpenInNewTab,
		Storage: s.storedBy,
	})
	s.tools.SetHeight(s.config.ToolsHeight)
	s.doc.Body().Append(
		s.tabs,
		s.navigation(),
		ui.Div(ui.ID("split"), ui.Div(ui.ID("stage"), s.view, s.inspect.Overlay(), s.status, s.list, s.recent),
			s.tools.Node()),
		s.shortcuts(),
		s.menu.Node())
	ui.OnContextMenu(s.contextMenu).Apply(s.doc.Body())
}

// Open shows address in the shown tab.
func (s *Screen) Open(address string) {
	s.session().note = ""
	s.browser.Tab().Visit(address)
	s.visit(address)
	s.show()
}

// OpenInNewTab shows address in a tab of its own.
func (s *Screen) OpenInNewTab(address string) {
	s.browser.Open(address)
	s.visit(address)
	s.show()
}

func (s *Screen) move(step func(*tabs.Tab)) {
	s.session().note = ""
	step(s.browser.Tab())
	s.show()
}

// Update runs what ended off the frame since the last one, has the shown
// web page drawn again (its Document lives by being drawn), and follows
// the window, the pointer and the requests.
func (s *Screen) Update(world.Frame) {
	s.header()
	s.closeUnderMiddle()
	s.closeMenuWhenMoved()
	s.follow()
	s.sizePicture()
	s.tools.Update()
	s.watchDownloads()
	s.keep()
	s.measure()
	s.runScripts()
	if s.host != nil {
		if page := s.session().pages[s.browser.Tab().Current()]; page != nil && page.Err != nil {
			s.show()
			return
		}
		if !s.overlaid() {
			s.doc.Invalidate(s.host)
		}
	}
	for {
		select {
		case land := <-s.landed:
			land()
		default:
			return
		}
	}
}

// overlaid reports whether a menu or a list of light's is open over the
// page. The page's Document is then left as last drawn: drawing it is
// what gives it the pointer, and a press on what is over it is not the
// page's.
func (s *Screen) overlaid() bool {
	return s.menu.IsOpen() || s.list.HasClass("shown") || s.recent.HasClass("shown") || s.inspect.Picking()
}

// Close ends the downloads in progress, whose files are removed, and
// the pages' scripts, and writes what is not yet written, as the window
// closes.
func (s *Screen) Close() error {
	s.saves.Close()
	s.saved.touch()
	for _, session := range s.sessions {
		s.end(session)
	}
	if s.sites.Changed() {
		s.stored.touch()
	}
	if s.jar.Changed() {
		s.jarFile.touch()
	}
	return errors.Join(s.settings.flush(s.opts.Dir), s.history.flush(s.opts.Dir), s.saved.flush(s.opts.Dir),
		s.stored.flush(s.opts.Dir), s.jarFile.flush(s.opts.Dir))
}

// show redraws the screen from the browser: the tabs, the shown tab's
// page, its address and its tools.
func (s *Screen) show() {
	s.forget()
	s.menu.Close()
	s.recent.RemoveClass("shown")
	s.showTabs()
	tab, session := s.browser.Tab(), s.session()
	empty(s.view)
	s.host, s.alone, s.listed, s.fields = nil, nil, nil, nil
	s.view.Append(s.page(tab.Current()))
	s.leave(tab, session)
	s.showNavigation()
	s.tools.SetDark(s.config.DarkMode)
	s.tools.Show(&session.tools, session.log, session.said, s.warnings)
	s.over = ""
	s.say()
}

// warnings are what ui refused in the shown page of the web.
func (s *Screen) warnings() []error {
	if page := s.session().pages[s.browser.Tab().Current()]; page != nil && page.Doc != nil {
		return page.Doc.Warnings()
	}
	return nil
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
