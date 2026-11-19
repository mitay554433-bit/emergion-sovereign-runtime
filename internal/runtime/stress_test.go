package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
)

// Stress tests use only temporary state. They never open a live .field
// directory and they never call decide on real state.
//
// UNIFUSION_STRESS=1 runs the scale timing test, which is kept out of the
// default suite so a phone `go test` stays fast.
// UNIFUSION_STRESS_FINDINGS=1 runs the tests that pin known gaps. Those
// tests are expected to fail until the gap is closed; they must not be
// "fixed" by changing production verify, coverage, or REG behavior.

const (
	stressEnv         = "UNIFUSION_STRESS"
	stressFindingsEnv = "UNIFUSION_STRESS_FINDINGS"
)

func skipUnlessEnv(t *testing.T, env string) {
	t.Helper()
	if os.Getenv(env) != "1" {
		t.Skip("set " + env + "=1 to run this stress test")
	}
}

// fieldVerify is the non-mutating part of `field verify`: the COSL chain
// and every preserved evidence object. It does not prune orphans.
func fieldVerify(s *store.Store) (int, error) {
	if _, err := s.Events(); err != nil {
		return 0, err
	}
	return s.VerifyEvidence()
}

func acceptAtGOV(t *testing.T, s *store.Store, em core.EmergION, why string) {
	t.Helper()
	if em.STA != core.StateAtGOV {
		t.Fatalf("%s is not at GOV: %s", em.IDN, em.STA)
	}
	approved, decision, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", why)
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

func rebuildState(t *testing.T, s *store.Store) core.State {
	t.Helper()
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

type namedResultReasoner struct {
	forName func(name string) (reason.Result, error)
}

func (n namedResultReasoner) Analyze(_ context.Context, in reason.Input) (reason.Result, error) {
	return n.forName(in.Name)
}

func (namedResultReasoner) Name() string { return "stress-named" }

func (namedResultReasoner) Version(context.Context) string { return "1" }

func completeResult(name string, facts ...string) reason.Result {
	if len(facts) == 0 {
		facts = []string{"source_preserved:" + name}
	}
	return reason.Result{
		Summary:       "stress candidate " + name,
		Relationships: map[string]string{"source_name": name},
		Capabilities:  []string{"OBS"},
		Facts:         facts,
		Risk:          "L",
	}
}

// capturedAcceptedRegistry builds n REG-accepted EmergIONs through
// Capture, then HUMAN_FINAL and REG, in a fresh temporary store.
func capturedAcceptedRegistry(t *testing.T, names []string) *store.Store {
	t.Helper()
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	r := Runtime{
		Store: s,
		Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
			return completeResult(name), nil
		}},
	}
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("stress source for "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		em, _, err := r.Capture(context.Background(), path, false)
		if err != nil {
			t.Fatalf("Capture %s: %v", name, err)
		}
		acceptAtGOV(t, s, em, "accept stress registry "+name)
	}
	st := rebuildState(t, s)
	if len(st.Accepted) != len(names) {
		t.Fatalf("accepted = %d want %d", len(st.Accepted), len(names))
	}
	if _, err := fieldVerify(s); err != nil {
		t.Fatalf("untampered registry failed verify: %v", err)
	}
	return s
}

func ledgerLines(t *testing.T, s *store.Store) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.Root, "field.cosl"))
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.TrimSuffix(b, []byte("\n"))
	if len(b) == 0 {
		t.Fatal("empty COSL ledger")
	}
	return strings.Split(string(b), "\n")
}

func writeLedger(t *testing.T, s *store.Store, lines []string) {
	t.Helper()
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(s.Root, "field.cosl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func verifyErrorLocalizes(err error, line int, ev core.Event) (ok bool, how string) {
	if err == nil {
		return false, "verify returned no error"
	}
	msg := err.Error()
	var hit []string
	if line > 0 && strings.Contains(msg, fmt.Sprintf("line %d", line)) {
		hit = append(hit, "index")
	}
	if ev.ID != "" && strings.Contains(msg, ev.ID) {
		hit = append(hit, "event-id")
	}
	if ev.SelfHash != "" && strings.Contains(msg, ev.SelfHash) {
		hit = append(hit, "hash")
	}
	if ev.EmergION != nil {
		if ev.EmergION.IDN != "" && strings.Contains(msg, ev.EmergION.IDN) {
			hit = append(hit, "IDN")
		}
		if ev.EmergION.MEM.SourceHash != "" && strings.Contains(msg, ev.EmergION.MEM.SourceHash) {
			hit = append(hit, "source-hash")
		}
	}
	if ev.REG != nil && ev.REG.EmergIONID != "" && strings.Contains(msg, ev.REG.EmergIONID) {
		hit = append(hit, "IDN")
	}
	if ev.Decision != nil && ev.Decision.EmergIONID != "" && strings.Contains(msg, ev.Decision.EmergIONID) {
		hit = append(hit, "IDN")
	}
	if len(hit) == 0 {
		return false, msg
	}
	return true, strings.Join(hit, ",") + ": " + msg
}

func TestVerifyFailsOnTamperedAcceptedRegistry(t *testing.T) {
	names := []string{"alpha.txt", "beta.txt", "gamma.txt", "delta.txt"}

	t.Run("delete-middle-event", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		events, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		// Pick a C event that is not the first or last ledger line.
		line := 0
		var ev core.Event
		for i, e := range events {
			if e.Type == "C" && i > 0 && i < len(events)-1 {
				line = i + 1
				ev = e
				break
			}
		}
		if line == 0 {
			t.Fatal("no middle candidate event to delete")
		}
		lines := ledgerLines(t, s)
		writeLedger(t, s, append(lines[:line-1], lines[line:]...))
		n, err := fieldVerify(s)
		if err == nil {
			t.Fatalf("verify passed after deleting line %d event %s IDN %s; evidence_verified=%d", line, ev.ID, ev.EmergION.IDN, n)
		}
		t.Logf("delete-middle-event: removed former line %d event %s IDN %s; verify error: %v", line, ev.ID, ev.EmergION.IDN, err)
	})

	t.Run("flip-byte-in-event", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		events, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		line := 0
		var ev core.Event
		for i, e := range events {
			if e.Type == "C" && i > 0 && i < len(events)-1 {
				line = i + 1
				ev = e
				break
			}
		}
		if line == 0 {
			t.Fatal("no middle candidate event to flip")
		}
		lines := ledgerLines(t, s)
		raw := []byte(lines[line-1])
		if len(raw) < 8 {
			t.Fatalf("line %d too short to flip", line)
		}
		raw[len(raw)/2] ^= 0x01
		lines[line-1] = string(raw)
		writeLedger(t, s, lines)
		n, err := fieldVerify(s)
		if err == nil {
			t.Fatalf("verify passed after flipping a byte in line %d event %s IDN %s; evidence_verified=%d", line, ev.ID, ev.EmergION.IDN, n)
		}
		t.Logf("flip-byte-in-event: line %d event %s IDN %s; verify error: %v", line, ev.ID, ev.EmergION.IDN, err)
		if ok, how := verifyErrorLocalizes(err, line, ev); ok {
			t.Logf("flip-byte-in-event localized by %s", how)
		} else {
			t.Logf("flip-byte-in-event did not name the record: %s", how)
		}
	})

	t.Run("reorder-two-events", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		events, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		if len(events) < 4 {
			t.Fatalf("need at least 4 events, got %d", len(events))
		}
		// Swap two adjacent middle events.
		i := 3
		j := 4
		left, right := events[i], events[j]
		lines := ledgerLines(t, s)
		lines[i], lines[j] = lines[j], lines[i]
		writeLedger(t, s, lines)
		n, err := fieldVerify(s)
		if err == nil {
			t.Fatalf("verify passed after reordering lines %d and %d; evidence_verified=%d", i+1, j+1, n)
		}
		t.Logf("reorder-two-events: swapped lines %d (%s) and %d (%s); verify error: %v", i+1, left.ID, j+1, right.ID, err)
	})

	t.Run("delete-evidence-object", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		st := rebuildState(t, s)
		var target core.EmergION
		for _, em := range st.Accepted {
			target = em
			break
		}
		if target.MEM.SourceHash == "" {
			t.Fatal("accepted EmergION has no source hash")
		}
		path := filepath.Join(s.Root, "o", target.MEM.SourceHash+".gz")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		n, err := fieldVerify(s)
		if err == nil {
			t.Fatalf("verify passed after deleting evidence for %s hash %s; evidence_verified=%d", target.IDN, target.MEM.SourceHash, n)
		}
		t.Logf("delete-evidence-object: IDN %s hash %s; verify error: %v", target.IDN, target.MEM.SourceHash, err)
	})

	t.Run("flip-byte-in-evidence", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		st := rebuildState(t, s)
		var target core.EmergION
		for _, em := range st.Accepted {
			target = em
			break
		}
		path := filepath.Join(s.Root, "o", target.MEM.SourceHash+".gz")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) == 0 {
			t.Fatal("empty evidence object")
		}
		raw[len(raw)/2] ^= 0x01
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		n, err := fieldVerify(s)
		if err == nil {
			t.Fatalf("verify passed after flipping a byte in evidence for %s hash %s; evidence_verified=%d", target.IDN, target.MEM.SourceHash, n)
		}
		t.Logf("flip-byte-in-evidence: IDN %s hash %s; verify error: %v", target.IDN, target.MEM.SourceHash, err)
	})
}

func fieldVerifyExpecting(s *store.Store, expectTip string, expectCount *int) (int, error) {
	events, err := s.Events()
	if err != nil {
		return 0, err
	}
	if err := store.CheckExpectedTip(events, expectTip, expectCount); err != nil {
		return 0, err
	}
	return s.VerifyEvidence()
}

func TestVerifyDetectsTipDeletionWhenExpectedTipSupplied(t *testing.T) {
	s := capturedAcceptedRegistry(t, []string{"alpha.txt", "beta.txt", "gamma.txt", "delta.txt"})
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	expectTip, expectCount := store.LedgerTip(events)
	if expectTip == "" || expectCount == 0 {
		t.Fatal("untampered ledger has no tip to save as a receipt")
	}

	// Matching receipt still passes.
	if _, err := fieldVerifyExpecting(s, expectTip, &expectCount); err != nil {
		t.Fatalf("verify failed against the current receipt: %v", err)
	}

	lines := ledgerLines(t, s)
	writeLedger(t, s, lines[:len(lines)-1])

	_, err = fieldVerifyExpecting(s, expectTip, &expectCount)
	if err == nil {
		t.Fatal("verify passed after the tip event was deleted even though an earlier receipt was supplied")
	}
	t.Logf("tip deletion with receipt: %v", err)
	msg := err.Error()
	if !strings.Contains(msg, "expected tip "+expectTip) {
		t.Fatalf("error did not name the expected tip %s: %v", expectTip, err)
	}
	if !strings.Contains(msg, fmt.Sprintf("expected count %d", expectCount)) {
		t.Fatalf("error did not name the expected count %d: %v", expectCount, err)
	}
	if !strings.Contains(msg, "actual ") {
		t.Fatalf("error did not name the actual tip or count: %v", err)
	}
}

func TestVerifyWithoutExpectedTipStillPassesAfterTipDeletion(t *testing.T) {
	s := capturedAcceptedRegistry(t, []string{"alpha.txt", "beta.txt", "gamma.txt", "delta.txt"})
	events, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("empty ledger")
	}
	lines := ledgerLines(t, s)
	writeLedger(t, s, lines[:len(lines)-1])

	// CHOICE B: without an operator-supplied receipt, tip deletion is
	// still invisible to verify. That is today's behavior, kept on purpose.
	if _, err := fieldVerify(s); err != nil {
		t.Fatalf("verify without an expected tip should still pass after tip deletion: %v", err)
	}
	if _, err := fieldVerifyExpecting(s, "", nil); err != nil {
		t.Fatalf("empty expectation should match no-flag verify: %v", err)
	}
}

func TestVerifyLocalizesRecordInErrors(t *testing.T) {
	names := []string{"alpha.txt", "beta.txt", "gamma.txt", "delta.txt"}

	t.Run("delete-middle-event-names-the-record", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		events, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		line := 0
		var ev core.Event
		for i, e := range events {
			if e.Type == "C" && i > 0 && i < len(events)-1 {
				line = i + 1
				ev = e
				break
			}
		}
		if line == 0 {
			t.Fatal("no middle candidate event to delete")
		}
		lines := ledgerLines(t, s)
		writeLedger(t, s, append(lines[:line-1], lines[line:]...))
		_, err = fieldVerify(s)
		if err == nil {
			t.Fatal("verify passed after a middle event was deleted")
		}
		// After deletion the successor occupies the old index, so a
		// "line N" report is not a name for the missing record. Only
		// the deleted event's IDN or hash counts. The successor's
		// claimed prev hash is the deleted event's SelfHash.
		msg := err.Error()
		named := ev.EmergION != nil && ev.EmergION.IDN != "" && strings.Contains(msg, ev.EmergION.IDN)
		if ev.SelfHash != "" && strings.Contains(msg, ev.SelfHash) {
			named = true
		}
		if ev.ID != "" && strings.Contains(msg, ev.ID) {
			named = true
		}
		if ev.EmergION != nil && ev.EmergION.MEM.SourceHash != "" && strings.Contains(msg, ev.EmergION.MEM.SourceHash) {
			named = true
		}
		if !named {
			t.Fatalf("verify failed but did not name deleted event %s IDN %s hash %s: %v", ev.ID, ev.EmergION.IDN, ev.SelfHash, err)
		}
		if !strings.Contains(msg, fmt.Sprintf("line %d", line)) {
			t.Fatalf("verify error dropped the line number %d: %v", line, err)
		}
	})

	t.Run("evidence-flip-names-the-record", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		st := rebuildState(t, s)
		var target core.EmergION
		for _, em := range st.Accepted {
			target = em
			break
		}
		path := filepath.Join(s.Root, "o", target.MEM.SourceHash+".gz")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw[len(raw)/2] ^= 0x01
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = fieldVerify(s)
		if err == nil {
			t.Fatal("verify passed after an evidence byte flip")
		}
		msg := err.Error()
		if !strings.Contains(msg, target.IDN) && !strings.Contains(msg, target.MEM.SourceHash) {
			t.Fatalf("verify failed but did not name IDN %s or hash %s: %v", target.IDN, target.MEM.SourceHash, err)
		}
	})

	t.Run("missing-evidence-names-the-record", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		st := rebuildState(t, s)
		var target core.EmergION
		for _, em := range st.Accepted {
			target = em
			break
		}
		path := filepath.Join(s.Root, "o", target.MEM.SourceHash+".gz")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		_, err := fieldVerify(s)
		if err == nil {
			t.Fatal("verify passed after evidence was deleted")
		}
		msg := err.Error()
		// The object path already contains the hash, so only an explicit
		// IDN proves the error named the record rather than the file path.
		if !strings.Contains(msg, target.IDN) {
			t.Fatalf("verify failed but did not name IDN %s: %v", target.IDN, err)
		}
	})

	t.Run("flipped-event-byte-names-the-record", func(t *testing.T) {
		s := capturedAcceptedRegistry(t, names)
		events, err := s.Events()
		if err != nil {
			t.Fatal(err)
		}
		line := 0
		var ev core.Event
		for i, e := range events {
			if e.Type == "C" && i > 0 && i < len(events)-1 {
				line = i + 1
				ev = e
				break
			}
		}
		if line == 0 {
			t.Fatal("no middle candidate event to flip")
		}
		lines := ledgerLines(t, s)
		raw := []byte(lines[line-1])
		if len(raw) < 8 {
			t.Fatalf("line %d too short to flip", line)
		}
		raw[len(raw)/2] ^= 0x01
		lines[line-1] = string(raw)
		writeLedger(t, s, lines)
		_, err = fieldVerify(s)
		if err == nil {
			t.Fatal("verify passed after flipping a byte in an event")
		}
		msg := err.Error()
		named := ev.EmergION != nil && ev.EmergION.IDN != "" && strings.Contains(msg, ev.EmergION.IDN)
		if ev.SelfHash != "" && strings.Contains(msg, ev.SelfHash) {
			named = true
		}
		if ev.ID != "" && strings.Contains(msg, ev.ID) {
			named = true
		}
		if ev.EmergION != nil && ev.EmergION.MEM.SourceHash != "" && strings.Contains(msg, ev.EmergION.MEM.SourceHash) {
			named = true
		}
		if !named {
			t.Fatalf("verify failed but did not name flipped event %s IDN %s hash %s: %v", ev.ID, ev.EmergION.IDN, ev.SelfHash, err)
		}
		if !strings.Contains(msg, fmt.Sprintf("line %d", line)) {
			t.Fatalf("verify error dropped the line number %d: %v", line, err)
		}
	})
}

func TestMalformedBatchCoverageStopsOnlyBadCandidates(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	dropzone := filepath.Join(root, "drop")
	if err := os.MkdirAll(dropzone, 0o700); err != nil {
		t.Fatal(err)
	}

	// Ten candidates, two missing facts (20%). The rest are complete.
	malformed := map[string]bool{
		"07-missing-facts-a.txt": true,
		"08-missing-facts-b.txt": true,
	}
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("%02d-ok.txt", i)
		if i == 7 {
			name = "07-missing-facts-a.txt"
		}
		if i == 8 {
			name = "08-missing-facts-b.txt"
		}
		if err := os.WriteFile(filepath.Join(dropzone, name), []byte("batch source "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	r := Runtime{
		Store: s,
		Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
			if malformed[name] {
				return reason.Result{
					Summary:       "candidate with no facts",
					Relationships: map[string]string{"source_name": name},
					Capabilities:  []string{"OBS"},
					Risk:          "L",
				}, nil
			}
			return completeResult(name), nil
		}},
	}

	ids, err := r.Once(context.Background(), dropzone)
	if err != nil {
		t.Fatalf("Once stopped the batch: %v", err)
	}
	if len(ids) != 10 {
		t.Fatalf("Once ids = %d want 10: %#v", len(ids), ids)
	}

	st := rebuildState(t, s)
	var stopped, ready []core.EmergION
	for _, id := range ids {
		em, ok := st.AtGOV[id]
		if !ok {
			t.Fatalf("%s not at GOV after Once", id)
		}
		if containsString(em.VAL.Gaps, "BRIDGEGAP:PIVOT_COVERAGE") || em.MEM.Provenance == "reciprocal_pivot" {
			stopped = append(stopped, em)
			continue
		}
		if !em.VAL.Recoil || !em.VAL.WVC || len(em.VAL.Facts) == 0 {
			t.Fatalf("well-formed candidate %s was not GOV-ready: recoil=%t wvc=%t facts=%v gaps=%v", em.IDN, em.VAL.Recoil, em.VAL.WVC, em.VAL.Facts, em.VAL.Gaps)
		}
		ready = append(ready, em)
	}
	if len(stopped) != 2 {
		t.Fatalf("coverage stopped %d candidates, want 2", len(stopped))
	}
	if len(ready) != 8 {
		t.Fatalf("GOV-ready candidates = %d want 8", len(ready))
	}
	for _, em := range stopped {
		if em.REL["source_name"] != "07-missing-facts-a.txt" && em.REL["source_name"] != "08-missing-facts-b.txt" {
			// Recapture evidence may not keep source_name; also accept
			// required_capability ESTABLISH_FACT as the facts gap.
			if em.REL["required_capability"] != "ESTABLISH_FACT" {
				t.Fatalf("stopped candidate was not one of the missing-facts files: IDN %s rel=%v gaps=%v", em.IDN, em.REL, em.VAL.Gaps)
			}
		}
	}

	for _, em := range ready {
		acceptAtGOV(t, s, em, "accept well-formed batch member")
	}
	st = rebuildState(t, s)
	if len(st.Accepted) != 8 {
		t.Fatalf("accepted = %d want 8", len(st.Accepted))
	}
	for _, em := range stopped {
		if _, ok := st.Accepted[em.IDN]; ok {
			t.Fatalf("coverage-stopped candidate %s was accepted", em.IDN)
		}
		if _, ok := st.AtGOV[em.IDN]; !ok {
			t.Fatalf("coverage-stopped candidate %s left GOV", em.IDN)
		}
	}
}

func TestMalformedBatchIsolationFindings(t *testing.T) {
	runBatch := func(t *testing.T, files map[string]string, analyze func(string) (reason.Result, error)) ([]string, string, error) {
		t.Helper()
		root := t.TempDir()
		s, err := store.Open(filepath.Join(root, "state"))
		if err != nil {
			t.Fatal(err)
		}
		dropzone := filepath.Join(root, "drop")
		if err := os.MkdirAll(dropzone, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dropzone, name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		r := Runtime{
			Store: s,
			Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
				return analyze(name)
			}},
		}
		ids, err := r.Once(context.Background(), dropzone)
		return ids, dropzone, err
	}

	complete := func(name string) (reason.Result, error) {
		return completeResult(name), nil
	}

	remaining := func(t *testing.T, dropzone string) []string {
		t.Helper()
		ents, err := os.ReadDir(dropzone)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, ent := range ents {
			names = append(names, ent.Name())
		}
		return names
	}

	t.Run("grammar-error-must-not-stop-the-rest", func(t *testing.T) {
		files := map[string]string{
			"01-ok.txt":          "ok 1\n",
			"02-bad-grammar.txt": "bad grammar\n",
			"03-ok.txt":          "ok 3\n",
			"04-ok.txt":          "ok 4\n",
			"05-ok.txt":          "ok 5\n",
		}
		ids, dropzone, err := runBatch(t, files, func(name string) (reason.Result, error) {
			if name == "02-bad-grammar.txt" {
				return reason.Result{}, fmt.Errorf("MXPD grammar rejected")
			}
			return complete(name)
		})
		if err != nil {
			t.Fatalf("Once aborted the batch on a grammar error instead of stopping only that candidate: %v (processed %d)", err, len(ids))
		}
		if len(ids) != 4 {
			t.Fatalf("Once ids = %d want 4 remaining well-formed candidates plus no grammar candidate: %#v", len(ids), ids)
		}
		left := remaining(t, dropzone)
		if len(left) != 1 || left[0] != "02-bad-grammar.txt" {
			t.Fatalf("dropzone leftover = %#v, want only the grammar-error file", left)
		}
	})

	t.Run("empty-source-must-not-stop-the-rest", func(t *testing.T) {
		files := map[string]string{
			"01-ok.txt":             "ok 1\n",
			"02-missing-source.txt": "",
			"03-ok.txt":             "ok 3\n",
			"04-ok.txt":             "ok 4\n",
			"05-ok.txt":             "ok 5\n",
		}
		ids, dropzone, err := runBatch(t, files, complete)
		if err != nil {
			t.Fatalf("Once aborted the batch on an empty source instead of stopping only that candidate: %v (processed %d)", err, len(ids))
		}
		if len(ids) != 4 {
			t.Fatalf("Once ids = %d want 4 well-formed candidates: %#v", len(ids), ids)
		}
		left := remaining(t, dropzone)
		if len(left) != 1 || left[0] != "02-missing-source.txt" {
			t.Fatalf("dropzone leftover = %#v, want only the empty source file", left)
		}
	})
}

func TestCoverageDoesNotCheckHashCorrespondenceFinding(t *testing.T) {
	skipUnlessEnv(t, stressFindingsEnv)
	em := core.EmergION{
		IDN: "E-BAD-HASH",
		MEM: core.Memory{
			SourceHash: "not-the-hash-of-any-preserved-object",
			Bytes:      10,
			Stored:     10,
			Summary:    "summary",
		},
		REL: map[string]string{"source_name": "bad-hash.txt"},
		CAP: []string{"OBS"},
		VAL: core.Validation{Facts: []string{"source_preserved"}},
	}
	if err := coverage(&em, ""); err == nil {
		t.Fatal("coverage accepted a candidate whose source hash cannot correspond to preserved evidence")
	}
}

func TestVerifyScaleGrowth(t *testing.T) {
	skipUnlessEnv(t, stressEnv)

	build := func(t *testing.T, n int) *store.Store {
		t.Helper()
		s, err := store.Open(filepath.Join(t.TempDir(), "state"))
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		for i := 0; i < n; i++ {
			content := []byte(fmt.Sprintf("scale source %d\n", i))
			ev, err := s.Preserve(content)
			if err != nil {
				t.Fatal(err)
			}
			em := core.EmergION{
				IDN: fmt.Sprintf("E-SCALE-%04d", i),
				STA: core.StateAtGOV,
				MEM: core.Memory{
					SourceHash: ev.Hash,
					Codec:      ev.Codec,
					Bytes:      ev.Bytes,
					Stored:     ev.Stored,
					Summary:    fmt.Sprintf("scale candidate %d", i),
					Provenance: "stress_scale",
				},
				REL: map[string]string{"source_name": fmt.Sprintf("scale-%04d.txt", i)},
				CAP: []string{"OBS"},
				VAL: core.Validation{
					Facts:  []string{fmt.Sprintf("scale_fact_%d", i)},
					Recoil: true,
					WVC:    true,
				},
				EVO: core.Evolution{Version: 1},
			}
			if _, err := s.SaveCandidate(em); err != nil {
				t.Fatal(err)
			}
			acceptAtGOV(t, s, em, "accept scale fixture")
		}
		t.Logf("build n=%d took %s", n, time.Since(start).Round(time.Millisecond))
		st := rebuildState(t, s)
		if len(st.Accepted) != n {
			t.Fatalf("accepted = %d want %d", len(st.Accepted), n)
		}
		return s
	}

	timeVerify := func(t *testing.T, label string, s *store.Store) time.Duration {
		t.Helper()
		start := time.Now()
		n, err := fieldVerify(s)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("%s verify failed: %v", label, err)
		}
		t.Logf("%s: verify passed in %s (%d evidence objects)", label, elapsed, n)
		return elapsed
	}

	const size1x = 200
	const size10x = 2000
	s1 := build(t, size1x)
	t1 := timeVerify(t, fmt.Sprintf("1x n=%d", size1x), s1)
	s10 := build(t, size10x)
	t10 := timeVerify(t, fmt.Sprintf("10x n=%d", size10x), s10)

	if t1 <= 0 || t10 <= 0 {
		t.Fatal("verify timing was not measurable")
	}
	ratio := float64(t10) / float64(t1)
	t.Logf("10x/1x verify ratio = %.2f (size ratio = %.1f). ~linear would be near 10.", ratio, float64(size10x)/float64(size1x))
}

func TestAcceptedContradictionGoesUndetected(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	r := Runtime{
		Store: s,
		Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
			switch name {
			case "door-open.txt":
				return reason.Result{
					Summary:       "observation that the chamber door is open",
					Relationships: map[string]string{"source_name": "chamber-door"},
					Capabilities:  []string{"OBS"},
					Facts:         []string{"chamber door is open"},
					Risk:          "L",
				}, nil
			case "door-closed.txt":
				return reason.Result{
					Summary:       "observation that the chamber door is closed",
					Relationships: map[string]string{"source_name": "chamber-door"},
					Capabilities:  []string{"OBS"},
					Facts:         []string{"chamber door is closed"},
					Risk:          "L",
				}, nil
			default:
				return completeResult(name), nil
			}
		}},
	}

	var accepted []core.EmergION
	for _, name := range []string{"door-open.txt", "door-closed.txt"} {
		em, _, err := r.CaptureBytes(context.Background(), name, []byte("source for "+name+"\n"), "stress_contradiction")
		if err != nil {
			t.Fatalf("CaptureBytes %s: %v", name, err)
		}
		acceptAtGOV(t, s, em, "accept possibly conflicting observation")
		accepted = append(accepted, em)
	}

	st := rebuildState(t, s)
	if len(st.Accepted) != 2 {
		t.Fatalf("accepted = %d want 2", len(st.Accepted))
	}
	for _, em := range accepted {
		got, ok := st.Accepted[em.IDN]
		if !ok {
			t.Fatalf("%s did not remain accepted", em.IDN)
		}
		t.Logf("accepted %s facts=%v source_name=%q", got.IDN, got.VAL.Facts, got.REL["source_name"])
	}
	if _, err := fieldVerify(s); err != nil {
		t.Fatalf("verify failed on a ledger that holds both conflicting accepted facts: %v", err)
	}

	// Characterization: today there is no read-only query that lists
	// conflicting accepted fact pairs. Rebuild, verify, coverage and
	// REG acceptance all treat the two records as independently valid.
	// Section 8's UNRESOLVED:difference_contradicts_accepted_fact applies
	// only to a TARGET_COMPARISON, not to two already-accepted records.
	t.Log("current runtime: both conflicting facts remain accepted; verify passes; no contradiction listing is produced")
}

func TestAcceptedContradictionListingFinding(t *testing.T) {
	skipUnlessEnv(t, stressFindingsEnv)
	root := t.TempDir()
	s, err := store.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	r := Runtime{
		Store: s,
		Reasoner: namedResultReasoner{forName: func(name string) (reason.Result, error) {
			fact := "chamber door is open"
			if name == "door-closed.txt" {
				fact = "chamber door is closed"
			}
			return reason.Result{
				Summary:       "observation of the chamber door",
				Relationships: map[string]string{"source_name": "chamber-door"},
				Capabilities:  []string{"OBS"},
				Facts:         []string{fact},
				Risk:          "L",
			}, nil
		}},
	}
	for _, name := range []string{"door-open.txt", "door-closed.txt"} {
		em, _, err := r.CaptureBytes(context.Background(), name, []byte("source for "+name+"\n"), "stress_contradiction")
		if err != nil {
			t.Fatal(err)
		}
		acceptAtGOV(t, s, em, "accept possibly conflicting observation")
	}
	st := rebuildState(t, s)
	pairs := listConflictingAcceptedFacts(st)
	if len(pairs) == 0 {
		t.Fatal("no read-only listing of conflicting accepted fact pairs exists, and two accepted records about chamber-door hold opposite facts")
	}
}

// listConflictingAcceptedFacts is a placeholder so the finding test
// compiles. It does not invent a conflict rule: without a canon decision
// on what makes two accepted facts contradict, it returns nothing.
func listConflictingAcceptedFacts(st core.State) []string {
	_ = st
	return nil
}

func TestFieldVerifyRejectsNothingWhenUntampered(t *testing.T) {
	s := capturedAcceptedRegistry(t, []string{"one.txt", "two.txt"})
	n, err := fieldVerify(s)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("evidence_verified = %d want 2", n)
	}
}
