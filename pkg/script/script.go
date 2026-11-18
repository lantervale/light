// Package script is the boundary to the engine that runs a page's
// scripts: a Context runs sources and calls the functions they gave
// back, and scripts call the Go functions bound into it. Values cross as
// plain Go ones; nothing of the engine shows through, and a build
// without one (engine_none.go) has no Context at all.
package script

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrUnavailable is why New gives nothing in a build without an
	// engine.
	ErrUnavailable = errors.New("this build of light has no JavaScript engine")
	// ErrInterrupted is how a script that ran over its budget ended.
	ErrInterrupted = errors.New("the script ran too long and was stopped")
	// ErrOutOfMemory is how a script that outgrew its memory ended.
	ErrOutOfMemory = errors.New("the script used too much memory and was stopped")
	// ErrClosed is what a closed Context answers.
	ErrClosed = errors.New("the script's context is closed")
)

// Func is a Go function scripts call. Its arguments and its result are
// plain values: nil for null and undefined, bool, float64, string,
// []any and map[string]any. An error is thrown to the script.
type Func func(args []any) (any, error)

// Function is a function of a script, kept to be called later.
type Function interface {
	// Call calls it with plain values and gives its result as Run does.
	Call(args ...any) (any, error)
	// Release lets the engine forget it.
	Release()
}

// Context is one page's scripts: their globals, and what they kept.
// Only one goroutine uses it at a time.
type Context interface {
	// Bind makes fn a global function of that name.
	Bind(name string, fn Func) error
	// Run runs source as a classic script that origin names in errors,
	// then the tasks it queued. Its value comes back as a plain one, a
	// function as a Function.
	Run(source, origin string) (any, error)
	// Interrupt stops the script that is running, from any goroutine.
	Interrupt()
	// Close frees the context; it is safe to call twice.
	Close()
}

// Options are a Context's limits and who hears of its failures.
type Options struct {
	// Budget is how long one Run or Call may take, DefaultBudget when 0.
	Budget time.Duration
	// Memory is the heap a context may grow to in bytes, DefaultMemory
	// when 0.
	Memory uint64
	// Rejected is told each promise rejected with nobody to handle it,
	// once the script that left it so has returned.
	Rejected func(*Error)
}

const (
	// DefaultBudget is how long a script runs before it is stopped.
	DefaultBudget = 2 * time.Second
	// DefaultMemory is the heap of a context.
	DefaultMemory = 512 << 20
)

// Error is an exception no script caught.
type Error struct {
	Message string
	// Source is the origin of the script it was thrown in, and Line and
	// Column where, from 1; 0 when the engine does not say.
	Source       string
	Line, Column int
	Stack        string
}

func (e *Error) Error() string {
	if e.Source == "" {
		return e.Message
	}
	return fmt.Sprintf("%s (%s:%d)", e.Message, e.Source, e.Line)
}
