package runtime

import (
	"context"
	"fmt"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
)

// CaptureGovernedExecutionResult is the strict execution-result admission seam.
//
// # ACCEPTED → EXECUTION REQUEST → EXECUTION RESULT → SIGNAL → G
//
// RESULT != TRUTH.
// EXECUTOR != AUTHORITY.
// Lineage continuity is required before the result may re-enter the
// governed EmergION circulation.
func (r *Runtime) CaptureGovernedExecutionResult(
	ctx context.Context,
	request adapters.ExecutionRequest,
	result adapters.ExecutionResult,
) (core.EmergION, bool, error) {
	if err := adapters.VerifyExecutionResult(request, result); err != nil {
		return core.EmergION{}, false, fmt.Errorf(
			"execution lineage verification failed: %w",
			err,
		)
	}

	if err := r.verifyExactQAuthorization(result); err != nil {
		return core.EmergION{}, false, fmt.Errorf(
			"execution authorization verification failed: %w",
			err,
		)
	}

	return r.CaptureExecutionResult(ctx, request, result)
}

// verifyExactQAuthorization re-resolves the result's AuthorizationID against
// canonical Q receipts rebuilt from COSL before recapture. A divergence
// fails closed and does not enter governed circulation.
func (r *Runtime) verifyExactQAuthorization(result adapters.ExecutionResult) error {
	if r.Store == nil {
		return fmt.Errorf("runtime store not configured")
	}

	events, err := r.Store.Events()
	if err != nil {
		return err
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		return err
	}

	return adapters.VerifyCanonicalActionAuthorization(st, result)
}
