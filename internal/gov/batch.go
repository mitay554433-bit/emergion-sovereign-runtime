package gov

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

const (
	observationBatchProvenance = "provider_population"
	observationBatchRisk       = "L"
	observationBatchCapability = "OBS"
)

// EligibleForObservationBatch reports whether one EmergION may join a
// low-risk, observation-only provider_population batch. Anything else
// is never included.
func EligibleForObservationBatch(em core.EmergION) bool {
	if em.STA != core.StateAtGOV {
		return false
	}
	if em.MEM.Provenance != observationBatchProvenance {
		return false
	}
	if em.VAL.Risk != observationBatchRisk {
		return false
	}
	if len(em.CAP) != 1 || em.CAP[0] != observationBatchCapability {
		return false
	}
	return true
}

// SelectObservationBatch returns the eligible candidates in stable ID order.
func SelectObservationBatch(candidates []core.EmergION) []core.EmergION {
	var selected []core.EmergION
	for _, em := range candidates {
		if EligibleForObservationBatch(em) {
			selected = append(selected, em)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].IDN < selected[j].IDN
	})
	return selected
}

// BatchPreview is a dry-run of the observation batch. It writes nothing.
type BatchPreview struct {
	BatchID string
	Count   int
	IDs     []string
}

// ObservationBatchID is the hash of the exact ID list a human is shown.
// IDs are sorted first so the same set always produces the same batch ID.
func ObservationBatchID(ids []string) string {
	ordered := append([]string(nil), ids...)
	sort.Strings(ordered)
	sum := sha256.Sum256([]byte(strings.Join(ordered, "\n")))
	return hex.EncodeToString(sum[:])
}

// PreviewObservationBatch lists the current eligible set and its batch ID.
func PreviewObservationBatch(candidates []core.EmergION) BatchPreview {
	selected := SelectObservationBatch(candidates)
	ids := make([]string, len(selected))
	for i, em := range selected {
		ids[i] = em.IDN
	}
	return BatchPreview{
		BatchID: ObservationBatchID(ids),
		Count:   len(ids),
		IDs:     ids,
	}
}

func batchReason(reason, batchID string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "batch " + batchID
	}
	return reason + " [batch " + batchID + "]"
}

func resolveBatch(candidates []core.EmergION, ids []string, batchID string) ([]core.EmergION, error) {
	if strings.TrimSpace(batchID) == "" {
		return nil, fmt.Errorf("batch id required")
	}
	byID := map[string]core.EmergION{}
	for _, em := range candidates {
		if em.IDN == "" {
			continue
		}
		byID[em.IDN] = em
	}

	var selected []core.EmergION
	if len(ids) == 0 {
		selected = SelectObservationBatch(candidates)
	} else {
		seen := map[string]bool{}
		for _, id := range ids {
			if id == "" || seen[id] {
				return nil, fmt.Errorf("batch candidate %q is not eligible", id)
			}
			seen[id] = true
			em, ok := byID[id]
			if !ok || !EligibleForObservationBatch(em) {
				return nil, fmt.Errorf("batch candidate %q is not eligible", id)
			}
			selected = append(selected, em)
		}
		sort.Slice(selected, func(i, j int) bool {
			return selected[i].IDN < selected[j].IDN
		})
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("batch is empty")
	}
	got := make([]string, len(selected))
	for i, em := range selected {
		got[i] = em.IDN
	}
	if ObservationBatchID(got) != batchID {
		return nil, fmt.Errorf("batch id mismatch")
	}
	return selected, nil
}

// DecideBatch applies one HUMAN_FINAL decision to an explicit ID list, or
// to the current eligible set when ids is empty, identified by the batch
// ID the human saw in PreviewObservationBatch. Every item is re-checked
// against the selector. If any item does not qualify, or authority is not
// HUMAN_FINAL, the whole batch fails closed and no receipts are returned.
// Each item still passes through Decide and receives its own receipt;
// the reason names the batch ID. DecideBatch itself writes nothing.
func DecideBatch(candidates []core.EmergION, ids []string, d Decision, authority, reason, batchID string) ([]core.EmergION, []core.DecisionReceipt, error) {
	if authority != "HUMAN_FINAL" {
		return nil, nil, fmt.Errorf("HUMAN_FINAL required")
	}
	selected, err := resolveBatch(candidates, ids, batchID)
	if err != nil {
		return nil, nil, err
	}
	reasonText := batchReason(reason, batchID)
	updated := make([]core.EmergION, 0, len(selected))
	receipts := make([]core.DecisionReceipt, 0, len(selected))
	for _, em := range selected {
		next, receipt, err := Decide(em, d, authority, reasonText)
		if err != nil {
			return nil, nil, err
		}
		updated = append(updated, next)
		receipts = append(receipts, receipt)
	}
	return updated, receipts, nil
}
