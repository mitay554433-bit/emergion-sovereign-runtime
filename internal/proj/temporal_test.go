package proj

import (
	"encoding/json"
	"testing"
	"time"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/policy"
)

func TestTemporalIsLiveFromLedgerEvents(t *testing.T) {
	t0 := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	em := func(id string) *core.EmergION { return &core.EmergION{IDN: id} }
	ev := []core.Event{
		{Type: "C", At: t0, EmergION: em("E-1")},
		{Type: "C", At: t0.Add(time.Minute), EmergION: em("E-2")},
		{Type: "I", At: t0.Add(2 * time.Minute), EmergION: em("E-1")},
		{Type: "I", At: t0.Add(3 * time.Minute), EmergION: em("E-1")},
		{Type: "D", At: t0.Add(4 * time.Minute), Decision: &core.DecisionReceipt{EmergIONID: "E-1", Decision: "APPROVE"}},
		{Type: "R", At: t0.Add(5 * time.Minute), REG: &core.REGReceipt{EmergIONID: "E-1"}},
		{Type: "D", At: t0.Add(-48 * time.Hour), Decision: &core.DecisionReceipt{EmergIONID: "E-2", Decision: "HOLD"}},
		// the newest event is a decision, not a capture: it is the last update
		{Type: "D", At: t0.Add(90 * time.Minute), Decision: &core.DecisionReceipt{EmergIONID: "E-2", Decision: "APPROVE"}},
	}
	tm := Temporal(core.State{TipHash: "tip"}, ev)
	// E-1: C, I1, I2, DAPPROVE, R = 5 ; E-2: C, DHOLD, DAPPROVE = 3
	if tm.Source != "ledger" || tm.Events != 8 || tm.States != 8 || tm.Records != 2 || tm.Revisions != 2 {
		t.Fatalf("%+v", tm)
	}
	if tm.TipAt != "2026-11-20T05:30:00Z" || tm.TipKind != "DECISION" || tm.LastCaptureAt != "2026-11-20T04:01:00Z" || tm.LastREGAt != "2026-11-20T04:05:00Z" {
		t.Fatalf("times %+v", tm)
	}
	if tm.FirstAt != "2026-11-18T04:00:00Z" || tm.Days != 2 || tm.Last24h != 7 || tm.TipHash != "tip" {
		t.Fatalf("window %+v", tm)
	}
	if tm.ByKind["DECISION"] != 3 || tm.ByKind["CANDIDATE_CAPTURED"] != 2 {
		t.Fatalf("by kind %v", tm.ByKind)
	}
	// one more event moves the view (it is not a fixed artifact)
	ev = append(ev, core.Event{Type: "R", At: t0.Add(2 * time.Hour), REG: &core.REGReceipt{EmergIONID: "E-2"}})
	if tm2 := Temporal(core.State{}, ev); tm2.States != 9 || tm2.TipAt != "2026-11-20T06:00:00Z" || tm2.TipKind != "REG_ACCEPTED" {
		t.Fatalf("not live: %+v", tm2)
	}
	empty := Temporal(core.State{}, nil)
	b, _ := json.Marshal(empty)
	if empty.States != 0 || empty.TipAt != "" || string(b) == "" {
		t.Fatalf("empty %+v", empty)
	}
}

func TestGapSummaryCountsEveryState(t *testing.T) {
	gap := func(id, class, of string) core.EmergION {
		return core.EmergION{IDN: id, REL: map[string]string{"source_kind": policy.KindGap, "gap_class": class, "gap_of": of}}
	}
	st := core.State{
		AtGOV:    map[string]core.EmergION{"g1": gap("g1", "proof", "u1"), "g2": gap("g2", "continuity", "u1")},
		Accepted: map[string]core.EmergION{"g3": gap("g3", "proof", "u2"), "i1": {IDN: "i1", REL: map[string]string{"source_kind": policy.KindInsight}}, "u1": {IDN: "u1"}},
		Held:     map[string]core.EmergION{"g4": gap("g4", "", "u3")},
		Rejected: map[string]core.EmergION{"g5": gap("g5", "proof", "u2")},
	}
	s := Linked(st).GapSummary
	if s.Records != 5 || s.AtGOV != 2 || s.Accepted != 1 || s.Held != 1 || s.Other != 1 || s.Classes != 3 || s.Sources != 3 || s.InsightsAccepted != 1 {
		t.Fatalf("%+v", s)
	}
	if z := Linked(core.State{}).GapSummary; z != (LinkedGapSummary{}) {
		t.Fatalf("empty %+v", z)
	}
}
