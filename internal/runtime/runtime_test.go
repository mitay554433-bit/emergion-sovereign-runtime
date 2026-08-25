package runtime

import (
	"context"
	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/pivot"
	"emergion-sovereign-runtime/internal/proj"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOnceClearsDropzone(t *testing.T) {
	root := t.TempDir()
	s, _ := store.Open(filepath.Join(root, "state"))
	dz := filepath.Join(root, "drop")
	os.MkdirAll(dz, 0700)
	os.WriteFile(filepath.Join(dz, "a.txt"), []byte("hello"), 0600)
	r := Runtime{Store: s, Reasoner: reason.Heuristic{}}
	ids, err := r.Once(context.Background(), dz)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatal(ids)
	}
	ents, _ := os.ReadDir(dz)
	if len(ents) != 0 {
		t.Fatalf("dropzone not clear")
	}
}

func TestProtectorHumanFinalGate(t *testing.T) {
	em := core.EmergION{
		CAP: []string{"SEND", "TRANSFER", "DEPLOY"},
		REL: map[string]string{},
	}

	if err := protector(&em); err != nil {
		t.Fatal(err)
	}

	if em.REL["protector_gate"] != "HUMAN_FINAL_BOUND" {
		t.Fatalf("protector did not preserve HUMAN_FINAL boundary: %#v", em.REL)
	}

	if em.REL["protector"] == "" {
		t.Fatalf("protector authority envelope missing: %#v", em.REL)
	}
}

type lineageReasoner struct {
	result reason.Result
}

func (l lineageReasoner) Analyze(context.Context, reason.Input) (reason.Result, error) {
	return l.result, nil
}

func (lineageReasoner) Name() string {
	return "lineage-test"
}

func (lineageReasoner) Version(context.Context) string {
	return "1"
}

func TestCaptureRejectsUnacceptedSupersedes(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(root, "candidate.txt")
	if err := os.WriteFile(source, []byte("candidate source"), 0600); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary:       "candidate",
				Relationships: map[string]string{"source_name": "candidate.txt"},
				Capabilities:  []string{"OBS"},
				Facts:         []string{"source_preserved"},
				Risk:          "L",
				Supersedes:    "E-NOT-ACCEPTED",
				Delta:         []string{"changed behavior"},
			},
		},
	}

	_, _, err = r.Capture(context.Background(), source, false)
	if err == nil {
		t.Fatal("expected unaccepted supersedes lineage to be rejected")
	}

	events, eventErr := s.Events()
	if eventErr != nil {
		t.Fatal(eventErr)
	}
	if len(events) != 0 {
		t.Fatalf("rejected lineage wrote events: %d", len(events))
	}
}

func TestCaptureAcceptsREGAcceptedSupersedes(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	predecessor := core.EmergION{
		IDN: "E-ACCEPTED-PREV",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "accepted-source-hash",
			Bytes:      1,
			Stored:     1,
		},
		VAL: core.Validation{
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{Version: 1},
	}

	if _, err := s.SaveCandidate(predecessor); err != nil {
		t.Fatal(err)
	}

	approved, decision, err := gov.Decide(
		predecessor,
		gov.Approve,
		"HUMAN_FINAL",
		"test predecessor approval",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}

	accepted, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.STA != core.StateAccepted {
		t.Fatalf("predecessor was not REG accepted: %s", accepted.STA)
	}

	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(root, "successor.txt")
	if err := os.WriteFile(source, []byte("successor source"), 0600); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "successor",
				Relationships: map[string]string{
					"source_name":    "successor.txt",
					"governed_state": "accepted_context_present",
				},
				Capabilities: []string{"OBS"},
				Facts: []string{
					"source_preserved",
					"living_state_projected",
				},
				Risk:       "L",
				Supersedes: predecessor.IDN,
				Delta:      []string{"changed behavior"},
			},
		},
	}

	em, duplicate, err := r.Capture(context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("successor unexpectedly treated as duplicate")
	}

	if em.EVO.Supersedes != predecessor.IDN {
		t.Fatalf(
			"supersedes lost: got %q want %q",
			em.EVO.Supersedes,
			predecessor.IDN,
		)
	}

	expectedDelta := []string{
		"DS",
		"DC:+:OBS",
		"DR:+:governed_state",
		"DR:+:source_name",
	}
	if len(em.EVO.Delta) != len(expectedDelta) {
		t.Fatalf("unexpected deterministic delta: %#v", em.EVO.Delta)
	}
	for i, want := range expectedDelta {
		if em.EVO.Delta[i] != want {
			t.Fatalf("delta[%d] = %q want %q; full=%#v", i, em.EVO.Delta[i], want, em.EVO.Delta)
		}
	}
	for _, item := range em.EVO.Delta {
		if item == "changed behavior" {
			t.Fatalf("model-supplied delta crossed runtime boundary: %#v", em.EVO.Delta)
		}
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("successor not delivered to GOV: %s", em.STA)
	}
}

func TestGovernedStateContextRemainsValidJSON(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("E-PROJECTION-%02d", i)
		sourceHash := fmt.Sprintf("projection-source-%02d", i)

		em := core.EmergION{
			IDN: id,
			STA: core.StateAtGOV,
			MEM: core.Memory{
				SourceHash: sourceHash,
				Bytes:      1,
				Stored:     1,
				Summary:    strings.Repeat("projection-summary-", 40),
			},
			REL: map[string]string{
				"relationship": strings.Repeat("accepted-context-", 20),
			},
			CAP: []string{"OBS", "CMP"},
			VAL: core.Validation{
				Recoil: true,
				WVC:    true,
			},
			EVO: core.Evolution{Version: 1},
		}

		if _, err := s.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			"projection test",
		)
		if err != nil {
			t.Fatal(err)
		}

		decisionID, err := s.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}

		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := s.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	r := Runtime{Store: s, Reasoner: reason.Heuristic{}}

	_, projected, err := r.governedStateContext()
	if err != nil {
		t.Fatal(err)
	}

	if len(projected) > 12000 {
		t.Fatalf("projection exceeded bound: %d", len(projected))
	}

	if projected == "" {
		t.Fatal("bounded projection unexpectedly empty")
	}

	if !strings.Contains(projected, "I=") {
		t.Fatal("bounded projection missing compact I record")
	}

	if !strings.Contains(projected, "S=") {
		t.Fatal("bounded projection missing compact S record")
	}
}

func TestReworkRejectsNonReturnedPredecessor(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(root, "rework.txt")
	if err := os.WriteFile(source, []byte("corrected source"), 0600); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:               s,
		ReturnedPredecessor: "E-NOT-RETURNED",
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary:       "corrected source",
				Relationships: map[string]string{"source_name": "rework.txt"},
				Capabilities:  []string{"OBS"},
				Facts:         []string{"source_preserved"},
				Risk:          "L",
			},
		},
	}

	_, _, err = r.Capture(context.Background(), source, false)
	if err == nil {
		t.Fatal("expected non-returned predecessor to be rejected")
	}

	events, eventErr := s.Events()
	if eventErr != nil {
		t.Fatal(eventErr)
	}
	if len(events) != 0 {
		t.Fatalf("invalid rework wrote events: %d", len(events))
	}
}

func TestReturnedEmergIONReentersThroughRework(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	original := core.EmergION{
		IDN: "E-RETURNED-ORIGINAL",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "returned-original-hash",
			Bytes:      1,
			Stored:     1,
			Summary:    "original implementation",
		},
		REL: map[string]string{
			"source_name": "original.txt",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Risk:   "M",
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(original); err != nil {
		t.Fatal(err)
	}

	_, returnedReceipt, err := gov.Decide(
		original,
		gov.Return,
		"HUMAN_FINAL",
		"correct and resubmit",
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveDecision(returnedReceipt); err != nil {
		t.Fatal(err)
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := state.Returned[original.IDN]; !ok {
		t.Fatal("original EmergION did not enter RETURNED state")
	}

	correctedPath := filepath.Join(root, "corrected.txt")
	if err := os.WriteFile(
		correctedPath,
		[]byte("corrected implementation"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:               s,
		ReturnedPredecessor: original.IDN,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "corrected implementation",
				Relationships: map[string]string{
					"source_name": "corrected.txt",
				},
				Capabilities: []string{"OBS", "CMP"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	successor, duplicate, err := r.Capture(
		context.Background(),
		correctedPath,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("corrected source unexpectedly treated as duplicate")
	}

	if successor.STA != core.StateAtGOV {
		t.Fatalf("reworked EmergION did not return to GOV: %s", successor.STA)
	}

	if successor.EVO.Supersedes != original.IDN {
		t.Fatalf(
			"rework lineage lost: got %q want %q",
			successor.EVO.Supersedes,
			original.IDN,
		)
	}

	if len(successor.EVO.Delta) == 0 {
		t.Fatal("reworked EmergION has no deterministic delta")
	}

	foundSummary := false
	foundCapability := false
	for _, item := range successor.EVO.Delta {
		switch item {
		case "DS":
			foundSummary = true
		case "DC:+:CMP":
			foundCapability = true
		}
	}

	if !foundSummary {
		t.Fatalf("summary delta missing: %#v", successor.EVO.Delta)
	}
	if !foundCapability {
		t.Fatalf("capability delta missing: %#v", successor.EVO.Delta)
	}
}

func TestFullGovernedEmergenceLoop(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	originalPath := filepath.Join(root, "original.txt")
	if err := os.WriteFile(originalPath, []byte("original implementation"), 0600); err != nil {
		t.Fatal(err)
	}

	initialReasoner := lineageReasoner{
		result: reason.Result{
			Summary:       "original implementation",
			Relationships: map[string]string{"source_name": "original.txt"},
			Capabilities:  []string{"OBS"},
			Facts:         []string{"source_preserved"},
			Risk:          "M",
		},
	}

	rt := Runtime{
		Store:    s,
		Reasoner: initialReasoner,
	}

	original, duplicate, err := rt.Capture(
		context.Background(),
		originalPath,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("original source unexpectedly duplicate")
	}
	if original.STA != core.StateAtGOV {
		t.Fatalf("original state = %s", original.STA)
	}

	_, returnReceipt, err := gov.Decide(
		original,
		gov.Return,
		"HUMAN_FINAL",
		"correct and resubmit",
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveDecision(returnReceipt); err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Returned[original.IDN]; !ok {
		t.Fatal("original did not enter returned state")
	}

	correctedPath := filepath.Join(root, "corrected.txt")
	if err := os.WriteFile(correctedPath, []byte("corrected implementation"), 0600); err != nil {
		t.Fatal(err)
	}

	rework := Runtime{
		Store:               s,
		ReturnedPredecessor: original.IDN,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "corrected implementation",
				Relationships: map[string]string{
					"source_name": "corrected.txt",
				},
				Capabilities: []string{"OBS", "CMP"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	successor, duplicate, err := rework.Capture(
		context.Background(),
		correctedPath,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("corrected source unexpectedly duplicate")
	}
	if successor.STA != core.StateAtGOV {
		t.Fatalf("reworked state = %s", successor.STA)
	}
	if successor.EVO.Supersedes != original.IDN {
		t.Fatalf(
			"rework predecessor = %q want %q",
			successor.EVO.Supersedes,
			original.IDN,
		)
	}

	held, holdReceipt, err := gov.Decide(
		successor,
		gov.Hold,
		"HUMAN_FINAL",
		"pause for review",
	)
	if err != nil {
		t.Fatal(err)
	}
	if held.STA != core.StateHeld {
		t.Fatalf("hold state = %s", held.STA)
	}
	if _, err := s.SaveDecision(holdReceipt); err != nil {
		t.Fatal(err)
	}

	state, err = livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}
	heldState, ok := state.Held[successor.IDN]
	if !ok {
		t.Fatal("successor did not enter held state")
	}

	resumed, resumeReceipt, err := gov.ResumeHeld(
		heldState,
		"HUMAN_FINAL",
		"resume review",
	)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.STA != core.StateAtGOV {
		t.Fatalf("resume state = %s", resumed.STA)
	}
	if _, err := s.SaveDecision(resumeReceipt); err != nil {
		t.Fatal(err)
	}

	state, err = livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}
	resumedState, ok := state.AtGOV[successor.IDN]
	if !ok {
		t.Fatal("resumed successor did not return to GOV")
	}

	approved, approveReceipt, err := gov.Decide(
		resumedState,
		gov.Approve,
		"HUMAN_FINAL",
		"approve corrected implementation",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(approveReceipt)
	if err != nil {
		t.Fatal(err)
	}

	accepted, regReceipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.STA != core.StateAccepted {
		t.Fatalf("accepted state = %s", accepted.STA)
	}

	if _, err := s.SaveAccepted(regReceipt); err != nil {
		t.Fatal(err)
	}

	state, err = livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	final, ok := state.Accepted[successor.IDN]
	if !ok {
		t.Fatal("successor did not reach REG accepted state")
	}
	if final.STA != core.StateAccepted {
		t.Fatalf("final state = %s", final.STA)
	}
	if final.EVO.Supersedes != original.IDN {
		t.Fatalf(
			"final lineage = %q want %q",
			final.EVO.Supersedes,
			original.IDN,
		)
	}
	if len(final.EVO.Delta) == 0 {
		t.Fatal("final governed delta missing")
	}
}

func mustEvents(t *testing.T, s *store.Store) []core.Event {
	t.Helper()

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestDeriveDeltaIgnoresRuntimeProtectorRelationships(t *testing.T) {
	previous := core.EmergION{
		MEM: core.Memory{
			Summary: "same summary",
		},
		REL: map[string]string{
			"source_name":    "before.txt",
			"protector":      "NO_EXTERNAL_AUTHORITY_CLAIMED",
			"protector_gate": "HUMAN_FINAL_BOUND",
		},
	}

	analysis := reason.Result{
		Summary: "same summary",
		Relationships: map[string]string{
			"source_name": "after.txt",
		},
	}

	got := deriveDelta(previous, analysis)

	want := []string{
		"DR:~:source_name",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("delta mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestPivotDivergenceReentersNormalAdmission(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: lineageReasoner{},
	}

	_, cause := pivot.Observe(
		"WVC",
		"SOURCE_IDENTITY_CLAIM",
		"PRESERVED_EVIDENCE_OBSERVATION",
		"SOURCE_HASH_MATCH",
		func() error {
			return fmt.Errorf("source hash mismatch")
		},
	)
	if cause == nil {
		t.Fatal("expected pivot divergence")
	}

	em, duplicate, err := r.recapture(
		context.Background(),
		"",
		nil,
		cause,
	)
	if err == nil {
		t.Fatal("RECAPTURE must preserve original failure")
	}

	if duplicate {
		t.Fatal("divergence unexpectedly reported duplicate")
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("divergence state = %q want %q", em.STA, core.StateAtGOV)
	}

	if !em.VAL.Recoil {
		t.Fatal("divergence did not pass RECOIL during normal admission")
	}

	if !em.VAL.WVC {
		t.Fatal("divergence did not pass WVC during normal admission")
	}

	if len(em.VAL.Gaps) != 1 ||
		em.VAL.Gaps[0] != "BRIDGEGAP:PIVOT_WVC" {
		t.Fatalf("unexpected divergence gaps: %#v", em.VAL.Gaps)
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	admitted, ok := state.AtGOV[em.IDN]
	if !ok {
		t.Fatal("divergence was not handed to HUMAN_FINAL review")
	}

	if admitted.IDN != em.IDN {
		t.Fatalf("admitted divergence = %q", admitted.IDN)
	}

	if _, ok := state.Accepted[em.IDN]; ok {
		t.Fatal("divergence self-authorized into REG")
	}
}

func TestRecaptureReturnsTypedContinuationResult(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: lineageReasoner{},
	}

	_, cause := pivot.Observe(
		"WVC",
		"SOURCE_IDENTITY_CLAIM",
		"PRESERVED_EVIDENCE_OBSERVATION",
		"SOURCE_HASH_MATCH",
		func() error {
			return fmt.Errorf("source hash mismatch")
		},
	)
	if cause == nil {
		t.Fatal("expected pivot divergence")
	}

	em, _, err := r.recapture(
		context.Background(),
		"",
		nil,
		cause,
	)
	if err == nil {
		t.Fatal("RECAPTURE must retain original divergence")
	}

	var recaptured *RecaptureError
	if !errors.As(err, &recaptured) {
		t.Fatalf("error type = %T", err)
	}

	if recaptured.EmergION.IDN != em.IDN {
		t.Fatalf(
			"RECAPTURE identity = %q want %q",
			recaptured.EmergION.IDN,
			em.IDN,
		)
	}

	if recaptured.EmergION.STA != core.StateAtGOV {
		t.Fatalf("RECAPTURE state = %q", recaptured.EmergION.STA)
	}

	if !recaptured.EmergION.VAL.Recoil ||
		!recaptured.EmergION.VAL.WVC {
		t.Fatal("RECAPTURE result is not verified")
	}
}

type recaptureContinuityReasoner struct{}

func (recaptureContinuityReasoner) Analyze(
	_ context.Context,
	in reason.Input,
) (reason.Result, error) {
	if in.Name == "a.txt" {
		return reason.Result{
			Summary: "",
			Risk:    "M",
		}, nil
	}

	return reason.Result{
		Summary:       "normal second capture",
		Relationships: map[string]string{"source_name": in.Name},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"source_preserved"},
		Risk:          "L",
	}, nil
}

func (recaptureContinuityReasoner) Name() string {
	return "recapture-continuity-test"
}

func (recaptureContinuityReasoner) Version(context.Context) string {
	return "1"
}

func TestOnceContinuesAfterNaturalRecapture(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	dropzone := filepath.Join(root, "drop")
	if err := os.MkdirAll(dropzone, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(dropzone, "a.txt"),
		[]byte("first source causes reciprocal divergence"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(dropzone, "b.txt"),
		[]byte("second source remains normal"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: recaptureContinuityReasoner{},
	}

	ids, err := r.Once(context.Background(), dropzone)
	if err != nil {
		t.Fatal(err)
	}

	if len(ids) != 2 {
		t.Fatalf("Once IDs = %#v", ids)
	}

	if ids[0] == "" || ids[1] == "" {
		t.Fatalf("empty EmergION identity: %#v", ids)
	}

	if ids[0] == ids[1] {
		t.Fatalf("RECAPTURE and normal capture collided: %#v", ids)
	}

	entries, err := os.ReadDir(dropzone)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Fatalf("dropzone not clear: %d entries", len(entries))
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	recaptured, ok := state.AtGOV[ids[0]]
	if !ok {
		t.Fatalf("RECAPTURE EmergION %s missing from G", ids[0])
	}

	if !recaptured.VAL.Recoil || !recaptured.VAL.WVC {
		t.Fatal("RECAPTURE EmergION not verified")
	}

	normal, ok := state.AtGOV[ids[1]]
	if !ok {
		t.Fatalf("normal EmergION %s missing from G", ids[1])
	}

	if !normal.VAL.Recoil || !normal.VAL.WVC {
		t.Fatal("normal EmergION not verified")
	}
}

type coverageRecaptureReasoner struct{}

func (coverageRecaptureReasoner) Analyze(
	_ context.Context,
	in reason.Input,
) (reason.Result, error) {
	if in.Name == "a.txt" {
		// Valid EmergER output, but deliberately incomplete for COVERAGE.
		return reason.Result{
			Summary: "candidate with unresolved coverage",
			Risk:    "M",
		}, nil
	}

	return reason.Result{
		Summary:       "complete second candidate",
		Relationships: map[string]string{"source_name": in.Name},
		Capabilities:  []string{"OBS"},
		Facts:         []string{"source_preserved"},
		Risk:          "L",
	}, nil
}

func (coverageRecaptureReasoner) Name() string {
	return "coverage-recapture-test"
}

func (coverageRecaptureReasoner) Version(context.Context) string {
	return "1"
}

func TestOnceContinuesAfterCoverageRecapture(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	dropzone := filepath.Join(root, "drop")
	if err := os.MkdirAll(dropzone, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(dropzone, "a.txt"),
		[]byte("candidate intentionally missing coverage dimensions"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(dropzone, "b.txt"),
		[]byte("complete candidate follows coverage divergence"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: coverageRecaptureReasoner{},
	}

	ids, err := r.Once(context.Background(), dropzone)
	if err != nil {
		t.Fatal(err)
	}

	if len(ids) != 2 {
		t.Fatalf("Once IDs = %#v", ids)
	}

	entries, err := os.ReadDir(dropzone)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("dropzone not clear: %d entries", len(entries))
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	recaptured, ok := state.AtGOV[ids[0]]
	if !ok {
		t.Fatalf("COVERAGE RECAPTURE %s missing from G", ids[0])
	}

	if recaptured.STA != core.StateAtGOV ||
		!recaptured.VAL.Recoil ||
		!recaptured.VAL.WVC {
		t.Fatalf("COVERAGE RECAPTURE not verified: %#v", recaptured)
	}

	if len(recaptured.VAL.Gaps) != 1 ||
		recaptured.VAL.Gaps[0] != "BRIDGEGAP:PIVOT_COVERAGE" {
		t.Fatalf(
			"unexpected RECAPTURE gaps: %#v",
			recaptured.VAL.Gaps,
		)
	}

	if !strings.Contains(
		recaptured.MEM.Summary,
		"BRIDGEGAP:facts",
	) {
		t.Fatalf(
			"original COVERAGE divergence missing from RECAPTURE summary: %q",
			recaptured.MEM.Summary,
		)
	}

	normal, ok := state.AtGOV[ids[1]]
	if !ok {
		t.Fatalf("normal candidate %s missing from G", ids[1])
	}

	if !normal.VAL.Recoil || !normal.VAL.WVC {
		t.Fatal("normal second candidate not verified")
	}
}

func TestProtectorOwnsAndRebuildsAuthorityEnvelope(t *testing.T) {
	em := core.EmergION{
		CAP: []string{"OBS"},
		REL: map[string]string{
			"source_name":    "candidate.txt",
			"protector":      "SEND_GATED",
			"protector_gate": "HUMAN_FINAL_BOUND",
		},
	}

	if err := protector(&em); err != nil {
		t.Fatal(err)
	}

	if em.REL["protector"] != "NO_EXTERNAL_AUTHORITY_CLAIMED" {
		t.Fatalf(
			"untrusted protector claim survived: %#v",
			em.REL,
		)
	}

	if _, ok := em.REL["protector_gate"]; ok {
		t.Fatalf(
			"stale protector gate survived: %#v",
			em.REL,
		)
	}

	if em.REL["source_name"] != "candidate.txt" {
		t.Fatal("PROTECTOR modified unrelated relationship")
	}
}

func TestProtectorReciprocalAuthorityEnvelope(t *testing.T) {
	em := core.EmergION{
		CAP: []string{
			"SEND",
			"TRANSFER",
			"DEPLOY",
			"PATENT",
		},
		REL: map[string]string{},
	}

	if err := protector(&em); err != nil {
		t.Fatal(err)
	}

	want := "DEPLOY_GATED,EVIDENCE_ONLY,SEND_GATED,TRANSFER_GATED"
	if em.REL["protector"] != want {
		t.Fatalf(
			"protector envelope = %q want %q",
			em.REL["protector"],
			want,
		)
	}

	if em.REL["protector_gate"] != "HUMAN_FINAL_BOUND" {
		t.Fatalf(
			"protector gate = %q",
			em.REL["protector_gate"],
		)
	}
}

func TestRecoilIntegrityRequiresProtectorEnvelope(t *testing.T) {
	em := core.EmergION{
		MEM: core.Memory{
			SourceHash: "source-hash",
			Bytes:      10,
			Stored:     10,
			Summary:    "candidate",
		},
		REL: map[string]string{
			"source_name": "candidate.txt",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts: []string{"source_preserved"},
		},
	}

	err := recoilIntegrity(em)
	if err == nil {
		t.Fatal("expected missing PROTECTOR envelope rejection")
	}

	if !strings.Contains(err.Error(), "PROTECTOR") {
		t.Fatalf("unexpected RECOIL error: %v", err)
	}
}

func TestRecoilIntegrityAcceptsPostProtectorCandidate(t *testing.T) {
	em := core.EmergION{
		MEM: core.Memory{
			SourceHash: "source-hash",
			Bytes:      10,
			Stored:     10,
			Summary:    "candidate",
		},
		REL: map[string]string{
			"source_name": "candidate.txt",
		},
		CAP: []string{"SEND"},
		VAL: core.Validation{
			Facts: []string{"source_preserved"},
		},
	}

	if err := protector(&em); err != nil {
		t.Fatal(err)
	}

	if err := recoilIntegrity(em); err != nil {
		t.Fatalf("post-PROTECTOR RECOIL failed: %v", err)
	}

	if em.REL["protector_gate"] != "HUMAN_FINAL_BOUND" {
		t.Fatalf(
			"external authority gate missing: %#v",
			em.REL,
		)
	}
}

func TestRecoilIntegrityAllowsRecaptureBridgegap(t *testing.T) {
	em := core.EmergION{
		MEM: core.Memory{
			SourceHash: "pivot-source-hash",
			Bytes:      20,
			Stored:     20,
			Summary:    "pivot divergence",
		},
		REL: map[string]string{
			"pivot": "COVERAGE",
		},
		CAP: []string{"OBS", "CMP", "VLD"},
		VAL: core.Validation{
			Facts: []string{"pivot_divergence_observed"},
			Gaps:  []string{"BRIDGEGAP:PIVOT_COVERAGE"},
		},
	}

	if err := protector(&em); err != nil {
		t.Fatal(err)
	}

	if err := recoilIntegrity(em); err != nil {
		t.Fatalf(
			"RECAPTURE BRIDGEGAP incorrectly rejected: %v",
			err,
		)
	}
}

func TestWVCEvidenceContinuityAcceptsExactEvidence(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("exact WVC evidence")

	ev, err := s.Preserve(content)
	if err != nil {
		t.Fatal(err)
	}

	em := core.EmergION{
		MEM: core.Memory{
			SourceHash: ev.Hash,
			Bytes:      ev.Bytes,
			Stored:     ev.Stored,
			Codec:      ev.Codec,
		},
	}

	if err := wvcEvidenceContinuity(s, em); err != nil {
		t.Fatalf("exact evidence continuity failed: %v", err)
	}
}

func TestWVCEvidenceContinuityRejectsMetadataMismatch(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("WVC mismatch evidence")

	ev, err := s.Preserve(content)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*core.EmergION)
	}{
		{
			name: "bytes",
			mutate: func(em *core.EmergION) {
				em.MEM.Bytes++
			},
		},
		{
			name: "stored",
			mutate: func(em *core.EmergION) {
				em.MEM.Stored++
			},
		},
		{
			name: "codec",
			mutate: func(em *core.EmergION) {
				em.MEM.Codec = "raw"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			em := core.EmergION{
				MEM: core.Memory{
					SourceHash: ev.Hash,
					Bytes:      ev.Bytes,
					Stored:     ev.Stored,
					Codec:      ev.Codec,
				},
			}

			tc.mutate(&em)

			if err := wvcEvidenceContinuity(s, em); err == nil {
				t.Fatal("expected WVC continuity rejection")
			}
		})
	}
}

func TestDeriveFieldDeltaAgainstAcceptedBoundary(t *testing.T) {
	accepted := map[string]core.EmergION{
		"E-KNOWN": {
			CAP: []string{"OBS", "CMP"},
			REL: map[string]string{
				"source_kind": "PROGRAM",
				"domain":      "runtime",
			},
			EVO: core.Evolution{
				Metadata: &core.Metadata{
					Facets: []core.Facet{
						core.FacetProgramForge,
					},
				},
			},
		},
	}

	analysis := reason.Result{
		Capabilities: []string{"CMP", "VLD"},
		Relationships: map[string]string{
			"source_kind": "PROGRAM",
			"domain":      "compiler",
			"novel_key":   "new",
		},
		Facets: []string{
			"PROGRAM_FORGE",
			"ANALYTICS_FORECAST",
		},
	}

	got := deriveFieldDelta(accepted, analysis)

	want := []string{
		"FC:K:CMP",
		"FC:N:VLD",
		"FR:V:domain",
		"FR:N:novel_key",
		"FR:M:source_kind",
		"FF:N:ANALYTICS_FORECAST",
		"FF:K:PROGRAM_FORGE",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FIELD delta mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestRecapturePreservesFieldObservation(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: lineageReasoner{},
	}

	_, cause := pivot.Observe(
		"WVC",
		"SOURCE_IDENTITY_CLAIM",
		"PRESERVED_EVIDENCE_OBSERVATION",
		"SOURCE_HASH_MATCH",
		func() error {
			return fmt.Errorf("source hash mismatch")
		},
	)
	if cause == nil {
		t.Fatal("expected pivot divergence")
	}

	fieldObservation := []string{
		"FIELD_CAP_KNOWN:CMP",
		"FIELD_CAP_NOVEL:VLD",
		"FIELD_REL_VARIANT:domain",
		"FIELD_FACET_NOVEL:ANALYTICS_FORECAST",
	}

	em, duplicate, err := r.recapture(
		context.Background(),
		"",
		fieldObservation,
		cause,
	)
	if err == nil {
		t.Fatal("RECAPTURE must retain original divergence")
	}
	if duplicate {
		t.Fatal("RECAPTURE unexpectedly reported duplicate")
	}

	if em.EVO.Metadata == nil {
		t.Fatal("RECAPTURE metadata missing")
	}

	if !reflect.DeepEqual(
		em.EVO.Metadata.FieldObservation,
		fieldObservation,
	) {
		t.Fatalf(
			"RECAPTURE field observation mismatch\nwant: %#v\ngot:  %#v",
			fieldObservation,
			em.EVO.Metadata.FieldObservation,
		)
	}

	events, eventsErr := s.Events()
	if eventsErr != nil {
		t.Fatal(eventsErr)
	}

	state, rebuildErr := livefield.Rebuild(events)
	if rebuildErr != nil {
		t.Fatal(rebuildErr)
	}

	admitted, ok := state.AtGOV[em.IDN]
	if !ok {
		t.Fatalf("RECAPTURE EmergION %s missing from GOV", em.IDN)
	}

	if admitted.EVO.Metadata == nil {
		t.Fatal("stored RECAPTURE metadata missing")
	}

	if !reflect.DeepEqual(
		admitted.EVO.Metadata.FieldObservation,
		fieldObservation,
	) {
		t.Fatalf(
			"stored RECAPTURE field observation mismatch\nwant: %#v\ngot:  %#v",
			fieldObservation,
			admitted.EVO.Metadata.FieldObservation,
		)
	}
}

func TestExecutionAlreadyObservedUsesCanonicalFieldLineage(t *testing.T) {
	st := core.EmptyState()

	signal := core.EmergION{
		IDN: "E-EXECUTION-SIGNAL",
		REL: map[string]string{
			"source_kind":     "EXECUTION_RESULT",
			"parent_emergion": "E-PARENT",
			"adapter":         "LOCAL_GEMMA",
			"action":          "ANALYZE",
		},
	}

	st.AtGOV[signal.IDN] = signal

	if !executionAlreadyObserved(
		st,
		"E-PARENT",
		"LOCAL_GEMMA",
		"ANALYZE",
	) {
		t.Fatal("existing execution signal was not detected")
	}

	if executionAlreadyObserved(
		st,
		"E-OTHER",
		"LOCAL_GEMMA",
		"ANALYZE",
	) {
		t.Fatal("different parent was incorrectly suppressed")
	}

	if executionAlreadyObserved(
		st,
		"E-PARENT",
		"LOCAL_GEMMA",
		"DRAFT",
	) {
		t.Fatal("different action was incorrectly suppressed")
	}
}

func TestCaptureRejectsRuntimeOwnedExecutionLineageRelationships(t *testing.T) {
	for _, key := range []string{
		"parent_emergion",
		"authorization_event",
		"parent",
		"origin",
		"predecessor",
		"ancestor",
		"successor",
		"kin",
		"lineage",
	} {
		t.Run(key, func(t *testing.T) {
			root := t.TempDir()

			s, err := store.Open(filepath.Join(root, "state"))
			if err != nil {
				t.Fatal(err)
			}

			source := filepath.Join(root, "candidate.txt")
			if err := os.WriteFile(source, []byte("bounded candidate"), 0600); err != nil {
				t.Fatal(err)
			}

			r := Runtime{
				Store: s,
				Reasoner: lineageReasoner{
					result: reason.Result{
						Summary: "bounded candidate",
						Relationships: map[string]string{
							"source_name": "candidate.txt",
							key:           "UNTRUSTED-LINEAGE",
						},
						Capabilities: []string{"OBS"},
						Facts:        []string{"source_preserved"},
						Risk:         "L",
					},
				},
			}

			_, _, err = r.Capture(
				context.Background(),
				source,
				false,
			)
			if err == nil {
				t.Fatalf("runtime-owned relationship %q unexpectedly admitted", key)
			}

			events, eventsErr := s.Events()
			if eventsErr != nil {
				t.Fatal(eventsErr)
			}
			if len(events) != 0 {
				t.Fatalf(
					"rejected runtime-owned relationship %q wrote %d event(s)",
					key,
					len(events),
				)
			}
		})
	}
}

func TestExecutionObservationRemainsConfinedToExactKinMember(t *testing.T) {
	st := core.EmptyState()

	predecessor := core.EmergION{
		IDN: "E-KIN-PREDECESSOR",
		STA: core.StateAccepted,
	}

	successor := core.EmergION{
		IDN: "E-KIN-SUCCESSOR",
		STA: core.StateAccepted,
		EVO: core.Evolution{
			Supersedes: predecessor.IDN,
		},
	}

	st.Accepted[predecessor.IDN] = predecessor
	st.Accepted[successor.IDN] = successor

	signal := core.EmergION{
		IDN: "E-EXECUTION-SIGNAL",
		STA: core.StateAtGOV,
		REL: map[string]string{
			"source_kind":     "EXECUTION_RESULT",
			"parent_emergion": predecessor.IDN,
			"adapter":         "LOCAL_GEMMA",
			"action":          "ANALYZE",
		},
	}

	st.AtGOV[signal.IDN] = signal

	if !executionAlreadyObserved(
		st,
		predecessor.IDN,
		"LOCAL_GEMMA",
		"ANALYZE",
	) {
		t.Fatal("exact predecessor execution observation was not detected")
	}

	if executionAlreadyObserved(
		st,
		successor.IDN,
		"LOCAL_GEMMA",
		"ANALYZE",
	) {
		t.Fatal("predecessor execution observation leaked across sovereign Kin boundary")
	}
}

func TestCaptureRejectsRuntimeOwnedSourceIdentityRelationships(t *testing.T) {
	for _, key := range []string{
		"source_hash",
		"provenance",
	} {
		t.Run(key, func(t *testing.T) {
			root := t.TempDir()

			s, err := store.Open(filepath.Join(root, "state"))
			if err != nil {
				t.Fatal(err)
			}

			sourceBytes := []byte("canonical source identity proof")
			source := filepath.Join(root, "source-proof.txt")
			if err := os.WriteFile(source, sourceBytes, 0600); err != nil {
				t.Fatal(err)
			}

			r := Runtime{
				Store: s,
				Reasoner: lineageReasoner{
					result: reason.Result{
						Summary: "source identity spoof attempt",
						Relationships: map[string]string{
							"source_name": "source-proof.txt",
							key:           "FAKE-RUNTIME-SOURCE-AUTHORITY",
						},
						Capabilities: []string{"OBS"},
						Facts:        []string{"source_preserved"},
						Risk:         "L",
					},
				},
			}

			_, _, err = r.Capture(
				context.Background(),
				source,
				false,
			)
			if err == nil {
				t.Fatalf("runtime-owned source relationship %q unexpectedly admitted", key)
			}

			events, eventsErr := s.Events()
			if eventsErr != nil {
				t.Fatal(eventsErr)
			}
			if len(events) != 0 {
				t.Fatalf(
					"rejected runtime-owned source relationship %q wrote %d event(s)",
					key,
					len(events),
				)
			}
		})
	}
}

func TestCaptureAllowsCompositionKinToREGAcceptedTarget(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	target := core.EmergION{
		IDN: "E-COMPOSITION-TARGET",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "composition-target-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted composition target",
		},
		REL: map[string]string{
			"source_name": "target.txt",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(target); err != nil {
		t.Fatal(err)
	}

	approved, decision, err := gov.Decide(
		target,
		gov.Approve,
		"HUMAN_FINAL",
		"approve composition target",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}

	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(root, "composition.txt")
	if err := os.WriteFile(
		source,
		[]byte("bounded composition source"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "bounded composition source",
				Relationships: map[string]string{
					"source_name":     "composition.txt",
					"COMPOSITION_KIN": target.IDN,
				},
				Capabilities: []string{"OBS"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	em, duplicate, err := r.Capture(
		context.Background(),
		source,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("new composition source unexpectedly treated as duplicate")
	}

	if em.REL["COMPOSITION_KIN"] != target.IDN {
		t.Fatalf(
			"COMPOSITION_KIN = %q want %q",
			em.REL["COMPOSITION_KIN"],
			target.IDN,
		)
	}

	if em.EVO.Supersedes != "" {
		t.Fatalf(
			"composition relationship altered sovereign Kin lineage: supersedes = %q",
			em.EVO.Supersedes,
		)
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("composition candidate state = %q", em.STA)
	}
}

func TestCaptureRejectsCompositionKinToNonAcceptedTarget(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(root, "composition-invalid.txt")
	if err := os.WriteFile(
		source,
		[]byte("invalid composition target source"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "invalid composition target",
				Relationships: map[string]string{
					"source_name":     "composition-invalid.txt",
					"COMPOSITION_KIN": "E-NOT-REG-ACCEPTED",
				},
				Capabilities: []string{"OBS"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	_, _, err = r.Capture(
		context.Background(),
		source,
		false,
	)
	if err == nil {
		t.Fatal("non-accepted COMPOSITION_KIN target unexpectedly admitted")
	}

	events, eventsErr := s.Events()
	if eventsErr != nil {
		t.Fatal(eventsErr)
	}
	if len(events) != 0 {
		t.Fatalf(
			"rejected COMPOSITION_KIN target wrote %d event(s)",
			len(events),
		)
	}
}

func TestCaptureRejectsCompositionKinSelfReference(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	sourceBytes := []byte("composition self reference source")
	sourceHash := store.Hash(sourceBytes)
	selfID := "E-" + strings.ToUpper(sourceHash[:16])

	target := core.EmergION{
		IDN: selfID,
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "different-accepted-source-hash",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted identity collision target",
		},
		REL: map[string]string{
			"source_name": "accepted-target.txt",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(target); err != nil {
		t.Fatal(err)
	}

	approved, decision, err := gov.Decide(
		target,
		gov.Approve,
		"HUMAN_FINAL",
		"approve self-reference proof target",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}

	_, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	before := len(mustEvents(t, s))

	source := filepath.Join(root, "self-reference.txt")
	if err := os.WriteFile(source, sourceBytes, 0600); err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "self-referencing composition proposal",
				Relationships: map[string]string{
					"source_name":     "self-reference.txt",
					"COMPOSITION_KIN": selfID,
				},
				Capabilities: []string{"OBS"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	_, _, err = r.Capture(
		context.Background(),
		source,
		false,
	)
	if err == nil {
		t.Fatal("self-referencing COMPOSITION_KIN unexpectedly admitted")
	}

	after := len(mustEvents(t, s))
	if after != before {
		t.Fatalf(
			"self-reference rejection changed event count: before=%d after=%d",
			before,
			after,
		)
	}
}

func TestSAWSourceReentersGovernedExecutionAndRecapture(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	// Build two already-accepted governed members whose composition yields one SAW source.
	memberA := core.EmergION{
		IDN: "E-SAW-CIRC-A",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saw-circ-a",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-SAW-CIRC-B",
		},
		CAP: []string{"ANALYZE"},
		VAL: core.Validation{
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt: time.Unix(1, 0).UTC(),
				Facets: []core.Facet{
					core.FacetAnalyticsForecast,
				},
				Monetization: &core.Monetization{
					Model:       "license",
					Customer:    "enterprise",
					Value:       "governed analysis artifact",
					RevenuePath: "deployment",
				},
			},
		},
	}

	memberB := core.EmergION{
		IDN: "E-SAW-CIRC-B",
		STA: core.StateAccepted,
		MEM: core.Memory{
			SourceHash: "saw-circ-b",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	// Persist through the existing governed event path.
	for _, em := range []core.EmergION{memberA, memberB} {
		candidate := em
		candidate.STA = core.StateAtGOV

		if _, err := s.SaveCandidate(candidate); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			candidate,
			gov.Approve,
			"HUMAN_FINAL",
			"SAW circulation proof",
		)
		if err != nil {
			t.Fatal(err)
		}

		decisionID, err := s.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}

		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := s.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	st, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	sources, err := proj.SAWSources(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("SAW sources = %d want 1", len(sources))
	}

	sourcePath := filepath.Join(root, "saw-source.mxpd")
	if err := os.WriteFile(sourcePath, sources[0].Content, 0600); err != nil {
		t.Fatal(err)
	}

	// Re-enter through the existing Capture seam. The reasoner supplies only
	// semantic analysis; runtime remains owner of source identity and lineage.
	rt := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "governed SAW source",
				Relationships: map[string]string{
					"source_name": "saw-source.mxpd",
				},
				Capabilities: []string{"ANALYZE"},
				Facts:        []string{"saw_source_preserved"},
				Risk:         "L",
				Facets: []string{
					"ANALYTICS_FORECAST",
				},
			},
		},
	}

	captured, duplicate, err := rt.Capture(
		context.Background(),
		sourcePath,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("SAW source unexpectedly duplicate")
	}
	if captured.STA != core.StateAtGOV {
		t.Fatalf("captured SAW state = %s", captured.STA)
	}

	// HUMAN_FINAL remains mandatory before execution.
	approved, decision, err := gov.Decide(
		captured,
		gov.Approve,
		"HUMAN_FINAL",
		"approve recaptured SAW source",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}

	accepted, receipt, err := reg.Accept(approved, decisionID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	st, err = livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.Accepted[accepted.IDN]; !ok {
		t.Fatal("recaptured SAW source did not reach REG")
	}

	request, err := adapters.PrepareExecution(
		st,
		accepted.IDN,
		"LOCAL_GEMMA",
		"ANALYZE",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Use the existing binding rule to produce an exact-lineage result.
	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "bounded SAW circulation proof",
		},
	)

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("execution result unexpectedly duplicate")
	}

	if signal.STA != core.StateAtGOV {
		t.Fatalf("execution signal state = %s", signal.STA)
	}

	if signal.REL["parent_emergion"] != accepted.IDN {
		t.Fatalf(
			"execution signal parent = %q want %q",
			signal.REL["parent_emergion"],
			accepted.IDN,
		)
	}

	if signal.REL["adapter"] != "LOCAL_GEMMA" ||
		signal.REL["action"] != "ANALYZE" {
		t.Fatalf("execution signal lineage = %#v", signal.REL)
	}

	if !signal.VAL.Recoil || !signal.VAL.WVC {
		t.Fatalf("execution signal not verified: %#v", signal.VAL)
	}
}

func TestGovernedStateContextDiagnostic(t *testing.T) {
	stateRoot := os.Getenv("FIELD_HOME")
	if stateRoot == "" {
		t.Skip("set FIELD_HOME to the existing runtime state root")
	}

	s, err := store.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: reason.Heuristic{},
	}

	state, projected, err := r.governedStateContext()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf(
		"accepted=%d governed_state_bytes=%d",
		len(state.Accepted),
		len(projected),
	)

	t.Logf("GOVERNED_STATE_BEGIN\\n%s\\nGOVERNED_STATE_END", projected)
}

func TestGemmaAnalyzeRealisticGovernedContextDiagnostic(t *testing.T) {
	stateRoot := os.Getenv("FIELD_HOME")
	binary := os.Getenv("GEMMA_BIN")
	model := os.Getenv("GEMMA_MODEL")

	if stateRoot == "" || binary == "" || model == "" {
		t.Skip("set FIELD_HOME, GEMMA_BIN, and GEMMA_MODEL")
	}

	s, err := store.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: reason.Heuristic{},
	}

	_, governedState, err := r.governedStateContext()
	if err != nil {
		t.Fatal(err)
	}

	source, err := os.ReadFile("../../docs/SYSTEM_STATUS.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(source) > 1200 {
		source = source[:1200]
	}

	g := reason.GemmaCLI{
		Binary:    binary,
		Model:     model,
		Threads:   4,
		Context:   2048,
		MaxTokens: 128,
		Timeout:   180 * time.Second,
		ExtraArgs: []string{"--seed", "1"},
	}

	start := time.Now()

	got, err := g.Analyze(context.Background(), reason.Input{
		Name:          "SYSTEM_STATUS.md",
		Content:       source,
		GovernedState: governedState,
	})

	elapsed := time.Since(start)

	t.Logf(
		"governed_state_bytes=%d source_bytes=%d elapsed=%s",
		len(governedState),
		len(source),
		elapsed,
	)

	if err != nil {
		t.Fatalf("realistic Gemma Analyze failed after %s: %v", elapsed, err)
	}

	t.Logf(
		"summary=%q risk=%q facts=%d capabilities=%d gaps=%d",
		got.Summary,
		got.Risk,
		len(got.Facts),
		len(got.Capabilities),
		len(got.Gaps),
	)
}

func TestCoverageRecaptureDerivesRequiredCapability(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: reason.Heuristic{},
	}

	_, cause := pivot.Observe(
		"COVERAGE",
		"CANDIDATE_COVERAGE_CLAIM",
		"BRIDGEGAP_OBSERVATION",
		"NO_UNRESOLVED_BRIDGEGAP",
		func() error {
			return errors.New("COVERAGE failed: BRIDGEGAP:capabilities")
		},
	)
	if cause == nil {
		t.Fatal("expected COVERAGE divergence")
	}

	em, duplicate, err := r.recapture(
		context.Background(),
		"",
		nil,
		cause,
	)

	if duplicate {
		t.Fatal("RECAPTURE unexpectedly reported duplicate")
	}

	var recaptured *RecaptureError
	if !errors.As(err, &recaptured) {
		t.Fatalf("expected RecaptureError, got %T: %v", err, err)
	}

	if em.REL["required_capability"] != "DERIVE_CAPABILITY" {
		t.Fatalf(
			"required_capability = %q want DERIVE_CAPABILITY",
			em.REL["required_capability"],
		)
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("RECAPTURE state = %q", em.STA)
	}

	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatal("RECAPTURE result is not verified")
	}
}

type validatingGemmaCapabilityReasoner struct{}

func (validatingGemmaCapabilityReasoner) Analyze(
	_ context.Context,
	_ reason.Input,
) (reason.Result, error) {
	return reason.Result{}, nil
}

func (validatingGemmaCapabilityReasoner) Name() string {
	return "gemma-llama-cli"
}

func (validatingGemmaCapabilityReasoner) Version(context.Context) string {
	return "test"
}

func (validatingGemmaCapabilityReasoner) Validate() error {
	return nil
}

func TestRequiredCapabilityRemainsUnresolvedWhenCompositionIncomplete(t *testing.T) {
	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_CAPABILITY",
		},
		CAP: []string{"OBS", "CMP", "VLD"},
	}

	r := Runtime{
		Reasoner: validatingGemmaCapabilityReasoner{},
	}

	r.resolveRequiredCapability(&em, core.EmptyState())

	if em.REL["capability_resolution"] != "UNRESOLVED" {
		t.Fatalf(
			"resolution = %q want UNRESOLVED",
			em.REL["capability_resolution"],
		)
	}

	if got := em.REL["capability_composition"]; got != "" {
		t.Fatalf("unexpected capability composition %q", got)
	}
}

func TestRequiredCapabilityProducesComposableCandidateWhenAllInputsExist(t *testing.T) {
	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_CAPABILITY",
		},
		CAP: []string{"OBS", "CMP", "RLT", "VLD"},
	}

	r := Runtime{
		Reasoner: validatingGemmaCapabilityReasoner{},
	}

	r.resolveRequiredCapability(&em, core.EmptyState())

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"resolution = %q want COMPOSABLE_CANDIDATE",
			em.REL["capability_resolution"],
		)
	}

	if em.REL["capability_composition"] != "ANALYZE+CMP+RLT" {
		t.Fatalf(
			"composition = %q want ANALYZE+CMP+RLT",
			em.REL["capability_composition"],
		)
	}
}

func TestRequiredCapabilityUsesREGAcceptedCapability(t *testing.T) {
	accepted := core.EmergION{
		IDN: "E-ACCEPTED-RLT",
		STA: core.StateAccepted,
		CAP: []string{"RLT"},
	}

	st := core.EmptyState()
	st.Accepted[accepted.IDN] = accepted

	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_CAPABILITY",
		},
		CAP: []string{"OBS", "CMP", "VLD"},
	}

	r := Runtime{
		Reasoner: validatingGemmaCapabilityReasoner{},
	}

	r.resolveRequiredCapability(&em, st)

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"resolution = %q want COMPOSABLE_CANDIDATE",
			em.REL["capability_resolution"],
		)
	}

	if em.REL["capability_composition"] != "ANALYZE+CMP+RLT" {
		t.Fatalf(
			"composition = %q want ANALYZE+CMP+RLT",
			em.REL["capability_composition"],
		)
	}
}

func TestRequiredCapabilityIgnoresNonAcceptedCapability(t *testing.T) {
	notAccepted := core.EmergION{
		IDN: "E-AT-GOV-RLT",
		STA: core.StateAtGOV,
		CAP: []string{"RLT"},
	}

	st := core.EmptyState()
	st.AtGOV[notAccepted.IDN] = notAccepted

	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_CAPABILITY",
		},
		CAP: []string{"OBS", "CMP", "VLD"},
	}

	r := Runtime{
		Reasoner: validatingGemmaCapabilityReasoner{},
	}

	r.resolveRequiredCapability(&em, st)

	if em.REL["capability_resolution"] != "UNRESOLVED" {
		t.Fatalf(
			"resolution = %q want UNRESOLVED",
			em.REL["capability_resolution"],
		)
	}

	if got := em.REL["capability_composition"]; got != "" {
		t.Fatalf("unexpected composition %q", got)
	}
}

func TestRequiredCapabilityFromCoverageDivergence(t *testing.T) {
	tests := []struct {
		divergence string
		want       string
	}{
		{"COVERAGE failed: BRIDGEGAP:capabilities", "DERIVE_CAPABILITY"},
		{"COVERAGE failed: BRIDGEGAP:facts", "ESTABLISH_FACT"},
		{"COVERAGE failed: BRIDGEGAP:relationships", "DERIVE_RELATIONSHIP"},
		{"COVERAGE failed: BRIDGEGAP:unknown", ""},
	}

	for _, tt := range tests {
		got := requiredCapabilityFromDivergence(pivot.Result{
			Name:       "COVERAGE",
			Divergence: tt.divergence,
		})

		if got != tt.want {
			t.Fatalf(
				"divergence %q required capability = %q want %q",
				tt.divergence,
				got,
				tt.want,
			)
		}
	}
}

func TestEstablishFactRequirementComposesFromSemanticCapabilities(t *testing.T) {
	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "ESTABLISH_FACT",
		},
		CAP: []string{"OBS", "VLD"},
	}

	r := Runtime{}
	r.resolveRequiredCapability(&em, core.EmptyState())

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"resolution = %q want COMPOSABLE_CANDIDATE",
			em.REL["capability_resolution"],
		)
	}

	if em.REL["capability_composition"] != "OBS+VLD" {
		t.Fatalf(
			"composition = %q want OBS+VLD",
			em.REL["capability_composition"],
		)
	}
}

func TestDeriveRelationshipRequirementUsesAcceptedCapability(t *testing.T) {
	st := core.EmptyState()
	st.Accepted["E-ACCEPTED-RLT"] = core.EmergION{
		IDN: "E-ACCEPTED-RLT",
		STA: core.StateAccepted,
		CAP: []string{"RLT"},
	}

	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_RELATIONSHIP",
		},
		CAP: []string{"CMP"},
	}

	r := Runtime{}
	r.resolveRequiredCapability(&em, st)

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"resolution = %q want COMPOSABLE_CANDIDATE",
			em.REL["capability_resolution"],
		)
	}

	if em.REL["capability_composition"] != "CMP+RLT" {
		t.Fatalf(
			"composition = %q want CMP+RLT",
			em.REL["capability_composition"],
		)
	}
}

func TestUnknownRequiredCapabilityFailsClosed(t *testing.T) {
	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "UNKNOWN_REQUIREMENT",
		},
		CAP: []string{"OBS", "CMP", "RLT", "VLD"},
	}

	r := Runtime{
		Reasoner: validatingGemmaCapabilityReasoner{},
	}

	r.resolveRequiredCapability(&em, core.EmptyState())

	if em.REL["capability_resolution"] != "UNRESOLVED" {
		t.Fatalf(
			"resolution = %q want UNRESOLVED",
			em.REL["capability_resolution"],
		)
	}

	if got := em.REL["capability_composition"]; got != "" {
		t.Fatalf("unknown requirement produced composition %q", got)
	}
}

func TestDeterministicBridgegapsDoNotBecomeCapabilityRequirements(t *testing.T) {
	tests := []string{
		"COVERAGE failed: BRIDGEGAP:source_hash",
		"COVERAGE failed: BRIDGEGAP:evidence",
		"COVERAGE failed: BRIDGEGAP:living_state_relationship",
		"COVERAGE failed: BRIDGEGAP:living_state_projection",
	}

	for _, divergence := range tests {
		got := requiredCapabilityFromDivergence(pivot.Result{
			Name:       "COVERAGE",
			Divergence: divergence,
		})

		if got != "" {
			t.Fatalf(
				"deterministic divergence %q produced required capability %q",
				divergence,
				got,
			)
		}
	}
}

func TestUnprovenSummaryBridgegapRemainsUnresolved(t *testing.T) {
	got := requiredCapabilityFromDivergence(pivot.Result{
		Name:       "COVERAGE",
		Divergence: "COVERAGE failed: BRIDGEGAP:summary",
	})

	if got != "" {
		t.Fatalf(
			"summary BRIDGEGAP produced unproven required capability %q",
			got,
		)
	}
}

func TestUnknownBridgegapRemainsUnresolved(t *testing.T) {
	got := requiredCapabilityFromDivergence(pivot.Result{
		Name:       "COVERAGE",
		Divergence: "COVERAGE failed: BRIDGEGAP:not_yet_known",
	})

	if got != "" {
		t.Fatalf(
			"unknown BRIDGEGAP produced required capability %q",
			got,
		)
	}
}

func TestKnownCompositionalBridgegapsRemainMapped(t *testing.T) {
	tests := []struct {
		divergence string
		want       string
	}{
		{
			divergence: "COVERAGE failed: BRIDGEGAP:facts",
			want:       "ESTABLISH_FACT",
		},
		{
			divergence: "COVERAGE failed: BRIDGEGAP:capabilities",
			want:       "DERIVE_CAPABILITY",
		},
		{
			divergence: "COVERAGE failed: BRIDGEGAP:relationships",
			want:       "DERIVE_RELATIONSHIP",
		},
	}

	for _, tt := range tests {
		got := requiredCapabilityFromDivergence(pivot.Result{
			Name:       "COVERAGE",
			Divergence: tt.divergence,
		})

		if got != tt.want {
			t.Fatalf(
				"divergence %q required capability = %q want %q",
				tt.divergence,
				got,
				tt.want,
			)
		}
	}
}

func TestCoverageRecaptureUsesGenericFactRequirement(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store:    s,
		Reasoner: reason.Heuristic{},
	}

	_, cause := pivot.Observe(
		"COVERAGE",
		"CANDIDATE_COVERAGE_CLAIM",
		"BRIDGEGAP_OBSERVATION",
		"NO_UNRESOLVED_BRIDGEGAP",
		func() error {
			return errors.New("COVERAGE failed: BRIDGEGAP:facts")
		},
	)
	if cause == nil {
		t.Fatal("expected COVERAGE divergence")
	}

	em, duplicate, err := r.recapture(
		context.Background(),
		"",
		nil,
		cause,
	)

	if duplicate {
		t.Fatal("RECAPTURE unexpectedly reported duplicate")
	}

	var recaptured *RecaptureError
	if !errors.As(err, &recaptured) {
		t.Fatalf("expected RecaptureError, got %T: %v", err, err)
	}

	if em.REL["required_capability"] != "ESTABLISH_FACT" {
		t.Fatalf(
			"required_capability = %q want ESTABLISH_FACT",
			em.REL["required_capability"],
		)
	}

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"capability_resolution = %q want COMPOSABLE_CANDIDATE",
			em.REL["capability_resolution"],
		)
	}

	if em.REL["capability_composition"] != "OBS+VLD" {
		t.Fatalf(
			"capability_composition = %q want OBS+VLD",
			em.REL["capability_composition"],
		)
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("RECAPTURE state = %q want %q", em.STA, core.StateAtGOV)
	}

	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatal("generic requirement RECAPTURE did not pass RECOIL/WVC")
	}
}

func TestAcceptedCapabilityProvidersAreDeterministic(t *testing.T) {
	st := core.EmptyState()

	st.Accepted["E-PROVIDER-ANALYZE"] = core.EmergION{
		IDN: "E-PROVIDER-ANALYZE",
		STA: core.StateAccepted,
		CAP: []string{"ANALYZE"},
	}

	st.Accepted["E-PROVIDER-CMP-Z"] = core.EmergION{
		IDN: "E-PROVIDER-CMP-Z",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}

	st.Accepted["E-PROVIDER-CMP-A"] = core.EmergION{
		IDN: "E-PROVIDER-CMP-A",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}

	st.Accepted["E-PROVIDER-RLT"] = core.EmergION{
		IDN: "E-PROVIDER-RLT",
		STA: core.StateAccepted,
		CAP: []string{"RLT"},
	}

	first, ok := acceptedCapabilityProviders(
		"DERIVE_CAPABILITY",
		st,
	)
	if !ok {
		t.Fatal("accepted provider composition unexpectedly unresolved")
	}

	second, ok := acceptedCapabilityProviders(
		"DERIVE_CAPABILITY",
		st,
	)
	if !ok {
		t.Fatal("second accepted provider resolution unexpectedly unresolved")
	}

	want := "ANALYZE:E-PROVIDER-ANALYZE,CMP:E-PROVIDER-CMP-A,RLT:E-PROVIDER-RLT"

	if first != want {
		t.Fatalf(
			"providers = %q want %q",
			first,
			want,
		)
	}

	if second != first {
		t.Fatalf(
			"provider resolution is not deterministic: %q != %q",
			first,
			second,
		)
	}
}

func TestAcceptedCapabilityProvidersIgnoreNonAcceptedState(t *testing.T) {
	st := core.EmptyState()

	st.Accepted["E-PROVIDER-ANALYZE"] = core.EmergION{
		IDN: "E-PROVIDER-ANALYZE",
		STA: core.StateAccepted,
		CAP: []string{"ANALYZE"},
	}

	st.Accepted["E-PROVIDER-CMP"] = core.EmergION{
		IDN: "E-PROVIDER-CMP",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}

	st.AtGOV["E-NOT-ACCEPTED-RLT"] = core.EmergION{
		IDN: "E-NOT-ACCEPTED-RLT",
		STA: core.StateAtGOV,
		CAP: []string{"RLT"},
	}

	if providers, ok := acceptedCapabilityProviders(
		"DERIVE_CAPABILITY",
		st,
	); ok {
		t.Fatalf(
			"non-accepted capability became provider identity: %q",
			providers,
		)
	}
}

func TestResolveRequiredCapabilityAddsOnlyCompleteAcceptedProviderProposal(t *testing.T) {
	st := core.EmptyState()

	st.Accepted["E-ANALYZE"] = core.EmergION{
		IDN: "E-ANALYZE",
		STA: core.StateAccepted,
		CAP: []string{"ANALYZE"},
	}

	st.Accepted["E-CMP"] = core.EmergION{
		IDN: "E-CMP",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}

	st.Accepted["E-RLT"] = core.EmergION{
		IDN: "E-RLT",
		STA: core.StateAccepted,
		CAP: []string{"RLT"},
	}

	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_CAPABILITY",
		},
	}

	r := Runtime{}
	r.resolveRequiredCapability(&em, st)

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"resolution = %q want COMPOSABLE_CANDIDATE",
			em.REL["capability_resolution"],
		)
	}

	if em.REL["capability_composition"] != "ANALYZE+CMP+RLT" {
		t.Fatalf(
			"composition = %q",
			em.REL["capability_composition"],
		)
	}

	wantProviders := "ANALYZE:E-ANALYZE,CMP:E-CMP,RLT:E-RLT"

	if em.REL["capability_providers"] != wantProviders {
		t.Fatalf(
			"providers = %q want %q",
			em.REL["capability_providers"],
			wantProviders,
		)
	}

	if _, exists := em.REL["COMPOSITION_KIN"]; exists {
		t.Fatal("provider resolution created unauthorized COMPOSITION_KIN")
	}
}

func TestResolveRequiredCapabilityDoesNotFabricateMissingProviderIdentity(t *testing.T) {
	st := core.EmptyState()

	st.Accepted["E-ANALYZE"] = core.EmergION{
		IDN: "E-ANALYZE",
		STA: core.StateAccepted,
		CAP: []string{"ANALYZE"},
	}

	st.Accepted["E-CMP"] = core.EmergION{
		IDN: "E-CMP",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}

	em := core.EmergION{
		REL: map[string]string{
			"required_capability": "DERIVE_CAPABILITY",
		},
		CAP: []string{"RLT"},
	}

	r := Runtime{}
	r.resolveRequiredCapability(&em, st)

	if em.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"existing composition behavior changed: %q",
			em.REL["capability_resolution"],
		)
	}

	if got := em.REL["capability_providers"]; got != "" {
		t.Fatalf(
			"missing REG provider identity was fabricated: %q",
			got,
		)
	}
}

func TestCapabilityProviderEdgesDeriveAdjacentComposition(t *testing.T) {
	got, err := capabilityProviderEdges(
		"ANALYZE:E-A,CMP:E-B,RLT:E-C",
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("edges = %d want 2", len(got))
	}

	if got[0].From != "E-A" || got[0].To != "E-B" {
		t.Fatalf("first edge = %#v", got[0])
	}

	if got[1].From != "E-B" || got[1].To != "E-C" {
		t.Fatalf("second edge = %#v", got[1])
	}
}

func TestCapabilityProviderEdgesRejectMalformedProvider(t *testing.T) {
	_, err := capabilityProviderEdges(
		"ANALYZE:E-A,BROKEN,RLT:E-C",
	)

	if err == nil {
		t.Fatal("malformed capability provider unexpectedly accepted")
	}
}

func TestCapabilityProviderEdgesRejectEmptyProviderIdentity(t *testing.T) {
	_, err := capabilityProviderEdges(
		"ANALYZE:E-A,CMP:,RLT:E-C",
	)

	if err == nil {
		t.Fatal("empty capability provider identity unexpectedly accepted")
	}
}

func TestCapabilityProviderEdgesRejectSelfReference(t *testing.T) {
	_, err := capabilityProviderEdges(
		"ANALYZE:E-A,CMP:E-A",
	)

	if err == nil {
		t.Fatal("self-referencing provider edge unexpectedly accepted")
	}
}

func TestCapabilityProviderEdgesSingleProviderProducesNoEdge(t *testing.T) {
	got, err := capabilityProviderEdges(
		"ANALYZE:E-A",
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatalf("single provider produced edges: %#v", got)
	}
}

func TestCapabilityProviderEdgesEmptyProducesNoEdge(t *testing.T) {
	got, err := capabilityProviderEdges("")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatalf("empty providers produced edges: %#v", got)
	}
}

func TestRecaptureCarriesGovernedProviderEdgeProposalWithoutCompositionKin(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	acceptProvider := func(id, capability string) {
		t.Helper()

		candidate := core.EmergION{
			IDN: id,
			STA: core.StateAtGOV,
			MEM: core.Memory{
				SourceHash: "source-" + strings.ToLower(capability),
				Bytes:      1,
				Stored:     1,
			},
			CAP: []string{capability},
			VAL: core.Validation{
				Recoil: true,
				WVC:    true,
			},
			EVO: core.Evolution{
				Version: 1,
			},
		}

		if _, err := s.SaveCandidate(candidate); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			candidate,
			gov.Approve,
			"HUMAN_FINAL",
			"provider-edge composition proof",
		)
		if err != nil {
			t.Fatal(err)
		}

		decisionID, err := s.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}

		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := s.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	acceptProvider("E-PROVIDER-ANALYZE", "ANALYZE")
	acceptProvider("E-PROVIDER-CMP", "CMP")
	acceptProvider("E-PROVIDER-RLT", "RLT")

	r := Runtime{
		Store:    s,
		Reasoner: reason.Heuristic{},
	}

	_, cause := pivot.Observe(
		"COVERAGE",
		"CANDIDATE_COVERAGE_CLAIM",
		"BRIDGEGAP_OBSERVATION",
		"NO_UNRESOLVED_BRIDGEGAP",
		func() error {
			return errors.New(
				"COVERAGE failed: BRIDGEGAP:capabilities",
			)
		},
	)
	if cause == nil {
		t.Fatal("expected COVERAGE divergence")
	}

	em, duplicate, err := r.recapture(
		context.Background(),
		"",
		nil,
		cause,
	)
	if duplicate {
		t.Fatal("RECAPTURE unexpectedly reported duplicate")
	}

	var recaptured *RecaptureError
	if !errors.As(err, &recaptured) {
		t.Fatalf("expected RecaptureError, got %T: %v", err, err)
	}

	wantProviders := "ANALYZE:E-PROVIDER-ANALYZE,CMP:E-PROVIDER-CMP,RLT:E-PROVIDER-RLT"
	if em.REL["capability_providers"] != wantProviders {
		t.Fatalf(
			"providers = %q want %q",
			em.REL["capability_providers"],
			wantProviders,
		)
	}

	wantProposal := "E-PROVIDER-ANALYZE->E-PROVIDER-CMP,E-PROVIDER-CMP->E-PROVIDER-RLT"
	if em.REL["capability_provider_edge_proposal"] != wantProposal {
		t.Fatalf(
			"provider edge proposal = %q want %q",
			em.REL["capability_provider_edge_proposal"],
			wantProposal,
		)
	}

	if _, exists := em.REL["COMPOSITION_KIN"]; exists {
		t.Fatal("provider edge proposal self-authorized COMPOSITION_KIN")
	}

	state, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	admitted, ok := state.AtGOV[em.IDN]
	if !ok {
		t.Fatal("provider-edge proposal did not remain at HUMAN_FINAL review")
	}

	if admitted.REL["capability_provider_edge_proposal"] != wantProposal {
		t.Fatal("stored provider-edge proposal changed")
	}

	if _, exists := admitted.REL["COMPOSITION_KIN"]; exists {
		t.Fatal("stored proposal bypassed governed COMPOSITION_KIN")
	}
}

func TestReturnedProviderEdgeProposalCanReenterAsGovernedComposition(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	target := core.EmergION{
		IDN: "E-GOVERNED-COMPOSITION-TARGET",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "governed-composition-target-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "governed composition target",
		},
		REL: map[string]string{
			"source_name": "governed-composition-target.txt",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(target); err != nil {
		t.Fatal(err)
	}

	approvedTarget, targetDecision, err := gov.Decide(
		target,
		gov.Approve,
		"HUMAN_FINAL",
		"approve exact governed composition target",
	)
	if err != nil {
		t.Fatal(err)
	}

	targetDecisionID, err := s.SaveDecision(targetDecision)
	if err != nil {
		t.Fatal(err)
	}

	_, targetReceipt, err := reg.Accept(
		approvedTarget,
		targetDecisionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(targetReceipt); err != nil {
		t.Fatal(err)
	}

	proposal := core.EmergION{
		IDN: "E-RETURNED-PROVIDER-EDGE-PROPOSAL",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "returned-provider-edge-proposal-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "provider edge proposal awaiting governed composition",
		},
		REL: map[string]string{
			"source_name":                       "provider-edge-proposal.txt",
			"capability_provider_edge_proposal": "E-PROVIDER-ANALYZE->E-PROVIDER-CMP,E-PROVIDER-CMP->E-PROVIDER-RLT",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(proposal); err != nil {
		t.Fatal(err)
	}

	returned, returnDecision, err := gov.Decide(
		proposal,
		gov.Return,
		"HUMAN_FINAL",
		"return provider-edge proposal for exact governed composition",
	)
	if err != nil {
		t.Fatal(err)
	}

	if returned.STA != core.StateReturned {
		t.Fatalf("returned state = %q want %q", returned.STA, core.StateReturned)
	}

	if _, err := s.SaveDecision(returnDecision); err != nil {
		t.Fatal(err)
	}

	beforeState, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	beforeReturned, ok := beforeState.Returned[proposal.IDN]
	if !ok {
		t.Fatal("provider-edge proposal did not enter HUMAN_FINAL RETURNED state")
	}

	source := filepath.Join(root, "governed-composition-rework.txt")
	if err := os.WriteFile(
		source,
		[]byte("governed rework of returned provider-edge proposal"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	rt := Runtime{
		Store:               s,
		ReturnedPredecessor: proposal.IDN,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "governed composition rework",
				Relationships: map[string]string{
					"source_name":     "governed-composition-rework.txt",
					"COMPOSITION_KIN": target.IDN,
				},
				Capabilities: []string{"OBS"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	em, duplicate, err := rt.Capture(
		context.Background(),
		source,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if duplicate {
		t.Fatal("governed composition rework unexpectedly treated as duplicate")
	}

	if em.EVO.Supersedes != proposal.IDN {
		t.Fatalf(
			"rework supersedes = %q want %q",
			em.EVO.Supersedes,
			proposal.IDN,
		)
	}

	if em.REL["COMPOSITION_KIN"] != target.IDN {
		t.Fatalf(
			"COMPOSITION_KIN = %q want %q",
			em.REL["COMPOSITION_KIN"],
			target.IDN,
		)
	}

	if em.IDN == target.IDN {
		t.Fatal("governed composition rework self-referenced target")
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf(
			"reworked candidate state = %q want %q",
			em.STA,
			core.StateAtGOV,
		)
	}

	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatal("governed composition rework did not pass RECOIL/WVC")
	}

	afterState, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	afterReturned, ok := afterState.Returned[proposal.IDN]
	if !ok {
		t.Fatal("returned predecessor disappeared after governed rework")
	}

	if afterReturned.IDN != beforeReturned.IDN ||
		afterReturned.MEM.SourceHash != beforeReturned.MEM.SourceHash ||
		afterReturned.REL["capability_provider_edge_proposal"] !=
			beforeReturned.REL["capability_provider_edge_proposal"] {
		t.Fatal("returned predecessor was mutated by governed composition rework")
	}

	admitted, ok := afterState.AtGOV[em.IDN]
	if !ok {
		t.Fatal("governed composition rework did not return to G")
	}

	if admitted.REL["COMPOSITION_KIN"] != target.IDN {
		t.Fatal("stored governed composition target changed")
	}

	if admitted.EVO.Supersedes != proposal.IDN {
		t.Fatal("stored sovereign rework lineage changed")
	}
}

func TestGovernedProviderCompositionCirculatesThroughSAWExecutionRecapture(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	target := core.EmergION{
		IDN: "E-CIRCULATION-COMPOSITION-TARGET",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "circulation-composition-target-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "governed circulation composition target",
		},
		REL: map[string]string{
			"source_name": "circulation-composition-target.txt",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(target); err != nil {
		t.Fatal(err)
	}

	approvedTarget, targetDecision, err := gov.Decide(
		target,
		gov.Approve,
		"HUMAN_FINAL",
		"approve governed circulation composition target",
	)
	if err != nil {
		t.Fatal(err)
	}

	targetDecisionID, err := s.SaveDecision(targetDecision)
	if err != nil {
		t.Fatal(err)
	}

	_, targetReceipt, err := reg.Accept(
		approvedTarget,
		targetDecisionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(targetReceipt); err != nil {
		t.Fatal(err)
	}

	proposal := core.EmergION{
		IDN: "E-CIRCULATION-PROVIDER-EDGE-PROPOSAL",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "circulation-provider-edge-proposal-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "provider edge proposal awaiting governed circulation composition",
		},
		REL: map[string]string{
			"source_name":                       "circulation-provider-edge-proposal.txt",
			"capability_provider_edge_proposal": "E-PROVIDER-ANALYZE->E-PROVIDER-CMP,E-PROVIDER-CMP->E-PROVIDER-RLT",
		},
		CAP: []string{"OBS"},
		VAL: core.Validation{
			Facts:  []string{"source_preserved"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	if _, err := s.SaveCandidate(proposal); err != nil {
		t.Fatal(err)
	}

	returned, returnDecision, err := gov.Decide(
		proposal,
		gov.Return,
		"HUMAN_FINAL",
		"return provider edge proposal for governed circulation composition",
	)
	if err != nil {
		t.Fatal(err)
	}

	if returned.STA != core.StateReturned {
		t.Fatalf(
			"returned proposal state = %q want %q",
			returned.STA,
			core.StateReturned,
		)
	}

	if _, err := s.SaveDecision(returnDecision); err != nil {
		t.Fatal(err)
	}

	reworkSource := filepath.Join(
		root,
		"governed-provider-composition-rework.txt",
	)

	if err := os.WriteFile(
		reworkSource,
		[]byte("governed provider composition circulation rework"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	compositionRuntime := Runtime{
		Store:               s,
		ReturnedPredecessor: proposal.IDN,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "governed provider composition circulation",
				Relationships: map[string]string{
					"source_name":     "governed-provider-composition-rework.txt",
					"COMPOSITION_KIN": target.IDN,
				},
				Capabilities: []string{"OBS"},
				Facts:        []string{"source_preserved"},
				Risk:         "L",
			},
		},
	}

	composed, duplicate, err := compositionRuntime.Capture(
		context.Background(),
		reworkSource,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if duplicate {
		t.Fatal("governed composition rework unexpectedly duplicate")
	}

	if composed.EVO.Supersedes != proposal.IDN {
		t.Fatalf(
			"composition supersedes = %q want %q",
			composed.EVO.Supersedes,
			proposal.IDN,
		)
	}

	if composed.REL["COMPOSITION_KIN"] != target.IDN {
		t.Fatalf(
			"composition target = %q want %q",
			composed.REL["COMPOSITION_KIN"],
			target.IDN,
		)
	}

	if composed.STA != core.StateAtGOV {
		t.Fatalf(
			"composition state = %q want %q",
			composed.STA,
			core.StateAtGOV,
		)
	}

	if !composed.VAL.Recoil || !composed.VAL.WVC {
		t.Fatal("governed composition did not pass RECOIL/WVC")
	}

	approvedComposition, compositionDecision, err := gov.Decide(
		composed,
		gov.Approve,
		"HUMAN_FINAL",
		"approve governed provider composition circulation",
	)
	if err != nil {
		t.Fatal(err)
	}

	compositionDecisionID, err := s.SaveDecision(compositionDecision)
	if err != nil {
		t.Fatal(err)
	}

	acceptedComposition, compositionReceipt, err := reg.Accept(
		approvedComposition,
		compositionDecisionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(compositionReceipt); err != nil {
		t.Fatal(err)
	}

	st, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.Accepted[acceptedComposition.IDN]; !ok {
		t.Fatal("governed composition did not reach REG")
	}

	sources, err := proj.SAWSources(st)
	if err != nil {
		t.Fatal(err)
	}

	if len(sources) != 1 {
		t.Fatalf(
			"SAW source count = %d want 1",
			len(sources),
		)
	}

	sawSourcePath := filepath.Join(
		root,
		"governed-provider-composition.saw.mxpd",
	)

	if err := os.WriteFile(
		sawSourcePath,
		sources[0].Content,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	executionRuntime := Runtime{
		Store: s,
		Reasoner: lineageReasoner{
			result: reason.Result{
				Summary: "governed provider composition SAW source",
				Relationships: map[string]string{
					"source_name": "governed-provider-composition.saw.mxpd",
				},
				Capabilities: []string{"ANALYZE"},
				Facts:        []string{"saw_source_preserved"},
				Risk:         "L",
				Facets: []string{
					"ANALYTICS_FORECAST",
				},
			},
		},
	}

	sawEmergION, duplicate, err := executionRuntime.Capture(
		context.Background(),
		sawSourcePath,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if duplicate {
		t.Fatal("governed composition SAW source unexpectedly duplicate")
	}

	if sawEmergION.STA != core.StateAtGOV {
		t.Fatalf(
			"SAW EmergION state = %q want %q",
			sawEmergION.STA,
			core.StateAtGOV,
		)
	}

	approvedSAW, sawDecision, err := gov.Decide(
		sawEmergION,
		gov.Approve,
		"HUMAN_FINAL",
		"approve governed composition SAW source for execution",
	)
	if err != nil {
		t.Fatal(err)
	}

	sawDecisionID, err := s.SaveDecision(sawDecision)
	if err != nil {
		t.Fatal(err)
	}

	acceptedSAW, sawReceipt, err := reg.Accept(
		approvedSAW,
		sawDecisionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(sawReceipt); err != nil {
		t.Fatal(err)
	}

	st, err = livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := st.Accepted[acceptedSAW.IDN]; !ok {
		t.Fatal("governed composition SAW source did not reach REG")
	}

	request, err := adapters.PrepareExecution(
		st,
		acceptedSAW.IDN,
		"LOCAL_GEMMA",
		"ANALYZE",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "governed provider composition circulation proof",
		},
	)

	if err := adapters.VerifyExecutionResult(request, result); err != nil {
		t.Fatal(err)
	}

	signal, duplicate, err := executionRuntime.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}

	if duplicate {
		t.Fatal("governed composition execution result unexpectedly duplicate")
	}

	if signal.STA != core.StateAtGOV {
		t.Fatalf(
			"execution RECAPTURE state = %q want %q",
			signal.STA,
			core.StateAtGOV,
		)
	}

	if !signal.VAL.Recoil || !signal.VAL.WVC {
		t.Fatal("execution RECAPTURE did not pass RECOIL/WVC")
	}

	if signal.REL["parent_emergion"] != acceptedSAW.IDN {
		t.Fatalf(
			"execution parent = %q want %q",
			signal.REL["parent_emergion"],
			acceptedSAW.IDN,
		)
	}

	finalState, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	admittedSignal, ok := finalState.AtGOV[signal.IDN]
	if !ok {
		t.Fatal("execution RECAPTURE did not return to G")
	}

	if admittedSignal.REL["parent_emergion"] != acceptedSAW.IDN {
		t.Fatal("stored execution lineage changed")
	}

	if _, ok := finalState.Accepted[acceptedComposition.IDN]; !ok {
		t.Fatal("governed composition disappeared during circulation")
	}

	if _, ok := finalState.Accepted[acceptedSAW.IDN]; !ok {
		t.Fatal("accepted SAW EmergION disappeared during circulation")
	}
}

func TestDeriveDeltaIncludesDeterministicBuildGraphEvolution(t *testing.T) {
	previous := core.EmergION{
		MEM: core.Memory{
			Summary: "same implementation",
		},
		EVO: core.Evolution{
			Metadata: &core.Metadata{
				BuildNodes: []core.BuildNode{
					{
						ID:     "A",
						System: "SOURCE",
						State:  "READY",
					},
					{
						ID:     "B",
						System: "TRANSFORM",
						State:  "READY",
					},
				},
				BuildEdges: []core.BuildEdge{
					{
						From: "A",
						To:   "B",
						Kind: "FLOW",
					},
				},
			},
		},
	}

	analysis := reason.Result{
		Summary: "same implementation",
		BuildNodes: []reason.BuildNode{
			{
				ID:     "A",
				System: "SOURCE",
				State:  "ACTIVE",
			},
			{
				ID:     "C",
				System: "OUTPUT",
				State:  "READY",
			},
		},
		BuildEdges: []reason.BuildEdge{
			{
				From: "A",
				To:   "C",
				Kind: "FLOW",
			},
		},
	}

	got := deriveDelta(previous, analysis)

	want := []string{
		"DN:+:C",
		"DN:-:B",
		"DN:~:A",
		"DE:+:A:C:FLOW",
		"DE:-:A:B:FLOW",
	}

	if len(got) != len(want) {
		t.Fatalf(
			"structural delta = %#v want %#v",
			got,
			want,
		)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf(
				"structural delta[%d] = %q want %q; full=%#v",
				i,
				got[i],
				want[i],
				got,
			)
		}
	}
}
func TestAcceptedCapabilityProviderPopulationPreservesAllDeterministicCombinations(t *testing.T) {
	st := core.EmptyState()

	st.Accepted["E-ANALYZE-A"] = core.EmergION{
		IDN: "E-ANALYZE-A",
		STA: core.StateAccepted,
		CAP: []string{"ANALYZE"},
	}
	st.Accepted["E-CMP-A"] = core.EmergION{
		IDN: "E-CMP-A",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}
	st.Accepted["E-CMP-B"] = core.EmergION{
		IDN: "E-CMP-B",
		STA: core.StateAccepted,
		CAP: []string{"CMP"},
	}
	st.Accepted["E-RLT-A"] = core.EmergION{
		IDN: "E-RLT-A",
		STA: core.StateAccepted,
		CAP: []string{"RLT"},
	}

	population, ok := acceptedCapabilityProviderPopulation(
		"DERIVE_CAPABILITY",
		st,
	)
	if !ok {
		t.Fatal("accepted provider population unexpectedly unresolved")
	}

	want := []string{
		"ANALYZE:E-ANALYZE-A,CMP:E-CMP-A,RLT:E-RLT-A",
		"ANALYZE:E-ANALYZE-A,CMP:E-CMP-B,RLT:E-RLT-A",
	}

	if !reflect.DeepEqual(population, want) {
		t.Fatalf("population = %#v want %#v", population, want)
	}
}
