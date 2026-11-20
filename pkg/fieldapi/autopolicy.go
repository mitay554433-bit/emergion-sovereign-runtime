package fieldapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/policy"
)

// AutoPolicyDefaultBatch bounds one auto-decision pass.
const AutoPolicyDefaultBatch = 200

// PolicyStatus is the read-only A01-AUTO status for /status and /living.
type PolicyStatus struct {
	On            bool   `json:"on"`
	Effective     bool   `json:"effective"`
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
	SwitchID      string `json:"switch_id,omitempty"`
	SwitchVersion string `json:"switch_version,omitempty"`
	SwitchSHA256  string `json:"switch_sha256,omitempty"`
	Stale         bool   `json:"stale,omitempty"`
	AutoAccepted  int    `json:"auto_accepted"`
	AutoHeld      int    `json:"auto_held"`
	Waiting       int    `json:"waiting"`
	Eligible      int    `json:"eligible"`
}

// PolicyStatusOf derives the policy status from st (read-only).
func PolicyStatusOf(st core.State) PolicyStatus {
	ps := PolicyStatus{Version: policy.Version(), SHA256: policy.SHA256(), Waiting: len(st.AtGOV)}
	if st.AutoPolicy != nil {
		ps.On = st.AutoPolicy.On
		ps.SwitchID = st.AutoPolicy.SwitchID
		ps.SwitchVersion = st.AutoPolicy.Version
		ps.SwitchSHA256 = st.AutoPolicy.SHA256
		ps.Stale = st.AutoPolicy.On && (st.AutoPolicy.SHA256 != policy.SHA256() || st.AutoPolicy.Version != policy.Version())
		ps.Effective = st.AutoPolicy.On && !ps.Stale
	}
	for _, d := range st.AutoDecided {
		switch d {
		case "APPROVE":
			ps.AutoAccepted++
		case "HOLD":
			ps.AutoHeld++
		}
	}
	x := policy.BuildIndex(st)
	for _, em := range st.AtGOV {
		if policy.Classify(st, x, em).Decision != "" {
			ps.Eligible++
		}
	}
	return ps
}

// PolicyStatus reads the ledger and reports the policy status.
func (r *Runtime) PolicyStatus() (PolicyStatus, error) {
	st, err := r.state()
	if err != nil {
		return PolicyStatus{}, err
	}
	return PolicyStatusOf(st), nil
}

type fixedResult struct {
	name, version string
	result        reason.Result
}

func (f fixedResult) Analyze(context.Context, reason.Input) (reason.Result, error) {
	return f.result, nil
}
func (f fixedResult) Name() string                   { return f.name }
func (f fixedResult) Version(context.Context) string { return f.version }

// SetAutoPolicy is the HUMAN_FINAL press that switches the A01-AUTO policy on
// or off. It captures one AUTO_POLICY_SWITCH record naming the exact policy
// version and sha256, then records a HUMAN_FINAL APPROVE and REG receipt for
// it. Nothing changes until that REG receipt exists. A reason is required.
func (r *Runtime) SetAutoPolicy(ctx context.Context, on bool, reasonText string) (string, error) {
	reasonText = strings.TrimSpace(reasonText)
	if reasonText == "" {
		return "", fmt.Errorf("a HUMAN_FINAL reason is required")
	}
	state := "off"
	if on {
		state = "on"
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	res := reason.Result{
		Summary: fmt.Sprintf("AUTO_POLICY switch %s (%s)", strings.ToUpper(state), policy.Version()),
		Relationships: map[string]string{
			"source_kind":    policy.KindSwitch,
			"policy_state":   state,
			"policy_version": policy.Version(),
			"policy_sha256":  policy.SHA256(),
		},
		Capabilities: []string{"OBS"},
		Facts:        []string{"policy_switch_requested", "policy_sha256:" + policy.SHA256()},
		Risk:         "L",
	}
	rt := fieldruntime.Runtime{Store: r.store, Reasoner: fixedResult{name: "policy-switch", version: "v1", result: res}}
	em, _, err := rt.CaptureBytes(ctx, "auto-policy-switch", policy.SwitchDescriptor(on, reasonText, at), policy.ProvenanceSwitch)
	if err != nil {
		return "", err
	}
	if err := r.Decide(em.IDN, string(gov.Approve), "HUMAN_FINAL AUTO_POLICY switch "+strings.ToUpper(state)+": "+reasonText); err != nil {
		return em.IDN, err
	}
	return em.IDN, nil
}

// AutoResult reports one auto-decision pass.
type AutoResult struct {
	Effective bool           `json:"effective"`
	Accepted  int            `json:"accepted"`
	Held      int            `json:"held"`
	Skipped   int            `json:"skipped"`
	Classes   map[string]int `json:"classes,omitempty"`
	Error     string         `json:"error,omitempty"`
}

func sourceFileHash(root, rel string) string {
	if root == "" || rel == "" || path.Clean(rel) != rel || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return ""
	}
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unreadable"
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ApplyAutoPolicy decides up to limit AtGOV records the active policy
// classifies as low-risk. It does nothing unless a HUMAN_FINAL switch has the
// policy on with this exact version and sha256. Each decision is a normal
// decision receipt with authority AUTO_POLICY (plus a REG receipt for
// APPROVE). sourcesRoot (optional) lets SOURCE uploads be checked against the
// file bytes on this device; a mismatch leaves the record for a human.
func (r *Runtime) ApplyAutoPolicy(limit int, sourcesRoot string) (AutoResult, error) {
	if limit <= 0 {
		limit = AutoPolicyDefaultBatch
	}
	res := AutoResult{Classes: map[string]int{}}
	err := r.store.Batch(func(events []core.Event, w *store.BatchWriter) error {
		st, err := livefield.Rebuild(events)
		if err != nil {
			return err
		}
		ps := PolicyStatusOf(st)
		if !ps.Effective {
			return nil
		}
		res.Effective = true
		x := policy.BuildIndex(st)
		ids := make([]string, 0, len(st.AtGOV))
		for id := range st.AtGOV {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		done := 0
		for _, id := range ids {
			if done >= limit {
				break
			}
			em, ok := st.AtGOV[id]
			if !ok {
				continue
			}
			v := policy.Classify(st, x, em)
			if v.Decision == "" {
				continue
			}
			if v.Decision == "APPROVE" {
				ev, err := r.store.ReadEvidence(em.MEM.SourceHash)
				if err != nil {
					res.Skipped++
					continue
				}
				if err := policy.VerifyEvidence(v, em, ev, sourceFileHash(sourcesRoot, em.REL["source_path"])); err != nil {
					res.Skipped++
					continue
				}
				if err := livefield.CheckArchonymForAccept(st, em); err != nil {
					res.Skipped++
					continue
				}
			}
			decided, receipt, err := gov.DecideAuto(em, gov.Decision(v.Decision), policy.ReasonText(v, ps.SwitchID))
			if err != nil {
				return err
			}
			decisionID, err := w.SaveDecision(receipt)
			if err != nil {
				return err
			}
			delete(st.AtGOV, id)
			if st.AutoDecided == nil {
				st.AutoDecided = map[string]string{}
			}
			st.AutoDecided[id] = v.Decision
			done++
			res.Classes[v.Class]++
			if v.Decision == "HOLD" {
				st.Held[id] = decided
				res.Held++
				continue
			}
			accepted, rr, err := reg.Accept(decided, decisionID)
			if err != nil {
				return err
			}
			if _, err := w.SaveAccepted(rr); err != nil {
				return err
			}
			st.Accepted[id] = accepted
			res.Accepted++
		}
		return nil
	})
	if err != nil {
		res.Error = err.Error()
	}
	return res, err
}

// UndoAuto is the one-press HUMAN_FINAL undo of an AUTO_POLICY decision:
// an auto-accepted record is REVERTed to HELD; an auto-held record is
// RESUMEd to AtGOV. Append-only. Anything else is refused.
func (r *Runtime) UndoAuto(id, reasonText string) (string, error) {
	reasonText = strings.TrimSpace(reasonText)
	if reasonText == "" {
		return "", fmt.Errorf("a HUMAN_FINAL reason is required")
	}
	var out string
	err := r.store.Batch(func(events []core.Event, w *store.BatchWriter) error {
		st, err := livefield.Rebuild(events)
		if err != nil {
			return err
		}
		switch st.AutoDecided[id] {
		case "APPROVE":
			em, ok := st.Accepted[id]
			if !ok {
				return fmt.Errorf("record is not REG-accepted: %s", id)
			}
			_, receipt, err := gov.RevertAuto(em, "HUMAN_FINAL", "HUMAN_FINAL undo of AUTO_POLICY accept: "+reasonText)
			if err != nil {
				return err
			}
			if _, err := w.SaveDecision(receipt); err != nil {
				return err
			}
			out = "HELD"
		case "HOLD":
			em, ok := st.Held[id]
			if !ok {
				return fmt.Errorf("record is not held: %s", id)
			}
			_, receipt, err := gov.ResumeHeld(em, "HUMAN_FINAL", "HUMAN_FINAL undo of AUTO_POLICY hold: "+reasonText)
			if err != nil {
				return err
			}
			if _, err := w.SaveDecision(receipt); err != nil {
				return err
			}
			out = "AT_GOV"
		default:
			return fmt.Errorf("record %s has no AUTO_POLICY decision in force", id)
		}
		return nil
	})
	return out, err
}
