package app

import (
	"context"
	_ "embed"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/script"
	"github.com/lantervale/light/pkg/tabs"
)

const (
	// scriptsPage is the page that shows what light runs of JavaScript.
	scriptsPage = own + scriptsName
	// scriptsData is what that page fetches.
	scriptsData = scriptsPage + "/data.json"
	// storageFile is the file of what sites store, in the folder light
	// is given.
	storageFile = "storage.json"
	// consoleLines is how many lines a tab's console keeps.
	consoleLines = 1000
)

//go:embed scripts.html
var scriptsDemo string

// scriptsAnswer is what scriptsData answers.
const scriptsAnswer = `{"message": "fetched by a script", "items": ["through light's fetcher", "listed in Network"]}`

// running is the scripts of the page a tab shows.
type running struct {
	page *web.Page
	rt   *dom.Runtime
	// at is the address the scripts know their page at, pressed the
	// element the pointer went down on, and keyboard whether the last
	// press was in the page, which then hears the keys.
	at       string
	pressed  *ui.Node
	keyboard bool
}

// scripting reports whether pages are loaded for their scripts to run.
func (s *Screen) scripting() bool { return s.config.JavaScript && script.Available() }

// scriptsBody is the page of light's that runs scripts: built as a page
// of the web is, from HTML light holds.
func (s *Screen) scriptsBody() shownPage {
	session := s.session()
	page := session.pages[scriptsPage]
	if page == nil {
		page = s.current().Local(scriptsPage, scriptsDemo, web.Links{Open: s.Open, Over: s.point, Save: s.download})
		session.pages[scriptsPage] = page
		s.zoomPages(session)
	}
	if page.Err != nil {
		return s.unreachable(scriptsPage, page.Err.Error())
	}
	toggle(page.Doc.Body(), "dark", s.config.DarkMode)
	return s.hosted(page)
}

// leave ends the scripts of the page a tab showed once it shows
// another. A page whose scripts ended is fetched again when the tab
// comes back to it.
func (s *Screen) leave(tab *tabs.Tab, session *session) {
	if was := session.scripts; was != nil && session.pages[shownBy(tab.Current())] != was.page {
		s.end(session)
	}
}

// end closes a tab's scripts and forgets their page.
func (s *Screen) end(session *session) {
	was := session.scripts
	if was == nil {
		return
	}
	session.scripts = nil
	was.rt.Close()
	for address, page := range session.pages {
		if page == was.page {
			delete(session.pages, address)
		}
	}
}

// runScripts runs a frame of the shown page's scripts: it starts them
// the first time, then tells them of the pointer and the keys. A page
// with nothing to run has none until the console starts them. A tab
// that is not shown runs nothing.
func (s *Screen) runScripts() {
	defer s.settle()
	tab, session := s.browser.Tab(), s.session()
	page := session.pages[tab.Current()]
	live := session.scripts
	if s.host == nil || page == nil || page.Err != nil || live == nil && !page.Scripted() {
		return
	}
	if live == nil {
		if live = s.start(tab, session, page); live == nil {
			return
		}
	}
	if live.at != tab.Current() {
		live.at = tab.Current()
		live.rt.Arrived(live.at)
	}
	live.rt.Frame()
	s.click(live)
	s.keys(live)
}

// settle does what scripts asked of the tab while they ran.
func (s *Screen) settle() {
	later := s.later
	s.later = nil
	for _, do := range later {
		do()
	}
}

// start makes the scripts of a page and runs them, unless another tab
// runs them already.
func (s *Screen) start(tab *tabs.Tab, session *session, page *web.Page) *running {
	if page.Tree.Activate != nil {
		return nil
	}
	live := &running{page: page, at: tab.Current()}
	rt, err := dom.New(page, s.scriptHost(tab, session, live))
	if err != nil {
		session.said.Add(console.Line{Level: console.Error, Text: "The page's scripts do not run: " + err.Error(),
			Source: page.Address})
		page.Scripts, page.Tree = nil, nil
		return nil
	}
	live.rt = rt
	session.scripts = live
	rt.Start()
	return live
}

// click tells the scripts of a click in their page: the left button
// released over the element it was pressed on. The page is asked what is
// under the pointer only as the button goes down or comes up.
func (s *Screen) click(live *running) {
	in := s.win.Input()
	pressed, released := in.Pressed(input.MouseLeft), in.Released(input.MouseLeft)
	if !pressed && !released {
		return
	}
	at, over := s.doc.PointerOn(s.host)
	over = over && !s.overlaid()
	var target *ui.Node
	if over {
		s.picked++
		target = live.page.Doc.ElementAt(at)
	}
	switch {
	case pressed:
		live.pressed, live.keyboard = target, over
	case target != nil && target == live.pressed:
		zoom := live.page.Doc.Zoom()
		live.rt.Click(target, at.X/zoom, at.Y/zoom)
	}
}

// silent are the keys that type nothing, as scripts name them.
func silent() []struct {
	key  input.Key
	name string
} {
	return []struct {
		key  input.Key
		name string
	}{
		{input.KeyEnter, "Enter"}, {input.KeyEscape, "Escape"}, {input.KeyBackspace, "Backspace"},
		{input.KeyTab, "Tab"}, {input.KeyDelete, "Delete"}, {input.KeyLeft, "ArrowLeft"},
		{input.KeyRight, "ArrowRight"}, {input.KeyUp, "ArrowUp"}, {input.KeyDown, "ArrowDown"},
		{input.KeyHome, "Home"}, {input.KeyEnd, "End"},
	}
}

// keys tells the scripts of the keys pressed since the page was last
// clicked in: what they typed, and those that type nothing. A text
// field with the keyboard keeps the second kind to itself.
func (s *Screen) keys(live *running) {
	in := s.win.Input()
	if !live.keyboard || s.editing || s.overlaid() {
		return
	}
	for _, r := range in.Text() {
		live.rt.Key(string(r), codeOf(r))
	}
	for _, k := range silent() {
		if in.Pressed(k.key) {
			live.rt.Key(k.name, k.name)
		}
	}
}

// codeOf is the code of the key that types r on a US keyboard, "" for
// one light does not know.
func codeOf(r rune) string {
	switch {
	case r == ' ':
		return "Space"
	case r >= 'a' && r <= 'z':
		return "Key" + strings.ToUpper(string(r))
	case r >= 'A' && r <= 'Z':
		return "Key" + string(r)
	case r >= '0' && r <= '9':
		return "Digit" + string(r)
	}
	return ""
}

// scriptHost is what a page's scripts ask of the screen. What moves
// the tab waits for the scripts to return.
func (s *Screen) scriptHost(tab *tabs.Tab, session *session, live *running) dom.Host {
	shown := func() bool { return s.browser.Tab() == tab && session.scripts == live }
	later := func(do func()) {
		s.later = append(s.later, func() {
			if shown() {
				do()
			}
		})
	}
	return dom.Host{
		Console: func(m dom.Message) {
			session.said.Add(console.Line{Level: console.Level(m.Level), Text: m.Text, Source: m.Source, Line: m.Line,
				Stack: m.Stack, Depth: m.Depth, At: m.At})
		},
		Cleared: session.said.Clear,
		Navigate: func(address string, replace bool) {
			later(func() {
				if replace {
					s.byPage(func() {
						tab.Redirect(tab.Current(), address)
						s.show()
					})
					return
				}
				s.linked(address)
			})
		},
		Reload: func() { later(s.reloadOrStop) },
		Moved: func(how, address string) {
			if how == "replace" {
				tab.Redirect(tab.Current(), address)
			} else {
				tab.Visit(address)
			}
			session.pages[address], live.at = live.page, address
			later(s.showNavigation)
		},
		Travel: func(delta int) {
			later(func() {
				for ; delta < 0; delta++ {
					tab.Back()
				}
				for ; delta > 0; delta-- {
					tab.Forward()
				}
				s.show()
			})
		},
		Steps:   tab.Len,
		Fetch:   func(q dom.Request, done func(dom.Response)) func() { return s.fetchFor(session, live, q, done) },
		Cookies: s.jar.Page(live.address),
		Refused: func(q dom.Request) {
			session.log.Block(q.Method, q.Address, asker(q), netlog.XHR, "light sends no "+q.Method+" for a script")
		},
		Storage: func(area string) dom.Storage { return s.storageOf(session, live.page.Address, area) },
		Viewport: func() (float32, float32) {
			if s.host == nil {
				return 0, 0
			}
			box, _ := s.doc.BoxOf(s.host)
			zoom := live.page.Doc.Zoom()
			return box.W / zoom, box.H / zoom
		},
		Titled: func(string) { later(s.showTabs) },
		Agent:  s.config.UserAgent,
		Budget: s.opts.ScriptBudget,
	}
}

// fetchFor fetches what a page's script asked for, off the frame, as
// the tab's own requests are: listed in its Network panel as a
// script's.
func (s *Screen) fetchFor(session *session, live *running, q dom.Request, done func(dom.Response)) func() {
	if q.Address == scriptsData {
		go done(dom.Response{Status: http.StatusOK, StatusText: "200 OK", Address: q.Address, Text: scriptsAnswer,
			Header: map[string]string{"Content-Type": "application/json"}})
		return func() {}
	}
	ctx, cancel := context.WithTimeout(cookies.Within(context.Background(), live.address()), s.timeout())
	kind, loader := netlog.XHR, s.current()
	switch {
	case q.Script:
		kind = netlog.Script
	case q.Sheet:
		kind = netlog.Stylesheet
	}
	go func() {
		defer cancel()
		answer, err := loader.Client.Get(netlog.With(ctx, session.log, asker(q), kind), q.Address, "*/*")
		if err != nil {
			done(dom.Response{Err: err})
			return
		}
		header := map[string]string{}
		for name := range answer.Header {
			if name != "Set-Cookie" {
				header[name] = answer.Header.Get(name)
			}
		}
		done(dom.Response{Status: answer.Status, StatusText: http.StatusText(answer.Status), Address: answer.Address,
			Header: header, Text: answer.Code()})
	}()
	return cancel
}

// asker is who a request of a script is listed as asked by: the script
// and the line of a fetch or an XMLHttpRequest, as the console names
// them, "script" for a script element and when they are unknown.
func asker(q dom.Request) string {
	if where := (console.Line{Source: q.Source, Line: q.Line}).Where(); where != "" && !q.Script && !q.Sheet {
		return where
	}
	return "script"
}

// storageOf is an area of the storage of the site a page is of: what it
// keeps between runs ("local"), or for as long as its tab ("session").
// A page with no site of its own keeps nothing longer than itself.
func (s *Screen) storageOf(session *session, address, area string) dom.Storage {
	origin := originOf(address)
	switch {
	case origin == "":
		return nil
	case area == "local":
		return s.sites.Area(origin)
	}
	if session.kept[origin] == nil {
		session.kept[origin] = tabStorage{}
	}
	return session.kept[origin]
}

// originOf is the site an address is of, "" for one that has none.
func originOf(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// tabStorage is what a site keeps for as long as its tab.
type tabStorage map[string]string

func (t tabStorage) Get(key string) (string, bool) {
	value, ok := t[key]
	return value, ok
}
func (t tabStorage) Set(key, value string) { t[key] = value }
func (t tabStorage) Remove(key string)     { delete(t, key) }
func (t tabStorage) Clear()                { clear(t) }
func (t tabStorage) Keys() []string {
	keys := make([]string, 0, len(t))
	for key := range t {
		keys = append(keys, key)
	}
	return keys
}

var errNoScripts = errors.New("this tab's page runs no script")

// unscripted is why the shown tab's page runs no script, "" when it
// does or would.
func (s *Screen) unscripted() string {
	switch {
	case !script.Available():
		return "This build of light has no JavaScript engine: no script runs, and the console evaluates nothing. " +
			"Build it with -tags v8 (task run) to have one."
	case !s.config.JavaScript:
		return "JavaScript is off in the settings (" + configPage + "): pages loaded so run no script, and the " +
			"console evaluates nothing."
	}
	return ""
}

// evaluate runs what the console's line holds in the shown tab's page,
// whose scripts start for it when the page had none to run.
func (s *Screen) evaluate(source string) (string, error) {
	tab, session := s.browser.Tab(), s.session()
	live := session.scripts
	if page := session.pages[tab.Current()]; live == nil && s.host != nil && page != nil && page.Err == nil &&
		page.Tree != nil {
		live = s.start(tab, session, page)
	}
	if live == nil {
		return "", errNoScripts
	}
	live.rt.Inspect(s.inspect.Selected())
	began := time.Now()
	value, err := live.rt.Evaluate(source)
	if err != nil && time.Since(began) > time.Second {
		return "", errors.New("the line ran too long and was stopped")
	}
	return value, err
}
