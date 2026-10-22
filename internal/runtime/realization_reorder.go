package runtime

import (
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

type realizationExecutionOutcome struct {
	ResultEmergION string
	Adapter        string
	Action         string
	Succeeded      bool
}

func acceptedRealizationExecutionOutcomes(
	st core.State,
	realizationID string,
) []realizationExecutionOutcome {
	out := []realizationExecutionOutcome{}

	for id, em := range st.Accepted {
		if em.REL["source_kind"] != "EXECUTION_RESULT" ||
			em.REL["kin_emergion"] != realizationID {
			continue
		}

		succeeded := false
		observed := false
		for _, fact := range em.VAL.Facts {
			switch strings.TrimSpace(fact) {
			case "execution_succeeded":
				succeeded = true
				observed = true
			case "execution_failed":
				succeeded = false
				observed = true
			}
		}
		if !observed {
			continue
		}

		out = append(out, realizationExecutionOutcome{
			ResultEmergION: id,
			Adapter:        strings.TrimSpace(em.REL["adapter"]),
			Action:         strings.TrimSpace(em.REL["action"]),
			Succeeded:      succeeded,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ResultEmergION < out[j].ResultEmergION
	})
	return out
}

// ReorderRealizationPopulationFromObservedExecution derives a new descriptive
// population order from governed execution evidence.
//
// RESULT != TRUTH. A preference is changed only when both realizations have
// REG-accepted execution-result observations for the same adapter/action and
// those outcomes distinguish success from failure. It creates no authority,
// authorization, or execution event.
func ReorderRealizationPopulationFromObservedExecution(
	st core.State,
	population RealizationPopulation,
) (RealizationPopulation, RealizationOrder, error) {
	if err := population.Validate(st); err != nil {
		return RealizationPopulation{}, RealizationOrder{}, err
	}

	updated := RealizationPopulation{
		TargetThing: population.TargetThing,
		Members:     append([]string(nil), population.Members...),
		Comparisons: append([]RealizationComparison(nil), population.Comparisons...),
	}

	for i, comparison := range updated.Comparisons {
		leftOutcomes := acceptedRealizationExecutionOutcomes(st, comparison.Left)
		rightOutcomes := acceptedRealizationExecutionOutcomes(st, comparison.Right)

		var matched bool
		for _, left := range leftOutcomes {
			for _, right := range rightOutcomes {
				if left.Adapter == "" || left.Action == "" ||
					left.Adapter != right.Adapter ||
					left.Action != right.Action {
					continue
				}
				if left.Succeeded == right.Succeeded {
					continue
				}

				matched = true
				if left.Succeeded {
					updated.Comparisons[i].Result = RealizationLeftPreferred
				} else {
					updated.Comparisons[i].Result = RealizationRightPreferred
				}
				updated.Comparisons[i].Differences = []string{
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

		// With no matched distinguishing pair, preserve the existing
		// evidence-backed comparison rather than inventing a new preference.
		_ = matched
	}

	order, err := DeriveRealizationOrder(updated)
	if err != nil {
		return RealizationPopulation{}, RealizationOrder{}, err
	}
	return updated, order, nil
}
