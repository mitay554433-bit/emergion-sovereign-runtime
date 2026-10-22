package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

func realizationReorderEvidence(
	population RealizationPopulation,
	order RealizationOrder,
) []byte {
	var b strings.Builder
	b.WriteString("REALIZATION_REORDER/1\n")
	b.WriteString("TARGET_THING:")
	b.WriteString(population.TargetThing)
	b.WriteByte('\n')

	for i, front := range order.Fronts {
		members := append([]string(nil), front...)
		sort.Strings(members)
		for _, id := range members {
			fmt.Fprintf(&b, "FRONT:%d:%s\n", i, id)
		}
	}

	comparisons := append([]RealizationComparison(nil), population.Comparisons...)
	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].Left != comparisons[j].Left {
			return comparisons[i].Left < comparisons[j].Left
		}
		if comparisons[i].Right != comparisons[j].Right {
			return comparisons[i].Right < comparisons[j].Right
		}
		return comparisons[i].Result < comparisons[j].Result
	})
	for _, comparison := range comparisons {
		fmt.Fprintf(
			&b,
			"COMPARISON:%s|%s|%s\n",
			comparison.Left,
			comparison.Right,
			comparison.Result,
		)
		differences := append([]string(nil), comparison.Differences...)
		sort.Strings(differences)
		for _, difference := range differences {
			b.WriteString("DIFFERENCE:")
			b.WriteString(difference)
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// CaptureObservedRealizationReorder derives a reorder only from current
// governed execution evidence and admits that transition through the ordinary
// RECOIL/WVC -> AtGOV membrane.
//
// It does not accept the reorder, authorize a realization, or execute it.
func (r Runtime) CaptureObservedRealizationReorder(
	ctx context.Context,
	population RealizationPopulation,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	state, _, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, err
	}

	updated, order, err := ReorderRealizationPopulationFromObservedExecution(
		state,
		population,
	)
	if err != nil {
		return core.EmergION{}, false, err
	}

	strongest := order.Strongest()
	if len(strongest) != 1 {
		return core.EmergION{}, false, fmt.Errorf(
			"realization reorder unresolved for target thing %q",
			population.TargetThing,
		)
	}

	runners := []string{}
	for _, front := range order.Fronts[1:] {
		runners = append(runners, front...)
	}

	reorderRuntime := r
	reorderRuntime.Reasoner = fixedReasoner{
		name:    "realization-reorder",
		version: "v1",
		result: reason.Result{
			Summary: "governed execution evidence realization reorder",
			Relationships: map[string]string{
				"source_kind":          "REALIZATION_REORDER",
				"target_thing":         updated.TargetThing,
				"selected_realization": strongest[0],
				"runners_up":           strings.Join(runners, ","),
			},
			Capabilities: []string{"OBS", "CMP", "DIF"},
			Facts:        []string{"realization_reorder_derived"},
			Risk:         "L",
		},
	}

	return reorderRuntime.CaptureBytes(
		ctx,
		"realization-reorder",
		realizationReorderEvidence(updated, order),
		"governed_realization_reorder",
	)
}
