package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

// TORQUE / next-probe (B_n -> P_n), CHOICE A: a probe is a proposed
// program/test patch through the existing patch path, approved by
// HUMAN_FINAL, run only after explicit Q, never automatically. The fracture
// probe itself is generated and exercised in fracture_probe_test.go.

func acceptBoundaryCandidate(t *testing.T, f differenceFixture, em core.EmergION) {
	t.Helper()
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "accept BRIDGEGAP boundary for TORQUE probe")
	if err != nil {
		t.Fatal(err)
	}
	decisionID, err := f.store.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}
}

// torqueFakeGemma is a validating local Gemma stand-in. Any patch request
// would receive no usable patch; the stoppage happens before it is asked.
func torqueFakeGemma(t *testing.T) reason.GemmaCLI {
	t.Helper()
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "llama-cli")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo fake; exit 0; fi\necho 'S|probe evidence'\necho 'K|L'\necho 'F|probe evidence'\necho 'C|ANALYZE'\necho 'Z'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return reason.GemmaCLI{
		Binary: binary, Model: model, Threads: 1,
		Context: 2048, MaxTokens: 64, Timeout: 5 * time.Second,
	}
}

// The model is not a hidden probe source: the comparison never records a
// next_probe relationship or a next_probe_derived fact the model proposed.
func TestTorqueProbeNotTakenFromModel(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "torque-model-source", "OBS"))

	observed := differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})
	observed.Relationships = map[string]string{"next_probe": "GITHUB:PROGRAM"}
	observed.Facts = append(observed.Facts, "next_probe_derived", " NEXT_PROBE_DERIVED ")
	boundary := captureDifferenceComparison(t, f, fixedDifferenceObserver{result: observed}, "invoice ledger reconciled")

	if _, ok := boundary.REL["next_probe"]; ok {
		t.Fatalf("model next_probe forwarded: %#v", boundary.REL)
	}
	for _, fact := range boundary.VAL.Facts {
		if strings.EqualFold(strings.TrimSpace(fact), "next_probe_derived") {
			t.Fatalf("model next_probe_derived forwarded: %#v", boundary.VAL.Facts)
		}
	}
	if len(boundary.VAL.Facts) == 0 {
		t.Fatal("ordinary comparison facts were dropped")
	}
}

func TestNextProbeRelationshipIsRuntimeOwned(t *testing.T) {
	f := newDifferenceFixture(t)
	observer := fixedDifferenceObserver{result: reason.Result{
		Summary:       "forged next probe",
		Relationships: map[string]string{"next_probe": "GITHUB:PROGRAM"},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"forged next probe"},
		Risk:          "L",
	}}
	_, _, err := (Runtime{Store: f.store, Reasoner: observer}).CaptureBytes(
		context.Background(), "forged", []byte("forged next probe"), "local_dropzone")
	if err == nil || !strings.Contains(err.Error(), "next_probe") {
		t.Fatalf("forged next_probe accepted: %v", err)
	}
}

func probePatchFixture(t *testing.T, s *store.Store, id, sourceKind string) core.EmergION {
	t.Helper()
	evidence, err := s.Preserve([]byte("probe patch evidence " + id + " " + sourceKind))
	if err != nil {
		t.Fatal(err)
	}
	return core.EmergION{
		IDN: id,
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: evidence.Hash,
			Codec:      evidence.Codec,
			Bytes:      evidence.Bytes,
			Stored:     evidence.Stored,
			Summary:    "probe patch " + id,
		},
		REL: map[string]string{
			"source_kind":    sourceKind,
			"program_target": "internal/probe_test.go",
			"origin":         "E-00000000000000C0",
		},
		// Worst case: the record carries ANALYZE as well as PROGRAM, so only
		// the patch guard keeps the safe-action scan away from it.
		CAP: []string{"ANALYZE", "PROGRAM"},
		VAL: core.Validation{
			Facts: []string{"probe patch proposal preserved"},
			Risk:  "H", Recoil: true, WVC: true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt:   time.Now().UTC(),
				Topology:     core.TopologyDodecahedronV1,
				PromptSchema: "MXPD/2",
				Facets:       []core.Facet{core.FacetProgramForge},
			},
		},
	}
}

// An accepted program/test patch (the CHOICE A shape of a TORQUE probe) is
// never executed by the safe-action scan and cannot be prepared for
// GITHUB:PROGRAM without an explicit HUMAN_FINAL Q.
func TestAcceptedProbePatchNeverRunsWithoutQ(t *testing.T) {
	gemma := torqueFakeGemma(t)

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	acceptCycleFixture(t, s, probePatchFixture(t, s, "E-00000000000000D0", "PROGRAM_PATCH"))

	r := Runtime{Store: s}
	_, executed, err := r.ExecuteOneSafeAction(context.Background(), gemma)
	if err != nil {
		t.Fatal(err)
	}
	if executed {
		t.Fatal("accepted probe patch executed without Q")
	}

	st, _, err := r.governedStateContext()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ActionAuthorizations) != 0 {
		t.Fatal("safe-action scan manufactured Q authorization")
	}
	if len(st.AtGOV) != 0 {
		t.Fatalf("safe-action scan recorded an execution signal: %d candidates", len(st.AtGOV))
	}
	if _, err := adapters.PrepareExecution(st, "E-00000000000000D0", "GITHUB", "PROGRAM", true); err == nil ||
		!strings.Contains(err.Error(), "requires authorization") {
		t.Fatalf("GITHUB:PROGRAM prepared without Q: %v", err)
	}

	// Control: the same record as an ordinary SOURCE is picked up, so the
	// guard above is what keeps the probe from running.
	control, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	acceptCycleFixture(t, control, probePatchFixture(t, control, "E-00000000000000D1", "SOURCE"))
	if _, executed, err := (Runtime{Store: control}).ExecuteOneSafeAction(context.Background(), gemma); err != nil {
		t.Fatal(err)
	} else if !executed {
		t.Fatal("control SOURCE record was not executed; guard test is vacuous")
	}
}
