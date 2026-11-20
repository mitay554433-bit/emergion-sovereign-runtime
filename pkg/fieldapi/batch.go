package fieldapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

// DefaultBatchCap is the largest batch one HUMAN_FINAL press may decide.
const DefaultBatchCap = 500

// BatchResult reports one batch HUMAN_FINAL press. Accepted records each got
// their own decision and REG receipt; Skipped were already in the requested
// end state; Held got their own HUMAN_FINAL HOLD receipt (decision HOLD).
type BatchResult struct {
	BatchID   string `json:"batch_id"`
	Requested int    `json:"requested"`
	Accepted  int    `json:"accepted"`
	Skipped   int    `json:"skipped"`
	Held      int    `json:"held,omitempty"`
	StoppedAt string `json:"stopped_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ValidateBatch refuses a batch before anything is written: a reason is
// required, IDs must be unique and non-empty, the human-typed confirm count
// must equal the number of IDs, and the batch may not exceed cap.
func ValidateBatch(ids []string, reason string, confirm, limit int) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("a HUMAN_FINAL reason is required")
	}
	if len(ids) == 0 {
		return fmt.Errorf("no records in batch")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return fmt.Errorf("batch IDs must be unique and non-empty")
		}
		seen[id] = true
	}
	if confirm != len(ids) {
		return fmt.Errorf("confirm count %d does not match %d records", confirm, len(ids))
	}
	if limit <= 0 {
		limit = DefaultBatchCap
	}
	if len(ids) > limit {
		return fmt.Errorf("batch of %d exceeds cap %d", len(ids), limit)
	}
	return nil
}

// batchID is derived from the tip, the IDs and the reason; it is recorded in
// the reason text only, so the receipt wire format is unchanged.
func batchID(tip string, ids []string, reason string, at time.Time) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(tip + "|" + strings.Join(sorted, ",") + "|" + reason + "|" + at.UTC().Format(time.RFC3339Nano)))
	return "HFB-" + strings.ToUpper(hex.EncodeToString(h[:6]))
}

// DecideBatchHuman records one human HUMAN_FINAL press over many AtGOV
// records. Each record goes through gov.Decide (authority HUMAN_FINAL) and
// reg.Accept and gets its own decision + REG receipt carrying the same reason
// plus the batch id. Already REG-accepted records are skipped (idempotent).
// The first error stops the batch; receipts already written stay valid.
func (r *Runtime) DecideBatchHuman(ids []string, reason string, confirm, limit int) (BatchResult, error) {
	return r.DecideBatchHumanAs(ids, reason, confirm, limit, "")
}

// ValidBatchID reports whether id has the HFB-<12 hex> form batchID produces.
func ValidBatchID(id string) bool {
	if len(id) != 16 || !strings.HasPrefix(id, "HFB-") {
		return false
	}
	for _, c := range id[4:] {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// DecideBatchHumanAs is DecideBatchHuman continuing an existing batch id, so
// one human press over a group larger than the cap can be sent as cap-sized
// chunks that share one batch id. Each chunk is still validated (reason,
// confirm count, cap) and every record still gets its own receipts. The id
// only appears in the reason text. Empty continueID derives a new one.
func (r *Runtime) DecideBatchHumanAs(ids []string, reason string, confirm, limit int, continueID string) (BatchResult, error) {
	return r.DecideBatchHumanDecision(ids, reason, confirm, limit, continueID, "")
}

// BatchDecision normalises the optional batch decision: empty or APPROVE
// approves (and REG-accepts); HOLD holds (reversible via RESUME, no REG
// acceptance). Both are existing gov decision types; nothing else is allowed.
func BatchDecision(d string) (gov.Decision, error) {
	switch strings.ToUpper(strings.TrimSpace(d)) {
	case "", "APPROVE":
		return gov.Approve, nil
	case "HOLD":
		return gov.Hold, nil
	default:
		return "", fmt.Errorf("batch decision must be APPROVE or HOLD")
	}
}

// DecideBatchHumanDecision is DecideBatchHumanAs with an explicit decision
// (APPROVE or HOLD). Every record still gets its own HUMAN_FINAL decision
// receipt; APPROVE also gets its REG acceptance. Records already in the
// requested end state (Accepted for APPROVE, Held for HOLD) are skipped.
func (r *Runtime) DecideBatchHumanDecision(ids []string, reason string, confirm, limit int, continueID, decision string) (BatchResult, error) {
	d, err := BatchDecision(decision)
	if err != nil {
		return BatchResult{Requested: len(ids)}, err
	}
	if err := ValidateBatch(ids, reason, confirm, limit); err != nil {
		return BatchResult{Requested: len(ids)}, err
	}
	continueID = strings.TrimSpace(continueID)
	if continueID != "" && !ValidBatchID(continueID) {
		return BatchResult{Requested: len(ids)}, fmt.Errorf("invalid batch id %q", continueID)
	}
	reason = strings.TrimSpace(reason)
	res := BatchResult{Requested: len(ids)}
	err = r.store.Batch(func(events []core.Event, w *store.BatchWriter) error {
		st, err := livefield.Rebuild(events)
		if err != nil {
			return err
		}
		res.BatchID = continueID
		if res.BatchID == "" {
			res.BatchID = batchID(st.TipHash, ids, reason, time.Now())
		}
		text := fmt.Sprintf("%s [HUMAN_FINAL batch %s, %d records]", reason, res.BatchID, len(ids))
		if d == gov.Hold {
			text = fmt.Sprintf("%s [HUMAN_FINAL HOLD batch %s, %d records]", reason, res.BatchID, len(ids))
		}
		for _, raw := range ids {
			id := strings.TrimSpace(raw)
			done := st.Accepted
			if d == gov.Hold {
				done = st.Held
			}
			if _, ok := done[id]; ok {
				res.Skipped++
				continue
			}
			em, ok := st.AtGOV[id]
			if !ok {
				res.StoppedAt = id
				return fmt.Errorf("candidate not at GOV: %s", id)
			}
			if d == gov.Approve {
				// Strict Archonym uniqueness at accept time, checked against
				// the state including records accepted earlier in this batch.
				if err := livefield.CheckArchonymForAccept(st, em); err != nil {
					res.StoppedAt = id
					return err
				}
			}
			em, receipt, err := gov.Decide(em, d, "HUMAN_FINAL", text)
			if err != nil {
				res.StoppedAt = id
				return err
			}
			decisionID, err := w.SaveDecision(receipt)
			if err != nil {
				res.StoppedAt = id
				return err
			}
			if d == gov.Hold {
				delete(st.AtGOV, id)
				st.Held[id] = em
				res.Held++
				continue
			}
			_, rr, err := reg.Accept(em, decisionID)
			if err != nil {
				res.StoppedAt = id
				return err
			}
			if _, err := w.SaveAccepted(rr); err != nil {
				res.StoppedAt = id
				return err
			}
			delete(st.AtGOV, id)
			st.Accepted[id] = em
			res.Accepted++
		}
		return nil
	})
	if err != nil {
		res.Error = err.Error()
	}
	return res, err
}

// SelectAtGOV lists AtGOV IDs (sorted) matching filter "all", "facet=F",
// "source_kind=K" or "target=E-…" (REL target_emergion). Read-only.
func (r *Runtime) SelectAtGOV(filter string) ([]string, error) {
	st, err := r.state()
	if err != nil {
		return nil, err
	}
	return SelectAtGOV(st, filter)
}

func SelectAtGOV(st core.State, filter string) ([]string, error) {
	key, val, _ := strings.Cut(strings.TrimSpace(filter), "=")
	key, val = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(val)
	match := func(em core.EmergION) bool { return false }
	switch key {
	case "all", "":
		match = func(core.EmergION) bool { return true }
	case "facet":
		match = func(em core.EmergION) bool {
			if em.EVO.Metadata == nil {
				return false
			}
			for _, f := range em.EVO.Metadata.Facets {
				if string(f) == val {
					return true
				}
			}
			return false
		}
	case "source_kind", "class":
		match = func(em core.EmergION) bool { return strings.TrimSpace(em.REL["source_kind"]) == val }
	case "target":
		match = func(em core.EmergION) bool { return strings.TrimSpace(em.REL["target_emergion"]) == val }
	default:
		return nil, fmt.Errorf("unknown filter %q (use all, facet=, source_kind=, target=)", filter)
	}
	var out []string
	for id, em := range st.AtGOV {
		if match(em) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}
