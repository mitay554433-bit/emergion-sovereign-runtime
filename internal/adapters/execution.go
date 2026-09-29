package adapters

import (
	"fmt"
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
