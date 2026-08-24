package adapters

import "fmt"

// BindExecutionResult copies immutable request lineage into an execution result.
// Executors produce observations; they do not choose lineage.
func BindExecutionResult(
	request ExecutionRequest,
	result ExecutionResult,
) ExecutionResult {
	result.EmergIONID = request.EmergIONID
	result.SourceHash = request.SourceHash
	result.AuthorizationID = request.AuthorizationID
	result.TransitionEmergIONID = request.TransitionEmergIONID
	result.Authority = request.Authority
	result.Adapter = request.Adapter
	result.Action = request.Action
	return result
}

// VerifyExecutionResult checks provenance continuity only.
// RESULT != TRUTH.
func VerifyExecutionResult(
	request ExecutionRequest,
	result ExecutionResult,
) error {
	if request.EmergIONID == "" {
		return fmt.Errorf("execution request missing EmergION ID")
	}
	if request.SourceHash == "" {
		return fmt.Errorf("execution request missing source hash")
	}
	if request.Adapter == "" {
		return fmt.Errorf("execution request missing adapter")
	}
	if request.Action == "" {
		return fmt.Errorf("execution request missing action")
	}

	if result.EmergIONID != request.EmergIONID {
		return fmt.Errorf(
			"execution result EmergION mismatch: got %q want %q",
			result.EmergIONID,
			request.EmergIONID,
		)
	}

	if result.SourceHash != request.SourceHash {
		return fmt.Errorf(
			"execution result source hash mismatch: got %q want %q",
			result.SourceHash,
			request.SourceHash,
		)
	}

	if result.AuthorizationID != request.AuthorizationID {
		return fmt.Errorf(
			"execution result authorization mismatch: got %q want %q",
			result.AuthorizationID,
			request.AuthorizationID,
		)
	}

	if request.TransitionEmergIONID == "" {
		return fmt.Errorf("execution request missing travelling EmergION identity")
	}

	if result.TransitionEmergIONID != request.TransitionEmergIONID {
		return fmt.Errorf(
			"execution travelling EmergION changed: %s != %s",
			result.TransitionEmergIONID,
			request.TransitionEmergIONID,
		)
	}

	if result.Authority != request.Authority {
		return fmt.Errorf(
			"execution result authority mismatch: got %q want %q",
			result.Authority,
			request.Authority,
		)
	}

	if result.Adapter != request.Adapter {
		return fmt.Errorf(
			"execution result adapter mismatch: got %q want %q",
			result.Adapter,
			request.Adapter,
		)
	}

	if result.Action != request.Action {
		return fmt.Errorf(
			"execution result action mismatch: got %q want %q",
			result.Action,
			request.Action,
		)
	}

	return nil
}
