package runtime

import (
	"fmt"

	"emergion-sovereign-runtime/internal/adapters"
	livefield "emergion-sovereign-runtime/internal/field"
)

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
		return Deliverable{}, err
	}
	if !result.Succeeded {
		return Deliverable{}, fmt.Errorf("execution result did not succeed: %s", id)
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
