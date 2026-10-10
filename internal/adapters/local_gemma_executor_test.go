package adapters

import (
	"path/filepath"
	"testing"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

type executorReasonerProof struct{}

func TestPrepareExecutionCarriesAcceptedSourceHash(t *testing.T) {
	st := executionProofState()

	em := st.Accepted["E-EXEC-PROOF"]
	em.MEM.SourceHash = "SOURCE-HASH-PROOF"
	em.CAP = append(em.CAP, "ANALYZE")
	st.Accepted[em.IDN] = em

	request, err := PrepareExecution(
		st,
		"E-EXEC-PROOF",
		"LOCAL_GEMMA",
		"ANALYZE",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	if request.SourceHash != "SOURCE-HASH-PROOF" {
		t.Fatalf(
			"source hash = %q want SOURCE-HASH-PROOF",
			request.SourceHash,
		)
	}
}

func TestLocalGemmaExecutorRejectsWrongAdapter(t *testing.T) {
	executor := LocalGemmaExecutor{}

	result, err := executor.Execute(ExecutionRequest{
		Adapter: "EMAIL",
		Action:  "ANALYZE",
	})
	if err == nil {
		t.Fatal("wrong adapter unexpectedly executed")
	}
	if result.Succeeded {
		t.Fatal("wrong adapter reported success")
	}
}

func TestLocalGemmaExecutorRejectsUnsupportedAction(t *testing.T) {
	executor := LocalGemmaExecutor{}

	result, err := executor.Execute(ExecutionRequest{
		Adapter: "LOCAL_GEMMA",
		Action:  "SEND",
	})
	if err == nil {
		t.Fatal("unsupported LOCAL_GEMMA action unexpectedly executed")
	}
	if result.Succeeded {
		t.Fatal("unsupported LOCAL_GEMMA action reported success")
	}
}

func TestLocalGemmaExecutorRequiresPreservedEvidence(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	executor := LocalGemmaExecutor{
		Store: s,
		Gemma: reason.GemmaFromEnv(),
	}

	result, err := executor.Execute(ExecutionRequest{
		Adapter:    "LOCAL_GEMMA",
		Action:     "ANALYZE",
		SourceHash: "missing-evidence",
	})
	if err == nil {
		t.Fatal("missing preserved evidence unexpectedly executed")
	}
	if result.Succeeded {
		t.Fatal("missing evidence reported success")
	}
}

func TestLocalGemmaExecutorDraftReachesPreservedEvidenceBoundary(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	executor := LocalGemmaExecutor{
		Store: s,
		Gemma: reason.GemmaFromEnv(),
	}

	result, err := executor.Execute(ExecutionRequest{
		Adapter:    "LOCAL_GEMMA",
		Action:     "DRAFT",
		SourceHash: "missing-evidence",
	})
	if err == nil {
		t.Fatal("DRAFT with missing preserved evidence unexpectedly executed")
	}
	if result.Succeeded {
		t.Fatal("DRAFT with missing evidence reported success")
	}
}
