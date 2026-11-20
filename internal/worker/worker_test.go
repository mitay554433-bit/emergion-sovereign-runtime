package worker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/reg"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/internal/store"
	"emergion-sovereign-runtime/policy"
)

// fakeInsighter links to peer 1 and records calls; failFor makes one source fail.
type fakeInsighter struct {
	calls   int
	failFor string
}

func (f *fakeInsighter) Name() string { return "fake" }
func (f *fakeInsighter) Insight(_ context.Context, in reason.InsightInput) (reason.Insight, error) {
	f.calls++
	if in.SourceID == f.failFor {
		return reason.Insight{}, errors.New("model failed")
	}
	r := reason.Insight{Summary: "about " + in.Title, Combinations: []string{"combine"}, Gaps: []string{"test plan"}}
	if len(in.Peers) > 0 {
		r.Kin = []reason.InsightKin{{Peer: 1, Why: "fake"}, {Peer: 99, Why: "not listed"}}
	}
	return r, nil
}

func line(seed, topic string) string {
	h := fmt.Sprintf("%x", sha256.Sum256([]byte(seed)))
	return fmt.Sprintf(`{"path":"sources/%s/%s.pdf","sha256":"%s","bytes":10,"mime":"application/pdf","topic":"%s","title":"Doc %s","source_kind":"upload","related":["TR-1"]}`, topic, seed, h, topic, seed)
}

// setup ingests n uploads and HUMAN_FINAL-accepts them.
func setup(t *testing.T, n int) (*store.Store, []string) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i := 0; i < n; i++ {
		lines = append(lines, line(fmt.Sprint("s", i), "inventions-ip"))
	}
	rep, err := (fieldruntime.Runtime{Store: s}).IngestSourceManifest(context.Background(), strings.NewReader(strings.Join(lines, "\n")), fieldruntime.SourceIngestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, _ := s.Events()
	st, _ := livefield.Rebuild(events)
	for _, id := range rep.CreatedIDs {
		em, rc, err := gov.Decide(st.AtGOV[id], gov.Approve, "HUMAN_FINAL", "test")
		if err != nil {
			t.Fatal(err)
		}
		did, _ := s.SaveDecision(rc)
		_, rr, _ := reg.Accept(em, did)
		if _, err := s.SaveAccepted(rr); err != nil {
			t.Fatal(err)
		}
	}
	return s, rep.CreatedIDs
}

func state(t *testing.T, s *store.Store) core.State {
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

func count(st core.State, kind string) int {
	n := 0
	for _, set := range []map[string]core.EmergION{st.AtGOV, st.Accepted, st.Held} {
		for _, em := range set {
			if em.REL["source_kind"] == kind {
				n++
			}
		}
	}
	return n
}

func TestWorkerStepIsBoundedIdempotentAndCheckpointed(t *testing.T) {
	s, ids := setup(t, 5)
	f := &fakeInsighter{}
	w := &Worker{Store: s, Insighter: f, Dir: t.TempDir()}
	rep, err := w.Step(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Processed != 2 || rep.Insights != 2 || rep.Pending != 5 {
		t.Fatalf("step 1: %+v", rep)
	}
	cp := w.LoadCheckpoint()
	if cp.Cursor == "" || cp.Processed != 2 {
		t.Fatalf("checkpoint: %+v", cp)
	}
	// Resume: a fresh worker (restart) continues after the cursor.
	w2 := &Worker{Store: s, Insighter: f, Dir: w.Dir}
	for i := 0; i < 5; i++ {
		if _, err := w2.Step(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
	}
	st := state(t, s)
	if got := count(st, policy.KindInsight); got != len(ids) {
		t.Fatalf("insights %d, want %d (one per source)", got, len(ids))
	}
	links := count(st, policy.KindKinLink)
	if links == 0 || links > len(ids)*MaxKinPerSource {
		t.Fatalf("links %d", links)
	}
	// Idempotent: everything is done, nothing new, no reasoner calls.
	calls := f.calls
	rep, _ = w2.Step(context.Background(), 10)
	if rep.Processed != 0 || rep.Pending != 0 || f.calls != calls {
		t.Fatalf("not idempotent: %+v calls %d->%d", rep, calls, f.calls)
	}
	// Even with the checkpoint deleted the ledger stops regeneration.
	w3 := &Worker{Store: s, Insighter: f, Dir: t.TempDir()}
	if rep, _ := w3.Step(context.Background(), 10); rep.Processed != 0 {
		t.Fatalf("checkpoint loss regenerated: %+v", rep)
	}
	// Every proposal waits AtGOV: the worker never decides.
	for _, em := range st.Accepted {
		if k := em.REL["source_kind"]; k == policy.KindInsight || k == policy.KindKinLink {
			t.Fatalf("worker proposal %s was accepted without GOV", em.IDN)
		}
	}
	// Each unordered pair is linked once (no Cartesian bloat).
	seen := map[string]bool{}
	for _, em := range st.AtGOV {
		if em.REL["source_kind"] == policy.KindKinLink {
			k := pairKey(em.REL["kin_link_from"], em.REL["kin_link_to"])
			if seen[k] {
				t.Fatalf("pair %s linked twice", k)
			}
			seen[k] = true
		}
	}
}

func TestWorkerFailuresAreBoundedAndRetried(t *testing.T) {
	s, ids := setup(t, 2)
	f := &fakeInsighter{failFor: ids[0]}
	w := &Worker{Store: s, Insighter: f, Dir: t.TempDir()}
	for i := 0; i < 6; i++ {
		if _, err := w.Step(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
	}
	cp := w.LoadCheckpoint()
	if cp.Failures[ids[0]] != MaxFailures {
		t.Fatalf("failures: %+v", cp.Failures)
	}
	rep, _ := w.Step(context.Background(), 5)
	if rep.Pending != 0 {
		t.Fatalf("failed source must stop after %d tries: %+v", MaxFailures, rep)
	}
}

func TestWorkerLinksAndInsightsAutoAcceptUnderPolicy(t *testing.T) {
	s, _ := setup(t, 3)
	w := &Worker{Store: s, Insighter: &fakeInsighter{}, Dir: t.TempDir()}
	if _, err := w.Step(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	st := state(t, s)
	x := policy.BuildIndex(st)
	for _, em := range st.AtGOV {
		v := policy.Classify(st, x, em)
		switch em.REL["source_kind"] {
		case policy.KindKinLink:
			if v.Class != policy.ClassKinLinkWorker {
				t.Fatalf("link %s not low-risk: %+v (REL %v)", em.IDN, v, em.REL)
			}
			ev, err := s.ReadEvidence(em.MEM.SourceHash)
			if err != nil || policy.VerifyEvidence(v, em, ev, "") != nil {
				t.Fatalf("link evidence: %v", err)
			}
		case policy.KindInsight:
			// A01-AUTO-2: worker insights on existing uploads are low-risk
			if v.Class != policy.ClassInsightWorker || v.Decision != "APPROVE" {
				t.Fatalf("insight %s not low-risk: %+v (REL %v)", em.IDN, v, em.REL)
			}
			ev, err := s.ReadEvidence(em.MEM.SourceHash)
			if err != nil || policy.VerifyEvidence(v, em, ev, "") != nil {
				t.Fatalf("insight evidence: %v %v", err, policy.VerifyEvidence(v, em, ev, ""))
			}
		}
	}
}

func TestOnlineNotCalledInLocalMode(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"SUMMARY: online\nKIN: none\nCOMBO: none\nGAP: none"}}]}`))
	}))
	defer srv.Close()
	on := reason.OnlineInsighter{URL: srv.URL + "/v1", Model: "m"}
	on = on.WithKey("secret-key")
	local := &fakeInsighter{}
	sw := &reason.SwitchInsighter{Local: local, Online: &on, Heuristic: reason.HeuristicInsighter{}}
	sw.SetMode(reason.ModeLocal)
	in := reason.InsightInput{SourceID: "E-1", Title: "t", Topic: "x"}
	if _, err := sw.Insight(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 0 || local.calls != 1 {
		t.Fatalf("local mode hit online %d times", hits)
	}
	sw.SetMode(reason.ModeOnline)
	r, err := sw.Insight(context.Background(), in)
	if err != nil || r.Summary != "online" || atomic.LoadInt32(&hits) == 0 {
		t.Fatalf("online: %+v %v", r, err)
	}
	// auto: online reachable -> online
	before := atomic.LoadInt32(&hits)
	sw.SetMode(reason.ModeAuto)
	if r, err := sw.Insight(context.Background(), in); err != nil || r.Summary != "online" || atomic.LoadInt32(&hits) <= before {
		t.Fatalf("auto: %+v %v", r, err)
	}
	// auto with no online configured -> local
	sw2 := &reason.SwitchInsighter{Local: local, Online: &reason.OnlineInsighter{}, Heuristic: reason.HeuristicInsighter{}}
	sw2.SetMode(reason.ModeAuto)
	calls := local.calls
	if _, err := sw2.Insight(context.Background(), in); err != nil || local.calls != calls+1 {
		t.Fatalf("auto fallback to local: %v", err)
	}
}

func TestParseInsightKeepsOnlyListedPeers(t *testing.T) {
	in := reason.InsightInput{Peers: []reason.InsightPeer{{N: 1, ID: "E-1"}, {N: 2, ID: "E-2"}}}
	out := "user\nprompt...\n" + "GAP: <one thing missing to make it useful, or none>\n" +
		"1. SUMMARY: A grant brief.\n2. KIN: 2, 7, 2\n3. COMBO: pair with 2\n4. GAP: none\n"
	r, err := reason.ParseInsight(out, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary != "A grant brief." || len(r.Kin) != 1 || r.Kin[0].Peer != 2 || len(r.Gaps) != 0 || len(r.Combinations) != 1 {
		t.Fatalf("%+v", r)
	}
	if _, err := reason.ParseInsight("nothing useful", in); err == nil {
		t.Fatal("missing SUMMARY must fail")
	}
}

func TestLocalArgsUseAllCoresAndNoConversation(t *testing.T) {
	args := strings.Join(reason.LocalInsighter{Binary: "llama-completion", Model: "m.gguf", Threads: 8}.Args("p"), " ")
	for _, want := range []string{"-t 8", "--temp 0", "-m m.gguf"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q lack %q", args, want)
		}
	}
	// llama-completion waits in chat mode without -no-cnv; llama-cli rejects it.
	if !strings.Contains(args, "-no-cnv") || !strings.Contains(args, "-n 64") {
		t.Fatalf("llama-completion args need -no-cnv and -n 64: %q", args)
	}
	cli := strings.Join(reason.LocalInsighter{Binary: "/usr/bin/llama-cli", Model: "m.gguf", Threads: 8}.Args("p"), " ")
	if strings.Contains(cli, "-no-cnv") {
		t.Fatalf("llama-cli rejects -no-cnv: %q", cli)
	}
}

func TestLocalPromptIsShort(t *testing.T) {
	in := reason.InsightInput{Title: strings.Repeat("t", 300), Topic: "grants"}
	for i := 1; i <= 8; i++ {
		in.Peers = append(in.Peers, reason.InsightPeer{N: i, ID: fmt.Sprint("E-", i), Title: strings.Repeat("p", 200), Topic: "grants"})
	}
	p := reason.LocalPrompt(in)
	if strings.Contains(p, "\n5 ") || len(p) > len(reason.InsightPrompt(in))/2 {
		t.Fatalf("local prompt too long (%d bytes):\n%s", len(p), p)
	}
}

type runFailInsighter struct{}

func (runFailInsighter) Name() string { return "runfail" }
func (runFailInsighter) Insight(context.Context, reason.InsightInput) (reason.Insight, error) {
	return reason.Insight{}, fmt.Errorf("%w: local model timed out after 20m0s", reason.ErrModelRun)
}

func TestModelRunErrorsDoNotBlameSources(t *testing.T) {
	s, _ := setup(t, 3)
	w := &Worker{Store: s, Insighter: runFailInsighter{}, Dir: t.TempDir()}
	for i := 0; i < 3; i++ {
		if _, err := w.Step(context.Background(), 3); err != nil {
			t.Fatal(err)
		}
	}
	cp := w.LoadCheckpoint()
	if len(cp.Failures) != 0 || !strings.Contains(cp.LastError, "timed out") {
		t.Fatalf("model-run errors must not count against sources: %+v", cp)
	}
}

type downInsighter struct{ calls int }

func (d *downInsighter) Name() string { return "down" }
func (d *downInsighter) Insight(context.Context, reason.InsightInput) (reason.Insight, error) {
	d.calls++
	return reason.Insight{}, fmt.Errorf("%w: no model", reason.ErrUnavailable)
}

func TestWorkerPausesWhenReasonerUnavailable(t *testing.T) {
	s, _ := setup(t, 3)
	d := &downInsighter{}
	w := &Worker{Store: s, Insighter: d, Dir: t.TempDir()}
	for i := 0; i < 5; i++ {
		rep, err := w.Step(context.Background(), 3)
		if err != nil || !rep.Unavailable || rep.Pending != 3 {
			t.Fatalf("step %d: %+v %v", i, rep, err)
		}
	}
	if d.calls != 5 || len(w.LoadCheckpoint().Failures) != 0 || w.LoadCheckpoint().LastError == "" {
		t.Fatalf("unavailable must pause without blaming sources: calls %d cp %+v", d.calls, w.LoadCheckpoint())
	}
	// Local mode with no model on this box reports unavailable, never hangs.
	_, err := (reason.LocalInsighter{Binary: "definitely-not-llama", Model: ""}).Insight(context.Background(), reason.InsightInput{})
	if !errors.Is(err, reason.ErrUnavailable) {
		t.Fatalf("missing local model: %v", err)
	}
}

func TestLocalPrefersCompletionBesideCLI(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"llama-cli", "llama-completion"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GEMMA_BIN", filepath.Join(dir, "llama-cli"))
	if got := reason.LocalInsighterFromEnv().Binary; got != filepath.Join(dir, "llama-completion") {
		t.Fatalf("want llama-completion beside llama-cli, got %s", got)
	}
}

func TestParseInsightWithoutLabels(t *testing.T) {
	in := reason.InsightInput{Peers: []reason.InsightPeer{{N: 1, ID: "E-1"}, {N: 2, ID: "E-2"}}}
	r, err := reason.ParseInsight("**A pitch deck for the BDC grant.**\nKIN: 2\nGAP: none", in)
	if err != nil || r.Summary != "A pitch deck for the BDC grant." || len(r.Kin) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := reason.ParseInsight("ok", in); err == nil || !strings.Contains(err.Error(), `"ok"`) {
		t.Fatalf("error should quote the reply: %v", err)
	}
}
