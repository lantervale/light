//go:build !(v8 && cgo && (darwin || linux) && (amd64 || arm64))

package script

// Engine names the engine of this build: none.
func Engine() string { return "" }

// Available reports whether this build runs scripts.
func Available() bool { return false }

// New gives no Context: this build has no engine.
func New(Options) (Context, error) { return nil, ErrUnavailable }
