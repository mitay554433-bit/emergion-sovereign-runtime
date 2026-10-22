package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

func acceptRealizationIdentityFixture(t *testing.T, s *store.Store, em core.EmergION) {
	t.Helper()
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "test acceptance")
	if err != nil { t.Fatal(err) }
	decisionID, err := s.SaveDecision(decision)
	if err != nil { t.Fatal(err) }
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil { t.Fatal(err) }
	if _, err := s.SaveAccepted(receipt); err != nil { t.Fatal(err) }
}

func captureAcceptedIdentityMember(t *testing.T, r Runtime, name string) core.EmergION {
	t.Helper()
	memberRuntime := r
	memberRuntime.Reasoner = fixedReasoner{
		name: "identity-member",
		version: "v1",
		result: reason.Result{
			Summary: name,
			Relationships: map[string]string{"source_name": name},
			Capabilities: []string{"OBS"},
			Facts: []string{name},
			Risk: "L",
		},
	}
	em, duplicate, err := memberRuntime.CaptureBytes(
		context.Background(), name, []byte(name+" evidence"), "identity_fixture",
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("member unexpectedly duplicate") }
	acceptRealizationIdentityFixture(t, r.Store, em)
	return em
}

func TestCaptureRealizationIdentityStopsAtGOVAndBindsAcceptedSource(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}
	member := captureAcceptedIdentityMember(t, r, "knife-A")

	identity, duplicate, err := r.CaptureRealizationIdentity(
		context.Background(), member.IDN, "KNIFE",
	)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("identity unexpectedly duplicate") }
	if identity.STA != core.StateAtGOV {
		t.Fatalf("identity state = %q want AtGOV", identity.STA)
	}
	if !identity.VAL.Recoil || !identity.VAL.WVC {
		t.Fatal("identity did not cross RECOIL/WVC")
	}
	if identity.REL["realization"] != member.IDN ||
		identity.REL["realization_of"] != "KNIFE" ||
		identity.REL["realization_source_hash"] != member.MEM.SourceHash {
		t.Fatalf("identity relationships = %#v", identity.REL)
	}

	events, err := s.Events()
	if err != nil { t.Fatal(err) }
	st, err := field.Rebuild(events)
	if err != nil { t.Fatal(err) }
	if len(DiscoverRealizationPopulations(st)) != 0 {
		t.Fatal("AtGOV identity must not form canonical population")
	}
}

func TestDiscoverRealizationPopulationsRequiresTwoREGAcceptedIdentities(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s}
	a := captureAcceptedIdentityMember(t, r, "knife-A")
	b := captureAcceptedIdentityMember(t, r, "knife-B")

	idA, _, err := r.CaptureRealizationIdentity(context.Background(), a.IDN, "KNIFE")
	if err != nil { t.Fatal(err) }
	acceptRealizationIdentityFixture(t, s, idA)

	events, err := s.Events()
	if err != nil { t.Fatal(err) }
	st, err := field.Rebuild(events)
	if err != nil { t.Fatal(err) }
	if len(DiscoverRealizationPopulations(st)) != 0 {
		t.Fatal("single accepted identity must not form population")
	}

	idB, _, err := r.CaptureRealizationIdentity(context.Background(), b.IDN, "KNIFE")
	if err != nil { t.Fatal(err) }
	acceptRealizationIdentityFixture(t, s, idB)

	events, err = s.Events()
	if err != nil { t.Fatal(err) }
	st, err = field.Rebuild(events)
	if err != nil { t.Fatal(err) }
	populations := DiscoverRealizationPopulations(st)
	if len(populations) != 1 {
		t.Fatalf("populations = %d want 1", len(populations))
	}
	population := populations[0]
	if population.TargetThing != "KNIFE" ||
		len(population.Members) != 2 ||
		population.Members[0] != a.IDN ||
		population.Members[1] != b.IDN {
		t.Fatalf("population = %#v", population)
	}
	if len(population.Comparisons) != 0 {
		t.Fatal("identity discovery must not invent comparison evidence")
	}
}

func TestCaptureRealizationIdentityRejectsNonREGRealization(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil { t.Fatal(err) }
	r := Runtime{Store: s, Reasoner: fixedReasoner{
		name: "identity-member", version: "v1",
		result: reason.Result{
			Summary: "pending member",
			Relationships: map[string]string{"source_name": "pending"},
			Capabilities: []string{"OBS"},
			Facts: []string{"pending"},
			Risk: "L",
		},
	}}
	member, _, err := r.CaptureBytes(
		context.Background(), "pending", []byte("pending evidence"), "identity_fixture",
	)
	if err != nil { t.Fatal(err) }

	if _, _, err := r.CaptureRealizationIdentity(
		context.Background(), member.IDN, "KNIFE",
	); err == nil {
		t.Fatal("non-REG realization identity was accepted")
	}
}
