package runtime

import (
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
)

type RealizationRecurrence struct {
	SelectionEmergION  string
	TargetThing        string
	SelectedRealization string
	Actions            []adapters.ActionCandidate
}

// ResolveAcceptedRealizationRecurrence binds a REG-accepted realization selection\n// or reorder to the realization it selected. The selected realization must\n// independently remain REG-accepted. This function derives no Q and executes nothing.
func ResolveAcceptedRealizationRecurrence(
	st core.State,
	selectionID string,
	localGemma bool,
) (RealizationRecurrence, error) {
	selection, ok := st.Accepted[selectionID]
	if !ok {
		return RealizationRecurrence{}, fmt.Errorf(
			"realization selection %q is not REG-accepted",
			selectionID,
		)
	}
	kind := selection.REL["source_kind"]
	if kind != "REALIZATION_SELECTION" && kind != "REALIZATION_REORDER" {
		return RealizationRecurrence{}, fmt.Errorf(
			"accepted EmergION %q is not a governed realization selection",
			selectionID,
		)
	}

	selectedID := strings.TrimSpace(selection.REL["selected_realization"])
	if selectedID == "" {
		return RealizationRecurrence{}, fmt.Errorf(
			"realization selection %q has no selected realization",
			selectionID,
		)
	}
	selected, ok := st.Accepted[selectedID]
	if !ok {
		return RealizationRecurrence{}, fmt.Errorf(
			"selected realization %q is not REG-accepted",
			selectedID,
		)
	}

	var facets []string
	if selected.EVO.Metadata != nil {
		for _, facet := range selected.EVO.Metadata.Facets {
			facets = append(facets, string(facet))
		}
	}
	actions := adapters.DeriveActionCandidates(
		facets,
		selected.CAP,
		localGemma,
	)
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Adapter != actions[j].Adapter {
			return actions[i].Adapter < actions[j].Adapter
		}
		return actions[i].Action < actions[j].Action
	})

	return RealizationRecurrence{
		SelectionEmergION:   selection.IDN,
		TargetThing:         selection.REL["target_thing"],
		SelectedRealization: selected.IDN,
		Actions:             actions,
	}, nil
}
