package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"emergion-sovereign-runtime/internal/adapters"
	livefield "emergion-sovereign-runtime/internal/field"
)

var ErrExecutionResultUnsuccessful = errors.New("execution result did not succeed")

type Deliverable struct {
	EmergIONID           string
	EvidenceHash         string
	SourceEmergIONID     string
	SourceHash           string
	TransitionEmergIONID string
	AuthorizationID      string
	Authority            string
	Adapter              string
	Action               string
	Output               string
	FieldTip             string
}

type historicalExecutionSignalV1 struct {
	Schema         string `json:"schema"`
	SourceKind     string `json:"source_kind"`
	ParentEmergION string `json:"parent_emergion"`
	SourceHash     string `json:"source_hash"`
	Authority      string `json:"authority"`
	Adapter        string `json:"adapter"`
	Action         string `json:"action"`
	Succeeded      bool   `json:"succeeded"`
	Output         string `json:"output"`
}

func parseHistoricalExecutionSignalV1(content []byte) (adapters.ExecutionResult, error) {
	var signal historicalExecutionSignalV1
	if err := json.Unmarshal(content, &signal); err != nil {
		return adapters.ExecutionResult{}, err
	}
	if signal.Schema != "EXECUTION_SIGNAL_V1" || signal.SourceKind != "EXECUTION_RESULT" {
		return adapters.ExecutionResult{}, fmt.Errorf("not an EXECUTION_SIGNAL_V1 execution result")
	}
	if signal.ParentEmergION == "" || signal.SourceHash == "" || signal.Authority == "" || signal.Adapter == "" || signal.Action == "" {
		return adapters.ExecutionResult{}, fmt.Errorf("incomplete EXECUTION_SIGNAL_V1 execution result")
	}
	return adapters.ExecutionResult{
		EmergIONID: signal.ParentEmergION,
		SourceHash: signal.SourceHash,
		Authority:  signal.Authority,
		Adapter:    signal.Adapter,
		Action:     signal.Action,
		Succeeded:  signal.Succeeded,
		Output:     signal.Output,
	}, nil
}

func (r Runtime) MaterializeAcceptedExecutionResult(id string) (Deliverable, error) {
	if r.Store == nil {
		return Deliverable{}, fmt.Errorf("runtime store not configured")
	}

	events, err := r.Store.Events()
	if err != nil {
		return Deliverable{}, err
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		return Deliverable{}, err
	}

	em, ok := st.Accepted[id]
	if !ok {
		return Deliverable{}, fmt.Errorf("deliverable source is not REG-accepted: %s", id)
	}
	if em.REL["source_kind"] != "EXECUTION_RESULT" {
		return Deliverable{}, fmt.Errorf("deliverable source is not an execution result: %s", id)
	}

	evidence, err := r.Store.ReadEvidence(em.MEM.SourceHash)
	if err != nil {
		return Deliverable{}, err
	}
	result, err := adapters.ParseExecutionResultBytes(evidence)
	if err != nil {
		var envelope struct {
			Schema string `json:"schema"`
		}
		if json.Unmarshal(evidence, &envelope) != nil || envelope.Schema != "EXECUTION_SIGNAL_V1" {
			return Deliverable{}, err
		}
		result, err = parseHistoricalExecutionSignalV1(evidence)
		if err != nil {
			return Deliverable{}, err
		}
	}
	if !result.Succeeded {
		return Deliverable{}, fmt.Errorf("%w: %s", ErrExecutionResultUnsuccessful, id)
	}

	if result.EmergIONID != em.REL["parent_emergion"] ||
		result.AuthorizationID != em.REL["authorization_event"] ||
		result.Adapter != em.REL["adapter"] ||
		result.Action != em.REL["action"] {
		return Deliverable{}, fmt.Errorf("execution evidence lineage does not match accepted EmergION: %s", id)
	}

	return Deliverable{
		EmergIONID:           em.IDN,
		EvidenceHash:         em.MEM.SourceHash,
		SourceEmergIONID:     result.EmergIONID,
		SourceHash:           result.SourceHash,
		TransitionEmergIONID: em.REL["transition_emergion"],
		AuthorizationID:      result.AuthorizationID,
		Authority:            result.Authority,
		Adapter:              result.Adapter,
		Action:               result.Action,
		Output:               result.Output,
		FieldTip:             st.TipHash,
	}, nil
}
