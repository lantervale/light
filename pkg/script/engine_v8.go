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

// live counts the Contexts made and not yet closed.
var live atomic.Int64 //nolint:gochecknoglobals // a count of the process's engines, for its tests and its tools

// Live is how many Contexts are open: each is an engine of its own.
func Live() int { return int(live.Load()) }

// initialHeap is the heap a context starts with.
const initialHeap = 8 << 20

// errorsSource makes an Error of a message, of the kind named when it
// is one of the engine's.
const errorsSource = `(({ TypeError, SyntaxError, RangeError, ReferenceError }) => (message, kind) =>
	new ({ TypeError, SyntaxError, RangeError, ReferenceError }[kind] || Error)(message))(globalThis)`

// context is a V8 isolate with one context: a page shares nothing with
// another, and closing it frees all of it.
type context struct {
	opts Options
	iso  *v8.Isolate
	ctx  *v8.Context
	// depth counts the runs and calls in progress: the budget is the
	// outermost's.
	depth int
	// watchdog stops the outermost run once its budget is spent, at
	// due (Pause).
	watchdog *time.Timer
	due      time.Time
	// watch guards expired, which the watchdog sets as it interrupts.
	watch   sync.Mutex
	expired bool
	stopped atomic.Bool
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
	iso := v8.NewIsolate(v8.WithResourceConstraints(initialHeap, opts.memory()), v8.WithExceptionMessages())
	c := &context{opts: opts, iso: iso, ctx: v8.NewContext(iso)}
	live.Add(1)
	iso.SetPromiseRejectedCallback(c.promiseRejected)
	made, err := c.ctx.RunScript(errorsSource, "light:error")
	if err != nil {
		c.Close()
		return nil, err
	}
	c.errors, err = made.AsFunction()
	if err == nil {
		err = c.importing()
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
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
	return c.ctx.Global().Set(name, tmpl.GetFunction(c.ctx))
}

func (c *context) Run(source, origin string) (any, error) {
	if c.ctx == nil {
		return nil, ErrClosed
	}
	source = c.calls(source, origin)
	return c.guarded(func() (*v8.Value, error) { return c.ctx.RunScript(source, origin) })
}

func (c *context) Interrupt() {
	if !c.stopped.Load() {
		c.iso.TerminateExecution()
	}
}

func (c *context) Close() {
	if c.ctx == nil {
		return
	}
	c.stopped.Store(true)
	c.watch.Lock()
	defer c.watch.Unlock()
	c.iso.SetPromiseRejectedCallback(nil)
	c.close()
	c.ctx.Close()
	c.iso.Dispose()
	c.ctx = nil
	live.Add(-1)
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
		watchdog = time.AfterFunc(c.opts.budget(), c.expire)
		c.watchdog, c.due = watchdog, time.Now().Add(c.opts.budget())
	}
	err := enter()
	c.depth--
	if outermost {
		watchdog.Stop()
		c.forget()
		defer c.report()
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
	if c.depth == 0 || c.watchdog == nil || !c.watchdog.Stop() {
		return func() {}
	}
	left := time.Until(c.due)
	return func() {
		c.due = time.Now().Add(left)
		c.watchdog.Reset(left)
	}
}

// expire is the watchdog: the budget is spent.
func (c *context) expire() {
	c.watch.Lock()
	defer c.watch.Unlock()
	if c.ctx != nil {
		c.expired = true
		c.iso.TerminateExecution()
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

// report tells of the rejections nobody handled.
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
