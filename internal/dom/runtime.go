// Package dom is the web's API as a page's scripts find it, bound onto
// the page's tree (web.Tree): window, document, elements, events,
// timers and requests. The API itself is written in JavaScript (js/),
// over one function of light's that does what only light can: read and
// change the tree, tell the time, fetch, navigate. A Runtime is one page's:
// it runs the page's scripts, and each frame what they left to run.
package dom

import (
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/script"
)

//go:embed js/*.js
var sources embed.FS

// origin names light's own script in errors and stacks.
const origin = "light:dom"

const (
	// frameBudget is how long a frame's timers may run before the rest
	// wait for the next.
	frameBudget = 8 * time.Millisecond
	// sweepEvery is how often the nodes scripts let go of are forgotten.
	sweepEvery = 2 * time.Second
	// maxNodes is how many nodes a page's scripts may make.
	maxNodes = 500_000
)

// Message is one line of the console: what a script printed, or an
// exception nothing caught.
type Message struct {
	// Level is log, info, warn, error or debug.
	Level, Text string
	// Source is the address of the script it came from and Line where,
	// "" and 0 when unknown.
	Source string
	Line   int
	// Stack is the calls that led to it, a line each, "" when it has
	// none to show, and Depth how many groups it was said in.
	Stack string
	Depth int
	At    time.Time
}

// Request is what a script asks to be fetched.
type Request struct {
	Method, Address string
	Header          map[string]string
	// Script is a script element's source, and Sheet a link's
	// stylesheet, where the rest is a fetch.
	Script bool
	Sheet  bool
	// Source is the address of the script that asked through fetch or
	// XMLHttpRequest and Line where in it, "" and 0 when unknown.
	Source string
	Line   int
}

// Response is what came of a Request: Err is why nothing did.
type Response struct {
	Status     int
	StatusText string
	// Address is where the answer came from, after its redirects.
	Address string
	Header  map[string]string
	Text    string
	Err     error
}

// Storage is one area of a site's storage.
type Storage interface {
	Get(key string) (string, bool)
	Set(key, value string)
	Remove(key string)
	Clear()
	Keys() []string
}

// Cookies are the browser's cookies as a page's scripts see them:
// what document.cookie reads, the cookie it is given, and whether any
// is kept.
type Cookies interface {
	Get() string
	Set(cookie string)
	Enabled() bool
}

// Host is what a Runtime asks of the browser around the page. Every
// function is optional: without one, what scripts ask of it does
// nothing.
type Host struct {
	// Console is told each line of the console.
	Console func(Message)
	// Cleared is told the console was cleared by a script.
	Cleared func()
	// Navigate opens address in the page's tab, in place of the page in
	// its history when replace is set, and Reload fetches the page again.
	Navigate func(address string, replace bool)
	Reload   func()
	// Moved says the page is at another address without having left:
	// how is "push" or "replace". Travel goes that many steps through
	// the tab's history, and Steps is how many it holds.
	Moved  func(how, address string)
	Travel func(delta int)
	Steps  func() int
	// Fetch asks for a request off the frame; done may be called from
	// any goroutine, once. What it returns stops the request.
	Fetch func(r Request, done func(Response)) (cancel func())
	// Refused is told a request of a script that light does not send.
	Refused func(r Request)
	// Storage is an area of the page's site: "local" or "session".
	Storage func(area string) Storage
	// Cookies are the page's cookies; without them, they last as long
	// as the page.
	Cookies Cookies
	// Viewport is the size of the page's view.
	Viewport func() (width, height float32)
	// Titled is told the title a script gave the page.
	Titled func(title string)
	// Agent is what navigator.userAgent says.
	Agent string
	// Budget is how long one script may run, script.DefaultBudget when 0.
	Budget time.Duration
}

// Runtime is one page's scripts and what they see of the page.
type Runtime struct {
	ctx   script.Context
	entry script.Function
	host  Host
	page  *web.Page
	doc   *ui.Document
	tree  *web.Tree
	ops   map[string]op
	began time.Time

	handles
	// selectors are the selectors read so far.
	selectors map[string]selectors
	// ran are the script elements that ran or are about to.
	ran map[*html.Node]bool
	// waiting are the scripts to run at the next frame.
	waiting []func()
	// landed is what ended off the frame, for the next one to run.
	landed struct {
		sync.Mutex
		list []func()
	}
	// requests are the fetches on their way, by the number scripts know
	// each by.
	requests map[int]func()
	// cookies are what document.cookie reads and sets (jar), and stores
	// the storage areas the browser has none for.
	cookies Cookies
	stores  map[string]Storage
	// sheets are the page's stylesheets as scripts change them
	// (sheets.go).
	sheets sheets
	// fonts is what scripts were last told of the page's fonts (fonts.go).
	fonts fontWatch
	// focused is the element that had the focus at the last frame.
	focused *ui.Node
	swept   time.Time
	// activation is the click a script was told of last (events.go).
	activation activation
	closed     bool
}

// New makes the Runtime of page, loaded with its scripts (web.Loader's
// Scripts), in a new context of the engine. Nothing runs until Start.
func New(page *web.Page, host Host) (*Runtime, error) {
	if page.Doc == nil || page.Tree == nil {
		return nil, errors.New("the page was not loaded for scripts to run")
	}
	r := &Runtime{
		host: host, page: page, doc: page.Doc, tree: page.Tree, began: time.Now(),
		handles: newHandles(), selectors: map[string]selectors{}, stores: map[string]Storage{},
		ran: map[*html.Node]bool{}, requests: map[int]func(){},
	}
	r.ops = r.table()
	r.sheets = newSheets()
	r.sheetOps()
	r.fontOps()
	r.seedSheets()
	ctx, err := script.New(script.Options{Budget: host.Budget, Rejected: r.rejected})
	if err != nil {
		return nil, err
	}
	r.ctx = ctx
	if err = r.install(); err != nil {
		ctx.Close()
		return nil, err
	}
	page.Tree.Activate, page.Tree.Heard, page.Tree.Submit = r.activate, r.heard, r.submit
	return r, nil
}

// install runs light's own script, which gives the page its globals.
func (r *Runtime) install() error {
	if err := r.ctx.Bind("__light", r.call); err != nil {
		return err
	}
	files, err := sources.ReadDir("js")
	if err != nil {
		return err
	}
	var source strings.Builder
	source.WriteString("(function () {\n\"use strict\";\n")
	for _, file := range files {
		text, failed := sources.ReadFile("js/" + file.Name())
		if failed != nil {
			return failed
		}
		source.Write(text)
		source.WriteByte('\n')
	}
	source.WriteString("})()")
	entry, err := r.ctx.Run(source.String(), origin)
	if err != nil {
		return fmt.Errorf("light's own script: %w", err)
	}
	fn, ok := entry.(script.Function)
	if !ok {
		return errors.New("light's own script gave no function")
	}
	r.entry = fn
	return nil
}

// call is the one function scripts reach light by: an operation by its
// name, and its arguments.
func (r *Runtime) call(args []any) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("light: no operation named")
	}
	name, _ := args[0].(string)
	do, known := r.ops[name]
	if !known {
		return nil, fmt.Errorf("light: no operation %q", name)
	}
	r.sheets.stale = r.sheets.stale || mutates[name]
	return do(arguments(args[1:]))
}

// Start runs the page's scripts in their order, then tells the page it
// is ready and loaded.
func (r *Runtime) Start() {
	for _, s := range r.page.Scripts {
		if s.Node != nil {
			r.ran[s.Node] = true
		}
		if s.Skipped != "" {
			r.say("warn", fmt.Sprintf("A script was not run: %s.", s.Skipped), s.Address)
			continue
		}
		r.run(s.Source, s.Address, s.Node)
	}
	r.enter("ready", "interactive")
	r.enter("ready", "complete")
	r.syncSheets()
}

// run runs one script as its element's.
func (r *Runtime) run(source, address string, element *html.Node) {
	if r.closed {
		return
	}
	r.enter("script", r.ref(element))
	_, err := r.ctx.Run(source, address)
	r.failed(err)
	r.enter("script", 0)
}

// enter calls light's own script, and reports what it throws.
func (r *Runtime) enter(call string, args ...any) any {
	if r.closed {
		return nil
	}
	value, err := r.entry.Call(append([]any{call}, args...)...)
	r.failed(err)
	return value
}

// failed tells the console of how a script ended, when it ended badly.
func (r *Runtime) failed(err error) {
	var thrown *script.Error
	switch {
	case err == nil || errors.Is(err, script.ErrClosed):
	case errors.As(err, &thrown):
		message := "Uncaught " + thrown.Message
		if taken, _ := r.quietly("failed", message, thrown.Source, thrown.Line, thrown.Column); taken != true {
			r.fault(message, thrown.Source, thrown.Line, thrown.Stack)
		}
	default:
		r.say("error", err.Error(), "")
	}
}

// quietly calls light's own script without reporting what it throws.
func (r *Runtime) quietly(call string, args ...any) (any, error) {
	if r.closed {
		return nil, script.ErrClosed
	}
	return r.entry.Call(append([]any{call}, args...)...)
}

// rejected tells the console of a promise rejected with no handler.
func (r *Runtime) rejected(err *script.Error) {
	source, line, _ := where(err.Stack)
	r.fault(err.Message, source, line, err.Stack)
}

func (r *Runtime) say(level, text, source string) {
	r.tell(Message{Level: level, Text: text, Source: source})
}

// fault tells the console of an exception nothing caught, with the
// calls that led to it when stack says more than where it is from.
func (r *Runtime) fault(text, source string, line int, stack string) {
	r.tell(Message{Level: "error", Text: text, Source: source, Line: line, Stack: nested(stack)})
}

// tell tells the console of a line, said now.
func (r *Runtime) tell(m Message) {
	if r.host.Console != nil {
		m.At = time.Now()
		r.host.Console(m)
	}
}

// Frame runs what the page's scripts left for this frame: the scripts
// they added, the answers that landed, the timers that are due and the
// frame callbacks. It reports whether anything is left to run later.
func (r *Runtime) Frame() bool {
	if r.closed {
		return false
	}
	r.activation = activation{}
	waiting := r.waiting
	r.waiting = nil
	for _, run := range waiting {
		run()
	}
	for _, land := range r.take() {
		land()
	}
	r.watchFocus()
	r.watchTitle()
	left, _ := r.enter("tick", r.now(), float64(frameBudget.Milliseconds())).(float64)
	if time.Since(r.swept) > sweepEvery {
		r.swept = time.Now()
		r.enter("sweep")
	}
	r.syncSheets()
	r.watchFonts()
	r.tree.Redraw()
	return r.fonts.waited || left > 0 || len(r.waiting) > 0 || len(r.requests) > 0 || r.sheets.loading > 0
}

// take is what landed since the last frame.
func (r *Runtime) take() []func() {
	r.landed.Lock()
	defer r.landed.Unlock()
	list := r.landed.list
	r.landed.list = nil
	return list
}

// land leaves fn for the next frame, from any goroutine.
func (r *Runtime) land(fn func()) {
	r.landed.Lock()
	defer r.landed.Unlock()
	r.landed.list = append(r.landed.list, fn)
}

// Evaluate runs source as the console's line does, and gives its value
// as the console prints it.
func (r *Runtime) Evaluate(source string) (string, error) {
	value, err := r.quietly("evaluate", source)
	var thrown *script.Error
	if errors.As(err, &thrown) {
		return "", errors.New("Uncaught " + thrown.Message)
	}
	if err != nil {
		return "", err
	}
	text, _ := value.(string)
	return text, nil
}

// Arrived tells the page its tab is back at an address it pushed.
func (r *Runtime) Arrived(address string) { r.enter("arrived", address) }

// Nodes is how many nodes of the page scripts hold a number for.
func (r *Runtime) Nodes() int { return len(r.nodes) }

// Sweep forgets now the nodes scripts let go of.
func (r *Runtime) Sweep() { r.enter("sweep") }

// Close stops the page's scripts for good and frees their context.
func (r *Runtime) Close() {
	if r.closed {
		return
	}
	r.closed = true
	for _, cancel := range r.requests {
		cancel()
	}
	r.tree.Activate, r.tree.Heard, r.tree.Submit = nil, nil, nil
	r.entry.Release()
	r.ctx.Close()
	r.handles = newHandles()
	r.ran, r.requests, r.waiting = nil, nil, nil
}

// Closed reports whether the Runtime was closed.
func (r *Runtime) Closed() bool { return r.closed }

func (r *Runtime) now() float64 { return float64(time.Since(r.began).Microseconds()) / 1000 }

// where reads the first place of a stack that is not light's own: its
// script's address, line and column.
func where(stack string) (string, int, int) {
	for _, line := range strings.Split(stack, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "at ") || strings.Contains(line, origin) {
			continue
		}
		place := strings.TrimPrefix(line, "at ")
		if open := strings.LastIndex(place, "("); open >= 0 && strings.HasSuffix(place, ")") {
			place = place[open+1 : len(place)-1]
		}
		parts := strings.Split(place, ":")
		if len(parts) < 3 {
			continue
		}
		row, column := number(parts[len(parts)-2]), number(parts[len(parts)-1])
		if row == 0 {
			continue
		}
		return strings.Join(parts[:len(parts)-2], ":"), row, column
	}
	return "", 0, 0
}

// calls are the calls of a stack that are not light's own, a line
// each, the innermost first.
func calls(stack string) string {
	var kept []string
	for _, line := range strings.Split(stack, "\n") {
		call, found := strings.CutPrefix(strings.TrimSpace(line), "at ")
		if found && !strings.Contains(call, origin) && !strings.Contains(call, "<anonymous>") {
			kept = append(kept, call)
		}
	}
	return strings.Join(kept, "\n")
}

// nested are the calls of the stack of an exception, when it says more
// than where the exception is from.
func nested(stack string) string {
	if kept := calls(stack); strings.Contains(kept, "\n") {
		return kept
	}
	return ""
}

func number(text string) int {
	n := 0
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
