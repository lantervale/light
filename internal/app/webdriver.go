package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/world"

	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/tabs"
	"github.com/lantervale/light/pkg/webdriver"
)

const (
	// blankAddress is the empty page a window the driver opens starts on.
	blankAddress = "about:blank"
	// blankSource is that page.
	blankSource = "<!DOCTYPE html><html><head></head><body></body></html>"
	// settleFrames is how many frames a window is given to take a size
	// or a state it was asked for.
	settleFrames = 3
	// headerWait is how long the driver's server waits for the headers
	// of a request.
	headerWait = 10 * time.Second
)

// errDriverClosed is what a command is answered once light ends.
var errDriverClosed = errors.New("light is ending")

// step is a part of a command, run once a frame until it says it is
// done.
type step func() (done bool, err error)

// job is a command on its way from the driver's server to the frame.
type job struct {
	start func() step
	step  step
	done  chan error
	// gone is set once nobody waits for it.
	gone atomic.Bool
}

// Driver drives light for a WebDriver client: it implements
// webdriver.Browser over a Profile's windows. The server's goroutines
// only hand it commands and wait; each command runs on the World's
// frame, where Update is called, a step a frame until it is done, so
// the screens, the tabs, the Documents and the pages' scripts are
// touched by the frame alone. It is a system: add it to the World
// after the Profile.
type Driver struct {
	p        *Profile
	listener net.Listener
	server   *http.Server
	jobs     chan *job
	current  *job
	closed   chan struct{}

	// tab is the session's current window, nil once it closed; frame its
	// current frame, nil for the page itself; handles the names the tabs
	// are known by.
	tab     *tabs.Tab
	frame   *frame
	handles map[*tabs.Tab]string
	named   int

	keys devices
	// capture reads what a window shows, the engine's own way unless a
	// test replaces it.
	capture func(win *platform.Window, done func(shot, error)) step
}

// Drive serves WebDriver at address, an address of this machine alone,
// for the windows of p, which is from then on driven: its pages'
// scripts read navigator.webdriver as true.
func Drive(ctx context.Context, p *Profile, address string) (*Driver, error) {
	listener, err := webdriver.Listen(ctx, address)
	if err != nil {
		return nil, err
	}
	p.opts.Driven = true
	d := &Driver{p: p, listener: listener, jobs: make(chan *job), closed: make(chan struct{}),
		handles: map[*tabs.Tab]string{}}
	d.capture = d.captured
	d.server = &http.Server{Handler: webdriver.New(d, d.about()), ReadHeaderTimeout: headerWait}
	go func() {
		if failed := d.server.Serve(listener); failed != nil && !errors.Is(failed, http.ErrServerClosed) {
			log.Print("light: WebDriver: ", failed)
		}
	}()
	return d, nil
}

// Address is where the driver listens, as host:port.
func (d *Driver) Address() string { return d.listener.Addr().String() }

// Close stops the server, and answers the commands that wait.
func (d *Driver) Close() error {
	select {
	case <-d.closed:
		return nil
	default:
	}
	close(d.closed)
	return d.server.Close()
}

func (d *Driver) about() webdriver.About {
	return webdriver.About{Name: "light", Version: identity.Version(d.p.opts.Version), Platform: platformName(d.p.system),
		UserAgent: d.p.agent()}
}

// platformName is the system as WebDriver names it.
func platformName(system string) string {
	if command(system) == input.KeySuper {
		return "mac"
	}
	return system
}

// Update runs the commands that wait, each a step a frame: one that
// ends lets the next begin in the same frame.
func (d *Driver) Update(world.Frame) {
	for {
		if d.current == nil {
			select {
			case next := <-d.jobs:
				d.current = next
			default:
				return
			}
		}
		running := d.current
		if running.gone.Load() {
			d.current = nil
			continue
		}
		if running.step == nil {
			running.step = running.start()
		}
		done, err := running.step()
		if !done {
			return
		}
		running.done <- err
		d.current = nil
	}
}

// run hands a command to the frame and waits for it.
func (d *Driver) run(ctx context.Context, start func() step) error {
	handed := &job{start: start, done: make(chan error, 1)}
	select {
	case d.jobs <- handed:
	case <-ctx.Done():
		return ctx.Err()
	case <-d.closed:
		return errDriverClosed
	}
	select {
	case err := <-handed.done:
		return err
	case <-ctx.Done():
		handed.gone.Store(true)
		return ctx.Err()
	case <-d.closed:
		return errDriverClosed
	}
}

// once is a command of one step.
func (d *Driver) once(ctx context.Context, do func() error) error {
	return d.run(ctx, func() step { return func() (bool, error) { return true, do() } })
}

// flow is a command of several steps, each done before the next
// begins; a step may add more.
type flow struct {
	steps []step
	at    int
}

func (f *flow) then(s step) *flow {
	f.steps = append(f.steps, s)
	return f
}

// do adds a step that is done at once.
func (f *flow) do(fn func() error) *flow {
	return f.then(func() (bool, error) { return true, fn() })
}

// frames adds a wait of n frames.
func (f *flow) frames(n int) *flow {
	for range n {
		waited := false
		f.then(func() (bool, error) {
			was := waited
			waited = true
			return was, nil
		})
	}
	return f
}

func (f *flow) run() (bool, error) {
	for f.at < len(f.steps) {
		done, err := f.steps[f.at]()
		if err != nil {
			return true, err
		}
		if !done {
			return false, nil
		}
		f.at++
	}
	return true, nil
}

// flowing runs a flow as a command.
func (d *Driver) flowing(ctx context.Context, build func(f *flow)) error {
	return d.run(ctx, func() step {
		f := &flow{}
		build(f)
		return f.run
	})
}

func noWindow() error {
	return webdriver.Errorf(webdriver.NoSuchWindow, "the session's window is closed")
}

// screenOf is the screen that holds tab and its place there, nil for a
// tab that is closed.
func (d *Driver) screenOf(tab *tabs.Tab) (*Screen, int) {
	for _, s := range d.p.screens {
		for i, held := range s.browser.Tabs {
			if held == tab && tab != nil {
				return s, i
			}
		}
	}
	return nil, -1
}

// window is the screen of the session's window, which it shows: a tab
// behind another runs no script and draws nothing.
func (d *Driver) window() (*Screen, error) {
	s, i := d.screenOf(d.tab)
	if s == nil {
		d.tab, d.frame = nil, nil
		return nil, noWindow()
	}
	if s.browser.Tab() != d.tab {
		s.selectTab(i)
	}
	return s, nil
}

// handle is the name a tab is known by.
func (d *Driver) handle(tab *tabs.Tab) string {
	if d.handles[tab] == "" {
		d.named++
		d.handles[tab] = "light-" + strconv.Itoa(d.named)
	}
	return d.handles[tab]
}

// open are the handles of the open tabs, window by window, and forgets
// those of the closed ones.
func (d *Driver) open() []string {
	var all []string
	held := map[*tabs.Tab]bool{}
	for _, s := range d.p.screens {
		for _, tab := range s.browser.Tabs {
			held[tab] = true
			all = append(all, d.handle(tab))
		}
	}
	for tab := range d.handles {
		if !held[tab] {
			delete(d.handles, tab)
		}
	}
	return all
}

// NewSession takes the shown tab of light's first window as the
// session's window, on an empty page scripts run in.
func (d *Driver) NewSession(ctx context.Context, _ webdriver.Settings) error {
	return d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			if len(d.p.screens) == 0 {
				return webdriver.Errorf(webdriver.SessionNotCreated, "light has no window")
			}
			s := d.p.screens[0]
			d.tab, d.frame = s.browser.Tab(), nil
			s.Open(blankAddress)
			return nil
		}).frames(settleFrames)
	})
}

// EndSession lets go of the keys and buttons held and leaves light
// with one window of one tab, on its start page.
func (d *Driver) EndSession(ctx context.Context) error {
	return d.once(ctx, func() error {
		d.release()
		d.tab, d.frame = nil, nil
		for i, s := range d.p.screens {
			if i > 0 {
				s.win.RequestClose()
				continue
			}
			s.browser.CloseOthers(s.browser.Active)
			s.Open(s.config.StartPage)
		}
		return nil
	})
}

// Navigate opens address as an address typed is: the user's own
// navigation. A page the tab holds already is fetched again.
func (d *Driver) Navigate(ctx context.Context, address string, wait webdriver.Load) error {
	if u, err := url.Parse(address); err != nil || u.Scheme == "" {
		return webdriver.Errorf(webdriver.InvalidArgument, fmt.Sprintf("%q is no absolute address", address))
	}
	if _, _, lights := tabs.Own(address); !lights && !onTheWeb(shownBy(address)) && address != blankAddress {
		return webdriver.Errorf(webdriver.UnsupportedOperation, address+" is not opened: light opens "+own+
			", http://, https:// and about:blank addresses")
	}
	return d.moving(ctx, wait, func(s *Screen) { s.openFresh(address) })
}

func (d *Driver) Back(ctx context.Context, wait webdriver.Load) error {
	return d.moving(ctx, wait, func(s *Screen) {
		if s.browser.Tab().CanBack() {
			s.move((*tabs.Tab).Back)
		}
	})
}

func (d *Driver) Forward(ctx context.Context, wait webdriver.Load) error {
	return d.moving(ctx, wait, func(s *Screen) {
		if s.browser.Tab().CanForward() {
			s.move((*tabs.Tab).Forward)
		}
	})
}

func (d *Driver) Refresh(ctx context.Context, wait webdriver.Load) error {
	return d.moving(ctx, wait, func(s *Screen) {
		if s.loading() {
			s.reloadOrStop()
		}
		s.reloadOrStop()
	})
}

// moving moves the session's window, then waits for its page.
func (d *Driver) moving(ctx context.Context, wait webdriver.Load, move func(s *Screen)) error {
	return d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			s, err := d.window()
			if err != nil {
				return err
			}
			d.frame = nil
			move(s)
			f.then(d.landing(s, d.tab, wait))
			return nil
		})
	})
}

// landing waits for the page tab is at, as far as the strategy says and
// no longer than its timeout.
func (d *Driver) landing(s *Screen, tab *tabs.Tab, wait webdriver.Load) step {
	began := time.Now()
	return func() (bool, error) {
		if wait.Strategy == webdriver.LoadNone {
			return true, nil
		}
		if at, _ := d.screenOf(tab); at != s {
			return true, noWindow()
		}
		landed, err := s.arrived(tab, wait.Strategy)
		if !landed && err == nil && time.Since(began) > wait.Timeout {
			return true, webdriver.Errorf(webdriver.Timeout, fmt.Sprintf("%s did not load in %s", tab.Current(),
				wait.Timeout))
		}
		return landed, err
	}
}

// arrived reports whether the page tab is at has loaded as far as the
// strategy asks: fetched and built, and for a page whose scripts run,
// its document complete, or interactive for "eager". A page that
// failed is an error.
func (s *Screen) arrived(tab *tabs.Tab, strategy string) (bool, error) {
	session, address := s.sessionOf(tab), shownBy(tab.Current())
	if session.loads[address] != nil || session.pending != nil {
		return false, nil
	}
	page := session.pages[address]
	switch {
	case page == nil:
		return !onTheWeb(address), nil
	case page.Err != nil:
		return true, webdriver.Errorf(webdriver.UnknownError, address+" was not reached: "+page.Err.Error())
	case page.Doc == nil || !s.scripting() || !page.Scripted():
		return true, nil
	}
	live := session.scripts
	if live == nil || live.page != page || live.rt.Closed() {
		return tab != s.browser.Tab(), nil
	}
	state, _ := live.rt.Evaluate("document.readyState")
	return state == "" || state == `"complete"` || (state == `"interactive"` && strategy == webdriver.LoadEager), nil
}

func (d *Driver) CurrentURL(ctx context.Context) (string, error) {
	var address string
	err := d.once(ctx, func() error {
		s, err := d.window()
		if err == nil {
			address = s.active()
		}
		return err
	})
	return address, err
}

// active is the address of the document the shown tab shows: the page
// it holds while the next one loads, and about:blank while the first
// page of a tab has not come.
func (s *Screen) active() string {
	address := s.browser.Tab().Current()
	if at := shownBy(address); onTheWeb(at) && s.session().pages[at] == nil {
		return blankAddress
	}
	return address
}

func (d *Driver) WindowHandle(ctx context.Context) (string, error) {
	var handle string
	err := d.once(ctx, func() error {
		if s, _ := d.screenOf(d.tab); s == nil {
			return noWindow()
		}
		handle = d.handle(d.tab)
		return nil
	})
	return handle, err
}

func (d *Driver) WindowHandles(ctx context.Context) ([]string, error) {
	var handles []string
	err := d.once(ctx, func() error {
		handles = d.open()
		return nil
	})
	return handles, err
}

func (d *Driver) SwitchToWindow(ctx context.Context, handle string) error {
	return d.once(ctx, func() error {
		d.open()
		for tab, name := range d.handles {
			if name == handle {
				d.tab, d.frame = tab, nil
				_, err := d.window()
				return err
			}
		}
		return webdriver.Errorf(webdriver.NoSuchWindow, "no window "+handle)
	})
}

// CloseWindow closes the session's tab, and its window with its last
// one. light's last tab is not closed, as light would end with it: it
// goes back to the start page, and no window is left to the session.
func (d *Driver) CloseWindow(ctx context.Context) ([]string, error) {
	var left []string
	err := d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			s, i := d.screenOf(d.tab)
			if s == nil {
				return noWindow()
			}
			closed := d.tab
			d.tab, d.frame = nil, nil
			switch {
			case len(s.browser.Tabs) > 1:
				s.closeTab(i)
			case len(d.p.screens) > 1:
				s.win.RequestClose()
			default:
				s.browser.Close(i)
				s.show()
				return nil
			}
			f.then(func() (bool, error) {
				at, _ := d.screenOf(closed)
				left = d.open()
				return at == nil, nil
			})
			return nil
		})
	})
	return left, err
}

// NewWindow opens a tab behind the session's, or a window, on an empty
// page.
func (d *Driver) NewWindow(ctx context.Context, kind string) (string, string, error) {
	var handle string
	err := d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			s, err := d.window()
			if err != nil {
				return err
			}
			if kind != "window" {
				handle = d.handle(s.browser.OpenBeside(blankAddress, false))
				s.showTabs()
				return nil
			}
			before := len(d.p.screens)
			s.newWindow(blankAddress)
			f.then(func() (bool, error) {
				if len(d.p.screens) <= before {
					return false, nil
				}
				handle = d.handle(d.p.screens[len(d.p.screens)-1].browser.Tab())
				return true, nil
			})
			return nil
		})
	})
	return handle, kind, err
}

func rectOf(win *platform.Window) webdriver.Rect {
	at, size := win.CurrentPosition(), win.Size()
	return webdriver.Rect{X: float64(at.X), Y: float64(at.Y), Width: float64(size.X), Height: float64(size.Y)}
}

func (d *Driver) WindowRect(ctx context.Context) (webdriver.Rect, error) {
	var rect webdriver.Rect
	err := d.once(ctx, func() error {
		s, err := d.window()
		if err == nil {
			rect = rectOf(s.win)
		}
		return err
	})
	return rect, err
}

// SetWindowRect moves and sizes the window, in points, no smaller than
// light's windows go.
func (d *Driver) SetWindowRect(ctx context.Context, to webdriver.Placement) (webdriver.Rect, error) {
	return d.placing(ctx, func(win *platform.Window) {
		restore(win)
		if to.Width != nil && to.Height != nil {
			least := win.MinSize()
			win.SetSize(geom.V2(max(float32(*to.Width), least.X), max(float32(*to.Height), least.Y)))
		}
		if to.X != nil && to.Y != nil {
			win.SetPosition(geom.V2(float32(*to.X), float32(*to.Y)))
		}
	})
}

// restore brings a window back from full screen, maximized or
// minimized.
func restore(win *platform.Window) {
	if win.Fullscreen() {
		win.SetFullscreen(false)
	}
	if win.Maximized() || win.Minimized() {
		win.Restore()
	}
}

func (d *Driver) Maximize(ctx context.Context) (webdriver.Rect, error) {
	return d.placing(ctx, func(win *platform.Window) {
		restore(win)
		win.Maximize()
	})
}

func (d *Driver) Minimize(ctx context.Context) (webdriver.Rect, error) {
	return d.placing(ctx, func(win *platform.Window) {
		if win.Fullscreen() {
			win.SetFullscreen(false)
		}
		win.Minimize()
	})
}

func (d *Driver) Fullscreen(ctx context.Context) (webdriver.Rect, error) {
	return d.placing(ctx, func(win *platform.Window) {
		restore(win)
		win.SetFullscreen(true)
	})
}

// placing changes the session's window, gives it a few frames to
// follow, and answers where it is then.
func (d *Driver) placing(ctx context.Context, change func(win *platform.Window)) (webdriver.Rect, error) {
	var rect webdriver.Rect
	err := d.flowing(ctx, func(f *flow) {
		var win *platform.Window
		f.do(func() error {
			s, err := d.window()
			if err != nil {
				return err
			}
			win = s.win
			change(win)
			return nil
		}).frames(settleFrames).do(func() error {
			rect = rectOf(win)
			return nil
		})
	})
	return rect, err
}
