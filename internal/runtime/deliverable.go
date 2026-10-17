package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

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

type DeliverableReceipt struct {
	EmergIONID           string    `json:"emergion_id"`
	EvidenceHash         string    `json:"evidence_sha256"`
	SourceEmergIONID     string    `json:"source_emergion_id"`
	SourceHash           string    `json:"source_hash"`
	TransitionEmergIONID string    `json:"transition_emergion_id"`
	AuthorizationID      string    `json:"authorization_event"`
	Authority            string    `json:"authority"`
	Adapter              string    `json:"adapter"`
	Action               string    `json:"action"`
	OutputSHA256         string    `json:"output_sha256"`
	FieldTip             string    `json:"field_tip"`
	PublishedAt          time.Time `json:"published_at"`
}

type DeliverableIndex struct {
	FieldTip     string               `json:"field_tip"`
	Deliverables []DeliverableReceipt `json:"deliverables"`
}

func deliverableFileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:]), nil
}

func publishDeliverableIndex(root, fieldTip string, receipts []DeliverableReceipt) (string, error) {
	dir := filepath.Join(root, "deliverables")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	index := DeliverableIndex{
		FieldTip:     fieldTip,
		Deliverables: receipts,
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(dir, ".index-*.json")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	final := filepath.Join(dir, "index.json")
	if err := os.Rename(tmpPath, final); err != nil {
		return "", fmt.Errorf("publish deliverable index: %w", err)
	}
	return final, nil
}

func (r Runtime) PublishAcceptedDeliverables(out string) ([]string, error) {
	if r.Store == nil {
		return nil, fmt.Errorf("runtime store not configured")
	}

	events, err := r.Store.Events()
	if err != nil {
		return nil, err
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0)
	for id, em := range st.Accepted {
		if em.REL["source_kind"] == "EXECUTION_RESULT" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	published := make([]string, 0, len(ids))
	receipts := make([]DeliverableReceipt, 0, len(ids))

	for _, id := range ids {
		deliverable, err := r.MaterializeAcceptedExecutionResult(id)
		if err != nil {
			if errors.Is(err, ErrExecutionResultUnsuccessful) {
				continue
			}
			return published, err
		}

		receipt, _, err := PublishDeliverable(out, deliverable)
		if err != nil {
			return published, err
		}
		receipts = append(receipts, receipt)
		published = append(published, id)
	}

	if _, err := publishDeliverableIndex(out, st.TipHash, receipts); err != nil {
		return published, err
	}

	return published, nil
}

func PublishDeliverable(root string, d Deliverable) (DeliverableReceipt, string, error) {
	dir := filepath.Join(root, "deliverables", d.EmergIONID)
	outputFinal := filepath.Join(dir, "output")
	receiptFinal := filepath.Join(dir, "receipt.json")

	if receiptBytes, err := os.ReadFile(receiptFinal); err == nil {
		var existing DeliverableReceipt
		if json.Unmarshal(receiptBytes, &existing) == nil {
			outputHash, hashErr := deliverableFileHash(outputFinal)
			expectedHash := sha256.Sum256([]byte(d.Output))
			expectedOutputHash := fmt.Sprintf("%x", expectedHash[:])

			if hashErr == nil &&
				existing.EmergIONID == d.EmergIONID &&
				existing.EvidenceHash == d.EvidenceHash &&
				existing.SourceEmergIONID == d.SourceEmergIONID &&
				existing.SourceHash == d.SourceHash &&
				existing.TransitionEmergIONID == d.TransitionEmergIONID &&
				existing.AuthorizationID == d.AuthorizationID &&
				existing.Authority == d.Authority &&
				existing.Adapter == d.Adapter &&
				existing.Action == d.Action &&
				existing.OutputSHA256 == outputHash &&
				existing.OutputSHA256 == expectedOutputHash &&
				existing.FieldTip != "" &&
				!existing.PublishedAt.IsZero() {
				return existing, outputFinal, nil
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DeliverableReceipt{}, "", err
	}

	tmp, err := os.MkdirTemp(dir, ".publish-*")
	if err != nil {
		return DeliverableReceipt{}, "", err
	}
	defer os.RemoveAll(tmp)

	outputTmp := filepath.Join(tmp, "output")
	if err := os.WriteFile(outputTmp, []byte(d.Output), 0o644); err != nil {
		return DeliverableReceipt{}, "", err
	}

	outputHash, err := deliverableFileHash(outputTmp)
	if err != nil {
		return DeliverableReceipt{}, "", err
	}

	receipt := DeliverableReceipt{
		EmergIONID:           d.EmergIONID,
		EvidenceHash:         d.EvidenceHash,
		SourceEmergIONID:     d.SourceEmergIONID,
		SourceHash:           d.SourceHash,
		TransitionEmergIONID: d.TransitionEmergIONID,
		AuthorizationID:      d.AuthorizationID,
		Authority:            d.Authority,
		Adapter:              d.Adapter,
		Action:               d.Action,
		OutputSHA256:         outputHash,
		FieldTip:             d.FieldTip,
		PublishedAt:          time.Now().UTC(),
	}

	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return DeliverableReceipt{}, "", err
	}

	receiptTmp := filepath.Join(tmp, "receipt.json")
	if err := os.WriteFile(receiptTmp, receiptBytes, 0o644); err != nil {
		return DeliverableReceipt{}, "", err
	}

	if err := os.Rename(outputTmp, outputFinal); err != nil {
		return DeliverableReceipt{}, "", fmt.Errorf("publish deliverable output: %w", err)
	}
	if err := os.Rename(receiptTmp, receiptFinal); err != nil {
		return DeliverableReceipt{}, "", fmt.Errorf("publish deliverable receipt: %w", err)
	}

	return receipt, outputFinal, nil
}
