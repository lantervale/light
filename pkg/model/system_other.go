//go:build !darwin && !linux && !windows

package model

// Memory is the memory of this machine, in bytes, 0 when unknown.
func Memory() int64 { return 0 }

func fastCores() int { return logicalHalf() }
