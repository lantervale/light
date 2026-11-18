package dom

import (
	"errors"
	"slices"

	"github.com/lantervale/light/pkg/script"
)

// Kin are documents of one tab's page that reach each other's objects:
// those of one origin that keep their storage in one place, as the
// browser says (frames.Kin). They share an engine, each with its own
// globals, and so share one turn to run, one budget and one memory;
// a document of another origin is never of them, and has an engine of
// its own.
//
// Each tells the others of its window through a registry only light's
// own script holds: a function of the engine, handed from one document
// to the next as it joins, which no script of a page ever sees.
type Kin struct {
	key     string
	turn    *handover
	members []*Runtime
}

// NewKin makes the kin of that key, with no document yet.
func NewKin(key string) *Kin { return &Kin{key: key, turn: &handover{}} }

// Key is what the kin was made for.
func (k *Kin) Key() string { return k.key }

// Members is how many documents are of the kin.
func (k *Kin) Members() int {
	if k == nil {
		return 0
	}
	return len(k.members)
}

// Held reports whether a script of the kin waits for an answer: it
// holds their engine, so no document joins them meanwhile.
func (k *Kin) Held() bool { return k != nil && k.turn.parked }

// handover is the turn the documents of the kin share, one of its own
// for a document that has no kin.
func (k *Kin) handover() *handover {
	if k == nil {
		return &handover{}
	}
	return k.turn
}

func (k *Kin) name() string {
	if k == nil {
		return ""
	}
	return k.key
}

// beside is the engine a new document of the kin shares, nil for the
// first.
func (k *Kin) beside() script.Context {
	if k == nil || len(k.members) == 0 {
		return nil
	}
	if gated, is := k.members[0].ctx.(*gate); is {
		return gated.ctx
	}
	return nil
}

// has reports whether the document of that window is of the kin.
func (k *Kin) has(window int) bool {
	return slices.ContainsFunc(k.members, func(r *Runtime) bool { return r.host.Self == window })
}

// join makes r one of the kin: its own script is given the registry
// the others hold, or makes it, and tells it of r's window.
func (k *Kin) join(r *Runtime) error {
	if k == nil {
		return nil
	}
	var registry script.Function
	if len(k.members) > 0 {
		held, err := k.members[0].lights("kin")
		if err != nil {
			return err
		}
		kept, is := held.(script.Function)
		if !is {
			return errors.New("light: the documents of a page lost what they share")
		}
		registry = kept
		defer registry.Release()
	}
	var given any
	if registry != nil {
		given = registry
	}
	if _, err := r.lights("join", given, float64(r.host.Self)); err != nil {
		return err
	}
	k.members = append(k.members, r)
	return nil
}

// leave takes r out of the kin, and its window out of the registry.
func (k *Kin) leave(r *Runtime) {
	if k == nil || !slices.Contains(k.members, r) {
		return
	}
	k.members = slices.DeleteFunc(k.members, func(of *Runtime) bool { return of == r })
	_, _ = r.lights("leave")
}

// lights calls light's own script in r's document for what runs nothing
// of the page: it may while a document of another engine waits for an
// answer, which a script of the page may not.
func (r *Runtime) lights(args ...any) (any, error) {
	entry, gated := r.entry.(*gatedFunction)
	if !gated {
		return r.entry.Call(args...)
	}
	for i, arg := range args {
		if held, is := arg.(*gatedFunction); is {
			args[i] = held.fn
		}
	}
	var value any
	err := ErrAsking
	r.through(func() { value, err = entry.fn.Call(args...) })
	if fn, kept := value.(script.Function); kept {
		value = &gatedFunction{g: entry.g, fn: fn}
	}
	return value, err
}
