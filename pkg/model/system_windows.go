package model

import (
	"syscall"
	"unsafe"
)

// Memory is the memory of this machine, in bytes, 0 when unknown.
func Memory() int64 {
	// MEMORYSTATUSEX
	var status struct {
		length uint32
		load   uint32
		total  uint64
		rest   [6]uint64
	}
	status.length = uint32(unsafe.Sizeof(status))
	call := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	if ok, _, _ := call.Call(uintptr(unsafe.Pointer(&status))); ok == 0 {
		return 0
	}
	return int64(status.total) //nolint:gosec // a machine's memory fits
}

func fastCores() int { return logicalHalf() }
