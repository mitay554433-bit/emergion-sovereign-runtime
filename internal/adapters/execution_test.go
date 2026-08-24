package adapters

import (
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
)

func executionProofState() core.State {
	st := core.EmptyState()

	st.Accepted["E-EXEC-PROOF"] = core.EmergION{
		IDN: "E-EXEC-PROOF",
		STA: core.StateAccepted,
		CAP: []string{
			"DRAFT",
			"SEND",
		},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				CapturedAt: time.Now().UTC(),
				Facets: []core.Facet{
					core.FacetCommunications,
				},
			},
		},
	}

	return st
}

func TestPrepareExecutionRejectsUnacceptedState(t *testing.T) {
	st := core.EmptyState()

	_, err := PrepareExecution(
		st,
		"E-NOT-ACCEPTED",
		"EMAIL",
		"SEND",
		false,
	)
	if err == nil {
		t.Fatal("unaccepted execution target unexpectedly prepared")
	}
}

func TestPrepareExecutionRejectsNonDerivableAction(t *testing.T) {
	st := executionProofState()

	_, err := PrepareExecution(
		st,
		"E-EXEC-PROOF",
		"PAYMENTS",
		"TRANSFER",
		false,
	)
	if err == nil {
		t.Fatal("non-derivable execution unexpectedly prepared")
	}
}

func TestPrepareExecutionRequiresQForGatedAction(t *testing.T) {
	st := executionProofState()

	_, err := PrepareExecution(
		st,
		"E-EXEC-PROOF",
		"EMAIL",
		"SEND",
		false,
	)
	if err == nil {
		t.Fatal("SEND without Q unexpectedly prepared")
	}
}

func TestPrepareExecutionAcceptsHumanFinalQ(t *testing.T) {
	st := executionProofState()

	st.ActionAuthorizations = append(
		st.ActionAuthorizations,
		core.ActionAuthorizationReceipt{
			EventID:    "EV-Q-PROOF",
			EmergIONID: "E-EXEC-PROOF",
			Adapter:    "EMAIL",
			Action:     "SEND",
			Authority:  "HUMAN_FINAL",
			Authorized: true,
			At:         time.Now().UTC(),
		},
	)

	request, err := PrepareExecution(
		st,
		"E-EXEC-PROOF",
		"EMAIL",
		"SEND",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if request.EmergIONID != "E-EXEC-PROOF" {
		t.Fatalf("unexpected EmergION: %s", request.EmergIONID)
	}
	if request.Adapter != "EMAIL" || request.Action != "SEND" {
		t.Fatalf("unexpected execution request: %#v", request)
	}
	if request.AuthorizationID != "EV-Q-PROOF" {
		t.Fatalf("authorization ID = %q want EV-Q-PROOF", request.AuthorizationID)
	}
}

func TestPrepareExecutionDoesNotMutateAuthorizationState(t *testing.T) {
	st := executionProofState()

	st.ActionAuthorizations = append(
		st.ActionAuthorizations,
		core.ActionAuthorizationReceipt{
			EventID:    "EV-Q-PROOF",
			EmergIONID: "E-EXEC-PROOF",
			Adapter:    "EMAIL",
			Action:     "SEND",
			Authority:  "HUMAN_FINAL",
			Authorized: true,
			At:         time.Now().UTC(),
		},
	)

	before := len(st.ActionAuthorizations)

	_, err := PrepareExecution(
		st,
		"E-EXEC-PROOF",
		"EMAIL",
		"SEND",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(st.ActionAuthorizations) != before {
		t.Fatal("execution preparation mutated authorization state")
	}
}

func TestPrepareExecutionRequiresHumanFinalForProgram(t *testing.T) {
	st := core.EmptyState()

	st.Accepted["E-PROGRAM-PROOF"] = core.EmergION{
		IDN: "E-PROGRAM-PROOF",
		STA: core.StateAccepted,
		CAP: []string{"PROGRAM"},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				CapturedAt: time.Now().UTC(),
				Facets: []core.Facet{
					core.FacetProgramForge,
				},
			},
		},
	}

	if _, err := PrepareExecution(
		st,
		"E-PROGRAM-PROOF",
		"GITHUB",
		"PROGRAM",
		false,
	); err == nil {
		t.Fatal("GITHUB:PROGRAM without HUMAN_FINAL authorization unexpectedly prepared")
	}

	st.ActionAuthorizations = append(
		st.ActionAuthorizations,
		core.ActionAuthorizationReceipt{
			EventID:    "EV-Q-PROGRAM",
			EmergIONID: "E-PROGRAM-PROOF",
			Adapter:    "GITHUB",
			Action:     "PROGRAM",
			Authority:  "HUMAN_FINAL",
			Authorized: true,
			At:         time.Now().UTC(),
		},
	)

	request, err := PrepareExecution(
		st,
		"E-PROGRAM-PROOF",
		"GITHUB",
		"PROGRAM",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if request.AuthorizationID != "EV-Q-PROGRAM" {
		t.Fatalf(
			"PROGRAM authorization ID = %q want EV-Q-PROGRAM",
			request.AuthorizationID,
		)
	}

	if request.Adapter != "GITHUB" || request.Action != "PROGRAM" {
		t.Fatalf("unexpected PROGRAM request: %#v", request)
	}
}

func TestPrepareExecutionBirthsTravellingEmergIONIdentity(t *testing.T) {
	st := executionProofState()

	request, err := PrepareExecution(
		st,
		"E-EXEC-PROOF",
		"EMAIL",
		"DRAFT",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if request.TransitionEmergIONID == "" {
		t.Fatal("PrepareExecution did not birth travelling EmergION identity")
	}
}
