package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/store"
)

func acceptedDiscoveryIdentity(t *testing.T, r Runtime, member core.EmergION, thing string) {
	t.Helper()
	identity, duplicate, err := r.CaptureRealizationIdentity(
		context.Background(), member.IDN, thing,
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("identity unexpectedly duplicate") }
	acceptRealizationIdentityFixture(t, r.Store, identity)
}

func addAcceptedDiscoveryOutcome(
	t *testing.T,
	r Runtime,
	kinID string,
	succeeded bool,
) {
	t.Helper()
	request := adapters.ExecutionRequest{
		EmergIONID: kinID,
		Adapter: "LOCAL_GEMMA",
		Action: "ANALYZE",
	}
	result := adapters.ExecutionResult{
		Adapter: "LOCAL_GEMMA",
		Action: "ANALYZE",
		Succeeded: succeeded,
	}
	em, duplicate, err := r.CaptureExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("execution result unexpectedly duplicate") }
	acceptRealizationIdentityFixture(t, r.Store, em)
}

func TestCaptureNextDiscoveredRealizationSelectionAdmitsUniqueStrongestAtGOV(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}
	a := captureAcceptedIdentityMember(t, r, "knife-A")
	b := captureAcceptedIdentityMember(t, r, "knife-B")
	acceptedDiscoveryIdentity(t, r, a, "KNIFE")
	acceptedDiscoveryIdentity(t, r, b, "KNIFE")
	addAcceptedDiscoveryOutcome(t, r, a.IDN, false)
	addAcceptedDiscoveryOutcome(t, r, b.IDN, true)

	selection, proposed, err := r.CaptureNextDiscoveredRealizationSelection(context.Background())
	if err != nil { t.Fatal(err) }
	if !proposed { t.Fatal("unique strongest did not produce governed selection") }
	if selection.STA != core.StateAtGOV {
		t.Fatalf("selection state = %q want AtGOV", selection.STA)
	}
	if selection.REL["source_kind"] != "REALIZATION_SELECTION" ||
		selection.REL["target_thing"] != "KNIFE" ||
		selection.REL["selected_realization"] != b.IDN {
		t.Fatalf("selection relationships = %#v", selection.REL)
	}

	events, err := s.Events()
	if err != nil { t.Fatal(err) }
	st, err := field.Rebuild(events)
	if err != nil { t.Fatal(err) }
	if _, ok := st.Accepted[selection.IDN]; ok {
		t.Fatal("discovered selection bypassed HUMAN_FINAL/REG")
	}
	if _, ok := st.AtGOV[selection.IDN]; !ok {
		t.Fatal("discovered selection missing from AtGOV")
	}
}

func TestCaptureNextDiscoveredRealizationSelectionSkipsUnresolvedPopulation(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}
	a := captureAcceptedIdentityMember(t, r, "knife-A")
	b := captureAcceptedIdentityMember(t, r, "knife-B")
	acceptedDiscoveryIdentity(t, r, a, "KNIFE")
	acceptedDiscoveryIdentity(t, r, b, "KNIFE")
	addAcceptedDiscoveryOutcome(t, r, a.IDN, true)
	addAcceptedDiscoveryOutcome(t, r, b.IDN, true)

	if selection, proposed, err := r.CaptureNextDiscoveredRealizationSelection(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	} else if proposed || selection.IDN != "" {
		t.Fatalf("unresolved population produced selection %#v", selection)
	}
}

func TestCaptureNextDiscoveredRealizationSelectionKeepsDuplicatePendingAtGOV(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}
	a := captureAcceptedIdentityMember(t, r, "knife-A")
	b := captureAcceptedIdentityMember(t, r, "knife-B")
	acceptedDiscoveryIdentity(t, r, a, "KNIFE")
	acceptedDiscoveryIdentity(t, r, b, "KNIFE")
	addAcceptedDiscoveryOutcome(t, r, a.IDN, false)
	addAcceptedDiscoveryOutcome(t, r, b.IDN, true)

	first, proposed, err := r.CaptureNextDiscoveredRealizationSelection(context.Background())
	if err != nil || !proposed { t.Fatalf("first selection proposed=%t err=%v", proposed, err) }

	second, proposed, err := r.CaptureNextDiscoveredRealizationSelection(context.Background())
	if err != nil { t.Fatal(err) }
	if !proposed || second.IDN != first.IDN || second.STA != core.StateAtGOV {
		t.Fatalf("pending duplicate = %#v proposed=%t", second, proposed)
	}
}
