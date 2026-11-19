//go:build !windows

package model_test

import "syscall"

// alive reports whether the process of that id still runs.
func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }
