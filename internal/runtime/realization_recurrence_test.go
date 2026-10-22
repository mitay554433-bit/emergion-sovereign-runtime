package runtime

import (
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func recurrenceStateFixture() core.State {
	st := core.EmptyState()
	st.Accepted["E-B"] = core.EmergION{
		IDN: "E-B",
		STA: core.StateAccepted,
		CAP: []string{"ANALYZE", "SEND"},
	}
	st.Accepted["E-R"] = core.EmergION{
		IDN: "E-R",
		STA: core.StateAccepted,
		REL: map[string]string{
			"source_kind": "REALIZATION_REORDER",
			"target_thing": "KNIFE",
			"selected_realization": "E-B",
			"runners_up": "E-A",
		},
	}
	return st
}

func TestResolveAcceptedRealizationReorderTargetsSelectedREGRealization(t *testing.T) {
	st := recurrenceStateFixture()
	got, err := ResolveAcceptedRealizationReorder(st, "E-R", true)
	if err != nil { t.Fatal(err) }
	if got.ReorderEmergION != "E-R" {
		t.Fatalf("reorder = %q", got.ReorderEmergION)
	}
	if got.SelectedRealization != "E-B" {
		t.Fatalf("selected = %q want E-B", got.SelectedRealization)
	}
	if got.TargetThing != "KNIFE" {
		t.Fatalf("target = %q want KNIFE", got.TargetThing)
	}

	foundAnalyze := false
	foundSend := false
	for _, action := range got.Actions {
		switch {
		case action.Adapter == "LOCAL_GEMMA" && action.Action == "ANALYZE":
			foundAnalyze = true
		case action.Action == "SEND":
			foundSend = true
			if !action.HumanFinalRequired {
				t.Fatal("SEND recurrence action lost HUMAN_FINAL requirement")
			}
		}
	}
	if !foundAnalyze {
		t.Fatal("selected realization ANALYZE action not derived")
	}
	if !foundSend {
		t.Fatal("selected realization SEND action not derived")
	}
}

func TestResolveAcceptedRealizationReorderRejectsAtGOVReorder(t *testing.T) {
	st := recurrenceStateFixture()
	reorder := st.Accepted["E-R"]
	delete(st.Accepted, "E-R")
	st.AtGOV["E-R"] = reorder

	_, err := ResolveAcceptedRealizationReorder(st, "E-R", true)
	if err == nil || !strings.Contains(err.Error(), "not REG-accepted") {
		t.Fatalf("err = %v want reorder REG rejection", err)
	}
}

func TestResolveAcceptedRealizationReorderRejectsNonREGSelectedRealization(t *testing.T) {
	st := recurrenceStateFixture()
	selected := st.Accepted["E-B"]
	delete(st.Accepted, "E-B")
	st.AtGOV["E-B"] = selected

	_, err := ResolveAcceptedRealizationReorder(st, "E-R", true)
	if err == nil || !strings.Contains(err.Error(), "selected realization") {
		t.Fatalf("err = %v want selected realization REG rejection", err)
	}
}

func TestResolveAcceptedRealizationReorderCreatesNoAuthorization(t *testing.T) {
	st := recurrenceStateFixture()
	before := len(st.ActionAuthorizations)
	got, err := ResolveAcceptedRealizationReorder(st, "E-R", true)
	if err != nil { t.Fatal(err) }
	if len(st.ActionAuthorizations) != before {
		t.Fatal("recurrence resolution created authorization")
	}
	for _, action := range got.Actions {
		if action.Action == "SEND" && !action.HumanFinalRequired {
			t.Fatal("HUMAN_FINAL action became ungated")
		}
	}
}
