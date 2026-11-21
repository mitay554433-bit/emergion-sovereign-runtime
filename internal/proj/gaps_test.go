package proj

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/policy"
)

func wireFixtureState() core.State {
	st := core.EmptyState()
	st.TipHash = "tip-fixture"
	st.Events = 9
	up := func(id, title string) core.EmergION {
		return core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{"source_kind": "SOURCE_UPLOAD", "source_title": title, "source_topic": "inventions-ip"}, MEM: core.Memory{Summary: "upload " + title}, CAP: []string{"OBS"}}
	}
	st.Accepted["E-u1"] = up("E-u1", "Helix brief 1")
	st.Accepted["E-u2"] = up("E-u2", "Helix brief 2")
	ins := core.EmergION{IDN: "E-i1", STA: core.StateAccepted, REL: map[string]string{"source_kind": "SOURCE_INSIGHT", "insight_of": "E-u1"}, MEM: core.Memory{Summary: "Insight on Helix brief 1"}, CAP: []string{"OBS"}}
	ins.VAL.Gaps = []string{"No backup after reboot"}
	st.Accepted["E-i1"] = ins
	st.Accepted["E-l1"] = core.EmergION{IDN: "E-l1", STA: core.StateAccepted, REL: map[string]string{"source_kind": "SOURCE_KIN_LINK", "kin_link_from": "E-u1", "kin_link_to": "E-u2"}, CAP: []string{"OBS"}}
	st.AtGOV["E-g0"] = core.EmergION{IDN: "E-g0", STA: core.StateAtGOV, MEM: core.Memory{Summary: "pending upload"}}
	return st
}

func gapEm(id, state, of, class string) core.EmergION {
	return core.EmergION{IDN: id, STA: state, MEM: core.Memory{Summary: "Gap (" + class + ") in X: y"}, REL: map[string]string{
		"source_kind": policy.KindGap, "gap_of": of, "gap_class": class, "gap_basis": "E-i1", "gap_pivot": policy.GapPivot[class],
	}}
}

func TestLinkedGapsGroupedByClassWithCounts(t *testing.T) {
	st := wireFixtureState()
	st.AtGOV["E-gp1"] = gapEm("E-gp1", core.StateAtGOV, "E-u1", policy.GapProof)
	st.AtGOV["E-gc1"] = gapEm("E-gc1", core.StateAtGOV, "E-u1", policy.GapContinuity)
	st.AtGOV["E-gc2"] = gapEm("E-gc2", core.StateAtGOV, "E-u2", policy.GapContinuity)
	st.Accepted["E-gc3"] = gapEm("E-gc3", core.StateAccepted, "E-u2", policy.GapContinuity)
	st.Held["E-gc4"] = gapEm("E-gc4", "HELD", "E-u1", policy.GapContinuity)
	st.AtGOV["E-go1"] = gapEm("E-go1", core.StateAtGOV, "E-u2", policy.GapOperational)

	lp := Linked(st)
	if len(lp.Gaps) != 3 {
		t.Fatalf("gap groups %+v", lp.Gaps)
	}
	if lp.Gaps[0].Class != policy.GapOperational || lp.Gaps[1].Class != policy.GapContinuity || lp.Gaps[2].Class != policy.GapProof {
		t.Fatalf("class order %s %s %s", lp.Gaps[0].Class, lp.Gaps[1].Class, lp.Gaps[2].Class)
	}
	c := lp.Gaps[1]
	if c.AtGOV != 2 || c.Accepted != 1 || c.Held != 1 || len(c.Items) != 3 || c.Pivot != "PIVOT_CONTINUITY_RECOVERY" {
		t.Fatalf("continuity group %+v", c)
	}
	if c.Items[0].State != "AtGOV" || c.Items[2].State != "REG_ACCEPTED" || c.Items[0].SourceTitle == "" {
		t.Fatalf("continuity items %+v", c.Items)
	}
	if lp.Gaps[2].AtGOV != 1 || lp.Gaps[2].Items[0].SourceTitle != "Helix brief 1" {
		t.Fatalf("proof group %+v", lp.Gaps[2])
	}
	// The projection writes nothing.
	if len(st.AtGOV) != 5 || len(st.Accepted) != 5 || len(st.Held) != 1 {
		t.Fatal("state mutated")
	}
}

// Wire additivity: the projection JSON for a state with no gaps equals the
// f3fb0d5 output once the two additive keys (gaps, records[].kin_family) are
// removed, and the old JSON decodes into the new struct unchanged.
func TestLinkedWireIsAdditiveOverF3fb0d5(t *testing.T) {
	oldB, err := os.ReadFile("testdata/linked_f3fb0d5.json")
	if err != nil {
		t.Fatal(err)
	}
	var oldM map[string]any
	if err := json.Unmarshal(oldB, &oldM); err != nil {
		t.Fatal(err)
	}
	newB, _ := json.Marshal(Linked(wireFixtureState()))
	var newM map[string]any
	if err := json.Unmarshal(newB, &newM); err != nil {
		t.Fatal(err)
	}
	if g, ok := newM["gaps"].([]any); !ok || len(g) != 0 {
		t.Fatalf("gaps = %v, want []", newM["gaps"])
	}
	delete(newM, "gaps")
	delete(newM, "gap_summary") // additive object (counts only)
	if _, ok := newM["temporal"]; ok {
		t.Fatal("Linked(st) without events must not emit temporal")
	}
	for _, r := range newM["records"].([]any) {
		delete(r.(map[string]any), "kin_family")
	}
	for _, g := range newM["derived_facet_groups"].([]any) {
		delete(g.(map[string]any), "source_uploads")
	}
	if !reflect.DeepEqual(oldM, newM) {
		t.Fatalf("old wire changed:\nold %s\nnew %s", oldB, newB)
	}
	var lp LinkedProjection
	if err := json.Unmarshal(oldB, &lp); err != nil {
		t.Fatalf("old JSON does not decode: %v", err)
	}
	back, _ := json.Marshal(lp)
	var backM map[string]any
	_ = json.Unmarshal(back, &backM)
	if backM["gaps"] != nil {
		t.Fatalf("decoded old JSON grew gaps: %v", backM["gaps"])
	}
	delete(backM, "gaps")
	delete(backM, "gap_summary") // additive, zero counts
	for _, g := range backM["derived_facet_groups"].([]any) {
		delete(g.(map[string]any), "source_uploads") // additive, defaults to 0
	}
	if !reflect.DeepEqual(oldM, backM) {
		t.Fatalf("old JSON round-trip changed:\n%s", back)
	}
}
