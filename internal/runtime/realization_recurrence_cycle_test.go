package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

func acceptCycleFixture(t *testing.T, s *store.Store, em core.EmergION) {
	t.Helper()
	if _, err := s.SaveCandidate(em); err != nil { t.Fatal(err) }
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "cycle recurrence fixture")
	if err != nil { t.Fatal(err) }
	decisionID, err := s.SaveDecision(decision)
	if err != nil { t.Fatal(err) }
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil { t.Fatal(err) }
	if _, err := s.SaveAccepted(receipt); err != nil { t.Fatal(err) }
}

func cycleRealizationFixture(t *testing.T, s *store.Store, id string) core.EmergION {
	t.Helper()
	evidence, err := s.Preserve([]byte("cycle evidence " + id))
	if err != nil { t.Fatal(err) }
	return core.EmergION{
		IDN: id,
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: evidence.Hash,
			Codec: evidence.Codec,
			Bytes: evidence.Bytes,
			Stored: evidence.Stored,
			Summary: "cycle realization " + id,
		},
		REL: map[string]string{
			"source_kind": "SOURCE",
			"target_thing": "KNIFE",
		},
		CAP: []string{"ANALYZE"},
		VAL: core.Validation{
			Facts: []string{"cycle realization evidence " + id},
			Risk: "L", Recoil: true, WVC: true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt: time.Now().UTC(),
				Topology: core.TopologyDodecahedronV1,
				PromptSchema: "MXPD/2",
			},
		},
	}
}

func TestExecuteOneSafeActionPrioritizesAcceptedReorderSelection(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }

	// Lexicographic order would execute E-A first. The accepted reorder selects
	// E-B, so the governed cycle must prefer E-B without granting new authority.
	acceptCycleFixture(t, s, cycleRealizationFixture(t, s, "E-A"))
	acceptCycleFixture(t, s, cycleRealizationFixture(t, s, "E-B"))

	reorderEvidence, err := s.Preserve([]byte("accepted reorder selects E-B"))
	if err != nil { t.Fatal(err) }
	acceptCycleFixture(t, s, core.EmergION{
		IDN: "E-REORDER",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: reorderEvidence.Hash,
			Codec: reorderEvidence.Codec,
			Bytes: reorderEvidence.Bytes,
			Stored: reorderEvidence.Stored,
			Summary: "accepted realization reorder",
		},
		REL: map[string]string{
			"source_kind": "REALIZATION_REORDER",
			"target_thing": "KNIFE",
			"selected_realization": "E-B",
			"runners_up": "E-A",
		},
		CAP: []string{"OBS", "CMP", "DIF"},
		VAL: core.Validation{
			Facts: []string{"realization_reorder_derived"},
			Risk: "L", Recoil: true, WVC: true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt: time.Now().UTC(),
				Topology: core.TopologyDodecahedronV1,
				PromptSchema: "MXPD/2",
			},
		},
	})

	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil { t.Fatal(err) }
	binary := filepath.Join(root, "llama-cli")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo fake; exit 0; fi\necho 'S|safe recurrence analysis'\necho 'K|L'\necho 'F|safe recurrence analysis'\necho 'C|ANALYZE'\necho 'Z'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil { t.Fatal(err) }

	r := Runtime{Store: s}
	signal, executed, err := r.ExecuteOneSafeAction(
		context.Background(),
		reason.GemmaCLI{
			Binary: binary, Model: model, Threads: 1,
			Context: 2048, MaxTokens: 64, Timeout: 5 * time.Second,
		},
	)
	if err != nil { t.Fatal(err) }
	if !executed { t.Fatal("accepted reorder selection was not executed") }
	if signal.REL["parent_emergion"] != "E-B" {
		t.Fatalf("execution parent = %q want selected E-B", signal.REL["parent_emergion"])
	}

	state, _, err := r.governedStateContext()
	if err != nil { t.Fatal(err) }
	if len(state.ActionAuthorizations) != 0 {
		t.Fatal("safe reorder recurrence manufactured Q authorization")
	}
}
