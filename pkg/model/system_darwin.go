package model

import (
	"encoding/binary"
	"syscall"
)

func sysctl(name string) uint64 {
	raw, err := syscall.Sysctl(name)
	if err != nil {
		return 0
	}
	// Sysctl takes the value for a string and drops its last byte when
	// it is a zero: the number is the bytes it gave, low first.
	var bytes [8]byte
	copy(bytes[:], raw)
	return binary.LittleEndian.Uint64(bytes[:])
}

// Memory is the memory of this machine, in bytes, 0 when unknown.
func Memory() int64 { return int64(sysctl("hw.memsize")) } //nolint:gosec // a machine's memory fits

func fastCores() int {
	if cores := int(sysctl("hw.perflevel0.physicalcpu")); cores > 0 { //nolint:gosec // a count of cores
		return cores
	}
	if cores := int(sysctl("hw.physicalcpu")); cores > 0 { //nolint:gosec // a count of cores
		return cores
	}
	return logicalHalf()
}
