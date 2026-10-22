package runtime

import (
	"context"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

func acceptRealizationForSelectionGovernance(t *testing.T, s *store.Store, em core.EmergION) {
	t.Helper()
	if _, err := s.SaveCandidate(em); err != nil { t.Fatal(err) }
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "realization selection governance fixture")
	if err != nil { t.Fatal(err) }
	decisionID, err := s.SaveDecision(decision)
	if err != nil { t.Fatal(err) }
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil { t.Fatal(err) }
	if _, err := s.SaveAccepted(receipt); err != nil { t.Fatal(err) }
}

func TestCaptureRealizationSelectionRejectsNonREGPopulation(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE", Left: "E-A", Right: "E-B",
			Result: RealizationLeftPreferred, Differences: []string{"A_BETTER_THAN_B"},
		}},
	}
	r := Runtime{Store: s}
	if _, _, err := r.CaptureRealizationSelection(context.Background(), p); err == nil ||
		!strings.Contains(err.Error(), "not REG-accepted") {
		t.Fatalf("non-REG population admission error = %v", err)
	}
}

func TestRealizationSelectionUsesExistingHumanFinalREGPath(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }

	member := func(id string) core.EmergION {
		return core.EmergION{
			IDN: id, STA: core.StateAtGOV,
			MEM: core.Memory{SourceHash: id + "-source", Bytes: 1, Stored: 1, Summary: id},
			REL: map[string]string{"source_kind": "SOURCE"},
			CAP: []string{"OBS"},
			VAL: core.Validation{Facts: []string{id + " evidence"}, Recoil: true, WVC: true},
			EVO: core.Evolution{Version: 1},
		}
	}
	acceptRealizationForSelectionGovernance(t, s, member("E-A"))
	acceptRealizationForSelectionGovernance(t, s, member("E-B"))

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE", Left: "E-A", Right: "E-B",
			Result: RealizationLeftPreferred, Differences: []string{"A_BETTER_THAN_B"},
		}},
	}
	r := Runtime{Store: s}
	selection, duplicate, err := r.CaptureRealizationSelection(context.Background(), p)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("selection unexpectedly duplicate") }

	if _, err := r.AuthorizeAction(selection.IDN, "LOCAL_GEMMA", "ANALYZE", "must fail before REG", true); err == nil ||
		!strings.Contains(err.Error(), "not REG-accepted") {
		t.Fatalf("pre-HUMAN_FINAL authorization error = %v", err)
	}

	approved, decision, err := gov.Decide(selection, gov.Approve, "HUMAN_FINAL", "approve evidence-bound realization selection")
	if err != nil { t.Fatal(err) }
	decisionID, err := s.SaveDecision(decision)
	if err != nil { t.Fatal(err) }
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil { t.Fatal(err) }
	if _, err := s.SaveAccepted(receipt); err != nil { t.Fatal(err) }

	state, _, err := r.governedStateContext()
	if err != nil { t.Fatal(err) }
	accepted, ok := state.Accepted[selection.IDN]
	if !ok { t.Fatal("HUMAN_FINAL approval did not produce REG acceptance") }
	if accepted.REL["source_kind"] != "REALIZATION_SELECTION" ||
		accepted.REL["selected_realization"] != "E-A" ||
		accepted.REL["runners_up"] != "E-B" {
		t.Fatalf("REG acceptance lost selection evidence: %#v", accepted.REL)
	}
	if _, ok := state.AtGOV[selection.IDN]; ok {
		t.Fatal("REG-accepted selection remained AtGOV")
	}
}
