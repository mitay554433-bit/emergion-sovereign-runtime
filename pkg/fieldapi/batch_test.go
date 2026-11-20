package fieldapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/cosl"
	livefield "emergion-sovereign-runtime/internal/field"
)

// seedAtGOV captures one real record, then writes n-1 more AtGOV candidates
// cloned from it, each with its own preserved evidence, chained directly into
// the COSL ledger so a large ledger does not cost n full-ledger appends.
func seedAtGOV(t testing.TB, n int, rel ...func(i int, r map[string]string)) (*Runtime, []string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "seed.txt")
	if err := os.WriteFile(src, []byte("batch seed record\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rt, err := Open(filepath.Join(root, "state"), skyhookSilentReasoner{})
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := rt.Capture(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{tmpl.IDN}
	if n <= 1 {
		return rt, ids
	}
	events, err := rt.store.Events()
	if err != nil {
		t.Fatal(err)
	}
	prev := events[len(events)-1].SelfHash
	var b strings.Builder
	at := time.Now().UTC()
	for i := 1; i < n; i++ {
		ev, err := rt.store.Preserve([]byte(fmt.Sprintf("batch scale record %d\n", i)))
		if err != nil {
			t.Fatal(err)
		}
		em := tmpl
		em.IDN = fmt.Sprintf("E-B%015X", i)
		em.MEM.SourceHash, em.MEM.Bytes, em.MEM.Stored, em.MEM.Codec = ev.Hash, ev.Bytes, ev.Stored, ev.Codec
		em.MEM.Summary = fmt.Sprintf("batch scale record %d", i)
		em.REL = map[string]string{}
		for k, v := range tmpl.REL {
			em.REL[k] = v
		}
		if i%2 == 0 {
			em.REL["source_kind"] = "EVEN"
		}
		for _, f := range rel {
			f(i, em.REL)
		}
		line, err := cosl.Encode(core.Event{Type: "C", ID: fmt.Sprintf("EV-B%015X", i), At: at, EmergION: &em, PrevHash: prev})
		if err != nil {
			t.Fatal(err)
		}
		d, err := cosl.Decode(line)
		if err != nil {
			t.Fatal(err)
		}
		prev = d.SelfHash
		b.WriteString(line)
		b.WriteByte('\n')
		ids = append(ids, em.IDN)
	}
	f, err := os.OpenFile(filepath.Join(rt.store.Root, "field.cosl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(b.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	st, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.AtGOV) != n {
		t.Fatalf("seeded %d AtGOV, want %d", len(st.AtGOV), n)
	}
	return rt, ids
}

func ledger(t testing.TB, rt *Runtime) []core.Event {
	t.Helper()
	ev, err := rt.store.Events()
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func verifyReplay(t testing.TB, rt *Runtime, wantAccepted int) {
	t.Helper()
	events := ledger(t, rt)
	st, err := livefield.Rebuild(events)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if st.TipHash != events[len(events)-1].SelfHash {
		t.Fatal("rebuilt tip does not match ledger tip")
	}
	if len(st.Accepted) != wantAccepted {
		t.Fatalf("accepted %d, want %d", len(st.Accepted), wantAccepted)
	}
	if _, err := rt.store.VerifyEvidence(); err != nil {
		t.Fatalf("verify evidence: %v", err)
	}
}

func TestBatchHumanFinalOneReceiptPerRecord(t *testing.T) {
	rt, ids := seedAtGOV(t, 3)
	before := len(ledger(t, rt))
	res, err := rt.DecideBatchHuman(ids, "reviewed these three", 3, DefaultBatchCap)
	if err != nil || res.Accepted != 3 || res.Skipped != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	events := ledger(t, rt)[before:]
	if len(events) != 6 {
		t.Fatalf("want 6 new events (D,R per record), got %d", len(events))
	}
	seen := map[string]bool{}
	for i := 0; i < len(events); i += 2 {
		d, r := events[i], events[i+1]
		if d.Type != "D" || r.Type != "R" || d.Decision == nil || r.REG == nil {
			t.Fatalf("pair %d is %s,%s", i/2, d.Type, r.Type)
		}
		if d.Decision.Authority != "HUMAN_FINAL" || d.Decision.Decision != "APPROVE" {
			t.Fatalf("decision %+v", d.Decision)
		}
		if want := "reviewed these three [HUMAN_FINAL batch " + res.BatchID + ", 3 records]"; d.Decision.Reason != want {
			t.Fatalf("reason %q, want %q", d.Decision.Reason, want)
		}
		if r.REG.DecisionID != d.ID || r.REG.EmergIONID != d.Decision.EmergIONID {
			t.Fatalf("REG receipt does not cite its decision: %+v", r.REG)
		}
		seen[d.Decision.EmergIONID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("receipts cover %d distinct records", len(seen))
	}
	verifyReplay(t, rt, 3)

	n := len(ledger(t, rt))
	again, err := rt.DecideBatchHuman(ids, "reviewed these three", 3, DefaultBatchCap)
	if err != nil || again.Accepted != 0 || again.Skipped != 3 {
		t.Fatalf("re-run res=%+v err=%v", again, err)
	}
	if len(ledger(t, rt)) != n {
		t.Fatal("re-run wrote events")
	}
}

func TestBatchHumanFinalRefusals(t *testing.T) {
	rt, ids := seedAtGOV(t, 3)
	n := len(ledger(t, rt))
	for name, c := range map[string]struct {
		ids            []string
		reason         string
		confirm, limit int
	}{
		"count mismatch": {ids, "ok", 2, 500},
		"cap":            {ids, "ok", 3, 2},
		"no reason":      {ids, "  ", 3, 500},
		"duplicate":      {[]string{ids[0], ids[0]}, "ok", 2, 500},
	} {
		if _, err := rt.DecideBatchHuman(c.ids, c.reason, c.confirm, c.limit); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if len(ledger(t, rt)) != n {
		t.Fatal("refused batch wrote events")
	}
}

func TestBatchHumanFinalStopsOnFirstError(t *testing.T) {
	rt, ids := seedAtGOV(t, 3)
	batch := []string{ids[0], "E-0000000000000000", ids[1]}
	res, err := rt.DecideBatchHuman(batch, "partial", 3, 500)
	if err == nil || res.Accepted != 1 || res.StoppedAt != "E-0000000000000000" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	verifyReplay(t, rt, 1)
}

func TestSelectAtGOVFilters(t *testing.T) {
	rt, ids := seedAtGOV(t, 5)
	st, _ := rt.state()
	all, err := SelectAtGOV(st, "all")
	if err != nil || len(all) != 5 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}
	even, err := SelectAtGOV(st, "source_kind=EVEN")
	if err != nil || len(even) != 2 {
		t.Fatalf("even=%v err=%v", even, err)
	}
	if _, err := SelectAtGOV(st, "bogus=1"); err == nil {
		t.Fatal("unknown filter accepted")
	}
	_ = ids
}

func TestBatchHumanFinalScale5000(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	rt, ids := seedAtGOV(t, 5000)
	start := time.Now()
	res, err := rt.DecideBatchHuman(ids, "scale review", 5000, 5000)
	took := time.Since(start)
	if err != nil || res.Accepted != 5000 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	t.Logf("5000-record HUMAN_FINAL batch took %s", took)
	if took > 60*time.Second {
		t.Fatalf("batch too slow: %s", took)
	}
	verifyReplay(t, rt, 5000)
}

// One press over a group larger than the cap: cap-sized chunks share one
// batch id, each record still gets its own HUMAN_FINAL receipt.
func TestBatchHumanFinalChunksShareBatchID(t *testing.T) {
	rt, ids := seedAtGOV(t, 5)
	before := len(ledger(t, rt))
	first, err := rt.DecideBatchHumanAs(ids[:2], "group review", 2, 2, "")
	if err != nil || first.Accepted != 2 || !ValidBatchID(first.BatchID) {
		t.Fatalf("chunk 1: %+v %v", first, err)
	}
	for _, part := range [][]string{ids[2:4], ids[4:]} {
		res, err := rt.DecideBatchHumanAs(part, "group review", len(part), 2, first.BatchID)
		if err != nil || res.BatchID != first.BatchID || res.Accepted != len(part) {
			t.Fatalf("chunk: %+v %v", res, err)
		}
	}
	if _, err := rt.DecideBatchHumanAs(ids[:1], "x", 1, 2, "HFB-not-valid"); err == nil {
		t.Fatal("invalid batch id accepted")
	}
	if _, err := rt.DecideBatchHumanAs(ids, "x", 5, 2, first.BatchID); err == nil {
		t.Fatal("chunk over cap accepted")
	}
	events := ledger(t, rt)[before:]
	if len(events) != 10 {
		t.Fatalf("want 10 events (D,R x5), got %d", len(events))
	}
	seen := map[string]bool{}
	for i := 0; i < len(events); i += 2 {
		d := events[i].Decision
		if d == nil || d.Authority != "HUMAN_FINAL" || !strings.Contains(d.Reason, "[HUMAN_FINAL batch "+first.BatchID+",") || events[i+1].REG.DecisionID != events[i].ID {
			t.Fatalf("receipt %d: %+v", i/2, d)
		}
		seen[d.EmergIONID] = true
	}
	if len(seen) != 5 {
		t.Fatalf("receipts cover %d records", len(seen))
	}
	verifyReplay(t, rt, 5)
}

// HOLD is the other existing decision a batch may carry: each record gets
// its own HUMAN_FINAL HOLD receipt and no REG acceptance; re-runs skip.
func TestBatchHumanFinalHold(t *testing.T) {
	rt, ids := seedAtGOV(t, 4)
	before := len(ledger(t, rt))
	res, err := rt.DecideBatchHumanDecision(ids[1:], "hold the rest", 3, 500, "", "HOLD")
	if err != nil || res.Held != 3 || res.Accepted != 0 {
		t.Fatalf("hold: %+v %v", res, err)
	}
	events := ledger(t, rt)[before:]
	if len(events) != 3 {
		t.Fatalf("want 3 D events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Type != "D" || ev.Decision.Decision != "HOLD" || ev.Decision.Authority != "HUMAN_FINAL" ||
			!strings.Contains(ev.Decision.Reason, "hold the rest [HUMAN_FINAL HOLD batch "+res.BatchID+", 3 records]") {
			t.Fatalf("hold receipt: %+v", ev)
		}
	}
	st, err := livefield.Rebuild(ledger(t, rt))
	if err != nil || len(st.Held) != 3 || len(st.AtGOV) != 1 || len(st.Accepted) != 0 {
		t.Fatalf("rebuild: held=%d atgov=%d accepted=%d err=%v", len(st.Held), len(st.AtGOV), len(st.Accepted), err)
	}
	again, err := rt.DecideBatchHumanDecision(ids[1:], "hold the rest", 3, 500, "", "HOLD")
	if err != nil || again.Skipped != 3 || again.Held != 0 {
		t.Fatalf("re-run: %+v %v", again, err)
	}
	if _, err := rt.DecideBatchHumanDecision(ids[:1], "x", 1, 500, "", "REJECT"); err == nil {
		t.Fatal("decision other than APPROVE/HOLD accepted")
	}
	if _, err := rt.store.VerifyEvidence(); err != nil {
		t.Fatal(err)
	}
}

// /projection.json trajectories: AtGOV provider_population candidates keyed
// by (required_capability, composition, first provider), with count,
// first/last capture time, newest ID and members. Nothing is written.
func TestLinkedProjectionTrajectories(t *testing.T) {
	rt, ids := seedAtGOV(t, 6, func(i int, r map[string]string) {
		if i == 5 {
			return
		}
		r["source_name"] = "provider_population"
		r["required_capability"] = "DERIVE_CAPABILITY"
		r["capability_composition"] = "ANALYZE+CMP+RLT"
		first := "E-AAAAAAAAAAAAAAA1"
		if i == 4 {
			first = "E-AAAAAAAAAAAAAAA2"
		}
		r["capability_providers"] = fmt.Sprintf("ANALYZE:%s,CMP:E-C%015X,RLT:E-D%015X", first, i, i)
	})
	n := len(ledger(t, rt))
	js, err := rt.LinkedProjectionJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger(t, rt)) != n {
		t.Fatal("projection wrote events")
	}
	var lp struct {
		Trajectories []struct {
			Key           string    `json:"key"`
			FirstProvider string    `json:"first_provider"`
			NewestID      string    `json:"newest_id"`
			Count         int       `json:"count"`
			Members       []string  `json:"members"`
			FirstAt       time.Time `json:"first_at"`
			LastAt        time.Time `json:"last_at"`
		} `json:"trajectories"`
	}
	if err := json.Unmarshal([]byte(js), &lp); err != nil {
		t.Fatal(err)
	}
	if len(lp.Trajectories) != 2 {
		t.Fatalf("trajectories = %+v", lp.Trajectories)
	}
	a := lp.Trajectories[0]
	if a.Key != "DERIVE_CAPABILITY|ANALYZE+CMP+RLT|E-AAAAAAAAAAAAAAA1" || a.Count != 3 || len(a.Members) != 3 || a.FirstAt.IsZero() || a.LastAt.Before(a.FirstAt) {
		t.Fatalf("trajectory = %+v", a)
	}
	if a.FirstProvider != "E-AAAAAAAAAAAAAAA1" || a.NewestID != ids[3] {
		t.Fatalf("newest = %s want %s", a.NewestID, ids[3])
	}
	if lp.Trajectories[1].Count != 1 || lp.Trajectories[1].Members[0] != ids[4] {
		t.Fatalf("second trajectory = %+v", lp.Trajectories[1])
	}
}
