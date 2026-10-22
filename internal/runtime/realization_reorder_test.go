package runtime

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func acceptedExecutionResultFixture(
	id, parent, adapter, action string,
	succeeded bool,
) core.EmergION {
	fact := "execution_failed"
	if succeeded { fact = "execution_succeeded" }
	return core.EmergION{
		IDN: id,
		STA: core.StateAccepted,
		REL: map[string]string{
			"source_kind": "EXECUTION_RESULT",
			"kin_emergion": parent,
			"adapter": adapter,
			"action": action,
		},
		VAL: core.Validation{
			Facts: []string{"execution_result_observed", fact},
		},
	}
}

func realizationStateFixture(ids ...string) core.State {
	st := core.EmptyState()
	for _, id := range ids {
		st.Accepted[id] = core.EmergION{IDN: id, STA: core.StateAccepted}
	}
	return st
}

func TestReorderRealizationPopulationObservedExecutionPromotesRunnerUp(t *testing.T) {
	st := realizationStateFixture("E-A", "E-B")
	st.Accepted["R-A"] = acceptedExecutionResultFixture("R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", false)
	st.Accepted["R-B"] = acceptedExecutionResultFixture("R-B", "E-B", "LOCAL_GEMMA", "ANALYZE", true)

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: "E-A",
			Right: "E-B",
			Result: RealizationLeftPreferred,
			Differences: []string{"INITIAL:A>B"},
		}},
	}

	updated, order, err := ReorderRealizationPopulationFromObservedExecution(st, p)
	if err != nil { t.Fatal(err) }
	if updated.Comparisons[0].Result != RealizationRightPreferred {
		t.Fatalf("comparison = %s want %s", updated.Comparisons[0].Result, RealizationRightPreferred)
	}
	if got := order.Strongest(); len(got) != 1 || got[0] != "E-B" {
		t.Fatalf("strongest = %#v want [E-B]", got)
	}
	if len(updated.Comparisons[0].Differences) != 1 ||
		updated.Comparisons[0].Differences[0] != "OBSERVED_EXECUTION:LOCAL_GEMMA:ANALYZE:R-A=false:R-B=true" {
		t.Fatalf("Difference = %#v", updated.Comparisons[0].Differences)
	}
}

func TestReorderRealizationPopulationRequiresBothObservedOutcomes(t *testing.T) {
	st := realizationStateFixture("E-A", "E-B")
	st.Accepted["R-A"] = acceptedExecutionResultFixture("R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", false)

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: "E-A",
			Right: "E-B",
			Result: RealizationLeftPreferred,
			Differences: []string{"INITIAL:A>B"},
		}},
	}

	updated, order, err := ReorderRealizationPopulationFromObservedExecution(st, p)
	if err != nil { t.Fatal(err) }
	if updated.Comparisons[0].Result != RealizationLeftPreferred {
		t.Fatalf("single outcome changed comparison to %s", updated.Comparisons[0].Result)
	}
	if got := order.Strongest(); len(got) != 1 || got[0] != "E-A" {
		t.Fatalf("strongest = %#v want [E-A]", got)
	}
}

func TestReorderRealizationPopulationDoesNotCompareDifferentActions(t *testing.T) {
	st := realizationStateFixture("E-A", "E-B")
	st.Accepted["R-A"] = acceptedExecutionResultFixture("R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", false)
	st.Accepted["R-B"] = acceptedExecutionResultFixture("R-B", "E-B", "LOCAL_GEMMA", "DRAFT", true)

	p := RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
		Comparisons: []RealizationComparison{{
			TargetThing: "KNIFE",
			Left: "E-A",
			Right: "E-B",
			Result: RealizationLeftPreferred,
			Differences: []string{"INITIAL:A>B"},
		}},
	}

	updated, order, err := ReorderRealizationPopulationFromObservedExecution(st, p)
	if err != nil { t.Fatal(err) }
	if updated.Comparisons[0].Result != RealizationLeftPreferred {
		t.Fatalf("different action changed comparison to %s", updated.Comparisons[0].Result)
	}
	if got := order.Strongest(); len(got) != 1 || got[0] != "E-A" {
		t.Fatalf("strongest = %#v want [E-A]", got)
	}
}
