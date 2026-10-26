package runtime

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func comparisonDiscoveryState() core.State {
	st := core.EmptyState()
	st.Accepted["E-A"] = core.EmergION{IDN: "E-A", STA: core.StateAccepted}
	st.Accepted["E-B"] = core.EmergION{IDN: "E-B", STA: core.StateAccepted}
	st.Accepted["E-C"] = core.EmergION{IDN: "E-C", STA: core.StateAccepted}
	return st
}

func addComparisonExecutionResult(
	st *core.State,
	id, parent, adapter, action string,
	succeeded bool,
) {
	fact := "execution_failed"
	if succeeded { fact = "execution_succeeded" }
	st.Accepted[id] = core.EmergION{
		IDN: id,
		STA: core.StateAccepted,
		REL: map[string]string{
			"source_kind": "EXECUTION_RESULT",
			"kin_emergion": parent,
			"adapter": adapter,
			"action": action,
		},
		VAL: core.Validation{Facts: []string{fact}},
	}
}

func TestDeriveDiscoveredRealizationComparisonsUsesComparableGovernedOutcomes(t *testing.T) {
	st := comparisonDiscoveryState()
	addComparisonExecutionResult(&st, "R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", false)
	addComparisonExecutionResult(&st, "R-B", "E-B", "LOCAL_GEMMA", "ANALYZE", true)

	population, err := DeriveDiscoveredRealizationComparisons(st, RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-B", "E-A"},
	})
	if err != nil { t.Fatal(err) }
	if len(population.Comparisons) != 1 {
		t.Fatalf("comparisons = %d want 1", len(population.Comparisons))
	}
	c := population.Comparisons[0]
	if c.Left != "E-A" || c.Right != "E-B" ||
		c.Result != RealizationRightPreferred ||
		len(c.Differences) != 1 {
		t.Fatalf("comparison = %#v", c)
	}
}

func TestDeriveDiscoveredRealizationComparisonsLeavesUnresolvedWithoutDistinguishingEvidence(t *testing.T) {
	st := comparisonDiscoveryState()
	addComparisonExecutionResult(&st, "R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", true)
	addComparisonExecutionResult(&st, "R-B", "E-B", "LOCAL_GEMMA", "ANALYZE", true)

	population, err := DeriveDiscoveredRealizationComparisons(st, RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
	})
	if err != nil { t.Fatal(err) }
	c := population.Comparisons[0]
	if c.Result != RealizationUnresolved || len(c.Differences) != 0 {
		t.Fatalf("comparison = %#v want unresolved", c)
	}
	if _, ok, err := SelectStrongestRealization(population); err != nil || ok {
		t.Fatalf("unresolved population selected strongest: ok=%t err=%v", ok, err)
	}
}

func TestDeriveDiscoveredRealizationComparisonsDoesNotCrossActions(t *testing.T) {
	st := comparisonDiscoveryState()
	addComparisonExecutionResult(&st, "R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", false)
	addComparisonExecutionResult(&st, "R-B", "E-B", "LOCAL_GEMMA", "SUMMARIZE", true)

	population, err := DeriveDiscoveredRealizationComparisons(st, RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-A", "E-B"},
	})
	if err != nil { t.Fatal(err) }
	if population.Comparisons[0].Result != RealizationUnresolved {
		t.Fatalf("different actions compared: %#v", population.Comparisons[0])
	}
}

func TestDeriveDiscoveredRealizationComparisonsPreservesPartialPopulation(t *testing.T) {
	st := comparisonDiscoveryState()
	addComparisonExecutionResult(&st, "R-A", "E-A", "LOCAL_GEMMA", "ANALYZE", true)
	addComparisonExecutionResult(&st, "R-B", "E-B", "LOCAL_GEMMA", "ANALYZE", false)

	population, err := DeriveDiscoveredRealizationComparisons(st, RealizationPopulation{
		TargetThing: "KNIFE",
		Members: []string{"E-C", "E-B", "E-A"},
	})
	if err != nil { t.Fatal(err) }
	if len(population.Comparisons) != 3 {
		t.Fatalf("comparisons = %d want 3", len(population.Comparisons))
	}
	if population.Comparisons[0].Result != RealizationLeftPreferred {
		t.Fatalf("A/B comparison = %#v", population.Comparisons[0])
	}
	if population.Comparisons[1].Result != RealizationUnresolved ||
		population.Comparisons[2].Result != RealizationUnresolved {
		t.Fatalf("unsupported pairs were resolved: %#v", population.Comparisons)
	}
}
