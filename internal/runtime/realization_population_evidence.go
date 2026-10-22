package runtime

import (
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

func populationFromRealizationEvidence(
	em core.EmergION,
	evidence []byte,
) (RealizationPopulation, error) {
	kind := em.REL["source_kind"]
	if kind != "REALIZATION_SELECTION" && kind != "REALIZATION_REORDER" {
		return RealizationPopulation{}, fmt.Errorf(
			"EmergION %q is not realization population evidence",
			em.IDN,
		)
	}

	target := strings.TrimSpace(em.REL["target_thing"])
	if target == "" {
		return RealizationPopulation{}, fmt.Errorf("realization target thing missing")
	}

	population := RealizationPopulation{TargetThing: target}
	members := map[string]bool{}

	for _, raw := range strings.Split(string(evidence), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "COMPARISON:") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(line, "COMPARISON:"), "|")
		if len(parts) != 3 {
			return RealizationPopulation{}, fmt.Errorf(
				"invalid realization comparison evidence %q",
				line,
			)
		}
		comparison := RealizationComparison{
			TargetThing: target,
			Left: strings.TrimSpace(parts[0]),
			Right: strings.TrimSpace(parts[1]),
			Result: RealizationComparisonResult(strings.TrimSpace(parts[2])),
		}
		population.Comparisons = append(population.Comparisons, comparison)
		members[comparison.Left] = true
		members[comparison.Right] = true
	}

	// Differences are emitted immediately after their comparison. Re-read the
	// canonical evidence to preserve that binding.
	comparisonIndex := -1
	for _, raw := range strings.Split(string(evidence), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "COMPARISON:"):
			comparisonIndex++
		case strings.HasPrefix(line, "DIFFERENCE:"):
			if comparisonIndex < 0 || comparisonIndex >= len(population.Comparisons) {
				return RealizationPopulation{}, fmt.Errorf(
					"orphan realization Difference evidence",
				)
			}
			population.Comparisons[comparisonIndex].Differences = append(
				population.Comparisons[comparisonIndex].Differences,
				strings.TrimPrefix(line, "DIFFERENCE:"),
			)
		}
	}

	for id := range members {
		if strings.TrimSpace(id) == "" {
			return RealizationPopulation{}, fmt.Errorf("empty realization member")
		}
		population.Members = append(population.Members, id)
	}
	sort.Strings(population.Members)
	if len(population.Comparisons) == 0 {
		return RealizationPopulation{}, fmt.Errorf("realization comparisons missing")
	}
	return population, nil
}

func latestAcceptedRealizationPopulation(
	st core.State,
	events []core.Event,
	readEvidence func(string) ([]byte, error),
) (RealizationPopulation, bool, error) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].REG == nil {
			continue
		}
		id := events[i].REG.EmergIONID
		em, ok := st.Accepted[id]
		if !ok {
			continue
		}
		kind := em.REL["source_kind"]
		if kind != "REALIZATION_REORDER" && kind != "REALIZATION_SELECTION" {
			continue
		}
		evidence, err := readEvidence(em.MEM.SourceHash)
		if err != nil {
			return RealizationPopulation{}, false, err
		}
		population, err := populationFromRealizationEvidence(em, evidence)
		if err != nil {
			return RealizationPopulation{}, false, err
		}
		return population, true, nil
	}
	return RealizationPopulation{}, false, nil
}
