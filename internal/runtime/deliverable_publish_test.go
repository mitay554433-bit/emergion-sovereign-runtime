package runtime

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPublishDeliverableIndexPreservesVerifiedOrder(t *testing.T) {
	root := t.TempDir()

	receipts := []DeliverableReceipt{
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

	var got DeliverableIndex
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
