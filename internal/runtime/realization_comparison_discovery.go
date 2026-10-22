package runtime

import (
	"fmt"
	"sort"

	"emergion-sovereign-runtime/internal/core"
)

// DeriveDiscoveredRealizationComparisons compares every member pair in a
// discovered same-THING population using only REG-accepted execution results.
//
// Comparable evidence requires the same adapter/action on both realizations.
// Success versus failure resolves the pair. Equal or absent outcomes remain
// UNRESOLVED. No scalar score, model preference, authority, or execution is
// created here.
func DeriveDiscoveredRealizationComparisons(
	st core.State,
	population RealizationPopulation,
) (RealizationPopulation, error) {
	if len(population.Members) < 2 {
		return RealizationPopulation{}, fmt.Errorf(
			"realization population requires at least two members",
		)
	}

	members := append([]string(nil), population.Members...)
	sort.Strings(members)

	out := RealizationPopulation{
		TargetThing: population.TargetThing,
		Members: members,
	}

	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			comparison := RealizationComparison{
				TargetThing: population.TargetThing,
				Left: members[i],
				Right: members[j],
				Result: RealizationUnresolved,
			}

			leftOutcomes := acceptedRealizationExecutionOutcomes(st, comparison.Left)
			rightOutcomes := acceptedRealizationExecutionOutcomes(st, comparison.Right)

			for _, left := range leftOutcomes {
				for _, right := range rightOutcomes {
					if left.Adapter == "" || left.Action == "" ||
						left.Adapter != right.Adapter ||
						left.Action != right.Action ||
						left.Succeeded == right.Succeeded {
						continue
					}

					if left.Succeeded {
						comparison.Result = RealizationLeftPreferred
					} else {
						comparison.Result = RealizationRightPreferred
					}
					comparison.Differences = []string{
						fmt.Sprintf(
							"OBSERVED_EXECUTION:%s:%s:%s=%t:%s=%t",
							left.Adapter,
							left.Action,
							left.ResultEmergION,
							left.Succeeded,
							right.ResultEmergION,
							right.Succeeded,
						),
					}
				}
			}

			out.Comparisons = append(out.Comparisons, comparison)
		}
	}

	if err := out.Validate(st); err != nil {
		return RealizationPopulation{}, err
	}
	return out, nil
}
