package model

import "runtime"

// Threads is how many threads compute when the settings name none: half
// the cores that are fast, two at least and eight at most. Every core
// at once starves light and the rest of the machine, and was many times
// slower on a busy one.
func Threads() int {
	return threadsOf(fastCores())
}

func threadsOf(cores int) int {
	return min(max(cores/2, 2), 8)
}

// logicalHalf is the cores of a machine that says only how many
// threads it runs at once: half of them, two a core.
func logicalHalf() int { return max(runtime.NumCPU()/2, 1) }
