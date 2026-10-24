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

func acceptCapturedRealizationSelectionExecution(
	t *testing.T,
	s *store.Store,
	em core.EmergION,
) {
	t.Helper()
	approved, decision, err := gov.Decide(
		em,
		gov.Approve,
		"HUMAN_FINAL",
		"realization execution integration fixture",
	)
	if err != nil { t.Fatal(err) }
	decisionID, err := s.SaveDecision(decision)
	if err != nil { t.Fatal(err) }
	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil { t.Fatal(err) }
	if _, err := s.SaveAccepted(receipt); err != nil { t.Fatal(err) }
}

func TestSelectedRealizationQExecuteObservesConcurrently(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil { t.Fatal(err) }

	captureMember := func(name, evidence string) core.EmergION {
		memberRuntime := Runtime{
			Store: s,
			Reasoner: fixedReasoner{
				name: "realization-fixture",
				version: "v1",
				result: reason.Result{
					Summary: evidence,
					Relationships: map[string]string{"target_thing": "KNIFE"},
					Capabilities: []string{"ANALYZE"},
					Facts: []string{evidence},
					Risk: "L",
				},
			},
		}
		em, duplicate, err := memberRuntime.CaptureBytes(
			context.Background(),
			name,
			[]byte(evidence),
			"realization_execution_fixture",
		)
		if err != nil { t.Fatal(err) }
		if duplicate { t.Fatalf("%s unexpectedly duplicate", name) }
		acceptCapturedRealizationSelectionExecution(t, s, em)
		return em
	}

	a := captureMember("knife-a", "knife A source evidence")
	b := captureMember("knife-b", "knife B source evidence")

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{a.IDN, b.IDN},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: a.IDN,
			Right: b.IDN,
			Result: RealizationLeftPreferred,
			Differences: []string{"A_BETTER_THAN_B"},
		}},
	}

	r := Runtime{Store: s}
	selection, duplicate, err := r.CaptureRealizationSelection(context.Background(), p)
	if err != nil { t.Fatal(err) }
	if duplicate { t.Fatal("selection unexpectedly duplicate") }
	acceptCapturedRealizationSelectionExecution(t, s, selection)

	q, err := r.AuthorizeAction(
		a.IDN,
		"LOCAL_GEMMA",
		"ANALYZE",
		"authorize selected realization execution",
		true,
	)
	if err != nil { t.Fatal(err) }
	if q == "" { t.Fatal("authorization did not produce Q identity") }

	root := t.TempDir()
	model := filepath.Join(root, "fake.gguf")
	if err := os.WriteFile(model, []byte("model"), 0600); err != nil { t.Fatal(err) }
	binary := filepath.Join(root, "llama-cli")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo fake-gemma-v1; exit 0; fi\n" +
		"printf 'S|knife A source evidence\\nK|L\\nF|knife A source evidence\\nC|ANALYZE\\nZ\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil { t.Fatal(err) }

	gemma := reason.GemmaCLI{
		Binary: binary,
		Model: model,
		Threads: 1,
		Context: 2048,
		MaxTokens: 64,
		Timeout: 5 * time.Second,
	}

	request, result, signal, resultDuplicate, err := r.ExecuteAction(
		context.Background(),
		a.IDN,
		"LOCAL_GEMMA",
		"ANALYZE",
		gemma,
	)
	if err != nil { t.Fatal(err) }
	if resultDuplicate { t.Fatal("execution result unexpectedly duplicate") }
	if request.EmergIONID != a.IDN {
		t.Fatalf("executed %q want selected realization %q", request.EmergIONID, a.IDN)
	}
	if request.AuthorizationID != q {
		t.Fatalf("execution Q = %q want %q", request.AuthorizationID, q)
	}
	if !result.Succeeded {
		t.Fatalf("selected realization execution failed: %#v", result)
	}
	if signal.REL["kin_emergion"] != a.IDN {
		t.Fatalf("execution result parent = %q want %q", signal.REL["kin_emergion"], a.IDN)
	}
	if signal.REL["authorization_event"] != q {
		t.Fatalf("execution result Q = %q want %q", signal.REL["authorization_event"], q)
	}

	state, _, err := r.governedStateContext()
	if err != nil { t.Fatal(err) }

	foundObservation := false
	groups := []map[string]core.EmergION{
		state.AtGOV,
		state.Approved,
		state.Accepted,
		state.Held,
		state.Rejected,
		state.Returned,
	}
	for _, group := range groups {
		for _, em := range group {
			if em.REL["source_kind"] == "EXECUTION_OBSERVATION" &&
				em.REL["kin_emergion"] == a.IDN &&
				em.REL["adapter"] == "LOCAL_GEMMA" &&
				em.REL["action"] == "ANALYZE" {
				foundObservation = true
			}
		}
	}
	if !foundObservation {
		t.Fatal("selected realization execution lacked concurrent observation")
	}
}
