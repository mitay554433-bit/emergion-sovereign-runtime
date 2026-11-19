package runtime

import (
	"context"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
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
	request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)
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
			"source_kind":  "EXECUTION_RESULT",
			"kin_emergion": request.EmergIONID,
			"adapter":      request.Adapter,
			"action":       request.Action,
		},
	}
	signal.VAL.Facts = []string{"execution_result_observed", "execution_succeeded"}
	signal.VAL.WVC = true
	signal.VAL.Recoil = true
	for _, capability := range []string{"OBS", "VLD"} {
		if got := capabilityEvidenceCount(signal, capability); got != 0 {
			t.Fatalf("execution success alone proved %s: %d", capability, got)
		}
	}
}

func acceptExactQTarget(t *testing.T, s *store.Store, id string) core.EmergION {
	t.Helper()
	em := core.EmergION{
		IDN: id,
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: id + "-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "exact Q recapture target",
		},
		CAP: []string{"DRAFT", "SEND"},
		VAL: core.Validation{
			Facts:  []string{"communication action supported"},
			Risk:   "L",
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt: time.Now().UTC(),
				Facets: []core.Facet{
					core.FacetCommunications,
				},
			},
		},
	}
	attachPreservedSource(t, s, &em)
	if _, err := s.SaveCandidate(em); err != nil {
		t.Fatal(err)
	}
	approved, decision, err := gov.Decide(
		em,
		gov.Approve,
		"HUMAN_FINAL",
		"exact Q recapture fixture",
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
	return accepted
}

func authorizeExactQ(
	t *testing.T,
	s *store.Store,
	emergionID, adapter, action, reasonText string,
) string {
	t.Helper()
	q, err := Runtime{Store: s}.AuthorizeAction(
		emergionID,
		adapter,
		action,
		reasonText,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if q == "" {
		t.Fatal("authorization did not produce Q identity")
	}
	return q
}

func rebuildExactQState(t *testing.T, s *store.Store) core.State {
	t.Helper()
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func preparedExactQExecution(
	t *testing.T,
	s *store.Store,
	emergionID string,
) adapters.ExecutionRequest {
	t.Helper()
	request, err := adapters.PrepareExecution(
		rebuildExactQState(t, s),
		emergionID,
		"EMAIL",
		"SEND",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func bindExactQResult(request adapters.ExecutionRequest) adapters.ExecutionResult {
	return adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "bounded exact Q recapture",
		},
	)
}

func TestGovernedRecaptureAcceptsCanonicalExactQ(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	em := acceptExactQTarget(t, s, "E-EXACT-Q")
	q := authorizeExactQ(t, s, em.IDN, "EMAIL", "SEND", "authorize exact Q recapture")
	request := preparedExactQExecution(t, s, em.IDN)
	if request.AuthorizationID != q {
		t.Fatalf("prepared Q = %q want %q", request.AuthorizationID, q)
	}

	signal, duplicate, err := (&Runtime{Store: s}).CaptureGovernedExecutionResult(
		context.Background(),
		request,
		bindExactQResult(request),
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("canonical exact Q recapture unexpectedly duplicate")
	}
	if signal.REL["authorization_event"] != q {
		t.Fatalf(
			"authorization event = %q want %q",
			signal.REL["authorization_event"],
			q,
		)
	}
}

func TestGovernedRecaptureRejectsUnknownAuthorizationID(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	em := acceptExactQTarget(t, s, "E-EXACT-Q-UNKNOWN")
	authorizeExactQ(t, s, em.IDN, "EMAIL", "SEND", "authorize before unknown Q swap")
	request := preparedExactQExecution(t, s, em.IDN)
	request.AuthorizationID = "EV-Q-UNKNOWN"
	result := bindExactQResult(request)

	if _, _, err := (&Runtime{Store: s}).CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	); err == nil {
		t.Fatal("unknown authorization ID unexpectedly recaptured")
	}
}

func TestGovernedRecaptureRejectsStaleAuthorizationID(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	em := acceptExactQTarget(t, s, "E-EXACT-Q-STALE")
	stale := authorizeExactQ(t, s, em.IDN, "EMAIL", "SEND", "earlier superseded exact Q")
	current := authorizeExactQ(t, s, em.IDN, "EMAIL", "SEND", "latest exact Q")
	if stale == current {
		t.Fatal("superseding Q reused earlier event identity")
	}

	request := preparedExactQExecution(t, s, em.IDN)
	if request.AuthorizationID != current {
		t.Fatalf("prepared Q = %q want latest %q", request.AuthorizationID, current)
	}

	request.AuthorizationID = stale
	result := bindExactQResult(request)

	if _, _, err := (&Runtime{Store: s}).CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	); err == nil {
		t.Fatal("stale authorization ID unexpectedly recaptured")
	}
}

func TestGovernedRecaptureRejectsSubstitutedAuthorizationID(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	target := acceptExactQTarget(t, s, "E-EXACT-Q-TARGET")
	other := acceptExactQTarget(t, s, "E-EXACT-Q-OTHER")
	authorizeExactQ(t, s, target.IDN, "EMAIL", "SEND", "authorize target exact Q")
	substituted := authorizeExactQ(t, s, other.IDN, "EMAIL", "SEND", "authorize other EmergION exact Q")

	request := preparedExactQExecution(t, s, target.IDN)
	if request.AuthorizationID == substituted {
		t.Fatal("target Q unexpectedly equaled substituted Q")
	}

	request.AuthorizationID = substituted
	result := bindExactQResult(request)

	if _, _, err := (&Runtime{Store: s}).CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	); err == nil {
		t.Fatal("substituted authorization ID unexpectedly recaptured")
	}
}
