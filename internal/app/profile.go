package app

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lantervale/ember/color"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/world"

	"github.com/lantervale/light/pkg/assist"
	"github.com/lantervale/light/pkg/bookmarks"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/idb"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/storage"
)

// cascade is how far down and right of the window it comes from a new
// window opens, in points.
const cascade = 28

// Profile is what light's windows share: the settings, the history, the
// bookmarks, the cookies, what sites store, the downloads, the client that asks the
// web, and the files they are kept in. It is the system that runs the
// windows' screens.
type Profile struct {
	opts Options

	// config is the settings, misread why their file could not be read,
	// and settings their file.
	config   config.Config
	misread  error
	settings *store

	// client asks the web as the settings stand.
	client atomic.Pointer[fetch.Client]
	// pictures are what the Documents' img elements draw that no file
	// holds, by name: ui asks for them off the frame.
	pictures sync.Map

	visits *history.Log
	// recorded counts the changes of the history, for the windows that
	// list it.
	recorded uint64
	// unreadable is why the history's file could not be read.
	unreadable error
	// history and saved are the files the visits and the downloads are
	// kept in.
	history, saved *store

	// marks is the bookmarks, marked the count of their changes, for the
	// windows that show them, unkept why their file could not be read,
	// and markFile that file.
	marks    *bookmarks.List
	marked   uint64
	unkept   error
	markFile *store

	// assists are the features the model helps with, and assisted their
	// file.
	assists  *assist.Features
	assisted *store
	// memory is what the pages visited said (recall.go).
	memory *memory
	// sorted is the page categories.
	sorted sortedPages
	// digests are the pages left that the model sums up, by address.
	digests map[string]*consulted

	// sites is what sites store through their scripts, and stored its
	// file.
	sites  *storage.Store
	stored *store
	// bases is the IndexedDB databases of every site, which writes its
	// own files.
	bases *idb.Store
	// jar is the cookies every request shares, and jarFile its file.
	jar     *cookies.Jar
	jarFile *store
	// guard keeps the pages of the Internet from this machine and from
	// the network around it.
	guard *localnet.Guard
	// saves runs the downloads.
	saves *downloads.Manager

	// system is the system light runs on, as runtime.GOOS names it, and
	// clock the seconds its frames have taken.
	system string
	clock  float32
	// shaded is whether light's own pages were dark and spoken the
	// system's languages when the settings last held (followSystem).
	shaded bool
	spoken []string
	// warning is whether this run showed that light is an early preview.
	warning bool
	// shuts are the tabs closed since light started, the newest first,
	// for the start page, and shutCount how many times they changed.
	shuts     []shutTab
	shutCount uint64
	// asks are the questions scripts ask the user (asking.go).
	modals modals

	// screens are the open windows' screens, the oldest first; windows
	// the windows to open at the next frame, closed those that closed,
	// quitting whether light was told to end, and ended whether Close
	// ran. dragged are the selections made by a drag of the right button.
	screens  []*Screen
	windows  []ask
	closed   world.Reader[platform.WindowClosed]
	dragged  world.Reader[ui.RightDragSelected]
	quitting bool
	ended    bool
}

// ask is a window to open: at an address, from a screen's window.
type ask struct {
	address string
	from    *Screen
}

// NewProfile is what light's windows share, read from the folder opts
// names.
func NewProfile(opts Options) *Profile {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	p := &Profile{opts: opts, system: runtime.GOOS}
	p.restore()
	p.restoreDownloads()
	p.assistFeatures()
	p.recollect()
	p.restoreSorted()
	p.hold()
	return p
}

// Window is a window of light's: the first where the system puts it,
// another the size of from, the window it comes from, a little down and
// right of it. Closing it leaves the others, and ends light when it is
// the last.
func Window(from *platform.Window) *platform.Window {
	opts := []platform.Option{
		platform.Title("light"),
		platform.Size(1024, 680),
		platform.MinSize(760, 480),
		platform.OwnTitleBar(),
		platform.Background(color.Hex("#2a2530")),
		platform.Closing(platform.CloseQuitsIfLast),
	}
	if from != nil {
		size, at := from.Size(), from.CurrentPosition()
		opts = append(opts, platform.Size(int(size.X), int(size.Y)),
			platform.Position(int(at.X)+cascade, int(at.Y)+cascade))
	}
	return platform.NewWindow(opts...)
}

// restore reads the settings, the history, the bookmarks, the cookies
// and what sites store from the folder light was given; with none, they are the
// defaults and empty.
func (p *Profile) restore() {
	p.config, p.visits = config.Default(), history.New(config.Default().HistoryLimit)
	p.settings = &store{name: configFile, write: func(path string) error { return config.Save(path, p.config) }}
	p.history = &store{name: historyFile, write: func(path string) error { return history.Save(path, p.visits) }}
	p.marks = bookmarks.New()
	p.markFile = &store{name: bookmarksFile, write: func(path string) error { return bookmarks.Save(path, p.marks) }}
	p.sites = storage.New()
	p.bases = idb.New(p.opts.Dir)
	p.stored = &store{name: storageFile, write: func(path string) error { return p.sites.Save(path) }}
	p.jar = cookies.New()
	p.guard = &localnet.Guard{Base: p.opts.Transport, Space: p.opts.Space}
	p.jarFile = &store{name: cookiesFile, write: func(path string) error { return p.jar.Save(path) }}
	if p.opts.Dir == "" {
		return
	}
	p.config, p.misread = config.Load(p.settings.path(p.opts.Dir))
	p.visits, p.unreadable = history.Load(p.history.path(p.opts.Dir), p.config.HistoryLimit)
	p.marks, p.unkept = bookmarks.Load(p.markFile.path(p.opts.Dir))
	var unread error
	if p.sites, unread = storage.Load(p.stored.path(p.opts.Dir)); p.unreadable == nil {
		p.unreadable = unread
	}
	if p.jar, unread = cookies.Load(p.jarFile.path(p.opts.Dir)); p.unreadable == nil {
		p.unreadable = unread
	}
}

// restoreDownloads makes the downloads' manager, with the list of the
// run before.
func (p *Profile) restoreDownloads() {
	p.saves = downloads.New(p.downloadsDir(), opener(p.open))
	p.saved = &store{name: downloadsFile, write: func(path string) error {
		return downloads.Save(path, p.saves.Items())
	}}
	if p.opts.Dir == "" {
		return
	}
	items, err := downloads.Load(p.saved.path(p.opts.Dir), maxDownloads)
	p.unreadable = errors.Join(p.unreadable, err)
	p.saves.Restore(items)
}

// open asks through the client of the settings as they stand, so a
// download is sent as a page is.
func (p *Profile) open(ctx context.Context, address, accept string) (*fetch.Stream, error) {
	return p.client.Load().Open(ctx, address, accept)
}

// hold has the settings hold for what the windows share: the requests
// made from now on, the cookies, the network around, the downloads'
// folder and the history's cap. It notes what it held of the system.
func (p *Profile) hold() {
	c := p.config
	p.client.Store(fetch.New(&cookies.Transport{Jar: p.jar, Base: &netlog.Transport{Base: p.guard}},
		fetch.Agent(p.agent()), fetch.Languages(identity.AcceptLanguage(p.languages())),
		fetch.Redirects(c.Redirects)))
	p.jar.SetEnabled(c.Cookies)
	p.jar.SetThirdParty(c.ThirdPartyCookies)
	p.guard.Open(c.LocalNetwork)
	p.visits.SetLimit(c.HistoryLimit)
	p.saves.SetDir(p.downloadsDir())
	if p.modelGiven() {
		p.opts.Library.Configure(c.ModelSettings())
	}
	p.shaded, p.spoken = p.dark(), p.preferred()
}

// keep writes the files that changed and were not written lately.
func (p *Profile) keep() {
	if p.sites.Changed() {
		p.stored.touch()
	}
	if p.jar.Changed() {
		p.jarFile.touch()
	}
	for _, file := range []*store{p.settings, p.history, p.markFile, p.saved, p.stored, p.jarFile, p.assisted} {
		if err := file.keep(p.opts.Dir); err != nil {
			p.unreadable = err
		}
	}
	if err := p.sorted.file.keep(p.opts.Dir); err != nil {
		p.sorted.unread = err
	}
}

// Screen is the screen of win, a window of light's, on the start page.
func (p *Profile) Screen(win *platform.Window) *Screen { return p.screen(win, p.config.StartPage) }

// Update runs a frame of every window's screen, lets go of those whose
// window closed, and opens the windows asked for.
func (p *Profile) Update(f world.Frame) {
	for gone := range p.closed.Read(f.World) {
		p.letGo(gone.Window)
	}
	for made := range p.dragged.Read(f.World) {
		for _, s := range p.screens {
			s.excerpt.dragged = made.Node
		}
	}
	p.clock = f.Time.RealElapsed
	p.followSystem()
	p.warn()
	for _, s := range slices.Clone(p.screens) {
		s.tick()
	}
	p.keep()
	for _, w := range p.windows {
		win := Window(w.from.win)
		win.Add(p.screen(win, w.address).Document())
		f.World.AddEntity(win)
	}
	p.windows = nil
	if p.quitting {
		f.Quit()
	}
}

// letGo forgets the screen of a window that closed.
func (p *Profile) letGo(win *platform.Window) {
	for i, s := range p.screens {
		if s.win == win {
			s.shut()
			p.screens = slices.Delete(p.screens, i, i+1)
			return
		}
	}
}

// Close ends the downloads in progress, whose files are removed, and
// the pages' scripts, and writes what is not yet written, once, as
// light ends.
func (p *Profile) Close() error {
	if p.ended {
		return nil
	}
	p.ended = true
	p.saves.Close()
	p.saved.touch()
	for _, s := range p.screens {
		s.shut()
	}
	p.screens = nil
	if p.sites.Changed() {
		p.stored.touch()
	}
	if p.jar.Changed() {
		p.jarFile.touch()
	}
	var stopped error
	if p.modelGiven() {
		stopped = p.opts.Library.Close()
	}
	return errors.Join(stopped, p.endRecall(), p.sorted.file.flush(p.opts.Dir), p.assisted.flush(p.opts.Dir),
		p.settings.flush(p.opts.Dir),
		p.history.flush(p.opts.Dir), p.markFile.flush(p.opts.Dir),
		p.saved.flush(p.opts.Dir), p.stored.flush(p.opts.Dir), p.jarFile.flush(p.opts.Dir), p.bases.Close())
}
