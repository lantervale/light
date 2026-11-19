//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package script

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v8 "github.com/gost-dom/v8go"
)

// Engine names the engine of this build.
func Engine() string { return "V8 " + strings.TrimSuffix(v8.Version(), "-v8go") }

// Available reports whether this build runs scripts.
func Available() bool { return true }

// live counts the engines made and not yet closed, and contexts the
// Contexts: more than the engines when some share one.
//
//nolint:gochecknoglobals // counts of the process's engines, for its tests and its tools
var live, contexts atomic.Int64

// Live is how many engines are open: one for each Context but those
// made beside another.
func Live() int { return int(live.Load()) }

// Contexts is how many Contexts are open.
func Contexts() int { return int(contexts.Load()) }

// initialHeap is the heap a context starts with.
const initialHeap = 8 << 20

// errorsSource makes an Error of a message, of the kind named when it
// is one of the engine's.
const errorsSource = `(({ TypeError, SyntaxError, RangeError, ReferenceError }) => (message, kind) =>
	new ({ TypeError, SyntaxError, RangeError, ReferenceError }[kind] || Error)(message))(globalThis)`

// isolate is a V8 isolate and what is its own, whichever of its
// contexts runs: one script runs in it at a time, under one budget.
type isolate struct {
	iso *v8.Isolate
	// kin is who shares it, and contexts those that do.
	kin      string
	contexts map[*v8.Context]*context
	// depth counts the runs and calls in progress: the budget is the
	// outermost's.
	depth int
	// watchdog stops the outermost run once its budget is spent, at
	// due (Pause).
	watchdog *time.Timer
	due      time.Time
	// late is told of a run that outlasts its budget in place of
	// stopping it (Options.Late), ran how long it was told the run has
	// taken, wait how long the watchdog was last set for, and paused
	// whether the run's clock is stopped: all under watch.
	late      func(time.Duration) time.Duration
	ran, wait time.Duration
	paused    bool
	// watch guards expired, which the watchdog sets as it interrupts,
	// and gone, set once the isolate is disposed of.
	watch         sync.Mutex
	expired, gone bool
	stopped       atomic.Bool
}

// context is a V8 context in an isolate of its own, or in that of its
// kin: a page shares nothing with another but with those, and closing
// the last of an isolate frees all of it.
type context struct {
	opts Options
	*isolate
	ctx *v8.Context
	// closed is set as the context closes, for Interrupt.
	closed atomic.Bool
	// fence stands between the scripts of a context that has kin and
	// the functions light gives it, and shut closes it (fenceSource).
	fence, shut *v8.Function
	// rejected are the promises rejected with no handler since the
	// outermost run began.
	rejected []rejection
	// answered is what the last bound function gave back, the engine's
	// own by the time of the next call.
	answered *v8.Value
	// errors makes an Error of a message, of the kind named.
	errors *v8.Function
	modules
}

// forget releases the last answer of a bound function.
func (c *context) forget() {
	if c.answered != nil && c.answered != v8.Null(c.iso) && c.answered != v8.Undefined(c.iso) {
		c.answered.Release()
	}
	c.answered = nil
}

type rejection struct {
	promise *v8.Promise
	err     *Error
}

// New makes a Context with nothing bound.
func New(opts Options) (Context, error) {
	shared, err := opts.isolate()
	if err != nil {
		return nil, err
	}
	c := &context{opts: opts, isolate: shared, ctx: v8.NewContext(shared.iso)}
	shared.contexts[c.ctx] = c
	contexts.Add(1)
	made, err := c.ctx.RunScript(errorsSource, "light:error")
	if err != nil {
		c.Close()
		return nil, err
	}
	c.errors, err = made.AsFunction()
	if err == nil {
		err = c.fencing()
	}
	if err == nil {
		err = c.importing()
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// fenceSource makes the fence of a context that has kin. What a
// context holds outlives it in the scripts of its kin, and a function
// of light's that is called once its context closed takes the process
// down; so the scripts never hold such a function, only one that calls
// it until the context closes, and throws from then on.
const fenceSource = `(() => {
	let gone = false;
	return [(fn) => function (...args) {
		if (gone) throw new Error("light: the document this was of is gone");
		return fn.apply(this, args);
	}, () => { gone = true; }];
})()`

// fencing makes the fence of a context that has kin.
func (c *context) fencing() error {
	if c.opts.Kin == "" {
		return nil
	}
	made, err := c.ctx.RunScript(fenceSource, "light:fence")
	if err != nil {
		return err
	}
	defer made.Release()
	pair, err := made.AsObject()
	if err != nil {
		return err
	}
	for i, kept := range []**v8.Function{&c.fence, &c.shut} {
		part, failed := pair.GetIdx(uint32(i))
		if failed != nil {
			return failed
		}
		if *kept, failed = part.AsFunction(); failed != nil {
			return failed
		}
	}
	return nil
}

// fenced is a function of light's as the scripts of the context hold
// it: behind the fence, where the context has one.
func (c *context) fenced(fn *v8.Function) (v8.Valuer, error) {
	if c.fence == nil {
		return fn, nil
	}
	return c.fence.Call(v8.Undefined(c.iso), fn)
}

// isolate is the isolate of a new Context: that of the one it is made
// beside, when they are kin, else a new one.
func (o Options) isolate() (*isolate, error) {
	if o.Beside != nil {
		beside, is := o.Beside.(*context)
		switch {
		case !is || o.Kin == "" || beside.kin != o.Kin:
			return nil, ErrNotKin
		case beside.ctx == nil:
			return nil, ErrClosed
		}
		return beside.isolate, nil
	}
	shared := &isolate{kin: o.Kin, contexts: map[*v8.Context]*context{},
		iso: v8.NewIsolate(v8.WithResourceConstraints(initialHeap, o.memory()), v8.WithExceptionMessages())}
	live.Add(1)
	shared.iso.SetPromiseRejectedCallback(func(m v8.PromiseRejectMessage) {
		if of := shared.contexts[m.Context]; of != nil {
			of.promiseRejected(m)
		}
	})
	return shared, nil
}

// exception is err as an Error of the page's own realm, which a script
// catches as it does any other.
func (c *context) exception(err error) error {
	message, failed := v8.NewValue(c.iso, err.Error())
	if failed != nil {
		return err
	}
	defer message.Release()
	thrown, failed := c.errors.Call(v8.Undefined(c.iso), message)
	if failed != nil {
		return err
	}
	c.forget()
	c.answered = thrown
	return &v8.Exception{Value: thrown}
}

func (c *context) Bind(name string, fn Func) error {
	if c.ctx == nil {
		return ErrClosed
	}
	tmpl := v8.NewFunctionTemplateWithError(c.iso, func(info *v8.FunctionCallbackInfo) (*v8.Value, error) {
		defer info.Release()
		c.forget()
		args := make([]any, len(info.Args()))
		for i, arg := range info.Args() {
			switch {
			case arg.IsFunction():
			case arg.IsSharedArrayBuffer():
				if shared, release, failed := arg.SharedArrayBufferGetContents(); failed == nil {
					args[i] = shared
					defer release()
				}
			default:
				args[i] = c.plain(arg)
			}
		}
		result, err := fn(args)
		if err != nil {
			return nil, c.exception(err)
		}
		value, err := c.value(result)
		c.answered = value
		return value, err
	})
	bound, err := c.fenced(tmpl.GetFunction(c.ctx))
	if err != nil {
		return err
	}
	return c.ctx.Global().Set(name, bound)
}

func (c *context) Run(source, origin string) (any, error) {
	if c.ctx == nil {
		return nil, ErrClosed
	}
	source = c.calls(source, origin)
	return c.guarded(func() (*v8.Value, error) { return c.ctx.RunScript(source, origin) })
}

func (c *context) Interrupt() {
	if !c.closed.Load() && !c.stopped.Load() {
		c.iso.TerminateExecution()
	}
}

// Close frees the context, and its isolate with the last of those
// that share it.
func (c *context) Close() {
	if c.ctx == nil {
		return
	}
	c.closed.Store(true)
	last := len(c.contexts) == 1
	if last {
		c.stopped.Store(true)
		c.watch.Lock()
		defer c.watch.Unlock()
		c.iso.SetPromiseRejectedCallback(nil)
	}
	if c.shut != nil && !last {
		_, _ = c.shut.Call(v8.Undefined(c.iso))
	}
	c.close()
	delete(c.contexts, c.ctx)
	c.ctx.Close()
	c.ctx = nil
	contexts.Add(-1)
	if last {
		c.iso.Dispose()
		c.gone = true
		live.Add(-1)
	}
}

// guarded runs one entry into the engine under the budget, and gives
// its value as a plain one.
func (c *context) guarded(enter func() (*v8.Value, error)) (any, error) {
	var value *v8.Value
	err := c.within(func() error {
		var failed error
		value, failed = enter()
		return failed
	})
	if err != nil {
		if value != nil {
			value.Release()
		}
		return nil, err
	}
	plain := c.plain(value)
	if _, kept := plain.(Function); !kept {
		value.Release()
	}
	return plain, nil
}

// within runs one entry into the engine under the budget: the
// outermost's, which also tells of the rejections nobody handled.
func (c *context) within(enter func() error) error {
	c.depth++
	outermost := c.depth == 1
	var watchdog *time.Timer
	if outermost {
		c.watch.Lock()
		c.late, c.ran, c.wait, c.paused = c.opts.Late, 0, c.opts.budget(), false
		watchdog = time.AfterFunc(c.wait, c.expire)
		c.watchdog, c.due = watchdog, time.Now().Add(c.wait)
		c.watch.Unlock()
	}
	err := enter()
	c.depth--
	if outermost {
		c.watch.Lock()
		watchdog.Stop()
		c.late = nil
		c.watch.Unlock()
		for _, of := range c.contexts {
			of.forget()
		}
		defer c.isolate.report()
	}
	if err != nil {
		return c.failure(err, outermost)
	}
	if outermost && c.settle() {
		return ErrInterrupted
	}
	return nil
}

func (c *context) Pause() func() {
	c.watch.Lock()
	defer c.watch.Unlock()
	if c.depth == 0 || c.watchdog == nil || c.paused {
		return func() {}
	}
	c.watchdog.Stop()
	c.paused = true
	left := time.Until(c.due)
	return func() {
		c.watch.Lock()
		defer c.watch.Unlock()
		c.paused = false
		c.due = time.Now().Add(left)
		c.watchdog.Reset(left)
	}
}

// expire is the watchdog: the budget is spent. The run is stopped,
// unless its lateness is told of instead; a clock that fired as it was
// being paused fires again once it goes on.
func (c *context) expire() {
	c.watch.Lock()
	defer c.watch.Unlock()
	switch {
	case c.gone || c.paused:
	case c.late == nil:
		c.expired = true
		c.iso.TerminateExecution()
	default:
		c.ran += c.wait
		if c.wait = c.late(c.ran); c.wait > 0 {
			c.due = time.Now().Add(c.wait)
			c.watchdog.Reset(c.wait)
		}
	}
}

// spent reports whether the watchdog fired, and forgets it.
func (c *context) spent() bool {
	c.watch.Lock()
	defer c.watch.Unlock()
	expired := c.expired
	c.expired = false
	return expired
}

// settle reports an interruption the engine did not: one that stopped
// the tasks a script queued, or came as the script was returning. It
// takes it, so the next run is not the one stopped.
func (c *context) settle() bool {
	if !c.spent() {
		return false
	}
	if value, err := c.ctx.RunScript("0", "light:settle"); err == nil {
		value.Release()
	}
	return true
}

// failure is err as the package says it.
func (c *context) failure(err error, outermost bool) error {
	var thrown *v8.JSError
	if !errors.As(err, &thrown) {
		return err
	}
	if errors.Is(err, v8.ErrHeapLimitReached) {
		c.spent()
		return ErrOutOfMemory
	}
	if message := thrown.ExceptionMessage(); message != nil {
		return &Error{Message: strings.TrimPrefix(message.Text, "Uncaught "), Source: message.ScriptResourceName,
			Line: message.LineNumber, Column: message.StartColumn + 1, Stack: thrown.StackTrace}
	}
	if thrown.Location == "" && (c.spent() || !outermost || thrown.Message == "" ||
		strings.Contains(thrown.Message, "terminated")) {
		return ErrInterrupted
	}
	source, line, column := located(thrown.Location)
	return &Error{Message: thrown.Message, Source: source, Line: line, Column: column, Stack: thrown.StackTrace}
}

// located reads "origin:line:column".
func located(location string) (string, int, int) {
	parts := strings.Split(location, ":")
	if len(parts) < 3 {
		return location, 0, 0
	}
	line, _ := strconv.Atoi(parts[len(parts)-2])
	column, _ := strconv.Atoi(parts[len(parts)-1])
	return strings.Join(parts[:len(parts)-2], ":"), line, column
}

// promiseRejected keeps a promise rejected with no handler until the
// script returns, and forgets it when a handler came after all.
func (c *context) promiseRejected(m v8.PromiseRejectMessage) {
	switch m.Event {
	case v8.PromiseRejectWithNoHandler:
		c.rejected = append(c.rejected, rejection{m.Promise, c.thrown(m.Value)})
	case v8.PromiseHandlerAddedAfterReject:
		for i, r := range c.rejected {
			if r.promise.SameValue(m.Promise.Value) {
				r.promise.Release()
				c.rejected = append(c.rejected[:i], c.rejected[i+1:]...)
				break
			}
		}
		m.Promise.Release()
	}
}

// thrown is the Error of a value a script threw or rejected with.
func (c *context) thrown(value *v8.Value) *Error {
	if value == nil {
		return &Error{Message: "undefined"}
	}
	defer value.Release()
	e := &Error{Message: value.DetailString()}
	if object, err := value.AsObject(); err == nil && value.IsNativeError() {
		if stack, failed := object.Get("stack"); failed == nil {
			e.Stack = stack.String()
			stack.Release()
		}
	}
	return e
}

// report tells of the rejections nobody handled, in each context of
// the isolate: a script of one may reject a promise of another.
func (i *isolate) report() {
	for _, of := range i.contexts {
		of.report()
	}
}

// report tells of the rejections of the context nobody handled.
func (c *context) report() {
	rejected := c.rejected
	c.rejected = nil
	for _, r := range rejected {
		r.promise.Release()
		if c.opts.Rejected != nil {
			if r.err.Message == unsupported {
				r.err.Message = unreached
			}
			r.err.Message = "Uncaught (in promise) " + r.err.Message
			c.opts.Rejected(r.err)
		}
	}
}

// plain is a value of a script as a Go one.
func (c *context) plain(v *v8.Value) any {
	switch {
	case v == nil || v.IsNullOrUndefined():
		return nil
	case v.IsBoolean():
		return v.Boolean()
	case v.IsNumber():
		return v.Number()
	case v.IsString():
		return v.String()
	case v.IsFunction():
		return c.function(v)
	case v.IsObject():
		return c.decoded(v)
	}
	return v.String()
}

// decoded is an object or an array through JSON, nil for one JSON has
// no word for.
func (c *context) decoded(v *v8.Value) any {
	text, err := v8.JSONStringify(c.ctx, v)
	if err != nil {
		return nil
	}
	var plain any
	if json.Unmarshal([]byte(text), &plain) != nil {
		return nil
	}
	return plain
}

// value is a Go value as a script's.
func (c *context) value(plain any) (*v8.Value, error) {
	switch p := plain.(type) {
	case nil:
		return v8.Null(c.iso), nil
	case bool, string, float64, int32, int64, uint32, uint64:
		return v8.NewValue(c.iso, p)
	case int:
		return v8.NewValue(c.iso, float64(p))
	case float32:
		return v8.NewValue(c.iso, float64(p))
	case *function:
		return p.fn.Value, nil
	}
	text, err := json.Marshal(plain)
	if err != nil {
		return nil, err
	}
	return v8.JSONParse(c.ctx, string(text))
}

// function is a script's function kept by Go.
type function struct {
	c  *context
	fn *v8.Function
}

// function keeps v, a function, which is then the Function's to release.
func (c *context) function(v *v8.Value) Function {
	fn, err := v.AsFunction()
	if err != nil {
		return nil
	}
	return &function{c: c, fn: fn}
}

func (f *function) Call(args ...any) (any, error) {
	if f.c.ctx == nil {
		return nil, ErrClosed
	}
	values := make([]v8.Valuer, len(args))
	for i, arg := range args {
		value, err := f.c.value(arg)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return f.c.guarded(func() (*v8.Value, error) { return f.fn.Call(v8.Undefined(f.c.iso), values...) })
}

func (f *function) Release() {
	if f.c.ctx != nil {
		f.fn.Release()
	}
}

func (o Options) budget() time.Duration {
	if o.Budget > 0 {
		return o.Budget
	}
	return DefaultBudget
}

func (o Options) memory() uint64 {
	if o.Memory > 0 {
		return o.Memory
	}
	return DefaultMemory
}
