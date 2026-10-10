package adapters

import (
	"context"
	"fmt"

	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

type LocalGemmaExecutor struct {
	Store *store.Store
	Gemma reason.GemmaCLI
}

func (e LocalGemmaExecutor) Execute(
	request ExecutionRequest,
) (ExecutionResult, error) {
	result := BindExecutionResult(request, ExecutionResult{})

	if request.Adapter != "LOCAL_GEMMA" {
		err := fmt.Errorf(
			"LOCAL_GEMMA executor cannot execute adapter %s",
			request.Adapter,
		)
		result.Error = err.Error()
		return result, err
	}

	if request.Action != "ANALYZE" && request.Action != "DRAFT" {
		err := fmt.Errorf(
			"LOCAL_GEMMA executor does not support action %s",
			request.Action,
		)
		result.Error = err.Error()
		return result, err
	}

	if e.Store == nil {
		err := fmt.Errorf("LOCAL_GEMMA executor store not configured")
		result.Error = err.Error()
		return result, err
	}

	if request.SourceHash == "" {
		err := fmt.Errorf("execution request missing source hash")
		result.Error = err.Error()
		return result, err
	}

	content, err := e.Store.ReadEvidence(request.SourceHash)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	if err := e.Gemma.Validate(); err != nil {
		result.Error = err.Error()
		return result, err
	}

	analyzed, err := e.Gemma.Analyze(
		context.Background(),
		reason.Input{
			Name:    "governed-execution-" + request.Action,
			Content: content,
		},
	)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Succeeded = true
	result.Output = reason.FormatResult(analyzed)

	return result, nil
}
