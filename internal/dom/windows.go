package dom

import (
	"errors"
	"fmt"
	"math"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/frames"
)

const (
	// OwnWindow names the window of the page itself among those Windows
	// answers with, and NoWindow none.
	OwnWindow = 0
	NoWindow  = -1
	// maxMessage is how long a message one window posts to another may
	// be once written down.
	maxMessage = 32 << 20
	// windowLength is what a script asks for how many frames a window
	// has, and opaqueOrigin the origin of a document nothing shares its
	// origin with.
	windowLength = "length"
	opaqueOrigin = "null"
	// timeStep is how fine a time scripts are told is, in milliseconds:
	// what browsers tell a page that is not isolated from other origins.
	timeStep = 0.1
	// maxBroadcasts is how many messages of BroadcastChannels may wait
	// for a page that is not shown.
	maxBroadcasts = 1024
)

// ErrTargetOrigin is a targetOrigin that is no origin.
var ErrTargetOrigin = errors.New("the target origin is none")

// Windows are the other windows of the page's tab, as far as a script
// may know them: the frames of its page, those around it, and theirs.
// A window is a number that means nothing to another page; what a
// script passes as one is whatever it likes, so each answer is one any
// page of the tab may have. Nothing of a window's page is ever given.
type Windows interface {
	// Parent and Top are the windows around of, itself for the tab's.
	Parent(of int) int
	Top(of int) int
	// Length is how many frames of's document shows, Child the one at
	// that index and Named the one of that name, NoWindow for none.
	Length(of int) int
	Child(of, index int) int
	Named(of int, name string) int
	// Closed reports whether of's frame is no more.
	Closed(of int) bool
	// Of is the window of an iframe of the page, NoWindow when it shows
	// none.
	Of(frame *ui.Node) int
	// Post leaves message, a value as its sender wrote it down, for the
	// page of to, if its origin is the one target names when it comes:
	// ErrTargetOrigin for a target that names none.
	Post(to int, message Posted, target string) error
	// Go sends to to address, in place of its page in its history when
	// replace is set, if the page that asks may.
	Go(to int, address string, replace bool)
	// Focus gives to the keyboard, if the page that asks may.
	Focus(to int)
	// Pipe makes a channel between two documents of the tab: its two
	// ends, both the page's until a message takes one away, false when
	// the tab holds too many. Send posts a message through an end the
	// page holds to whoever holds the other, Requeue puts back in an end
	// the page is giving away a message it got and never read, and Shut
	// closes an end.
	Pipe() (int, int, bool)
	Send(port int, message Posted)
	Requeue(port int, message Posted)
	Shut(port int)
}

// Posted is a message as its sender wrote it down: a text, the bytes
// of what it holds that is no text, and the ends of the channels that
// leave with it, which its sender must hold. Nothing else of a page
// goes from one document to another.
type Posted struct {
	Text  string
	Bytes []byte
	Ports []int
}

// Sandboxed is what the page's frame keeps its document from.
type Sandboxed struct {
	// Opaque is a document whose origin nothing shares: it reads no
	// cookie and keeps none, and has no storage.
	Opaque bool
	// NoModals is one that may not ask the user anything.
	NoModals bool
}

// alone is the windows of a page that is in no tab.
type alone struct{}

func (alone) Parent(int) int                 { return OwnWindow }
func (alone) Top(int) int                    { return OwnWindow }
func (alone) Length(int) int                 { return 0 }
func (alone) Child(int, int) int             { return NoWindow }
func (alone) Named(int, string) int          { return NoWindow }
func (alone) Closed(int) bool                { return false }
func (alone) Of(*ui.Node) int                { return NoWindow }
func (alone) Post(int, Posted, string) error { return nil }
func (alone) Focus(int)                      {}
func (alone) Pipe() (int, int, bool)         { return 0, 0, false }
func (alone) Send(int, Posted)               {}
func (alone) Requeue(int, Posted)            {}
func (alone) Shut(int)                       {}
func (alone) Go(int, string, bool)           {}

func (r *Runtime) windows() Windows {
	if r.host.Windows != nil {
		return r.host.Windows
	}
	return alone{}
}

// origin is the origin of the page's document: what the browser holds
// it to be, else that of its address.
func (r *Runtime) origin() string {
	if r.host.Sandboxed.Opaque {
		return opaqueOrigin
	}
	if r.host.Origin != nil {
		return r.host.Origin()
	}
	if at := r.located(); at != nil {
		return originOf(at)
	}
	return opaqueOrigin
}

// window answers what a script asks of another window, by the name of
// what it asks: only what the table of the facade is made of.
func (r *Runtime) window(a arguments) (any, error) {
	w, of := r.windows(), whole(a.number(1))
	switch a.text(0) {
	case "table":
		marks := []string{}
		for _, kind := range frames.Carries() {
			marks = append(marks, kind.Mark)
		}
		return map[string]any{"window": shapes(frames.WindowMembers()), "location": shapes(frames.LocationMembers()),
			"quiet": frames.Quiet(), "carries": marks}, nil
	case "parent":
		return w.Parent(of), nil
	case "top":
		return w.Top(of), nil
	case windowLength:
		return w.Length(of), nil
	case "child":
		return w.Child(of, whole(a.number(2))), nil
	case "named":
		return w.Named(of, a.text(2)), nil
	case "closed":
		return w.Closed(of), nil
	case "of":
		if n, err := r.node(a, 1); err == nil && n != nil {
			if shown := r.shown(n); shown != nil {
				return w.Of(shown), nil
			}
		}
		return NoWindow, nil
	case "aims":
		if _, is := frames.Reaches(a.text(1), opaqueOrigin, opaqueOrigin); !is {
			//nolint:revive,staticcheck // a DOMException's message, as Chrome words it
			return nil, fmt.Errorf("SyntaxError: Invalid target origin '%s' in a call to 'postMessage'.", a.text(1))
		}
	case "post":
		m, err := r.sending(a, 3)
		if err != nil {
			return nil, err
		}
		_ = w.Post(of, m, a.text(2))
	case "keyboard":
		w.Focus(of)
	case "bytes":
		into, _ := a[1].([]byte)
		return copy(into, r.posted), nil
	case "go":
		w.Go(of, a.text(2), a.flag(3))
	}
	return nothing()
}

// whole is a number a script gave as a window or an index: one that is
// none is no window.
func whole(n float64) int {
	if math.IsNaN(n) || n < 0 || n > math.MaxInt32 || n != math.Trunc(n) {
		return NoWindow
	}
	return int(n)
}

func shapes(members []frames.Member) map[string]string {
	out := map[string]string{}
	for _, m := range members {
		out[m.Name] = string(m.Shape)
	}
	return out
}

// sending reads a message from what a script gave: its text, the bytes
// it shares and the ends that leave with it, from the argument at. One
// too long is refused, and the ends it took closed.
func (r *Runtime) sending(a arguments, at int) (Posted, error) {
	m := read(a, at)
	if len(m.Text)+len(m.Bytes) > maxMessage {
		for _, end := range m.Ports {
			r.windows().Shut(end)
		}
		return Posted{}, fmt.Errorf("DataCloneError: light posts at most %d bytes to another document at once", maxMessage)
	}
	return m, nil
}

func read(a arguments, at int) Posted {
	m := Posted{Text: a.text(at)}
	if len(a) > at+1 {
		if shared, _ := a[at+1].([]byte); len(shared) > 0 {
			m.Bytes = append([]byte(nil), shared...)
		}
	}
	if len(a) > at+2 {
		list, _ := a[at+2].([]any)
		for _, end := range list {
			if n, is := end.(float64); is && whole(n) > 0 {
				m.Ports = append(m.Ports, whole(n))
			}
		}
	}
	return m
}

// port answers what a script asks of the channels that reach other
// documents.
func (r *Runtime) port(a arguments) (any, error) {
	w, end := r.windows(), whole(a.number(1))
	switch a.text(0) {
	case "pipe":
		if one, other, made := w.Pipe(); made {
			return []int{one, other}, nil
		}
		return nil, errors.New("DataCloneError: light holds no more channels between the frames of this tab")
	case "send", "requeue":
		m, err := r.sending(a, 2)
		if err != nil {
			return nil, err
		}
		if a.text(0) == "send" {
			w.Send(end, m)
		} else {
			w.Requeue(end, m)
		}
	case "shut":
		w.Shut(end)
	}
	return nothing()
}

// broadcast gives a message of one of the page's BroadcastChannels to
// the other documents of its origin.
func (r *Runtime) broadcast(a arguments) (any, error) {
	m, err := r.sending(a, 1)
	if err == nil && r.host.Broadcast != nil && !r.host.Sandboxed.Opaque {
		r.host.Broadcast(a.text(0), Posted{Text: m.Text, Bytes: m.Bytes})
	}
	return nil, err
}

// given tells the page of a message by call, with what comes before the
// message itself: its bytes wait for the page to take them.
func (r *Runtime) given(call string, m Posted, before ...any) {
	r.whenFree(func() { r.giving(call, m, before...) })
}

func (r *Runtime) giving(call string, m Posted, before ...any) {
	ports := make([]any, len(m.Ports))
	for i, end := range m.Ports {
		ports[i] = end
	}
	r.posted = m.Bytes
	r.enter(call, append(before, m.Text, ports, len(m.Bytes))...)
	r.posted = nil
}

// Message tells the page of a message another window posted: what it
// wrote down, the origin of its document and the window it is, as the
// browser knows them.
func (r *Runtime) Message(m Posted, origin string, source int) { r.given("message", m, origin, source) }

// PortMessage tells the page of a message that came through an end it
// holds, and PortClosed that the other end of one was closed.
func (r *Runtime) PortMessage(port int, m Posted) { r.given("port", m, port) }
func (r *Runtime) PortClosed(port int)            { r.whenFree(func() { r.enter("portClosed", port) }) }

// Broadcast tells the page, at its next frame, of a message a
// BroadcastChannel of that name got from another document of its
// origin. It is safe from any goroutine.
func (r *Runtime) Broadcast(name, origin string, m Posted) {
	if r.airing.Add(1) > maxBroadcasts {
		r.airing.Add(-1)
		return
	}
	r.land(func() {
		r.airing.Add(-1)
		r.given("broadcast", Posted{Text: m.Text, Bytes: m.Bytes}, name, origin)
	})
}

// Loaded reports whether the page's window was told it loaded.
func (r *Runtime) Loaded() bool { return r.state == complete }
