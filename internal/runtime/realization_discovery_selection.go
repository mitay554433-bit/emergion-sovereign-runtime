package runtime

import (
	"context"
	"fmt"

	"emergion-sovereign-runtime/internal/core"
)

// CaptureNextDiscoveredRealizationSelection closes the native discovery path:
//
// REG identities -> same-THING population -> governed outcome comparisons ->
// unique strongest -> existing governed selection admission.
//
// It admits at most one deterministic selection per call and stops at AtGOV.
// Populations without a unique evidence-supported strongest are skipped.
func (r Runtime) CaptureNextDiscoveredRealizationSelection(
	ctx context.Context,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	st, _, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, err
	}

	for _, discovered := range DiscoverRealizationPopulations(st) {
		population, err := DeriveDiscoveredRealizationComparisons(st, discovered)
		if err != nil {
			return core.EmergION{}, false, err
		}

		if _, selected, err := SelectStrongestRealization(population); err != nil {
			return core.EmergION{}, false, err
		} else if !selected {
			continue
		}

		em, duplicate, err := r.CaptureRealizationSelection(ctx, population)
		if err != nil {
			return core.EmergION{}, false, err
		}
		if !duplicate {
			return em, true, nil
		}

		// A duplicate selection that is still AtGOV remains a governance
		// boundary. Return it so callers can stop rather than fall through.
		live, _, err := r.governedStateContext()
		if err != nil {
			return core.EmergION{}, false, err
		}
		if pending, ok := live.AtGOV[em.IDN]; ok {
			return pending, true, nil
		}
	}

	return core.EmergION{}, false, nil
}
