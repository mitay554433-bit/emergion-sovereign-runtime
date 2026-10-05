package main

import (
	"encoding/json"
	"os"
	"testing"

	fieldruntime "emergion-sovereign-runtime/internal/runtime"
)

func TestPublishDeliverableIsIdempotent(t *testing.T) {
	root := t.TempDir()
	d := fieldruntime.Deliverable{
		EmergIONID:       "E-TEST",
		EvidenceHash:     "evidence-hash",
		SourceEmergIONID: "E-SOURCE",
		SourceHash:       "source-hash",
		AuthorizationID:  "EV-AUTH",
		Authority:        "CAP_ONLY",
		Adapter:          "LOCAL_GEMMA",
		Action:           "ANALYZE",
		Output:           "verified output\n",
		FieldTip:         "tip-one",
	}

	first, artifact, err := publishDeliverable(root, d)
	if err != nil {
		t.Fatal(err)
	}

	d.FieldTip = "tip-two"

	second, artifact2, err := publishDeliverable(root, d)
	if err != nil {
		t.Fatal(err)
	}

	if artifact2 != artifact {
		t.Fatalf("artifact changed: %q != %q", artifact2, artifact)
	}
	if !second.PublishedAt.Equal(first.PublishedAt) {
		t.Fatalf("republished: %s != %s", second.PublishedAt, first.PublishedAt)
	}

	got, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != d.Output {
		t.Fatalf("output changed: %q", got)
	}
}

func TestPublishDeliverableIndexPreservesVerifiedOrder(t *testing.T) {
	root := t.TempDir()

	receipts := []deliverableReceipt{
		{
			EmergIONID:   "E-A",
			EvidenceHash: "evidence-a",
			OutputSHA256: "output-a",
			FieldTip:     "tip-1",
		},
		{
			EmergIONID:   "E-B",
			EvidenceHash: "evidence-b",
			OutputSHA256: "output-b",
			FieldTip:     "tip-1",
		},
	}

	path, err := publishDeliverableIndex(root, "tip-1", receipts)
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var got deliverableIndex
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	if got.FieldTip != "tip-1" {
		t.Fatalf("field tip = %q", got.FieldTip)
	}
	if len(got.Deliverables) != 2 {
		t.Fatalf("deliverables = %d", len(got.Deliverables))
	}
	if got.Deliverables[0].EmergIONID != "E-A" ||
		got.Deliverables[1].EmergIONID != "E-B" {
		t.Fatalf("delivery order changed: %#v", got.Deliverables)
	}

	receipts = receipts[:1]
	if _, err := publishDeliverableIndex(root, "tip-2", receipts); err != nil {
		t.Fatal(err)
	}

	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	if got.FieldTip != "tip-2" ||
		len(got.Deliverables) != 1 ||
		got.Deliverables[0].EmergIONID != "E-A" {
		t.Fatalf("index was not atomically replaced: %#v", got)
	}
}
