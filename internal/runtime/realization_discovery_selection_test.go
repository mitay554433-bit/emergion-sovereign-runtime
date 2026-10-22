package runtime

import (
	"context"
	"path/filepath"
	"testing"

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
	s *store.Store,
	parent string,
	succeeded bool,
) {
	t.Helper()
	fact := "execution_failed"
	if succeeded { fact = "execution_succeeded" }
	em := core.EmergION{
		IDN: "R-" + parent,
		STA: core.StateAtGOV,
		REL: map[string]string{
			"source_kind": "EXECUTION_RESULT",
			"parent_emergion": parent,
			"adapter": "LOCAL_GEMMA",
			"action": "ANALYZE",
		},
		VAL: core.Validation{Facts: []string{fact}},
	}
	if _, err := s.SaveCandidate(em); err != nil { t.Fatal(err) }
	acceptRealizationIdentityFixture(t, s, em)
}

func TestCaptureNextDiscoveredRealizationSelectionAdmitsUniqueStrongestAtGOV(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}
	a := captureAcceptedIdentityMember(t, r, "knife-A")
	b := captureAcceptedIdentityMember(t, r, "knife-B")
	acceptedDiscoveryIdentity(t, r, a, "KNIFE")
	acceptedDiscoveryIdentity(t, r, b, "KNIFE")
	addAcceptedDiscoveryOutcome(t, s, a.IDN, false)
	addAcceptedDiscoveryOutcome(t, s, b.IDN, true)

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
	addAcceptedDiscoveryOutcome(t, s, a.IDN, true)
	addAcceptedDiscoveryOutcome(t, s, b.IDN, true)

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
	addAcceptedDiscoveryOutcome(t, s, a.IDN, false)
	addAcceptedDiscoveryOutcome(t, s, b.IDN, true)

	first, proposed, err := r.CaptureNextDiscoveredRealizationSelection(context.Background())
	if err != nil || !proposed { t.Fatalf("first selection proposed=%t err=%v", proposed, err) }

	second, proposed, err := r.CaptureNextDiscoveredRealizationSelection(context.Background())
	if err != nil { t.Fatal(err) }
	if !proposed || second.IDN != first.IDN || second.STA != core.StateAtGOV {
		t.Fatalf("pending duplicate = %#v proposed=%t", second, proposed)
	}
}
