package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
)

type fractureCase struct {
	name     string
	reality  func(differenceFixture)
	target   string
	gap      string
	required string
	stoppage string // first recipe token the current structure cannot express
}

func fractureCases() []fractureCase {
	return []fractureCase{
		{
			name: "facts",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A0", "fracture-obs-source", "OBS"))
				f.accept(acceptedProviderFixture("E-00000000000000A2", "fracture-vld-source", "VLD"))
			},
			target:   "invoice ledger reconciled",
			gap:      "BRIDGEGAP:facts",
			required: "ESTABLISH_FACT",
			stoppage: "OBS",
		},
		{
			name: "relationships",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A1", "fracture-a1-source", "OBS"))
				f.accept(acceptedProviderFixture("E-00000000000000B2", "fracture-b2-source", "VLD"))
			},
			target:   "E-00000000000000A1 composes with E-00000000000000B2",
			gap:      "BRIDGEGAP:relationships",
			required: "DERIVE_RELATIONSHIP",
			stoppage: "CMP",
		},
		{
			name: "capabilities",
			reality: func(f differenceFixture) {
				f.accept(acceptedProviderFixture("E-00000000000000A0", "fracture-cap-source", "OBS"))
			},
			target:   "PROGRAM capability is available",
			gap:      "BRIDGEGAP:capabilities",
			required: "DERIVE_CAPABILITY",
			stoppage: "CMP",
		},
	}
}

func acceptedFractureBoundary(t *testing.T, tc fractureCase) (differenceFixture, core.EmergION) {
	t.Helper()
	f := newDifferenceFixture(t)
	tc.reality(f)
	boundary := captureDifferenceComparison(t, f,
		fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})},
		tc.target)
	if len(boundary.VAL.Gaps) != 1 || boundary.VAL.Gaps[0] != tc.gap {
		t.Fatalf("boundary gaps = %#v want [%q]", boundary.VAL.Gaps, tc.gap)
	}
	acceptBoundaryCandidate(t, f, boundary)
	accepted, ok := f.state().Accepted[boundary.IDN]
	if !ok {
		t.Fatal("boundary not REG-accepted")
	}
	return f, accepted
}

// fractureRepo builds a clean git repository holding this module's current
// non-test sources for every package the generated probe compiles against.
// The executor's go test ./... therefore runs only the probe, against the
// real current code, and never re-enters this test.
func fractureRepo(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps",
		"-f", "{{if not .Standard}}{{.ImportPath}}|{{.Dir}}|{{join .GoFiles \",\"}}{{end}}",
		"emergion-sovereign-runtime/internal/adapters")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module emergion-sovereign-runtime\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		rel := strings.TrimPrefix(parts[0], "emergion-sovereign-runtime/")
		if err := os.MkdirAll(filepath.Join(root, rel), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range strings.Split(parts[2], ",") {
			content, err := os.ReadFile(filepath.Join(parts[1], name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, rel, name), content, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "fracture@example.invalid"},
		{"config", "user.name", "Fracture Probe"},
		{"add", "-A"},
		{"commit", "-qm", "current structure"},
	} {
		c := exec.Command("git", args...)
		c.Dir = root
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	return root
}

func TestFractureProbeGenerationIsDeterministicAndBoundaryDerived(t *testing.T) {
	for _, tc := range fractureCases() {
		t.Run(tc.name, func(t *testing.T) {
			f, boundary := acceptedFractureBoundary(t, tc)

			first, err := renderFractureProbe(boundary)
			if err != nil {
				t.Fatal(err)
			}
			second, err := renderFractureProbe(boundary)
			if err != nil {
				t.Fatal(err)
			}
			if string(first) != string(second) {
				t.Fatal("fracture probe rendering is not deterministic")
			}
			path := fractureProbePath(boundary.IDN)
			want := "probes/fracture_" + strings.ReplaceAll(strings.ToLower(boundary.IDN), "-", "_") + "_test.go"
			if path != want {
				t.Fatalf("probe path = %q want %q", path, want)
			}

			content := string(first)
			for _, needle := range append([]string{
				boundary.IDN,
				boundary.REL["target_emergion"],
				boundary.REL["requirement_basis"],
				tc.required,
				boundary.REL["target_state"],
				boundary.MEM.SourceHash,
				"gap := \"" + strings.TrimPrefix(tc.gap, "BRIDGEGAP:") + "\"",
			}, requiredCapabilityRecipe(tc.required)...) {
				if !strings.Contains(content, needle) {
					t.Fatalf("probe is not derived from boundary field %q:\n%s", needle, content)
				}
			}
			// No fix and no edit of an existing file: one new test file only.
			patch := string(fractureProbePatch(path, first))
			removes := strings.Count(patch, "\n-") - strings.Count(patch, "\n--- /dev/null")
			if strings.Count(patch, "diff --git ") != 1 || !strings.Contains(patch, "--- /dev/null") || removes != 0 {
				t.Fatalf("probe patch is not a single new file:\n%s", patch)
			}

			// Records that are not genuine-remainder BRIDGEGAPs yield no probe.
			provider := f.state().Accepted["E-00000000000000A0"]
			if provider.IDN == "" {
				provider = f.state().Accepted["E-00000000000000A1"]
			}
			if _, err := renderFractureProbe(provider); err == nil {
				t.Fatal("probe rendered from a non-boundary record")
			}
			tampered := boundary
			tampered.REL = map[string]string{}
			for k, v := range boundary.REL {
				tampered.REL[k] = v
			}
			tampered.REL["requirement_basis"] = basisNoRule
			if _, err := renderFractureProbe(tampered); err == nil {
				t.Fatal("probe rendered from an UNRESOLVED basis")
			}
		})
	}
}

func TestSatisfiedComparisonGetsNoFractureProbe(t *testing.T) {
	f := newDifferenceFixture(t)
	f.accept(acceptedProviderFixture("E-00000000000000A0", "fracture-satisfied-source", "OBS", "invoice_ledger_reconciled"))
	comparison := captureDifferenceComparison(t, f, fixedDifferenceObserver{result: differenceObserved(nil)}, "invoice ledger reconciled")
	acceptBoundaryCandidate(t, f, comparison)

	_, proposed, err := (Runtime{Store: f.store}).ProposeOneFractureProbe(context.Background(), fractureRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	if proposed {
		t.Fatal("SATISFIED comparison produced a fracture probe")
	}
}

// End to end for every gap type: propose -> HUMAN_FINAL -> no auto-run ->
// explicit Q -> run -> first truthful stoppage captured as evidence with
// lineage -> evidence cannot close the gap.
func TestFractureProbeFailsAndStoppageIsCapturedAsEvidence(t *testing.T) {
	gemma := torqueFakeGemma(t)
	for _, tc := range fractureCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f, boundary := acceptedFractureBoundary(t, tc)
			repo := fractureRepo(t)
			rt := Runtime{Store: f.store}

			probe, proposed, err := rt.ProposeOneFractureProbe(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			if !proposed {
				t.Fatal("no fracture probe proposed for an accepted BRIDGEGAP")
			}
			if probe.STA != core.StateAtGOV {
				t.Fatalf("probe state = %q want AT_GOV", probe.STA)
			}
			if probe.REL["source_kind"] != "PROGRAM_PATCH" ||
				probe.REL["origin"] != boundary.IDN ||
				probe.REL["fracture_gap"] != tc.gap ||
				probe.REL["program_target"] != fractureProbePath(boundary.IDN) {
				t.Fatalf("probe linkage = %#v", probe.REL)
			}
			if _, again, err := rt.ProposeOneFractureProbe(ctx, repo); err != nil || again {
				t.Fatalf("second proposal for the same boundary: proposed=%v err=%v", again, err)
			}
			if _, err := adapters.PrepareExecution(f.state(), probe.IDN, "GITHUB", "PROGRAM", false); err == nil {
				t.Fatal("probe prepared before HUMAN_FINAL")
			}

			acceptBoundaryCandidate(t, f, probe)

			if _, executed, err := rt.ExecuteOneSafeAction(ctx, gemma); err != nil {
				t.Fatal(err)
			} else if executed {
				t.Fatal("accepted fracture probe auto-ran")
			}
			if _, err := adapters.PrepareExecution(f.state(), probe.IDN, "GITHUB", "PROGRAM", false); err == nil ||
				!strings.Contains(err.Error(), "requires authorization") {
				t.Fatalf("probe prepared without Q: %v", err)
			}

			qID, err := rt.AuthorizeAction(probe.IDN, "GITHUB", "PROGRAM", "HUMAN_FINAL runs fracture probe", false)
			if err != nil {
				t.Fatal(err)
			}
			request, err := adapters.PrepareExecution(f.state(), probe.IDN, "GITHUB", "PROGRAM", false)
			if err != nil {
				t.Fatal(err)
			}
			if request.TransitionEmergIONID == "" || request.SourceHash != probe.MEM.SourceHash {
				t.Fatalf("request = %#v", request)
			}

			result, execErr := (adapters.GitHubProgramExecutor{Store: f.store, WorkDir: repo}).Execute(request)
			if execErr == nil || result.Succeeded {
				t.Fatalf("fracture probe did not fracture: %#v", result)
			}
			for _, needle := range []string{
				"--- FAIL: TestFracture" + strings.ReplaceAll(boundary.IDN, "-", "_"),
				"FRACTURE_STOPPAGE boundary=" + boundary.IDN,
				"token=" + tc.stoppage + ":",
				"patch rolled back",
			} {
				if !strings.Contains(result.Error, needle) {
					t.Fatalf("stoppage missing %q:\n%s", needle, result.Error)
				}
			}
			if strings.Contains(result.Error, "BRIDGEGAP:") {
				t.Fatalf("probe output re-exposes a gap token:\n%s", result.Error)
			}
			if _, err := os.Stat(filepath.Join(repo, fractureProbePath(boundary.IDN))); !os.IsNotExist(err) {
				t.Fatalf("failed probe left in the repository: %v", err)
			}

			result = adapters.BindExecutionResult(request, result)
			signal, _, err := rt.CaptureGovernedExecutionResult(ctx, request, result)
			if err != nil {
				t.Fatal(err)
			}
			if signal.REL["source_kind"] != "EXECUTION_RESULT" ||
				signal.REL["kin_emergion"] != probe.IDN ||
				signal.REL["transition_emergion"] != request.TransitionEmergIONID ||
				signal.REL["authorization_event"] != qID {
				t.Fatalf("stoppage evidence lineage = %#v", signal.REL)
			}
			if _, ok := signal.REL["required_capability"]; ok {
				t.Fatalf("stoppage text was re-read as a requirement: %#v", signal.REL)
			}
			if !reflect.DeepEqual(signal.VAL.Facts, []string{"execution_result_observed", "execution_failed"}) {
				t.Fatalf("stoppage evidence facts = %#v", signal.VAL.Facts)
			}
			raw, err := f.store.ReadEvidence(signal.MEM.SourceHash)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := adapters.ParseExecutionResultBytes(raw)
			if err != nil {
				t.Fatal(err)
			}
			if preserved.EmergIONID != probe.IDN || preserved.SourceHash != probe.MEM.SourceHash ||
				preserved.Succeeded || !strings.Contains(preserved.Error, "FRACTURE_STOPPAGE") {
				t.Fatalf("preserved stoppage evidence = %#v", preserved)
			}

			// Evidence only: once HUMAN_FINAL accepts the stoppage signal, the
			// recomputed requirement is unchanged. Neither the probe's PROGRAM
			// capability nor its signal closes the gap.
			acceptBoundaryCandidate(t, f, signal)
			reality := withoutFractureProbeEvidence(f.reality())
			if _, ok := reality[probe.IDN]; ok {
				t.Fatal("probe counted as accepted reality")
			}
			if _, ok := reality[signal.IDN]; ok {
				t.Fatal("probe signal counted as accepted reality")
			}
			target := core.EmergION{
				IDN: boundary.REL["target_emergion"],
				REL: map[string]string{"source_kind": "TARGET", "target_state": boundary.REL["target_state"]},
			}
			required, basis := deriveDifferenceRequirement(target, reality,
				differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"}))
			if required != tc.required || basis != boundary.REL["requirement_basis"] {
				t.Fatalf("probe evidence moved the requirement: (%q, %q) want (%q, %q)",
					required, basis, tc.required, boundary.REL["requirement_basis"])
			}
		})
	}
}

// A PASS is recorded truthfully: the boundary was already crossable at the
// probed level, and the result says execution_succeeded, nothing more.
func TestFractureProbePassIsRecordedTruthfully(t *testing.T) {
	ctx := context.Background()
	tc := fractureCases()[0]
	f, _ := acceptedFractureBoundary(t, tc)
	repo := fractureRepo(t)

	// Give the copied structure the recipe tokens so this probe can pass.
	catalog := filepath.Join(repo, "internal", "adapters", "catalog.go")
	content, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	crossable := strings.Replace(string(content),
		`{ID: "RESEARCH", Capabilities: []string{"PATENT", "GRANT", "MARKET", "MA"}`,
		`{ID: "RESEARCH", Capabilities: []string{"PATENT", "GRANT", "MARKET", "MA", "OBS", "VLD"}`, 1)
	if crossable == string(content) {
		t.Fatal("catalog fixture line not found")
	}
	if err := os.WriteFile(catalog, []byte(crossable), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"commit", "-qam", "crossable structure"}} {
		c := exec.Command("git", args...)
		c.Dir = repo
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}

	rt := Runtime{Store: f.store}
	probe, proposed, err := rt.ProposeOneFractureProbe(ctx, repo)
	if err != nil || !proposed {
		t.Fatalf("proposal: proposed=%v err=%v", proposed, err)
	}
	acceptBoundaryCandidate(t, f, probe)
	if _, err := rt.AuthorizeAction(probe.IDN, "GITHUB", "PROGRAM", "HUMAN_FINAL runs fracture probe", false); err != nil {
		t.Fatal(err)
	}
	request, err := adapters.PrepareExecution(f.state(), probe.IDN, "GITHUB", "PROGRAM", false)
	if err != nil {
		t.Fatal(err)
	}
	result, execErr := (adapters.GitHubProgramExecutor{Store: f.store, WorkDir: repo}).Execute(request)
	if execErr != nil || !result.Succeeded {
		t.Fatalf("crossable probe did not pass: %v\n%s", execErr, result.Error)
	}
	result = adapters.BindExecutionResult(request, result)
	signal, _, err := rt.CaptureGovernedExecutionResult(ctx, request, result)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(signal.VAL.Facts, []string{"execution_result_observed", "execution_succeeded"}) {
		t.Fatalf("pass evidence facts = %#v", signal.VAL.Facts)
	}
	for _, fact := range signal.VAL.Facts {
		if normalizeRequirementText(fact) == normalizeRequirementText(tc.target) {
			t.Fatal("probe PASS asserted the target fact")
		}
	}
	if signal.REL["kin_emergion"] != probe.IDN || signal.REL["transition_emergion"] != request.TransitionEmergIONID {
		t.Fatalf("pass evidence lineage = %#v", signal.REL)
	}
}

func TestFractureGapLinkIsRuntimeOwned(t *testing.T) {
	f := newDifferenceFixture(t)
	observer := fixedDifferenceObserver{result: differenceObserved(nil)}
	observer.result.Relationships = map[string]string{"fracture_gap": "BRIDGEGAP:facts"}
	observer.result.Capabilities = []string{"OBS"}
	_, _, err := (Runtime{Store: f.store, Reasoner: observer}).CaptureBytes(
		context.Background(), "forged", []byte("forged fracture link"), "local_dropzone")
	if err == nil || !strings.Contains(err.Error(), "fracture_gap") {
		t.Fatalf("forged fracture_gap accepted: %v", err)
	}
}

// CaptureTargetComparison itself drops probe evidence from reality: with an
// accepted probe present, the comparison evidence is byte-identical to the
// boundary's, so the existing comparison is recognised as a duplicate and
// the capability gap is not closed by the probe's PROGRAM capability.
func TestTargetComparisonIgnoresFractureProbeEvidence(t *testing.T) {
	ctx := context.Background()
	tc := fractureCases()[2]
	f, boundary := acceptedFractureBoundary(t, tc)
	rt := Runtime{Store: f.store}
	probe, proposed, err := rt.ProposeOneFractureProbe(ctx, fractureRepo(t))
	if err != nil || !proposed {
		t.Fatalf("proposal: proposed=%v err=%v", proposed, err)
	}
	acceptBoundaryCandidate(t, f, probe)
	if _, ok := f.reality()[probe.IDN]; !ok {
		t.Fatal("fixture: accepted probe missing from raw reality")
	}

	target, _, err := (Runtime{Store: f.store, Reasoner: fixedDifferenceObserver{}}).CaptureTarget(ctx, tc.target)
	if err != nil {
		t.Fatal(err)
	}
	observer := fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})}
	again, duplicate, err := (Runtime{Store: f.store, Reasoner: observer}).CaptureTargetComparison(ctx, target, f.reality(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate || again.IDN != boundary.IDN {
		t.Fatalf("probe changed comparison evidence: duplicate=%v idn=%s want %s", duplicate, again.IDN, boundary.IDN)
	}
}
