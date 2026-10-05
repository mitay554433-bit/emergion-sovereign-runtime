package adapters

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/store"
)

type ExecutionRequest struct {
	EmergIONID           string
	SourceHash           string
	AuthorizationID      string
	TransitionEmergIONID string
	Adapter              string
	Action               string
	Authority            string
}

type ExecutionResult struct {
	EmergIONID           string
	SourceHash           string
	AuthorizationID      string
	TransitionEmergIONID string
	Authority            string
	Adapter              string
	Action               string
	Succeeded            bool
	Output               string
	Error                string
}

func ExecutionResultBytes(request ExecutionRequest, result ExecutionResult) []byte {
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
	return []byte(content.String())
}

func ParseExecutionResultBytes(content []byte) (ExecutionResult, error) {
	fields := make(map[string]string)
	rest := content
	for len(rest) > 0 {
		eq := bytes.IndexByte(rest, '=')
		if eq <= 0 {
			return ExecutionResult{}, fmt.Errorf("invalid execution result field")
		}
		key := string(rest[:eq])
		rest = rest[eq+1:]
		colon := bytes.IndexByte(rest, ':')
		if colon <= 0 {
			return ExecutionResult{}, fmt.Errorf("invalid execution result length for %s", key)
		}
		n, err := strconv.Atoi(string(rest[:colon]))
		if err != nil || n < 0 {
			return ExecutionResult{}, fmt.Errorf("invalid execution result length for %s", key)
		}
		rest = rest[colon+1:]
		if len(rest) < n+1 || rest[n] != '\n' {
			return ExecutionResult{}, fmt.Errorf("truncated execution result field %s", key)
		}
		if _, exists := fields[key]; exists {
			return ExecutionResult{}, fmt.Errorf("duplicate execution result field %s", key)
		}
		fields[key] = string(rest[:n])
		rest = rest[n+1:]
	}

	if fields["S"] != "XS/1" || fields["K"] != "XR" {
		return ExecutionResult{}, fmt.Errorf("not an XS/1 execution result")
	}
	for _, key := range []string{"P", "H", "A", "D", "X", "Y", "O", "E"} {
		if _, ok := fields[key]; !ok {
			return ExecutionResult{}, fmt.Errorf("execution result missing field %s", key)
		}
	}
	succeeded, err := strconv.ParseBool(fields["Y"])
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("invalid execution result success value: %w", err)
	}
	return ExecutionResult{
		EmergIONID:      fields["P"],
		SourceHash:      fields["H"],
		AuthorizationID: fields["Q"],
		Authority:       fields["A"],
		Adapter:         fields["D"],
		Action:          fields["X"],
		Succeeded:       succeeded,
		Output:          fields["O"],
		Error:           fields["E"],
	}, nil
}

type Executor interface {
	Execute(ExecutionRequest) (ExecutionResult, error)
}

func ExecutionTransitionBytes(request ExecutionRequest) []byte {
	var content strings.Builder

	writeField := func(key, value string) {
		fmt.Fprintf(&content, "%s=%d:", key, len(value))
		content.WriteString(value)
		content.WriteByte('\n')
	}

	writeField("S", "XS/1")
	writeField("K", "XT")
	writeField("P", request.EmergIONID)
	writeField("H", request.SourceHash)
	writeField("Q", request.AuthorizationID)
	writeField("A", request.Authority)
	writeField("D", request.Adapter)
	writeField("X", request.Action)

	return []byte(content.String())
}

func ExecutionTransitionID(request ExecutionRequest) string {
	hash := store.Hash(ExecutionTransitionBytes(request))
	return "E-" + strings.ToUpper(hash[:16])
}

func PrepareExecution(
	st core.State,
	emergionID string,
	adapter string,
	action string,
	localGemma bool,
) (ExecutionRequest, error) {
	em, ok := st.Accepted[emergionID]
	if !ok {
		return ExecutionRequest{}, fmt.Errorf(
			"execution target is not REG-accepted: %s",
			emergionID,
		)
	}

	var facets []string
	if em.EVO.Metadata != nil {
		for _, facet := range em.EVO.Metadata.Facets {
			facets = append(facets, string(facet))
		}
	}

	var candidate *ActionCandidate
	for _, item := range DeriveActionCandidates(
		facets,
		em.CAP,
		localGemma,
	) {
		if item.Adapter == adapter && item.Action == action {
			value := item
			candidate = &value
			break
		}
	}

	if candidate == nil {
		return ExecutionRequest{}, fmt.Errorf(
			"action %s:%s is not derivable from accepted EmergION %s",
			adapter,
			action,
			emergionID,
		)
	}

	var authorization *core.ActionAuthorizationReceipt
	for i := len(st.ActionAuthorizations) - 1; i >= 0; i-- {
		item := st.ActionAuthorizations[i]

		if item.EmergIONID == emergionID &&
			item.Adapter == adapter &&
			item.Action == action &&
			item.Authorized {
			value := item
			authorization = &value
			break
		}
	}

	if candidate.HumanFinalRequired {
		if authorization == nil {
			return ExecutionRequest{}, fmt.Errorf(
				"action %s:%s requires authorization",
				adapter,
				action,
			)
		}

		if authorization.Authority != "HUMAN_FINAL" {
			return ExecutionRequest{}, fmt.Errorf(
				"action %s:%s requires HUMAN_FINAL",
				adapter,
				action,
			)
		}

		if strings.TrimSpace(authorization.EventID) == "" {
			return ExecutionRequest{}, fmt.Errorf(
				"action %s:%s authorization is missing exact COSL Q event identity",
				adapter,
				action,
			)
		}
	}

	request := ExecutionRequest{
		EmergIONID: emergionID,
		SourceHash: em.MEM.SourceHash,
		AuthorizationID: func() string {
			if authorization == nil {
				return ""
			}
			return authorization.EventID
		}(),
		Adapter:   adapter,
		Action:    action,
		Authority: candidate.Authority,
	}

	request.TransitionEmergIONID = ExecutionTransitionID(request)

	return request, nil
}
