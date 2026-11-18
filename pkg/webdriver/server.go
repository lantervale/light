package webdriver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
)

// maxBody is how much of a command's body is read.
const maxBody = 64 << 20

// Server answers WebDriver commands over HTTP for one Browser, one
// session at a time, a command at a time.
type Server struct {
	browser Browser
	about   About

	// running is held while a command runs; state guards the session.
	running sync.Mutex
	state   sync.Mutex
	session *session
}

// session is the one session: its id, and what its commands go by.
type session struct {
	id       string
	settings Settings
	timeouts Timeouts
}

// New is a Server driving browser, which says about of itself.
func New(browser Browser, about About) *Server { return &Server{browser: browser, about: about} }

// Listen listens at address for a Server: an address of this machine
// alone, as a remote control of the browser must not be reached from
// another. "localhost" is 127.0.0.1.
func Listen(ctx context.Context, address string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("webdriver: %q is no host:port: %w", address, err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("webdriver: %q is not an address of this machine alone: WebDriver drives the "+
			"browser, and is served on 127.0.0.1, ::1 or localhost only", host)
	}
	var config net.ListenConfig
	return config.Listen(ctx, "tcp", net.JoinHostPort(ip.String(), port))
}

// local reports whether a Host header names this machine: localhost or
// a loopback address, with or without a port.
func local(host string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// refused is why a request is not a WebDriver client's, "" for one that
// may be: a page of the web sends an Origin with what it posts, and a
// name it made turn to this machine is still in its Host.
func refused(r *http.Request) string {
	switch {
	case !local(r.Host):
		return fmt.Sprintf("the Host header %q does not name this machine", r.Host)
	case len(r.Header.Values("Origin")) > 0:
		return "requests with an Origin header, those a web page makes, are refused"
	}
	return ""
}

// call is one command: what its route named in its path, and the
// object it was sent.
type call struct {
	ctx     context.Context //nolint:containedctx // a command's, for as long as its request
	vars    map[string]string
	body    map[string]json.RawMessage
	raw     json.RawMessage
	session *session
}

// param is the string a command was sent under name.
func (c *call) param(name string) (string, *Error) {
	value, ok := text(c.body[name])
	if !ok {
		return "", Errorf(InvalidArgument, name+" must be a string")
	}
	return value, nil
}

func (c *call) load() Load {
	return Load{Strategy: c.session.settings.PageLoadStrategy, Timeout: c.session.timeouts.PageLoad}
}

// route is one command: its method, its path with a name in braces for
// each part that varies, and what it does.
type route struct {
	method, path string
	do           func(s *Server, c *call) (any, error)
}

// ServeHTTP answers a command.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if why := refused(r); why != "" {
		write(w, http.StatusForbidden, errorBody(&Error{Code: UnknownError, Message: "light's WebDriver: " + why}))
		return
	}
	value, err := s.answer(r)
	if err != nil {
		failed := failure(err)
		write(w, failed.Code.Status(), errorBody(failed))
		return
	}
	write(w, http.StatusOK, value)
}

func errorBody(e *Error) any {
	body := map[string]any{"error": e.Code, "message": e.Message, "stacktrace": e.Stack}
	if e.Data != nil {
		body["data"] = e.Data
	}
	return body
}

func write(w http.ResponseWriter, status int, value any) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{"value": value}); err != nil {
		status = http.StatusInternalServerError
		body.Reset()
		_ = encoder.Encode(map[string]any{"value": errorBody(&Error{Code: UnknownError, Message: err.Error()})})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	_, _ = w.Write(bytes.TrimSuffix(body.Bytes(), []byte("\n")))
}

// answer runs the command a request is, once those before it ended.
func (s *Server) answer(r *http.Request) (any, error) {
	found, vars, err := match(r.Method, r.URL.EscapedPath())
	if err != nil {
		return nil, err
	}
	c := &call{ctx: r.Context(), vars: vars}
	if r.Method == http.MethodPost {
		raw, failed := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if failed != nil {
			return nil, Errorf(InvalidArgument, "the body was not read: "+failed.Error())
		}
		var ok bool
		if c.body, ok = object(raw); !ok {
			return nil, Errorf(InvalidArgument, "the body of a POST must be a JSON object")
		}
		c.raw = raw
	}
	if id, within := vars["session"]; within {
		s.state.Lock()
		c.session = s.session
		s.state.Unlock()
		if c.session == nil || c.session.id != id {
			return nil, Errorf(InvalidSessionID, "no session "+id)
		}
	}
	s.running.Lock()
	defer s.running.Unlock()
	if c.session != nil && !s.current(c.session) {
		return nil, Errorf(InvalidSessionID, "the session "+c.session.id+" has ended")
	}
	if ended := c.ctx.Err(); ended != nil {
		return nil, ended
	}
	if c.session != nil && !unprompted(found.method, found.path) {
		if err = s.prompts(c); err != nil {
			return nil, err
		}
	}
	return found.do(s, c)
}

func (s *Server) current(held *session) bool {
	s.state.Lock()
	defer s.state.Unlock()
	return s.session == held
}

// match is the route of a method and a path, with what the path names:
// an unknown command for a path no route has, an unknown method for
// one that has it under other methods.
func match(method, path string) (route, map[string]string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	known := false
	for _, each := range routes() {
		pattern := strings.Split(each.path, "/")
		if len(pattern) != len(parts) {
			continue
		}
		vars, fits := map[string]string{}, true
		for i, part := range pattern {
			name, varies := strings.CutPrefix(part, "{")
			value, err := url.PathUnescape(parts[i])
			switch {
			case err != nil, !varies && part != parts[i], varies && value == "":
				fits = false
			case varies:
				vars[strings.TrimSuffix(name, "}")] = value
			}
		}
		if !fits {
			continue
		}
		if each.method == method {
			return each, vars, nil
		}
		known = true
	}
	if known {
		return route{}, nil, Errorf(UnknownMethod, method+" is no method of "+path)
	}
	return route{}, nil, Errorf(UnknownCommand, method+" "+path+" is no command of light's WebDriver")
}

// status says whether a session can be created.
func (s *Server) status(*call) (any, error) {
	s.state.Lock()
	defer s.state.Unlock()
	message := s.about.Name + " " + s.about.Version + " is ready for a session"
	if s.session != nil {
		message = s.about.Name + " is driven by a session already, and has one at a time"
	}
	return map[string]any{"ready": s.session == nil, "message": message}, nil
}

func (s *Server) create(c *call) (any, error) {
	if !s.current(nil) {
		return nil, Errorf(SessionNotCreated, "light is driven by a session already, and has one at a time")
	}
	settings, invalid := s.about.settle(c.body)
	if invalid != nil {
		return nil, invalid
	}
	if err := s.browser.NewSession(c.ctx, settings); err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	made := &session{id: hex.EncodeToString(id), settings: settings, timeouts: settings.Timeouts}
	s.state.Lock()
	s.session = made
	s.state.Unlock()
	return map[string]any{"sessionId": made.id, "capabilities": s.about.capabilities(settings)}, nil
}

// end ends the session, whatever the browser says of it.
func (s *Server) end(c *call) (any, error) {
	err := s.browser.EndSession(context.WithoutCancel(c.ctx))
	s.state.Lock()
	s.session = nil
	s.state.Unlock()
	return nil, err
}

// closeWindow closes the current window, and the session with the last.
func (s *Server) closeWindow(c *call) (any, error) {
	left, err := s.browser.CloseWindow(c.ctx)
	if err != nil {
		return nil, err
	}
	if len(left) == 0 {
		if _, err = s.end(c); err != nil {
			return nil, err
		}
		return []string{}, nil
	}
	return left, nil
}

func (s *Server) setTimeouts(c *call) (any, error) {
	changed, invalid := c.session.timeouts.with(c.raw)
	if invalid != nil {
		return nil, invalid
	}
	c.session.timeouts = changed
	return nil, nil //nolint:nilnil // a command that answers null
}

// find answers the elements a locator finds, from the element the path
// names when it names one.
func (s *Server) find(c *call) ([]any, error) {
	using, invalid := c.param("using")
	if invalid != nil {
		return nil, invalid
	}
	switch using {
	case ByCSS, ByLinkText, ByPartialLinkText, ByTagName, ByXPath:
	default:
		return nil, Errorf(InvalidArgument, fmt.Sprintf("%q is no location strategy", using))
	}
	value, invalid := c.param("value")
	if invalid != nil {
		return nil, invalid
	}
	ids, err := s.browser.FindElements(c.ctx, Locator{Using: using, Value: value, From: c.vars["element"]},
		c.session.timeouts.Implicit)
	if err != nil {
		return nil, err
	}
	found := make([]any, 0, len(ids))
	for _, id := range ids {
		found = append(found, element(id))
	}
	return found, nil
}

func (s *Server) findOne(c *call) (any, error) {
	found, err := s.find(c)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, Errorf(NoSuchElement, fmt.Sprintf("no element is found by %s %q", c.body["using"],
			c.body["value"]))
	}
	return found[0], nil
}

func element(id string) map[string]string { return map[string]string{ElementKey: id} }

func (s *Server) switchFrame(c *call) (any, error) {
	id, given := c.body["id"]
	to := Frame{Top: given && isNull(id)}
	index, whole := integer(id)
	name, refers := reference(id, ElementKey)
	switch {
	case to.Top:
	case whole && index <= 0xffff:
		to.Index = int(index)
	case refers:
		to.Element = name
	default:
		return nil, Errorf(InvalidArgument, "the id of a frame must be null, a number from 0 to 65535 or an element")
	}
	return nil, s.browser.SwitchToFrame(c.ctx, to)
}

func (s *Server) newWindow(c *call) (any, error) {
	kind := "tab"
	if hint, given := c.body["type"]; given && !isNull(hint) {
		var ok bool
		if kind, ok = text(hint); !ok {
			return nil, Errorf(InvalidArgument, "the type of a new window must be a string or null")
		}
		if kind != "window" {
			kind = "tab"
		}
	}
	handle, opened, err := s.browser.NewWindow(c.ctx, kind)
	if err != nil {
		return nil, err
	}
	return map[string]string{"handle": handle, "type": opened}, nil
}

func (s *Server) setRect(c *call) (any, error) {
	var to Placement
	for name, into := range map[string]**float64{"x": &to.X, "y": &to.Y, "width": &to.Width, "height": &to.Height} {
		raw, given := c.body[name]
		if !given || isNull(raw) {
			continue
		}
		value, ok := number(raw)
		lowest := float64(-1 << 31)
		if name == "width" || name == "height" {
			lowest = 0
		}
		if !ok || value < lowest || value > 1<<31-1 {
			return nil, Errorf(InvalidArgument, name+" is not a number a window's rect holds")
		}
		*into = &value
	}
	return s.browser.SetWindowRect(c.ctx, to)
}

func (s *Server) execute(c *call, async bool) (any, error) {
	source, invalid := c.param("script")
	if invalid != nil {
		return nil, invalid
	}
	var args []json.RawMessage
	if isNull(c.body["args"]) || json.Unmarshal(c.body["args"], &args) != nil {
		return nil, Errorf(InvalidArgument, "args must be a list")
	}
	t := c.session.timeouts
	return s.browser.Execute(c.ctx, Script{Source: source, Args: c.body["args"], Async: async, Timeout: t.Script,
		Endless: t.ScriptEndless})
}

func (s *Server) cookie(c *call) (any, error) {
	all, err := s.browser.Cookies(c.ctx)
	if err != nil {
		return nil, err
	}
	for _, each := range all {
		if each.Name == c.vars["name"] {
			return each, nil
		}
	}
	return nil, Errorf(NoSuchCookie, "the document has no cookie named "+c.vars["name"])
}

func (s *Server) addCookie(c *call) (any, error) {
	fields, ok := object(c.body["cookie"])
	if !ok {
		return nil, Errorf(InvalidArgument, "cookie must be an object")
	}
	var cookie Cookie
	name, named := text(fields["name"])
	value, valued := text(fields["value"])
	if !named || !valued {
		return nil, Errorf(InvalidArgument, "a cookie must have a name and a value that are strings")
	}
	cookie.Name, cookie.Value = name, value
	for field, into := range map[string]any{"path": &cookie.Path, "domain": &cookie.Domain, "secure": &cookie.Secure,
		"httpOnly": &cookie.HTTPOnly, "sameSite": &cookie.SameSite} {
		raw, given := fields[field]
		if given && !isNull(raw) && json.Unmarshal(raw, into) != nil {
			return nil, Errorf(InvalidArgument, "the "+field+" of the cookie is not what a cookie holds")
		}
	}
	if raw, given := fields["expiry"]; given && !isNull(raw) {
		expiry, whole := integer(raw)
		if !whole {
			return nil, Errorf(InvalidArgument, "the expiry of a cookie must be a whole number of seconds")
		}
		cookie.Expiry = &expiry
	}
	switch cookie.SameSite {
	case "", "Lax", "Strict", "None":
	default:
		return nil, Errorf(InvalidArgument, `the sameSite of a cookie must be "Lax", "Strict" or "None"`)
	}
	return nil, s.browser.AddCookie(c.ctx, cookie)
}

func (s *Server) perform(c *call) (any, error) {
	list, invalid := sources(c.body["actions"])
	if invalid != nil {
		return nil, invalid
	}
	return nil, s.browser.PerformActions(c.ctx, list)
}

// null is a command that answers nothing.
func null(do func(s *Server, c *call) error) func(*Server, *call) (any, error) {
	return func(s *Server, c *call) (any, error) { return nil, do(s, c) }
}

// said is a command that sends a string named name.
func said(name string, do func(s *Server, c *call, value string) error) func(*Server, *call) (any, error) {
	return func(s *Server, c *call) (any, error) {
		value, invalid := c.param(name)
		if invalid != nil {
			return nil, invalid
		}
		return nil, do(s, c, value)
	}
}
