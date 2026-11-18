package runtime

import (
	"context"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

// Facts hold only source-grounded entries: the runtime no longer adds the
// "living_state_projected" label, and coverage no longer requires it.
func TestCoverageWithGovernedStateNeedsNoProjectionLabel(t *testing.T) {
	em := core.EmergION{
		IDN: "E-COVERAGE-NO-LABEL",
		MEM: core.Memory{SourceHash: "h", Bytes: 10, Stored: 10, Summary: "summary"},
		REL: map[string]string{"governed_state": "accepted_context_present"},
		CAP: []string{"OBS"},
		VAL: core.Validation{Facts: []string{"frames sampled 1"}},
	}
	if err := coverage(&em, "accepted context"); err != nil {
		t.Fatalf("coverage without the projection label failed: %v", err)
	}
	for _, f := range em.VAL.Facts {
		if f == "living_state_projected" {
			t.Fatal("coverage added the projection label to facts")
		}
	}
}

// Admission outcomes are unchanged: with governed state present the facts gap
// never fired before (the label always filled Facts), and it still does not.
func TestCoverageWithGovernedStateKeepsFactsGapOutcome(t *testing.T) {
	em := core.EmergION{
		MEM: core.Memory{SourceHash: "h", Bytes: 10, Stored: 10, Summary: "summary"},
		REL: map[string]string{"governed_state": "accepted_context_present"},
		CAP: []string{"OBS"},
	}
	if err := coverage(&em, "accepted context"); err != nil {
		t.Fatalf("coverage = %v", err)
	}
	em.REL = map[string]string{"source_name": "x"}
	if err := coverage(&em, ""); err == nil || !containsString(em.VAL.Gaps, "BRIDGEGAP:facts") {
		t.Fatalf("facts gap without governed state: err=%v gaps=%v", err, em.VAL.Gaps)
	}
}

func TestHeuristicFactsOmitProjectionLabel(t *testing.T) {
	res, err := reason.Heuristic{}.Analyze(context.Background(), reason.Input{
		Name:          "note.txt",
		Content:       []byte("plain source"),
		GovernedState: "accepted context",
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsString(res.Facts, "living_state_projected") {
		t.Fatalf("facts = %#v", res.Facts)
	}
	if res.Relationships["governed_state"] != "accepted_context_present" {
		t.Fatalf("relationships = %#v", res.Relationships)
	}
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
