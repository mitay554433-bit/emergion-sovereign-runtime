package adapters

import "testing"

func TestDeriveActionCandidatesDoesNotExecute(t *testing.T) {
	got := DeriveActionCandidates(
		[]string{"COMMUNICATIONS", "PAYMENTS_FINANCE"},
		nil,
		true,
	)

	if len(got) == 0 {
		t.Fatal("expected action candidates")
	}

	foundDraft := false
	foundSend := false
	foundTransfer := false

	for _, candidate := range got {
		switch candidate.Action {
		case "DRAFT":
			foundDraft = true
			if candidate.HumanFinalRequired {
				t.Fatal("DRAFT unexpectedly HUMAN_FINAL-bound")
			}

		case "SEND":
			foundSend = true
			if !candidate.HumanFinalRequired {
				t.Fatal("SEND must require HUMAN_FINAL")
			}

		case "TRANSFER":
			foundTransfer = true
			if !candidate.HumanFinalRequired {
				t.Fatal("TRANSFER must require HUMAN_FINAL")
			}
		}
	}

	if !foundDraft || !foundSend || !foundTransfer {
		t.Fatalf("missing expected candidates: %#v", got)
	}
}

func TestDeriveActionCandidatesIncludesDeployGate(t *testing.T) {
	got := DeriveActionCandidates(
		[]string{"PRODUCT_STORE"},
		nil,
		false,
	)

	for _, candidate := range got {
		if candidate.Action == "DEPLOY" {
			if !candidate.HumanFinalRequired {
				t.Fatal("DEPLOY must require HUMAN_FINAL")
			}
			return
		}
	}

	t.Fatal("DEPLOY candidate missing")
}

func TestDeriveActionCandidatesIsDeterministic(t *testing.T) {
	a := DeriveActionCandidates(
		[]string{"COMMUNICATIONS", "PATENT_IP"},
		[]string{"ANALYZE"},
		true,
	)
	b := DeriveActionCandidates(
		[]string{"COMMUNICATIONS", "PATENT_IP"},
		[]string{"ANALYZE"},
		true,
	)

	if len(a) != len(b) {
		t.Fatalf("candidate count differs: %d != %d", len(a), len(b))
	}

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic candidate %d: %#v != %#v", i, a[i], b[i])
		}
	}
}

func TestProgramRequiresHumanFinal(t *testing.T) {
	actions := DeriveActionCandidates(
		[]string{"PROGRAM_FORGE"},
		[]string{"PROGRAM"},
		false,
	)

	for _, action := range actions {
		if action.Adapter == "GITHUB" && action.Action == "PROGRAM" {
			if !action.HumanFinalRequired {
				t.Fatal("GITHUB:PROGRAM must require HUMAN_FINAL")
			}
			return
		}
	}

	t.Fatal("GITHUB:PROGRAM candidate missing")
}
