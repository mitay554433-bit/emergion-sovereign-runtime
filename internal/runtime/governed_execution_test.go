package runtime

import (
	"context"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
)

func governedExecutionProofRequest() adapters.ExecutionRequest {
	return adapters.ExecutionRequest{
		EmergIONID:      "E-GOVERNED-EXECUTION-PROOF",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "Q-PROOF",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
		Authority:       "CAP_ONLY",
	}
}

func TestGovernedExecutionRejectsBrokenLineageBeforeAdmission(t *testing.T) {
	request := governedExecutionProofRequest()

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    `{"summary":"bounded proof"}`,
		},
	)

	result.SourceHash = "OTHER-SOURCE"

	r := Runtime{}

	_, _, err := r.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err == nil {
		t.Fatal("broken execution lineage unexpectedly admitted")
	}
}

func TestGovernedExecutionRejectsActionPivotBeforeAdmission(t *testing.T) {
	request := governedExecutionProofRequest()

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
		},
	)

	result.Action = "SEND"

	r := Runtime{}

	_, _, err := r.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err == nil {
		t.Fatal("execution action pivot unexpectedly admitted")
	}
}

func TestGovernedExecutionRejectsAuthorityPivotBeforeAdmission(t *testing.T) {
	request := governedExecutionProofRequest()

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: false,
			Error:     "bounded execution defect",
		},
	)

	result.Authority = "HUMAN_FINAL"

	r := Runtime{}

	_, _, err := r.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err == nil {
		t.Fatal("execution authority pivot unexpectedly admitted")
	}
}

func TestExecutionSuccessWithoutProviderValidationDoesNotProveCapability(t *testing.T) {
	request := governedExecutionProofRequest()
	result := adapters.BindExecutionResult(request, adapters.ExecutionResult{
		Succeeded: true,
		Output:    "adapter completed, not independently validated",
	})
	// A valid execution receipt is not independent proof that the
	// originating provider demonstrated OBS or VLD competence.
	if err := adapters.VerifyExecutionResult(request, result); err != nil {
		t.Fatalf("valid execution lineage rejected: %v", err)
	}
	signal := core.EmergION{
		REL: map[string]string{
			"source_kind": "EXECUTION_RESULT",
			"kin_emergion": request.EmergIONID,
			"adapter": request.Adapter,
			"action": request.Action,
		},
		VAL: core.Validation{Facts: []string{"execution_result_observed", "execution_succeeded"}, WVC: true, Recoil: true},
	}
	for _, capability := range []string{"OBS", "VLD"} {
		if got := capabilityEvidenceCount(signal, capability); got != 0 {
			t.Fatalf("execution success alone proved %s: %d", capability, got)
		}
	}
}
