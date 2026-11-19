package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
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

// Every candidate needs at least one source fact. A candidate with zero
// facts gets the facts gap whether or not governed state exists.
func TestCoverageRequiresFactWithOrWithoutGovernedState(t *testing.T) {
	cases := []struct {
		name          string
		rel           map[string]string
		governedState string
	}{
		{"governed state present", map[string]string{"governed_state": "accepted_context_present"}, "accepted context"},
		{"no governed state", map[string]string{"source_name": "x"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			em := core.EmergION{
				MEM: core.Memory{SourceHash: "h", Bytes: 10, Stored: 10, Summary: "summary"},
				REL: tc.rel,
				CAP: []string{"OBS"},
			}
			err := coverage(&em, tc.governedState)
			if err == nil || !strings.Contains(err.Error(), "BRIDGEGAP:facts") {
				t.Fatalf("coverage err = %v, want BRIDGEGAP:facts", err)
			}
			if !containsString(em.VAL.Gaps, "BRIDGEGAP:facts") {
				t.Fatalf("gaps = %#v, want BRIDGEGAP:facts", em.VAL.Gaps)
			}
		})
	}
}

// With accepted (governed) state present, a candidate whose reasoner gives
// zero facts now gets the facts gap and stops for review as a recapture at
// GOV, the same as it does when no governed state exists.
func TestCaptureWithGovernedStateZeroFactsGetsFactsGap(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	prior := core.EmergION{
		IDN: "E-ACCEPTED-CONTEXT",
		STA: core.StateAtGOV,
		MEM: core.Memory{SourceHash: "accepted-source-hash", Bytes: 1, Stored: 1},
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1},
	}
	attachPreservedSource(t, s, &prior)
	if _, err := s.SaveCandidate(prior); err != nil {
		t.Fatal(err)
	}
	approved, decision, err := gov.Decide(prior, gov.Approve, "HUMAN_FINAL", "accepted context for facts coverage test")
	if err != nil {
		t.Fatal(err)
	}
	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	_, governedState, err := Runtime{Store: s}.governedStateContext()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(governedState) == "" {
		t.Fatal("test setup: governed state is empty")
	}

	source := filepath.Join(root, "nofacts.txt")
	if err := os.WriteFile(source, []byte("candidate source with no extracted facts"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
		Reasoner: lineageReasoner{result: reason.Result{
			Summary: "candidate with no facts",
			Relationships: map[string]string{
				"source_name":    "nofacts.txt",
				"governed_state": "accepted_context_present",
			},
			Capabilities: []string{"OBS"},
			Risk:         "L",
		}},
	}

	em, _, err := r.Capture(context.Background(), source, false)
	var recaptured *RecaptureError
	if !errors.As(err, &recaptured) {
		t.Fatalf("Capture err = %v, want RecaptureError for BRIDGEGAP:facts", err)
	}
	if !strings.Contains(err.Error(), "BRIDGEGAP:facts") {
		t.Fatalf("Capture err = %v, want BRIDGEGAP:facts", err)
	}
	if em.STA != core.StateAtGOV {
		t.Fatalf("recapture state = %s, want %s (stopped for review)", em.STA, core.StateAtGOV)
	}
	if em.REL["required_capability"] != "ESTABLISH_FACT" {
		t.Fatalf("required_capability = %q, want ESTABLISH_FACT", em.REL["required_capability"])
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
