package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/policy"
)

func TestClassifyGapIsDeterministic(t *testing.T) {
	cases := []struct{ text, class, pivot string }{
		{"No watchdog restarts fieldd after a reboot; backup and restore untested", policy.GapContinuity, "PIVOT_CONTINUITY_RECOVERY"},
		{"No API adapter or connector to import the registry", policy.GapIntegration, "PIVOT_ADAPTER_LAYER"},
		{"No paying customer, pricing or pilot revenue yet", policy.GapCommercial, "PIVOT_PRODUCT_SURVIVAL"},
		{"Crash on low battery; error handling and monitoring missing", policy.GapOperational, "PIVOT_FAIL_CLOSED_REPAIR"},
		{"Compression ratio claim lacks a reproducible benchmark", policy.GapProof, "PIVOT_EVIDENCE_BINDING"},
		{"unclear", policy.GapProof, "PIVOT_EVIDENCE_BINDING"}, // no hit falls back to proof
		{"", policy.GapProof, "PIVOT_EVIDENCE_BINDING"},
		{"rapid growth", policy.GapProof, "PIVOT_EVIDENCE_BINDING"}, // "api" is a whole word, not a substring
	}
	for _, c := range cases {
		for i := 0; i < 3; i++ {
			class, pivot := ClassifyGap(c.text)
			if class != c.class || pivot != c.pivot {
				t.Fatalf("ClassifyGap(%q) = %s %s, want %s %s", c.text, class, pivot, c.class, c.pivot)
			}
		}
	}
	for _, c := range policy.GapClasses {
		if !strings.HasPrefix(policy.GapPivot[c], "PIVOT_") {
			t.Fatalf("class %s has no PIVOT_* suggestion", c)
		}
	}
}

// gapInsighter returns fixed gaps for every source.
type gapInsighter struct{ gaps []string }

func (g gapInsighter) Name() string { return "gapfake" }
func (g gapInsighter) Insight(_ context.Context, in reason.InsightInput) (reason.Insight, error) {
	return reason.Insight{Summary: "about " + in.Title, Gaps: g.gaps}, nil
}

// acceptAtGOV HUMAN_FINAL-accepts every AtGOV record of kind.
func acceptAtGOV(t *testing.T, s *store.Store, kind string) {
	t.Helper()
	st := state(t, s)
	for _, em := range st.AtGOV {
		if em.REL["source_kind"] != kind {
			continue
		}
		d, rc, err := gov.Decide(em, gov.Approve, "HUMAN_FINAL", "test")
		if err != nil {
			t.Fatal(err)
		}
		did, _ := s.SaveDecision(rc)
		_, rr, err := reg.Accept(d, did)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveAccepted(rr); err != nil {
			t.Fatal(err)
		}
	}
}

func gapsIn(st core.State) []core.EmergION {
	var out []core.EmergION
	for _, set := range []map[string]core.EmergION{st.AtGOV, st.Approved, st.Accepted, st.Held} {
		for _, em := range set {
			if em.REL["source_kind"] == policy.KindGap {
				out = append(out, em)
			}
		}
	}
	return out
}

func TestGapStepDedupesPerSourceClassAndCaps(t *testing.T) {
	s, ids := setup(t, 4)
	ins := gapInsighter{gaps: []string{
		"Backup and restore after reboot untested", // continuity
		"No recovery checkpoint for the watchdog",  // continuity again: deduped
		"Benchmark claim not reproduced",           // proof
		"BRIDGEGAP:relationships",                  // engine token: ignored
	}}
	w := &Worker{Store: s, Insighter: ins, Dir: t.TempDir()}
	if _, err := w.Step(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	// Insights still AtGOV: no gap may be read from them.
	if g := gapsIn(state(t, s)); len(g) != 0 {
		t.Fatalf("gaps written from unaccepted insights: %d", len(g))
	}
	acceptAtGOV(t, s, policy.KindInsight)
	rep, err := w.Step(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	// 4 sources x 2 classes = 8 candidates, capped at MaxGapsPerStep.
	if rep.Gaps != MaxGapsPerStep {
		t.Fatalf("step gaps %d, want cap %d", rep.Gaps, MaxGapsPerStep)
	}
	rep, _ = w.Step(context.Background(), 4)
	if rep.Gaps != 8-MaxGapsPerStep {
		t.Fatalf("second step gaps %d, want %d", rep.Gaps, 8-MaxGapsPerStep)
	}
	rep, _ = w.Step(context.Background(), 4)
	if rep.Gaps != 0 {
		t.Fatalf("not idempotent: %+v", rep)
	}
	// A fresh worker (checkpoint lost) also regenerates nothing.
	if rep, _ := (&Worker{Store: s, Insighter: ins, Dir: t.TempDir()}).Step(context.Background(), 4); rep.Gaps != 0 {
		t.Fatalf("checkpoint loss regenerated gaps: %+v", rep)
	}
	st := state(t, s)
	seen := map[string]bool{}
	srcs := map[string]bool{}
	for _, id := range ids {
		srcs[id] = true
	}
	for _, em := range gapsIn(st) {
		if _, ok := st.AtGOV[em.IDN]; !ok {
			t.Fatalf("gap %s left AtGOV without HUMAN_FINAL", em.IDN)
		}
		k := em.REL["gap_of"] + "|" + em.REL["gap_class"]
		if seen[k] {
			t.Fatalf("duplicate (source, class) %s", k)
		}
		seen[k] = true
		if !srcs[em.REL["gap_of"]] || em.REL["gap_pivot"] != policy.GapPivot[em.REL["gap_class"]] {
			t.Fatalf("gap REL %v", em.REL)
		}
		if b := st.Accepted[em.REL["gap_basis"]]; b.REL["source_kind"] != policy.KindInsight {
			t.Fatalf("gap basis %s is not an accepted insight", em.REL["gap_basis"])
		}
		// REL carries only the gap keys plus existing keys.
		for k := range em.REL {
			switch k {
			case "source_kind", "gap_of", "gap_class", "gap_basis", "gap_pivot", "source_topic", "worker_version", "protector", "governed_state":
			default:
				t.Fatalf("unexpected REL key %s", k)
			}
		}
		ev, err := s.ReadEvidence(em.MEM.SourceHash)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(ev), "\n")
		if lines[0] != "SOURCE_GAP/1" {
			t.Fatalf("descriptor header %q", lines[0])
		}
		for _, p := range []string{"OF:" + em.REL["gap_of"], "CLASS:" + em.REL["gap_class"], "BASIS:" + em.REL["gap_basis"], "GAP:", "PROPOSED:" + em.REL["gap_pivot"], "WORKER:" + Version} {
			if !strings.Contains(string(ev), "\n"+p) {
				t.Fatalf("descriptor lacks %q:\n%s", p, ev)
			}
		}
		if em.MEM.Provenance != policy.ProvenanceGap || em.VAL.Risk != "M" {
			t.Fatalf("provenance %q risk %q", em.MEM.Provenance, em.VAL.Risk)
		}
	}
	if len(seen) != 8 {
		t.Fatalf("gaps %d, want 8", len(seen))
	}
}

func TestPolicyNeverAutoAcceptsSourceGap(t *testing.T) {
	s, _ := setup(t, 2)
	w := &Worker{Store: s, Insighter: gapInsighter{gaps: []string{"No customer pricing"}}, Dir: t.TempDir()}
	if _, err := w.Step(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	acceptAtGOV(t, s, policy.KindInsight)
	if _, err := w.Step(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	st := state(t, s)
	x := policy.BuildIndex(st)
	n := 0
	for _, em := range st.AtGOV {
		if em.REL["source_kind"] != policy.KindGap {
			continue
		}
		n++
		if v := policy.Classify(st, x, em); v.Decision != "" || v.Class != "" {
			t.Fatalf("SOURCE_GAP got a policy verdict: %+v", v)
		}
		// Even a gap shaped exactly like a low-risk record stays undecided.
		forged := em
		forged.VAL.Risk = "L"
		forged.REL = map[string]string{"source_kind": policy.KindGap}
		if v := policy.Classify(st, x, forged); v.Decision != "" {
			t.Fatalf("forged low-risk SOURCE_GAP decided: %+v", v)
		}
	}
	if n != 2 {
		t.Fatalf("gaps AtGOV %d, want 2", n)
	}
}

func TestOldWorkerJSONDecodesUnchanged(t *testing.T) {
	old := `{"version":"worker-1","cursor":"E-x","processed":7,"insights":7,"links":12,"last_reasoner":"local-gemma-server"}`
	var cp Checkpoint
	if err := json.Unmarshal([]byte(old), &cp); err != nil {
		t.Fatal(err)
	}
	if cp.Processed != 7 || cp.Insights != 7 || cp.Links != 12 || cp.Cursor != "E-x" || cp.Gaps != 0 {
		t.Fatalf("checkpoint %+v", cp)
	}
	b, _ := json.Marshal(StepReport{Pending: 1, Processed: 1, Insights: 1, Links: 2})
	if strings.Contains(string(b), "gaps") {
		t.Fatalf("zero gaps must not change the old step wire: %s", b)
	}
}
