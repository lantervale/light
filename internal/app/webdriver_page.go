package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/script"
	"github.com/lantervale/light/pkg/webdriver"
)

// driverScript is what the driver asks of a page, in the page's own
// context: evaluated once in each document it reads.
//
//go:embed webdriver.js
var driverScript string

// contextWait is how long a command waits for a document that is on
// its way before it reads it.
const contextWait = 30 * time.Second

// errOnItsWay is why a document cannot be read yet: it is loading.
var errOnItsWay = errors.New("the document is loading")

// blankBody is the empty page: a document with nothing in it, which
// scripts run in as in any page.
func (s *Screen) blankBody() shownPage {
	session := s.session()
	page := session.pages[blankAddress]
	if page == nil {
		page = s.current().Local(blankAddress, blankSource, web.Links{Open: s.followed, Over: s.point,
			Save: s.download})
		session.pages[blankAddress] = page
		s.zoomPages(session)
	}
	if page.Err != nil {
		return s.unreachable(blankAddress, page.Err.Error())
	}
	return s.hosted(page)
}

// shownWeb is the page of the web the screen shows, nil for none.
func (s *Screen) shownWeb() *web.Page {
	if s.host == nil {
		return nil
	}
	if page := s.session().pages[s.browser.Tab().Current()]; page != nil && page.Err == nil && page.Doc != nil {
		return page
	}
	return nil
}

// unscriptedPage is why a command that reads a document has none.
func unscriptedPage(why string) error {
	return webdriver.Errorf(webdriver.UnsupportedOperation, why)
}

// document is the scripts of the session's current document, the
// page's or its current frame's, started for the driver when the
// document had none to run; errOnItsWay while it loads.
func (d *Driver) document(s *Screen) (*running, error) {
	if why := s.unscripted(); why != "" {
		return nil, unscriptedPage(why + " WebDriver reads a page through its scripts' context: without one it " +
			"runs no script and finds no element.")
	}
	tab, session := s.browser.Tab(), s.session()
	address := shownBy(tab.Current())
	if session.loads[address] != nil || (onTheWeb(address) && session.pages[address] == nil) {
		return nil, errOnItsWay
	}
	page := s.shownWeb()
	if page == nil {
		return nil, unscriptedPage(tab.Current() + " is no document scripts run in: it is a page of light's own, " +
			"a file shown as it is, or a page that was not reached.")
	}
	if d.frame != nil {
		return d.framed(s, page)
	}
	live := session.scripts
	if (live == nil || live.page != page) && page.Tree != nil {
		live = s.start(tab, session, page)
	}
	if live == nil || live.page != page || live.rt.Closed() {
		return nil, unscriptedPage("the scripts of " + page.Address + " do not run: the console says why.")
	}
	return live, nil
}

// framed is document for the session's current frame.
func (d *Driver) framed(s *Screen, top *web.Page) (*running, error) {
	f := d.frame
	if f.gone || !f.live || s.session().frames[top] != f.set {
		d.frame = nil
		return nil, webdriver.Errorf(webdriver.NoSuchWindow, "the session's frame is no longer in its page")
	}
	if f.load != nil {
		return nil, errOnItsWay
	}
	if f.run == nil && f.page != nil && f.page.Tree != nil && f.page.Tree.Activate == nil && f.held.Runs() {
		s.startFrame(f)
	}
	if f.run == nil || f.run.rt.Closed() {
		return nil, unscriptedPage("the frame's document runs no script: it is empty, sandboxed without " +
			"allow-scripts, or was not shown.")
	}
	return f.run, nil
}

// reply is what the driver's script answers.
type reply struct {
	Value   json.RawMessage `json:"value"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Stack   string          `json:"stack"`
}

// literal is text as a string of a script.
func literal(text string) string {
	quoted, _ := json.Marshal(text)
	return string(quoted)
}

// ask asks the driver's script of a document for a command, giving it
// the script first where it is not yet.
func (d *Driver) ask(rt *dom.Runtime, command string, args ...any) (json.RawMessage, error) {
	if args == nil {
		args = []any{}
	}
	asked, err := json.Marshal(map[string]any{"command": command, "args": args})
	if err != nil {
		return nil, webdriver.Errorf(webdriver.InvalidArgument, err.Error())
	}
	source := "__lightwd.call(" + literal(string(asked)) + ")"
	out, err := rt.Evaluate(source)
	if err != nil && strings.Contains(err.Error(), "__lightwd") {
		if _, err = rt.Evaluate(driverScript); err == nil {
			_, _ = rt.Evaluate("__lightwd.call(" + literal(`{"command":"handle","args":[`+literal(d.handle(d.tab))+
				`]}`) + ")")
			out, err = rt.Evaluate(source)
		}
	}
	switch {
	case errors.Is(err, dom.ErrAsking):
		return nil, errPrompted
	case errors.Is(err, script.ErrInterrupted):
		return nil, webdriver.Errorf(webdriver.ScriptTimeout, "the script ran longer than light lets one run "+
			"without a pause, and was stopped")
	case errors.Is(err, script.ErrClosed):
		return nil, webdriver.Errorf(webdriver.JavaScriptError, "the document was left while it was read")
	case err != nil:
		return nil, webdriver.Errorf(webdriver.JavaScriptError, err.Error())
	}
	var encoded string
	var answered reply
	if json.Unmarshal([]byte(out), &encoded) != nil || json.Unmarshal([]byte(encoded), &answered) != nil {
		return nil, webdriver.Errorf(webdriver.UnknownError, "light's script answered "+out)
	}
	if answered.Error != "" {
		code := webdriver.Code(answered.Error)
		if !code.Known() {
			code = webdriver.UnknownError
		}
		return nil, &webdriver.Error{Code: code, Message: answered.Message, Stack: answered.Stack}
	}
	return answered.Value, nil
}

// reading adds to f the wait for the session's document, then what
// reads it.
func (d *Driver) reading(f *flow, read func(s *Screen, live *running) error) {
	began := time.Now()
	f.then(func() (bool, error) {
		s, err := d.window()
		if err != nil {
			return true, err
		}
		live, err := d.document(s)
		if errors.Is(err, errOnItsWay) {
			if time.Since(began) > contextWait {
				return true, webdriver.Errorf(webdriver.Timeout, "the document is still loading")
			}
			return false, nil
		}
		if err != nil {
			return true, err
		}
		return true, read(s, live)
	})
}

// asking is a command that asks the document one thing, read into
// into when it is not nil.
func (d *Driver) asking(ctx context.Context, into any, command string, args ...any) error {
	return d.flowing(ctx, func(f *flow) {
		d.reading(f, func(_ *Screen, live *running) error {
			value, err := d.ask(live.rt, command, args...)
			if err != nil || into == nil {
				return err
			}
			return json.Unmarshal(value, into)
		})
	})
}

// Title is the title of the window's page, as its scripts have it now.
func (d *Driver) Title(ctx context.Context) (string, error) {
	var title string
	err := d.once(ctx, func() error {
		s, err := d.window()
		if err != nil {
			return err
		}
		tab, session := s.browser.Tab(), s.session()
		page := session.pages[shownBy(tab.Current())]
		if unread := page == nil || page.Err != nil; unread {
			title = s.title(tab.Current())
			return nil
		}
		title = page.Title
		if live := session.scripts; live != nil && live.page == page && !live.rt.Closed() {
			if value, failed := d.ask(live.rt, "title"); failed == nil {
				_ = json.Unmarshal(value, &title)
			}
		}
		return nil
	})
	return title, err
}

// PageSource is the document as its scripts have it now; in a build
// that runs none, the page as it was sent.
func (d *Driver) PageSource(ctx context.Context) (string, error) {
	var source string
	err := d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			s, err := d.window()
			if err != nil {
				return err
			}
			if s.unscripted() == "" {
				d.reading(f, func(_ *Screen, live *running) error {
					value, failed := d.ask(live.rt, "source")
					if failed != nil {
						return failed
					}
					return json.Unmarshal(value, &source)
				})
				return nil
			}
			if page := s.session().pages[shownBy(s.browser.Tab().Current())]; page != nil {
				source = page.Source
			}
			return nil
		})
	})
	return source, err
}

// FindElements looks in the session's document, again each frame for
// as long as wait while nothing is found.
func (d *Driver) FindElements(ctx context.Context, by webdriver.Locator, wait time.Duration) ([]string, error) {
	var found []string
	err := d.flowing(ctx, func(f *flow) {
		began := time.Now()
		var look func(s *Screen, live *running) error
		look = func(_ *Screen, live *running) error {
			value, err := d.ask(live.rt, "find", by.Using, by.Value, by.From)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(value, &found); err == nil && len(found) == 0 && time.Since(began) < wait {
				f.frames(1)
				d.reading(f, look)
			}
			return err
		}
		d.reading(f, look)
	})
	return found, err
}

func (d *Driver) ActiveElement(ctx context.Context) (string, error) {
	var id string
	err := d.asking(ctx, &id, "active")
	return id, err
}

func (d *Driver) ElementAttribute(ctx context.Context, element, name string) (*string, error) {
	var value *string
	err := d.asking(ctx, &value, "attribute", element, name)
	return value, err
}

func (d *Driver) ElementProperty(ctx context.Context, element, name string) (json.RawMessage, error) {
	var value json.RawMessage
	err := d.asking(ctx, &value, "property", element, name)
	return value, err
}

func (d *Driver) ElementCSSValue(ctx context.Context, element, name string) (string, error) {
	var value string
	err := d.asking(ctx, &value, "css", element, name)
	return value, err
}

func (d *Driver) ElementText(ctx context.Context, element string) (string, error) {
	var value string
	err := d.asking(ctx, &value, "text", element)
	return value, err
}

func (d *Driver) ElementTagName(ctx context.Context, element string) (string, error) {
	var value string
	err := d.asking(ctx, &value, "tag", element)
	return value, err
}

func (d *Driver) ElementRect(ctx context.Context, element string) (webdriver.Rect, error) {
	var value webdriver.Rect
	err := d.asking(ctx, &value, "rect", element)
	return value, err
}

func (d *Driver) ElementEnabled(ctx context.Context, element string) (bool, error) {
	var value bool
	err := d.asking(ctx, &value, "enabled", element)
	return value, err
}

func (d *Driver) ElementSelected(ctx context.Context, element string) (bool, error) {
	var value bool
	err := d.asking(ctx, &value, "selected", element)
	return value, err
}

func (d *Driver) ElementDisplayed(ctx context.Context, element string) (bool, error) {
	var value bool
	err := d.asking(ctx, &value, "displayed", element)
	return value, err
}

// Execute runs a script in the session's document and waits for what
// it returns, or for its callback: polled each frame, after the page's
// own scripts ran theirs. One run of a script without a pause is still
// stopped by the engine's own budget. A script that asks the user
// something is answered null at once, as the standard has it, and goes
// on once its prompt is answered.
func (d *Driver) Execute(ctx context.Context, run webdriver.Script) (json.RawMessage, error) {
	var args []json.RawMessage
	if err := json.Unmarshal(run.Args, &args); err != nil {
		return nil, webdriver.Errorf(webdriver.InvalidArgument, "args must be a list")
	}
	result := json.RawMessage("null")
	err := d.flowing(ctx, func(f *flow) {
		began := time.Now()
		d.reading(f, func(_ *Screen, live *running) error {
			started, err := d.ask(live.rt, "start", run.Source, args, run.Async)
			if errors.Is(err, errPrompted) {
				f.frames(1)
				return nil
			}
			if err != nil {
				return err
			}
			var id int
			if err = json.Unmarshal(started, &id); err != nil {
				return err
			}
			f.then(func() (bool, error) {
				value, failed := d.ask(live.rt, "poll", id)
				var state struct {
					Pending bool            `json:"pending"`
					Value   json.RawMessage `json:"value"`
				}
				switch {
				case errors.Is(failed, errPrompted):
					f.frames(1)
					return true, nil
				case failed != nil:
					return true, failed
				case json.Unmarshal(value, &state) != nil:
					return true, webdriver.Errorf(webdriver.UnknownError, "light's script answered "+string(value))
				case !state.Pending:
					if state.Value != nil {
						result = state.Value
					}
					return true, nil
				case !run.Endless && time.Since(began) > run.Timeout:
					_, _ = d.ask(live.rt, "drop", id)
					return true, webdriver.Errorf(webdriver.ScriptTimeout, fmt.Sprintf("the script did not end in %s",
						run.Timeout))
				}
				return false, nil
			})
			return nil
		})
	})
	return result, err
}

// SwitchToFrame makes a frame of the session's document its current
// one: its scripts and elements are those of the document it shows.
func (d *Driver) SwitchToFrame(ctx context.Context, to webdriver.Frame) error {
	if to.Top {
		return d.once(ctx, func() error {
			_, err := d.window()
			d.frame = nil
			return err
		})
	}
	return d.flowing(ctx, func(f *flow) {
		d.reading(f, func(s *Screen, live *running) error {
			index := to.Index
			if to.Element != "" {
				value, err := d.ask(live.rt, "frame", to.Element)
				if err != nil {
					return err
				}
				if err = json.Unmarshal(value, &index); err != nil {
					return err
				}
			}
			set := s.session().frames[s.shownWeb()]
			var within []*frame
			if set != nil {
				within = set.within(d.frame)
			}
			if index < 0 || index >= len(within) {
				return webdriver.Errorf(webdriver.NoSuchFrame, "the document has no such frame that shows a document")
			}
			d.frame = within[index]
			return nil
		})
	})
}

func (d *Driver) SwitchToParentFrame(ctx context.Context) error {
	return d.once(ctx, func() error {
		_, err := d.window()
		if err == nil && d.frame != nil {
			d.frame = d.frame.parent
		}
		return err
	})
}

// origin is where the session's document begins in its window, in
// points, and its zoom: the page's view, or the box of its current
// frame in it.
func (d *Driver) origin(s *Screen) (geom.Vec2, float32, error) {
	page := s.shownWeb()
	if page == nil {
		return geom.Vec2{}, 1, unscriptedPage("the window shows no page of the web")
	}
	host, _ := s.doc.BoxOf(s.host)
	at, zoom := geom.V2(host.X, host.Y), page.Doc.Zoom()
	for f := d.frame; f != nil; f = f.parent {
		box, ok := f.in.Doc.BoxOf(f.node)
		if !ok {
			return at, zoom, webdriver.Errorf(webdriver.NoSuchWindow, "the session's frame is not shown")
		}
		at = at.Add(geom.V2(box.X, box.Y))
	}
	if zoom <= 0 {
		zoom = 1
	}
	return at, zoom, nil
}

// pageView is the page's view in its window, in points.
func (s *Screen) pageView() (geom.Rect, bool) {
	if s.host == nil {
		return geom.Rect{}, false
	}
	return s.doc.BoxOf(s.host)
}
