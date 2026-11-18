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
	Post(to int, message, target string) error
	// Go sends to to address, in place of its page in its history when
	// replace is set, if the page that asks may.
	Go(to int, address string, replace bool)
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
func (alone) Post(int, string, string) error { return nil }
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
		return map[string]any{"window": shapes(frames.WindowMembers()), "location": shapes(frames.LocationMembers()),
			"quiet": frames.Quiet()}, nil
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
	case "post":
		message := a.text(2)
		if len(message) > maxMessage {
			return nil, fmt.Errorf("DataCloneError: light posts at most %d bytes to another window at once", maxMessage)
		}
		if errors.Is(w.Post(of, message, a.text(3)), ErrTargetOrigin) {
			//nolint:revive,staticcheck // a DOMException's message, as Chrome words it
			return nil, fmt.Errorf("SyntaxError: Invalid target origin '%s' in a call to 'postMessage'.", a.text(3))
		}
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

// Message tells the page of a message another window posted: what it
// wrote down, the origin of its document and the window it is, as the
// browser knows them.
func (r *Runtime) Message(message, origin string, source int) {
	r.enter("message", message, origin, source)
}
