//go:build !windows

package engine

import (
	"runtime"
	"syscall"
)

// peakMemory is the most memory the helper held at once, in bytes: what
// the system counts as resident, which leaves out what a graphics card
// holds.
func peakMemory() int64 {
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return 0
	}
	peak := int64(usage.Maxrss) //nolint:unconvert // narrower on 32-bit systems
	if runtime.GOOS != "darwin" {
		peak <<= 10
	}
	return peak
}
