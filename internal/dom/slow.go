package dom

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/lantervale/light/pkg/script"
)

// ErrBusy is what a page says to what would run its scripts while one
// of them, late, still runs.
var ErrBusy = errors.New("a script of the page is still running")

// Patience is how long a browser that asks lets a script run before it
// acts: After, the frames go on though the script still runs; Notice,
// the user is told and asked (Host.Slow); Again, how much longer one
// who chose to wait is left alone. The zero values are a second, five
// and ten.
type Patience struct {
	After, Notice, Again time.Duration
}

func (p Patience) after() time.Duration  { return or(p.After, time.Second) }
func (p Patience) notice() time.Duration { return or(p.Notice, 5*time.Second) }
func (p Patience) again() time.Duration  { return or(p.Again, 10*time.Second) }

func or(d, otherwise time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return otherwise
}

// sliceOf is how long of a frame a late script's operations are given.
const sliceOf = 8 * time.Millisecond

// lateness is a script that outlasted the browser's patience and runs
// on while the frames do: the two then run at once, which nothing else
// of the hand-over lets them.
//
// What keeps them apart is the token: whoever runs Go code that
// touches the page's documents holds it. The script's side takes it
// for each operation the engine calls and gives it up to go back into
// the engine, where no script touches anything of light's. Until a
// script is late the frame waits and never asks for it. Once it is,
// the frame holds it for the whole frame, the window's layout and
// painting included, but for a slice of each inside Frame, where the
// script's operations run and its calls of the browser are answered.
type lateness struct {
	token chan struct{}
	// tardy is told, by the engine's clock, that the run under way
	// outlasted After, and ran how long it has taken.
	tardy chan struct{}
	ran   atomic.Int64
	// late is whether the frames go on while the script runs, and
	// answering whether the frame runs a call of the script's side.
	late, answering bool
	// engaged counts the calls into the engine under way: the run ends
	// with the outermost.
	engaged int
	// told is whether the user was told of the run under way, and
	// askAt how long it must have taken to tell them.
	told  bool
	askAt time.Duration
}

// take and give are the script's side taking the token for the Go code
// it runs, and giving it up.
func (p *handover) take() {
	p.token <- struct{}{}
	p.script = true
}

func (p *handover) give() {
	p.script = false
	<-p.token
}

// inEngine runs raw, a call into the engine, with the token given up
// for as long as it lasts: what the engine calls back takes it.
func (r *Runtime) inEngine(raw func()) {
	p := r.asking.handover
	if !p.script || !p.entered {
		raw()
		return
	}
	p.engaged++
	p.give()
	raw()
	p.take()
	if p.engaged--; p.engaged == 0 {
		p.ran.Store(0)
		select {
		case <-p.tardy:
		default:
		}
		p.told, p.askAt = false, r.host.Patience.notice()
	}
}

// inGo runs do, Go code the engine calls, with the token.
func (r *Runtime) inGo(do func()) {
	p := r.asking.handover
	if p.script || !p.entered {
		do()
		return
	}
	p.take()
	defer p.give()
	do()
}

// tardiness is what the engine tells of a run that outlasted After,
// from a goroutine of its own: the frame hears of it where it waits.
func (r *Runtime) tardiness(ran time.Duration) time.Duration {
	p := r.asking.handover
	p.ran.Store(int64(ran))
	select {
	case p.tardy <- struct{}{}:
	default:
	}
	return min(max(r.host.Patience.after()/4, 5*time.Millisecond), 250*time.Millisecond)
}

// served runs on the frame what the script's side handed it, and gives
// it back its turn.
func (r *Runtime) served(do func()) {
	p := r.asking.handover
	was := p.answering
	p.answering = true
	do()
	p.answering = was
	p.script = true
	p.toScript <- nil
}

// belated is the frame taking the token of a script that is late, and
// going on: the console is told once.
func (r *Runtime) belated() {
	p := r.asking.handover
	p.late = true
	run := p.running
	if run == nil {
		run = r
	}
	if h := run.host.Hold; h != nil {
		h.busy = run
	}
	run.say("warn", "A script of this page has run for "+time.Duration(p.ran.Load()).Round(100*time.Millisecond).String()+
		" and still runs: the window goes on without it.", run.page.Address)
}

// slice gives the late script of r a part of the frame: its
// operations run and its calls of the browser are answered, then the
// frame takes the token back. The user is told of a script that runs
// longer than they were to be left alone.
func (r *Runtime) slice() {
	p := r.asking.handover
	if !p.late || p.running != r || p.answering {
		return
	}
	if ran := time.Duration(p.ran.Load()); !p.told && ran >= p.askAt && r.host.Slow != nil {
		p.told = true
		r.host.Slow(ran)
	}
	<-p.token
	timer := time.NewTimer(sliceOf)
	defer timer.Stop()
	for given := true; given; {
		select {
		case do := <-p.toFrame:
			if r.handed(do) {
				return
			}
		case <-timer.C:
			given = false
		}
	}
	for {
		select {
		case p.token <- struct{}{}:
			if !p.leaving() {
				return
			}
			<-p.token
			r.handed(<-p.toFrame)
			return
		case do := <-p.toFrame:
			if r.handed(do) {
				return
			}
		}
	}
}

// leaving reports whether the script's side gave the token up for
// good, or for the time of a question: what it hands the frame next
// says so.
func (p *handover) leaving() bool { return p.parked || !p.entered }

// handed is the frame, the token given up, doing what the late
// script's side hands it; it reports whether that was its end, or a
// question it now waits the answer of.
func (r *Runtime) handed(do func()) bool {
	if do != nil {
		r.served(do)
		return false
	}
	r.over()
	return true
}

// over is the late script ended, or waiting for an answer: what
// waited for it runs.
func (r *Runtime) over() {
	p := r.asking.handover
	p.late = false
	run := p.running
	if run == nil {
		run = r
	}
	if h := run.host.Hold; h != nil && h.busy == run {
		h.busy = nil
	}
	if failed := p.failed; failed != "" {
		p.failed = ""
		panic(failed)
	}
	if !p.parked {
		run.left()
		run.replay()
	}
}

// halt ends the late script of r as its page closes: it is stopped,
// and the frame waits for its side to end, the console hearing nothing
// of how.
func (r *Runtime) halt() {
	p := r.asking.handover
	if !p.late || p.running != r || p.answering {
		return
	}
	closed := r.closed
	r.closed = true
	r.ctx.Interrupt()
	r.host.Console = nil
	<-p.token
	for ended := false; !ended; {
		ended = r.handed(<-p.toFrame)
	}
	r.closed = closed
}

// patient is o for a browser that asks: a run that outlasts After is
// told of, not stopped.
func (r *Runtime) patient(o script.Options) script.Options {
	if r.host.Slow != nil && r.host.Ask != nil {
		o.Budget, o.Late = r.host.Patience.after(), r.tardiness
	}
	return o
}

// refusal is why the page's scripts are not entered now.
func (r *Runtime) refusal() error {
	if r.asking.late || r.host.Hold != nil && r.host.Hold.busy != nil {
		return ErrBusy
	}
	return ErrAsking
}

// Slow reports whether a script of the page runs on while the frames
// do, then whether the user was told of it and has not answered.
func (r *Runtime) Slow() (bool, bool) {
	p := r.asking.handover
	if !p.late || p.running != r {
		return false, false
	}
	return true, p.told
}

// Wait leaves the page's slow script alone for as long again as
// Patience says, and Stop ends it: the page stays, and what it runs
// next runs.
func (r *Runtime) Wait() {
	if p := r.asking.handover; p.late && p.running == r {
		p.told, p.askAt = false, time.Duration(p.ran.Load())+r.host.Patience.again()
	}
}

func (r *Runtime) Stop() {
	if p := r.asking.handover; p.late && p.running == r {
		p.told = false
		r.ctx.Interrupt()
	}
}
