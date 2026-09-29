package fieldapi

import (
	"context"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
)

func TestPersistentTargetSurvivesRebuildRestartAndDuplicateCapture(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")

	s, err := store.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	rt := fieldruntime.Runtime{Store: s, Reasoner: sawCirculationReasoner{}}
	ctx := context.Background()

	target, duplicate, err := rt.CaptureTarget(ctx, "TARGET: governed persistent objective")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("first target capture was incorrectly treated as duplicate")
	}

	if target.STA != core.StateAtGOV {
		t.Fatalf("target state = %q want %q", target.STA, core.StateAtGOV)
	}
	if target.REL["source_kind"] != "TARGET" {
		t.Fatalf("target source_kind = %q want TARGET", target.REL["source_kind"])
	}
	if target.REL["target_state"] != "TARGET: governed persistent objective" {
		t.Fatalf("target_state = %q", target.REL["target_state"])
	}
	if target.MEM.SourceHash == "" {
		t.Fatal("persistent target has no source hash")
	}

	api, err := Open(stateRoot, sawCirculationReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Decide(target.IDN, "APPROVE", "accept persistent target"); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	events, err := reopened.Events()
	if err != nil {
		t.Fatal(err)
	}
	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	accepted, ok := state.Accepted[target.IDN]
	if !ok {
		t.Fatalf("persistent target %s did not survive FIELD rebuild", target.IDN)
	}
	if accepted.MEM.SourceHash != target.MEM.SourceHash {
		t.Fatalf("target source identity changed across restart: %q -> %q", target.MEM.SourceHash, accepted.MEM.SourceHash)
	}
	if accepted.REL["source_kind"] != "TARGET" || accepted.REL["target_state"] != target.REL["target_state"] {
		t.Fatal("target relationships changed across restart")
	}

	rt2 := fieldruntime.Runtime{Store: reopened, Reasoner: sawCirculationReasoner{}}
	duplicateTarget, duplicate, err := rt2.CaptureTarget(ctx, "TARGET: governed persistent objective")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("repeated target capture did not resolve as duplicate")
	}
	if duplicateTarget.IDN != target.IDN {
		t.Fatalf("duplicate target identity changed: %q -> %q", target.IDN, duplicateTarget.IDN)
	}

	finalEvents, err := reopened.Events()
	if err != nil {
		t.Fatal(err)
	}
	candidateCount := 0
	for _, event := range finalEvents {
		if event.Type == "C" && event.EmergION != nil && event.EmergION.REL["source_kind"] == "TARGET" {
			candidateCount++
		}
	}
	if candidateCount != 1 {
		t.Fatalf("persistent target candidate count = %d want 1", candidateCount)
	}
}
