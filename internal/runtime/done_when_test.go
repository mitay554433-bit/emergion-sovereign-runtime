package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

// Done-when proofs for the open backlog items TR-048 (named tamper tests),
// Phase 1.2 (20% malformed batch), Phase 2.3 (target recompute) and
// Phase 2.4 (bounded wave). Every test uses temporary state only; none of
// them opens a live .field directory or decides on real state.

func ledgerEventCount(t *testing.T, s *store.Store) int {
	t.Helper()
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

func evidenceObjectPath(t *testing.T, s *store.Store, hash string) string {
	t.Helper()
	var found string
	err := filepath.Walk(s.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), hash) {
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatalf("no evidence object for hash %s under %s", hash, s.Root)
	}
	return found
}

// TestTamperSwappedEvidenceObjectFailsVerify is the named swapped-evidence
// tamper test (TR-048): the stored bytes of one accepted record are
// replaced by another record's valid stored object. Each object is still a
// well-formed gzip stream, so only the hash check can catch it, and verify
// must fail naming the swapped hash.
func TestTamperSwappedEvidenceObjectFailsVerify(t *testing.T) {
	s := capturedAcceptedRegistry(t, []string{"swap-a.txt", "swap-b.txt", "swap-c.txt"})
	st := rebuildState(t, s)
	var accepted []core.EmergION
	for _, em := range st.Accepted {
		accepted = append(accepted, em)
	}
	if len(accepted) < 2 {
		t.Fatal("need two accepted records to swap evidence")
	}
	victim, donor := accepted[0], accepted[1]
	if victim.MEM.SourceHash == donor.MEM.SourceHash {
		t.Fatal("fixture records share a source hash")
	}
	victimPath := evidenceObjectPath(t, s, victim.MEM.SourceHash)
	donorBytes, err := os.ReadFile(evidenceObjectPath(t, s, donor.MEM.SourceHash))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(victimPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(original, donorBytes) {
		t.Fatal("swap would not change the stored bytes")
	}
	if err := os.WriteFile(victimPath, donorBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := fieldVerify(s)
	if err == nil {
		t.Fatalf("verify passed after swapping evidence of %s with %s; evidence_verified=%d", victim.IDN, donor.IDN, n)
	}
	if !strings.Contains(err.Error(), victim.MEM.SourceHash) {
		t.Fatalf("swap caught but the swapped hash %s was not named: %v", victim.MEM.SourceHash, err)
	}
	t.Logf("swapped evidence: IDN %s hash %s; verify error: %v", victim.IDN, victim.MEM.SourceHash, err)

	// Putting the original bytes back restores a clean verify: detection
	// is about the bytes, not a sticky flag.
	if err := os.WriteFile(victimPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fieldVerify(s); err != nil {
		t.Fatalf("verify still failing after restoring original evidence: %v", err)
	}
}

// TestMalformedBatchDoneWhen confirms the Phase 1.2 done-when on a ten-file
// batch with two malformed items of each remaining kind (20% per run):
// every malformed item fails closed, its evidence is kept (the file stays in
// the dropzone, or it waits at GOV with a coverage gap), every valid item is
// admitted unchanged, and nothing half-written is left behind: the ledger
// verifies and no orphan evidence exists.
func TestMalformedBatchDoneWhen(t *testing.T) {
	type malformedKind struct {
		name       string
		content    string
		analyze    func(name string) (reason.Result, error)
		leftInDrop bool // fails before admission: file kept as the report
		atGOVGap   bool // admitted but stopped by coverage at GOV
	}
	kinds := []malformedKind{
		{
			name:    "runtime-owned-lineage-claim",
			content: "claims lineage it may not write\n",
			analyze: func(name string) (reason.Result, error) {
				r := completeResult(name)
				r.Relationships["kin"] = "E-FORGED"
				return r, nil
			},
			leftInDrop: true,
		},
		{
			name:    "runtime-owned-provenance-claim",
			content: "claims provenance it may not write\n",
			analyze: func(name string) (reason.Result, error) {
				r := completeResult(name)
				r.Relationships["provenance"] = "forged"
				return r, nil
			},
			leftInDrop: true,
		},
		{
			name:    "composition-kin-not-accepted",
			content: "composes with a record that is not REG-accepted\n",
			analyze: func(name string) (reason.Result, error) {
				r := completeResult(name)
				r.Relationships["COMPOSITION_KIN"] = "E-NOT-ACCEPTED"
				return r, nil
			},
			leftInDrop: true,
		},
		{
			name:    "supersedes-not-accepted",
			content: "supersedes a record that is not REG-accepted\n",
			analyze: func(name string) (reason.Result, error) {
				r := completeResult(name)
				r.Supersedes = "E-NOT-ACCEPTED"
				return r, nil
			},
			leftInDrop: true,
		},
		{
			name:       "grammar-error",
			content:    "bad grammar\n",
			analyze:    func(string) (reason.Result, error) { return reason.Result{}, fmt.Errorf("MXPD grammar rejected") },
			leftInDrop: true,
		},
		{
			name:       "empty-source",
			content:    "",
			analyze:    func(name string) (reason.Result, error) { return completeResult(name), nil },
			leftInDrop: true,
		},
		{
			name:    "missing-facts",
			content: "candidate with no facts\n",
			analyze: func(name string) (reason.Result, error) {
				return reason.Result{Summary: "no facts", Relationships: map[string]string{"source_name": name}, Capabilities: []string{"OBS"}, Risk: "L"}, nil
			},
			atGOVGap: true,
		},
	}

	for _, kind := range kinds {
		kind := kind
		t.Run(kind.name, func(t *testing.T) {
			root := t.TempDir()
			s, err := store.Open(filepath.Join(root, "state"))
			if err != nil {
				t.Fatal(err)
			}
			dropzone := filepath.Join(root, "drop")
			if err := os.MkdirAll(dropzone, 0o700); err != nil {
				t.Fatal(err)
			}
			bad := map[string]bool{}
			for i := 1; i <= 10; i++ {
				name := fmt.Sprintf("%02d-ok.txt", i)
				content := "valid batch source " + name + "\n"
				if i == 3 || i == 7 {
					name = fmt.Sprintf("%02d-%s.txt", i, kind.name)
					content = kind.content
					if content != "" {
						content = name + ": " + content
					}
					bad[name] = true
				}
				if err := os.WriteFile(filepath.Join(dropzone, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			r := Runtime{Store: s, Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
				if bad[name] {
					return kind.analyze(name)
				}
				return completeResult(name), nil
			}}}

			ids, err := r.Once(context.Background(), dropzone)
			if err != nil {
				t.Fatalf("one malformed kind (%s) aborted the batch: %v (processed %d)", kind.name, err, len(ids))
			}

			ents, err := os.ReadDir(dropzone)
			if err != nil {
				t.Fatal(err)
			}
			var left []string
			for _, ent := range ents {
				left = append(left, ent.Name())
			}
			st := rebuildState(t, s)
			switch {
			case kind.leftInDrop:
				if len(ids) != 8 {
					t.Fatalf("admitted %d want the 8 valid items: %v", len(ids), ids)
				}
				if len(left) != 2 || !bad[left[0]] || !bad[left[1]] {
					t.Fatalf("dropzone leftover = %v want only the two malformed files", left)
				}
			case kind.atGOVGap:
				if len(ids) != 10 || len(left) != 0 {
					t.Fatalf("ids=%d left=%v want 10 admitted, empty dropzone", len(ids), left)
				}
				gapped := 0
				for _, id := range ids {
					em := st.AtGOV[id]
					if len(em.VAL.Gaps) > 0 {
						gapped++
					}
				}
				if gapped != 2 {
					t.Fatalf("coverage-gapped candidates = %d want 2", gapped)
				}
			}
			clean := 0
			for _, id := range ids {
				em, ok := st.AtGOV[id]
				if !ok {
					t.Fatalf("%s not at GOV", id)
				}
				if len(em.VAL.Gaps) != 0 {
					if !kind.atGOVGap {
						t.Fatalf("item %s stopped by coverage in a %s batch: gaps=%v", em.IDN, kind.name, em.VAL.Gaps)
					}
					continue
				}
				if !em.VAL.Recoil || !em.VAL.WVC || bad[em.REL["source_name"]] {
					t.Fatalf("item %s not a clean valid candidate: recoil=%t wvc=%t rel=%v", em.IDN, em.VAL.Recoil, em.VAL.WVC, em.REL)
				}
				clean++
			}
			if clean != 8 {
				t.Fatalf("clean GOV-ready valid items = %d want 8", clean)
			}
			if len(st.Accepted) != 0 {
				t.Fatalf("batch accepted %d records without HUMAN_FINAL", len(st.Accepted))
			}

			// No partial writes: the chain verifies and every preserved
			// object belongs to a recorded candidate (no orphans).
			n, err := fieldVerify(s)
			if err != nil {
				t.Fatalf("verify failed after malformed batch: %v", err)
			}
			hashes := map[string]bool{}
			for _, id := range ids {
				hashes[st.AtGOV[id].MEM.SourceHash] = true
			}
			if n < len(hashes) {
				t.Fatalf("evidence_verified = %d, fewer than the %d distinct admitted source hashes", n, len(hashes))
			}
			orphans, err := s.PruneOrphans()
			if err != nil {
				t.Fatal(err)
			}
			if orphans != 0 {
				t.Fatalf("malformed batch left %d orphan evidence objects", orphans)
			}
		})
	}
}

// TestMalformedBatchInfrastructureErrorStillAborts pins the other side of
// isolation: a reasoner result that is fine but whose source cannot be read
// is not a malformed item, so the batch stops and reports it.
func TestMalformedBatchInfrastructureErrorStillAborts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a 0000 file; permission stoppage cannot be staged")
	}
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	dropzone := filepath.Join(root, "drop")
	if err := os.MkdirAll(dropzone, 0o700); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dropzone, "01-unreadable.txt")
	if err := os.WriteFile(unreadable, []byte("x\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropzone, "02-ok.txt"), []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Runtime{Store: s, Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) { return completeResult(name), nil }}}
	if _, err := r.Once(context.Background(), dropzone); err == nil || !strings.Contains(err.Error(), "01-unreadable.txt") {
		t.Fatalf("unreadable source did not stop the batch with its name: %v", err)
	}
	if n := ledgerEventCount(t, s); n != 0 {
		t.Fatalf("aborted batch wrote %d events", n)
	}
}

// TestTargetRecomputeDoneWhen confirms the Phase 2.3 done-when: unchanged
// REG reality does not reopen the target, including after a restart (a
// fresh store.Open on the same state directory), and changed reality does.
// Unchanged recomputes append nothing to the ledger.
func TestTargetRecomputeDoneWhen(t *testing.T) {
	ctx := context.Background()
	tc := fractureCases()[0]
	f, boundary := acceptedFractureBoundary(t, tc)
	target := boundary.REL["target_emergion"]
	observer := fixedDifferenceObserver{result: differenceObserved([]string{"TARGET_STATE!=ACCEPTED_REALITY"})}

	recompute := func(t *testing.T, s *store.Store) (FractureWaveStatus, core.EmergION) {
		t.Helper()
		rt := Runtime{Store: s, Reasoner: observer}
		status, candidate, err := rt.recomputeFractureTarget(ctx, rebuildState(t, s),
			FractureWaveStatus{Target: target, Boundary: boundary.IDN}, target)
		if err != nil {
			t.Fatal(err)
		}
		return status, candidate
	}

	before := ledgerEventCount(t, f.store)
	status, candidate := recompute(t, f.store)
	if status.Status != waveTargetUnchanged || candidate.IDN != "" {
		t.Fatalf("unchanged reality reopened the target: %+v candidate=%s", status, candidate.IDN)
	}
	if got := ledgerEventCount(t, f.store); got != before {
		t.Fatalf("unchanged recompute appended %d events", got-before)
	}

	// Restart: a fresh store handle on the same directory.
	restarted, err := store.Open(f.store.Root)
	if err != nil {
		t.Fatal(err)
	}
	status, candidate = recompute(t, restarted)
	if status.Status != waveTargetUnchanged || candidate.IDN != "" {
		t.Fatalf("unchanged reality reopened the target after restart: %+v candidate=%s", status, candidate.IDN)
	}
	if got := ledgerEventCount(t, restarted); got != before {
		t.Fatalf("recompute after restart appended %d events", got-before)
	}

	// Changed reality: a new REG-accepted record. The target is reopened as
	// a fresh comparison at GOV, never accepted by the runtime itself.
	f.accept(acceptedProviderFixture("E-00000000000000F3", "done-when-new-reality", "OBS", "new accepted observation"))
	status, candidate = recompute(t, restarted)
	if status.Status != waveRecomputeProposed || candidate.STA != core.StateAtGOV {
		t.Fatalf("changed reality did not reopen the target: %+v", status)
	}
	if _, ok := rebuildState(t, restarted).Accepted[candidate.IDN]; ok {
		t.Fatal("recomputed comparison accepted without HUMAN_FINAL")
	}
}

// TestBoundedWaveDoneWhen confirms the Phase 2.4 done-when for the stop
// states: while a proposal waits for HUMAN_FINAL, while an accepted probe
// waits for its run, and at the round cap, repeated wave calls propose
// nothing and append nothing (no invented work). Every proposal that is
// made is a candidate at GOV with preserved evidence.
func TestBoundedWaveDoneWhen(t *testing.T) {
	tc := fractureCases()[0]
	f, _ := acceptedFractureBoundary(t, tc)
	repo := fractureRepo(t)
	rt := Runtime{Store: f.store, ProbeWorkDir: repo}

	stillStopped := func(t *testing.T, maxRounds int, want string) {
		t.Helper()
		before := ledgerEventCount(t, f.store)
		for i := 0; i < 3; i++ {
			status, candidate := waveStep(t, rt, repo, maxRounds)
			if status.Status != want || candidate.IDN != "" {
				t.Fatalf("call %d: wave = %+v candidate=%s want stop %s with no proposal", i+1, status, candidate.IDN, want)
			}
		}
		if got := ledgerEventCount(t, f.store); got != before {
			t.Fatalf("stopped wave (%s) appended %d events", want, got-before)
		}
	}

	status, probe := waveStep(t, rt, repo, 1)
	wantWave(t, status, waveProposedProbe, fractureLevelCatalog)
	if probe.STA != core.StateAtGOV {
		t.Fatalf("proposal not at GOV: %s", probe.STA)
	}
	if _, err := f.store.ReadEvidence(probe.MEM.SourceHash); err != nil {
		t.Fatalf("proposal has no preserved evidence: %v", err)
	}
	stillStopped(t, 1, waveAwaitingHumanFinal)

	acceptBoundaryCandidate(t, f, probe)
	stillStopped(t, 1, waveAwaitingRun)

	// Cross the level, then the cap of one round stops the wave.
	structuralAddition(t, repo, researchCatalogLine,
		`{ID: "RESEARCH", Capabilities: []string{"PATENT", "GRANT", "MARKET", "MA", "OBS", "VLD"}, Authority: "EVIDENCE_ONLY"}`)
	runWaveProbe(t, f, rt, probe.IDN)
	stillStopped(t, 1, waveMaxRounds)
}
