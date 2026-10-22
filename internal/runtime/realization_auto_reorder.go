package runtime

import (
	"context"
	"reflect"

	"emergion-sovereign-runtime/internal/core"
)

// CaptureNextObservedRealizationReorder reconstructs the latest REG-accepted
// realization population from its canonical evidence and admits a new reorder
// only when REG-accepted execution evidence changes that population.
//
// A produced reorder stops at AtGOV. No decision, REG receipt, Q, or execution
// is created here.
func (r Runtime) CaptureNextObservedRealizationReorder(
	ctx context.Context,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, nil
	}
	state, events, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, err
	}
	population, found, err := latestAcceptedRealizationPopulation(
		state,
		events,
		r.Store.ReadEvidence,
	)
	if err != nil || !found {
		return core.EmergION{}, false, err
	}

	updated, _, err := ReorderRealizationPopulationFromObservedExecution(
		state,
		population,
	)
	if err != nil {
		return core.EmergION{}, false, err
	}
	if reflect.DeepEqual(population.Comparisons, updated.Comparisons) {
		return core.EmergION{}, false, nil
	}

	em, duplicate, err := r.CaptureObservedRealizationReorder(ctx, population)
	if err != nil {
		return core.EmergION{}, false, err
	}
	return em, !duplicate, nil
}
