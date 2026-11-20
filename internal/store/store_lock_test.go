package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStaleLockFromDeadProcessIsCleared(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Root: dir}
	// pid far above any real pid: not running
	if err := os.WriteFile(filepath.Join(dir, ".lock"), []byte("2147480000"), 0o600); err != nil {
		t.Fatal(err)
	}
	unlock, err := s.lock()
	if err != nil {
		t.Fatalf("stale lock not cleared: %v", err)
	}
	unlock()
}

func TestLiveLockIsRespected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".lock")
	// our own pid: a live holder, never treated as stale
	if err := os.WriteFile(path, []byte(itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if staleLock(path) {
		t.Fatal("own-pid lock treated as stale")
	}
	if !pidAlive(os.Getppid()) {
		t.Fatal("parent process reported dead")
	}
}

func itoa(n int) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
