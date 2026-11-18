//go:build !darwin

package identity

import "os"

// System is the languages the user of this machine prefers, most
// wanted first: the environment's.
func System() []string { return Environment(os.Getenv) }
