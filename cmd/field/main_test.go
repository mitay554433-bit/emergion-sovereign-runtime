package main

import (
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
