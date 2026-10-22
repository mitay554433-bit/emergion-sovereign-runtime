package runtime

import (
	"context"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/store"
)

func TestCaptureObservedRealizationReorderAdmitsAtGOV(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }

	for _, id := range []string{"E-A", "E-B"} {
		acceptRealizationForSelectionGovernance(t, s, core.EmergION{
			IDN: id,
			STA: core.StateAtGOV,
			MEM: core.Memory{SourceHash: id + "-source", Bytes: 1, Stored: 1, Summary: id},
			REL: map[string]string{"source_kind": "SOURCE", "target_thing": "KNIFE"},
			CAP: []string{"ANALYZE"},
			VAL: core.Validation{Facts: []string{id + " evidence"}, Recoil: true, WVC: true},
			EVO: core.Evolution{Version: 1},
		})
	}

	r := Runtime{Store: s}
	for _, outcome := range []struct {
		parent    string
		succeeded bool
	}{
		{parent: "E-A", succeeded: false},
		{parent: "E-B", succeeded: true},
	} {
		request := adapters.ExecutionRequest{
			EmergIONID:      outcome.parent,
			SourceHash:      outcome.parent + "-source",
			AuthorizationID: "EV-Q-" + outcome.parent,
			Authority:       "HUMAN_FINAL",
			Adapter:         "LOCAL_GEMMA",
			Action:          "ANALYZE",
		}
		request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)
		result := adapters.BindExecutionResult(
			request,
			adapters.ExecutionResult{
				Succeeded: outcome.succeeded,
				Output:    "observed realization execution",
				Error: func() string {
					if outcome.succeeded { return "" }
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
		acceptRealizationForSelectionGovernance(t, s, signal)
	}

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: "E-A",
			Right: "E-B",
			Result: RealizationLeftPreferred,
			Differences: []string{"INITIAL:A>B"},
		}},
	}

	em, duplicate, err := r.CaptureObservedRealizationReorder(context.Background(), p)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("first reorder unexpectedly duplicate") }
	if em.STA != core.StateAtGOV {
		t.Fatalf("state = %q want %q", em.STA, core.StateAtGOV)
	}
	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatalf("reorder missing RECOIL/WVC: %#v", em.VAL)
	}
	if em.REL["source_kind"] != "REALIZATION_REORDER" {
		t.Fatalf("source_kind = %q", em.REL["source_kind"])
	}
	if em.REL["selected_realization"] != "E-B" {
		t.Fatalf("selected_realization = %q want E-B", em.REL["selected_realization"])
	}
	if em.REL["runners_up"] != "E-A" {
		t.Fatalf("runners_up = %q want E-A", em.REL["runners_up"])
	}
}

func TestCaptureObservedRealizationReorderEvidenceDeterministic(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-B", "E-A"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: "E-A",
			Right: "E-B",
			Result: RealizationRightPreferred,
			Differences: []string{"Z", "A"},
		}},
	}
	order := RealizationOrder{Fronts: [][]string{{"E-B"}, {"E-A"}}}
	got := string(realizationReorderEvidence(p, order))
	if strings.Index(got, "DIFFERENCE:A") > strings.Index(got, "DIFFERENCE:Z") {
		t.Fatalf("Difference evidence not deterministic: %q", got)
	}
	if !strings.Contains(got, "FRONT:0:E-B") ||
		!strings.Contains(got, "FRONT:1:E-A") {
		t.Fatalf("order evidence incomplete: %q", got)
	}
}
