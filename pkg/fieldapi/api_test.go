package fieldapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

type sawCirculationReasoner struct{}

func (sawCirculationReasoner) Analyze(
	_ context.Context,
	in reason.Input,
) (reason.Result, error) {
	return reason.Result{
		Summary: "bounded " + in.Name,
		Relationships: map[string]string{
			"source_kind": "PROGRAM",
		},
		Capabilities: []string{
			"OBS",
			"CMP",
			"ANALYZE",
		},
		Facts: []string{
			"source_preserved",
		},
		Risk: "L",
	}, nil
}

func (sawCirculationReasoner) Name() string {
	return "fieldapi-saw-circulation-test"
}

func (sawCirculationReasoner) Version(context.Context) string {
	return "1"
}

func TestCirculateSAWsUsesGovernedCaptureWithoutSelfAcceptance(t *testing.T) {
	root := t.TempDir()

	rt, err := Open(
		filepath.Join(root, "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	sourceA := core.EmergION{
		IDN: "E-FIELDAPI-SAW-A",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "fieldapi-saw-source-a",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted SAW source A",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-FIELDAPI-SAW-B",
		},
		CAP: []string{
			"OBS",
			"ANALYZE",
		},
		VAL: core.Validation{
			Facts:  []string{"bounded source A"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	sourceB := core.EmergION{
		IDN: "E-FIELDAPI-SAW-B",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "fieldapi-saw-source-b",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted SAW source B",
		},
		REL: map[string]string{},
		CAP: []string{
			"CMP",
		},
		VAL: core.Validation{
			Facts:  []string{"bounded source B"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	accept := func(em core.EmergION) {
		if _, err := rt.store.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			"fieldapi SAW circulation proof",
		)
		if err != nil {
			t.Fatal(err)
		}

		decisionID, err := rt.store.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}

		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := rt.store.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	accept(sourceA)
	accept(sourceB)

	before, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	if len(before.Accepted) != 2 {
		t.Fatalf(
			"accepted prerequisite count = %d want 2",
			len(before.Accepted),
		)
	}

	circulated, err := rt.CirculateSAWs(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(circulated) != 1 {
		t.Fatalf(
			"circulated SAWs = %d want 1",
			len(circulated),
		)
	}

	em := circulated[0]

	if em.STA != core.StateAtGOV {
		t.Fatalf(
			"circulated SAW state = %q want %q",
			em.STA,
			core.StateAtGOV,
		)
	}

	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatal("circulated SAW did not pass governed RECOIL/WVC")
	}

	if em.MEM.SourceHash == "" {
		t.Fatal("circulated SAW source identity missing")
	}

	wantSourceName := "SAW:SAAB:E-FIELDAPI-SAW-A+E-FIELDAPI-SAW-B"
	if got := em.REL["source_name"]; got != wantSourceName {
		t.Fatalf(
			"circulated SAW source_name = %q want %q",
			got,
			wantSourceName,
		)
	}

	if !strings.Contains(em.MEM.Summary, "SAW:") {
		t.Fatalf(
			"circulated SAW summary does not identify SAW source: %q",
			em.MEM.Summary,
		)
	}

	after, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := after.AtGOV[em.IDN]; !ok {
		t.Fatalf(
			"circulated SAW %s did not enter GOV",
			em.IDN,
		)
	}

	if _, ok := after.Accepted[em.IDN]; ok {
		t.Fatal("CirculateSAWs self-authorized SAW into REG")
	}

	second, err := rt.CirculateSAWs(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(second) != 0 {
		t.Fatalf(
			"duplicate SAW circulation emitted %d candidates",
			len(second),
		)
	}

	finalState, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	if len(finalState.Accepted) != 2 {
		t.Fatalf(
			"CirculateSAWs mutated accepted authority: %d",
			len(finalState.Accepted),
		)
	}

	if len(finalState.AtGOV) != 1 {
		t.Fatalf(
			"unexpected GOV candidate count after idempotent circulation: %d",
			len(finalState.AtGOV),
		)
	}

	events, err := rt.store.Events()
	if err != nil {
		t.Fatal(err)
	}

	for _, event := range events {
		if event.EmergION == nil ||
			event.EmergION.IDN != em.IDN {
			continue
		}

		if event.Type != "C" {
			t.Fatalf(
				"circulated SAW unexpectedly wrote authority event %q",
				event.Type,
			)
		}
	}
}

func TestCirculateSAWsDoesNothingWithoutGovernedComposition(t *testing.T) {
	root := t.TempDir()

	rt, err := Open(
		filepath.Join(root, "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := rt.CirculateSAWs(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 0 {
		t.Fatalf(
			"empty accepted FIELD produced %d SAWs",
			len(out),
		)
	}

	events, err := rt.store.Events()
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 0 {
		t.Fatalf(
			"empty circulation wrote %d events",
			len(events),
		)
	}
}

var _ = store.Hash

func TestStatusJSONUsesExistingRuntimeState(t *testing.T) {
	rt, err := Open(
		filepath.Join(t.TempDir(), "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	wire, err := rt.StatusJSON()
	if err != nil {
		t.Fatal(err)
	}

	if !json.Valid([]byte(wire)) {
		t.Fatalf("StatusJSON returned invalid JSON: %q", wire)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatal(err)
	}

	if _, ok := got["events"]; !ok {
		t.Fatalf("StatusJSON missing canonical events metric: %s", wire)
	}

	if _, ok := got["tip_hash"]; !ok {
		t.Fatalf("StatusJSON missing canonical tip hash: %s", wire)
	}
}

func TestActionsJSONCannotBypassREGAcceptance(t *testing.T) {
	rt, err := Open(
		filepath.Join(t.TempDir(), "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := rt.ActionsJSON("E-NOT-REG-ACCEPTED", true); err == nil {
		t.Fatal("ActionsJSON bypassed REG acceptance")
	}
}

func TestDecideBindingCannotBypassGOV(t *testing.T) {
	rt, err := Open(
		filepath.Join(t.TempDir(), "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.DecideBinding(
		"E-NOT-AT-GOV",
		"APPROVE",
		"binding boundary rejection proof",
	); err == nil {
		t.Fatal("DecideBinding bypassed GOV")
	}
}

func TestAuthorizeBindingCannotBypassREGAcceptance(t *testing.T) {
	rt, err := Open(
		filepath.Join(t.TempDir(), "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := rt.AuthorizeBinding(
		"E-NOT-REG-ACCEPTED",
		"LOCAL_GEMMA",
		"ANALYZE",
		"binding boundary rejection proof",
		true,
	); err == nil {
		t.Fatal("AuthorizeBinding bypassed REG acceptance")
	}
}

func TestRenderCurrentJSONUsesCanonicalProjectionReceipt(t *testing.T) {
	rt, err := Open(
		filepath.Join(t.TempDir(), "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "projection")

	wire, err := rt.RenderCurrentJSON(out)
	if err != nil {
		t.Fatal(err)
	}

	if !json.Valid([]byte(wire)) {
		t.Fatalf("RenderCurrentJSON returned invalid JSON: %q", wire)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatal(err)
	}

	if _, ok := got["tip_hash"]; !ok {
		t.Fatalf("projection receipt missing tip_hash: %s", wire)
	}

	if _, ok := got["field_json_sha256"]; !ok {
		t.Fatalf("projection receipt missing field_json_sha256: %s", wire)
	}

	if _, ok := got["field_html_sha256"]; !ok {
		t.Fatalf("projection receipt missing field_html_sha256: %s", wire)
	}
}

func TestGovernedCycleCirculatesAndExecutesExistingSafeWork(t *testing.T) {
	binary := os.Getenv("GEMMA_BIN")
	model := os.Getenv("GEMMA_MODEL")
	if binary == "" || model == "" {
		t.Skip("set GEMMA_BIN and GEMMA_MODEL for GovernedCycle integration test")
	}

	root := t.TempDir()

	rt, err := Open(
		filepath.Join(root, "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	accept := func(em core.EmergION, reasonText string) {
		if _, err := rt.store.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			reasonText,
		)
		if err != nil {
			t.Fatal(err)
		}

		decisionID, err := rt.store.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}

		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := rt.store.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	compositionTarget := core.EmergION{
		IDN: "E-GOVERNED-CYCLE-TARGET",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "governed-cycle-target-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "governed cycle composition target",
		},
		REL: map[string]string{},
		CAP: []string{"CMP"},
		VAL: core.Validation{
			Facts:  []string{"bounded target"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	compositionEvidence := []byte("governed cycle composition source")
	compositionStored, err := rt.store.Preserve(compositionEvidence)
	if err != nil {
		t.Fatal(err)
	}

	compositionSource := core.EmergION{
		IDN: "E-GOVERNED-CYCLE-COMPOSITION",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: compositionStored.Hash,
			Codec:      compositionStored.Codec,
			Bytes:      compositionStored.Bytes,
			Stored:     compositionStored.Stored,
			Summary:    "governed cycle composition source",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": compositionTarget.IDN,
		},
		CAP: []string{"OBS", "ANALYZE"},
		VAL: core.Validation{
			Facts:  []string{"bounded source"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	safeEvidence := []byte("existing REG accepted safe local analyze work")
	evidence, err := rt.store.Preserve(safeEvidence)
	if err != nil {
		t.Fatal(err)
	}

	safeWork := core.EmergION{
		IDN: "E-GOVERNED-CYCLE-SAFE-WORK",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: evidence.Hash,
			Codec:      evidence.Codec,
			Bytes:      evidence.Bytes,
			Stored:     evidence.Stored,
			Summary:    "existing safe ANALYZE work",
		},
		REL: map[string]string{
			"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED",
		},
		CAP: []string{"ANALYZE"},
		VAL: core.Validation{
			Facts:  []string{"safe work preserved"},
			Risk:   "L",
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   time.Now().UTC(),
				AIIntegrated: false,
				PromptSchema: "MXPD/2",
				Facets: []core.Facet{
					core.FacetAnalyticsForecast,
				},
			},
		},
	}

	accept(compositionTarget, "accept governed cycle target")
	accept(compositionSource, "accept governed cycle composition")
	accept(safeWork, "accept existing safe local analyze work")

	gemma := reason.GemmaCLI{
		Binary:    binary,
		Model:     model,
		Threads:   4,
		Context:   2048,
		MaxTokens: 80,
		Timeout:   180 * time.Second,
		ExtraArgs: []string{"--seed", "1"},
	}

	circulated, signal, executed, err := rt.GovernedCycle(
		context.Background(),
		gemma,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(circulated) != 1 {
		t.Fatalf("circulated = %d want 1", len(circulated))
	}

	if !executed {
		t.Fatal("GovernedCycle did not execute existing safe work")
	}

	if signal.STA != core.StateAtGOV {
		t.Fatalf(
			"execution signal state = %q want %q",
			signal.STA,
			core.StateAtGOV,
		)
	}

	if !signal.VAL.Recoil || !signal.VAL.WVC {
		t.Fatal("execution signal did not pass RECOIL/WVC")
	}
	if signal.REL["transition_emergion"] == "" {
		t.Fatal("T_n execution signal lost travelling EmergION identity")
	}

	state, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	circulatedID := circulated[0].IDN

	if _, ok := state.AtGOV[circulatedID]; !ok {
		t.Fatal("circulated SAW did not remain at GOV")
	}

	if _, ok := state.Accepted[circulatedID]; ok {
		t.Fatal("GovernedCycle self-authorized circulated SAW into REG")
	}

	if _, ok := state.AtGOV[signal.IDN]; !ok {
		t.Fatal("execution RECAPTURE signal did not return to GOV")
	}

	if _, ok := state.Accepted[signal.IDN]; ok {
		t.Fatal("GovernedCycle self-authorized execution signal into REG")
	}

	// Cycle N produced candidates only. HUMAN_FINAL now makes the explicit
	// authority transition through the existing Decide -> REG path.
	if err := rt.Decide(
		circulatedID,
		string(gov.Approve),
		"approve cycle N circulated SAW for recursive emergence",
	); err != nil {
		t.Fatal(err)
	}

	if err := rt.Decide(
		signal.IDN,
		string(gov.Approve),
		"approve cycle N execution observation for recursive emergence",
	); err != nil {
		t.Fatal(err)
	}

	acceptedState, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := acceptedState.Accepted[circulatedID]; !ok {
		t.Fatal("HUMAN_FINAL-approved cycle N SAW did not reach REG")
	}

	if _, ok := acceptedState.Accepted[signal.IDN]; !ok {
		t.Fatal("HUMAN_FINAL-approved cycle N execution signal did not reach REG")
	}

	// T_n+1 must now operate through the same existing governed cycle,
	// using the newly REG-accepted canonical state rather than bypassing it.
	circulatedNext, signalNext, executedNext, err := rt.GovernedCycle(
		context.Background(),
		gemma,
	)
	if err != nil {
		t.Fatal(err)
	}

	nextState, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	if !executedNext {
		t.Fatal("T_n+1 did not execute newly governed safe work")
	}

	if signalNext.STA != core.StateAtGOV {
		t.Fatalf("T_n+1 execution signal state = %q want %q", signalNext.STA, core.StateAtGOV)
	}

	if signalNext.REL["transition_emergion"] == "" {
		t.Fatal("T_n+1 execution signal lost travelling EmergION identity")
	}

	if signalNext.REL["transition_emergion"] == signal.REL["transition_emergion"] {
		t.Fatalf(
			"T_n+1 travelling EmergION identity reused T_n identity: %q",
			signalNext.REL["transition_emergion"],
		)
	}

	if !signalNext.VAL.Recoil || !signalNext.VAL.WVC {
		t.Fatal("T_n+1 execution signal did not pass RECOIL/WVC")
	}

	if signalNext.REL["parent_emergion"] != circulatedID {
		t.Fatalf(
			"T_n+1 execution parent = %q want governed T_n SAW %q",
			signalNext.REL["parent_emergion"],
			circulatedID,
		)
	}

	if _, ok := nextState.AtGOV[signalNext.IDN]; !ok {
		t.Fatal("T_n+1 execution RECAPTURE did not return to GOV")
	}

	if _, ok := nextState.Accepted[signalNext.IDN]; ok {
		t.Fatal("T_n+1 self-authorized its execution signal into REG")
	}

	for _, em := range circulatedNext {
		if _, ok := nextState.Accepted[em.IDN]; ok {
			t.Fatalf("T_n+1 self-authorized circulated SAW %s into REG", em.IDN)
		}
	}

}

func TestRunDrivesSuccessiveGovernedCyclesWithoutManualInvocation(t *testing.T) {
	root := t.TempDir()
	binary := os.Getenv("GEMMA_BIN")
	model := os.Getenv("GEMMA_MODEL")
	if binary == "" || model == "" {
		t.Skip("set GEMMA_BIN and GEMMA_MODEL for unattended Run integration test")
	}

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	rt := &Runtime{
		store:    s,
		reasoner: sawCirculationReasoner{},
	}

	accept := func(em core.EmergION, reasonText string, evidence []byte) {
		t.Helper()

		em.STA = core.StateAtGOV
		em.VAL.Recoil = true
		em.VAL.WVC = true

		if em.EVO.Version == 0 {
			em.EVO.Version = 1
		}
		if em.EVO.Metadata == nil {
			em.EVO.Metadata = &core.Metadata{
				CapturedAt:   time.Now().UTC(),
				AIIntegrated: false,
				PromptSchema: "MXPD/2",
			}
		}

		if len(evidence) == 0 {

			evidence = []byte(em.IDN)

		}
		preserved, err := s.Preserve(evidence)
		if err != nil {
			t.Fatal(err)
		}

		em.MEM.SourceHash = preserved.Hash
		em.MEM.Codec = preserved.Codec
		em.MEM.Bytes = preserved.Bytes
		em.MEM.Stored = preserved.Stored

		if _, err := s.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			reasonText,
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

	safeWork := core.EmergION{
		IDN: "E-RUN-AUTONOMY-SAFE-WORK",
		CAP: []string{"ANALYZE"},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt:   time.Now().UTC(),
				AIIntegrated: false,
				PromptSchema: "MXPD/2",
				Facets: []core.Facet{
					core.FacetAnalyticsForecast,
				},
			},
		},
	}

	accept(safeWork, "accept unattended run safe work", []byte("This preserved source is an analytics forecast workload for analysis."))

	gemma := reason.GemmaCLI{
		Binary:    binary,
		Model:     model,
		Threads:   4,
		Context:   2048,
		MaxTokens: 80,
		Timeout:   180 * time.Second,
		ExtraArgs: []string{"--seed", "1"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type cycleObservation struct {
		circulated []core.EmergION
		signal     core.EmergION
		executed   bool
	}

	cycles := make(chan cycleObservation, 4)
	runErr := make(chan error, 1)

	go func() {
		runErr <- rt.Run(
			ctx,
			filepath.Join(root, "dropzone"),
			10*time.Millisecond,
			gemma,
			nil,
			func(
				circulated []core.EmergION,
				signal core.EmergION,
				executed bool,
			) {
				cycles <- cycleObservation{
					circulated: circulated,
					signal:     signal,
					executed:   executed,
				}
			},
		)
	}()

	var observed []cycleObservation

	timeout := time.NewTimer(300 * time.Second)
	defer timeout.Stop()

	for len(observed) < 2 {
		select {
		case cycle := <-cycles:
			observed = append(observed, cycle)

		case err := <-runErr:
			if err != nil {
				t.Fatalf("Run exited before two cycles: %v", err)
			}
			t.Fatal("Run exited before two cycles without error")

		case <-timeout.C:
			t.Fatal("Run did not produce two unattended governed cycles")
		}
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run cancellation error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}

	if !observed[0].executed {
		t.Fatal("first unattended governed cycle did not execute existing safe work")
	}

	firstSignal := observed[0].signal
	if firstSignal.STA != core.StateAtGOV {
		t.Fatalf(
			"first unattended execution signal state = %q want %q",
			firstSignal.STA,
			core.StateAtGOV,
		)
	}

	if !firstSignal.VAL.Recoil || !firstSignal.VAL.WVC {
		t.Fatal("first unattended execution signal bypassed RECOIL/WVC")
	}

	if firstSignal.REL["parent_emergion"] != safeWork.IDN {
		t.Fatalf(
			"first unattended execution parent = %q want %q",
			firstSignal.REL["parent_emergion"],
			safeWork.IDN,
		)
	}

	state, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := state.AtGOV[firstSignal.IDN]; !ok {
		t.Fatal("unattended execution RECAPTURE did not return to GOV")
	}

	if _, ok := state.Accepted[firstSignal.IDN]; ok {
		t.Fatal("unattended Run self-authorized execution result into REG")
	}

	if len(observed) < 2 {
		t.Fatal("unattended Run did not reach a second governed cycle")
	}
}

func TestRunContinuesHumanFinalReturnedBeforeIdleProjection(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{
		store:    s,
		reasoner: sawCirculationReasoner{},
	}

	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "fake-gemma")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	gemma := reason.GemmaCLI{
		Binary:    fake,
		Model:     model,
		Context:   2048,
		MaxTokens: 80,
		Threads:   1,
		Timeout:   5 * time.Second,
	}
	if err := gemma.Validate(); err != nil {
		t.Fatalf("fake Gemma validation failed: %v", err)
	}

	returned := core.EmergION{
		IDN: "E-RUN-RETURNED-CONTINUATION",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "run-returned-continuation-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "governed source requiring HUMAN_FINAL rework",
		},
		REL: map[string]string{
			"source_kind": "SOURCE",
		},
		CAP: []string{"OBS", "CMP"},
		VAL: core.Validation{
			Facts:  []string{"bounded returned source"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{Version: 1},
	}

	if _, err := rt.store.SaveCandidate(returned); err != nil {
		t.Fatal(err)
	}
	_, decision, err := gov.Decide(
		returned,
		gov.Return,
		"HUMAN_FINAL",
		"return for bounded governed continuation",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.store.SaveDecision(decision); err != nil {
		t.Fatal(err)
	}

	before, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := before.Returned[returned.IDN]; !ok {
		t.Fatal("HUMAN_FINAL RETURN was not reconstructed in FIELD")
	}
	if len(before.AtGOV) != 0 {
		t.Fatalf("fixture has %d AtGOV candidates; want 0", len(before.AtGOV))
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- rt.Run(
			ctx,
			filepath.Join(root, "dropzone"),
			10*time.Millisecond,
			gemma,
			nil,
			nil,
		)
	}()

	var continuation core.EmergION
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, stateErr := rt.state()
		if stateErr != nil {
			cancel()
			t.Fatal(stateErr)
		}
		for _, em := range state.AtGOV {
			if em.EVO.Supersedes == returned.IDN {
				continuation = em
				break
			}
		}
		if continuation.IDN != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}

	if continuation.IDN == "" {
		t.Fatal("Run did not derive governed continuation from HUMAN_FINAL RETURNED source")
	}
	if continuation.STA != core.StateAtGOV {
		t.Fatalf("continuation state = %q want %q", continuation.STA, core.StateAtGOV)
	}
	if !continuation.VAL.Recoil || !continuation.VAL.WVC {
		t.Fatal("Returned continuation bypassed RECOIL/WVC")
	}

	after, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Accepted[continuation.IDN]; ok {
		t.Fatal("Returned continuation self-authorized into REG")
	}
	if _, ok := after.Returned[returned.IDN]; !ok {
		t.Fatal("Returned continuation mutated its HUMAN_FINAL predecessor")
	}
}

func TestGovernedCycleCirculatesSAWBeforeProgramProposalFailure(t *testing.T) {
	root := t.TempDir()

	rt, err := Open(
		filepath.Join(root, "state"),
		sawCirculationReasoner{},
	)
	if err != nil {
		t.Fatal(err)
	}

	sourceA := core.EmergION{
		IDN: "E-FIELDAPI-CYCLE-SAW-A",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "fieldapi-cycle-saw-source-a",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted governed composition source A",
		},
		REL: map[string]string{
			"COMPOSITION_KIN": "E-FIELDAPI-CYCLE-SAW-B",
			"source_name":     "internal/adapters/actions.go",
		},
		CAP: []string{
			"OBS",
			"ANALYZE",
		},
		VAL: core.Validation{
			Facts:  []string{"bounded source A"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				CapturedAt:   time.Now().UTC(),
				Topology:     core.TopologyDodecahedronV1,
				PromptSchema: "MXPD/2",
				Facets:       []core.Facet{core.FacetProgramForge},
			},
		},
	}

	sourceB := core.EmergION{
		IDN: "E-FIELDAPI-CYCLE-SAW-B",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "fieldapi-cycle-saw-source-b",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted governed composition source B",
		},
		REL: map[string]string{},
		CAP: []string{"CMP"},
		VAL: core.Validation{
			Facts:  []string{"bounded source B"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{
			Version: 1,
		},
	}

	accept := func(em core.EmergION) {
		if _, err := rt.store.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			"fieldapi governed-cycle ordering proof",
		)
		if err != nil {
			t.Fatal(err)
		}

		decisionID, err := rt.store.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}

		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rt.store.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	accept(sourceA)
	accept(sourceB)

	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := filepath.Join(root, "fake-gemma")
	if err := os.WriteFile(
		fake,
		[]byte("#!/bin/sh\nexit 7\n"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}

	circulated, _, executed, err := rt.GovernedCycle(
		context.Background(),
		reason.GemmaCLI{
			Binary:    fake,
			Model:     model,
			Context:   2048,
			MaxTokens: 80,
			Threads:   1,
			Timeout:   5 * time.Second,
		},
	)
	if err == nil {
		t.Fatal("PROGRAM proposal failure was not returned")
	}
	if executed {
		t.Fatal("safe action executed after PROGRAM proposal failure")
	}
	if len(circulated) == 0 {
		t.Fatal("SAW circulation was lost when PROGRAM proposal failed")
	}

	state, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}

	for _, em := range circulated {
		if _, ok := state.AtGOV[em.IDN]; !ok {
			t.Fatalf("circulated SAW %s missing from GOV", em.IDN)
		}
		if _, ok := state.Accepted[em.IDN]; ok {
			t.Fatalf("circulated SAW %s self-authorized into REG", em.IDN)
		}
	}
}

func TestRunRecomparesPersistentTargetWhenAcceptedRealityChanges(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{
		store:    s,
		reasoner: sawCirculationReasoner{},
	}
	if rt.store == nil {
		t.Fatal("runtime store not initialized")
	}

	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := filepath.Join(root, "fake-gemma")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	gemma := reason.GemmaCLI{
		Binary:    fake,
		Model:     model,
		Context:   2048,
		MaxTokens: 80,
		Threads:   1,
		Timeout:   5 * time.Second,
	}
	if err := gemma.Validate(); err != nil {
		t.Fatalf("fake Gemma validation failed: %v", err)
	}
	accept := func(em core.EmergION) {
		if _, err := rt.store.SaveCandidate(em); err != nil {
			t.Fatal(err)
		}
		approved, decision, err := gov.Decide(
			em,
			gov.Approve,
			"HUMAN_FINAL",
			"deterministic target recurrence proof",
		)
		if err != nil {
			t.Fatal(err)
		}
		decisionID, err := rt.store.SaveDecision(decision)
		if err != nil {
			t.Fatal(err)
		}
		_, receipt, err := reg.Accept(approved, decisionID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rt.store.SaveAccepted(receipt); err != nil {
			t.Fatal(err)
		}
	}

	target := core.EmergION{
		IDN: "E-RUN-TARGET",
		MEM: core.Memory{SourceHash: "run-target-source", Bytes: 1, Stored: 1, Summary: "persistent target"},
		STA: core.StateAtGOV,
		REL: map[string]string{
			"source_kind":  "TARGET",
			"target_state": "runtime deployment state is deployed",
		},
		VAL: core.Validation{Recoil: true, WVC: true},
		EVO: core.Evolution{Version: 1},
	}
	reality0 := core.EmergION{
		IDN: "E-RUN-REALITY-0",
		MEM: core.Memory{SourceHash: "run-reality-0-source", Bytes: 1, Stored: 1, Summary: "accepted reality zero"},
		STA: core.StateAtGOV,
		REL: map[string]string{"source_kind": "SOURCE"},
		VAL: core.Validation{
			Facts:  []string{"runtime deployment state is observation-only"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{Version: 1},
	}

	accept(target)
	accept(reality0)

	st, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Accepted) != 2 {
		t.Fatalf("accepted fixture count = %d want 2", len(st.Accepted))
	}

	runUntilComparison := func(excludeID string) core.EmergION {
		t.Helper()

		ctx, cancel := context.WithCancel(context.Background())
		runErr := make(chan error, 1)

		go func() {
			runErr <- rt.Run(
				ctx,
				filepath.Join(root, "dropzone"),
				10*time.Millisecond,
				gemma,
				nil,
				nil,
			)
		}()

		var found core.EmergION
		deadline := time.Now().Add(2 * time.Second)

		for time.Now().Before(deadline) {
			state, stateErr := rt.state()
			if stateErr != nil {
				cancel()
				t.Fatal(stateErr)
			}

			for _, em := range state.AtGOV {
				if em.REL["source_kind"] == "TARGET_COMPARISON" &&
					em.REL["target_emergion"] == target.IDN &&
					em.IDN != excludeID {
					found = em
					break
				}
			}

			if found.IDN != "" {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		cancel()

		select {
		case err := <-runErr:
			if err != nil && err != context.Canceled {
				t.Fatalf("Run cancellation error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not stop after context cancellation")
		}

		if found.IDN == "" {
			t.Fatal("Run did not produce governed TARGET_COMPARISON")
		}

		return found
	}

	comparison0 := runUntilComparison("")
	if comparison0.STA != core.StateAtGOV {
		t.Fatalf(
			"target comparison state = %q want %q",
			comparison0.STA,
			core.StateAtGOV,
		)
	}
	if !comparison0.VAL.Recoil || !comparison0.VAL.WVC {
		t.Fatal("target comparison bypassed RECOIL/WVC")
	}

	// HUMAN_FINAL accepts the first comparison before accepted reality changes.
	accept(comparison0)

	reality1 := core.EmergION{
		IDN: "E-RUN-REALITY-1",
		STA: core.StateAtGOV,
		MEM: core.Memory{
			SourceHash: "run-reality-1-source",
			Bytes:      1,
			Stored:     1,
			Summary:    "accepted reality one",
		},
		REL: map[string]string{"source_kind": "SOURCE"},
		VAL: core.Validation{
			Facts:  []string{"runtime deployment state is staged"},
			Recoil: true,
			WVC:    true,
		},
		EVO: core.Evolution{Version: 1},
	}
	accept(reality1)

	comparison1 := runUntilComparison(comparison0.IDN)

	if comparison1.IDN == comparison0.IDN {
		t.Fatal("changed accepted reality reused previous target comparison")
	}
	if comparison1.REL["target_emergion"] != target.IDN {
		t.Fatalf(
			"recomparison target = %q want %q",
			comparison1.REL["target_emergion"],
			target.IDN,
		)
	}
	if comparison1.STA != core.StateAtGOV {
		t.Fatalf(
			"recomparison state = %q want %q",
			comparison1.STA,
			core.StateAtGOV,
		)
	}
	if !comparison1.VAL.Recoil || !comparison1.VAL.WVC {
		t.Fatal("recomparison bypassed RECOIL/WVC")
	}
}
