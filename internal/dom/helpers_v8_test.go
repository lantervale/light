//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
)

const address = "https://site.test/dir/page.html?q=1#top"

// walled refuses every request that would leave this machine: no test
// reaches the Internet.
type walled struct{}

func (walled) RoundTrip(r *http.Request) (*http.Response, error) {
	if host := r.URL.Hostname(); host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, errors.New(host + " is not reached from a test")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// scripted is a page of markup with its scripts running, and what they
// asked of the browser.
type scripted struct {
	t       *testing.T
	page    *web.Page
	rt      *dom.Runtime
	console []dom.Message
	opened  []string
	went    []string
	// posted are the forms sent with a POST, each as its address, its
	// type and what it carries, after "script" for one a script sent.
	posted  []string
	fetched []dom.Request
	// answers are what the page's fetches get, by address: one whose
	// status is pending never comes. stopped are the fetches the page
	// stopped, by address.
	answers map[string]dom.Response
	stopped []string
	// at is the page's address, address when it is "".
	at     string
	stores map[string]dom.Storage
}

func run(t *testing.T, markup string) *scripted {
	t.Helper()
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start(markup, dom.Host{})
	return s
}

func (s *scripted) start(markup string, host dom.Host) {
	s.t.Helper()
	loader := &web.Loader{Client: fetch.New(walled{}), Scripts: true}
	open := func(to string) { s.opened = append(s.opened, to) }
	post := func(by string) func(web.Submission) {
		return func(sent web.Submission) {
			s.posted = append(s.posted, by+sent.Address+" "+sent.Form.Type+" "+string(sent.Form.Body))
		}
	}
	at := s.at
	if at == "" {
		at = address
	}
	s.page = loader.Local(at, markup, web.Links{Open: open, Over: func(string) {}, Post: post("")})
	if s.page.Err != nil {
		s.t.Fatal(s.page.Err)
	}
	host.Console = func(m dom.Message) { s.console = append(s.console, m) }
	host.Navigate = func(to string, replace bool) {
		if replace {
			to = "replace " + to
		}
		s.went = append(s.went, to)
	}
	host.Post = post("script ")
	host.Moved = func(how, to string) { s.went = append(s.went, how+" "+to) }
	host.Viewport = func() (float32, float32) { return 800, 600 }
	host.Fetch = s.fetch
	if s.stores != nil {
		host.Storage = func(area string) dom.Storage { return s.stores[area] }
	}
	host.Navigator.Agent = "light/test"
	rt, err := dom.New(s.page, host)
	if err != nil {
		s.t.Fatal(err)
	}
	s.rt = rt
	s.t.Cleanup(rt.Close)
	s.layout()
	rt.Start()
	s.layout()
}

// pending is the status of an answer that never comes.
const pending = -1

func (s *scripted) fetch(q dom.Request, done func(dom.Response)) func() {
	s.fetched = append(s.fetched, q)
	answer, known := s.answers[q.Address]
	if answer.Status == pending {
		return func() { s.stopped = append(s.stopped, q.Address) }
	}
	if !known {
		answer = dom.Response{Status: 404, StatusText: "404 Not Found", Address: q.Address}
	}
	if answer.Address == "" {
		answer.Address = q.Address
	}
	go done(answer)
	return func() {}
}

// window shows the page in a window, and gives what runs a frame of it.
func (s *scripted) window() func() {
	win := platform.NewWindow(platform.Size(800, 600))
	win.Add(s.page.Doc)
	w := ember.New().AddEntities(win)
	frame := func() { w.RunFrames(1) }
	frame()
	return frame
}

func (s *scripted) layout() { s.page.Doc.Layout(geom.Vec2{X: 800, Y: 600}) }

// eval is the value of an expression in the page, as the console
// prints it, a string without its quotes.
func (s *scripted) eval(expression string) string {
	s.t.Helper()
	value, err := s.rt.Evaluate(expression)
	if err != nil {
		s.t.Fatalf("%s: %v", expression, err)
	}
	if text, quoted := strconv.Unquote(value); quoted == nil {
		return text
	}
	return value
}

// frames runs frames until held, for a second at most.
func (s *scripted) frames(what string, held func() bool) {
	s.t.Helper()
	for end := time.Now().Add(time.Second); !held(); {
		if time.Now().After(end) {
			s.t.Fatalf("%s never came; the console says %s", what, s.said())
		}
		s.rt.Frame()
		s.layout()
		time.Sleep(time.Millisecond)
	}
}

// said is the console, a line a message.
func (s *scripted) said() string {
	lines := make([]string, len(s.console))
	for i, m := range s.console {
		lines[i] = m.Level + ": " + m.Text
	}
	return strings.Join(lines, "\n")
}

func (s *scripted) byID(id string) *ui.Node {
	s.t.Helper()
	n := s.page.Doc.ByID(id)
	if n == nil {
		s.t.Fatalf("the page has no #%s", id)
	}
	return n
}

// quiet fails the test when the console has an error or a warning.
func (s *scripted) quiet() {
	s.t.Helper()
	for _, m := range s.console {
		if m.Level == "error" || m.Level == "warn" {
			s.t.Errorf("the console says %s: %s (%s:%d)", m.Level, m.Text, m.Source, m.Line)
		}
	}
}
