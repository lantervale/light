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
	// ErrNotKin is why no Context is made beside another: the two are
	// not of the same kin.
	ErrNotKin = errors.New("a context shares an engine only with its kin")
)

// Func is a Go function scripts call. Its arguments and its result are
// plain values: nil for null and undefined, bool, float64, string,
// []any and map[string]any. A SharedArrayBuffer comes as []byte, the
// script's own memory, to read and to write until the function returns:
// how bytes cross either way. An error is thrown to the script.
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
	// Module reads source as a module that address names in errors and
	// in its import.meta.url; nothing of it runs yet. JSON is the module
	// whose default export is what a JSON text says.
	Module(source, address string) (Module, error)
	JSON(text, address string) (Module, error)
	// Interrupt stops the script that is running, from any goroutine.
	Interrupt()
	// Pause stops the clock of the script that is running, from a
	// function it called: the time until resume is called is none of
	// its budget.
	Pause() (resume func())
	// Close frees the context; it is safe to call twice.
	Close()
}

// Import is one module another asks for: its specifier as written, and
// the type its attributes name, "" for a script.
type Import struct {
	Specifier, Type string
}

// Module is a module of a page: read, linked to those it imports, then
// run, once.
type Module interface {
	// Imports are the modules it imports by name, in its order.
	Imports() []Import
	// Link binds it to the modules it imports and those to theirs, each
	// asked of resolve by the module that names it. A module already
	// linked is left as it is.
	Link(resolve func(from Module, what Import) (Module, error)) error
	// Run runs the modules it imports, then it, each once in the life
	// of the Context. done is told how it ended, an Error for what it
	// threw: before Run returns, or when what it awaits at its top
	// level has come.
	Run(done func(error))
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
	// Import is asked for the module an import() names, from the
	// script or the module at the address from: done is given it once
	// it ran, or why there is none, an Error whose Message begins with
	// the name of its kind ("TypeError: …"). Without it an import() is
	// rejected.
	Import func(from string, what Import, done func(Module, error))
	// Resolve is the address import.meta.resolve gives for a specifier
	// in the module at from; without it a module has no such function.
	Resolve func(from, specifier string) (string, error)
	// Beside is a Context whose engine the new one shares: the two then
	// hold each other's objects, though each has its own globals, and
	// one budget and one memory are theirs both. Kin names who may: a
	// Context is made beside another only when both name the same kin,
	// and one that names none shares its engine with nobody.
	Beside Context
	Kin    string
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
