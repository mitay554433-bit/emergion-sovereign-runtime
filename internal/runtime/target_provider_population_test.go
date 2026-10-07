package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

func TestTargetComparisonMaterializesAcceptedProviderPopulationAtGOV(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	accept := func(em core.EmergION) {
		t.Helper()

		if _, err := s.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			"accept provider fixture",
		)
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
	}

	accept(core.EmergION{
		IDN: "E-TARGET-PROVIDER-OBS",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "target-provider-obs-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted OBS provider",
		},
		REL: map[string]string{
			"source_name": "target-provider-obs",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{Version: 1},
	})

	accept(core.EmergION{
		IDN: "E-TARGET-PROVIDER-VLD",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "target-provider-vld-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted VLD provider",
		},
		REL: map[string]string{
			"source_name": "target-provider-vld",
		},
		CAP: []string{"VLD"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{Version: 1},
	})

	observer := &recordingTargetComparisonReasoner{}
	rt := Runtime{Store: s, Reasoner: observer}

	target, duplicate, err := rt.CaptureTarget(
		context.Background(),
		"persistent governed target requiring accepted fact establishment",
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("target unexpectedly duplicate")
	}

	comparison, duplicate, err := rt.CaptureTargetComparison(
		context.Background(),
		target,
		map[string]core.EmergION{
			"E-TARGET-PROVIDER-OBS": {
				IDN: "E-TARGET-PROVIDER-OBS",
				STA: core.StateAccepted,
				CAP: []string{"OBS"},
			},
			"E-TARGET-PROVIDER-VLD": {
				IDN: "E-TARGET-PROVIDER-VLD",
				STA: core.StateAccepted,
				CAP: []string{"VLD"},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("target comparison unexpectedly duplicate")
	}

	if comparison.REL["required_capability"] != "ESTABLISH_FACT" {
		t.Fatalf(
			"required_capability = %q want ESTABLISH_FACT",
			comparison.REL["required_capability"],
		)
	}

	if comparison.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"capability_resolution = %q want COMPOSABLE_CANDIDATE",
			comparison.REL["capability_resolution"],
		)
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.AtGOV[comparison.IDN]; !ok {
		t.Fatal("target comparison missing from GOV")
	}
	if _, ok := st.Accepted[comparison.IDN]; ok {
		t.Fatal("target comparison self-authorized into REG")
	}

	var populations []core.EmergION
	for _, em := range st.AtGOV {
		if em.MEM.Provenance == "provider_population" &&
			em.REL["origin"] == comparison.IDN {
			populations = append(populations, em)
		}
	}

	if len(populations) != 1 {
		t.Fatalf(
			"provider populations for comparison = %d want 1",
			len(populations),
		)
	}

	population := populations[0]

	if population.REL["required_capability"] != "ESTABLISH_FACT" {
		t.Fatalf(
			"provider required_capability = %q want ESTABLISH_FACT",
			population.REL["required_capability"],
		)
	}

	if population.REL["capability_composition"] != "OBS+VLD" {
		t.Fatalf(
			"provider composition = %q want OBS+VLD",
			population.REL["capability_composition"],
		)
	}

	providers := population.REL["capability_providers"]
	if !strings.Contains(providers, "OBS:E-TARGET-PROVIDER-OBS") ||
		!strings.Contains(providers, "VLD:E-TARGET-PROVIDER-VLD") {
		t.Fatalf("provider population = %q", providers)
	}

	if population.REL["origin"] != comparison.IDN {
		t.Fatalf(
			"provider origin = %q want %q",
			population.REL["origin"],
			comparison.IDN,
		)
	}

	if population.STA != core.StateAtGOV {
		t.Fatalf(
			"provider state = %q want %q",
			population.STA,
			core.StateAtGOV,
		)
	}

	if !population.VAL.Recoil || !population.VAL.WVC {
		t.Fatal("provider population bypassed RECOIL/WVC")
	}

	if _, exists := population.REL["COMPOSITION_KIN"]; exists {
		t.Fatal("provider population self-created COMPOSITION_KIN")
	}

	if _, ok := st.Accepted[population.IDN]; ok {
		t.Fatal("provider population self-authorized into REG")
	}

	// Exactly one provider_population branch proves that its own admission
	// did not recursively materialize another provider population.
	totalPopulation := 0
	for _, em := range st.AtGOV {
		if em.MEM.Provenance == "provider_population" {
			totalPopulation++
		}
	}
	if totalPopulation != 1 {
		t.Fatalf(
			"provider_population candidates = %d want 1",
			totalPopulation,
		)
	}
}
