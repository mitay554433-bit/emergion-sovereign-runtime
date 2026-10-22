package runtime

import (
	"context"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/store"
)

func acceptedRealization(id string) core.EmergION {
	return core.EmergION{
		IDN: id,
		STA: core.StateAccepted,
	}
}

func TestCaptureRealizationSelectionAdmitsAtGOV(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	state := core.EmptyState()
	for _, id := range []string{"E-A", "E-B", "E-C"} {
		em := acceptedRealization(id)
		state.Accepted[id] = em
		acceptRealizationForSelectionGovernance(t, s, core.EmergION{
			IDN: em.IDN,
			STA: core.StateAtGOV,
			MEM: core.Memory{
				SourceHash: id + "-source",
				Bytes:      1,
				Stored:     1,
				Summary:    id,
			},
			REL: map[string]string{"source_kind": "SOURCE"},
			CAP: []string{"OBS"},
			VAL: core.Validation{
				Facts:  []string{id + " evidence"},
				Recoil: true,
				WVC:    true,
			},
			EVO: core.Evolution{Version: 1},
		})
	}

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B", "E-C"},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationLeftPreferred,
				Differences: []string{"A_BETTER_THAN_B"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-B",
				Right:       "E-C",
				Result:      RealizationLeftPreferred,
				Differences: []string{"B_BETTER_THAN_C"},
			},
		},
	}
	if err := p.Validate(state); err != nil {
		t.Fatal(err)
	}

	r := Runtime{Store: s}
	em, duplicate, err := r.CaptureRealizationSelection(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("first realization selection unexpectedly duplicate")
	}
	if em.STA != core.StateAtGOV {
		t.Fatalf("state = %q want %q", em.STA, core.StateAtGOV)
	}
	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatalf("selection admission missing RECOIL/WVC: %#v", em.VAL)
	}
	if em.REL["source_kind"] != "REALIZATION_SELECTION" {
		t.Fatalf("source_kind = %q", em.REL["source_kind"])
	}
	if em.REL["target_thing"] != "KNIFE" {
		t.Fatalf("target_thing = %q", em.REL["target_thing"])
	}
	if em.REL["selected_realization"] != "E-A" {
		t.Fatalf("selected_realization = %q", em.REL["selected_realization"])
	}
	if em.REL["runners_up"] != "E-B,E-C" {
		t.Fatalf("runners_up = %q", em.REL["runners_up"])
	}
}

func TestCaptureRealizationSelectionRefusesAmbiguousStrongest(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationUnresolved,
			},
		},
	}

	r := Runtime{Store: s}
	if _, _, err := r.CaptureRealizationSelection(context.Background(), p); err == nil {
		t.Fatal("ambiguous strongest front unexpectedly admitted")
	}
}

func TestCaptureRealizationSelectionEvidenceIsDeterministic(t *testing.T) {
	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members:     []string{"E-A", "E-B", "E-C"},
		Comparisons: []RealizationComparison{
			{
				TargetThing: "KNIFE",
				Left:        "E-B",
				Right:       "E-C",
				Result:      RealizationLeftPreferred,
				Differences: []string{"B2", "B1"},
			},
			{
				TargetThing: "KNIFE",
				Left:        "E-A",
				Right:       "E-B",
				Result:      RealizationLeftPreferred,
				Differences: []string{"A1"},
			},
		},
	}
	selection := RealizationSelection{
		TargetThing: "KNIFE",
		Selected:    "E-A",
		RunnersUp:   []string{"E-C", "E-B"},
	}

	got := string(realizationSelectionEvidence(p, selection))
	if strings.Index(got, "RUNNER_UP:E-B") > strings.Index(got, "RUNNER_UP:E-C") {
		t.Fatalf("runner-up evidence not deterministic: %q", got)
	}
	if strings.Index(got, "COMPARISON:E-A|E-B") > strings.Index(got, "COMPARISON:E-B|E-C") {
		t.Fatalf("comparison evidence not deterministic: %q", got)
	}
	if strings.Index(got, "DIFFERENCE:B1") > strings.Index(got, "DIFFERENCE:B2") {
		t.Fatalf("Difference evidence not deterministic: %q", got)
	}
}
