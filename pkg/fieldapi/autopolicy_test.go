package fieldapi

import (
	"context"
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/gov"
	"emergion-sovereign-runtime/internal/reason"
	fieldruntime "emergion-sovereign-runtime/internal/runtime"
	"emergion-sovereign-runtime/policy"
)

func ingestLines(t *testing.T, rt *Runtime, lines ...string) []string {
	t.Helper()
	rep, err := (fieldruntime.Runtime{Store: rt.store}).IngestSourceManifest(context.Background(), strings.NewReader(strings.Join(lines, "\n")), fieldruntime.SourceIngestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return rep.CreatedIDs
}

func openTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt, err := Open(t.TempDir(), reason.Heuristic{})
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func decisionsFor(t *testing.T, rt *Runtime, id string) []core.DecisionReceipt {
	t.Helper()
	events, err := rt.store.Events()
	if err != nil {
		t.Fatal(err)
	}
	var out []core.DecisionReceipt
	for _, ev := range events {
		if ev.Decision != nil && ev.Decision.EmergIONID == id {
			out = append(out, *ev.Decision)
		}
	}
	return out
}

func TestAutoPolicyOffDecidesNothing(t *testing.T) {
	rt := openTestRuntime(t)
	ids := ingestLines(t, rt, manifestLine("a1", "Alpha"), manifestLine("a2", "Beta"))
	res, err := rt.ApplyAutoPolicy(0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Effective || res.Accepted+res.Held != 0 {
		t.Fatalf("policy off must decide nothing: %+v", res)
	}
	st, _ := rt.state()
	for _, id := range ids {
		if _, ok := st.AtGOV[id]; !ok {
			t.Fatalf("%s left AtGOV with the policy off", id)
		}
	}
	ps := PolicyStatusOf(st)
	if ps.On || ps.Effective || ps.Eligible != 2 {
		t.Fatalf("status: %+v", ps)
	}
}

func TestAutoPolicyOnAcceptsOnlyLowRiskWithReceipts(t *testing.T) {
	rt := openTestRuntime(t)
	ids := ingestLines(t, rt, manifestLine("b1", "Alpha"), manifestLine("b2", "Beta"))
	// Not low risk: an Archonym-claiming record and a capability record.
	named := captureNamed(t, rt, "Sovereign Name", "SOURCE_UPLOAD", "named body")
	capEm, _, err := (fieldruntime.Runtime{Store: rt.store, Reasoner: fixedResult{name: "t", version: "1", result: reason.Result{
		Summary: "cap", Relationships: map[string]string{"source_kind": "SOURCE_UPLOAD"}, Capabilities: []string{"OBS", "SEND"}, Facts: []string{"f"}, Risk: "L",
	}}}).CaptureBytes(context.Background(), "cap", []byte("cap body"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.SetAutoPolicy(context.Background(), true, ""); err == nil {
		t.Fatal("switching on without a reason must fail")
	}
	switchID, err := rt.SetAutoPolicy(context.Background(), true, "Michael approved A01-AUTO-1")
	if err != nil {
		t.Fatal(err)
	}
	if d := decisionsFor(t, rt, switchID); len(d) != 1 || d[0].Authority != "HUMAN_FINAL" {
		t.Fatalf("switch must be HUMAN_FINAL: %+v", d)
	}
	res, err := rt.ApplyAutoPolicy(0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Effective || res.Accepted != 2 || res.Held != 0 {
		t.Fatalf("expected 2 auto accepts: %+v", res)
	}
	st, err := rt.state()
	if err != nil {
		t.Fatalf("replay after auto decisions: %v", err)
	}
	for _, id := range ids {
		if _, ok := st.Accepted[id]; !ok {
			t.Fatalf("%s not auto-accepted", id)
		}
		d := decisionsFor(t, rt, id)
		if len(d) != 1 || d[0].Authority != policy.Authority || d[0].Authority == "HUMAN_FINAL" {
			t.Fatalf("receipt authority: %+v", d)
		}
		if !strings.Contains(d[0].Reason, "sha256="+policy.SHA256()) || !strings.Contains(d[0].Reason, policy.Version()) ||
			!strings.Contains(d[0].Reason, "class="+policy.ClassSourceUpload) || !strings.Contains(d[0].Reason, "switch="+switchID) {
			t.Fatalf("receipt reason lacks policy identity: %q", d[0].Reason)
		}
	}
	for _, id := range []string{named.IDN, capEm.IDN} {
		if _, ok := st.AtGOV[id]; !ok {
			t.Fatalf("non-low-risk %s must stay AtGOV", id)
		}
		if len(decisionsFor(t, rt, id)) != 0 {
			t.Fatalf("non-low-risk %s got a decision", id)
		}
	}
	ps := PolicyStatusOf(st)
	if !ps.Effective || ps.AutoAccepted != 2 || ps.Waiting != 2 {
		t.Fatalf("status: %+v", ps)
	}
	// Idempotent: a second pass decides nothing.
	res, _ = rt.ApplyAutoPolicy(0, "")
	if res.Accepted+res.Held != 0 {
		t.Fatalf("second pass decided again: %+v", res)
	}
	// Switch off: new uploads stay AtGOV.
	if _, err := rt.SetAutoPolicy(context.Background(), false, "pause"); err != nil {
		t.Fatal(err)
	}
	late := ingestLines(t, rt, manifestLine("b3", "Gamma"))
	res, _ = rt.ApplyAutoPolicy(0, "")
	if res.Effective || res.Accepted != 0 {
		t.Fatalf("policy off decided: %+v", res)
	}
	st, _ = rt.state()
	if _, ok := st.AtGOV[late[0]]; !ok {
		t.Fatal("late upload left AtGOV with the policy off")
	}
}

func TestAutoPolicyUndo(t *testing.T) {
	rt := openTestRuntime(t)
	ids := ingestLines(t, rt, manifestLine("c1", "Alpha"))
	if _, err := rt.SetAutoPolicy(context.Background(), true, "on"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ApplyAutoPolicy(0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.UndoAuto(ids[0], ""); err == nil {
		t.Fatal("undo without reason must fail")
	}
	out, err := rt.UndoAuto(ids[0], "not sure about this one")
	if err != nil || out != "HELD" {
		t.Fatalf("undo: %q %v", out, err)
	}
	st, err := rt.state()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Held[ids[0]]; !ok {
		t.Fatal("reverted record is not held")
	}
	d := decisionsFor(t, rt, ids[0])
	if len(d) != 2 || d[1].Decision != "REVERT" || d[1].Authority != "HUMAN_FINAL" {
		t.Fatalf("revert receipt: %+v", d)
	}
	// Held by a human revert: the policy must not touch it, and a second undo is refused.
	if res, _ := rt.ApplyAutoPolicy(0, ""); res.Accepted+res.Held != 0 {
		t.Fatalf("policy re-decided a reverted record: %+v", res)
	}
	if _, err := rt.UndoAuto(ids[0], "again"); err == nil {
		t.Fatal("undo of a record with no auto decision must fail")
	}
	// A human-accepted record can never be reverted through the auto undo.
	human := ingestLines(t, rt, manifestLine("c2", "Beta"))
	if err := rt.Decide(human[0], "APPROVE", "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.UndoAuto(human[0], "x"); err == nil {
		t.Fatal("auto undo of a HUMAN_FINAL accept must fail")
	}
}

func TestAutoPolicyReplayRejectsForgedAutoDecision(t *testing.T) {
	rt := openTestRuntime(t)
	ids := ingestLines(t, rt, manifestLine("e1", "Alpha"))
	named := captureNamed(t, rt, "Forged Name", "SOURCE_UPLOAD", "forged body")
	st, _ := rt.state()
	// Policy off: an AUTO_POLICY receipt fails replay.
	em := st.AtGOV[ids[0]]
	_, receipt, err := gov.DecideAuto(em, gov.Approve, policy.ReasonText(policy.Verdict{Class: policy.ClassSourceUpload}, "E-NONE"))
	if err != nil {
		t.Fatal(err)
	}
	events, _ := rt.store.Events()
	forged := append(append([]core.Event(nil), events...), core.Event{Type: "D", ID: "EV-X", Decision: &receipt})
	if _, err := livefield.Rebuild(forged); err == nil {
		t.Fatal("AUTO_POLICY receipt with the policy off must fail replay")
	}
	// Policy on: an AUTO_POLICY APPROVE of an Archonym record fails replay.
	if _, err := rt.SetAutoPolicy(context.Background(), true, "on"); err != nil {
		t.Fatal(err)
	}
	st, _ = rt.state()
	_, receipt, _ = gov.DecideAuto(st.AtGOV[named.IDN], gov.Approve, policy.ReasonText(policy.Verdict{Class: policy.ClassSourceUpload}, st.AutoPolicy.SwitchID))
	events, _ = rt.store.Events()
	forged = append(append([]core.Event(nil), events...), core.Event{Type: "D", ID: "EV-Y", Decision: &receipt})
	if _, err := livefield.Rebuild(forged); err == nil {
		t.Fatal("AUTO_POLICY approve of an Archonym record must fail replay")
	}
	// gov refuses anything but APPROVE/HOLD for the policy.
	if _, _, err := gov.DecideAuto(st.AtGOV[ids[0]], gov.Reject, policy.ReasonText(policy.Verdict{}, "x")); err == nil {
		t.Fatal("AUTO_POLICY REJECT must be refused")
	}
	// gov.Decide still refuses the AUTO_POLICY authority.
	if _, _, err := gov.Decide(st.AtGOV[ids[0]], gov.Approve, policy.Authority, "x"); err == nil {
		t.Fatal("gov.Decide must stay HUMAN_FINAL only")
	}
}
