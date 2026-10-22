package runtime

import (
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
)

type RealizationRecurrence struct {
	ReorderEmergION    string
	TargetThing        string
	SelectedRealization string
	Actions            []adapters.ActionCandidate
}

// ResolveAcceptedRealizationReorder binds a REG-accepted reorder to the
// realization it selected. The selected realization must independently remain
// REG-accepted. This function derives no Q and executes nothing.
func ResolveAcceptedRealizationReorder(
	st core.State,
	reorderID string,
	localGemma bool,
) (RealizationRecurrence, error) {
	reorder, ok := st.Accepted[reorderID]
	if !ok {
		return RealizationRecurrence{}, fmt.Errorf(
			"realization reorder %q is not REG-accepted",
			reorderID,
		)
	}
	if reorder.REL["source_kind"] != "REALIZATION_REORDER" {
		return RealizationRecurrence{}, fmt.Errorf(
			"accepted EmergION %q is not a REALIZATION_REORDER",
			reorderID,
		)
	}

	selectedID := strings.TrimSpace(reorder.REL["selected_realization"])
	if selectedID == "" {
		return RealizationRecurrence{}, fmt.Errorf(
			"realization reorder %q has no selected realization",
			reorderID,
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
		ReorderEmergION:     reorder.IDN,
		TargetThing:         reorder.REL["target_thing"],
		SelectedRealization: selected.IDN,
		Actions:             actions,
	}, nil
}
