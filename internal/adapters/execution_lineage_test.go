package adapters

import "testing"

func executionLineageProofRequest() ExecutionRequest {
	request := ExecutionRequest{
		EmergIONID:      "E-LINEAGE-PROOF",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "Q-PROOF",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
		Authority:       "CAP_ONLY",
	}
	request.TransitionEmergIONID = ExecutionTransitionID(request)
	return request
}

func TestBindExecutionResultCopiesRequestLineage(t *testing.T) {
	request := executionLineageProofRequest()

	result := BindExecutionResult(
		request,
		ExecutionResult{
			Succeeded: true,
			Output:    `{"summary":"proof"}`,
		},
	)

	if err := VerifyExecutionResult(request, result); err != nil {
		t.Fatal(err)
	}

	if result.EmergIONID != request.EmergIONID {
		t.Fatal("EmergION lineage not retained")
	}

	if result.SourceHash != request.SourceHash {
		t.Fatal("source lineage not retained")
	}

	if result.AuthorizationID != request.AuthorizationID {
		t.Fatal("authorization lineage not retained")
	}

	if result.Authority != request.Authority {
		t.Fatal("authority lineage not retained")
	}

	if result.Adapter != request.Adapter {
		t.Fatal("adapter lineage not retained")
	}

	if result.Action != request.Action {
		t.Fatal("action lineage not retained")
	}
}

func TestVerifyExecutionResultRejectsWrongSource(t *testing.T) {
	request := executionLineageProofRequest()
	result := BindExecutionResult(request, ExecutionResult{})

	result.SourceHash = "OTHER-SOURCE"

	if err := VerifyExecutionResult(request, result); err == nil {
		t.Fatal("mismatched execution source unexpectedly verified")
	}
}

func TestVerifyExecutionResultRejectsWrongEmergION(t *testing.T) {
	request := executionLineageProofRequest()
	result := BindExecutionResult(request, ExecutionResult{})

	result.EmergIONID = "E-OTHER"

	if err := VerifyExecutionResult(request, result); err == nil {
		t.Fatal("mismatched execution EmergION unexpectedly verified")
	}
}

func TestVerifyExecutionResultRejectsWrongAuthorization(t *testing.T) {
	request := executionLineageProofRequest()
	result := BindExecutionResult(request, ExecutionResult{})

	result.AuthorizationID = "Q-OTHER"

	if err := VerifyExecutionResult(request, result); err == nil {
		t.Fatal("mismatched authorization unexpectedly verified")
	}
}

func TestVerifyExecutionResultRejectsWrongAuthority(t *testing.T) {
	request := executionLineageProofRequest()
	result := BindExecutionResult(request, ExecutionResult{})

	result.Authority = "OTHER"

	if err := VerifyExecutionResult(request, result); err == nil {
		t.Fatal("mismatched authority unexpectedly verified")
	}
}

func TestVerifyExecutionResultRejectsWrongAdapter(t *testing.T) {
	request := executionLineageProofRequest()
	result := BindExecutionResult(request, ExecutionResult{})

	result.Adapter = "EMAIL"

	if err := VerifyExecutionResult(request, result); err == nil {
		t.Fatal("mismatched adapter unexpectedly verified")
	}
}

func TestVerifyExecutionResultRejectsWrongAction(t *testing.T) {
	request := executionLineageProofRequest()
	result := BindExecutionResult(request, ExecutionResult{})

	result.Action = "SEND"

	if err := VerifyExecutionResult(request, result); err == nil {
		t.Fatal("mismatched action unexpectedly verified")
	}
}

func TestFailedExecutionRetainsLineage(t *testing.T) {
	request := executionLineageProofRequest()

	result := BindExecutionResult(
		request,
		ExecutionResult{
			Succeeded: false,
			Error:     "bounded execution failure",
		},
	)

	if err := VerifyExecutionResult(request, result); err != nil {
		t.Fatal(err)
	}

	if result.Error == "" {
		t.Fatal("failed execution lost error observation")
	}
}

func TestExecutionLineageCarriesTravellingEmergIONIdentity(t *testing.T) {
	request := ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",

		// This is the pre-run transition identity.
		// The current architecture does not carry it yet.
		TransitionEmergIONID: "E-TRANSITION-PROOF",
	}

	result := BindExecutionResult(
		request,
		ExecutionResult{
			Succeeded: true,
			Output:    "transition proof",
		},
	)

	if err := VerifyExecutionResult(request, result); err != nil {
		t.Fatal(err)
	}

	if result.TransitionEmergIONID != request.TransitionEmergIONID {
		t.Fatalf(
			"travelling EmergION identity = %q want %q",
			result.TransitionEmergIONID,
			request.TransitionEmergIONID,
		)
	}
}
