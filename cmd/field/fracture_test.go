package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeWorktreeSynced(t *testing.T) {
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	run(root, "init", "-q", repo)
	run(repo, "config", "user.email", "probe@example.invalid")
	run(repo, "config", "user.name", "Probe")
	run(repo, "commit", "-q", "--allow-empty", "-m", "one")
	probes := filepath.Join(root, "esr-probes")
	run(repo, "worktree", "add", "-q", "--detach", probes, "HEAD")

	if err := probeWorktreeSynced(repo, ""); err == nil || !strings.Contains(err.Error(), "git worktree add --detach") {
		t.Fatalf("missing worktree not explained: %v", err)
	}
	if err := probeWorktreeSynced(repo, probes); err != nil {
		t.Fatalf("synced worktree refused: %v", err)
	}
	run(repo, "commit", "-q", "--allow-empty", "-m", "two")
	if err := probeWorktreeSynced(repo, probes); err == nil || !strings.Contains(err.Error(), "checkout -q --detach") {
		t.Fatalf("stale worktree not refused: %v", err)
	}
}
