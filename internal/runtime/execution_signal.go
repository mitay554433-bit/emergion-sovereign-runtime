package runtime

import (
	"context"
	"fmt"
	"strings"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/pivot"
	"emergion-sovereign-runtime/internal/reason"
)

func (r Runtime) CaptureExecutionResult(
	ctx context.Context,
	request adapters.ExecutionRequest,
	result adapters.ExecutionResult,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	if request.EmergIONID == "" ||
		request.Adapter == "" ||
		request.Action == "" {
		return core.EmergION{}, false, fmt.Errorf(
			"incomplete execution request",
		)
	}

	if result.Adapter != request.Adapter ||
		result.Action != request.Action {
		return core.EmergION{}, false, fmt.Errorf(
			"execution result does not match request",
		)
	}

	var content strings.Builder
	writeField := func(key, value string) {
		fmt.Fprintf(&content, "%s=%d:", key, len(value))
		content.WriteString(value)
		content.WriteByte('\n')
	}

	writeField("S", "XS/1")
	writeField("K", "XR")
	writeField("P", request.EmergIONID)
	writeField("H", request.SourceHash)
	writeField("Q", request.AuthorizationID)
	writeField("A", request.Authority)
	writeField("D", request.Adapter)
	writeField("X", request.Action)
	writeField("Y", fmt.Sprintf("%t", result.Succeeded))
	writeField("O", result.Output)
	writeField("E", result.Error)

	facts := []string{
		"execution_result_observed",
	}

	risk := "L"
	if result.Succeeded {
		facts = append(facts, "execution_succeeded")
	} else {
		facts = append(facts, "execution_failed")
		risk = "M"
	}

	relationships := map[string]string{
		"source_kind":         "EXECUTION_RESULT",
		"parent_emergion":     request.EmergIONID,
		"transition_emergion": request.TransitionEmergIONID,
		"adapter":             request.Adapter,
		"action":              request.Action,
	}

	if request.AuthorizationID != "" {
		relationships["authorization_event"] = request.AuthorizationID
	}

	if !result.Succeeded && strings.TrimSpace(result.Error) != "" {
		required := requiredCapabilityFromDivergence(pivot.Result{
			Name:       "COVERAGE",
			Divergence: result.Error,
		})
		if required != "" {
			relationships["required_capability"] = required
		}
	}

	signalRuntime := r
	signalRuntime.Reasoner = fixedReasoner{
		name:    "execution-signal",
		version: "v1",
		result: reason.Result{
			Summary:       "bounded execution result observation",
			Relationships: relationships,
			Capabilities: []string{
				"OBS",
				"CMP",
			},
			Facts: facts,
			Risk:  risk,
		},
	}

	return signalRuntime.captureBytes(
		ctx,
		"execution-result",
		[]byte(content.String()),
		"execution_signal",
	)
}
