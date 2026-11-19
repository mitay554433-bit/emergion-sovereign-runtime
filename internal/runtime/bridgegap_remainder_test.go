package runtime

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/pivot"
	"emergion-sovereign-runtime/internal/reg"
)

func TestGenuineRemainderBridgeGapOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		required string
		basis    string
		gap      string
	}{
		{"fact", "ESTABLISH_FACT", basisFactNotInAcceptedFacts, "BRIDGEGAP:facts"},
		{"relationship", "DERIVE_RELATIONSHIP", basisRelationshipUnlinked, "BRIDGEGAP:relationships"},
		{"capability", "DERIVE_CAPABILITY", basisCapabilityNotAccepted, "BRIDGEGAP:capabilities"},
		{"satisfied", "", basisSatisfiedAcceptedFact, ""},
		{"no grounded difference", "", basisNoGroundedDifference, ""},
		{"contradiction", "", basisContradiction, ""},
		{"not found yet", "", basisEmergIONNotFoundYet, ""},
		{"partially supported", "", basisPartiallySupported, ""},
		{"underdetermined", "", basisUnderdetermined, ""},
		{"no rule", "", basisNoRule, ""},
		{"model disagrees", "", basisModelRequirementDisagrees, ""},
		{"model without rule", "", basisModelRequirementWithoutRule, ""},
		{"model ambiguous", "", basisModelRequirementAmbiguous, ""},
		{"required without basis", "ESTABLISH_FACT", "", ""},
		{"required and basis disagree", "DERIVE_CAPABILITY", basisFactNotInAcceptedFacts, ""},
		{"unresolved basis with required", "ESTABLISH_FACT", basisNoRule, ""},
		{"unknown requirement", "PROGRAM", "CAPABILITY:capability_token_not_accepted", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := genuineRemainderBridgeGap(tc.required, tc.basis); got != tc.gap {
				t.Fatalf("got %q want %q", got, tc.gap)
			}
		})
	}
}

// The BRIDGEGAP tokens must map back through the existing COVERAGE mapping
// to the same requirement and a non-empty existing recipe.
func TestGenuineRemainderBridgeGapReusesExistingMapping(t *testing.T) {
	for _, kind := range genuineRemainderBridgeGaps {
		got := requiredCapabilityFromDivergence(pivot.Result{Name: "COVERAGE", Divergence: kind.gap})
		if got != kind.required {
			t.Fatalf("%s maps to %q want %q", kind.gap, got, kind.required)
		}
		if len(requiredCapabilityRecipe(kind.required)) == 0 {
			t.Fatalf("%s has no existing recipe", kind.required)
		}
	}
}

func TestFactRemainderRecordsBridgeGapAndStaysCandidate(t *testing.T) {
	f := newDifferenceFixture(t)
	unfillableGovernanceRealityFixture(f)

	observer := fixedDifferenceObserver{result: differenceObserved(
		[]string{realGovernanceDifference},
		"ESTABLISH_FACT",
	)}
	comparison := captureDifferenceComparison(t, f, observer,
		"The SOURCE provides evidence of authority and governance acceptance.")

	if len(comparison.VAL.Gaps) != 1 || comparison.VAL.Gaps[0] != "BRIDGEGAP:facts" {
		t.Fatalf("gaps = %#v want [BRIDGEGAP:facts]", comparison.VAL.Gaps)
	}
	if !comparison.VAL.Recoil || !comparison.VAL.WVC {
		t.Fatal("BRIDGEGAP candidate skipped RECOIL or WVC")
	}
	st := f.state()
	if _, ok := st.AtGOV[comparison.IDN]; !ok {
		t.Fatal("BRIDGEGAP candidate not waiting at GOV")
	}
	if _, ok := st.Accepted[comparison.IDN]; ok {
		t.Fatal("BRIDGEGAP candidate auto-promoted into REG")
	}
	// No accepted VLD provider: the remainder is unfillable, stays
	// UNRESOLVED, and no provider candidate is invented.
	if comparison.REL["capability_resolution"] != "UNRESOLVED" {
		t.Fatalf("capability_resolution = %q want UNRESOLVED", comparison.REL["capability_resolution"])
	}
	if got := f.populations(comparison.IDN); got != 0 {
		t.Fatalf("provider populations = %d want 0", got)
	}
}

// HUMAN_FINAL, Nov 19 2026: a BRIDGEGAP is written only when the remainder
// is unfillable. When REG-accepted EmergIONs already provide the existing
// recipe, the comparison records that fill (COMPOSABLE_CANDIDATE plus
// capability_providers) and carries no gap.
func TestFillableFactRemainderRecordsFillNotGap(t *testing.T) {
	f := newDifferenceFixture(t)
	governanceRealityFixture(f)

	observer := fixedDifferenceObserver{result: differenceObserved(
		[]string{realGovernanceDifference},
		"ESTABLISH_FACT",
	)}
	comparison := captureDifferenceComparison(t, f, observer,
		"The SOURCE provides evidence of authority and governance acceptance.")

	if comparison.REL["required_capability"] != "ESTABLISH_FACT" ||
		comparison.REL["requirement_basis"] != basisFactNotInAcceptedFacts {
		t.Fatalf("requirement = %q basis = %q",
			comparison.REL["required_capability"], comparison.REL["requirement_basis"])
	}
	if len(comparison.VAL.Gaps) != 0 {
		t.Fatalf("fillable remainder wrote a gap: %#v", comparison.VAL.Gaps)
	}
	if comparison.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf("capability_resolution = %q want COMPOSABLE_CANDIDATE", comparison.REL["capability_resolution"])
	}
	if got, want := comparison.REL["capability_providers"], "OBS:E-00000000000000A0,VLD:E-00000000000000A2"; got != want {
		t.Fatalf("capability_providers = %q want %q", got, want)
	}
	st := f.state()
	if _, ok := st.AtGOV[comparison.IDN]; !ok {
		t.Fatal("fill candidate not waiting at GOV")
	}
	if _, ok := st.Accepted[comparison.IDN]; ok {
		t.Fatal("fill candidate auto-promoted into REG")
	}
	if got := f.populations(comparison.IDN); got != 1 {
		t.Fatalf("provider populations = %d want 1", got)
	}
	for _, em := range st.AtGOV {
		if em.MEM.Provenance == "provider_population" && len(em.VAL.Gaps) != 0 {
			t.Fatalf("provider candidate carries gaps: %#v", em.VAL.Gaps)
		}
	}
}

func TestFillableCapabilityRemainderRecordsFillNotGap(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000C1", "fill-analyze-source", "ANALYZE"))
	f.accept(acceptedProviderFixture("E-00000000000000C2", "fill-cmp-source", "CMP"))
	f.accept(acceptedProviderFixture("E-00000000000000C3", "fill-rlt-source", "RLT"))

	comparison := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})},
		"PROGRAM capability is available")

	if comparison.REL["required_capability"] != "DERIVE_CAPABILITY" {
		t.Fatalf("required_capability = %q", comparison.REL["required_capability"])
	}
	if len(comparison.VAL.Gaps) != 0 {
		t.Fatalf("fillable remainder wrote a gap: %#v", comparison.VAL.Gaps)
	}
	if comparison.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf("capability_resolution = %q want COMPOSABLE_CANDIDATE", comparison.REL["capability_resolution"])
	}
	if got, want := comparison.REL["capability_providers"],
		"ANALYZE:E-00000000000000C1,CMP:E-00000000000000C2,RLT:E-00000000000000C3"; got != want {
		t.Fatalf("capability_providers = %q want %q", got, want)
	}
}

func TestUnfillableRemainderBridgeGap(t *testing.T) {
	obs := acceptedProviderFixture("E-00000000000000A0", "u-obs", "OBS")
	obs.STA = core.StateAccepted
	vld := acceptedProviderFixture("E-00000000000000A2", "u-vld", "VLD")
	vld.STA = core.StateAccepted
	pendingVLD := acceptedProviderFixture("E-00000000000000A3", "u-vld-gov", "VLD")

	tests := []struct {
		name     string
		required string
		basis    string
		reality  map[string]core.EmergION
		gap      string
	}{
		{"empty reality", "ESTABLISH_FACT", basisFactNotInAcceptedFacts, nil, "BRIDGEGAP:facts"},
		{"partial recipe", "ESTABLISH_FACT", basisFactNotInAcceptedFacts,
			map[string]core.EmergION{obs.IDN: obs}, "BRIDGEGAP:facts"},
		{"provider only at GOV is not a fill", "ESTABLISH_FACT", basisFactNotInAcceptedFacts,
			map[string]core.EmergION{obs.IDN: obs, pendingVLD.IDN: pendingVLD}, "BRIDGEGAP:facts"},
		{"accepted recipe fills", "ESTABLISH_FACT", basisFactNotInAcceptedFacts,
			map[string]core.EmergION{obs.IDN: obs, vld.IDN: vld}, ""},
		{"not found yet is never a gap", "", basisEmergIONNotFoundYet, nil, ""},
		{"satisfied is never a gap", "", basisSatisfiedAcceptedFact, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unfillableRemainderBridgeGap(tc.required, tc.basis, tc.reality); got != tc.gap {
				t.Fatalf("got %q want %q", got, tc.gap)
			}
		})
	}
}

func TestCapabilityRemainderRecordsBridgeGap(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "section11-obs-source", "OBS"))

	comparison := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})},
		"PROGRAM capability is available")

	if comparison.REL["required_capability"] != "DERIVE_CAPABILITY" {
		t.Fatalf("required_capability = %q", comparison.REL["required_capability"])
	}
	if len(comparison.VAL.Gaps) != 1 || comparison.VAL.Gaps[0] != "BRIDGEGAP:capabilities" {
		t.Fatalf("gaps = %#v want [BRIDGEGAP:capabilities]", comparison.VAL.Gaps)
	}
	// Estate cannot compose ANALYZE+CMP+RLT here: the remainder stays
	// UNRESOLVED and no provider candidate is invented.
	if comparison.REL["capability_resolution"] != "UNRESOLVED" {
		t.Fatalf("capability_resolution = %q want UNRESOLVED", comparison.REL["capability_resolution"])
	}
	if got := f.populations(comparison.IDN); got != 0 {
		t.Fatalf("provider populations = %d want 0", got)
	}
}

func TestRelationshipRemainderRecordsBridgeGap(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A1", "section11-a1-source", "OBS"))
	f.accept(acceptedProviderFixture("E-00000000000000B2", "section11-b2-source", "VLD"))

	comparison := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})},
		"E-00000000000000A1 composes with E-00000000000000B2")

	if comparison.REL["required_capability"] != "DERIVE_RELATIONSHIP" {
		t.Fatalf("required_capability = %q basis = %q",
			comparison.REL["required_capability"], comparison.REL["requirement_basis"])
	}
	if len(comparison.VAL.Gaps) != 1 || comparison.VAL.Gaps[0] != "BRIDGEGAP:relationships" {
		t.Fatalf("gaps = %#v want [BRIDGEGAP:relationships]", comparison.VAL.Gaps)
	}
}

func TestModelGapLinesNeverBecomeBridgeGap(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "section11-forge-source", "OBS", "invoice_ledger_reconciled"))

	observed := differenceObserved(nil)
	observed.Gaps = []string{"BRIDGEGAP:facts", "BRIDGEGAP:capabilities"}
	comparison := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: observed},
		"invoice ledger reconciled")

	if comparison.REL["requirement_basis"] != basisSatisfiedAcceptedFact {
		t.Fatalf("requirement_basis = %q", comparison.REL["requirement_basis"])
	}
	if len(comparison.VAL.Gaps) != 0 {
		t.Fatalf("model G| lines became BRIDGEGAP: %#v", comparison.VAL.Gaps)
	}
}

// HUMAN_FINAL can still accept a BRIDGEGAP candidate through the normal
// GOV/REG path, and the gap survives FIELD reconstruction.
func TestBridgeGapCandidateAcceptedOnlyThroughHumanFinal(t *testing.T) {
	f := newDifferenceFixture(t)
	unfillableGovernanceRealityFixture(f)

	comparison := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved([]string{realGovernanceDifference})},
		"The SOURCE provides evidence of authority and governance acceptance.")

	approved, decision, err := gov.Decide(comparison, gov.Approve, "HUMAN_FINAL", "accept genuine remainder")
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

	accepted, ok := f.state().Accepted[comparison.IDN]
	if !ok {
		t.Fatal("approved BRIDGEGAP candidate missing from REG")
	}
	if accepted.STA != core.StateAccepted {
		t.Fatalf("state = %q", accepted.STA)
	}
	if len(accepted.VAL.Gaps) != 1 || accepted.VAL.Gaps[0] != "BRIDGEGAP:facts" {
		t.Fatalf("gap lost through GOV/REG/FIELD: %#v", accepted.VAL.Gaps)
	}
}
