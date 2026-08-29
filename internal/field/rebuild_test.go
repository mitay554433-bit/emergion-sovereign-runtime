package field

import (
	"emergion-sovereign-runtime/internal/core"
	"testing"
	"time"
)

func TestRebuild(t *testing.T) {
	em := core.EmergION{IDN: "E-1", STA: core.StateAtGOV, EVO: core.Evolution{Version: 1}}
	d := core.DecisionReceipt{EmergIONID: "E-1", Decision: "APPROVE", Authority: "HUMAN_FINAL", At: time.Now()}
	r := core.REGReceipt{EmergIONID: "E-1", DecisionID: "EV-D", At: time.Now()}
	st, err := Rebuild([]core.Event{{Type: "C", ID: "EV-C", EmergION: &em}, {Type: "D", ID: "EV-D", Decision: &d}, {Type: "R", ID: "EV-R", REG: &r}})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Accepted) != 1 || len(st.AtGOV) != 0 {
		t.Fatalf("bad state %#v", st)
	}
}

func TestRebuildRejectsUnlinkedREGReceipt(t *testing.T) {
	em := core.EmergION{IDN: "E-1", STA: core.StateAtGOV, EVO: core.Evolution{Version: 1}}
	d := core.DecisionReceipt{EmergIONID: "E-1", Decision: "APPROVE", Authority: "HUMAN_FINAL", At: time.Now()}
	r := core.REGReceipt{EmergIONID: "E-1", DecisionID: "EV-OTHER", At: time.Now()}
	if _, err := Rebuild([]core.Event{{Type: "C", ID: "EV-C", EmergION: &em}, {Type: "D", ID: "EV-D", Decision: &d}, {Type: "R", ID: "EV-R", REG: &r}}); err == nil {
		t.Fatal("expected unlinked REG receipt to fail")
	}
}

func TestHeldEmergIONCanResumeToGOV(t *testing.T) {
	em := core.EmergION{
		IDN: "E-HOLD-RESUME",
		STA: core.StateAtGOV,
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-HOLD",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "HOLD",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "D",
			ID:   "EV-RESUME",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "RESUME",
				Authority:  "HUMAN_FINAL",
			},
		},
	}

	st, err := Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.Held[em.IDN]; ok {
		t.Fatal("resumed EmergION remained held")
	}

	resumed, ok := st.AtGOV[em.IDN]
	if !ok {
		t.Fatal("resumed EmergION did not return to GOV")
	}

	if resumed.STA != core.StateAtGOV {
		t.Fatalf("resumed state = %s", resumed.STA)
	}
}

func TestResumeRejectsNonHeldTarget(t *testing.T) {
	em := core.EmergION{
		IDN: "E-NOT-HELD",
		STA: core.StateAtGOV,
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-RESUME",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "RESUME",
				Authority:  "HUMAN_FINAL",
			},
		},
	}

	if _, err := Rebuild(events); err == nil {
		t.Fatal("non-held EmergION was resumed")
	}
}

func TestRejectedEmergIONRemainsTerminal(t *testing.T) {
	em := core.EmergION{
		IDN: "E-REJECTED",
		STA: core.StateAtGOV,
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-REJECT",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "REJECT",
				Authority:  "HUMAN_FINAL",
			},
		},
	}

	st, err := Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.Rejected[em.IDN]; !ok {
		t.Fatal("rejected EmergION missing from rejected state")
	}

	if _, ok := st.AtGOV[em.IDN]; ok {
		t.Fatal("rejected EmergION remained at GOV")
	}

	if _, ok := st.Held[em.IDN]; ok {
		t.Fatal("rejected EmergION entered held state")
	}

	if _, ok := st.Returned[em.IDN]; ok {
		t.Fatal("rejected EmergION entered returned state")
	}

	if _, ok := st.Accepted[em.IDN]; ok {
		t.Fatal("rejected EmergION entered accepted state")
	}
}

func TestRejectedEmergIONCannotResume(t *testing.T) {
	em := core.EmergION{
		IDN: "E-REJECTED",
		STA: core.StateAtGOV,
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-REJECT",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "REJECT",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "D",
			ID:   "EV-RESUME",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "RESUME",
				Authority:  "HUMAN_FINAL",
			},
		},
	}

	if _, err := Rebuild(events); err == nil {
		t.Fatal("rejected EmergION was resumed")
	}
}

func TestDecisionReplayRequiresHumanFinal(t *testing.T) {
	em := core.EmergION{
		IDN: "E-AUTHORITY",
		STA: core.StateAtGOV,
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-D",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "APPROVE",
				Authority:  "MODEL",
			},
		},
	}

	if _, err := Rebuild(events); err == nil {
		t.Fatal("non-HUMAN_FINAL decision replay was accepted")
	}
}

func TestExactHumanFinalREGReplayProducesAcceptedState(t *testing.T) {
	em := core.EmergION{
		IDN: "E-SCZ-EXACT",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "source",
			Bytes:      1,
			Stored:     1,
			Summary:    "governed accepted structure",
		},
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-D-APPROVE",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R",
			REG: &core.REGReceipt{
				EmergIONID: em.IDN,
				DecisionID: "EV-D-APPROVE",
			},
		},
	}

	st, err := Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	accepted, ok := st.Accepted[em.IDN]
	if !ok {
		t.Fatal("exact HUMAN_FINAL + REG replay did not enter Accepted")
	}

	if accepted.STA != core.StateAccepted {
		t.Fatalf(
			"accepted state = %q want %q",
			accepted.STA,
			core.StateAccepted,
		)
	}

	if _, ok := st.Approved[em.IDN]; ok {
		t.Fatal("REG-accepted EmergION remained in Approved")
	}
}

func TestREGReplayRejectsWrongApprovingDecisionLink(t *testing.T) {
	em := core.EmergION{
		IDN: "E-SCZ-BAD-LINK",
		STA: core.StateAtGOV,
		EVO: core.Evolution{Version: 1},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C",
			EmergION: &em,
		},
		{
			Type: "D",
			ID:   "EV-D-APPROVE",
			Decision: &core.DecisionReceipt{
				EmergIONID: em.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R",
			REG: &core.REGReceipt{
				EmergIONID: em.IDN,
				DecisionID: "EV-D-WRONG",
			},
		},
	}

	if _, err := Rebuild(events); err == nil {
		t.Fatal("REG replay accepted wrong approving decision link")
	}
}

func TestAcceptedKinRootSingleMember(t *testing.T) {
	accepted := map[string]core.EmergION{
		"E-A": {
			IDN: "E-A",
			STA: core.StateAccepted,
		},
	}

	root, err := AcceptedKinRoot(accepted, nil, "E-A")
	if err != nil {
		t.Fatal(err)
	}
	if root != "E-A" {
		t.Fatalf("root = %q want E-A", root)
	}
}

func TestAcceptedKinRootTraversesAcceptedAncestry(t *testing.T) {
	accepted := map[string]core.EmergION{
		"E-A": {
			IDN: "E-A",
			STA: core.StateAccepted,
		},
		"E-B": {
			IDN: "E-B",
			STA: core.StateAccepted,
			EVO: core.Evolution{
				Supersedes: "E-A",
			},
		},
		"E-C": {
			IDN: "E-C",
			STA: core.StateAccepted,
			EVO: core.Evolution{
				Supersedes: "E-B",
			},
		},
	}

	root, err := AcceptedKinRoot(accepted, nil, "E-C")
	if err != nil {
		t.Fatal(err)
	}
	if root != "E-A" {
		t.Fatalf("root = %q want E-A", root)
	}
}

func TestAcceptedKinRootRejectsMissingAcceptedPredecessor(t *testing.T) {
	accepted := map[string]core.EmergION{
		"E-B": {
			IDN: "E-B",
			STA: core.StateAccepted,
			EVO: core.Evolution{
				Supersedes: "E-A",
			},
		},
	}

	if _, err := AcceptedKinRoot(accepted, nil, "E-B"); err == nil {
		t.Fatal("missing accepted predecessor unexpectedly produced Kin root")
	}
}

func TestAcceptedKinRootRejectsCycle(t *testing.T) {
	accepted := map[string]core.EmergION{
		"E-A": {
			IDN: "E-A",
			STA: core.StateAccepted,
			EVO: core.Evolution{
				Supersedes: "E-B",
			},
		},
		"E-B": {
			IDN: "E-B",
			STA: core.StateAccepted,
			EVO: core.Evolution{
				Supersedes: "E-A",
			},
		},
	}

	if _, err := AcceptedKinRoot(accepted, nil, "E-A"); err == nil {
		t.Fatal("cyclic accepted Kin lineage unexpectedly produced root")
	}
}

func TestAcceptedKinRootStopsAtGovernedReturnedPredecessor(t *testing.T) {
	accepted := map[string]core.EmergION{
		"E-SUCCESSOR": {
			IDN: "E-SUCCESSOR",
			STA: core.StateAccepted,
			EVO: core.Evolution{
				Supersedes: "E-RETURNED",
			},
		},
	}

	returned := map[string]core.EmergION{
		"E-RETURNED": {
			IDN: "E-RETURNED",
			STA: core.StateReturned,
		},
	}

	root, err := AcceptedKinRoot(
		accepted,
		returned,
		"E-SUCCESSOR",
	)
	if err != nil {
		t.Fatal(err)
	}

	if root != "E-SUCCESSOR" {
		t.Fatalf("root = %q want E-SUCCESSOR", root)
	}
}

func TestREGReplayRejectsArchonymCollisionAcrossSovereignEmergIONs(t *testing.T) {
	first := core.EmergION{
		IDN: "E-ARCHONYM-FIRST",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	second := core.EmergION{
		IDN: "E-ARCHONYM-SECOND",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C-FIRST",
			EmergION: &first,
		},
		{
			Type: "D",
			ID:   "EV-D-FIRST",
			Decision: &core.DecisionReceipt{
				EmergIONID: first.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-FIRST",
			REG: &core.REGReceipt{
				EmergIONID: first.IDN,
				DecisionID: "EV-D-FIRST",
			},
		},
		{
			Type:     "C",
			ID:       "EV-C-SECOND",
			EmergION: &second,
		},
		{
			Type: "D",
			ID:   "EV-D-SECOND",
			Decision: &core.DecisionReceipt{
				EmergIONID: second.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-SECOND",
			REG: &core.REGReceipt{
				EmergIONID: second.IDN,
				DecisionID: "EV-D-SECOND",
			},
		},
	}

	if _, err := Rebuild(events); err == nil {
		t.Fatal("duplicate sovereign Archonym unexpectedly entered REG")
	}
}

func TestREGReplayAllowsArchonymContinuationThroughDirectSupersedes(t *testing.T) {
	first := core.EmergION{
		IDN: "E-ARCHONYM-ROOT",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	successor := core.EmergION{
		IDN: "E-ARCHONYM-SUCCESSOR",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version:    2,
			Supersedes: first.IDN,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C-ROOT",
			EmergION: &first,
		},
		{
			Type: "D",
			ID:   "EV-D-ROOT",
			Decision: &core.DecisionReceipt{
				EmergIONID: first.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-ROOT",
			REG: &core.REGReceipt{
				EmergIONID: first.IDN,
				DecisionID: "EV-D-ROOT",
			},
		},
		{
			Type:     "C",
			ID:       "EV-C-SUCCESSOR",
			EmergION: &successor,
		},
		{
			Type: "D",
			ID:   "EV-D-SUCCESSOR",
			Decision: &core.DecisionReceipt{
				EmergIONID: successor.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-SUCCESSOR",
			REG: &core.REGReceipt{
				EmergIONID: successor.IDN,
				DecisionID: "EV-D-SUCCESSOR",
			},
		},
	}

	st, err := Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.Accepted[first.IDN]; !ok {
		t.Fatal("Archonym predecessor disappeared from accepted state")
	}
	if _, ok := st.Accepted[successor.IDN]; !ok {
		t.Fatal("governed Archonym successor did not enter accepted state")
	}
}

func TestREGReplayAllowsArchonymContinuationAcrossAcceptedKinAncestry(t *testing.T) {
	root := core.EmergION{
		IDN: "E-ARCHONYM-KIN-A",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	middle := core.EmergION{
		IDN: "E-ARCHONYM-KIN-B",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version:    2,
			Supersedes: root.IDN,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	latest := core.EmergION{
		IDN: "E-ARCHONYM-KIN-C",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version:    3,
			Supersedes: middle.IDN,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C-KIN-A",
			EmergION: &root,
		},
		{
			Type: "D",
			ID:   "EV-D-KIN-A",
			Decision: &core.DecisionReceipt{
				EmergIONID: root.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-KIN-A",
			REG: &core.REGReceipt{
				EmergIONID: root.IDN,
				DecisionID: "EV-D-KIN-A",
			},
		},
		{
			Type:     "C",
			ID:       "EV-C-KIN-B",
			EmergION: &middle,
		},
		{
			Type: "D",
			ID:   "EV-D-KIN-B",
			Decision: &core.DecisionReceipt{
				EmergIONID: middle.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-KIN-B",
			REG: &core.REGReceipt{
				EmergIONID: middle.IDN,
				DecisionID: "EV-D-KIN-B",
			},
		},
		{
			Type:     "C",
			ID:       "EV-C-KIN-C",
			EmergION: &latest,
		},
		{
			Type: "D",
			ID:   "EV-D-KIN-C",
			Decision: &core.DecisionReceipt{
				EmergIONID: latest.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-KIN-C",
			REG: &core.REGReceipt{
				EmergIONID: latest.IDN,
				DecisionID: "EV-D-KIN-C",
			},
		},
	}

	st, err := Rebuild(events)
	if err != nil {
		t.Fatalf(
			"three-generation governed Archonym Kin continuation rejected: %v",
			err,
		)
	}

	for _, id := range []string{
		root.IDN,
		middle.IDN,
		latest.IDN,
	} {
		if _, ok := st.Accepted[id]; !ok {
			t.Fatalf(
				"Archonym Kin member %s missing from accepted state",
				id,
			)
		}
	}
}

func TestREGReplayAllowsArchonymContinuationFromGovernedReturnedPredecessor(t *testing.T) {
	returned := core.EmergION{
		IDN: "E-ARCHONYM-RETURNED",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	successor := core.EmergION{
		IDN: "E-ARCHONYM-RETURNED-SUCCESSOR",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version:    2,
			Supersedes: returned.IDN,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C-ARCHONYM-RETURNED",
			EmergION: &returned,
		},
		{
			Type: "D",
			ID:   "EV-D-ARCHONYM-RETURN",
			Decision: &core.DecisionReceipt{
				EmergIONID: returned.IDN,
				Decision:   "RETURN",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type:     "C",
			ID:       "EV-C-ARCHONYM-RETURNED-SUCCESSOR",
			EmergION: &successor,
		},
		{
			Type: "D",
			ID:   "EV-D-ARCHONYM-RETURNED-SUCCESSOR",
			Decision: &core.DecisionReceipt{
				EmergIONID: successor.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-ARCHONYM-RETURNED-SUCCESSOR",
			REG: &core.REGReceipt{
				EmergIONID: successor.IDN,
				DecisionID: "EV-D-ARCHONYM-RETURNED-SUCCESSOR",
			},
		},
	}

	st, err := Rebuild(events)
	if err != nil {
		t.Fatalf(
			"governed Archonym continuation from HUMAN_FINAL RETURNED predecessor rejected: %v",
			err,
		)
	}

	if _, ok := st.Returned[returned.IDN]; !ok {
		t.Fatal("governed returned predecessor missing from returned state")
	}

	if _, ok := st.Accepted[successor.IDN]; !ok {
		t.Fatal("governed Archonym successor did not enter accepted state")
	}
}

func TestREGReplayRejectsArchonymReuseAfterReturnWithoutReturnedKinLink(t *testing.T) {
	returned := core.EmergION{
		IDN: "E-ARCHONYM-RETURNED-OWNER",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	unlinked := core.EmergION{
		IDN: "E-ARCHONYM-UNLINKED-CLAIMANT",
		STA: core.StateAtGOV,
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Archonym: "VERITEX CORE",
			},
		},
	}

	events := []core.Event{
		{
			Type:     "C",
			ID:       "EV-C-ARCHONYM-RETURNED-OWNER",
			EmergION: &returned,
		},
		{
			Type: "D",
			ID:   "EV-D-ARCHONYM-RETURNED-OWNER",
			Decision: &core.DecisionReceipt{
				EmergIONID: returned.IDN,
				Decision:   "RETURN",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type:     "C",
			ID:       "EV-C-ARCHONYM-UNLINKED-CLAIMANT",
			EmergION: &unlinked,
		},
		{
			Type: "D",
			ID:   "EV-D-ARCHONYM-UNLINKED-CLAIMANT",
			Decision: &core.DecisionReceipt{
				EmergIONID: unlinked.IDN,
				Decision:   "APPROVE",
				Authority:  "HUMAN_FINAL",
			},
		},
		{
			Type: "R",
			ID:   "EV-R-ARCHONYM-UNLINKED-CLAIMANT",
			REG: &core.REGReceipt{
				EmergIONID: unlinked.IDN,
				DecisionID: "EV-D-ARCHONYM-UNLINKED-CLAIMANT",
			},
		},
	}

	if _, err := Rebuild(events); err == nil {
		t.Fatal("unlinked EmergION reused governed returned Archonym")
	}
}

func TestInterpretationRevisionReplacesActiveAtGOV(t *testing.T) {
	now := time.Now().UTC()

	original := core.EmergION{
		IDN: "E-INTERPRETATION-REVISION",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "same-source-hash",
			Codec:      "gzip",
			Bytes:      128,
			Stored:     64,
			Summary:    "malformed interpretation",
			Provenance: "saw_projection",
		},
		VAL: core.Validation{
			Facts:  []string{"malformed fact"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   now,
				PromptSchema: "MXPD/2",
			},
		},
	}

	revised := original
	revised.MEM.Summary = "corrected interpretation"
	revised.VAL.Facts = []string{"corrected fact"}

	decision := core.DecisionReceipt{
		EmergIONID: revised.IDN,
		Decision:   "APPROVE",
		Authority:  "HUMAN_FINAL",
		At:         now,
	}

	st, err := Rebuild([]core.Event{
		{Type: "C", ID: "EV-C", EmergION: &original},
		{Type: "I", ID: "EV-I", EmergION: &revised},
		{Type: "D", ID: "EV-D", Decision: &decision},
	})
	if err != nil {
		t.Fatal(err)
	}

	approved, ok := st.Approved[revised.IDN]
	if !ok {
		t.Fatalf("revised interpretation was not HUMAN_FINAL approved")
	}
	if approved.MEM.Summary != "corrected interpretation" {
		t.Fatalf("summary = %q", approved.MEM.Summary)
	}
	if len(approved.VAL.Facts) != 1 || approved.VAL.Facts[0] != "corrected fact" {
		t.Fatalf("facts = %#v", approved.VAL.Facts)
	}
	if len(st.AtGOV) != 0 {
		t.Fatalf("AtGOV = %#v", st.AtGOV)
	}
}

func TestInterpretationRevisionRejectsTargetNoLongerAtGOV(t *testing.T) {
	now := time.Now().UTC()

	original := core.EmergION{
		IDN: "E-INTERPRETATION-LEFT-GOV",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "same-source-hash",
			Codec:      "gzip",
			Bytes:      128,
			Stored:     64,
			Provenance: "saw_projection",
		},
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   now,
				PromptSchema: "MXPD/2",
			},
		},
	}

	hold := core.DecisionReceipt{
		EmergIONID: original.IDN,
		Decision:   "HOLD",
		Authority:  "HUMAN_FINAL",
		At:         now,
	}

	revised := original
	revised.MEM.Summary = "must not replace held state"

	_, err := Rebuild([]core.Event{
		{Type: "C", ID: "EV-C", EmergION: &original},
		{Type: "D", ID: "EV-D", Decision: &hold},
		{Type: "I", ID: "EV-I", EmergION: &revised},
	})
	if err == nil {
		t.Fatal("expected interpretation revision of non-AtGOV target to fail")
	}
}

func TestInterpretationRevisionRejectsEvidenceIdentityChange(t *testing.T) {
	now := time.Now().UTC()

	original := core.EmergION{
		IDN: "E-INTERPRETATION-EVIDENCE",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "same-source-hash",
			Codec:      "gzip",
			Bytes:      128,
			Stored:     64,
			Provenance: "saw_projection",
		},
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   now,
				PromptSchema: "MXPD/2",
			},
		},
	}

	revised := original
	revised.MEM.Codec = "changed-codec"

	_, err := Rebuild([]core.Event{
		{Type: "C", ID: "EV-C", EmergION: &original},
		{Type: "I", ID: "EV-I", EmergION: &revised},
	})
	if err == nil {
		t.Fatal("expected evidence identity change to fail")
	}
}
