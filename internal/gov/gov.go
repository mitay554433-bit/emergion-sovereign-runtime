package gov

import (
	"fmt"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/pivot"
)

type Decision string

const (
	Approve Decision = "APPROVE"
	Hold    Decision = "HOLD"
	Reject  Decision = "REJECT"
	Return  Decision = "RETURN"
	Resume  Decision = "RESUME"
)

func Decide(em core.EmergION, d Decision, authority, reason string) (core.EmergION, core.DecisionReceipt, error) {
	_, err := pivot.Observe(
		"GOV_DECISION",
		"REQUESTED_DECISION",
		"GOVERNED_STATE_AND_AUTHORITY",
		"HUMAN_FINAL_AND_STATE_G",
		func() error {
			if authority != "HUMAN_FINAL" {
				return fmt.Errorf("HUMAN_FINAL required")
			}
			if em.STA != core.StateAtGOV {
				return fmt.Errorf("EmergION is not at GOV")
			}
			return nil
		},
	)
	if err != nil {
		return em, core.DecisionReceipt{}, err
	}
	d = Decision(strings.ToUpper(string(d)))
	switch d {
	case Approve:
		em.STA = core.StateApproved
	case Hold:
		em.STA = core.StateHeld
	case Reject:
		em.STA = core.StateRejected
	case Return:
		em.STA = core.StateReturned
	default:
		return em, core.DecisionReceipt{}, fmt.Errorf("invalid decision")
	}
	r := core.DecisionReceipt{EmergIONID: em.IDN, Decision: string(d), Authority: authority, Reason: reason, At: time.Now().UTC()}
	return em, r, nil
}

func ResumeHeld(em core.EmergION, authority, reason string) (core.EmergION, core.DecisionReceipt, error) {
	_, err := pivot.Observe(
		"GOV_RESUME",
		"RESUME_REQUEST",
		"HELD_STATE_AND_AUTHORITY",
		"HUMAN_FINAL_AND_STATE_H",
		func() error {
			if authority != "HUMAN_FINAL" {
				return fmt.Errorf("HUMAN_FINAL required")
			}
			if em.STA != core.StateHeld {
				return fmt.Errorf("EmergION is not held")
			}
			return nil
		},
	)
	if err != nil {
		return em, core.DecisionReceipt{}, err
	}

	em.STA = core.StateAtGOV

	r := core.DecisionReceipt{
		EmergIONID: em.IDN,
		Decision:   string(Resume),
		Authority:  authority,
		Reason:     reason,
		At:         time.Now().UTC(),
	}

	return em, r, nil
}

// AutoAuthority is the decision source of the A01-AUTO policy (HUMAN_FINAL
// amendment, 2026-11-20). It is never HUMAN_FINAL.
const AutoAuthority = "AUTO_POLICY"

// Revert is the HUMAN_FINAL undo of an AUTO_POLICY REG acceptance.
const Revert Decision = "REVERT"

// DecideAuto records an AUTO_POLICY decision. Only APPROVE and HOLD exist
// for the policy; the caller must have classified em under the active policy
// (replay re-checks it). reason must carry the policy version and hash.
func DecideAuto(em core.EmergION, d Decision, reason string) (core.EmergION, core.DecisionReceipt, error) {
	_, err := pivot.Observe(
		"GOV_AUTO_DECISION",
		"AUTO_POLICY_DECISION",
		"GOVERNED_STATE_AND_POLICY",
		"AUTO_POLICY_APPROVE_OR_HOLD_AT_G",
		func() error {
			if em.STA != core.StateAtGOV {
				return fmt.Errorf("EmergION is not at GOV")
			}
			if d != Approve && d != Hold {
				return fmt.Errorf("AUTO_POLICY may only APPROVE or HOLD")
			}
			if !strings.HasPrefix(reason, "AUTO_POLICY ") {
				return fmt.Errorf("AUTO_POLICY reason must name the policy")
			}
			return nil
		},
	)
	if err != nil {
		return em, core.DecisionReceipt{}, err
	}
	if d == Approve {
		em.STA = core.StateApproved
	} else {
		em.STA = core.StateHeld
	}
	return em, core.DecisionReceipt{EmergIONID: em.IDN, Decision: string(d), Authority: AutoAuthority, Reason: reason, At: time.Now().UTC()}, nil
}

// RevertAuto is the one-press HUMAN_FINAL undo of an AUTO_POLICY accept: the
// REG-accepted record moves to HELD. The caller must check that the record
// was accepted by AUTO_POLICY (replay re-checks it).
func RevertAuto(em core.EmergION, authority, reason string) (core.EmergION, core.DecisionReceipt, error) {
	_, err := pivot.Observe(
		"GOV_REVERT",
		"REVERT_REQUEST",
		"AUTO_ACCEPTED_STATE_AND_AUTHORITY",
		"HUMAN_FINAL_AND_STATE_F",
		func() error {
			if authority != "HUMAN_FINAL" {
				return fmt.Errorf("HUMAN_FINAL required")
			}
			if em.STA != core.StateAccepted {
				return fmt.Errorf("EmergION is not REG-accepted")
			}
			if strings.TrimSpace(reason) == "" {
				return fmt.Errorf("a HUMAN_FINAL reason is required")
			}
			return nil
		},
	)
	if err != nil {
		return em, core.DecisionReceipt{}, err
	}
	em.STA = core.StateHeld
	return em, core.DecisionReceipt{EmergIONID: em.IDN, Decision: string(Revert), Authority: authority, Reason: reason, At: time.Now().UTC()}, nil
}
