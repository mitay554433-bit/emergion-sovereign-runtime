package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/cosl"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

// HUMAN_FINAL, Nov 19 2026 (TR-042): originated_from_question is an
// additive, optional REL key. New captures that descend from a TARGET carry
// it; everything else, and every older record, does not.

func TestOriginatedFromQuestionRoundTrip(t *testing.T) {
	f := newDifferenceFixture(t)
	governanceRealityFixture(f)

	rt := Runtime{Store: f.store, Reasoner: fixedDifferenceObserver{result: differenceObserved(
		[]string{realGovernanceDifference}, "ESTABLISH_FACT")}}
	target, _, err := rt.CaptureTarget(context.Background(),
		"The SOURCE provides evidence of authority and governance acceptance.")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := target.REL[originatedFromQuestionKey]; ok {
		t.Fatalf("TARGET carries %s: it is the question itself", originatedFromQuestionKey)
	}
	comparison, _, err := rt.CaptureTargetComparison(context.Background(), target, f.reality(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := comparison.REL[originatedFromQuestionKey]; got != target.IDN {
		t.Fatalf("comparison %s = %q want %q", originatedFromQuestionKey, got, target.IDN)
	}

	// The provider population candidate is KIN to the comparison (origin)
	// and so carries the same question, one hop.
	st := f.state()
	populations := 0
	for _, em := range st.AtGOV {
		if em.MEM.Provenance != "provider_population" || em.REL["origin"] != comparison.IDN {
			continue
		}
		populations++
		if got := em.REL[originatedFromQuestionKey]; got != target.IDN {
			t.Fatalf("provider candidate %s = %q want %q", originatedFromQuestionKey, got, target.IDN)
		}
	}
	if populations != 1 {
		t.Fatalf("provider populations = %d want 1", populations)
	}

	// Round trip through COSL and a fresh store handle (restart).
	restarted, err := store.Open(f.store.Root)
	if err != nil {
		t.Fatal(err)
	}
	events, err := restarted.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		line, err := cosl.Encode(ev)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := cosl.Decode(line)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.SelfHash != ev.SelfHash {
			t.Fatalf("event %s hash changed in round trip", ev.ID)
		}
		if ev.EmergION != nil && decoded.EmergION.REL[originatedFromQuestionKey] != ev.EmergION.REL[originatedFromQuestionKey] {
			t.Fatalf("event %s lost %s", ev.ID, originatedFromQuestionKey)
		}
	}
	if got := rebuildState(t, restarted).AtGOV[comparison.IDN].REL[originatedFromQuestionKey]; got != target.IDN {
		t.Fatalf("rebuilt %s = %q want %q", originatedFromQuestionKey, got, target.IDN)
	}
}

func TestOriginatedFromQuestionAbsentWithoutQuestionLineage(t *testing.T) {
	f := newDifferenceFixture(t)
	rt := Runtime{Store: f.store, Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
		return completeResult(name), nil
	}}}
	em, _, err := rt.CaptureBytes(context.Background(), "plain-source.txt", []byte("plain source evidence\n"), "local_dropzone")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := em.REL[originatedFromQuestionKey]; ok {
		t.Fatalf("capture without a question carries %s: %#v", originatedFromQuestionKey, em.REL)
	}
}

func TestOriginatedFromQuestionIsRuntimeOwned(t *testing.T) {
	f := newDifferenceFixture(t)
	rt := Runtime{Store: f.store, Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
		r := completeResult(name)
		if r.Relationships == nil {
			r.Relationships = map[string]string{}
		}
		r.Relationships[originatedFromQuestionKey] = "E-00000000000000FF"
		return r, nil
	}}}
	_, _, err := rt.CaptureBytes(context.Background(), "claims-question.txt", []byte("claims a question\n"), "local_dropzone")
	if err == nil || !strings.Contains(err.Error(), originatedFromQuestionKey) {
		t.Fatalf("model-claimed %s not rejected: %v", originatedFromQuestionKey, err)
	}
}

// A closed SAW recirculated through CaptureBytes carries the question only
// when its accepted members agree on one.
func TestOriginatedFromQuestionClosedSAW(t *testing.T) {
	member := func(id, question string) core.EmergION {
		em := core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{}}
		if question != "" {
			em.REL[originatedFromQuestionKey] = question
		}
		return em
	}
	const qa, qb = "E-00000000000000AA", "E-00000000000000BB"
	st := core.EmptyState()
	for _, em := range []core.EmergION{
		member("E-0000000000000001", qa),
		member("E-0000000000000002", qa),
		member("E-0000000000000003", ""),
		member("E-0000000000000004", qb),
	} {
		st.Accepted[em.IDN] = em
	}
	none := func(string) (core.EmergION, bool) { return core.EmergION{}, false }
	saw := core.EmergION{IDN: "E-00000000000000C0", REL: map[string]string{}}

	tests := []struct {
		name string
		want string
	}{
		{"SAW:SAAB:E-0000000000000001+E-0000000000000002", qa},
		{"SAW:SAAB:E-0000000000000001+E-0000000000000003", qa},
		{"SAW:SAAB:E-0000000000000001+E-0000000000000004", ""},
		{"SAW:SAAB:E-0000000000000003+E-0000000000000009", ""},
		{"not-a-saw", ""},
	}
	for _, tc := range tests {
		if got := originatedFromQuestion(saw, "saw_projection", tc.name, st, none); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	// The same name under another provenance is not a SAW.
	if got := originatedFromQuestion(saw, "local_dropzone", tests[0].name, st, none); got != "" {
		t.Fatalf("non-SAW provenance derived %q", got)
	}
}

// oldLedgerEvent is a candidate event in the format written before this
// key existed. Its sealed hash was computed at governed-demand-v1 1e9c558
// and must never change.
func oldLedgerEvent() core.Event {
	return core.Event{
		Type:     "C",
		ID:       "EV-OLD-1",
		At:       time.Unix(1763500000, 0).UTC(),
		PrevHash: "",
		EmergION: &core.EmergION{
			IDN: "E-0123456789ABCDEF",
			STA: core.StateAtGOV,
			MEM: core.Memory{SourceHash: "abc123", Bytes: 10, Stored: 10, Summary: "old record", Provenance: "governed_target_comparison"},
			REL: map[string]string{
				"source_kind":     "TARGET_COMPARISON",
				"target_emergion": "E-FEDCBA9876543210",
			},
			CAP: []string{"CMP", "DIF", "PRJ"},
			VAL: core.Validation{Facts: []string{"old fact"}, Recoil: true, WVC: true},
			EVO: core.Evolution{Version: 1},
		},
	}
}

const oldLedgerEventHash = "9e9bec59024b1a46d69a670f81ddc0747ea55719ed4c25b66d25c685f34744e9"

func TestOriginatedFromQuestionOldLedgerStillVerifies(t *testing.T) {
	sealed, err := cosl.Seal(oldLedgerEvent())
	if err != nil {
		t.Fatal(err)
	}
	if sealed.SelfHash != oldLedgerEventHash {
		t.Fatalf("old record hash changed: %s want %s", sealed.SelfHash, oldLedgerEventHash)
	}
	line, err := cosl.Encode(oldLedgerEvent())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, originatedFromQuestionKey) {
		t.Fatal("old record encoding gained the new key")
	}
	decoded, err := cosl.Decode(line)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SelfHash != oldLedgerEventHash {
		t.Fatalf("decoded old record hash = %s", decoded.SelfHash)
	}
	if _, ok := decoded.EmergION.REL[originatedFromQuestionKey]; ok {
		t.Fatal("decoding an old record invented the key")
	}

	// A ledger whose records predate the key: every event re-seals to its
	// stored hash, evidence verifies and FIELD rebuilds, before and after a
	// new capture that does carry the key is appended on top.
	f := newDifferenceFixture(t)
	governanceRealityFixture(f)
	before, err := f.store.Events()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range before {
		if ev.EmergION != nil {
			if _, ok := ev.EmergION.REL[originatedFromQuestionKey]; ok {
				t.Fatalf("old-format fixture carries the key: %s", ev.ID)
			}
		}
	}
	captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved([]string{realGovernanceDifference})},
		"The SOURCE provides evidence of authority and governance acceptance.")

	after, err := f.store.Events()
	if err != nil {
		t.Fatal(err)
	}
	for i, ev := range before {
		if after[i].SelfHash != ev.SelfHash {
			t.Fatalf("old event %s hash changed after new capture", ev.ID)
		}
	}
	carried := false
	for _, ev := range after {
		resealed, err := cosl.Seal(ev)
		if err != nil {
			t.Fatal(err)
		}
		if resealed.SelfHash != ev.SelfHash {
			t.Fatalf("event %s does not re-seal to its hash", ev.ID)
		}
		if ev.EmergION != nil && ev.EmergION.REL[originatedFromQuestionKey] != "" {
			carried = true
		}
	}
	if !carried {
		t.Fatal("new capture on the old ledger did not carry the key")
	}
	if _, err := f.store.VerifyEvidence(); err != nil {
		t.Fatal(err)
	}
	_ = rebuildState(t, f.store)
}
