//go:build !unix

package store

// pidAlive is conservative where liveness can't be checked: assume alive.
func pidAlive(pid int) bool { return true }
