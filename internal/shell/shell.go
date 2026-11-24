// Package shell asks the system to open a file with what it opens its
// kind with, or to show it in its folder. It runs the system's own
// command for that, with the file as one argument, and never the file.
package shell

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
)

// Action is what the system is asked to do with a file.
type Action int

const (
	// Open opens the file with what the system opens its kind with.
	Open Action = iota
	// Reveal shows the file in its folder.
	Reveal
)

// Command is the command a system does action on file with: its name,
// then its arguments. file is a path of that system, its folder taken by
// that system's separator whatever system light runs on.
func Command(system string, action Action, file string) ([]string, error) {
	switch system {
	case "darwin":
		if action == Reveal {
			return []string{"open", "-R", "--", file}, nil
		}
		return []string{"open", "--", file}, nil
	case "windows":
		if action == Reveal {
			return []string{"explorer", "/select," + file}, nil
		}
		return []string{"explorer", file}, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		if action == Reveal {
			return []string{"xdg-open", path.Dir(file)}, nil
		}
		return []string{"xdg-open", file}, nil
	}
	return nil, fmt.Errorf("light does not know how %s opens a file", system)
}

// System does the actions on the system light runs on.
type System struct{}

func (System) Open(file string) error   { return run(Open, file) }
func (System) Reveal(file string) error { return run(Reveal, file) }

func run(action Action, file string) error {
	args, err := Command(runtime.GOOS, action, file)
	if err != nil {
		return err
	}
	//nolint:gosec // the system's own opener, the file one argument of it
	if err = exec.CommandContext(context.Background(), args[0], args[1:]...).Start(); err != nil {
		return fmt.Errorf("asking the system to open %s: %w", filepath.Base(file), err)
	}
	return nil
}
