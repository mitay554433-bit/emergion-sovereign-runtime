package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	first, artifact, err := fieldruntime.PublishDeliverable(root, d)
	if err != nil {
		t.Fatal(err)
	}

	d.FieldTip = "tip-two"

	second, artifact2, err := fieldruntime.PublishDeliverable(root, d)
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

	published, err := (fieldruntime.Runtime{Store: s}).PublishAcceptedDeliverables(out)
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

	var publishedReceipt fieldruntime.DeliverableReceipt
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

	var index fieldruntime.DeliverableIndex
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


func TestDaemonPathsUsePlatformTempDirectory(t *testing.T) {
	t.Setenv("FIELD_SOCKET", "")
	t.Setenv("FIELD_PID", "")

	sockPath, pidPath := daemonPaths()
	wantBase := os.TempDir()

	if filepath.Dir(sockPath) != wantBase {
		t.Fatalf("socket directory = %q want %q", filepath.Dir(sockPath), wantBase)
	}
	if filepath.Dir(pidPath) != wantBase {
		t.Fatalf("pid directory = %q want %q", filepath.Dir(pidPath), wantBase)
	}
}

func TestDaemonPathsRespectExplicitConfiguration(t *testing.T) {
	root := t.TempDir()
	wantSocket := filepath.Join(root, "operator.sock")
	wantPID := filepath.Join(root, "operator.pid")
	t.Setenv("FIELD_SOCKET", wantSocket)
	t.Setenv("FIELD_PID", wantPID)

	sockPath, pidPath := daemonPaths()
	if sockPath != wantSocket || pidPath != wantPID {
		t.Fatalf("daemon paths = %q %q want %q %q", sockPath, pidPath, wantSocket, wantPID)
	}
}

func TestDaemonStartCommandUsesAdjacentContractAndLoopbackHTTP(t *testing.T) {
	t.Setenv("FIELDD_BIN", "/pilot/bin/fieldd")
	t.Setenv("FIELD_HTTP", "127.0.0.1:8899")

	cmd := daemonStartCommand(".field", "/tmp/pilot.sock", "/tmp/pilot.pid")
	if cmd.Path != "/pilot/bin/fieldd" {
		t.Fatalf("fieldd binary = %q", cmd.Path)
	}

	args := strings.Join(cmd.Args[1:], " ")
	for _, want := range []string{
		"-state .field",
		"-socket /tmp/pilot.sock",
		"-pid /tmp/pilot.pid",
		"-http 127.0.0.1:8899",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("daemon args %q missing %q", args, want)
		}
	}
}

func TestDaemonStartCommandAllowsHTTPOptOut(t *testing.T) {
	t.Setenv("FIELDD_BIN", "/pilot/bin/fieldd")
	t.Setenv("FIELD_HTTP", "")

	cmd := daemonStartCommand(".field", "/tmp/pilot.sock", "/tmp/pilot.pid")
	args := strings.Join(cmd.Args[1:], " ")
	if strings.Contains(args, "-http") {
		t.Fatalf("daemon args unexpectedly enable HTTP: %q", args)
	}
}
