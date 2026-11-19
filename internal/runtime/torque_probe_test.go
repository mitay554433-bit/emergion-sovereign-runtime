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
// HUMAN_FINAL, run only after explicit Q, never automatically.
//
// TestTorqueProbePatchPathStoppage is the APProver probe for CHOICE A. From
// a HUMAN_FINAL / REG-accepted genuine-remainder BRIDGEGAP (the boundary) it
// asks the existing patch path, ProposeOneProgramPatch, for a proposal and
// records exactly where the path stops. By default it pins the stoppage and
// passes, failing loudly the day the path carries a probe. With
// UNIFUSION_TORQUE_PROBE=1 it fails on purpose and prints the stoppage.

const (
	torqueStopNotProgramOrigin = "BOUNDARY_NOT_PROGRAM_ORIGIN"
	torqueStopNoProgramTarget  = "BOUNDARY_HAS_NO_TRACKED_PROGRAM_TARGET"
	torqueStopNoProposal       = "PATCH_PATH_PROPOSED_NOTHING"
)

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

func TestTorqueProbePatchPathStoppage(t *testing.T) {
	symbolic := []string{"TARGET_STATE!=ACCEPTED_REALITY"}

	cases := []struct {
		name     string
		reality  func(differenceFixture)
		target   string
		gap      string
		required string
	}{
		{
			name: "facts",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A0", "torque-obs-source", "OBS"))
				f.accept(acceptedProviderFixture("E-00000000000000A2", "torque-vld-source", "VLD"))
			},
			target:   "invoice ledger reconciled",
			gap:      "BRIDGEGAP:facts",
			required: "ESTABLISH_FACT",
		},
		{
			name: "relationships",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A1", "torque-a1-source", "OBS"))
				f.accept(acceptedProviderFixture("E-00000000000000B2", "torque-b2-source", "VLD"))
			},
			target:   "E-00000000000000A1 composes with E-00000000000000B2",
			gap:      "BRIDGEGAP:relationships",
			required: "DERIVE_RELATIONSHIP",
		},
		{
			name: "capabilities",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A0", "torque-cap-source", "OBS"))
			},
			target:   "PROGRAM capability is available",
			gap:      "BRIDGEGAP:capabilities",
			required: "DERIVE_CAPABILITY",
		},
	}

	gemma := torqueFakeGemma(t)
	var report []string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDifferenceFixture(t)
			tc.reality(f)

			boundary := captureDifferenceComparison(t, f,
				fixedDifferenceObserver{result: differenceObserved(symbolic)},
				tc.target)
			if len(boundary.VAL.Gaps) != 1 || boundary.VAL.Gaps[0] != tc.gap {
				t.Fatalf("boundary gaps = %#v want [%q]", boundary.VAL.Gaps, tc.gap)
			}
			if boundary.REL["required_capability"] != tc.required {
				t.Fatalf("required_capability = %q want %q", boundary.REL["required_capability"], tc.required)
			}
			acceptBoundaryCandidate(t, f, boundary)
			st := f.state()
			accepted, ok := st.Accepted[boundary.IDN]
			if !ok {
				t.Fatal("boundary not REG-accepted")
			}

			// Stop 1: the patch path only starts from an accepted PROGRAM
			// origin (CAP PROGRAM with an enabled GITHUB:PROGRAM action).
			// The boundary carries CMP+DIF+PRJ.
			if acceptedProgramActionEnabled(accepted, true) {
				t.Fatal("stoppage changed: boundary is now a PROGRAM origin; revisit TORQUE design")
			}
			report = append(report, tc.gap+" -> "+torqueStopNotProgramOrigin)

			// Stop 2: the patch path edits one tracked file named by the
			// origin's source_name. The boundary's evidence is
			// TARGET_COMPARISON/1, not a repository file, and choosing a
			// file from prose would preselect the answer.
			if _, _, err := currentProgramTarget(t.TempDir(), accepted); err == nil {
				t.Fatal("stoppage changed: boundary now names a program target; revisit TORQUE design")
			}
			report = append(report, tc.gap+" -> "+torqueStopNoProgramTarget)

			// Stop 3: so the existing path proposes nothing for the boundary.
			proposal, proposed, err := (Runtime{Store: f.store}).ProposeOneProgramPatch(
				context.Background(), gemma, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if proposed {
				t.Fatalf("stoppage changed: patch path proposed %s; revisit TORQUE design", proposal.IDN)
			}
			for _, em := range f.state().AtGOV {
				if em.REL["source_kind"] == "PROGRAM_PATCH" {
					t.Fatalf("unexpected patch proposal %s", em.IDN)
				}
			}
			report = append(report, tc.gap+" -> "+torqueStopNoProposal)
		})
	}

	if os.Getenv("UNIFUSION_TORQUE_PROBE") == "1" {
		t.Fatalf("TORQUE B_n -> P_n stoppage on the patch path (CHOICE A):\n%s",
			strings.Join(report, "\n"))
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
