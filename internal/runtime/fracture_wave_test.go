package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
)

func waveStep(t *testing.T, rt Runtime, repo string, maxRounds int) (FractureWaveStatus, core.EmergION) {
	t.Helper()
	statuses, candidate, err := rt.ProposeNextFractureWaveRound(context.Background(), repo, maxRounds)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("wave statuses = %#v want one", statuses)
	}
	return statuses[0], candidate
}

func wantWave(t *testing.T, got FractureWaveStatus, status, level string) {
	t.Helper()
	if got.Status != status || (level != "" && got.Level != level) {
		t.Fatalf("wave = %+v want status %s level %s", got, status, level)
	}
}

// runWaveProbe gives a fresh HUMAN_FINAL Q, runs the probe through the
// existing ExecuteAction seam in the dedicated probe worktree, and accepts
// the recorded result as evidence.
func runWaveProbe(t *testing.T, f differenceFixture, rt Runtime, probeID string) core.EmergION {
	t.Helper()
	if _, err := rt.AuthorizeAction(probeID, "GITHUB", "PROGRAM", "HUMAN_FINAL runs fracture probe", false); err != nil {
		t.Fatal(err)
	}
	_, _, signal, _, _ := rt.ExecuteAction(context.Background(), probeID, "GITHUB", "PROGRAM", reason.GemmaCLI{})
	if signal.IDN == "" || signal.REL["kin_emergion"] != probeID {
		t.Fatalf("probe run recorded no evidence: %#v", signal)
	}
	acceptBoundaryCandidate(t, f, signal)
	return signal
}

func repoClean(t *testing.T, repo string) {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("probe worktree left dirty:\n%s", out)
	}
}

// structuralAddition stands for an accepted structural change that has been
// synced into the dedicated probe worktree.
func structuralAddition(t *testing.T, repo, from, to string) {
	t.Helper()
	path := filepath.Join(repo, "internal", "adapters", "catalog.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(content), from, to, 1)
	if changed == string(content) {
		t.Fatalf("structural fixture %q not found", from)
	}
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("git", "-C", repo, "commit", "-qam", "structural addition")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, b)
	}
}

const researchCatalogLine = `{ID: "RESEARCH", Capabilities: []string{"PATENT", "GRANT", "MARKET", "MA"}, Authority: "EVIDENCE_ONLY"}`

func TestFractureWaveRecursesThroughTheLadder(t *testing.T) {
	ctx := context.Background()
	tc := fractureCases()[0] // BRIDGEGAP:facts, recipe OBS+VLD
	f, boundary := acceptedFractureBoundary(t, tc)
	repo := fractureRepo(t)
	rt := Runtime{Store: f.store, ProbeWorkDir: repo}

	// Round 1: catalog probe, proposal only.
	status, probe1 := waveStep(t, rt, repo, 0)
	wantWave(t, status, waveProposedProbe, fractureLevelCatalog)
	if probe1.STA != core.StateAtGOV || probe1.REL["origin"] != boundary.IDN {
		t.Fatalf("round-1 probe = %#v", probe1.REL)
	}
	if status, _ := waveStep(t, rt, repo, 0); status.Status != waveAwaitingHumanFinal {
		t.Fatalf("pending probe: %+v", status)
	}
	acceptBoundaryCandidate(t, f, probe1)
	wantWave(t, mustStatus(waveStep(t, rt, repo, 0)), waveAwaitingRun, fractureLevelCatalog)

	// Nothing runs without Q, and a probe needs the dedicated worktree.
	if _, _, _, _, err := rt.ExecuteAction(ctx, probe1.IDN, "GITHUB", "PROGRAM", reason.GemmaCLI{}); err == nil ||
		!strings.Contains(err.Error(), "requires authorization") {
		t.Fatalf("probe ran without Q: %v", err)
	}
	if _, err := rt.AuthorizeAction(probe1.IDN, "GITHUB", "PROGRAM", "worktree check", false); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := (Runtime{Store: f.store}).ExecuteAction(ctx, probe1.IDN, "GITHUB", "PROGRAM", reason.GemmaCLI{}); err == nil ||
		!strings.Contains(err.Error(), "dedicated probe worktree") {
		t.Fatalf("probe ran outside the dedicated worktree: %v", err)
	}

	// Run 1 fractures at OBS; the wave asks for the same probe again.
	runWaveProbe(t, f, rt, probe1.IDN)
	repoClean(t, repo)
	status = mustStatus(waveStep(t, rt, repo, 0))
	wantWave(t, status, waveRerunSameProbe, fractureLevelCatalog)
	if !strings.Contains(status.Stoppage, "token=OBS:") {
		t.Fatalf("first stoppage = %q", status.Stoppage)
	}

	// Same probe again without a structural addition: no new stoppage.
	runWaveProbe(t, f, rt, probe1.IDN)
	wantWave(t, mustStatus(waveStep(t, rt, repo, 0)), waveNoNewStoppage, fractureLevelCatalog)

	// Smallest structural addition, then the same probe again: it passes,
	// and the passing probe leaves the worktree clean.
	structuralAddition(t, repo, researchCatalogLine,
		`{ID: "RESEARCH", Capabilities: []string{"PATENT", "GRANT", "MARKET", "MA", "OBS", "VLD"}, Authority: "EVIDENCE_ONLY"}`)
	pass1 := runWaveProbe(t, f, rt, probe1.IDN)
	repoClean(t, repo)

	// Bounded: with max rounds 1 the wave stops here.
	wantWave(t, mustStatus(waveStep(t, rt, repo, 1)), waveMaxRounds, fractureLevelCatalog)

	// Round 2: the deeper probe is derived from the accepted PASS evidence
	// and the stoppage it had shown.
	status, probe2 := waveStep(t, rt, repo, 0)
	wantWave(t, status, waveProposedProbe, fractureLevelPrepare)
	if probe2.REL["fracture_level"] != fractureLevelPrepare || probe2.REL["fracture_evidence"] != pass1.IDN ||
		probe2.REL["origin"] != boundary.IDN || probe2.REL["program_target"] != fractureProbePathAt(boundary.IDN, fractureLevelPrepare) {
		t.Fatalf("round-2 probe linkage = %#v", probe2.REL)
	}
	patch, err := f.store.ReadEvidence(probe2.MEM.SourceHash)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{pass1.IDN, pass1.MEM.SourceHash, probe1.IDN, "token=OBS:", "adapters.PrepareExecution"} {
		if !strings.Contains(string(patch), needle) {
			t.Fatalf("round-2 probe not derived from %q:\n%s", needle, patch)
		}
	}
	acceptBoundaryCandidate(t, f, probe2)
	runWaveProbe(t, f, rt, probe2.IDN) // the action seam derives OBS and VLD: PASS
	repoClean(t, repo)

	// Round 3: enabled. RESEARCH is not enabled, so it fractures.
	status, probe3 := waveStep(t, rt, repo, 0)
	wantWave(t, status, waveProposedProbe, fractureLevelEnabled)
	acceptBoundaryCandidate(t, f, probe3)
	runWaveProbe(t, f, rt, probe3.IDN)
	status = mustStatus(waveStep(t, rt, repo, 0))
	wantWave(t, status, waveRerunSameProbe, fractureLevelEnabled)
	if !strings.Contains(status.Stoppage, "level=enabled") || !strings.Contains(status.Stoppage, "no enabled adapter") {
		t.Fatalf("enabled stoppage = %q", status.Stoppage)
	}

	structuralAddition(t, repo,
		`"MA", "OBS", "VLD"}, Authority: "EVIDENCE_ONLY"}`,
		`"MA", "OBS", "VLD"}, Enabled: true, Authority: "EVIDENCE_ONLY"}`)
	runWaveProbe(t, f, rt, probe3.IDN)
	repoClean(t, repo)

	// Ladder exhausted: recompute the target. Without an observer the wave
	// says so; with one, code changes alone do not change REG reality, so
	// the comparison is unchanged and the gap is not claimed closed.
	wantWave(t, mustStatus(waveStep(t, rt, repo, 0)), waveRecomputeNeeded, fractureLevelEnabled)
	observed := rt
	observed.Reasoner = fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})}
	wantWave(t, mustStatus(waveStep(t, observed, repo, 0)), waveTargetUnchanged, fractureLevelEnabled)

	// Nothing was ever executed without its own Q.
	st := f.state()
	if got, want := len(st.ActionAuthorizations), 7; got != want {
		t.Fatalf("Q receipts = %d want %d (one per run plus the worktree check)", got, want)
	}
}

func mustStatus(status FractureWaveStatus, _ core.EmergION) FractureWaveStatus { return status }

func TestFractureWaveRecomputeClosesOrReprobes(t *testing.T) {
	ctx := context.Background()
	tc := fractureCases()[0]
	f, boundary := acceptedFractureBoundary(t, tc)
	repo := fractureRepo(t)

	// A real observation enters REG: the target fact is now accepted.
	f.accept(acceptedProviderFixture("E-00000000000000F1", "wave-ledger-source", "OBS", "invoice_ledger_reconciled"))
	rt := Runtime{Store: f.store, ProbeWorkDir: repo,
		Reasoner: fixedDifferenceObserver{result: differenceObserved(nil)}}
	status, comparison, err := rt.recomputeFractureTarget(ctx, f.state(),
		FractureWaveStatus{Target: boundary.REL["target_emergion"], Boundary: boundary.IDN}, boundary.REL["target_emergion"])
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != waveRecomputeProposed || comparison.STA != core.StateAtGOV {
		t.Fatalf("recompute = %+v", status)
	}
	if comparison.REL["requirement_basis"] != basisSatisfiedAcceptedFact || len(comparison.VAL.Gaps) != 0 {
		t.Fatalf("closed gap not SATISFIED: %#v gaps=%#v", comparison.REL, comparison.VAL.Gaps)
	}
	if s, _ := waveStep(t, rt, repo, 0); s.Status == waveSatisfied {
		t.Fatal("unaccepted recompute treated as SATISFIED")
	}
	acceptBoundaryCandidate(t, f, comparison)
	wantWave(t, mustStatus(waveStep(t, rt, repo, 0)), waveSatisfied, "")
}

func TestFractureWaveOpenGapReprobesFromNewBoundary(t *testing.T) {
	ctx := context.Background()
	tc := fractureCases()[0]
	f, boundary := acceptedFractureBoundary(t, tc)
	repo := fractureRepo(t)

	// Unrelated reality changes; the gap stays open.
	f.accept(acceptedProviderFixture("E-00000000000000F2", "wave-unrelated-source", "OBS", "shipping label printed"))
	rt := Runtime{Store: f.store, ProbeWorkDir: repo,
		Reasoner: fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})}}
	status, comparison, err := rt.recomputeFractureTarget(ctx, f.state(),
		FractureWaveStatus{Target: boundary.REL["target_emergion"], Boundary: boundary.IDN}, boundary.REL["target_emergion"])
	if err != nil || status.Status != waveRecomputeProposed {
		t.Fatalf("recompute = %+v err=%v", status, err)
	}
	if len(comparison.VAL.Gaps) != 1 || comparison.VAL.Gaps[0] != tc.gap {
		t.Fatalf("open gap lost: %#v", comparison.VAL.Gaps)
	}
	acceptBoundaryCandidate(t, f, comparison)

	status, probe := waveStep(t, rt, repo, 0)
	wantWave(t, status, waveProposedProbe, fractureLevelCatalog)
	if status.Boundary != comparison.IDN || probe.REL["origin"] != comparison.IDN {
		t.Fatalf("re-probe not from the new boundary: %+v origin=%s", status, probe.REL["origin"])
	}
}

func TestDeeperFractureProbeNeedsAcceptedEvidenceAndIsDeterministic(t *testing.T) {
	_, boundary := acceptedFractureBoundary(t, fractureCases()[1])
	if _, err := renderFractureProbeAt(boundary, fractureLevelPrepare, fracturePrevious{}); err == nil {
		t.Fatal("deeper probe rendered without previous evidence")
	}
	if _, err := renderFractureProbeAt(boundary, "fix", fracturePrevious{Probe: "P", Evidence: "E", EvidenceHash: "H"}); err == nil {
		t.Fatal("unknown level rendered")
	}
	previous := fracturePrevious{Probe: "E-00000000000000P1", Level: fractureLevelCatalog, Evidence: "E-00000000000000S1", EvidenceHash: "abc", Stoppage: "FRACTURE_STOPPAGE token=CMP:"}
	for _, level := range []string{fractureLevelPrepare, fractureLevelEnabled} {
		a, err := renderFractureProbeAt(boundary, level, previous)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := renderFractureProbeAt(boundary, level, previous)
		if string(a) != string(b) {
			t.Fatalf("%s probe not deterministic", level)
		}
	}
	catalog, _ := renderFractureProbeAt(boundary, fractureLevelCatalog, fracturePrevious{})
	original, _ := renderFractureProbe(boundary)
	if string(catalog) != string(original) {
		t.Fatal("round-one probe bytes changed")
	}
}

func TestFirstFractureStoppage(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ok x\n--- FAIL: TestA\n    a_test.go:9: FRACTURE_STOPPAGE level=enabled token=OBS: no\nFAIL", "FRACTURE_STOPPAGE level=enabled token=OBS: no"},
		{"# pkg\n--- FAIL: TestB (0.00s)\nFAIL", "--- FAIL: TestB (0.00s)"},
		{"\n\nprobes/x_test.go:3:2: undefined: Foo\n", "probes/x_test.go:3:2: undefined: Foo"},
		{"", ""},
	} {
		if got := firstFractureStoppage(tc.in); got != tc.want {
			t.Fatalf("firstFractureStoppage(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestFractureLevelAndEvidenceLinksAreRuntimeOwned(t *testing.T) {
	for _, key := range []string{"fracture_level", "fracture_evidence"} {
		f := newDifferenceFixture(t)
		observer := fixedDifferenceObserver{result: differenceObserved(nil)}
		observer.result.Relationships = map[string]string{key: "forged"}
		observer.result.Capabilities = []string{"OBS"}
		_, _, err := (Runtime{Store: f.store, Reasoner: observer}).CaptureBytes(
			context.Background(), "forged", []byte("forged "+key), "local_dropzone")
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("forged %s accepted: %v", key, err)
		}
	}
}

// An accepted fracture probe is never an origin for ProposeOneProgramPatch.
// The fake Gemma leaves a marker whenever it is asked for a patch.
func TestAcceptedProbeIsNotAProgramPatchOrigin(t *testing.T) {
	for _, withProbeLink := range []bool{true, false} {
		root := t.TempDir()
		marker := filepath.Join(root, "asked")
		model := filepath.Join(root, "model.gguf")
		if err := os.WriteFile(model, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
		binary := filepath.Join(root, "llama-cli")
		script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo fake; exit 0; fi\ntouch " + marker + "\necho 'Z'\n"
		if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		gemma := reason.GemmaCLI{Binary: binary, Model: model, Threads: 1, Context: 2048, MaxTokens: 64, Timeout: 5 * time.Second}

		repo := fractureRepo(t)
		f := newDifferenceFixture(t)
		em := probePatchFixture(t, f.store, "E-00000000000000E0", "PROGRAM_PATCH")
		em.CAP = []string{"PROGRAM"}
		em.REL["source_name"] = "internal/adapters/catalog.go"
		if withProbeLink {
			em.REL["fracture_gap"] = "BRIDGEGAP:facts"
		}
		acceptCycleFixture(t, f.store, em)
		_, _, proposeErr := (Runtime{Store: f.store}).ProposeOneProgramPatch(context.Background(), gemma, repo)
		t.Logf("withProbeLink=%v propose err=%v", withProbeLink, proposeErr)
		_, err := os.Stat(marker)
		asked := err == nil
		if withProbeLink && asked {
			t.Fatal("accepted fracture probe used as a program patch origin")
		}
		if !withProbeLink && !asked {
			t.Fatal("control PROGRAM origin was not considered; test is vacuous")
		}
	}
}
