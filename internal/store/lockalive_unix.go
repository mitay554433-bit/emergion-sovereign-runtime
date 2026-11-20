//go:build unix

package store

import (
	"errors"
	"syscall"
)

// pidAlive reports whether a process with this pid exists. EPERM means it
// exists but belongs to someone else, so it counts as alive.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
