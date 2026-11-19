package dom

import (
	"reflect"

	"github.com/lantervale/ember/ui"
)

// framed is h with every call of it made on the frame's goroutine,
// whichever side asks: what the browser does for a page reaches its
// window, which is the frame's thread's alone. The functions a call
// returns are called there too.
func (r *Runtime) framed(h Host) Host {
	fields := reflect.ValueOf(&h).Elem()
	for i := range fields.NumField() {
		if f := fields.Field(i); f.Kind() == reflect.Func && !f.IsNil() {
			f.Set(r.framedFunc(reflect.ValueOf(f.Interface())))
		}
	}
	if h.Cookies != nil {
		h.Cookies = framedCookies{r, h.Cookies}
	}
	if h.Windows != nil {
		h.Windows = framedWindows{r, h.Windows}
	}
	return h
}

func (r *Runtime) framedFunc(fn reflect.Value) reflect.Value {
	return reflect.MakeFunc(fn.Type(), func(in []reflect.Value) []reflect.Value {
		var out []reflect.Value
		r.onFrame(func() {
			r.offScript()
			out = fn.Call(in)
		})
		for i, o := range out {
			switch held := o; {
			case o.Kind() == reflect.Func && !o.IsNil():
				out[i] = r.framedFunc(o)
			case o.Kind() == reflect.Interface && !o.IsNil():
				if store, is := held.Interface().(Storage); is {
					var framed Storage = framedStorage{r, store}
					out[i] = reflect.ValueOf(&framed).Elem()
				}
			}
		}
		return out
	})
}

type framedStorage struct {
	r *Runtime
	s Storage
}

func (f framedStorage) Get(key string) (string, bool) {
	var value string
	var has bool
	f.r.onFrame(func() { value, has = f.s.Get(key) })
	return value, has
}
func (f framedStorage) Set(key, value string) { f.r.onFrame(func() { f.s.Set(key, value) }) }
func (f framedStorage) Remove(key string)     { f.r.onFrame(func() { f.s.Remove(key) }) }
func (f framedStorage) Clear()                { f.r.onFrame(f.s.Clear) }
func (f framedStorage) Keys() []string {
	var keys []string
	f.r.onFrame(func() { keys = f.s.Keys() })
	return keys
}

type framedCookies struct {
	r *Runtime
	c Cookies
}

func (f framedCookies) Get() string {
	var cookies string
	f.r.onFrame(func() { cookies = f.c.Get() })
	return cookies
}
func (f framedCookies) Set(cookie string) { f.r.onFrame(func() { f.c.Set(cookie) }) }
func (f framedCookies) Enabled() bool {
	var enabled bool
	f.r.onFrame(func() { enabled = f.c.Enabled() })
	return enabled
}

type framedWindows struct {
	r *Runtime
	w Windows
}

// window is one number the windows answer with, asked on the frame.
func (f framedWindows) window(ask func() int) int {
	var window int
	f.r.onFrame(func() { window = ask() })
	return window
}

func (f framedWindows) Parent(of int) int { return f.window(func() int { return f.w.Parent(of) }) }
func (f framedWindows) Top(of int) int    { return f.window(func() int { return f.w.Top(of) }) }
func (f framedWindows) Length(of int) int { return f.window(func() int { return f.w.Length(of) }) }
func (f framedWindows) Child(of, index int) int {
	return f.window(func() int { return f.w.Child(of, index) })
}
func (f framedWindows) Named(of int, name string) int {
	return f.window(func() int { return f.w.Named(of, name) })
}
func (f framedWindows) Of(frame *ui.Node) int { return f.window(func() int { return f.w.Of(frame) }) }
func (f framedWindows) Element(of int) *ui.Node {
	var shown *ui.Node
	f.r.onFrame(func() { shown = f.w.Element(of) })
	return shown
}
func (f framedWindows) May(to int, address string, form bool) bool {
	var may bool
	f.r.onFrame(func() { may = f.w.May(to, address, form) })
	return may
}
func (f framedWindows) Kin(of int) bool {
	var kin bool
	f.r.onFrame(func() { kin = f.w.Kin(of) })
	return kin
}
func (f framedWindows) Closed(of int) bool {
	var closed bool
	f.r.onFrame(func() { closed = f.w.Closed(of) })
	return closed
}
func (f framedWindows) Post(to int, message Posted, target string) error {
	var err error
	f.r.onFrame(func() { err = f.w.Post(to, message, target) })
	return err
}
func (f framedWindows) Go(to int, address string, replace bool) {
	f.r.onFrame(func() { f.w.Go(to, address, replace) })
}
func (f framedWindows) Focus(to int) { f.r.onFrame(func() { f.w.Focus(to) }) }
func (f framedWindows) Pipe() (int, int, bool) {
	var one, other int
	var made bool
	f.r.onFrame(func() { one, other, made = f.w.Pipe() })
	return one, other, made
}
func (f framedWindows) Send(port int, message Posted) {
	f.r.onFrame(func() { f.w.Send(port, message) })
}
func (f framedWindows) Requeue(port int, message Posted) {
	f.r.onFrame(func() { f.w.Requeue(port, message) })
}
func (f framedWindows) Shut(port int) { f.r.onFrame(func() { f.w.Shut(port) }) }
