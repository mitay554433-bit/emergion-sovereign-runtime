package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/proj"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

func TestExecutionResultReentersEmergIONPipeline(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		AuthorizationID: "EV-Q-PROOF",
		Adapter:         "EMAIL",
		Action:          "SEND",
		Authority:       "SEND_GATED",
	}

	result := adapters.ExecutionResult{
		Adapter:   "EMAIL",
		Action:    "SEND",
		Succeeded: true,
		Output:    "bounded execution proof",
	}

	em, duplicate, err := r.CaptureExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("execution signal unexpectedly duplicate")
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("execution signal state = %s", em.STA)
	}
	if !em.VAL.Recoil || !em.VAL.WVC {
		t.Fatalf("execution signal not verified: %#v", em.VAL)
	}
	if em.MEM.Provenance == "" {
		t.Fatal("execution signal provenance missing")
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := st.AtGOV[em.IDN]
	if !ok {
		t.Fatalf("execution signal %s missing from GOV", em.IDN)
	}

	if got.REL["source_kind"] != "EXECUTION_RESULT" {
		t.Fatalf(
			"source kind = %q",
			got.REL["source_kind"],
		)
	}

	if got.REL["parent_emergion"] != "E-PARENT" {
		t.Fatalf(
			"parent = %q",
			got.REL["parent_emergion"],
		)
	}
	if got.REL["authorization_event"] != "EV-Q-PROOF" {
		t.Fatalf(
			"authorization event = %q want EV-Q-PROOF",
			got.REL["authorization_event"],
		)
	}

}

func TestFailedExecutionAlsoBecomesSignal(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
	}

	request := adapters.ExecutionRequest{
		EmergIONID: "E-PARENT",
		Adapter:    "EMAIL",
		Action:     "SEND",
		Authority:  "SEND_GATED",
	}

	result := adapters.ExecutionResult{
		Adapter:   "EMAIL",
		Action:    "SEND",
		Succeeded: false,
		Error:     "simulated execution failure",
	}

	em, duplicate, err := r.CaptureExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("failed execution signal unexpectedly duplicate")
	}

	if em.STA != core.StateAtGOV {
		t.Fatalf("failed execution signal state = %s", em.STA)
	}

	found := false
	for _, fact := range em.VAL.Facts {
		if fact == "execution_failed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("execution_failed fact missing: %#v", em.VAL.Facts)
	}

	if em.VAL.Risk != "M" {
		t.Fatalf("failed execution risk = %q", em.VAL.Risk)
	}
}

func TestExecutionResultMustMatchRequest(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	r := Runtime{
		Store: s,
	}

	_, _, err = r.CaptureExecutionResult(
		context.Background(),
		adapters.ExecutionRequest{
			EmergIONID: "E-PARENT",
			Adapter:    "EMAIL",
			Action:     "SEND",
		},
		adapters.ExecutionResult{
			Adapter:   "WEB",
			Action:    "DEPLOY",
			Succeeded: true,
		},
	)

	if err == nil {
		t.Fatal("mismatched execution result unexpectedly admitted")
	}
}

func TestExecutionTransitionPreRunSerializationIsDeterministic(t *testing.T) {
	request := adapters.ExecutionRequest{
		EmergIONID:           "E-PARENT",
		SourceHash:           "SOURCE-HASH-PROOF",
		AuthorizationID:      "EV-Q-PROOF",
		TransitionEmergIONID: "E-TRANSITION-PROOF",
		Authority:            "HUMAN_FINAL",
		Adapter:              "LOCAL_GEMMA",
		Action:               "ANALYZE",
	}

	first := adapters.ExecutionTransitionBytes(request)
	second := adapters.ExecutionTransitionBytes(request)

	if string(first) != string(second) {
		t.Fatalf(
			"pre-run transition serialization is nondeterministic\nfirst:\n%s\nsecond:\n%s",
			first,
			second,
		)
	}

	want := "" +
		"S=4:XS/1\n" +
		"K=2:XT\n" +
		"P=8:E-PARENT\n" +
		"H=17:SOURCE-HASH-PROOF\n" +
		"Q=10:EV-Q-PROOF\n" +
		"A=11:HUMAN_FINAL\n" +
		"D=11:LOCAL_GEMMA\n" +
		"X=7:ANALYZE\n"

	if string(first) != want {
		t.Fatalf(
			"unexpected pre-run transition bytes\nwant:\n%s\ngot:\n%s",
			want,
			first,
		)
	}
}

func TestExecutionTransitionIdentityDerivesDeterministicallyFromCanonicalBytes(t *testing.T) {
	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}

	first := adapters.ExecutionTransitionID(request)
	second := adapters.ExecutionTransitionID(request)

	if first == "" {
		t.Fatal("pre-run transition identity is empty")
	}

	if first != second {
		t.Fatalf(
			"pre-run transition identity is nondeterministic: %q != %q",
			first,
			second,
		)
	}

	if request.TransitionEmergIONID != "" {
		t.Fatalf(
			"identity probe mutated request carrier: %q",
			request.TransitionEmergIONID,
		)
	}
}

func TestExecutionRecapturePreservesTravellingEmergIONIdentity(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "APP Rover terminal proof",
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("terminal APP Rover signal unexpectedly duplicate")
	}

	if signal.REL["transition_emergion"] != request.TransitionEmergIONID {
		t.Fatalf(
			"terminal EmergION lost travelling identity: got %q want %q",
			signal.REL["transition_emergion"],
			request.TransitionEmergIONID,
		)
	}
}

func TestExecutionRecapturePreservesTravellingEmergIONThroughCOSLRebuild(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "APP Rover COSL rebuild proof",
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("APP Rover COSL proof unexpectedly duplicate")
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	rebuilt, ok := state.AtGOV[signal.IDN]
	if !ok {
		t.Fatalf(
			"recaptured execution EmergION %s missing after COSL rebuild",
			signal.IDN,
		)
	}

	if rebuilt.REL["transition_emergion"] != request.TransitionEmergIONID {
		t.Fatalf(
			"COSL rebuild lost travelling EmergION identity: got %q want %q",
			rebuilt.REL["transition_emergion"],
			request.TransitionEmergIONID,
		)
	}
}

func TestExecutionRoverAppearsInAtGOVFieldLifecycleProjection(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID =
		adapters.ExecutionTransitionID(request)

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "APP Rover FIELD lifecycle proof",
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("APP Rover FIELD proof unexpectedly duplicate")
	}

	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := state.AtGOV[signal.IDN]; !ok {
		t.Fatalf(
			"terminal APP Rover %s not AtGOV before projection",
			signal.IDN,
		)
	}

	fieldPath := filepath.Join(root, "field.html")

	if err := proj.HTML(fieldPath, state); err != nil {
		t.Fatal(err)
	}

	fieldBytes, err := os.ReadFile(fieldPath)
	if err != nil {
		t.Fatal(err)
	}

	field := string(fieldBytes)

	if !strings.Contains(field, signal.IDN) {
		t.Fatalf(
			"FIELD lifecycle projection lost terminal EmergION %s",
			signal.IDN,
		)
	}

	if !strings.Contains(field, "transition_emergion") {
		t.Fatal(
			"FIELD lifecycle projection does not expose transition_emergion",
		)
	}

	if !strings.Contains(
		field,
		request.TransitionEmergIONID,
	) {
		t.Fatalf(
			"FIELD lifecycle projection lost APP Rover identity %s",
			request.TransitionEmergIONID,
		)
	}
}

func TestAcceptedExecutionSignalPreservesTravellingEmergIONIdentity(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID =
		adapters.ExecutionTransitionID(request)

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    "APP Rover REG acceptance proof",
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("APP Rover REG proof unexpectedly duplicate")
	}

	approved, decision, err := gov.Decide(
		signal,
		gov.Approve,
		"HUMAN_FINAL",
		"accept APP Rover terminal evidence",
	)
	if err != nil {
		t.Fatal(err)
	}

	decisionID, err := s.SaveDecision(decision)
	if err != nil {
		t.Fatal(err)
	}

	accepted, receipt, err := reg.Accept(
		approved,
		decisionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveAccepted(receipt); err != nil {
		t.Fatal(err)
	}

	state, err := livefield.Rebuild(mustEvents(t, s))
	if err != nil {
		t.Fatal(err)
	}

	final, ok := state.Accepted[accepted.IDN]
	if !ok {
		t.Fatalf(
			"accepted APP Rover terminal EmergION %s missing after REG rebuild",
			accepted.IDN,
		)
	}

	if final.REL["transition_emergion"] != request.TransitionEmergIONID {
		t.Fatalf(
			"REG acceptance lost travelling EmergION identity: got %q want %q",
			final.REL["transition_emergion"],
			request.TransitionEmergIONID,
		)
	}
}

func TestExecutionCoverageFailureDerivesOnlyRecognizedRequiredCapability(t *testing.T) {
	tests := []struct {
		name      string
		execError string
		want      string
	}{
		{
			name:      "recognized capability bridgegap",
			execError: "COVERAGE failed: BRIDGEGAP:capabilities",
			want:      "DERIVE_CAPABILITY",
		},
		{
			name:      "ordinary execution failure",
			execError: "model process unavailable",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := adapters.ExecutionRequest{
				EmergIONID:      "E-PARENT",
				SourceHash:      "SOURCE-HASH-PROOF",
				AuthorizationID: "EV-Q-PROOF",
				Authority:       "HUMAN_FINAL",
				Adapter:         "LOCAL_GEMMA",
				Action:          "ANALYZE",
			}
			request.TransitionEmergIONID =
				adapters.ExecutionTransitionID(request)

			result := adapters.BindExecutionResult(
				request,
				adapters.ExecutionResult{
					Succeeded: false,
					Error:     tt.execError,
				},
			)

			root := t.TempDir()

			s, err := store.Open(filepath.Join(root, "state"))
			if err != nil {
				t.Fatal(err)
			}

			rt := Runtime{Store: s}

			signal, duplicate, err := rt.CaptureGovernedExecutionResult(
				context.Background(),
				request,
				result,
			)
			if err != nil {
				t.Fatal(err)
			}
			if duplicate {
				t.Fatal("execution BRIDGEGAP probe unexpectedly duplicate")
			}

			got := signal.REL["required_capability"]
			if got != tt.want {
				t.Fatalf(
					"execution error %q required_capability = %q want %q",
					tt.execError,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestExecutionCoverageFailureResolvesThroughAcceptedCapabilityProviders(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	providers := []core.EmergION{
		{
			IDN: "E-ANALYZE",
			STA: core.StateAtGOV,
			CAP: []string{"ANALYZE"},
		},
		{
			IDN: "E-CMP",
			STA: core.StateAtGOV,
			CAP: []string{"CMP"},
		},
		{
			IDN: "E-RLT",
			STA: core.StateAtGOV,
			CAP: []string{"RLT"},
		},
	}

	for _, provider := range providers {
		provider.MEM.SourceHash = store.Hash([]byte(provider.IDN))
		provider.MEM.Codec = "test"
		provider.MEM.Bytes = int64(len(provider.IDN))
		provider.MEM.Stored = int64(len(provider.IDN))
		provider.MEM.Summary = "accepted capability provider"
		provider.REL = map[string]string{
			"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED",
		}
		provider.VAL = core.Validation{
			Facts:  []string{"provider proof"},
			Risk:   "L",
			Recoil: true,
			WVC:    true,
		}
		provider.EVO = core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   time.Now().UTC(),
				AIIntegrated: false,
				PromptSchema: "MXPD/2",
			},
		}

		if _, err := s.Preserve([]byte(provider.IDN)); err != nil {
			t.Fatal(err)
		}

		if _, err := s.SaveCandidate(provider); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			provider,
			gov.Approve,
			"HUMAN_FINAL",
			"accept capability provider",
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

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID =
		adapters.ExecutionTransitionID(request)

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: false,
			Error:     "COVERAGE failed: BRIDGEGAP:capabilities",
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("execution capability-resolution signal unexpectedly duplicate")
	}

	if signal.REL["required_capability"] != "DERIVE_CAPABILITY" {
		t.Fatalf(
			"required_capability = %q want DERIVE_CAPABILITY",
			signal.REL["required_capability"],
		)
	}

	if signal.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf(
			"capability_resolution = %q want COMPOSABLE_CANDIDATE",
			signal.REL["capability_resolution"],
		)
	}

	if signal.REL["capability_composition"] != "ANALYZE+CMP+RLT" {
		t.Fatalf(
			"capability_composition = %q want ANALYZE+CMP+RLT",
			signal.REL["capability_composition"],
		)
	}

	wantProviders := "ANALYZE:E-ANALYZE,CMP:E-CMP,RLT:E-RLT"
	if signal.REL["capability_providers"] != wantProviders {
		t.Fatalf(
			"capability_providers = %q want %q",
			signal.REL["capability_providers"],
			wantProviders,
		)
	}
}

func TestExecutionCoverageFailureProducesCapabilityProviderEdgeProposal(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	providers := []core.EmergION{
		{
			IDN: "E-ANALYZE",
			STA: core.StateAtGOV,
			CAP: []string{"ANALYZE"},
		},
		{
			IDN: "E-CMP",
			STA: core.StateAtGOV,
			CAP: []string{"CMP"},
		},
		{
			IDN: "E-RLT",
			STA: core.StateAtGOV,
			CAP: []string{"RLT"},
		},
	}

	for _, provider := range providers {
		provider.MEM.SourceHash = store.Hash([]byte(provider.IDN))
		provider.MEM.Codec = "test"
		provider.MEM.Bytes = int64(len(provider.IDN))
		provider.MEM.Stored = int64(len(provider.IDN))
		provider.MEM.Summary = "accepted capability provider"
		provider.REL = map[string]string{
			"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED",
		}
		provider.VAL = core.Validation{
			Facts:  []string{"provider proof"},
			Risk:   "L",
			Recoil: true,
			WVC:    true,
		}
		provider.EVO = core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   time.Now().UTC(),
				AIIntegrated: false,
				PromptSchema: "MXPD/2",
			},
		}

		if _, err := s.Preserve([]byte(provider.IDN)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveCandidate(provider); err != nil {
			t.Fatal(err)
		}

		approved, decision, err := gov.Decide(
			provider,
			gov.Approve,
			"HUMAN_FINAL",
			"accept capability provider",
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

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-PARENT",
		SourceHash:      "SOURCE-HASH-PROOF",
		AuthorizationID: "EV-Q-PROOF",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID =
		adapters.ExecutionTransitionID(request)

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: false,
			Error:     "COVERAGE failed: BRIDGEGAP:capabilities",
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("provider-edge execution signal unexpectedly duplicate")
	}

	want := "E-ANALYZE->E-CMP,E-CMP->E-RLT"

	if signal.REL["capability_provider_edge_proposal"] != want {
		t.Fatalf(
			"capability_provider_edge_proposal = %q want %q",
			signal.REL["capability_provider_edge_proposal"],
			want,
		)
	}
}
func TestGovernedRecursiveEmergenceTinyLoop(t *testing.T) {
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

	providers := []core.EmergION{
		{IDN: "E-PROVIDER-ANALYZE", STA: core.StateAtGOV, CAP: []string{"ANALYZE"}},
		{IDN: "E-PROVIDER-CMP", STA: core.StateAtGOV, CAP: []string{"CMP"}},
		{IDN: "E-PROVIDER-RLT", STA: core.StateAtGOV, CAP: []string{"RLT"}},
	}

	for _, provider := range providers {
		provider.MEM.SourceHash = store.Hash([]byte(provider.IDN))
		provider.MEM.Codec = "test"
		provider.MEM.Bytes = int64(len(provider.IDN))
		provider.MEM.Stored = int64(len(provider.IDN))
		provider.MEM.Summary = "recursive emergence capability provider"
		provider.REL = map[string]string{
			"protector": "NO_EXTERNAL_AUTHORITY_CLAIMED",
		}
		provider.VAL = core.Validation{
			Facts:  []string{"provider proof"},
			Risk:   "L",
			Recoil: true,
			WVC:    true,
		}
		provider.EVO = core.Evolution{
			Version: 1,
			Metadata: &core.Metadata{
				Topology:     core.TopologyDodecahedronV1,
				CapturedAt:   time.Now().UTC(),
				AIIntegrated: false,
				PromptSchema: "MXPD/2",
			},
		}

		if _, err := s.Preserve([]byte(provider.IDN)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveCandidate(provider); err != nil {
			t.Fatal(err)
		}

		approvedProvider, providerDecision, err := gov.Decide(
			provider,
			gov.Approve,
			"HUMAN_FINAL",
			"approve recursive emergence provider",
		)
		if err != nil {
			t.Fatal(err)
		}

		providerDecisionID, err := s.SaveDecision(providerDecision)
		if err != nil {
			t.Fatal(err)
		}

		_, providerReceipt, err := reg.Accept(
			approvedProvider,
			providerDecisionID,
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := s.SaveAccepted(providerReceipt); err != nil {
			t.Fatal(err)
		}
	}

	requestN := adapters.ExecutionRequest{
		EmergIONID:      target.IDN,
		SourceHash:      target.MEM.SourceHash,
		AuthorizationID: "EV-Q-TINY-N",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	requestN.TransitionEmergIONID = adapters.ExecutionTransitionID(requestN)

	if requestN.TransitionEmergIONID == "" {
		t.Fatal("T_n travelling EmergION identity missing")
	}

	resultN := adapters.BindExecutionResult(
		requestN,
		adapters.ExecutionResult{
			Succeeded: false,
			Error:     "COVERAGE failed: BRIDGEGAP:capabilities",
		},
	)

	proposalRuntime := Runtime{Store: s}

	proposal, duplicate, err := proposalRuntime.CaptureGovernedExecutionResult(
		context.Background(),
		requestN,
		resultN,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("recursive emergence proposal unexpectedly duplicate")
	}

	if proposal.REL["transition_emergion"] != requestN.TransitionEmergIONID {
		t.Fatal("execution-derived proposal lost T_n identity")
	}
	if proposal.REL["required_capability"] != "DERIVE_CAPABILITY" {
		t.Fatalf("required_capability = %q want DERIVE_CAPABILITY", proposal.REL["required_capability"])
	}
	if proposal.REL["capability_resolution"] != "COMPOSABLE_CANDIDATE" {
		t.Fatalf("capability_resolution = %q want COMPOSABLE_CANDIDATE", proposal.REL["capability_resolution"])
	}

	wantProposal := "E-PROVIDER-ANALYZE->E-PROVIDER-CMP,E-PROVIDER-CMP->E-PROVIDER-RLT"
	if proposal.REL["capability_provider_edge_proposal"] != wantProposal {
		t.Fatalf("provider-edge proposal = %q want %q", proposal.REL["capability_provider_edge_proposal"], wantProposal)
	}

	if _, exists := proposal.REL["COMPOSITION_KIN"]; exists {
		t.Fatal("execution-derived proposal self-authorized COMPOSITION_KIN")
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

	if request.TransitionEmergIONID == "" {
		t.Fatal("T_n+1 travelling EmergION identity missing")
	}

	if request.TransitionEmergIONID == requestN.TransitionEmergIONID {
		t.Fatalf(
			"T_n+1 == T_n: %q",
			request.TransitionEmergIONID,
		)
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

func TestAcceptedSuccessfulExecutionMaterializesVerifiedDeliverable(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	request := adapters.ExecutionRequest{
		EmergIONID:      "E-DELIVERABLE-SOURCE",
		SourceHash:      "SOURCE-DELIVERABLE-PROOF",
		AuthorizationID: "EV-Q-DELIVERABLE",
		Authority:       "HUMAN_FINAL",
		Adapter:         "LOCAL_GEMMA",
		Action:          "ANALYZE",
	}
	request.TransitionEmergIONID = adapters.ExecutionTransitionID(request)

	const output = "deliverable line one\nline=two:preserved\nline three"

	result := adapters.BindExecutionResult(
		request,
		adapters.ExecutionResult{
			Succeeded: true,
			Output:    output,
		},
	)

	rt := Runtime{Store: s}

	signal, duplicate, err := rt.CaptureGovernedExecutionResult(
		context.Background(),
		request,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("deliverable execution signal unexpectedly duplicate")
	}

	approved, decision, err := gov.Decide(
		signal,
		gov.Approve,
		"HUMAN_FINAL",
		"accept deliverable proof",
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

	got, err := rt.MaterializeAcceptedExecutionResult(accepted.IDN)
	if err != nil {
		t.Fatal(err)
	}

	if got.Output != output {
		t.Fatalf("deliverable output = %q want %q", got.Output, output)
	}
	if got.EvidenceHash != accepted.MEM.SourceHash {
		t.Fatal("deliverable lost immutable evidence identity")
	}
	if got.SourceEmergIONID != request.EmergIONID {
		t.Fatal("deliverable lost source EmergION lineage")
	}
	if got.TransitionEmergIONID != request.TransitionEmergIONID {
		t.Fatal("deliverable lost travelling transition identity")
	}
	if got.AuthorizationID != request.AuthorizationID {
		t.Fatal("deliverable lost authorization identity")
	}
	if got.FieldTip == "" {
		t.Fatal("deliverable missing FIELD tip")
	}
}

func TestHistoricalExecutionSignalV1MaterializesVerifiedDeliverable(t *testing.T) {
	root := t.TempDir()

	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}

	const (
		sourceID   = "E-HISTORICAL-SOURCE"
		sourceHash = "SOURCE-HISTORICAL-PROOF"
		output     = `{"summary":"historical verified output"}`
	)

	evidence := []byte(`{"schema":"EXECUTION_SIGNAL_V1","source_kind":"EXECUTION_RESULT","parent_emergion":"E-HISTORICAL-SOURCE","source_hash":"SOURCE-HISTORICAL-PROOF","authority":"CAP_ONLY","adapter":"LOCAL_GEMMA","action":"ANALYZE","succeeded":true,"output":"{\"summary\":\"historical verified output\"}"}`)

	rt := Runtime{Store: s}
	signalRuntime := rt
	signalRuntime.Reasoner = fixedReasoner{
		name:    "execution-signal",
		version: "v1",
		result: reason.Result{
			Summary: "bounded execution result observation",
			Relationships: map[string]string{
				"source_kind":     "EXECUTION_RESULT",
				"parent_emergion": sourceID,
				"adapter":         "LOCAL_GEMMA",
				"action":          "ANALYZE",
			},
			Capabilities: []string{"OBS", "CMP"},
			Facts:        []string{"execution_result_observed", "execution_succeeded"},
			Risk:         "L",
		},
	}

	signal, duplicate, err := signalRuntime.captureBytes(
		context.Background(),
		"execution-result",
		evidence,
		"execution_signal",
	)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("historical execution signal unexpectedly duplicate")
	}

	approved, decision, err := gov.Decide(
		signal,
		gov.Approve,
		"HUMAN_FINAL",
		"accept historical deliverable proof",
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

	got, err := rt.MaterializeAcceptedExecutionResult(accepted.IDN)
	if err != nil {
		t.Fatal(err)
	}

	if got.Output != output {
		t.Fatalf("deliverable output = %q want %q", got.Output, output)
	}
	if got.EvidenceHash != accepted.MEM.SourceHash {
		t.Fatal("deliverable lost immutable historical evidence identity")
	}
	if got.SourceEmergIONID != sourceID {
		t.Fatal("deliverable lost historical source EmergION lineage")
	}
	if got.SourceHash != sourceHash {
		t.Fatal("deliverable lost historical source hash")
	}
	if got.AuthorizationID != "" {
		t.Fatalf("historical authorization fabricated: %q", got.AuthorizationID)
	}
	if got.TransitionEmergIONID != "" {
		t.Fatalf("historical transition fabricated: %q", got.TransitionEmergIONID)
	}
	if got.Authority != "CAP_ONLY" ||
		got.Adapter != "LOCAL_GEMMA" ||
		got.Action != "ANALYZE" {
		t.Fatal("historical execution identity changed")
	}
	if got.FieldTip == "" {
		t.Fatal("deliverable missing FIELD tip")
	}
}
