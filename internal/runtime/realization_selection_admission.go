package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

func realizationSelectionEvidence(
	population RealizationPopulation,
	selection RealizationSelection,
) []byte {
	var b strings.Builder
	b.WriteString("REALIZATION_SELECTION/1\n")
	b.WriteString("TARGET_THING:")
	b.WriteString(selection.TargetThing)
	b.WriteByte('\n')
	b.WriteString("SELECTED:")
	b.WriteString(selection.Selected)
	b.WriteByte('\n')

	runners := append([]string(nil), selection.RunnersUp...)
	sort.Strings(runners)
	for _, id := range runners {
		b.WriteString("RUNNER_UP:")
		b.WriteString(id)
		b.WriteByte('\n')
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
		b.WriteString("COMPARISON:")
		b.WriteString(comparison.Left)
		b.WriteByte('|')
		b.WriteString(comparison.Right)
		b.WriteByte('|')
		b.WriteString(string(comparison.Result))
		b.WriteByte('\n')

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

// CaptureRealizationSelection derives a unique strongest realization and
// admits that deterministic selection as ordinary governed evidence.
//
// It does not accept, authorize, or execute the selected realization.
func (r Runtime) CaptureRealizationSelection(
	ctx context.Context,
	population RealizationPopulation,
) (core.EmergION, bool, error) {
	selection, selected, err := SelectStrongestRealization(population)
	if err != nil {
		return core.EmergION{}, false, err
	}
	if !selected {
		return core.EmergION{}, false, fmt.Errorf(
			"realization selection unresolved for target thing %q",
			population.TargetThing,
		)
	}

	selectionRuntime := r
	selectionRuntime.Reasoner = fixedReasoner{
		name:    "realization-selection",
		version: "v1",
		result: reason.Result{
			Summary: "evidence-bound strongest realization selection",
			Relationships: map[string]string{
				"source_kind":          "REALIZATION_SELECTION",
				"target_thing":         selection.TargetThing,
				"selected_realization": selection.Selected,
				"runners_up":           strings.Join(selection.RunnersUp, ","),
			},
			Capabilities: []string{"CMP", "DIF"},
			Facts:        []string{"realization_selection_derived"},
			Risk:         "L",
		},
	}

	return selectionRuntime.CaptureBytes(
		ctx,
		"realization-selection",
		realizationSelectionEvidence(population, selection),
		"governed_realization_selection",
	)
}
