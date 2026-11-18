package dom

import (
	"bytes"
	"runtime"
	"strconv"
	"testing"

	"github.com/lantervale/light/pkg/script"
)

// proven is whether the hand-over between the frame and the script's
// side is proven as it is used: under test, where a call on the wrong
// goroutine panics.
var proven = testing.Testing() //nolint:gochecknoglobals // set once, for the tests of every package that runs scripts

// goroutine is the number of the goroutine that runs.
func goroutine() uint64 {
	var stack [64]byte
	fields := bytes.Fields(stack[:runtime.Stack(stack[:], false)])
	if len(fields) < 2 {
		return 0
	}
	id, _ := strconv.ParseUint(string(fields[1]), 10, 64)
	return id
}

// onScript checks that what enters the engine runs on the script's
// side.
func (r *Runtime) onScript() {
	if proven && goroutine() == r.asking.frame {
		panic("light: the engine of a page is entered from the frame while its script runs")
	}
}

// offScript checks that what the browser answers runs on the frame.
func (r *Runtime) offScript() {
	if p := &r.asking; proven && (p.script || p.entered && goroutine() != p.frame) {
		panic("light: the browser answers a script off the frame's goroutine")
	}
}

// gate is the engine of a Runtime as the Runtime uses it: every call
// that enters it goes through, so none does while a script waits for
// an answer, and all do on the script's side.
type gate struct {
	r       *Runtime
	ctx     script.Context
	modules map[script.Module]*gatedModule
}

// engine makes the Runtime's engine behind its gate.
func (r *Runtime) engine(o script.Options) (script.Context, error) {
	g := &gate{r: r, modules: map[script.Module]*gatedModule{}}
	if answer := o.Import; answer != nil {
		o.Import = func(from string, what script.Import, done func(script.Module, error)) {
			answer(from, what, func(m script.Module, err error) { done(bare(m), err) })
		}
	}
	ctx, err := script.New(o)
	if err != nil {
		return nil, err
	}
	g.ctx = ctx
	return g, nil
}

// runs runs touch, which runs a script, where one may run: not from
// outside an entry while a document of the tab waits for an answer,
// but for light's own script, which gives a new page its globals.
func (g *gate) runs(touch func()) bool {
	if p := &g.r.asking; g.r.entry != nil && !p.script && !p.entered && g.r.withheld() {
		if proven {
			panic("light: a script of a page is run while a document of its tab waits for an answer")
		}
		return false
	}
	return g.r.through(touch)
}

func (g *gate) Bind(name string, fn script.Func) error {
	err := ErrAsking
	g.r.through(func() { err = g.ctx.Bind(name, fn) })
	return err
}

func (g *gate) Run(source, origin string) (any, error) {
	var value any
	err := ErrAsking
	g.runs(func() { value, err = g.ctx.Run(source, origin) })
	if fn, kept := value.(script.Function); kept {
		value = &gatedFunction{g: g, fn: fn}
	}
	return value, err
}

func (g *gate) Module(source, address string) (script.Module, error) {
	var m script.Module
	err := ErrAsking
	g.r.through(func() { m, err = g.ctx.Module(source, address) })
	return g.module(m), err
}

func (g *gate) JSON(text, address string) (script.Module, error) {
	var m script.Module
	err := ErrAsking
	g.r.through(func() { m, err = g.ctx.JSON(text, address) })
	return g.module(m), err
}

// Interrupt stops the script that runs, from any goroutine.
func (g *gate) Interrupt() { g.ctx.Interrupt() }

func (g *gate) Pause() func() {
	if !g.r.asking.script {
		return func() {}
	}
	return g.ctx.Pause()
}

func (g *gate) Close() {
	g.r.unanswered()
	g.r.through(g.ctx.Close)
}

// module is m behind the gate, the same for the same module.
func (g *gate) module(m script.Module) script.Module {
	if m == nil {
		return nil
	}
	if g.modules[m] == nil {
		g.modules[m] = &gatedModule{g: g, m: m}
	}
	return g.modules[m]
}

// bare is the engine's own module of one behind a gate.
func bare(m script.Module) script.Module {
	if gated, is := m.(*gatedModule); is {
		return gated.m
	}
	return m
}

type gatedFunction struct {
	g  *gate
	fn script.Function
}

func (f *gatedFunction) Call(args ...any) (any, error) {
	for i, arg := range args {
		if gated, is := arg.(*gatedFunction); is {
			args[i] = gated.fn
		}
	}
	var value any
	err := ErrAsking
	f.g.runs(func() { value, err = f.fn.Call(args...) })
	if fn, kept := value.(script.Function); kept {
		value = &gatedFunction{g: f.g, fn: fn}
	}
	return value, err
}

// Release lets the function go, once the answer came if the page's
// scripts wait for one.
func (f *gatedFunction) Release() {
	if p := &f.g.r.asking; p.parked {
		p.queued = append(p.queued, f.fn.Release)
		return
	}
	f.g.r.through(f.fn.Release)
}

type gatedModule struct {
	g *gate
	m script.Module
}

func (m *gatedModule) Imports() []script.Import { return m.m.Imports() }

func (m *gatedModule) Link(resolve func(from script.Module, what script.Import) (script.Module, error)) error {
	err := ErrAsking
	m.g.r.through(func() {
		err = m.m.Link(func(from script.Module, what script.Import) (script.Module, error) {
			to, failed := resolve(m.g.module(from), what)
			return bare(to), failed
		})
	})
	return err
}

func (m *gatedModule) Run(done func(error)) {
	if !m.g.runs(func() { m.m.Run(done) }) {
		done(ErrAsking)
	}
}
