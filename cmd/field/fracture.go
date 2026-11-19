package main

import (
	"fmt"
	"os/exec"
	"strings"

	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/store"
)

// probeWorktreeSetup is the documented one-time setup for the dedicated
// fracture probe worktree. It is run by the operator, never by field.
const probeWorktreeSetup = `git worktree add --detach ../esr-probes HEAD
export FIELD_PROBE_WORKTREE="$(cd ../esr-probes && pwd)"`

// probeWorktreeSync is the documented command that moves the dedicated
// probe worktree to the current HEAD after a structural addition lands.
const probeWorktreeSync = `git -C "$FIELD_PROBE_WORKTREE" checkout -q --detach "$(git rev-parse HEAD)"`

// probeWorktreeSynced refuses to use a probe worktree that is missing or not
// at the same commit as the operator's current worktree, so a probe always
// fractures against the structure as it is now. It never moves or deletes
// anything.
func probeWorktreeSynced(currentDir, probeDir string) error {
	probeDir = strings.TrimSpace(probeDir)
	if probeDir == "" {
		return fmt.Errorf("fracture probes need a dedicated clean worktree; one-time setup from the repository:\n%s", probeWorktreeSetup)
	}
	head := func(dir string) (string, error) {
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err != nil {
			return "", fmt.Errorf("%s is not a git worktree: %w", dir, err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	current, err := head(currentDir)
	if err != nil {
		return err
	}
	probe, err := head(probeDir)
	if err != nil {
		return fmt.Errorf("%w; one-time setup from the repository:\n%s", err, probeWorktreeSetup)
	}
	if current != probe {
		return fmt.Errorf("probe worktree is at %s but the current HEAD is %s; sync it with:\n%s", probe, current, probeWorktreeSync)
	}
	return nil
}

func targetIsFractureProbe(s *store.Store, id string) (bool, error) {
	events, err := s.Events()
	if err != nil {
		return false, err
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(st.Accepted[id].REL["fracture_gap"]) != "", nil
}
