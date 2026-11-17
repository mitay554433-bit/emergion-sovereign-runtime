package gov

import (
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/store"
)

func eligibleObservation(id string) core.EmergION {
	return core.EmergION{
		IDN: id,
		STA: core.StateAtGOV,
		MEM: core.Memory{Provenance: "provider_population", SourceHash: "h-" + id},
		CAP: []string{"OBS"},
		VAL: core.Validation{Risk: "L", Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1},
	}
}

func TestSelectObservationBatchIncludesOnlyEligibleItems(t *testing.T) {
	okA := eligibleObservation("E-A")
	okB := eligibleObservation("E-B")
	medium := eligibleObservation("E-M")
	medium.VAL.Risk = "M"
	otherSource := eligibleObservation("E-S")
	otherSource.MEM.Provenance = "local_dropzone"
	multiCAP := eligibleObservation("E-C")
	multiCAP.CAP = []string{"OBS", "CMP"}
	alreadyDecided := eligibleObservation("E-D")
	alreadyDecided.STA = core.StateApproved
	emptyCAP := eligibleObservation("E-E")
	emptyCAP.CAP = nil

	selected := SelectObservationBatch([]core.EmergION{
		okB, medium, otherSource, multiCAP, alreadyDecided, emptyCAP, okA,
	})
	if len(selected) != 2 || selected[0].IDN != "E-A" || selected[1].IDN != "E-B" {
		t.Fatalf("selected = %v", idsOf(selected))
	}
}

func TestPreviewObservationBatchWritesNothing(t *testing.T) {
	s := openTestStore(t)
	em := mustSave(t, s, eligibleObservation("E-P1"))
	before, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	preview := PreviewObservationBatch([]core.EmergION{em})
	if preview.Count != 1 || preview.BatchID == "" || len(preview.IDs) != 1 || preview.IDs[0] != "E-P1" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.BatchID != ObservationBatchID(preview.IDs) {
		t.Fatalf("batch id = %q", preview.BatchID)
	}

	after, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("preview wrote %d events, want %d", len(after), len(before))
	}
}

func TestDecideBatchRejectsNonHumanFinal(t *testing.T) {
	s := openTestStore(t)
	em := mustSave(t, s, eligibleObservation("E-A"))
	preview := PreviewObservationBatch([]core.EmergION{em})
	before := eventCount(t, s)

	updated, receipts, err := DecideBatch(
		[]core.EmergION{em},
		nil,
		Approve,
		"MODEL",
		"batch review",
		preview.BatchID,
	)
	if err == nil || !strings.Contains(err.Error(), "HUMAN_FINAL") {
		t.Fatalf("error = %v", err)
	}
	if updated != nil || receipts != nil {
		t.Fatalf("fail-closed returned results: %v %v", updated, receipts)
	}
	if eventCount(t, s) != before {
		t.Fatal("non-HUMAN_FINAL batch wrote ledger events")
	}
}

func TestDecideBatchFailsClosedWhenAnyItemIsIneligible(t *testing.T) {
	s := openTestStore(t)
	ok := mustSave(t, s, eligibleObservation("E-OK"))
	medium := eligibleObservation("E-M")
	medium.VAL.Risk = "M"
	medium = mustSave(t, s, medium)
	otherSource := eligibleObservation("E-S")
	otherSource.MEM.Provenance = "local_dropzone"
	otherSource = mustSave(t, s, otherSource)
	multiCAP := eligibleObservation("E-C")
	multiCAP.CAP = []string{"OBS", "CMP"}
	multiCAP = mustSave(t, s, multiCAP)

	cases := []struct {
		name string
		item core.EmergION
	}{
		{"medium-risk", medium},
		{"non-provider_population", otherSource},
		{"multi-CAP", multiCAP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids := []string{ok.IDN, tc.item.IDN}
			batchID := ObservationBatchID(ids)
			before := eventCount(t, s)
			updated, receipts, err := DecideBatch(
				[]core.EmergION{ok, tc.item},
				ids,
				Approve,
				"HUMAN_FINAL",
				"batch review",
				batchID,
			)
			if err == nil {
				t.Fatal("ineligible item was accepted")
			}
			if updated != nil || receipts != nil {
				t.Fatalf("fail-closed returned results: %v %v", updated, receipts)
			}
			if eventCount(t, s) != before {
				t.Fatal("failed batch wrote ledger events")
			}
		})
	}
}

func TestDecideBatchWritesOneReceiptPerItem(t *testing.T) {
	s := openTestStore(t)
	a := mustSave(t, s, eligibleObservation("E-A"))
	b := mustSave(t, s, eligibleObservation("E-B"))
	preview := PreviewObservationBatch([]core.EmergION{a, b})
	if preview.Count != 2 {
		t.Fatalf("count = %d", preview.Count)
	}

	updated, receipts, err := DecideBatch(
		[]core.EmergION{a, b},
		preview.IDs,
		Hold,
		"HUMAN_FINAL",
		"one decision for low-risk observations",
		preview.BatchID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 || len(receipts) != 2 {
		t.Fatalf("got %d updates and %d receipts", len(updated), len(receipts))
	}

	before := eventCount(t, s)
	for i, receipt := range receipts {
		if receipt.Authority != "HUMAN_FINAL" {
			t.Fatalf("authority = %q", receipt.Authority)
		}
		if receipt.Decision != string(Hold) {
			t.Fatalf("decision = %q", receipt.Decision)
		}
		if receipt.EmergIONID != updated[i].IDN {
			t.Fatalf("receipt id = %q", receipt.EmergIONID)
		}
		if updated[i].STA != core.StateHeld {
			t.Fatalf("state = %s", updated[i].STA)
		}
		if !strings.Contains(receipt.Reason, preview.BatchID) {
			t.Fatalf("reason %q missing batch id", receipt.Reason)
		}
		if _, err := s.SaveDecision(receipt); err != nil {
			t.Fatal(err)
		}
	}
	if eventCount(t, s) != before+2 {
		t.Fatalf("wrote %d events, want 2 receipts", eventCount(t, s)-before)
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	written := 0
	for _, ev := range events {
		if ev.Type != "D" || ev.Decision == nil {
			continue
		}
		written++
		if ev.Decision.Authority != "HUMAN_FINAL" {
			t.Fatalf("ledger authority = %q", ev.Decision.Authority)
		}
		if !strings.Contains(ev.Decision.Reason, preview.BatchID) {
			t.Fatalf("ledger reason %q missing batch id", ev.Decision.Reason)
		}
	}
	if written != 2 {
		t.Fatalf("ledger decisions = %d", written)
	}
}

func TestDecideBatchRejectsStaleBatchID(t *testing.T) {
	a := eligibleObservation("E-A")
	b := eligibleObservation("E-B")
	preview := PreviewObservationBatch([]core.EmergION{a})
	_, receipts, err := DecideBatch(
		[]core.EmergION{a, b},
		nil,
		Hold,
		"HUMAN_FINAL",
		"stale",
		preview.BatchID,
	)
	if err == nil || receipts != nil {
		t.Fatalf("stale batch id accepted: %v %v", err, receipts)
	}
}

func idsOf(items []core.EmergION) []string {
	out := make([]string, len(items))
	for i, em := range items {
		out[i] = em.IDN
	}
	return out
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustSave(t *testing.T, s *store.Store, em core.EmergION) core.EmergION {
	t.Helper()
	ev, err := s.Preserve([]byte("evidence-" + em.IDN))
	if err != nil {
		t.Fatal(err)
	}
	em.MEM.SourceHash = ev.Hash
	em.MEM.Codec = ev.Codec
	em.MEM.Bytes = ev.Bytes
	em.MEM.Stored = ev.Stored
	if _, err := s.SaveCandidate(em); err != nil {
		t.Fatal(err)
	}
	return em
}

func eventCount(t *testing.T, s *store.Store) int {
	t.Helper()
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}
