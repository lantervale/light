// Package globals holds what light's commands share of the command
// line: the build and the folders.
package globals

import "fmt"

// Build is the light that was built.
type Build struct {
	Name, Version, Commit, Date string
}

func (b Build) String() string {
	return fmt.Sprintf("%s %s (%s, %s)", b.Name, b.Version, b.Commit, b.Date)
}
