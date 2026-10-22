package runtime

import (
	"context"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

func captureAcceptedAutoReorderMember(
	t *testing.T,
	s *store.Store,
	name string,
) string {
	t.Helper()
	r := Runtime{
		Store: s,
		Reasoner: fixedReasoner{
			name: "auto-reorder-member",
			version: "v1",
			result: reason.Result{
				Summary: name + " evidence",
				Relationships: map[string]string{"target_thing": "KNIFE"},
				Capabilities: []string{"ANALYZE"},
				Facts: []string{name + " evidence"},
				Risk: "L",
			},
		},
	}
	em, duplicate, err := r.CaptureBytes(
		context.Background(),
		name,
		[]byte(name+" evidence"),
		"auto_reorder_fixture",
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatalf("%s unexpectedly duplicate", name) }
	acceptRealizationForSelectionGovernance(t, s, em)
	return em.IDN
}

func captureAcceptedAutoReorderResult(
	t *testing.T,
	r Runtime,
	parent string,
	succeeded bool,
) {
	t.Helper()
	request := adapters.ExecutionRequest{
		EmergIONID: parent,
		SourceHash: parent + "-source",
		AuthorizationID: "EV-Q-" + parent,
		Authority: "HUMAN_FINAL",
		Adapter: "LOCAL_GEMMA",
		Action: "ANALYZE",
	}
	request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)
	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: succeeded,
			Output: "observed realization execution",
			Error: func() string {
				if succeeded { return "" }
				return "observed execution failure"
			}(),
		},
	)
	signal, duplicate, err := r.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("execution result unexpectedly duplicate") }
	acceptRealizationForSelectionGovernance(t, r.Store, signal)
}

func TestCaptureNextObservedRealizationReorderUsesCanonicalSelectionEvidence(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}

	a := captureAcceptedAutoReorderMember(t, s, "knife-a")
	b := captureAcceptedAutoReorderMember(t, s, "knife-b")

	population := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{a, b},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: a,
			Right: b,
			Result: RealizationLeftPreferred,
			Differences: []string{"INITIAL:A>B"},
		}},
	}
	selection, duplicate, err := r.CaptureRealizationSelection(
		context.Background(),
		population,
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("selection unexpectedly duplicate") }
	acceptRealizationForSelectionGovernance(t, s, selection)

	captureAcceptedAutoReorderResult(t, r, a, false)
	captureAcceptedAutoReorderResult(t, r, b, true)

	reorder, proposed, err := r.CaptureNextObservedRealizationReorder(
		context.Background(),
	)
	if err != nil { t.Fatal(err) }
	if !proposed { t.Fatal("changed observed order did not produce reorder candidate") }
	if reorder.REL["source_kind"] != "REALIZATION_REORDER" {
		t.Fatalf("source_kind = %q", reorder.REL["source_kind"])
	}
	if reorder.REL["selected_realization"] != b {
		t.Fatalf("selected = %q want %q", reorder.REL["selected_realization"], b)
	}
	if reorder.STA != "G" || !reorder.VAL.Recoil || !reorder.VAL.WVC {
		t.Fatalf("reorder did not stop GOV-ready at AtGOV: %#v", reorder)
	}
}

func TestCaptureNextObservedRealizationReorderWaitsForComparableResults(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}

	a := captureAcceptedAutoReorderMember(t, s, "knife-a")
	b := captureAcceptedAutoReorderMember(t, s, "knife-b")
	population := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{a, b},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: a,
			Right: b,
			Result: RealizationLeftPreferred,
			Differences: []string{"INITIAL:A>B"},
		}},
	}
	selection, duplicate, err := r.CaptureRealizationSelection(context.Background(), population)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("selection unexpectedly duplicate") }
	acceptRealizationForSelectionGovernance(t, s, selection)

	captureAcceptedAutoReorderResult(t, r, a, false)

	_, proposed, err := r.CaptureNextObservedRealizationReorder(context.Background())
	if err != nil { t.Fatal(err) }
	if proposed {
		t.Fatal("one-sided execution evidence produced false reorder candidate")
	}
}
