package runtime

import (
	"os"
	"sort"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reg"
)

// TORQUE / next-probe stoppage probe (APProver method).
//
// Canon frontier: B_n -> P_n, deriving the next bounded probe from a recorded
// genuine-remainder BRIDGEGAP without a new architecture, without preselecting
// the capability the probe should discover, and without using provider
// population as pressure.
//
// This test tries the only existing probe seam, adapters.PrepareExecution,
// for every recipe token of each BRIDGEGAP kind, against the HUMAN_FINAL /
// REG-accepted comparison that carries the BRIDGEGAP (the boundary). It
// records the exact stoppage for each token. No production code derives a
// probe, nothing executes, and nothing is written beyond the fixture.
//
// By default the test pins the stoppage and passes, so it fails loudly the
// day an existing seam can carry a probe and the design must be revisited.
// With UNIFUSION_TORQUE_PROBE=1 it fails on purpose and prints the stoppage,
// as the APProver failing probe.

const (
	torqueStopNoAdapter     = "NO_ADAPTER_FOR_RECIPE_TOKEN"
	torqueStopNotDerivable  = "NOT_DERIVABLE_FROM_BOUNDARY_EMERGION"
	torqueProbeWouldPrepare = "PREPARED"
)

func acceptBoundaryCandidate(t *testing.T, f differenceFixture, em core.EmergION) {
	t.Helper()
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "accept BRIDGEGAP boundary for TORQUE probe")
	if err != nil {
		t.Fatal(err)
	}
	decisionID, err := f.store.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}
}

// probeRecipeToken tries the existing probe seam for one recipe token
// against the accepted boundary EmergION.
func probeRecipeToken(st core.State, boundaryID, token string) string {
	var adapterIDs []string
	for _, adapter := range adapters.Catalog(true) {
		for _, capability := range adapter.Capabilities {
			if capability == token {
				adapterIDs = append(adapterIDs, adapter.ID)
			}
		}
	}
	if len(adapterIDs) == 0 {
		return torqueStopNoAdapter
	}
	sort.Strings(adapterIDs)
	for _, adapterID := range adapterIDs {
		request, err := adapters.PrepareExecution(st, boundaryID, adapterID, token, true)
		if err == nil && request.TransitionEmergIONID != "" {
			return torqueProbeWouldPrepare
		}
	}
	return torqueStopNotDerivable
}

func TestTorqueNextProbeStoppage(t *testing.T) {
	symbolic := []string{"TARGET_STATE!=ACCEPTED_REALITY"}

	cases := []struct {
		name     string
		reality  func(differenceFixture)
		target   string
		gap      string
		required string
		want     map[string]string
	}{
		{
			name: "facts",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A0", "torque-obs-source", "OBS"))
				f.accept(acceptedProviderFixture("E-00000000000000A2", "torque-vld-source", "VLD"))
			},
			target:   "invoice ledger reconciled",
			gap:      "BRIDGEGAP:facts",
			required: "ESTABLISH_FACT",
			want:     map[string]string{"OBS": torqueStopNoAdapter, "VLD": torqueStopNoAdapter},
		},
		{
			name: "relationships",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A1", "torque-a1-source", "OBS"))
				f.accept(acceptedProviderFixture("E-00000000000000B2", "torque-b2-source", "VLD"))
			},
			target:   "E-00000000000000A1 composes with E-00000000000000B2",
			gap:      "BRIDGEGAP:relationships",
			required: "DERIVE_RELATIONSHIP",
			want:     map[string]string{"CMP": torqueStopNoAdapter, "RLT": torqueStopNoAdapter},
		},
		{
			name: "capabilities",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A0", "torque-cap-source", "OBS"))
			},
			target:   "PROGRAM capability is available",
			gap:      "BRIDGEGAP:capabilities",
			required: "DERIVE_CAPABILITY",
			want: map[string]string{
				"ANALYZE": torqueStopNotDerivable,
				"CMP":     torqueStopNoAdapter,
				"RLT":     torqueStopNoAdapter,
			},
		},
	}

	var report []string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDifferenceFixture(t)
			tc.reality(f)

			boundary := captureDifferenceComparison(t, f,
				fixedDifferenceObserver{result: differenceObserved(symbolic)},
				tc.target)
			if len(boundary.VAL.Gaps) != 1 || boundary.VAL.Gaps[0] != tc.gap {
				t.Fatalf("boundary gaps = %#v want [%q]", boundary.VAL.Gaps, tc.gap)
			}
			if boundary.REL["required_capability"] != tc.required {
				t.Fatalf("required_capability = %q want %q", boundary.REL["required_capability"], tc.required)
			}
			if _, ok := boundary.REL["next_probe"]; ok {
				t.Fatal("boundary predeclared next_probe")
			}

			acceptBoundaryCandidate(t, f, boundary)
			st := f.state()
			if _, ok := st.Accepted[boundary.IDN]; !ok {
				t.Fatal("boundary not REG-accepted")
			}
			if len(st.ActionAuthorizations) != 0 {
				t.Fatal("probe fixture created an authorization")
			}

			got := map[string]string{}
			for _, token := range requiredCapabilityRecipe(tc.required) {
				got[token] = probeRecipeToken(st, boundary.IDN, token)
				report = append(report, tc.gap+" "+tc.required+" "+token+" -> "+got[token])
			}
			for token, want := range tc.want {
				if got[token] != want {
					t.Fatalf("%s %s: got %s want %s (stoppage changed: revisit TORQUE design)",
						tc.gap, token, got[token], want)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("recipe tokens = %#v want %#v", got, tc.want)
			}
		})
	}

	if os.Getenv("UNIFUSION_TORQUE_PROBE") == "1" {
		t.Fatalf("TORQUE B_n -> P_n stoppage: no existing seam derives a bounded probe from the BRIDGEGAP boundary:\n%s",
			strings.Join(report, "\n"))
	}
}

// The model is not a hidden probe source: the comparison never carries a
// next_probe relationship, whatever the observer proposes.
func TestTorqueProbeNotTakenFromModel(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "torque-model-source", "OBS"))

	observed := differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})
	observed.Relationships = map[string]string{"next_probe": "GITHUB:PROGRAM"}
	boundary := captureDifferenceComparison(t, f, fixedDifferenceObserver{result: observed}, "invoice ledger reconciled")

	if _, ok := boundary.REL["next_probe"]; ok {
		t.Fatalf("model next_probe forwarded: %#v", boundary.REL)
	}
}
