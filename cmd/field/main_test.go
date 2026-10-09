package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
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

func TestPublishAcceptedDeliverablesMaterializesAcceptedExecutionResult(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PUBLISH-SOURCE",
		SourceHash:      "SOURCE-PUBLISH-PROOF",
		AuthorizationID: "EV-Q-PUBLISH",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)

	const output = "verified published deliverable\nline=two:preserved"

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    output,
		},
	)

	rt := fieldruntime.Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("execution result unexpectedly duplicate")
	}

	approved, decision, err := gov.Decide(
		signal,
		gov.Approve,
		"HUMAN_FINAL",
		"accept publisher integration proof",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}

	accepted, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(root, "outputs")

	published, err := publishAcceptedDeliverables(s, out)
	if err != nil {
		t.Fatal(err)
	}

	if len(published) != 1 || published[0] != accepted.IDN {
		t.Fatalf("published = %#v want [%q]", published, accepted.IDN)
	}

	artifact := filepath.Join(
		out,
		"deliverables",
		accepted.IDN,
		"output",
	)
	got, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != output {
		t.Fatalf("published output = %q want %q", got, output)
	}

	receiptPath := filepath.Join(
		out,
		"deliverables",
		accepted.IDN,
		"receipt.json",
	)
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}

	var publishedReceipt deliverableReceipt
	if err := json.Unmarshal(receiptBytes, &publishedReceipt); err != nil {
		t.Fatal(err)
	}
	if publishedReceipt.EmergIONID != accepted.IDN {
		t.Fatalf(
			"receipt EmergION = %q want %q",
			publishedReceipt.EmergIONID,
			accepted.IDN,
		)
	}

	indexBytes, err := os.ReadFile(
		filepath.Join(out, "deliverables", "index.json"),
	)
	if err != nil {
		t.Fatal(err)
	}

	var index deliverableIndex
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatal(err)
	}

	if index.FieldTip == "" {
		t.Fatal("deliverable index missing FIELD tip")
	}
	if index.FieldTip != publishedReceipt.FieldTip {
		t.Fatalf(
			"index FIELD tip = %q receipt FIELD tip = %q",
			index.FieldTip,
			publishedReceipt.FieldTip,
		)
	}
	if len(index.Deliverables) != 1 ||
		index.Deliverables[0].EmergIONID != accepted.IDN {
		t.Fatalf("deliverable index = %#v", index.Deliverables)
	}
}
