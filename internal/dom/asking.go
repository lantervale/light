package dom

import (
	"errors"
	"fmt"
	"runtime/debug"
)

// The kinds of Question.
const (
	Alert   = "alert"
	Confirm = "confirm"
	Prompt  = "prompt"
)

// Question is what a script asks the user and waits the answer of:
// alert, confirm or prompt, with its message and, for a prompt, the
// text its field starts with.
type Question struct {
	Kind, Message, Default string
}

// Answer is what the user answered: OK or not, and the text of a
// prompt's field.
type Answer struct {
	OK   bool
	Text string
}

// ErrAsking is what a page whose scripts wait for an answer says to
// what would run them meanwhile.
var ErrAsking = errors.New("the page waits for the answer to a question it asked")

// errUnanswered ends a script whose page was closed while it asked.
var errUnanswered = errors.New("the page was closed before its question was answered")

// Hold is what the documents of one tab share: while a script of one
// waits for an answer, no script of another runs.
type Hold struct {
	by *Runtime
}

// Asking is the Runtime whose script waits for an answer, nil for none.
func (h *Hold) Asking() *Runtime { return h.by }

// parking is how a script waits for an answer while the frames go on.
// A script asks from inside the engine, deep under the call of light
// that runs it, and that call cannot return while it waits; so each
// entry into the page's scripts runs on a goroutine of its own, the
// script's side, the frame waiting for it to end or to ask. One of the
// two runs at a time, never both, and each hands the other what is its
// to run: the script's side what the browser answers (the calls of
// Host, on the frame's goroutine, which is the window's thread), the
// frame's what enters the engine while a script waits for such a call.
type parking struct {
	// entered is whether an entry runs, script whether its side has
	// the turn, and parked whether it waits for an answer: the engine
	// is then held, and nothing else enters it.
	entered, script, parked bool
	// calls counts the calls of the script's side the frame is
	// answering: a script that asks under one cannot wait.
	calls int
	// toFrame and toScript hand the turn over with what to run, nil to
	// go on; answer gives the script that waits its answer, nil to end
	// it; failed is what the entry panicked with.
	toFrame, toScript chan func()
	answer            chan *Answer
	failed            string
	// queued is what waits for the answer to run.
	queued    []func()
	replaying bool
	// frame is the frame's goroutine, kept under test.
	frame uint64
}

// withheld reports whether the page's scripts must not be entered: one of
// them, or of another document of its tab, waits for an answer.
func (r *Runtime) withheld() bool {
	return r.asking.parked || r.host.Hold != nil && r.host.Hold.by != nil
}

// free runs fn as an entry into the page's scripts and reports whether
// it did: not while they wait for an answer. It returns when fn ended
// or asked.
func (r *Runtime) free(fn func()) bool {
	p := &r.asking
	if p.script {
		fn()
		return true
	}
	if r.withheld() {
		return false
	}
	if r.replay(); r.withheld() {
		return false
	}
	return r.through(fn)
}

// replay runs what waited for an answer, in its order.
func (r *Runtime) replay() {
	p := &r.asking
	if p.replaying {
		return
	}
	p.replaying = true
	for len(p.queued) > 0 && !r.withheld() && !r.closed {
		next := p.queued[0]
		p.queued = p.queued[1:]
		r.through(next)
	}
	p.replaying = false
}

// whenFree runs fn as an entry, once the answer came if one is waited
// for.
func (r *Runtime) whenFree(fn func()) {
	if !r.free(fn) {
		r.asking.queued = append(r.asking.queued, fn)
	}
}

// through runs touch, which enters the engine, where it may be
// entered, and reports whether it did: on the script's side, which is
// a goroutine of its own wherever a browser may show a question.
func (r *Runtime) through(touch func()) bool {
	p := &r.asking
	switch {
	case p.script:
		r.onScript()
		touch()
	case p.parked:
		if proven {
			panic("light: the engine of a page is entered while its script waits for an answer")
		}
		return false
	case p.entered:
		p.script = true
		p.toScript <- touch
		r.serve()
	case r.host.Ask == nil:
		touch()
	default:
		r.enterScript(touch)
	}
	return true
}

// enterScript runs touch on a new script's side, until it ends or asks.
func (r *Runtime) enterScript(touch func()) {
	p := &r.asking
	if p.toFrame == nil {
		p.toFrame, p.toScript, p.answer = make(chan func()), make(chan func()), make(chan *Answer)
	}
	if proven {
		p.frame = goroutine()
	}
	p.entered, p.script = true, true
	go func() {
		defer func() {
			if thrown := recover(); thrown != nil {
				p.failed = fmt.Sprintf("%v\n\n%s", thrown, debug.Stack())
			}
			p.entered, p.script = false, false
			p.toFrame <- nil
		}()
		touch()
	}()
	r.serve()
}

// serve gives the script's side its turn, running on the frame what it
// hands over, until it ends, asks, or is done with what it was handed.
func (r *Runtime) serve() {
	p := &r.asking
	for {
		do := <-p.toFrame
		if do == nil {
			if failed := p.failed; failed != "" {
				p.failed = ""
				panic(failed)
			}
			return
		}
		do()
		p.script = true
		p.toScript <- nil
	}
}

// onFrame runs do on the frame's goroutine: at once there, handed over
// from the script's side, which waits for it and meanwhile runs what
// the frame hands back.
func (r *Runtime) onFrame(do func()) {
	p := &r.asking
	if !p.script {
		do()
		return
	}
	p.script = false
	p.calls++
	p.toFrame <- do
	for back := <-p.toScript; back != nil; back = <-p.toScript {
		back()
		p.script = false
		p.toFrame <- nil
	}
	p.calls--
}

// ask is alert, confirm and prompt: the browser is asked to show the
// question, and the script waits there for its answer, its clock
// stopped. Nothing comes back from a question that is not shown; with
// no browser to show it, the console is told of it.
func (r *Runtime) ask(a arguments) (any, error) {
	p := &r.asking
	q := Question{Kind: a.text(0), Message: a.text(1), Default: a.text(2)}
	switch {
	case r.host.Ask == nil:
		r.say("info", unasked(q), "")
		return nothing()
	case p.calls > 0:
		r.say("error", "Ignored call to '"+q.Kind+"()'. It was made while light answered another call of the page.", "")
		return nothing()
	case r.closed || !p.script || !r.host.Ask(q):
		return nothing()
	}
	resume := r.ctx.Pause()
	p.parked = true
	if r.host.Hold != nil {
		r.host.Hold.by = r
	}
	p.script = false
	p.toFrame <- nil
	answer := <-p.answer
	resume()
	if answer == nil {
		return nil, errUnanswered
	}
	return []any{answer.OK, answer.Text}, nil
}

// unasked is what the console is told of a question no browser is
// there to show.
func unasked(q Question) string {
	switch q.Kind {
	case Confirm:
		return "confirm, answered no: " + q.Message
	case Prompt:
		return "prompt, left unanswered: " + q.Message
	}
	return "alert: " + q.Message
}

// Asking reports whether the page's scripts wait for an answer.
func (r *Runtime) Asking() bool { return r.asking.parked }

// resumed gives the script that waits what it waits for, and the turn.
func (r *Runtime) resumed(a *Answer) {
	p := &r.asking
	p.parked = false
	if r.host.Hold != nil && r.host.Hold.by == r {
		r.host.Hold.by = nil
	}
	p.script = true
	p.answer <- a
	r.serve()
}

// Answer gives the page's scripts the answer they wait for: they go
// on until they end or ask again, then what waited for them runs.
func (r *Runtime) Answer(a Answer) {
	if !r.asking.parked || r.closed {
		return
	}
	r.resumed(&a)
	r.replay()
}

// unanswered ends the script that waits for an answer, as the page
// closes: it is stopped where it asked, and the console hears nothing
// of how it ends.
func (r *Runtime) unanswered() {
	if !r.asking.parked {
		return
	}
	r.ctx.Interrupt()
	r.host.Console = nil
	r.asking.queued = nil
	r.resumed(nil)
}
