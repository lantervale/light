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
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/frames"
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
	// acting is whether the scripts are being told of a click of the
	// player's: what they do meanwhile is the player's act.
	acting bool
	// kin are the documents the scripts share their engine with, nil
	// for none (framekin.go).
	kin *dom.Kin
}

// scripting reports whether pages are loaded for their scripts to run.
func (s *Screen) scripting() bool { return s.config.JavaScript && script.Available() }

// scriptsBody is the page of light's that runs scripts: built as a page
// of the web is, from HTML light holds.
func (s *Screen) scriptsBody() shownPage {
	session := s.session()
	page := session.pages[scriptsPage]
	if page == nil {
		page = s.current().Local(scriptsPage, scriptsDemo, web.Links{Open: s.activate, Over: s.point, Save: s.download})
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

// runScripts runs a frame of the scripts of the shown page, then of
// each of its frames' documents: it gives them what they posted each
// other, starts them the first time, then tells them of the pointer
// and the keys. A document with nothing to run has none, the page's
// until the console starts them. A tab that is not shown runs nothing.
func (s *Screen) runScripts() {
	defer s.settle()
	if s.asked() {
		return
	}
	tab, session := s.browser.Tab(), s.session()
	page := session.pages[tab.Current()]
	if s.host == nil || page == nil || page.Err != nil {
		return
	}
	set := session.frames[page]
	if set != nil {
		set.deliver()
	}
	live := session.scripts
	if live == nil && page.Scripted() {
		live = s.start(tab, session, page)
	}
	if live != nil && live.page == page {
		if live.at != tab.Current() {
			live.at = tab.Current()
			live.rt.Arrived(live.at)
		}
		live.rt.Frame()
	}
	s.runFrames(set)
	if set != nil {
		set.deliver()
	}
	s.click(page, set)
	s.keys(set)
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
	if live.kin = s.framesOf(tab, page).kinOf(nil); live.kin.Held() {
		return nil
	}
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

// scripted are the scripts that run in the shown page and its frames.
func (s *Screen) scripted(set *framed) []*running {
	var all []*running
	if live := s.session().scripts; live != nil {
		all = append(all, live)
	}
	if set != nil {
		for _, f := range set.all {
			if f.run != nil {
				all = append(all, f.run)
			}
		}
	}
	return all
}

// click tells the scripts of a click in their document, the page's or
// a frame's: the left button released over the element it was pressed
// on. The page is asked what is under the pointer only as the button
// goes down or comes up, and the scripts of one document never hear of
// a click in another.
func (s *Screen) click(page *web.Page, set *framed) {
	in := s.win.Input()
	pressed, released := in.Pressed(input.MouseLeft), in.Released(input.MouseLeft)
	if !pressed && !released {
		return
	}
	at, over := s.onPage()
	over = over && !s.overlaid()
	var (
		target *ui.Node
		live   *running
	)
	if over {
		s.picked++
		var within *frame
		target, within, at = s.under(page, at)
		if live = set.scriptsIn(s.session(), within); live == nil || live.page.Doc.Frame(target) != nil {
			target = nil
		}
	}
	switch {
	case pressed:
		for _, other := range s.scripted(set) {
			other.pressed, other.keyboard = nil, false
		}
		if live != nil {
			live.pressed, live.keyboard = target, true
		}
	case target != nil && target == live.pressed:
		zoom := live.page.Doc.Zoom()
		if zoom <= 0 {
			zoom = 1
		}
		live.acting = true
		live.rt.Click(target, at.X/zoom, at.Y/zoom, s.held())
		live.acting = false
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

// keys tells the scripts of the document last clicked in of the keys
// pressed since: what they typed, and those that type nothing. A text
// field with the keyboard keeps the second kind to itself.
func (s *Screen) keys(set *framed) {
	in := s.win.Input()
	if s.editing || s.overlaid() {
		return
	}
	for _, live := range s.scripted(set) {
		if !live.keyboard || live.rt.Closed() {
			continue
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
		Ask:     s.pageDialogs(tab, session, live),
		Hold:    &s.modalOf(tab).hold,
		Cleared: session.said.Clear,
		Navigate: func(address string, replace bool) {
			by := live.by(s)
			later(func() {
				if replace {
					by(func() {
						tab.Redirect(tab.Current(), address)
						s.show()
					})
					return
				}
				by(func() { s.Open(address) })
			})
		},
		Reload: func() { later(s.reloadOrStop) },
		Post: func(sent web.Submission) {
			by := live.by(s)
			later(func() { by(func() { s.posted(sent) }) })
		},
		Moved: func(how, address string) {
			if how == "replace" {
				tab.Redirect(tab.Current(), address)
			} else {
				tab.Push(address)
			}
			session.pages[address], live.at = live.page, address
			later(s.showNavigation)
		},
		Travel:  func(delta int) { later(func() { s.travel(nil, live.acting, delta) }) },
		Steps:   tab.Len,
		Fetch:   func(q dom.Request, done func(dom.Response)) func() { return s.fetchFor(session, live, q, done) },
		Cookies: s.jar.Page(live.address),
		Storage: func(area string) dom.Storage { return s.storageIn(session, "", originOf(live.page.Address), area) },
		Bases:   s.pageDatabases(live),
		Windows: &windows{s: s, set: s.framesOf(tab, live.page)},
		Kin:     live.kin, Self: topWindow,
		Loading: func() bool { return s.framesOf(tab, live.page).loading(nil) },
		Broadcast: func(name string, m dom.Posted) {
			if origin := frames.Origin(live.page.Base); origin != opaqueOrigin && origin != "" {
				s.broadcast(live, district{origin: origin}, name, m)
			}
		},
		Viewport: func() (float32, float32) {
			if s.host == nil {
				return 0, 0
			}
			box, _ := s.doc.BoxOf(s.host)
			zoom := live.page.Doc.Zoom()
			return box.W / zoom, box.H / zoom
		},
		Titled:    func(string) { later(s.showTabs) },
		Navigator: s.navigator(),
		Window:    func() dom.Window { return s.around(live.page.Doc.Zoom()) },
		Budget:    s.opts.ScriptBudget,
	}
}

// fetchFor fetches what a page's script asked for, off the frame, as
// the tab's own requests are: listed in its Network panel as a
// script's.
func (s *Screen) fetchFor(session *session, live *running, q dom.Request, done func(dom.Response)) func() {
	if q.Address == scriptsData {
		go done(dom.Response{Status: http.StatusOK, StatusText: "200 OK", Address: q.Address, Text: scriptsAnswer,
			Header: map[string]string{"Content-Type": "application/json"}, Kind: string(fetch.Basic)})
		return func() {}
	}
	page := live.address()
	ctx, cancel := context.WithTimeout(asking(cookies.Within(context.Background(), page), live, q), s.timeout())
	client := s.current().Client
	if q.Mode != "" || (!q.Script && !q.Sheet) {
		go func() {
			defer cancel()
			done(called(ctx, client, session.log, fetch.Call{Page: page}, q))
		}()
		return cancel
	}
	kind := netlog.Script
	if q.Sheet {
		kind = netlog.Stylesheet
	}
	go func() {
		defer cancel()
		done(got(client.Get(netlog.With(ctx, session.log, asker(q), kind), q.Address, "*/*")))
	}()
	return cancel
}

// called is what came of a fetch of the script of a document, by, or
// of its asking for a module script: sent as a
// browser sends it, another origin's answer given only when that origin
// shares it. A request that was blocked is a blocked row of log: one of
// its own when nothing was sent, the row of what was sent otherwise.
func called(ctx context.Context, client *fetch.Client, log *netlog.Log, by fetch.Call, q dom.Request) dom.Response {
	kind := netlog.XHR
	if q.Script {
		kind = netlog.Script
	}
	ctx, trail := netlog.Trailed(netlog.With(ctx, log, asker(q), kind))
	header := http.Header{}
	for name, value := range q.Header {
		header.Set(name, value)
	}
	by.Method, by.Address, by.Header, by.Body = q.Method, q.Address, header, q.Body
	by.Mode, by.Credentials, by.Redirect = fetch.Mode(q.Mode), fetch.Credentials(q.Credentials), fetch.Redirect(q.Redirect)
	result, err := client.Script(ctx, by)
	var blocked *fetch.BlockedError
	switch {
	case errors.As(err, &blocked) && blocked.Sent:
		log.Refuse(trail.Last(), blocked.Why)
	case errors.As(err, &blocked):
		log.Block(q.Method, blocked.Address, asker(q), kind, blocked.Why)
	}
	if err != nil {
		return dom.Response{Err: err}
	}
	return dom.Response{Status: result.Status, StatusText: http.StatusText(result.Status), Address: result.Address,
		Header: flat(result.Header), Body: result.Body, Kind: string(result.Kind), Redirected: result.Redirected}
}

// flat is an answer's headers as scripts read them, a value each, but
// the cookies it sets.
func flat(header http.Header) map[string]string {
	out := map[string]string{}
	for name := range header {
		if name != "Set-Cookie" {
			out[name] = strings.Join(header.Values(name), ", ")
		}
	}
	return out
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

// storageIn is an area of the storage of the documents of an origin:
// what they keep between runs ("local"), or for as long as their tab
// ("session"). top is "" for the site's own, and the site of the tab's
// page for what a frame of another site, or in one, keeps apart under
// it. A document with no site of its own keeps nothing longer than
// itself.
func (s *Screen) storageIn(session *session, top, origin, area string) dom.Storage {
	switch {
	case origin == "":
		return nil
	case area == "local" && top == "":
		return s.sites.Area(origin)
	case area == "local":
		return s.sites.Framed(top, origin)
	}
	key := origin
	if top != "" {
		key = top + "\x00" + origin
	}
	if session.kept[key] == nil {
		session.kept[key] = tabStorage{}
	}
	return session.kept[key]
}

// fetchInFrame is fetchFor for the scripts of the document f shows.
func (s *Screen) fetchInFrame(f *frame, live *running, q dom.Request, done func(dom.Response)) func() {
	session := f.set.session
	ctx, cancel := context.WithTimeout(s.asksInFrame(context.Background(), f, live, q), s.timeout())
	client := s.current().Client
	if q.Mode != "" || (!q.Script && !q.Sheet) {
		by := fetch.Call{Page: live.address(), Opaque: f.held.Opaque(), Top: f.set.top.Base}
		go func() {
			defer cancel()
			done(called(ctx, client, session.log, by, q))
		}()
		return cancel
	}
	kind := netlog.Script
	if q.Sheet {
		kind = netlog.Stylesheet
	}
	go func() {
		defer cancel()
		done(got(client.Get(netlog.With(ctx, session.log, asker(q), kind), q.Address, "*/*")))
	}()
	return cancel
}

// got is the answer to a script element's or a sheet's request, as
// its document's scripts are told it.
func got(answer *fetch.Answer, err error) dom.Response {
	if err != nil {
		return dom.Response{Err: err}
	}
	return dom.Response{Status: answer.Status, StatusText: http.StatusText(answer.Status), Address: answer.Address,
		Header: flat(answer.Header), Text: answer.Code()}
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
	s.name()
	return s.evaluated(live, source)
}

// evaluated is what a line of the console gives in a document's
// scripts.
func (s *Screen) evaluated(live *running, source string) (string, error) {
	began := time.Now()
	value, err := live.rt.Evaluate(source)
	if err != nil && time.Since(began) > time.Second {
		return "", errors.New("the line ran too long and was stopped")
	}
	return value, err
}
