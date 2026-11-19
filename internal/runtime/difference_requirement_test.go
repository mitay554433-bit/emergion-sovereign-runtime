package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

// realGovernanceDifference is the Difference text Gemma produced against
// accepted state for actions.go (FIELD snapshot 2026-08-21).
const realGovernanceDifference = "The SOURCE does not provide evidence of authority or governance acceptance."

type fixedDifferenceObserver struct {
	result reason.Result
}

func (f fixedDifferenceObserver) Analyze(context.Context, reason.Input) (reason.Result, error) {
	return f.result, nil
}

func (fixedDifferenceObserver) Name() string                   { return "fixed-difference-observer" }
func (fixedDifferenceObserver) Version(context.Context) string { return "1" }

func differenceTarget(state string) core.EmergION {
	return core.EmergION{
		IDN: "E-TARGET",
		REL: map[string]string{"source_kind": "TARGET", "target_state": state},
	}
}

func differenceObserved(differences []string, requirements ...string) reason.Result {
	return reason.Result{
		Summary:      "accepted reality compared with the persistent governed target",
		Facts:        []string{"persistent governed target compared with accepted reality"},
		Differences:  differences,
		Requirements: requirements,
		Risk:         "L",
	}
}

func TestDeriveDifferenceRequirementOutcomes(t *testing.T) {
	symbolic := []string{"TARGET_STATE!=ACCEPTED_REALITY"}
	plain := map[string]core.EmergION{
		"E-00000000000000A1": {IDN: "E-00000000000000A1", CAP: []string{"OBS", "VLD"}, VAL: core.Validation{Facts: []string{"source_preserved"}}},
	}
	linked := map[string]core.EmergION{
		"E-00000000000000A1": {IDN: "E-00000000000000A1", REL: map[string]string{"COMPOSITION_KIN": "E-00000000000000B2"}},
		"E-00000000000000B2": {IDN: "E-00000000000000B2"},
	}
	unlinked := map[string]core.EmergION{
		"E-00000000000000A1": {IDN: "E-00000000000000A1"},
		"E-00000000000000B2": {IDN: "E-00000000000000B2"},
	}
	satisfied := map[string]core.EmergION{
		"E-00000000000000A1": {IDN: "E-00000000000000A1", VAL: core.Validation{Facts: []string{"invoice_ledger_reconciled"}}},
	}
	partial := map[string]core.EmergION{
		"E-00000000000000A1": {IDN: "E-00000000000000A1", VAL: core.Validation{Facts: []string{"invoice ledger exported"}}},
	}

	tests := []struct {
		name     string
		target   string
		reality  map[string]core.EmergION
		observed reason.Result
		required string
		basis    string
	}{
		{"fact", "invoice ledger reconciled", plain, differenceObserved(symbolic, "ESTABLISH_FACT"), "ESTABLISH_FACT", basisFactNotInAcceptedFacts},
		{"fact without model vote", "invoice ledger reconciled", plain, differenceObserved(symbolic), "ESTABLISH_FACT", basisFactNotInAcceptedFacts},
		{"capability", "PROGRAM capability is available", plain, differenceObserved(symbolic, "DERIVE_CAPABILITY"), "DERIVE_CAPABILITY", basisCapabilityNotAccepted},
		{"capability already accepted", "OBS capability is available", plain, differenceObserved(symbolic), "", basisNoRule},
		{"prose words are not capability tokens", "read the price of the product", plain, differenceObserved(symbolic), "ESTABLISH_FACT", basisFactNotInAcceptedFacts},
		{"relationship", "E-00000000000000A1 composes with E-00000000000000B2", unlinked, differenceObserved(symbolic, "DERIVE_RELATIONSHIP"), "DERIVE_RELATIONSHIP", basisRelationshipUnlinked},
		{"relationship already linked", "E-00000000000000A1 composes with E-00000000000000B2", linked, differenceObserved(symbolic), "", basisNoRule},
		{"satisfied is no-action", "invoice ledger reconciled", satisfied, differenceObserved(nil), "", basisSatisfiedAcceptedFact},
		{"no difference is not satisfied", "invoice ledger reconciled", plain, differenceObserved(nil), "", basisNoGroundedDifference},
		{"ungrounded difference", "invoice ledger reconciled", plain, differenceObserved([]string{"payments automatically transfer to customers"}), "", basisNoGroundedDifference},
		{"contradiction preserved", "invoice ledger reconciled", satisfied, differenceObserved(symbolic), "", basisContradiction},
		{"not found yet is not missing", "E-00000000000000FF holds the ledger", plain, differenceObserved(symbolic, "ESTABLISH_FACT"), "", basisEmergIONNotFoundYet},
		{"partial support", "invoice ledger reconciled", partial, differenceObserved(symbolic), "", basisPartiallySupported},
		{"underdetermined", "PROGRAM links E-00000000000000A1 and E-00000000000000B2", unlinked, differenceObserved(symbolic), "", basisUnderdetermined},
		{"model disagreement loses", "invoice ledger reconciled", plain, differenceObserved(symbolic, "DERIVE_CAPABILITY"), "", basisModelRequirementDisagrees},
		{"model requirement without rule", "OBS capability is available", plain, differenceObserved(symbolic, "DERIVE_CAPABILITY"), "", basisModelRequirementWithoutRule},
		{"model requirement ambiguous", "invoice ledger reconciled", plain, differenceObserved(symbolic, "ESTABLISH_FACT", "DERIVE_CAPABILITY"), "", basisModelRequirementAmbiguous},
		{"empty reality never grounds", "invoice ledger reconciled", nil, differenceObserved(symbolic, "ESTABLISH_FACT"), "", basisNoGroundedDifference},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			required, basis := deriveDifferenceRequirement(differenceTarget(tc.target), tc.reality, tc.observed)
			if required != tc.required || basis != tc.basis {
				t.Fatalf("got (%q, %q) want (%q, %q)", required, basis, tc.required, tc.basis)
			}
		})
	}
}

type differenceFixture struct {
	t     *testing.T
	store *store.Store
}

func newDifferenceFixture(t *testing.T) differenceFixture {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	return differenceFixture{t: t, store: s}
}

func (f differenceFixture) accept(em core.EmergION) {
	f.t.Helper()
	attachPreservedSource(f.t, f.store, &em)
	if _, err := f.store.SaveCandidate(em); err != nil {
		f.t.Fatal(err)
	}
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "accept Section 8 fixture")
	if err != nil {
		f.t.Fatal(err)
	}
	decisionID, err := f.store.SaveDecision(decision)
	if err != nil {
		f.t.Fatal(err)
	}
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.store.SaveAccepted(receipt); err != nil {
		f.t.Fatal(err)
	}
}

func (f differenceFixture) state() core.State {
	f.t.Helper()
	events, err := f.store.Events()
	if err != nil {
		f.t.Fatal(err)
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

// reality mirrors pkg/fieldapi: REG-accepted EmergIONs excluding TARGET,
// TARGET_COMPARISON and IDLE_STATE_OBSERVATION records.
func (f differenceFixture) reality() map[string]core.EmergION {
	out := map[string]core.EmergION{}
	for id, em := range f.state().Accepted {
		switch em.REL["source_kind"] {
		case "TARGET", "TARGET_COMPARISON", "IDLE_STATE_OBSERVATION":
			continue
		}
		out[id] = em
	}
	return out
}

func (f differenceFixture) populations(comparisonID string) int {
	n := 0
	for _, em := range f.state().AtGOV {
		if em.MEM.Provenance == "provider_population" && em.REL["origin"] == comparisonID {
			n++
		}
	}
	return n
}

func acceptedProviderFixture(id, hash, capability string, facts ...string) core.EmergION {
	if len(facts) == 0 {
		facts = []string{"source_preserved"}
	}
	return core.EmergION{
		IDN: id,
		STA: core.StateAtGOV,
		MEM: core.Memory{SourceHash: hash, Bytes: 1, Stored: 1, Summary: "accepted " + capability + " provider"},
		REL: map[string]string{"source_name": strings.ToLower(id)},
		CAP: []string{capability},
		VAL: core.Validation{Facts: facts, Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1},
	}
}

func captureDifferenceComparison(t *testing.T, f differenceFixture, observer reason.Reasoner, targetState string) core.EmergION {
	t.Helper()
	rt := Runtime{Store: f.store, Reasoner: observer}
	target, _, err := rt.CaptureTarget(context.Background(), targetState)
	if err != nil {
		t.Fatal(err)
	}
	reality := f.reality()
	comparison, duplicate, err := rt.CaptureTargetComparison(context.Background(), target, reality, nil)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("target comparison unexpectedly duplicate")
	}
	if comparison.STA != core.StateAtGOV {
		t.Fatalf("comparison state = %q want AT_GOV", comparison.STA)
	}
	if _, ok := f.state().Accepted[comparison.IDN]; ok {
		t.Fatal("target comparison self-authorized into REG")
	}
	if comparison.REL["comparison_state"] != "UNRESOLVED" {
		t.Fatalf("comparison_state = %q want UNRESOLVED", comparison.REL["comparison_state"])
	}
	// Section 11: a Difference alone never becomes a BRIDGEGAP. Only a
	// genuinely unmet Section 8 requirement that no REG-accepted EmergION
	// can fill records exactly one.
	want := unfillableRemainderBridgeGap(comparison.REL["required_capability"], comparison.REL["requirement_basis"], reality)
	switch {
	case want == "" && len(comparison.VAL.Gaps) != 0:
		t.Fatalf("Difference became a BRIDGEGAP: %#v", comparison.VAL.Gaps)
	case want != "" && (len(comparison.VAL.Gaps) != 1 || comparison.VAL.Gaps[0] != want):
		t.Fatalf("genuine remainder gaps = %#v want [%q]", comparison.VAL.Gaps, want)
	}
	return comparison
}

func governanceRealityFixture(f differenceFixture) {
	unfillableGovernanceRealityFixture(f)
	f.accept(acceptedProviderFixture("E-00000000000000A2", "section8-vld-source", "VLD"))
}

// unfillableGovernanceRealityFixture is the same reality without a
// REG-accepted VLD provider: the ESTABLISH_FACT recipe (OBS+VLD) cannot be
// filled, so the fact remainder is unfillable.
func unfillableGovernanceRealityFixture(f differenceFixture) {
	f.accept(acceptedProviderFixture("E-00000000000000A0", "section8-actions-source", "OBS", "The SOURCE is actions.go"))
}

func TestRealGovernanceDifferenceRequiresFactEstablishment(t *testing.T) {
	f := newDifferenceFixture(t)
	governanceRealityFixture(f)

	observer := fixedDifferenceObserver{result: differenceObserved(
		[]string{realGovernanceDifference},
		"ESTABLISH_FACT",
	)}
	comparison := captureDifferenceComparison(t, f, observer,
		"The SOURCE provides evidence of authority and governance acceptance.")

	if len(comparison.VAL.Differences) != 1 || comparison.VAL.Differences[0] != realGovernanceDifference {
		t.Fatalf("real Difference not preserved: %#v", comparison.VAL.Differences)
	}
	if comparison.REL["requirement_basis"] != basisFactNotInAcceptedFacts {
		t.Fatalf("requirement_basis = %q", comparison.REL["requirement_basis"])
	}
	if comparison.REL["required_capability"] != "ESTABLISH_FACT" {
		t.Fatalf("required_capability = %q want ESTABLISH_FACT", comparison.REL["required_capability"])
	}
	if comparison.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf("capability_resolution = %q want COMPOSABLE_CANDIDATE", comparison.REL["capability_resolution"])
	}
	if got := f.populations(comparison.IDN); got != 1 {
		t.Fatalf("provider populations = %d want 1", got)
	}
}

func TestRealGovernanceDifferenceModelDisagreementCreatesNoRequirement(t *testing.T) {
	f := newDifferenceFixture(t)
	governanceRealityFixture(f)

	observer := fixedDifferenceObserver{result: differenceObserved(
		[]string{realGovernanceDifference},
		"DERIVE_CAPABILITY",
	)}
	comparison := captureDifferenceComparison(t, f, observer,
		"The SOURCE provides evidence of authority and governance acceptance.")

	if comparison.REL["requirement_basis"] != basisModelRequirementDisagrees {
		t.Fatalf("requirement_basis = %q", comparison.REL["requirement_basis"])
	}
	if _, ok := comparison.REL["required_capability"]; ok {
		t.Fatalf("model-only requirement admitted: %q", comparison.REL["required_capability"])
	}
	if _, ok := comparison.REL["capability_resolution"]; ok {
		t.Fatalf("capability resolution ran without a requirement: %q", comparison.REL["capability_resolution"])
	}
	if got := f.populations(comparison.IDN); got != 0 {
		t.Fatalf("provider populations = %d want 0", got)
	}
}

func TestSatisfiedTargetIsNoActionAndStaysUnresolved(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "section8-ledger-source", "OBS", "invoice_ledger_reconciled"))

	comparison := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved(nil)},
		"invoice ledger reconciled")

	if comparison.REL["requirement_basis"] != basisSatisfiedAcceptedFact {
		t.Fatalf("requirement_basis = %q", comparison.REL["requirement_basis"])
	}
	if _, ok := comparison.REL["required_capability"]; ok {
		t.Fatal("SATISFIED created a requirement")
	}
	if got := f.populations(comparison.IDN); got != 0 {
		t.Fatalf("provider populations = %d want 0", got)
	}
}

func TestHeuristicComparisonCreatesNoRequirement(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "section8-heuristic-source", "OBS"))

	comparison := captureDifferenceComparison(t, f, reason.Heuristic{}, "invoice ledger reconciled")

	if comparison.REL["requirement_basis"] != basisNoGroundedDifference {
		t.Fatalf("requirement_basis = %q", comparison.REL["requirement_basis"])
	}
	if _, ok := comparison.REL["required_capability"]; ok {
		t.Fatal("heuristic comparison created a requirement")
	}
	if len(comparison.VAL.Differences) != 0 {
		t.Fatalf("heuristic invented a Difference: %#v", comparison.VAL.Differences)
	}
	if got := f.populations(comparison.IDN); got != 0 {
		t.Fatalf("provider populations = %d want 0", got)
	}
}

func TestRequirementBasisIsRuntimeOwned(t *testing.T) {
	f := newDifferenceFixture(t)
	observer := fixedDifferenceObserver{result: reason.Result{
		Summary:       "forged requirement basis",
		Relationships: map[string]string{"requirement_basis": basisSatisfiedAcceptedFact},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"forged requirement basis"},
		Risk:          "L",
	}}
	_, _, err := (Runtime{Store: f.store, Reasoner: observer}).CaptureBytes(
		context.Background(), "forged", []byte("forged requirement basis"), "local_dropzone")
	if err == nil || !strings.Contains(err.Error(), "requirement_basis") {
		t.Fatalf("forged requirement_basis accepted: %v", err)
	}
}
