// Package shell asks the system to open a file with what it opens its
// kind with, or to show it in its folder. It runs the system's own
// command for that, with the file as one argument, and never the file.
package shell

import (
	"context"
	"fmt"
	"os/exec"
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

// Command is the command a system does action on path with: its name,
// then its arguments.
func Command(system string, action Action, path string) ([]string, error) {
	switch system {
	case "darwin":
		if action == Reveal {
			return []string{"open", "-R", "--", path}, nil
		}
		return []string{"open", "--", path}, nil
	case "windows":
		if action == Reveal {
			return []string{"explorer", "/select," + path}, nil
		}
		return []string{"explorer", path}, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		if action == Reveal {
			return []string{"xdg-open", filepath.Dir(path)}, nil
		}
		return []string{"xdg-open", path}, nil
	}
	return nil, fmt.Errorf("light does not know how %s opens a file", system)
}

// System does the actions on the system light runs on.
type System struct{}

func (System) Open(path string) error   { return run(Open, path) }
func (System) Reveal(path string) error { return run(Reveal, path) }

func run(action Action, path string) error {
	args, err := Command(runtime.GOOS, action, path)
	if err != nil {
		return err
	}
	//nolint:gosec // the system's own opener, the file one argument of it
	if err = exec.CommandContext(context.Background(), args[0], args[1:]...).Start(); err != nil {
		return fmt.Errorf("asking the system to open %s: %w", filepath.Base(path), err)
	}
	return nil
}
